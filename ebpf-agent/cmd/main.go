package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/google/uuid"
	"github.com/poc-amp/ebpf-agent/pkg/classifier"
	"github.com/poc-amp/ebpf-agent/pkg/container"
	ebpfpkg "github.com/poc-amp/ebpf-agent/pkg/ebpf"
	"github.com/poc-amp/ebpf-agent/pkg/parser"
	"github.com/poc-amp/ebpf-agent/pkg/store"
)

var (
	ampURL    = flag.String("amp-url", "http://localhost:8080", "AMP backend URL")
	iface     = flag.String("iface", "", "Network interface to attach to (default: docker0 or br-*)")
	logLevel  = flag.String("log-level", "info", "Log level: debug, info, warn, error")
)

type Agent struct {
	ebpfLoader       *ebpfpkg.Loader
	containerTracker *container.Tracker
	httpParser       *parser.Parser
	classifier       *classifier.Classifier
	ampClient        *store.AMPClient

	// Track tool calls by connection for request/response correlation
	pendingCalls map[string]*parser.ToolCall
}

func main() {
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting eBPF Agent for AMP")

	agent := &Agent{
		httpParser:   parser.NewParser(),
		classifier:   classifier.NewClassifier(),
		ampClient:    store.NewAMPClient(*ampURL),
		pendingCalls: make(map[string]*parser.ToolCall),
	}

	// Initialize container tracker
	var err error
	agent.containerTracker, err = container.NewTracker(agent.onContainerEvent)
	if err != nil {
		log.Fatalf("Failed to create container tracker: %v", err)
	}

	// Initialize eBPF loader
	agent.ebpfLoader, err = ebpfpkg.NewLoader(agent.onHTTPEvent)
	if err != nil {
		log.Fatalf("Failed to create eBPF loader: %v", err)
	}

	// Load eBPF programs
	if err := agent.ebpfLoader.Load(); err != nil {
		log.Fatalf("Failed to load eBPF programs: %v", err)
	}

	// Start container tracker
	if err := agent.containerTracker.Start(); err != nil {
		log.Fatalf("Failed to start container tracker: %v", err)
	}

	// Attach to network interface
	ifaceName := *iface
	if ifaceName == "" {
		ifaceName = findDockerInterface()
	}

	if ifaceName != "" {
		if err := agent.ebpfLoader.AttachToInterface(ifaceName); err != nil {
			log.Printf("Warning: could not attach to interface %s: %v", ifaceName, err)
		}
	} else {
		log.Println("Warning: no network interface found, only kprobe-based capture will work")
	}

	// Attach kprobes
	if err := agent.ebpfLoader.AttachKprobes(); err != nil {
		log.Printf("Warning: could not attach kprobes: %v", err)
	}

	// Start event reader
	if err := agent.ebpfLoader.Start(); err != nil {
		log.Fatalf("Failed to start eBPF event reader: %v", err)
	}

	log.Println("eBPF Agent is running. Press Ctrl+C to stop.")

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down...")

	agent.ebpfLoader.Stop()
	agent.containerTracker.Stop()

	log.Println("eBPF Agent stopped")
}

func (a *Agent) onContainerEvent(event string, info *container.ContainerInfo) {
	switch event {
	case "start":
		// Track this container in eBPF
		if info.CgroupID > 0 {
			if err := a.ebpfLoader.TrackContainer(info.CgroupID, info.AgentID); err != nil {
				log.Printf("Error tracking container %s: %v", info.Name, err)
			}
		}
		log.Printf("Tracking container: %s (agent: %s)", info.Name, info.AgentID)

	case "stop":
		// Untrack container
		if info.CgroupID > 0 {
			if err := a.ebpfLoader.UntrackContainer(info.CgroupID); err != nil {
				log.Printf("Error untracking container %s: %v", info.Name, err)
			}
		}
		log.Printf("Untracked container: %s", info.Name)
	}
}

