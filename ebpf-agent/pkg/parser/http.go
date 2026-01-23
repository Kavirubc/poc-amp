package parser

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	ebpfpkg "github.com/poc-amp/ebpf-agent/pkg/ebpf"
)

// HTTPTransaction represents a parsed HTTP request/response
type HTTPTransaction struct {
	ID          string                 `json:"id"`
	AgentID     string                 `json:"agent_id"`
	Method      string                 `json:"method"`
	URL         string                 `json:"url"`
	Host        string                 `json:"host"`
	Path        string                 `json:"path"`
	Headers     map[string]string      `json:"headers"`
	Body        json.RawMessage        `json:"body,omitempty"`
	BodyRaw     string                 `json:"body_raw,omitempty"`
	StatusCode  int                    `json:"status_code,omitempty"`
	Response    json.RawMessage        `json:"response,omitempty"`
	ResponseRaw string                 `json:"response_raw,omitempty"`
	SrcIP       string                 `json:"src_ip"`
	DstIP       string                 `json:"dst_ip"`
	SrcPort     uint16                 `json:"src_port"`
	DstPort     uint16                 `json:"dst_port"`
	PID         uint32                 `json:"pid"`
	Timestamp   time.Time              `json:"timestamp"`
	IsRequest   bool                   `json:"is_request"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// Parser parses HTTP data from eBPF events
type Parser struct {
	// Connection tracking for correlating requests/responses
	pendingRequests map[string]*HTTPTransaction // key: src_ip:src_port:dst_ip:dst_port
}

// NewParser creates a new HTTP parser
func NewParser() *Parser {
	return &Parser{
		pendingRequests: make(map[string]*HTTPTransaction),
	}
}

// Parse parses an eBPF HTTP event into a structured transaction
func (p *Parser) Parse(event *ebpfpkg.HTTPEvent) (*HTTPTransaction, error) {
	if len(event.Data) == 0 {
		return nil, fmt.Errorf("empty event data")
	}

	tx := &HTTPTransaction{
		SrcIP:     event.SrcIP.String(),
		DstIP:     event.DstIP.String(),
		SrcPort:   event.SrcPort,
		DstPort:   event.DstPort,
		PID:       event.PID,
		Timestamp: event.Timestamp,
		AgentID:   event.AgentID,
		Headers:   make(map[string]string),
		Metadata:  make(map[string]interface{}),
	}

	if event.EventType == ebpfpkg.EventHTTPRequest {
		tx.IsRequest = true
		return p.parseRequest(tx, event.Data)
	} else {
		tx.IsRequest = false
		return p.parseResponse(tx, event.Data)
	}
}

func (p *Parser) parseRequest(tx *HTTPTransaction, data []byte) (*HTTPTransaction, error) {
	reader := bufio.NewReader(bytes.NewReader(data))

	// Read request line
	requestLine, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("reading request line: %w", err)
	}

	parts := strings.Fields(strings.TrimSpace(requestLine))
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid request line: %s", requestLine)
	}

	tx.Method = parts[0]
	tx.Path = parts[1]

	// Read headers until empty line
	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			break
		}

		line = strings.TrimSpace(line)
		if line == "" {
			break
		}

		colonIdx := strings.Index(line, ":")
		if colonIdx > 0 {
			key := strings.TrimSpace(line[:colonIdx])
			value := strings.TrimSpace(line[colonIdx+1:])
			tx.Headers[key] = value

			if strings.ToLower(key) == "host" {
				tx.Host = value
			}
		}
	}

	// Build full URL
	if tx.Host != "" {
		scheme := "http"
		if tx.DstPort == 443 {
			scheme = "https"
		}
		tx.URL = fmt.Sprintf("%s://%s%s", scheme, tx.Host, tx.Path)
	} else {
		tx.URL = fmt.Sprintf("http://%s:%d%s", tx.DstIP, tx.DstPort, tx.Path)
	}

	// Read body
	remaining, _ := io.ReadAll(reader)
	if len(remaining) > 0 {
		tx.BodyRaw = string(remaining)

		// Try to parse as JSON
		var jsonBody interface{}
		if err := json.Unmarshal(remaining, &jsonBody); err == nil {
			tx.Body, _ = json.Marshal(jsonBody)
		}
	}

	// Store for response correlation
	connKey := fmt.Sprintf("%s:%d:%s:%d", tx.SrcIP, tx.SrcPort, tx.DstIP, tx.DstPort)
	p.pendingRequests[connKey] = tx

	return tx, nil
}

func (p *Parser) parseResponse(tx *HTTPTransaction, data []byte) (*HTTPTransaction, error) {
	reader := bufio.NewReader(bytes.NewReader(data))

	// Read status line
	statusLine, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("reading status line: %w", err)
	}

	parts := strings.Fields(strings.TrimSpace(statusLine))
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid status line: %s", statusLine)
	}

	fmt.Sscanf(parts[1], "%d", &tx.StatusCode)

	// Read headers
	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			break
		}

		line = strings.TrimSpace(line)
		if line == "" {
			break
		}

		colonIdx := strings.Index(line, ":")
		if colonIdx > 0 {
			key := strings.TrimSpace(line[:colonIdx])
			value := strings.TrimSpace(line[colonIdx+1:])
			tx.Headers[key] = value
		}
	}

	// Read body
	remaining, _ := io.ReadAll(reader)
	if len(remaining) > 0 {
		tx.ResponseRaw = string(remaining)

		// Try to parse as JSON
		var jsonBody interface{}
		if err := json.Unmarshal(remaining, &jsonBody); err == nil {
			tx.Response, _ = json.Marshal(jsonBody)
		}
	}

	// Try to correlate with request
	connKey := fmt.Sprintf("%s:%d:%s:%d", tx.DstIP, tx.DstPort, tx.SrcIP, tx.SrcPort)
	if req, ok := p.pendingRequests[connKey]; ok {
		// Copy request info to response
		tx.Method = req.Method
		tx.URL = req.URL
		tx.Host = req.Host
		tx.Path = req.Path
		tx.Body = req.Body
		tx.BodyRaw = req.BodyRaw
		tx.AgentID = req.AgentID

		delete(p.pendingRequests, connKey)
	}

	return tx, nil
}

// ExtractToolCall attempts to identify a tool call from an HTTP transaction
func (p *Parser) ExtractToolCall(tx *HTTPTransaction) *ToolCall {
	if !tx.IsRequest {
		return nil
	}

	// Skip non-mutating requests (GET, HEAD, OPTIONS)
	if tx.Method == "GET" || tx.Method == "HEAD" || tx.Method == "OPTIONS" {
		return nil
	}

	tool := &ToolCall{
		ID:        tx.ID,
		AgentID:   tx.AgentID,
		Timestamp: tx.Timestamp,
		Endpoint: Endpoint{
			Method: tx.Method,
			URL:    tx.URL,
			Host:   tx.Host,
			Path:   tx.Path,
		},
		Headers: tx.Headers,
	}

	// Parse URL to extract path parameters
	parsedURL, err := url.Parse(tx.URL)
	if err == nil {
		tool.Endpoint.Query = parsedURL.Query()
	}

	// Extract input from body
	if tx.Body != nil {
		var input map[string]interface{}
		if err := json.Unmarshal(tx.Body, &input); err == nil {
			tool.Input = input
		}
	} else if tx.BodyRaw != "" {
		tool.InputRaw = tx.BodyRaw
	}

	// Generate a descriptive tool name from the endpoint
	tool.Name = generateToolName(tx.Method, tx.Path, tx.Host)

	return tool
}

// UpdateToolCallWithResponse updates a tool call with response data
func (p *Parser) UpdateToolCallWithResponse(tool *ToolCall, tx *HTTPTransaction) {
	tool.StatusCode = tx.StatusCode
	tool.Success = tx.StatusCode >= 200 && tx.StatusCode < 300

	if tx.Response != nil {
		var output map[string]interface{}
		if err := json.Unmarshal(tx.Response, &output); err == nil {
			tool.Output = output
		}
	} else if tx.ResponseRaw != "" {
		tool.OutputRaw = tx.ResponseRaw
	}
}

// ToolCall represents a detected tool/API call
type ToolCall struct {
	ID         string                 `json:"id"`
	AgentID    string                 `json:"agent_id"`
	Name       string                 `json:"name"`
	Endpoint   Endpoint               `json:"endpoint"`
	Headers    map[string]string      `json:"headers,omitempty"`
	Input      map[string]interface{} `json:"input,omitempty"`
	InputRaw   string                 `json:"input_raw,omitempty"`
	Output     map[string]interface{} `json:"output,omitempty"`
	OutputRaw  string                 `json:"output_raw,omitempty"`
	StatusCode int                    `json:"status_code"`
	Success    bool                   `json:"success"`
	Timestamp  time.Time              `json:"timestamp"`
}

// Endpoint represents an API endpoint
type Endpoint struct {
	Method string              `json:"method"`
	URL    string              `json:"url"`
	Host   string              `json:"host"`
	Path   string              `json:"path"`
	Query  map[string][]string `json:"query,omitempty"`
}

// generateToolName creates a human-readable tool name from endpoint info
func generateToolName(method, path, host string) string {
	// Clean up path
	path = strings.Trim(path, "/")

	// Remove common prefixes
	path = strings.TrimPrefix(path, "api/")
	path = strings.TrimPrefix(path, "v1/")
	path = strings.TrimPrefix(path, "v2/")

	// Replace path separators with underscores
	path = strings.ReplaceAll(path, "/", "_")

	// Extract host name (remove TLD)
	hostParts := strings.Split(host, ".")
	hostName := ""
	if len(hostParts) > 0 {
		hostName = hostParts[0]
	}

	// Build tool name
	methodPrefix := strings.ToLower(method)
	switch methodPrefix {
	case "post":
		methodPrefix = "create"
	case "put":
		methodPrefix = "update"
	case "delete":
		methodPrefix = "delete"
	case "patch":
		methodPrefix = "patch"
	}

	if hostName != "" && hostName != "api" && hostName != "localhost" {
		return fmt.Sprintf("%s_%s_%s", hostName, methodPrefix, path)
	}

	return fmt.Sprintf("%s_%s", methodPrefix, path)
}

// InferSchema attempts to infer a JSON schema from the input/output
func InferSchema(data map[string]interface{}) map[string]interface{} {
	schema := map[string]interface{}{
		"type":       "object",
		"properties": make(map[string]interface{}),
	}

	properties := schema["properties"].(map[string]interface{})
	required := []string{}

	for key, value := range data {
		propSchema := inferPropertySchema(value)
		properties[key] = propSchema
		required = append(required, key)
	}

	if len(required) > 0 {
		schema["required"] = required
	}

	return schema
}

func inferPropertySchema(value interface{}) map[string]interface{} {
	schema := make(map[string]interface{})

	switch v := value.(type) {
	case string:
		schema["type"] = "string"
	case float64:
		if v == float64(int(v)) {
			schema["type"] = "integer"
		} else {
			schema["type"] = "number"
		}
	case bool:
		schema["type"] = "boolean"
	case []interface{}:
		schema["type"] = "array"
		if len(v) > 0 {
			schema["items"] = inferPropertySchema(v[0])
		}
	case map[string]interface{}:
		schema["type"] = "object"
		properties := make(map[string]interface{})
		for k, val := range v {
			properties[k] = inferPropertySchema(val)
		}
		schema["properties"] = properties
	default:
		schema["type"] = "string"
	}

	return schema
}
