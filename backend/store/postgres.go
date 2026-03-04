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

// EnsureEnvoyAgent upserts a synthetic sentinel agent row so that Envoy-intercepted
// transaction logs satisfy the FK constraint on agent_id.
func (s *Store) EnsureEnvoyAgent(agentID string) error {
	query := `
		INSERT INTO agents (id, name, repo_url, branch, type, status, created_at, updated_at)
		VALUES ($1, $2, '', 'main', 'envoy-interceptor', 'running', NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`
	_, err := s.db.Exec(query, agentID, "Envoy Network Interceptor ("+agentID+")")
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

// Transaction methods (for eBPF agent)

func (s *Store) SaveTransaction(tx *models.Transaction) error {
	query := `
		INSERT INTO transactions (id, agent_id, session_id, tool_name, input, output, status, started_at, completed_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (id) DO UPDATE SET
			output = EXCLUDED.output,
			status = EXCLUDED.status,
			completed_at = EXCLUDED.completed_at
	`
	_, err := s.db.Exec(query,
		tx.ID, tx.AgentID, tx.SessionID, tx.ToolName, tx.Input, tx.Output,
		tx.Status, tx.StartedAt, tx.CompletedAt, tx.CreatedAt,
	)
	return err
}

func (s *Store) ListTransactions(agentID string) ([]*models.Transaction, error) {
	query := `
		SELECT id, agent_id, session_id, tool_name, input, output, status, started_at, completed_at, created_at
		FROM transactions WHERE agent_id = $1 ORDER BY created_at DESC LIMIT 100
	`
	rows, err := s.db.Query(query, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var txs []*models.Transaction
	for rows.Next() {
		tx := &models.Transaction{}
		err := rows.Scan(
			&tx.ID, &tx.AgentID, &tx.SessionID, &tx.ToolName, &tx.Input, &tx.Output,
			&tx.Status, &tx.StartedAt, &tx.CompletedAt, &tx.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		txs = append(txs, tx)
	}
	return txs, nil
}

// Tool methods (for eBPF agent)

func (s *Store) SaveTool(tool *models.Tool) error {
	query := `
		INSERT INTO tools (id, agent_id, name, description, endpoint, input_schema, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (agent_id, name) DO UPDATE SET
			description = EXCLUDED.description,
			endpoint = EXCLUDED.endpoint,
			input_schema = EXCLUDED.input_schema,
			updated_at = EXCLUDED.updated_at
	`
	_, err := s.db.Exec(query,
		tool.ID, tool.AgentID, tool.Name, tool.Description, tool.Endpoint, tool.InputSchema,
		tool.CreatedAt, tool.UpdatedAt,
	)
	return err
}

// SaveCompensationMappingFromEBPF creates a new compensation mapping from eBPF discovery
func (s *Store) SaveCompensationMappingFromEBPF(agentID string, req *models.DiscoverCompensationRequest) (string, error) {
	id := fmt.Sprintf("cm-%d", time.Now().UnixNano())

	// Map suggestion source
	suggestedBy := models.SuggestionHeuristic
	if req.SuggestedBy == "llm" {
		suggestedBy = models.SuggestionLLM
	} else if req.SuggestedBy == "manual" {
		suggestedBy = models.SuggestionManual
	}

	query := `
		INSERT INTO compensation_mappings (id, agent_id, tool_name, compensator_name, parameter_mapping, status, suggested_by, confidence, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (agent_id, tool_name) DO NOTHING
	`
	_, err := s.db.Exec(query,
		id, agentID, req.ToolName, req.CompensatorName, req.ParameterMapping,
		models.MappingStatusPending, suggestedBy, 0.7, time.Now(), time.Now(),
	)
	if err != nil {
		return "", err
	}
	return id, nil
}

// ApproveCompensationMappingByID approves or rejects a compensation mapping
func (s *Store) ApproveCompensationMappingByID(agentID, mappingID string, approved bool) error {
	status := "approved"
	if !approved {
		status = "rejected"
	}

	query := `
		UPDATE compensation_mappings SET status = $3, reviewed_at = $4, updated_at = $4
		WHERE id = $1 AND agent_id = $2
	`
	result, err := s.db.Exec(query, mappingID, agentID, status, time.Now())
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("mapping not found")
	}

	return nil
}
