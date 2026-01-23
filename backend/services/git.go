package services

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/poc-amp/backend/models"
)

type GitService struct {
	workspacePath string
}

func NewGitService(workspacePath string) *GitService {
	return &GitService{workspacePath: workspacePath}
}

func (g *GitService) CloneRepo(agentID, repoURL, branch string) (string, error) {
	repoPath := filepath.Join(g.workspacePath, agentID)

	if err := os.MkdirAll(repoPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	cloneOptions := &git.CloneOptions{
		URL:      repoURL,
		Progress: os.Stdout,
		Depth:    1,
	}

	if branch != "" && branch != "main" && branch != "master" {
		cloneOptions.ReferenceName = plumbing.NewBranchReferenceName(branch)
		cloneOptions.SingleBranch = true
	}

	_, err := git.PlainClone(repoPath, false, cloneOptions)
	if err != nil {
		os.RemoveAll(repoPath)
		return "", fmt.Errorf("failed to clone repository: %w", err)
	}

	return repoPath, nil
}

func (g *GitService) DetectAgentType(repoPath string) models.AgentType {
	if _, err := os.Stat(filepath.Join(repoPath, "requirements.txt")); err == nil {
		return models.TypePython
	}
	if _, err := os.Stat(filepath.Join(repoPath, "pyproject.toml")); err == nil {
		return models.TypePython
	}
	if _, err := os.Stat(filepath.Join(repoPath, "package.json")); err == nil {
		return models.TypeNode
	}
	return models.TypeUnknown
}

func (g *GitService) WriteEnvFile(repoPath, envContent string) error {
	if envContent == "" {
		return nil
	}
	envPath := filepath.Join(repoPath, ".env")
	return os.WriteFile(envPath, []byte(envContent), 0644)
}

func (g *GitService) CleanupRepo(agentID string) error {
	repoPath := filepath.Join(g.workspacePath, agentID)
	return os.RemoveAll(repoPath)
}

func (g *GitService) GetRepoPath(agentID string) string {
	return filepath.Join(g.workspacePath, agentID)
}
