package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/poc-amp/backend/models"
	"github.com/poc-amp/backend/store"
)

type SuggestionService struct {
	store     *store.Store
	apiKey    string
	apiURL    string
}

func NewSuggestionService(store *store.Store) *SuggestionService {
	apiKey := os.Getenv("GEMINI_API_KEY")
	return &SuggestionService{
		store:  store,
		apiKey: apiKey,
		apiURL: "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent",
	}
}

// Heuristic patterns for compensation
var undoPrefixes = map[string]string{
	"book_":      "cancel_",
	"create_":    "cancel_", // For reservations
	"make_":      "cancel_",
	"add_":       "remove_",
	"insert_":    "delete_",
	"reserve_":   "cancel_",
	"allocate_":  "deallocate_",
	"assign_":    "unassign_",
	"start_":     "stop_",
	"enable_":    "disable_",
	"open_":      "close_",
	"send_":      "recall_",
	"publish_":   "unpublish_",
	"activate_":  "deactivate_",
	"lock_":      "unlock_",
	"subscribe_": "unsubscribe_",
}

// SuggestCompensatorHeuristic tries pattern matching before calling LLM
func (s *SuggestionService) SuggestCompensatorHeuristic(toolName string, availableTools []string) (string, bool) {
	toolsSet := make(map[string]bool)
	for _, t := range availableTools {
		toolsSet[t] = true
	}

	for prefix, undoPrefix := range undoPrefixes {
		if strings.HasPrefix(toolName, prefix) {
			candidate := strings.Replace(toolName, prefix, undoPrefix, 1)
			if toolsSet[candidate] {
				return candidate, true
			}
		}
	}

	return "", false
}

// SuggestCompensatorLLM uses Gemini to analyze and suggest compensation
func (s *SuggestionService) SuggestCompensatorLLM(tool models.ToolSchema, availableTools []models.ToolSchema) (*models.SuggestionResponse, error) {
	if s.apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY not configured")
	}

	toolJSON, _ := json.MarshalIndent(tool, "", "  ")
	availableJSON, _ := json.MarshalIndent(availableTools, "", "  ")

	prompt := fmt.Sprintf(`You are analyzing tools for an AI agent compensation system.

Given this tool, identify if there's a compensating action among the available tools.

TOOL TO ANALYZE:
%s

AVAILABLE TOOLS:
%s

Respond ONLY with valid JSON (no markdown, no explanation outside JSON):
{
  "has_side_effects": true or false,
  "needs_compensation": true or false,
  "suggested_compensator": "tool_name" or null,
  "confidence": 0.0 to 1.0,
  "parameter_mapping": {
    "compensation_param": "source (e.g., 'result.booking_id' or 'input.flight_id')"
  },
  "reasoning": "brief explanation"
}`, string(toolJSON), string(availableJSON))

	// Prepare Gemini API request
	reqBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]string{
					{"text": prompt},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature": 0.1,
			"maxOutputTokens": 1024,
		},
	}

	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s?key=%s", s.apiURL, s.apiKey)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
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
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("empty response from API")
	}

	responseText := geminiResp.Candidates[0].Content.Parts[0].Text

	// Clean up the response (remove markdown code blocks if present)
	responseText = strings.TrimPrefix(responseText, "```json")
	responseText = strings.TrimPrefix(responseText, "```")
	responseText = strings.TrimSuffix(responseText, "```")
	responseText = strings.TrimSpace(responseText)

	var suggestion models.SuggestionResponse
	if err := json.Unmarshal([]byte(responseText), &suggestion); err != nil {
		return nil, fmt.Errorf("failed to parse suggestion: %w (response: %s)", err, responseText)
	}

	return &suggestion, nil
}

