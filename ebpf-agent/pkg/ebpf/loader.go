package ebpf

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/vishvananda/netlink"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags "-O2 -g -Wall -Werror" -target amd64 bpf ../../bpf/http_capture.c -- -I../../bpf

const (
	EventHTTPRequest  = 1
	EventHTTPResponse = 2
)

// HTTPEvent represents a captured HTTP event
type HTTPEvent struct {
	EventType uint32
	SrcIP     net.IP
	DstIP     net.IP
	SrcPort   uint16
	DstPort   uint16
	PID       uint32
	Timestamp time.Time
	IfIndex   uint32
	Data      []byte
	AgentID   string // Filled in by container tracker
}

// rawHTTPEvent is the raw structure from eBPF
type rawHTTPEvent struct {
	EventType uint32
	SrcIP     uint32
	DstIP     uint32
	SrcPort   uint16
	DstPort   uint16
	PID       uint32
	DataLen   uint32
	Timestamp uint64
	IfIndex   uint32
	Data      [1024]byte
}

// EventHandler is called for each captured HTTP event
type EventHandler func(event *HTTPEvent)

// Loader manages eBPF program loading and event processing
type Loader struct {
	objs          *bpfObjects
	links         []link.Link
	qdisc         *netlink.GenericQdisc
	filter        *netlink.BpfFilter
	reader        *ringbuf.Reader
	handler       EventHandler
	containerMap  map[uint64]string // cgroup_id -> agent_id
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewLoader creates a new eBPF loader
func NewLoader(handler EventHandler) (*Loader, error) {
	ctx, cancel := context.WithCancel(context.Background())

	return &Loader{
		handler:      handler,
		containerMap: make(map[uint64]string),
		ctx:          ctx,
		cancel:       cancel,
	}, nil
}

// Load loads the eBPF programs
func (l *Loader) Load() error {
	// Load pre-compiled eBPF programs
	l.objs = &bpfObjects{}
	if err := loadBpfObjects(l.objs, nil); err != nil {
		return fmt.Errorf("loading eBPF objects: %w", err)
	}

	return nil
}

// AttachToInterface attaches TC programs to a network interface
func (l *Loader) AttachToInterface(ifaceName string) error {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return fmt.Errorf("finding interface %s: %w", ifaceName, err)
	}

	// Create clsact qdisc
	link, err := netlink.LinkByIndex(iface.Index)
	if err != nil {
		return fmt.Errorf("getting link: %w", err)
	}

	qdisc := &netlink.GenericQdisc{
		QdiscAttrs: netlink.QdiscAttrs{
			LinkIndex: link.Attrs().Index,
			Handle:    netlink.MakeHandle(0xffff, 0),
			Parent:    netlink.HANDLE_CLSACT,
		},
		QdiscType: "clsact",
	}

	if err := netlink.QdiscAdd(qdisc); err != nil {
		// Ignore if already exists
		if !errors.Is(err, os.ErrExist) {
			log.Printf("Warning: could not add qdisc (may already exist): %v", err)
		}
	}
	l.qdisc = qdisc

	// Attach TC filter for egress (outgoing traffic)
	filter := &netlink.BpfFilter{
		FilterAttrs: netlink.FilterAttrs{
			LinkIndex: link.Attrs().Index,
			Parent:    netlink.HANDLE_MIN_EGRESS,
			Handle:    1,
			Priority:  1,
			Protocol:  0x0003, // ETH_P_ALL
		},
		Fd:           l.objs.CaptureHttp.FD(),
		Name:         "amp_http_capture",
		DirectAction: true,
	}

	if err := netlink.FilterAdd(filter); err != nil {
		return fmt.Errorf("adding TC filter: %w", err)
	}
	l.filter = filter

	log.Printf("Attached TC program to interface %s", ifaceName)
	return nil
}

// AttachKprobes attaches kprobe programs for socket-level capture
func (l *Loader) AttachKprobes() error {
	// Attach to tcp_sendmsg
	sendLink, err := link.Kprobe("tcp_sendmsg", l.objs.TraceTcpSendmsg, nil)
	if err != nil {
		log.Printf("Warning: could not attach tcp_sendmsg kprobe: %v", err)
	} else {
		l.links = append(l.links, sendLink)
		log.Println("Attached kprobe to tcp_sendmsg")
	}

	// Attach to tcp_recvmsg
	recvLink, err := link.Kprobe("tcp_recvmsg", l.objs.TraceTcpRecvmsg, nil)
	if err != nil {
		log.Printf("Warning: could not attach tcp_recvmsg kprobe: %v", err)
	} else {
		l.links = append(l.links, recvLink)
		log.Println("Attached kprobe to tcp_recvmsg")
	}

	return nil
}

