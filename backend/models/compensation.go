package models

import (
	"encoding/json"
	"time"
)

type MappingStatus string

const (
	MappingStatusPending        MappingStatus = "pending"
	MappingStatusApproved       MappingStatus = "approved"
	MappingStatusRejected       MappingStatus = "rejected"
	MappingStatusNoCompensation MappingStatus = "no_compensation"
)

type SuggestionSource string

const (
	SuggestionHeuristic SuggestionSource = "heuristic"
	SuggestionLLM       SuggestionSource = "llm"
	SuggestionManual    SuggestionSource = "manual"
)

type CompensationMapping struct {
	ID               string           `json:"id"`
	AgentID          string           `json:"agent_id"`
	ToolName         string           `json:"tool_name"`
	ToolSchema       json.RawMessage  `json:"tool_schema,omitempty"`
	ToolDescription  string           `json:"tool_description,omitempty"`
	CompensatorName  string           `json:"compensator_name,omitempty"`
	ParameterMapping json.RawMessage  `json:"parameter_mapping,omitempty"`
	Status           MappingStatus    `json:"status"`
	SuggestedBy      SuggestionSource `json:"suggested_by"`
	Confidence       float64          `json:"confidence"`
	Reasoning        string           `json:"reasoning,omitempty"`
	ReviewedBy       string           `json:"reviewed_by,omitempty"`
	ReviewedAt       *time.Time       `json:"reviewed_at,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
}

type TransactionLog struct {
	ID                 string          `json:"id"`
	AgentID            string          `json:"agent_id"`
	SessionID          string          `json:"session_id"`
	ToolName           string          `json:"tool_name"`
	InputParams        json.RawMessage `json:"input_params"`
	OutputResult       json.RawMessage `json:"output_result"`
	Status             string          `json:"status"` // executed, compensated, failed
	ExecutedAt         time.Time       `json:"executed_at"`
	CompensatedAt      *time.Time      `json:"compensated_at,omitempty"`
	CompensationID     string          `json:"compensation_id,omitempty"`
	CompensationResult json.RawMessage `json:"compensation_result,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
}

// Tool schema for LLM analysis
type ToolSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
}

// LLM Suggestion Request/Response
type SuggestionRequest struct {
	AgentID        string       `json:"agent_id"`
	Tool           ToolSchema   `json:"tool"`
	AvailableTools []ToolSchema `json:"available_tools"`
}

type SuggestionResponse struct {
	HasSideEffects      bool            `json:"has_side_effects"`
	NeedsCompensation   bool            `json:"needs_compensation"`
	SuggestedCompensator string         `json:"suggested_compensator"`
	Confidence          float64         `json:"confidence"`
	ParameterMapping    json.RawMessage `json:"parameter_mapping"`
	Reasoning           string          `json:"reasoning"`
}

// API Request/Response types
type RegisterToolsRequest struct {
	AgentID string       `json:"agent_id"`
	Tools   []ToolSchema `json:"tools"`
}

type ApproveCompensationRequest struct {
	CompensatorName  string          `json:"compensator_name,omitempty"`
	ParameterMapping json.RawMessage `json:"parameter_mapping,omitempty"`
}

type CompensationMappingResponse struct {
	Mapping *CompensationMapping `json:"mapping,omitempty"`
	Message string               `json:"message,omitempty"`
	Error   string               `json:"error,omitempty"`
}

type CompensationMappingsListResponse struct {
	Mappings []*CompensationMapping `json:"mappings"`
	Total    int                    `json:"total"`
}

// Execution tracking
type ExecuteToolRequest struct {
	AgentID   string          `json:"agent_id"`
	SessionID string          `json:"session_id"`
	ToolName  string          `json:"tool_name"`
	Input     json.RawMessage `json:"input"`
}

type ToolExecutionResult struct {
	TransactionID string          `json:"transaction_id"`
	Output        json.RawMessage `json:"output"`
	Error         string          `json:"error,omitempty"`
}

type RollbackRequest struct {
	AgentID   string `json:"agent_id"`
	SessionID string `json:"session_id"`
}

type RollbackResult struct {
	TotalTransactions int      `json:"total_transactions"`
	Compensated       int      `json:"compensated"`
	Failed            int      `json:"failed"`
	Skipped           int      `json:"skipped"`
	Errors            []string `json:"errors,omitempty"`
}
