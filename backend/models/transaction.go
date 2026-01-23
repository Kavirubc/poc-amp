package models

import (
	"encoding/json"
	"time"
)

// Transaction represents a tool execution recorded by eBPF
type Transaction struct {
	ID          string          `json:"id"`
	AgentID     string          `json:"agent_id"`
	SessionID   string          `json:"session_id"`
	ToolName    string          `json:"tool_name"`
	Input       json.RawMessage `json:"input"`
	Output      json.RawMessage `json:"output"`
	Status      string          `json:"status"` // success, failed
	StartedAt   time.Time       `json:"started_at"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}

// Tool represents a discovered tool schema
type Tool struct {
	ID          string          `json:"id"`
	AgentID     string          `json:"agent_id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Endpoint    json.RawMessage `json:"endpoint,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// LogTransactionRequest for eBPF agent
type LogTransactionRequest struct {
	SessionID string          `json:"session_id"`
	ToolName  string          `json:"tool_name"`
	Input     json.RawMessage `json:"input"`
	Output    json.RawMessage `json:"output"`
}

type LogTransactionResponse struct {
	TransactionID string `json:"transaction_id"`
	Message       string `json:"message,omitempty"`
	Error         string `json:"error,omitempty"`
}

// DiscoverCompensationRequest from eBPF agent
type DiscoverCompensationRequest struct {
	ToolName            string          `json:"tool_name"`
	CompensatorName     string          `json:"compensator_name"`
	ParameterMapping    json.RawMessage `json:"parameter_mapping"`
	SuggestedBy         string          `json:"suggested_by"`
	CompensatorEndpoint json.RawMessage `json:"compensator_endpoint,omitempty"`
}

type TransactionsListResponse struct {
	Transactions []*Transaction `json:"transactions"`
	Total        int            `json:"total"`
}

// CompensationMappingsResponse for eBPF agent (approved mappings)
type EBPFCompensationMappingsResponse struct {
	Registry map[string]EBPFCompensationRegistryEntry `json:"registry"`
}

type EBPFCompensationRegistryEntry struct {
	Compensator      string          `json:"compensator"`
	ParameterMapping json.RawMessage `json:"parameter_mapping"`
	Endpoint         json.RawMessage `json:"endpoint,omitempty"`
}