// RegisterTools registers tools and generates compensation suggestions
func (s *SuggestionService) RegisterTools(agentID string, tools []models.ToolSchema) ([]*models.CompensationMapping, error) {
	var mappings []*models.CompensationMapping

	// Build list of available tool names
	toolNames := make([]string, len(tools))
	for i, t := range tools {
		toolNames[i] = t.Name
	}

	for _, tool := range tools {
		// Check if mapping already exists
		existing, _ := s.store.GetCompensationMappingByTool(agentID, tool.Name)
		if existing != nil {
			mappings = append(mappings, existing)
			continue
		}

		// Create new mapping with proper defaults
		mapping := &models.CompensationMapping{
			ID:               uuid.New().String(),
			AgentID:          agentID,
			ToolName:         tool.Name,
			ToolDescription:  tool.Description,
			ToolSchema:       json.RawMessage("{}"),
			ParameterMapping: json.RawMessage("{}"),
			Status:           models.MappingStatusPending,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		}

		if tool.InputSchema != nil && len(tool.InputSchema) > 0 {
			mapping.ToolSchema = tool.InputSchema
		}

		// Try heuristic first
		if compensator, found := s.SuggestCompensatorHeuristic(tool.Name, toolNames); found {
			mapping.CompensatorName = compensator
			mapping.SuggestedBy = models.SuggestionHeuristic
			mapping.Confidence = 0.85
			mapping.Reasoning = fmt.Sprintf("Heuristic match: %s appears to undo %s based on naming pattern", compensator, tool.Name)

			// Generate basic parameter mapping
			paramMapping := map[string]string{}
			paramMapping["id"] = "result.id"
			pmJSON, _ := json.Marshal(paramMapping)
			mapping.ParameterMapping = pmJSON
		} else if s.apiKey != "" {
			// Fall back to LLM
			suggestion, err := s.SuggestCompensatorLLM(tool, tools)
			if err == nil && suggestion.NeedsCompensation && suggestion.SuggestedCompensator != "" {
				mapping.CompensatorName = suggestion.SuggestedCompensator
				mapping.SuggestedBy = models.SuggestionLLM
				mapping.Confidence = suggestion.Confidence
				mapping.Reasoning = suggestion.Reasoning
				if suggestion.ParameterMapping != nil && len(suggestion.ParameterMapping) > 0 {
					mapping.ParameterMapping = suggestion.ParameterMapping
				}
			} else if err == nil && !suggestion.NeedsCompensation {
				mapping.Status = models.MappingStatusNoCompensation
				mapping.SuggestedBy = models.SuggestionLLM
				mapping.Reasoning = suggestion.Reasoning
			}
		}

		if err := s.store.CreateCompensationMapping(mapping); err != nil {
			return nil, fmt.Errorf("failed to create mapping for %s: %w", tool.Name, err)
		}

		mappings = append(mappings, mapping)
	}

	return mappings, nil
}

// ApproveMapping approves a compensation mapping
func (s *SuggestionService) ApproveMapping(id string, compensator string, paramMapping json.RawMessage, reviewer string) error {
	mapping, err := s.store.GetCompensationMapping(id)
	if err != nil {
		return err
	}
	if mapping == nil {
		return fmt.Errorf("mapping not found")
	}

	now := time.Now()
	mapping.Status = models.MappingStatusApproved
	mapping.ReviewedBy = reviewer
	mapping.ReviewedAt = &now

	if compensator != "" {
		mapping.CompensatorName = compensator
	}
	if paramMapping != nil {
		mapping.ParameterMapping = paramMapping
	}

	return s.store.UpdateCompensationMapping(mapping)
}

// RejectMapping rejects a compensation mapping
func (s *SuggestionService) RejectMapping(id string, noCompensation bool, reviewer string) error {
	mapping, err := s.store.GetCompensationMapping(id)
	if err != nil {
		return err
	}
	if mapping == nil {
		return fmt.Errorf("mapping not found")
	}

	now := time.Now()
	if noCompensation {
		mapping.Status = models.MappingStatusNoCompensation
	} else {
		mapping.Status = models.MappingStatusRejected
	}
	mapping.ReviewedBy = reviewer
	mapping.ReviewedAt = &now

	return s.store.UpdateCompensationMapping(mapping)
}
