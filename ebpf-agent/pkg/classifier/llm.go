package classifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/poc-amp/ebpf-agent/pkg/parser"
)

// ToolClassification represents the LLM's analysis of a tool
type ToolClassification struct {
	ToolName            string            `json:"tool_name"`
	Description         string            `json:"description"`
	HasSideEffects      bool              `json:"has_side_effects"`
	NeedsCompensation   bool              `json:"needs_compensation"`
	SuggestedCompensator *CompensatorInfo `json:"suggested_compensator,omitempty"`
	InputSchema         map[string]interface{} `json:"input_schema"`
	OutputSchema        map[string]interface{} `json:"output_schema"`
	Confidence          float64           `json:"confidence"`
	Reasoning           string            `json:"reasoning"`
}

// CompensatorInfo describes how to compensate a tool call
type CompensatorInfo struct {
	Method           string            `json:"method"`
	URLPattern       string            `json:"url_pattern"`
	ParameterMapping map[string]string `json:"parameter_mapping"`
	BodyTemplate     map[string]interface{} `json:"body_template,omitempty"`
}

// Classifier uses LLM to classify and analyze tool calls
type Classifier struct {
	apiKey   string
	apiURL   string
	client   *http.Client
	cache    map[string]*ToolClassification // endpoint -> classification
	history  []*parser.ToolCall             // recent tool calls for pattern analysis
}

// NewClassifier creates a new LLM-based classifier
func NewClassifier() *Classifier {
	apiKey := os.Getenv("GEMINI_API_KEY")

	return &Classifier{
		apiKey:  apiKey,
		apiURL:  "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent",
		client:  &http.Client{Timeout: 30 * time.Second},
		cache:   make(map[string]*ToolClassification),
		history: make([]*parser.ToolCall, 0),
	}
}

// AddToHistory adds a tool call to the history for pattern analysis
func (c *Classifier) AddToHistory(tool *parser.ToolCall) {
	c.history = append(c.history, tool)

	// Keep only last 100 calls
	if len(c.history) > 100 {
		c.history = c.history[len(c.history)-100:]
	}
}

// Classify analyzes a tool call and returns classification
func (c *Classifier) Classify(tool *parser.ToolCall) (*ToolClassification, error) {
	// Check cache first
	cacheKey := fmt.Sprintf("%s:%s", tool.Endpoint.Method, tool.Endpoint.Path)
	if cached, ok := c.cache[cacheKey]; ok {
		return cached, nil
	}

	// Try heuristic classification first
	if classification := c.classifyHeuristic(tool); classification != nil {
		c.cache[cacheKey] = classification
		return classification, nil
	}

	// Fall back to LLM
	if c.apiKey == "" {
		return c.defaultClassification(tool), nil
	}

	classification, err := c.classifyWithLLM(tool)
	if err != nil {
		return c.defaultClassification(tool), err
	}

	c.cache[cacheKey] = classification
	return classification, nil
}

// classifyHeuristic tries to classify using patterns
func (c *Classifier) classifyHeuristic(tool *parser.ToolCall) *ToolClassification {
	path := strings.ToLower(tool.Endpoint.Path)
	method := strings.ToUpper(tool.Endpoint.Method)

	classification := &ToolClassification{
		ToolName:       tool.Name,
		HasSideEffects: method != "GET" && method != "HEAD" && method != "OPTIONS",
	}

	// Infer schemas from actual data
	if tool.Input != nil {
		classification.InputSchema = parser.InferSchema(tool.Input)
	}
	if tool.Output != nil {
		classification.OutputSchema = parser.InferSchema(tool.Output)
	}

	// Look for compensation patterns in history
	compensator := c.findCompensatorInHistory(tool)
	if compensator != nil {
		classification.NeedsCompensation = true
		classification.SuggestedCompensator = compensator
		classification.Confidence = 0.85
		classification.Reasoning = "Compensation pattern detected from traffic history"
		return classification
	}

	// Check for common patterns
	patterns := map[string]struct {
		compensatorMethod string
		urlTransform      func(string) string
		paramMapping      map[string]string
	}{
		"/book":        {"DELETE", func(p string) string { return strings.Replace(p, "/book", "/cancel", 1) }, map[string]string{"booking_id": "result.booking_id"}},
		"/reserve":     {"DELETE", func(p string) string { return strings.Replace(p, "/reserve", "/cancel", 1) }, map[string]string{"reservation_id": "result.reservation_id"}},
		"/create":      {"DELETE", func(p string) string { return strings.Replace(p, "/create", "/delete", 1) }, map[string]string{"id": "result.id"}},
		"/order":       {"DELETE", func(p string) string { return p + "/cancel" }, map[string]string{"order_id": "result.order_id"}},
		"/subscribe":   {"DELETE", func(p string) string { return strings.Replace(p, "/subscribe", "/unsubscribe", 1) }, map[string]string{"subscription_id": "result.subscription_id"}},
		"/publish":     {"DELETE", func(p string) string { return strings.Replace(p, "/publish", "/unpublish", 1) }, map[string]string{"id": "result.id"}},
		"/allocate":    {"DELETE", func(p string) string { return strings.Replace(p, "/allocate", "/deallocate", 1) }, map[string]string{"allocation_id": "result.allocation_id"}},
		"/enable":      {"POST", func(p string) string { return strings.Replace(p, "/enable", "/disable", 1) }, map[string]string{"id": "input.id"}},
		"/activate":    {"POST", func(p string) string { return strings.Replace(p, "/activate", "/deactivate", 1) }, map[string]string{"id": "input.id"}},
		"/start":       {"POST", func(p string) string { return strings.Replace(p, "/start", "/stop", 1) }, map[string]string{"id": "input.id"}},
	}

	for pattern, info := range patterns {
		if strings.Contains(path, pattern) && (method == "POST" || method == "PUT") {
			classification.NeedsCompensation = true
			classification.SuggestedCompensator = &CompensatorInfo{
				Method:           info.compensatorMethod,
				URLPattern:       info.urlTransform(tool.Endpoint.Path),
				ParameterMapping: info.paramMapping,
			}
			classification.Confidence = 0.75
			classification.Reasoning = fmt.Sprintf("Matched heuristic pattern: %s", pattern)
			return classification
		}
	}

	// POST/PUT without clear pattern - may need compensation
	if method == "POST" || method == "PUT" {
		classification.NeedsCompensation = true
		classification.Confidence = 0.5
		classification.Reasoning = "Mutating HTTP method detected, compensation may be needed"
	}

	return nil
}

