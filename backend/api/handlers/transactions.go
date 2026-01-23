package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/poc-amp/backend/models"
	"github.com/poc-amp/backend/store"
)

type TransactionHandler struct {
	store *store.Store
}

func NewTransactionHandler(s *store.Store) *TransactionHandler {
	return &TransactionHandler{store: s}
}

// LogTransaction records a tool execution from eBPF agent
func (h *TransactionHandler) LogTransaction(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")

	var req models.LogTransactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	tx := &models.Transaction{
		ID:        uuid.New().String(),
		AgentID:   agentID,
		SessionID: req.SessionID,
		ToolName:  req.ToolName,
		Input:     req.Input,
		Output:    req.Output,
		Status:    "success",
		StartedAt: time.Now(),
		CreatedAt: time.Now(),
	}

	// Check for error indicators in output
	if len(req.Output) > 0 {
		var outputMap map[string]interface{}
		if err := json.Unmarshal(req.Output, &outputMap); err == nil {
			if _, hasError := outputMap["error"]; hasError {
				tx.Status = "failed"
			}
			if statusCode, ok := outputMap["_status_code"].(float64); ok && statusCode >= 400 {
				tx.Status = "failed"
			}
		}
	}

	if err := h.store.SaveTransaction(tx); err != nil {
		log.Printf("Error saving transaction: %v", err)
		respondError(w, http.StatusInternalServerError, "Failed to save transaction")
		return
	}

	log.Printf("[eBPF] Logged transaction %s for agent %s: %s", tx.ID, agentID, req.ToolName)

	respondJSON(w, http.StatusCreated, models.LogTransactionResponse{
		TransactionID: tx.ID,
		Message:       "Transaction logged",
	})
}

// ListTransactions returns all transactions for an agent
func (h *TransactionHandler) ListTransactions(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")

	transactions, err := h.store.ListTransactions(agentID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to list transactions")
		return
	}

	if transactions == nil {
		transactions = []*models.Transaction{}
	}

	respondJSON(w, http.StatusOK, models.TransactionsListResponse{
		Transactions: transactions,
		Total:        len(transactions),
	})
}

// RegisterTools registers discovered tools from eBPF agent
func (h *TransactionHandler) RegisterTools(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")

	var req models.RegisterToolsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	for _, toolSchema := range req.Tools {
		// Convert input schema to json.RawMessage
		inputSchemaBytes, _ := json.Marshal(toolSchema.InputSchema)

		tool := &models.Tool{
			ID:          uuid.New().String(),
			AgentID:     agentID,
			Name:        toolSchema.Name,
			Description: toolSchema.Description,
			InputSchema: inputSchemaBytes,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		if err := h.store.SaveTool(tool); err != nil {
			log.Printf("Error saving tool: %v", err)
		} else {
			log.Printf("[eBPF] Registered tool %s for agent %s", tool.Name, agentID)
		}
	}

	respondJSON(w, http.StatusCreated, map[string]string{
		"message": "Tools registered",
	})
}

// DiscoverCompensation registers a discovered compensation mapping
func (h *TransactionHandler) DiscoverCompensation(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")

	var req models.DiscoverCompensationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Check if mapping already exists
	existing, _ := h.store.GetCompensationMappingByTool(agentID, req.ToolName)
	if existing != nil {
		respondJSON(w, http.StatusOK, map[string]string{
			"message": "Mapping already exists",
			"id":      existing.ID,
		})
		return
	}

	id, err := h.store.SaveCompensationMappingFromEBPF(agentID, &req)
	if err != nil {
		log.Printf("Error saving compensation mapping: %v", err)
		respondError(w, http.StatusInternalServerError, "Failed to save mapping")
		return
	}

	log.Printf("[eBPF] Discovered compensation: %s -> %s (agent: %s)",
		req.ToolName, req.CompensatorName, agentID)

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"message":          "Compensation mapping discovered",
		"id":               id,
		"pending_approval": true,
	})
}

// ListCompensationMappings returns all compensation mappings for an agent
func (h *TransactionHandler) ListCompensationMappings(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")

	mappings, err := h.store.ListCompensationMappings(agentID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to list mappings")
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"mappings": mappings,
		"total":    len(mappings),
	})
}

// GetApprovedMappings returns only approved compensation mappings
func (h *TransactionHandler) GetApprovedMappings(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")

	mappings, err := h.store.GetApprovedMappings(agentID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to list mappings")
		return
	}

	registry := make(map[string]models.EBPFCompensationRegistryEntry)
	for _, m := range mappings {
		registry[m.ToolName] = models.EBPFCompensationRegistryEntry{
			Compensator:      m.CompensatorName,
			ParameterMapping: m.ParameterMapping,
		}
	}

	respondJSON(w, http.StatusOK, models.EBPFCompensationMappingsResponse{
		Registry: registry,
	})
}

// ApproveCompensation approves or rejects a compensation mapping
func (h *TransactionHandler) ApproveCompensation(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")
	mappingID := chi.URLParam(r, "mappingId")

	var req struct {
		Approved bool `json:"approved"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := h.store.ApproveCompensationMappingByID(agentID, mappingID, req.Approved); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to update mapping")
		return
	}

	action := "approved"
	if !req.Approved {
		action = "rejected"
	}

	log.Printf("[Compensation] Mapping %s %s for agent %s", mappingID, action, agentID)

	respondJSON(w, http.StatusOK, map[string]string{
		"message": "Mapping " + action,
	})
}
