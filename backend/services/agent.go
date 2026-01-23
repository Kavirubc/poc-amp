package services

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/poc-amp/backend/models"
	"github.com/poc-amp/backend/store"
)

type AgentService struct {
	store          *store.Store
	gitService     *GitService
	dockerService  *DockerService
	portRangeStart int
	portRangeEnd   int
}

func NewAgentService(store *store.Store, gitService *GitService, dockerService *DockerService, portStart, portEnd int) *AgentService {
	return &AgentService{
		store:          store,
		gitService:     gitService,
		dockerService:  dockerService,
		portRangeStart: portStart,
		portRangeEnd:   portEnd,
	}
}

func (s *AgentService) CreateAgent(req *models.CreateAgentRequest) (*models.Agent, error) {
	port, err := s.allocatePort()
	if err != nil {
		return nil, err
	}

	agent := &models.Agent{
		ID:         uuid.New().String(),
		Name:       req.Name,
		RepoURL:    req.RepoURL,
		Branch:     req.Branch,
		Status:     models.StatusPending,
		Port:       port,
		EnvContent: req.EnvContent,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if agent.Branch == "" {
		agent.Branch = "main"
	}

	if err := s.store.CreateAgent(agent); err != nil {
		return nil, fmt.Errorf("failed to create agent: %w", err)
	}

	go s.deployAgent(agent)

	return agent, nil
}

func (s *AgentService) deployAgent(agent *models.Agent) {
	ctx := context.Background()

	s.updateStatus(agent, models.StatusCloning, "")

	repoPath, err := s.gitService.CloneRepo(agent.ID, agent.RepoURL, agent.Branch)
	if err != nil {
		s.updateStatus(agent, models.StatusFailed, fmt.Sprintf("Clone failed: %v", err))
		return
	}

	agentType := s.gitService.DetectAgentType(repoPath)
	if agentType == models.TypeUnknown {
		s.updateStatus(agent, models.StatusFailed, "Could not detect agent type (no requirements.txt or package.json found)")
		return
	}
	agent.Type = agentType

	if err := s.gitService.WriteEnvFile(repoPath, agent.EnvContent); err != nil {
		s.updateStatus(agent, models.StatusFailed, fmt.Sprintf("Failed to write .env: %v", err))
		return
	}

	s.updateStatus(agent, models.StatusBuilding, "")

	imageID, err := s.dockerService.BuildImage(ctx, agent.ID, repoPath, agentType)
	if err != nil {
		s.updateStatus(agent, models.StatusFailed, fmt.Sprintf("Build failed: %v", err))
		return
	}
	agent.ImageID = imageID

	containerID, err := s.dockerService.StartContainer(ctx, agent, repoPath, agent.Port)
	if err != nil {
		s.updateStatus(agent, models.StatusFailed, fmt.Sprintf("Start failed: %v", err))
		return
	}
	agent.ContainerID = containerID

	if err := s.dockerService.WaitForContainer(ctx, containerID, 30*time.Second); err != nil {
		s.updateStatus(agent, models.StatusFailed, fmt.Sprintf("Container health check failed: %v", err))
		return
	}

	s.updateStatus(agent, models.StatusRunning, "")
	log.Printf("Agent %s deployed successfully on port %d", agent.Name, agent.Port)
}

func (s *AgentService) GetAgent(id string) (*models.Agent, error) {
	return s.store.GetAgent(id)
}

func (s *AgentService) ListAgents() ([]*models.Agent, error) {
	return s.store.ListAgents()
}

func (s *AgentService) StartAgent(id string) error {
	agent, err := s.store.GetAgent(id)
	if err != nil {
		return err
	}
	if agent == nil {
		return fmt.Errorf("agent not found")
	}

	if agent.Status == models.StatusRunning {
		return fmt.Errorf("agent is already running")
	}

	if agent.ContainerID != "" {
		ctx := context.Background()
		if s.dockerService.IsContainerRunning(ctx, agent.ContainerID) {
			s.updateStatus(agent, models.StatusRunning, "")
			return nil
		}
	}

	go s.deployAgent(agent)
	return nil
}

func (s *AgentService) StopAgent(id string) error {
	agent, err := s.store.GetAgent(id)
	if err != nil {
		return err
	}
	if agent == nil {
		return fmt.Errorf("agent not found")
	}

	if agent.ContainerID != "" {
		ctx := context.Background()
		if err := s.dockerService.StopContainer(ctx, agent.ContainerID); err != nil {
			log.Printf("Failed to stop container: %v", err)
		}
	}

	s.updateStatus(agent, models.StatusStopped, "")
	return nil
}

func (s *AgentService) DeleteAgent(id string) error {
	agent, err := s.store.GetAgent(id)
	if err != nil {
		return err
	}
	if agent == nil {
		return fmt.Errorf("agent not found")
	}

	ctx := context.Background()

	if agent.ContainerID != "" {
		if err := s.dockerService.RemoveContainer(ctx, agent.ContainerID); err != nil {
			log.Printf("Failed to remove container: %v", err)
		}
	}

	if agent.ImageID != "" {
		if err := s.dockerService.RemoveImage(ctx, agent.ImageID); err != nil {
			log.Printf("Failed to remove image: %v", err)
		}
	}

	if err := s.gitService.CleanupRepo(id); err != nil {
		log.Printf("Failed to cleanup repo: %v", err)
	}

	return s.store.DeleteAgent(id)
}

func (s *AgentService) GetLogs(id string, follow bool) (io.ReadCloser, error) {
	agent, err := s.store.GetAgent(id)
	if err != nil {
		return nil, err
	}
	if agent == nil {
		return nil, fmt.Errorf("agent not found")
	}
	if agent.ContainerID == "" {
		return nil, fmt.Errorf("no container associated with agent")
	}

	ctx := context.Background()
	return s.dockerService.GetContainerLogs(ctx, agent.ContainerID, follow)
}

func (s *AgentService) updateStatus(agent *models.Agent, status models.AgentStatus, errorMsg string) {
	agent.Status = status
	agent.Error = errorMsg
	agent.UpdatedAt = time.Now()
	if err := s.store.UpdateAgent(agent); err != nil {
		log.Printf("Failed to update agent status: %v", err)
	}
}

func (s *AgentService) allocatePort() (int, error) {
	usedPorts, err := s.store.GetUsedPorts()
	if err != nil {
		return 0, fmt.Errorf("failed to get used ports: %w", err)
	}

	usedSet := make(map[int]bool)
	for _, p := range usedPorts {
		usedSet[p] = true
	}

	for port := s.portRangeStart; port <= s.portRangeEnd; port++ {
		if !usedSet[port] {
			return port, nil
		}
	}

	return 0, fmt.Errorf("no available ports in range %d-%d", s.portRangeStart, s.portRangeEnd)
}