// findCompensatorInHistory looks for compensation patterns in traffic history
func (c *Classifier) findCompensatorInHistory(tool *parser.ToolCall) *CompensatorInfo {
	// Look for DELETE/POST calls to similar endpoints after this type of call
	for i := len(c.history) - 1; i >= 0; i-- {
		hist := c.history[i]

		// Skip same type of calls
		if hist.Endpoint.Method == tool.Endpoint.Method &&
			hist.Endpoint.Path == tool.Endpoint.Path {
			continue
		}

		// Check if this could be a compensation
		if hist.Endpoint.Method == "DELETE" || hist.Endpoint.Method == "POST" {
			// Check if the URL contains an ID from a previous response
			if tool.Output != nil {
				for key, value := range tool.Output {
					if strings.Contains(key, "id") || strings.Contains(key, "Id") {
						if valStr, ok := value.(string); ok {
							if strings.Contains(hist.Endpoint.Path, valStr) {
								return &CompensatorInfo{
									Method:     hist.Endpoint.Method,
									URLPattern: extractURLPattern(hist.Endpoint.Path, valStr),
									ParameterMapping: map[string]string{
										key: fmt.Sprintf("result.%s", key),
									},
								}
							}
						}
					}
				}
			}
		}
	}

	return nil
}

// extractURLPattern creates a URL pattern with placeholders
func extractURLPattern(path, id string) string {
	return strings.Replace(path, id, "{id}", 1)
}

// classifyWithLLM uses Gemini to classify the tool
func (c *Classifier) classifyWithLLM(tool *parser.ToolCall) (*ToolClassification, error) {
	toolJSON, _ := json.MarshalIndent(tool, "", "  ")
	historyJSON, _ := json.MarshalIndent(c.history[max(0, len(c.history)-10):], "", "  ")

	prompt := fmt.Sprintf(`Analyze this HTTP API call captured from an AI agent and determine if it needs compensation (rollback capability).

API CALL:
%s

RECENT TRAFFIC HISTORY (for pattern detection):
%s

Analyze and respond with ONLY valid JSON (no markdown):
{
  "tool_name": "descriptive_name_for_this_api_call",
  "description": "what this API call does",
  "has_side_effects": true/false,
  "needs_compensation": true/false,
  "suggested_compensator": {
    "method": "DELETE or POST or PUT",
    "url_pattern": "URL with {param} placeholders for compensation",
    "parameter_mapping": {
      "param_name": "source (result.field_name or input.field_name)"
    }
  } or null if no compensation needed,
  "input_schema": { JSON schema inferred from input },
  "output_schema": { JSON schema inferred from output },
  "confidence": 0.0 to 1.0,
  "reasoning": "brief explanation"
}

Consider:
- POST/PUT to create/book/reserve endpoints usually need DELETE/cancel compensation
- Email/notification sends typically cannot be compensated
- GET requests don't need compensation
- Look for patterns in the traffic history`, string(toolJSON), string(historyJSON))

	reqBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]string{
					{"text": prompt},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature":     0.1,
			"maxOutputTokens": 2048,
		},
	}

	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	url := fmt.Sprintf("%s?key=%s", c.apiURL, c.apiKey)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	// Parse Gemini response
	var geminiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("empty response from API")
	}

	responseText := geminiResp.Candidates[0].Content.Parts[0].Text
	responseText = strings.TrimPrefix(responseText, "```json")
	responseText = strings.TrimPrefix(responseText, "```")
	responseText = strings.TrimSuffix(responseText, "```")
	responseText = strings.TrimSpace(responseText)

	var classification ToolClassification
	if err := json.Unmarshal([]byte(responseText), &classification); err != nil {
		return nil, fmt.Errorf("parsing classification: %w (response: %s)", err, responseText)
	}

	return &classification, nil
}

// defaultClassification returns a basic classification without LLM
func (c *Classifier) defaultClassification(tool *parser.ToolCall) *ToolClassification {
	method := strings.ToUpper(tool.Endpoint.Method)

	classification := &ToolClassification{
		ToolName:       tool.Name,
		HasSideEffects: method != "GET" && method != "HEAD" && method != "OPTIONS",
		Confidence:     0.3,
		Reasoning:      "Default classification without LLM analysis",
	}

	if tool.Input != nil {
		classification.InputSchema = parser.InferSchema(tool.Input)
	}
	if tool.Output != nil {
		classification.OutputSchema = parser.InferSchema(tool.Output)
	}

	if method == "POST" || method == "PUT" {
		classification.NeedsCompensation = true
		classification.Reasoning = "Mutating method may need compensation (LLM unavailable for detailed analysis)"
	}

	return classification
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
