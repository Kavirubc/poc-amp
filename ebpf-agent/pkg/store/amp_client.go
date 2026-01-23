package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/poc-amp/ebpf-agent/pkg/classifier"
	"github.com/poc-amp/ebpf-agent/pkg/parser"
)

// AMPClient communicates with the AMP backend
type AMPClient struct {
	baseURL string
	client  *http.Client
}

// NewAMPClient creates a new AMP backend client
func NewAMPClient(baseURL string) *AMPClient {
	return &AMPClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

// LogTransaction logs a tool execution to the AMP backend
func (c *AMPClient) LogTransaction(agentID string, tool *parser.ToolCall) (string, error) {
	payload := map[string]interface{}{
		"session_id": fmt.Sprintf("ebpf-%d", time.Now().Unix()),
		"tool_name":  tool.Name,
		"input":      tool.Input,
		"output":     tool.Output,
	}

	// Add endpoint info as metadata
	if tool.Input == nil {
		payload["input"] = map[string]interface{}{
			"_endpoint": map[string]interface{}{
				"method": tool.Endpoint.Method,
				"url":    tool.Endpoint.URL,
				"path":   tool.Endpoint.Path,
			},
			"_raw": tool.InputRaw,
		}
	}

	if tool.Output == nil && tool.OutputRaw != "" {
		payload["output"] = map[string]interface{}{
			"_raw":         tool.OutputRaw,
			"_status_code": tool.StatusCode,
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshaling payload: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/agents/%s/transactions", c.baseURL, agentID)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		TransactionID string `json:"transaction_id"`
	}
	json.Unmarshal(respBody, &result)

	return result.TransactionID, nil
}

// RegisterTool registers a discovered tool with the AMP backend
func (c *AMPClient) RegisterTool(agentID string, classification *classifier.ToolClassification, tool *parser.ToolCall) error {
	toolSchema := map[string]interface{}{
		"name":        classification.ToolName,
		"description": classification.Description,
		"inputSchema": classification.InputSchema,
		"endpoint": map[string]interface{}{
			"method": tool.Endpoint.Method,
			"url":    tool.Endpoint.URL,
			"path":   tool.Endpoint.Path,
			"host":   tool.Endpoint.Host,
		},
	}

	payload := map[string]interface{}{
		"tools": []interface{}{toolSchema},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling payload: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/agents/%s/tools", c.baseURL, agentID)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// RegisterDiscoveredCompensation registers a compensation mapping discovered by eBPF
func (c *AMPClient) RegisterDiscoveredCompensation(agentID string, toolName string, comp *classifier.CompensatorInfo) error {
	payload := map[string]interface{}{
		"tool_name":         toolName,
		"compensator_name":  fmt.Sprintf("%s %s", comp.Method, comp.URLPattern),
		"parameter_mapping": comp.ParameterMapping,
		"suggested_by":      "ebpf_traffic_analysis",
		"compensator_endpoint": map[string]interface{}{
			"method":      comp.Method,
			"url_pattern": comp.URLPattern,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling payload: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/agents/%s/compensation-mappings/discover", c.baseURL, agentID)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// GetApprovedMappings retrieves approved compensation mappings for an agent
func (c *AMPClient) GetApprovedMappings(agentID string) (map[string]*CompensationMapping, error) {
	url := fmt.Sprintf("%s/api/v1/agents/%s/compensation-mappings/approved", c.baseURL, agentID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Registry map[string]struct {
			Compensator      string            `json:"compensator"`
			ParameterMapping map[string]string `json:"parameter_mapping"`
			Endpoint         *struct {
				Method     string `json:"method"`
				URLPattern string `json:"url_pattern"`
			} `json:"endpoint,omitempty"`
		} `json:"registry"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	mappings := make(map[string]*CompensationMapping)
	for toolName, data := range result.Registry {
		mapping := &CompensationMapping{
			ToolName:         toolName,
			CompensatorName:  data.Compensator,
			ParameterMapping: data.ParameterMapping,
		}
		if data.Endpoint != nil {
			mapping.CompensatorEndpoint = &EndpointInfo{
				Method:     data.Endpoint.Method,
				URLPattern: data.Endpoint.URLPattern,
			}
		}
		mappings[toolName] = mapping
	}

	return mappings, nil
}

// ExecuteCompensation executes a compensation by making an HTTP call
func (c *AMPClient) ExecuteCompensation(mapping *CompensationMapping, originalInput, originalOutput map[string]interface{}) error {
	if mapping.CompensatorEndpoint == nil {
		return fmt.Errorf("no endpoint info for compensator")
	}

	// Build URL from pattern
	url := mapping.CompensatorEndpoint.URLPattern
	for param, source := range mapping.ParameterMapping {
		value := extractValue(source, originalInput, originalOutput)
		if value != "" {
			url = replaceParam(url, param, value)
		}
	}

	// Create request
	req, err := http.NewRequest(mapping.CompensatorEndpoint.Method, url, nil)
	if err != nil {
		return fmt.Errorf("creating compensation request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("executing compensation: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("compensation failed %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// CompensationMapping represents a compensation mapping
type CompensationMapping struct {
	ToolName            string            `json:"tool_name"`
	CompensatorName     string            `json:"compensator_name"`
	ParameterMapping    map[string]string `json:"parameter_mapping"`
	CompensatorEndpoint *EndpointInfo     `json:"compensator_endpoint,omitempty"`
}

// EndpointInfo describes an HTTP endpoint
type EndpointInfo struct {
	Method     string `json:"method"`
	URLPattern string `json:"url_pattern"`
}

// extractValue extracts a value from input or output based on source path
func extractValue(source string, input, output map[string]interface{}) string {
	parts := splitPath(source)
	if len(parts) < 2 {
		return ""
	}

	var data map[string]interface{}
	switch parts[0] {
	case "input":
		data = input
	case "result", "output":
		data = output
	default:
		return ""
	}

	// Navigate path
	current := interface{}(data)
	for _, key := range parts[1:] {
		if m, ok := current.(map[string]interface{}); ok {
			current = m[key]
		} else {
			return ""
		}
	}

	if s, ok := current.(string); ok {
		return s
	}
	if f, ok := current.(float64); ok {
		return fmt.Sprintf("%v", f)
	}

	return ""
}

func splitPath(path string) []string {
	return splitString(path, ".")
}

func splitString(s, sep string) []string {
	var result []string
	for _, part := range splitByChar(s, '.') {
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func splitByChar(s string, sep rune) []string {
	var result []string
	current := ""
	for _, c := range s {
		if c == sep {
			result = append(result, current)
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

func replaceParam(url, param, value string) string {
	// Replace {param} with value
	patterns := []string{
		fmt.Sprintf("{%s}", param),
		fmt.Sprintf(":%s", param),
	}

	for _, pattern := range patterns {
		if idx := findSubstring(url, pattern); idx >= 0 {
			return url[:idx] + value + url[idx+len(pattern):]
		}
	}

	return url
}

func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