// TrackContainer adds a container to the tracking map
func (l *Loader) TrackContainer(cgroupID uint64, agentID string) error {
	// Add to eBPF map
	one := uint32(1)
	if err := l.objs.AmpContainers.Put(cgroupID, one); err != nil {
		return fmt.Errorf("adding container to eBPF map: %w", err)
	}

	// Add to local map for agent ID lookup
	l.containerMap[cgroupID] = agentID

	log.Printf("Tracking container: cgroup=%d, agent=%s", cgroupID, agentID)
	return nil
}

// UntrackContainer removes a container from tracking
func (l *Loader) UntrackContainer(cgroupID uint64) error {
	if err := l.objs.AmpContainers.Delete(cgroupID); err != nil {
		if !errors.Is(err, ebpf.ErrKeyNotExist) {
			return fmt.Errorf("removing container from eBPF map: %w", err)
		}
	}

	delete(l.containerMap, cgroupID)
	log.Printf("Untracked container: cgroup=%d", cgroupID)
	return nil
}

// Start begins reading events from the ring buffer
func (l *Loader) Start() error {
	reader, err := ringbuf.NewReader(l.objs.Rb)
	if err != nil {
		return fmt.Errorf("creating ring buffer reader: %w", err)
	}
	l.reader = reader

	go l.readLoop()
	log.Println("Started eBPF event reader")
	return nil
}

func (l *Loader) readLoop() {
	for {
		select {
		case <-l.ctx.Done():
			return
		default:
		}

		record, err := l.reader.Read()
		if err != nil {
			if errors.Is(err, ringbuf.ErrClosed) {
				return
			}
			log.Printf("Error reading from ring buffer: %v", err)
			continue
		}

		event := l.parseEvent(record.RawSample)
		if event != nil && l.handler != nil {
			l.handler(event)
		}
	}
}

func (l *Loader) parseEvent(data []byte) *HTTPEvent {
	if len(data) < 36 { // Minimum size without data
		return nil
	}

	raw := &rawHTTPEvent{}
	raw.EventType = binary.LittleEndian.Uint32(data[0:4])
	raw.SrcIP = binary.LittleEndian.Uint32(data[4:8])
	raw.DstIP = binary.LittleEndian.Uint32(data[8:12])
	raw.SrcPort = binary.LittleEndian.Uint16(data[12:14])
	raw.DstPort = binary.LittleEndian.Uint16(data[14:16])
	raw.PID = binary.LittleEndian.Uint32(data[16:20])
	raw.DataLen = binary.LittleEndian.Uint32(data[20:24])
	raw.Timestamp = binary.LittleEndian.Uint64(data[24:32])
	raw.IfIndex = binary.LittleEndian.Uint32(data[32:36])

	dataStart := 36
	dataLen := int(raw.DataLen)
	if dataLen > len(data)-dataStart {
		dataLen = len(data) - dataStart
	}

	httpData := make([]byte, dataLen)
	copy(httpData, data[dataStart:dataStart+dataLen])

	event := &HTTPEvent{
		EventType: raw.EventType,
		SrcIP:     intToIP(raw.SrcIP),
		DstIP:     intToIP(raw.DstIP),
		SrcPort:   raw.SrcPort,
		DstPort:   raw.DstPort,
		PID:       raw.PID,
		Timestamp: time.Unix(0, int64(raw.Timestamp)),
		IfIndex:   raw.IfIndex,
		Data:      httpData,
	}

	// Try to find agent ID from PID's cgroup
	// This is a simplified version - in production, use cgroup lookup
	// For now, we'll rely on the container tracker to correlate

	return event
}

func intToIP(ip uint32) net.IP {
	result := make(net.IP, 4)
	binary.LittleEndian.PutUint32(result, ip)
	return result
}

// Stop stops the loader and cleans up resources
func (l *Loader) Stop() error {
	l.cancel()

	if l.reader != nil {
		l.reader.Close()
	}

	// Remove TC filter
	if l.filter != nil {
		netlink.FilterDel(l.filter)
	}

	// Remove qdisc
	if l.qdisc != nil {
		netlink.QdiscDel(l.qdisc)
	}

	// Close kprobe links
	for _, lnk := range l.links {
		lnk.Close()
	}

	// Close eBPF objects
	if l.objs != nil {
		l.objs.Close()
	}

	log.Println("eBPF loader stopped")
	return nil
}
