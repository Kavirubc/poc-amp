package container

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

// ContainerInfo holds information about a tracked container
type ContainerInfo struct {
	ID        string
	Name      string
	AgentID   string
	AgentName string
	CgroupID  uint64
	PID       int
	Running   bool
}

// ContainerHandler is called when container events occur
type ContainerHandler func(event string, info *ContainerInfo)

// Tracker watches Docker for AMP-managed containers
type Tracker struct {
	client     *client.Client
	containers map[string]*ContainerInfo // container ID -> info
	cgroupMap  map[uint64]string         // cgroup ID -> container ID
	handler    ContainerHandler
	mu         sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewTracker creates a new container tracker
func NewTracker(handler ContainerHandler) (*Tracker, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("creating Docker client: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Tracker{
		client:     cli,
		containers: make(map[string]*ContainerInfo),
		cgroupMap:  make(map[uint64]string),
		handler:    handler,
		ctx:        ctx,
		cancel:     cancel,
	}, nil
}

// Start begins watching for container events
func (t *Tracker) Start() error {
	// First, discover existing AMP containers
	if err := t.discoverExisting(); err != nil {
		log.Printf("Warning: error discovering existing containers: %v", err)
	}

	// Watch for new container events
	go t.watchEvents()

	log.Println("Container tracker started")
	return nil
}

// discoverExisting finds all currently running AMP-managed containers
func (t *Tracker) discoverExisting() error {
	filterArgs := filters.NewArgs()
	filterArgs.Add("label", "amp.managed=true")

	containers, err := t.client.ContainerList(t.ctx, types.ContainerListOptions{
		Filters: filterArgs,
	})
	if err != nil {
		return fmt.Errorf("listing containers: %w", err)
	}

	for _, container := range containers {
		if container.State == "running" {
			info, err := t.inspectContainer(container.ID)
			if err != nil {
				log.Printf("Warning: could not inspect container %s: %v", container.ID[:12], err)
				continue
			}

			t.mu.Lock()
			t.containers[container.ID] = info
			if info.CgroupID > 0 {
				t.cgroupMap[info.CgroupID] = container.ID
			}
			t.mu.Unlock()

			if t.handler != nil {
				t.handler("start", info)
			}

			log.Printf("Discovered existing container: %s (agent: %s)", info.Name, info.AgentID)
		}
	}

	return nil
}

// watchEvents watches Docker events for container start/stop
func (t *Tracker) watchEvents() {
	filterArgs := filters.NewArgs()
	filterArgs.Add("type", "container")
	filterArgs.Add("event", "start")
	filterArgs.Add("event", "die")
	filterArgs.Add("label", "amp.managed=true")

	eventCh, errCh := t.client.Events(t.ctx, types.EventsOptions{
		Filters: filterArgs,
	})

	for {
		select {
		case <-t.ctx.Done():
			return

		case event := <-eventCh:
			t.handleEvent(event)

		case err := <-errCh:
			if err != nil && t.ctx.Err() == nil {
				log.Printf("Docker events error: %v", err)
			}
			return
		}
	}
}

func (t *Tracker) handleEvent(event events.Message) {
	containerID := event.Actor.ID

	switch event.Action {
	case "start":
		info, err := t.inspectContainer(containerID)
		if err != nil {
			log.Printf("Error inspecting container %s: %v", containerID[:12], err)
			return
		}

		t.mu.Lock()
		t.containers[containerID] = info
		if info.CgroupID > 0 {
			t.cgroupMap[info.CgroupID] = containerID
		}
		t.mu.Unlock()

		if t.handler != nil {
			t.handler("start", info)
		}

		log.Printf("Container started: %s (agent: %s, cgroup: %d)", info.Name, info.AgentID, info.CgroupID)

	case "die":
		t.mu.Lock()
		info, exists := t.containers[containerID]
		if exists {
			info.Running = false
			delete(t.cgroupMap, info.CgroupID)
			delete(t.containers, containerID)
		}
		t.mu.Unlock()

		if exists && t.handler != nil {
			t.handler("stop", info)
		}

		if exists {
			log.Printf("Container stopped: %s (agent: %s)", info.Name, info.AgentID)
		}
	}
}

func (t *Tracker) inspectContainer(containerID string) (*ContainerInfo, error) {
	inspect, err := t.client.ContainerInspect(t.ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("inspecting container: %w", err)
	}

	info := &ContainerInfo{
		ID:      containerID,
		Name:    strings.TrimPrefix(inspect.Name, "/"),
		Running: inspect.State.Running,
		PID:     inspect.State.Pid,
	}

	// Extract AMP labels
	if labels := inspect.Config.Labels; labels != nil {
		info.AgentID = labels["amp.id"]
		info.AgentName = labels["amp.agent"]
	}

	// Get cgroup ID
	if inspect.State.Pid > 0 {
		cgroupID, err := getCgroupID(inspect.State.Pid)
		if err != nil {
			log.Printf("Warning: could not get cgroup ID for PID %d: %v", inspect.State.Pid, err)
		} else {
			info.CgroupID = cgroupID
		}
	}

	return info, nil
}

// getCgroupID gets the cgroup ID for a process
func getCgroupID(pid int) (uint64, error) {
	// Read cgroup path from /proc/PID/cgroup
	cgroupPath := fmt.Sprintf("/proc/%d/cgroup", pid)
	data, err := os.ReadFile(cgroupPath)
	if err != nil {
		return 0, fmt.Errorf("reading cgroup file: %w", err)
	}

	// Parse cgroup path
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		parts := strings.Split(line, ":")
		if len(parts) >= 3 {
			// Get the cgroup path
			path := parts[2]
			if path != "" {
				// Get inode of the cgroup directory
				cgroupDir := filepath.Join("/sys/fs/cgroup", path)
				stat, err := os.Stat(cgroupDir)
				if err != nil {
					// Try unified cgroup (cgroup v2)
					cgroupDir = filepath.Join("/sys/fs/cgroup/unified", path)
					stat, err = os.Stat(cgroupDir)
					if err != nil {
						continue
					}
				}

				// Use the inode as cgroup ID (simplified approach)
				// In production, use proper cgroup ID from kernel
				if sysStat, ok := stat.Sys().(interface{ Ino() uint64 }); ok {
					return sysStat.Ino(), nil
				}
			}
		}
	}

	// Fallback: use PID as a pseudo cgroup ID
	return uint64(pid), nil
}

