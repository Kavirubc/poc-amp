package models

import (
	"time"
)

type AgentStatus string

const (
	StatusPending  AgentStatus = "pending"
	StatusCloning  AgentStatus = "cloning"
	StatusBuilding AgentStatus = "building"
	StatusRunning  AgentStatus = "running"
	StatusStopped  AgentStatus = "stopped"
	StatusFailed   AgentStatus = "failed"
)

type AgentType string

const (
	TypePython AgentType = "python"
	TypeNode   AgentType = "node"
	TypeUnknown AgentType = "unknown"
)

type Agent struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	RepoURL     string      `json:"repo_url"`
	Branch      string      `json:"branch"`
	Type        AgentType   `json:"type"`
	Status      AgentStatus `json:"status"`
	Port        int         `json:"port"`
	ContainerID string      `json:"container_id,omitempty"`
	ImageID     string      `json:"image_id,omitempty"`
	EnvContent  string      `json:"env_content,omitempty"`
	Error       string      `json:"error,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

type CreateAgentRequest struct {
	Name       string `json:"name"`
	RepoURL    string `json:"repo_url"`
	Branch     string `json:"branch"`
	EnvContent string `json:"env_content"`
}

type AgentResponse struct {
	Agent   *Agent `json:"agent,omitempty"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

type AgentsListResponse struct {
	Agents []*Agent `json:"agents"`
	Total  int      `json:"total"`
}
