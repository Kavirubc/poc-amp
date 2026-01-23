package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
	"github.com/poc-amp/backend/models"
)

type Store struct {
	db *sql.DB
}

func New(databaseURL string) (*Store, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) CreateAgent(agent *models.Agent) error {
	query := `
		INSERT INTO agents (id, name, repo_url, branch, type, status, port, container_id, image_id, env_content, error, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`
	_, err := s.db.Exec(query,
		agent.ID, agent.Name, agent.RepoURL, agent.Branch, agent.Type, agent.Status,
		agent.Port, agent.ContainerID, agent.ImageID, agent.EnvContent, agent.Error,
		agent.CreatedAt, agent.UpdatedAt,
	)
	return err
}

func (s *Store) GetAgent(id string) (*models.Agent, error) {
	query := `
		SELECT id, name, repo_url, branch, type, status, port, container_id, image_id, env_content, error, created_at, updated_at
		FROM agents WHERE id = $1
	`
	agent := &models.Agent{}
	err := s.db.QueryRow(query, id).Scan(
		&agent.ID, &agent.Name, &agent.RepoURL, &agent.Branch, &agent.Type, &agent.Status,
		&agent.Port, &agent.ContainerID, &agent.ImageID, &agent.EnvContent, &agent.Error,
		&agent.CreatedAt, &agent.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return agent, nil
}

func (s *Store) ListAgents() ([]*models.Agent, error) {
	query := `
		SELECT id, name, repo_url, branch, type, status, port, container_id, image_id, env_content, error, created_at, updated_at
		FROM agents ORDER BY created_at DESC
	`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []*models.Agent
	for rows.Next() {
		agent := &models.Agent{}
		err := rows.Scan(
			&agent.ID, &agent.Name, &agent.RepoURL, &agent.Branch, &agent.Type, &agent.Status,
			&agent.Port, &agent.ContainerID, &agent.ImageID, &agent.EnvContent, &agent.Error,
			&agent.CreatedAt, &agent.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		agents = append(agents, agent)
	}
	return agents, nil
}

func (s *Store) UpdateAgent(agent *models.Agent) error {
	query := `
		UPDATE agents SET
			name = $2, repo_url = $3, branch = $4, type = $5, status = $6,
			port = $7, container_id = $8, image_id = $9, env_content = $10,
			error = $11, updated_at = $12
		WHERE id = $1
	`
	agent.UpdatedAt = time.Now()
	_, err := s.db.Exec(query,
		agent.ID, agent.Name, agent.RepoURL, agent.Branch, agent.Type, agent.Status,
		agent.Port, agent.ContainerID, agent.ImageID, agent.EnvContent, agent.Error,
		agent.UpdatedAt,
	)
	return err
}

func (s *Store) DeleteAgent(id string) error {
	_, err := s.db.Exec("DELETE FROM agents WHERE id = $1", id)
	return err
}

func (s *Store) GetUsedPorts() ([]int, error) {
	query := `SELECT port FROM agents WHERE status IN ('running', 'building', 'cloning', 'pending')`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ports []int
	for rows.Next() {
		var port int
		if err := rows.Scan(&port); err != nil {
			return nil, err
		}
		if port > 0 {
			ports = append(ports, port)
		}
	}
	return ports, nil
}