// GetContainerByID returns container info by ID
func (t *Tracker) GetContainerByID(containerID string) *ContainerInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.containers[containerID]
}

// GetContainerByCgroup returns container info by cgroup ID
func (t *Tracker) GetContainerByCgroup(cgroupID uint64) *ContainerInfo {
	t.mu.RLock()
	containerID, ok := t.cgroupMap[cgroupID]
	t.mu.RUnlock()

	if !ok {
		return nil
	}

	return t.GetContainerByID(containerID)
}

// GetContainerByPID returns container info by process ID
func (t *Tracker) GetContainerByPID(pid uint32) *ContainerInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()

	for _, info := range t.containers {
		if uint32(info.PID) == pid {
			return info
		}
		// Check if PID is a child of container's init process
		if isChildProcess(int(pid), info.PID) {
			return info
		}
	}

	return nil
}

// isChildProcess checks if pid is a child of parentPID
func isChildProcess(pid, parentPID int) bool {
	if parentPID <= 0 {
		return false
	}

	// Read parent PID from /proc/PID/stat
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	data, err := os.ReadFile(statPath)
	if err != nil {
		return false
	}

	// Parse stat file - format: pid (comm) state ppid ...
	// Find the closing parenthesis to skip the command name
	s := string(data)
	closeIdx := strings.LastIndex(s, ")")
	if closeIdx < 0 {
		return false
	}

	// Parse fields after the command
	fields := strings.Fields(s[closeIdx+1:])
	if len(fields) < 2 {
		return false
	}

	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return false
	}

	if ppid == parentPID {
		return true
	}

	// Recursively check parent
	if ppid > 1 {
		return isChildProcess(ppid, parentPID)
	}

	return false
}

// ListContainers returns all tracked containers
func (t *Tracker) ListContainers() []*ContainerInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]*ContainerInfo, 0, len(t.containers))
	for _, info := range t.containers {
		result = append(result, info)
	}
	return result
}

// Stop stops the tracker
func (t *Tracker) Stop() error {
	t.cancel()

	if t.client != nil {
		t.client.Close()
	}

	log.Println("Container tracker stopped")
	return nil
}
