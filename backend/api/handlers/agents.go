package handlers

import (
	"bufio"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/poc-amp/backend/models"
	"github.com/poc-amp/backend/services"
)

type AgentHandler struct {
	agentService *services.AgentService
}

func NewAgentHandler(agentService *services.AgentService) *AgentHandler {
	return &AgentHandler{agentService: agentService}
}

func (h *AgentHandler) ListAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := h.agentService.ListAgents()
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if agents == nil {
		agents = []*models.Agent{}
	}

	respondJSON(w, http.StatusOK, models.AgentsListResponse{
		Agents: agents,
		Total:  len(agents),
	})
}

func (h *AgentHandler) CreateAgent(w http.ResponseWriter, r *http.Request) {
	var req models.CreateAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "Name is required")
		return
	}
	if req.RepoURL == "" {
		respondError(w, http.StatusBadRequest, "Repository URL is required")
		return
	}

	agent, err := h.agentService.CreateAgent(&req)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, models.AgentResponse{
		Agent:   agent,
		Message: "Agent creation started",
	})
}

func (h *AgentHandler) GetAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	agent, err := h.agentService.GetAgent(id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if agent == nil {
		respondError(w, http.StatusNotFound, "Agent not found")
		return
	}

	respondJSON(w, http.StatusOK, models.AgentResponse{Agent: agent})
}

func (h *AgentHandler) DeleteAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.agentService.DeleteAgent(id); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, models.AgentResponse{Message: "Agent deleted"})
}

func (h *AgentHandler) StartAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.agentService.StartAgent(id); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, models.AgentResponse{Message: "Agent start initiated"})
}

func (h *AgentHandler) StopAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.agentService.StopAgent(id); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, models.AgentResponse{Message: "Agent stopped"})
}

func (h *AgentHandler) StreamLogs(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		respondError(w, http.StatusInternalServerError, "Streaming not supported")
		return
	}

	logs, err := h.agentService.GetLogs(id, true)
	if err != nil {
		w.Write([]byte("event: error\ndata: " + err.Error() + "\n\n"))
		flusher.Flush()
		return
	}
	defer logs.Close()

	scanner := bufio.NewScanner(logs)
	for scanner.Scan() {
		select {
		case <-r.Context().Done():
			return
		default:
			line := scanner.Text()
			if len(line) > 8 {
				line = line[8:]
			}
			w.Write([]byte("data: " + line + "\n\n"))
			flusher.Flush()
		}
	}
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, models.AgentResponse{Error: message})
}
