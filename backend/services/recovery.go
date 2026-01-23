package services

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/poc-amp/backend/models"
	"github.com/poc-amp/backend/store"
)

type RecoveryService struct {
	store *store.Store
}

func NewRecoveryService(store *store.Store) *RecoveryService {
	return &RecoveryService{store: store}
}

// LogToolExecution records a tool execution in the transaction log
func (r *RecoveryService) LogToolExecution(agentID, sessionID, toolName string, input, output json.RawMessage) (*models.TransactionLog, error) {
	log := &models.TransactionLog{
		ID:          uuid.New().String(),
		AgentID:     agentID,
		SessionID:   sessionID,
		ToolName:    toolName,
		InputParams: input,
		OutputResult: output,
		Status:      "executed",
		ExecutedAt:  time.Now(),
		CreatedAt:   time.Now(),
	}

	if err := r.store.CreateTransactionLog(log); err != nil {
		return nil, fmt.Errorf("failed to log execution: %w", err)
	}

	return log, nil
}

// GetSessionTransactions returns all transactions for a session
func (r *RecoveryService) GetSessionTransactions(agentID, sessionID string) ([]*models.TransactionLog, error) {
	return r.store.GetSessionTransactions(agentID, sessionID)
}

// GenerateRollbackPlan creates a plan for rolling back a session
func (r *RecoveryService) GenerateRollbackPlan(agentID, sessionID string) ([]RollbackStep, error) {
	transactions, err := r.store.GetSessionTransactions(agentID, sessionID)
	if err != nil {
		return nil, err
	}

	mappings, err := r.store.GetApprovedMappings(agentID)
	if err != nil {
		return nil, err
	}

	// Build mapping lookup
	mappingLookup := make(map[string]*models.CompensationMapping)
	for _, m := range mappings {
		mappingLookup[m.ToolName] = m
	}

	var steps []RollbackStep
	// Process transactions in reverse order (LIFO)
	for i := len(transactions) - 1; i >= 0; i-- {
		tx := transactions[i]

		if tx.Status != "executed" {
			continue
		}

		mapping, exists := mappingLookup[tx.ToolName]
		step := RollbackStep{
			TransactionID: tx.ID,
			ToolName:      tx.ToolName,
			OriginalInput: tx.InputParams,
			OriginalOutput: tx.OutputResult,
		}

		if !exists || mapping.Status != models.MappingStatusApproved {
			step.Action = "skip"
			step.Reason = "no approved compensation mapping"
		} else {
			step.Action = "compensate"
			step.CompensatorName = mapping.CompensatorName

			// Build compensation parameters from mapping
			compensationParams, err := r.buildCompensationParams(mapping.ParameterMapping, tx.InputParams, tx.OutputResult)
			if err != nil {
				step.Action = "skip"
				step.Reason = fmt.Sprintf("failed to build params: %v", err)
			} else {
				step.CompensationParams = compensationParams
			}
		}

		steps = append(steps, step)
	}

	return steps, nil
}

type RollbackStep struct {
	TransactionID      string          `json:"transaction_id"`
	ToolName           string          `json:"tool_name"`
	OriginalInput      json.RawMessage `json:"original_input"`
	OriginalOutput     json.RawMessage `json:"original_output"`
	Action             string          `json:"action"` // compensate, skip
	CompensatorName    string          `json:"compensator_name,omitempty"`
	CompensationParams json.RawMessage `json:"compensation_params,omitempty"`
	Reason             string          `json:"reason,omitempty"`
}

// buildCompensationParams constructs the parameters for the compensation call
func (r *RecoveryService) buildCompensationParams(mappingJSON, inputJSON, outputJSON json.RawMessage) (json.RawMessage, error) {
	if mappingJSON == nil || len(mappingJSON) == 0 {
		return nil, fmt.Errorf("no parameter mapping defined")
	}

	var mapping map[string]string
	if err := json.Unmarshal(mappingJSON, &mapping); err != nil {
		return nil, fmt.Errorf("invalid mapping format: %w", err)
	}

	var input map[string]interface{}
	var output map[string]interface{}

	if inputJSON != nil {
		json.Unmarshal(inputJSON, &input)
	}
	if outputJSON != nil {
		json.Unmarshal(outputJSON, &output)
	}

	params := make(map[string]interface{})

	for paramName, source := range mapping {
		value, err := r.extractValue(source, input, output)
		if err != nil {
			return nil, fmt.Errorf("failed to extract %s: %w", paramName, err)
		}
		params[paramName] = value
	}

	return json.Marshal(params)
}

// extractValue extracts a value from input or output based on source path
func (r *RecoveryService) extractValue(source string, input, output map[string]interface{}) (interface{}, error) {
	parts := splitPath(source)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid source path: %s", source)
	}

	var data map[string]interface{}
	switch parts[0] {
	case "input":
		data = input
	case "result", "output":
		data = output
	default:
		return nil, fmt.Errorf("unknown source prefix: %s", parts[0])
	}

	if data == nil {
		return nil, fmt.Errorf("source data is nil")
	}

	// Navigate the path
	current := interface{}(data)
	for _, key := range parts[1:] {
		if m, ok := current.(map[string]interface{}); ok {
			current = m[key]
		} else {
			return nil, fmt.Errorf("cannot navigate path at %s", key)
		}
	}

	return current, nil
}

func splitPath(path string) []string {
	var parts []string
	current := ""
	for _, c := range path {
		if c == '.' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

// MarkCompensated marks a transaction as compensated
func (r *RecoveryService) MarkCompensated(transactionID string, result json.RawMessage) error {
	return r.store.MarkTransactionCompensated(transactionID, result)
}
