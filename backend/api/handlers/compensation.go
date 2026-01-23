package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/poc-amp/backend/models"
	"github.com/poc-amp/backend/services"
	"github.com/poc-amp/backend/store"
)

type CompensationHandler struct {
	store             *store.Store
	suggestionService *services.SuggestionService
	recoveryService   *services.RecoveryService
}

func NewCompensationHandler(store *store.Store, suggestionSvc *services.SuggestionService, recoverySvc *services.RecoveryService) *CompensationHandler {
	return &CompensationHandler{
		store:             store,
		suggestionService: suggestionSvc,
		recoveryService:   recoverySvc,
	}
}

// RegisterTools handles POST /api/v1/agents/:id/tools
func (h *CompensationHandler) RegisterTools(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")

	var req models.RegisterToolsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	req.AgentID = agentID

	mappings, err := h.suggestionService.RegisterTools(agentID, req.Tools)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, models.CompensationMappingsListResponse{
		Mappings: mappings,
		Total:    len(mappings),
	})
}

// ListMappings handles GET /api/v1/agents/:id/compensation-mappings
func (h *CompensationHandler) ListMappings(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")

	mappings, err := h.store.ListCompensationMappings(agentID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if mappings == nil {
		mappings = []*models.CompensationMapping{}
	}

	respondJSON(w, http.StatusOK, models.CompensationMappingsListResponse{
		Mappings: mappings,
		Total:    len(mappings),
	})
}

// GetMapping handles GET /api/v1/compensation-mappings/:mappingId
func (h *CompensationHandler) GetMapping(w http.ResponseWriter, r *http.Request) {
	mappingID := chi.URLParam(r, "mappingId")

	mapping, err := h.store.GetCompensationMapping(mappingID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if mapping == nil {
		respondError(w, http.StatusNotFound, "Mapping not found")
		return
	}

	respondJSON(w, http.StatusOK, models.CompensationMappingResponse{Mapping: mapping})
}

// ApproveMapping handles POST /api/v1/compensation-mappings/:mappingId/approve
func (h *CompensationHandler) ApproveMapping(w http.ResponseWriter, r *http.Request) {
	mappingID := chi.URLParam(r, "mappingId")

	var req models.ApproveCompensationRequest
	json.NewDecoder(r.Body).Decode(&req)

	if err := h.suggestionService.ApproveMapping(mappingID, req.CompensatorName, req.ParameterMapping, "admin"); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	mapping, _ := h.store.GetCompensationMapping(mappingID)
	respondJSON(w, http.StatusOK, models.CompensationMappingResponse{
		Mapping: mapping,
		Message: "Mapping approved",
	})
}

// RejectMapping handles POST /api/v1/compensation-mappings/:mappingId/reject
func (h *CompensationHandler) RejectMapping(w http.ResponseWriter, r *http.Request) {
	mappingID := chi.URLParam(r, "mappingId")

	var req struct {
		NoCompensation bool `json:"no_compensation"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if err := h.suggestionService.RejectMapping(mappingID, req.NoCompensation, "admin"); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	mapping, _ := h.store.GetCompensationMapping(mappingID)
	respondJSON(w, http.StatusOK, models.CompensationMappingResponse{
		Mapping: mapping,
		Message: "Mapping rejected",
	})
}

// UpdateMapping handles PUT /api/v1/compensation-mappings/:mappingId
func (h *CompensationHandler) UpdateMapping(w http.ResponseWriter, r *http.Request) {
	mappingID := chi.URLParam(r, "mappingId")

	mapping, err := h.store.GetCompensationMapping(mappingID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if mapping == nil {
		respondError(w, http.StatusNotFound, "Mapping not found")
		return
	}

	var req models.ApproveCompensationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.CompensatorName != "" {
		mapping.CompensatorName = req.CompensatorName
	}
	if req.ParameterMapping != nil {
		mapping.ParameterMapping = req.ParameterMapping
	}

	if err := h.store.UpdateCompensationMapping(mapping); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, models.CompensationMappingResponse{
		Mapping: mapping,
		Message: "Mapping updated",
	})
}

// LogExecution handles POST /api/v1/agents/:id/transactions
func (h *CompensationHandler) LogExecution(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")

	var req struct {
		SessionID string          `json:"session_id"`
		ToolName  string          `json:"tool_name"`
		Input     json.RawMessage `json:"input"`
		Output    json.RawMessage `json:"output"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	// Ensure we have valid JSON for input and output
	if req.Input == nil {
		req.Input = json.RawMessage("{}")
	}
	if req.Output == nil {
		req.Output = json.RawMessage("{}")
	}

	log, err := h.recoveryService.LogToolExecution(agentID, req.SessionID, req.ToolName, req.Input, req.Output)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to log execution: "+err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"transaction_id": log.ID,
		"message":        "Execution logged",
	})
}

// GetRollbackPlan handles GET /api/v1/agents/:id/sessions/:sessionId/rollback-plan
func (h *CompensationHandler) GetRollbackPlan(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")
	sessionID := chi.URLParam(r, "sessionId")

	plan, err := h.recoveryService.GenerateRollbackPlan(agentID, sessionID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"agent_id":   agentID,
		"session_id": sessionID,
		"steps":      plan,
		"total":      len(plan),
	})
}

// ExecuteRollback handles POST /api/v1/agents/:id/sessions/:sessionId/rollback
func (h *CompensationHandler) ExecuteRollback(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")
	sessionID := chi.URLParam(r, "sessionId")

	plan, err := h.recoveryService.GenerateRollbackPlan(agentID, sessionID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// In a real implementation, this would call the agent's compensation tools
	// For the POC, we return the plan that would be executed
	result := models.RollbackResult{
		TotalTransactions: len(plan),
	}

	for _, step := range plan {
		switch step.Action {
		case "compensate":
			// In production, would call the compensator tool here
			result.Compensated++
			h.recoveryService.MarkCompensated(step.TransactionID, nil)
		case "skip":
			result.Skipped++
		}
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"result": result,
		"plan":   plan,
	})
}

// GetApprovedMappings handles GET /api/v1/agents/:id/compensation-mappings/approved
func (h *CompensationHandler) GetApprovedMappings(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")

	mappings, err := h.store.GetApprovedMappings(agentID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if mappings == nil {
		mappings = []*models.CompensationMapping{}
	}

	// Convert to a simpler format for agents to consume
	registry := make(map[string]interface{})
	for _, m := range mappings {
		var paramMap map[string]string
		json.Unmarshal(m.ParameterMapping, &paramMap)

		registry[m.ToolName] = map[string]interface{}{
			"compensator":       m.CompensatorName,
			"parameter_mapping": paramMap,
		}
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"agent_id": agentID,
		"registry": registry,
	})
}