func (a *Agent) onHTTPEvent(event *ebpfpkg.HTTPEvent) {
	// Try to correlate with container
	containerInfo := a.containerTracker.GetContainerByPID(event.PID)
	if containerInfo != nil {
		event.AgentID = containerInfo.AgentID
	}

	// Parse HTTP event
	tx, err := a.httpParser.Parse(event)
	if err != nil {
		if *logLevel == "debug" {
			log.Printf("Failed to parse HTTP event: %v", err)
		}
		return
	}

	// Generate connection key for request/response correlation
	connKey := generateConnKey(tx)

	if tx.IsRequest {
		// Extract tool call from request
		toolCall := a.httpParser.ExtractToolCall(tx)
		if toolCall == nil {
			return
		}

		toolCall.ID = uuid.New().String()

		// Store for response correlation
		a.pendingCalls[connKey] = toolCall

		// Log the request
		log.Printf("[%s] Request: %s %s (agent: %s)",
			toolCall.ID[:8], toolCall.Endpoint.Method, toolCall.Endpoint.URL, toolCall.AgentID)

	} else {
		// Response - correlate with request
		toolCall, ok := a.pendingCalls[connKey]
		if !ok {
			return
		}
		delete(a.pendingCalls, connKey)

		// Update tool call with response
		a.httpParser.UpdateToolCallWithResponse(toolCall, tx)

		// Log the response
		log.Printf("[%s] Response: %d %s", toolCall.ID[:8], toolCall.StatusCode,
			func() string {
				if toolCall.Success {
					return "OK"
				}
				return "FAILED"
			}())

		// Process the complete tool call
		a.processToolCall(toolCall)
	}
}

func (a *Agent) processToolCall(tool *parser.ToolCall) {
	if tool.AgentID == "" {
		log.Printf("Warning: tool call without agent ID, skipping")
		return
	}

	// Add to classifier history
	a.classifier.AddToHistory(tool)

	// Classify the tool call
	classification, err := a.classifier.Classify(tool)
	if err != nil {
		log.Printf("Warning: classification failed: %v", err)
	}

	// Log transaction to AMP backend
	txID, err := a.ampClient.LogTransaction(tool.AgentID, tool)
	if err != nil {
		log.Printf("Warning: failed to log transaction: %v", err)
	} else {
		log.Printf("[%s] Logged transaction: %s", tool.ID[:8], txID)
	}

	// If this is a new tool type, register it with AMP
	if classification != nil {
		if err := a.ampClient.RegisterTool(tool.AgentID, classification, tool); err != nil {
			if *logLevel == "debug" {
				log.Printf("Warning: failed to register tool: %v", err)
			}
		}

		// If compensation was discovered, suggest it
		if classification.NeedsCompensation && classification.SuggestedCompensator != nil {
			if err := a.ampClient.RegisterDiscoveredCompensation(
				tool.AgentID,
				classification.ToolName,
				classification.SuggestedCompensator,
			); err != nil {
				if *logLevel == "debug" {
					log.Printf("Warning: failed to register compensation: %v", err)
				}
			} else {
				log.Printf("[%s] Discovered compensation: %s -> %s %s",
					tool.ID[:8],
					classification.ToolName,
					classification.SuggestedCompensator.Method,
					classification.SuggestedCompensator.URLPattern)
			}
		}
	}
}

func generateConnKey(tx *parser.HTTPTransaction) string {
	if tx.IsRequest {
		return tx.SrcIP + ":" + strconv.Itoa(int(tx.SrcPort)) + ":" + tx.DstIP + ":" + strconv.Itoa(int(tx.DstPort))
	}
	// For responses, reverse the direction
	return tx.DstIP + ":" + strconv.Itoa(int(tx.DstPort)) + ":" + tx.SrcIP + ":" + strconv.Itoa(int(tx.SrcPort))
}

func findDockerInterface() string {
	// Try common Docker bridge interfaces
	interfaces := []string{"docker0", "br-amp-network"}

	for _, iface := range interfaces {
		if _, err := os.Stat("/sys/class/net/" + iface); err == nil {
			return iface
		}
	}

	// Look for any br-* interface (Docker networks)
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		name := entry.Name()
		if len(name) > 3 && name[:3] == "br-" {
			return name
		}
	}

	return ""
}
