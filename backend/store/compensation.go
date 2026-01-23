package store

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/poc-amp/backend/models"
)

// Compensation Mappings

func (s *Store) CreateCompensationMapping(m *models.CompensationMapping) error {
	query := `
		INSERT INTO compensation_mappings
		(id, agent_id, tool_name, tool_schema, tool_description, compensator_name,
		 parameter_mapping, status, suggested_by, confidence, reasoning, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (agent_id, tool_name) DO UPDATE SET
			tool_schema = EXCLUDED.tool_schema,
			tool_description = EXCLUDED.tool_description,
			compensator_name = EXCLUDED.compensator_name,
			parameter_mapping = EXCLUDED.parameter_mapping,
			status = EXCLUDED.status,
			suggested_by = EXCLUDED.suggested_by,
			confidence = EXCLUDED.confidence,
			reasoning = EXCLUDED.reasoning,
			updated_at = EXCLUDED.updated_at
	`
	_, err := s.db.Exec(query,
		m.ID, m.AgentID, m.ToolName, m.ToolSchema, m.ToolDescription,
		m.CompensatorName, m.ParameterMapping, m.Status, m.SuggestedBy,
		m.Confidence, m.Reasoning, m.CreatedAt, m.UpdatedAt,
	)
	return err
}

func (s *Store) GetCompensationMapping(id string) (*models.CompensationMapping, error) {
	query := `
		SELECT id, agent_id, tool_name, tool_schema, tool_description, compensator_name,
		       parameter_mapping, status, suggested_by, confidence, reasoning,
		       reviewed_by, reviewed_at, created_at, updated_at
		FROM compensation_mappings WHERE id = $1
	`
	m := &models.CompensationMapping{}
	var reviewedAt sql.NullTime
	var reviewedBy sql.NullString

	err := s.db.QueryRow(query, id).Scan(
		&m.ID, &m.AgentID, &m.ToolName, &m.ToolSchema, &m.ToolDescription,
		&m.CompensatorName, &m.ParameterMapping, &m.Status, &m.SuggestedBy,
		&m.Confidence, &m.Reasoning, &reviewedBy, &reviewedAt,
		&m.CreatedAt, &m.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if reviewedAt.Valid {
		m.ReviewedAt = &reviewedAt.Time
	}
	if reviewedBy.Valid {
		m.ReviewedBy = reviewedBy.String
	}

	return m, nil
}

func (s *Store) GetCompensationMappingByTool(agentID, toolName string) (*models.CompensationMapping, error) {
	query := `
		SELECT id, agent_id, tool_name, tool_schema, tool_description, compensator_name,
		       parameter_mapping, status, suggested_by, confidence, reasoning,
		       reviewed_by, reviewed_at, created_at, updated_at
		FROM compensation_mappings WHERE agent_id = $1 AND tool_name = $2
	`
	m := &models.CompensationMapping{}
	var reviewedAt sql.NullTime
	var reviewedBy sql.NullString

	err := s.db.QueryRow(query, agentID, toolName).Scan(
		&m.ID, &m.AgentID, &m.ToolName, &m.ToolSchema, &m.ToolDescription,
		&m.CompensatorName, &m.ParameterMapping, &m.Status, &m.SuggestedBy,
		&m.Confidence, &m.Reasoning, &reviewedBy, &reviewedAt,
		&m.CreatedAt, &m.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if reviewedAt.Valid {
		m.ReviewedAt = &reviewedAt.Time
	}
	if reviewedBy.Valid {
		m.ReviewedBy = reviewedBy.String
	}

	return m, nil
}

func (s *Store) ListCompensationMappings(agentID string) ([]*models.CompensationMapping, error) {
	query := `
		SELECT id, agent_id, tool_name,
		       COALESCE(tool_schema, '{}') as tool_schema,
		       COALESCE(tool_description, '') as tool_description,
		       COALESCE(compensator_name, '') as compensator_name,
		       COALESCE(parameter_mapping, '{}') as parameter_mapping,
		       status, suggested_by, confidence,
		       COALESCE(reasoning, '') as reasoning,
		       reviewed_by, reviewed_at, created_at, updated_at
		FROM compensation_mappings WHERE agent_id = $1
		ORDER BY created_at DESC
	`
	rows, err := s.db.Query(query, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var mappings []*models.CompensationMapping
	for rows.Next() {
		m := &models.CompensationMapping{}
		var reviewedAt sql.NullTime
		var reviewedBy sql.NullString

		err := rows.Scan(
			&m.ID, &m.AgentID, &m.ToolName, &m.ToolSchema, &m.ToolDescription,
			&m.CompensatorName, &m.ParameterMapping, &m.Status, &m.SuggestedBy,
			&m.Confidence, &m.Reasoning, &reviewedBy, &reviewedAt,
			&m.CreatedAt, &m.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		if reviewedAt.Valid {
			m.ReviewedAt = &reviewedAt.Time
		}
		if reviewedBy.Valid {
			m.ReviewedBy = reviewedBy.String
		}

		mappings = append(mappings, m)
	}
	return mappings, nil
}

func (s *Store) UpdateCompensationMapping(m *models.CompensationMapping) error {
	query := `
		UPDATE compensation_mappings SET
			compensator_name = $2,
			parameter_mapping = $3,
			status = $4,
			reviewed_by = $5,
			reviewed_at = $6,
			updated_at = $7
		WHERE id = $1
	`
	m.UpdatedAt = time.Now()
	_, err := s.db.Exec(query,
		m.ID, m.CompensatorName, m.ParameterMapping, m.Status,
		m.ReviewedBy, m.ReviewedAt, m.UpdatedAt,
	)
	return err
}

func (s *Store) GetApprovedMappings(agentID string) ([]*models.CompensationMapping, error) {
	query := `
		SELECT id, agent_id, tool_name,
		       COALESCE(tool_schema, '{}') as tool_schema,
		       COALESCE(tool_description, '') as tool_description,
		       COALESCE(compensator_name, '') as compensator_name,
		       COALESCE(parameter_mapping, '{}') as parameter_mapping,
		       status, suggested_by, confidence,
		       COALESCE(reasoning, '') as reasoning,
		       reviewed_by, reviewed_at, created_at, updated_at
		FROM compensation_mappings
		WHERE agent_id = $1 AND status = 'approved'
	`
	rows, err := s.db.Query(query, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var mappings []*models.CompensationMapping
	for rows.Next() {
		m := &models.CompensationMapping{}
		var reviewedAt sql.NullTime
		var reviewedBy sql.NullString

		err := rows.Scan(
			&m.ID, &m.AgentID, &m.ToolName, &m.ToolSchema, &m.ToolDescription,
			&m.CompensatorName, &m.ParameterMapping, &m.Status, &m.SuggestedBy,
			&m.Confidence, &m.Reasoning, &reviewedBy, &reviewedAt,
			&m.CreatedAt, &m.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		if reviewedAt.Valid {
			m.ReviewedAt = &reviewedAt.Time
		}
		if reviewedBy.Valid {
			m.ReviewedBy = reviewedBy.String
		}

		mappings = append(mappings, m)
	}
	return mappings, nil
}

// Transaction Logs

func (s *Store) CreateTransactionLog(t *models.TransactionLog) error {
	query := `
		INSERT INTO transaction_logs
		(id, agent_id, session_id, tool_name, input_params, output_result, status, executed_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err := s.db.Exec(query,
		t.ID, t.AgentID, t.SessionID, t.ToolName, t.InputParams,
		t.OutputResult, t.Status, t.ExecutedAt, t.CreatedAt,
	)
	return err
}

func (s *Store) GetTransactionLog(id string) (*models.TransactionLog, error) {
	query := `
		SELECT id, agent_id, session_id, tool_name, input_params, output_result,
		       status, executed_at, compensated_at, compensation_id, compensation_result, created_at
		FROM transaction_logs WHERE id = $1
	`
	t := &models.TransactionLog{}
	var compensatedAt sql.NullTime
	var compensationID sql.NullString
	var compensationResult []byte

	err := s.db.QueryRow(query, id).Scan(
		&t.ID, &t.AgentID, &t.SessionID, &t.ToolName, &t.InputParams,
		&t.OutputResult, &t.Status, &t.ExecutedAt, &compensatedAt,
		&compensationID, &compensationResult, &t.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if compensatedAt.Valid {
		t.CompensatedAt = &compensatedAt.Time
	}
	if compensationID.Valid {
		t.CompensationID = compensationID.String
	}
	if compensationResult != nil {
		t.CompensationResult = json.RawMessage(compensationResult)
	}

	return t, nil
}

func (s *Store) GetSessionTransactions(agentID, sessionID string) ([]*models.TransactionLog, error) {
	query := `
		SELECT id, agent_id, session_id, tool_name, input_params, output_result,
		       status, executed_at, compensated_at, compensation_id, compensation_result, created_at
		FROM transaction_logs
		WHERE agent_id = $1 AND session_id = $2 AND status = 'executed'
		ORDER BY executed_at DESC
	`
	rows, err := s.db.Query(query, agentID, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*models.TransactionLog
	for rows.Next() {
		t := &models.TransactionLog{}
		var compensatedAt sql.NullTime
		var compensationID sql.NullString
		var compensationResult []byte

		err := rows.Scan(
			&t.ID, &t.AgentID, &t.SessionID, &t.ToolName, &t.InputParams,
			&t.OutputResult, &t.Status, &t.ExecutedAt, &compensatedAt,
			&compensationID, &compensationResult, &t.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		if compensatedAt.Valid {
			t.CompensatedAt = &compensatedAt.Time
		}
		if compensationID.Valid {
			t.CompensationID = compensationID.String
		}
		if compensationResult != nil {
			t.CompensationResult = json.RawMessage(compensationResult)
		}

		logs = append(logs, t)
	}
	return logs, nil
}

func (s *Store) MarkTransactionCompensated(id string, result json.RawMessage) error {
	now := time.Now()
	query := `
		UPDATE transaction_logs SET
			status = 'compensated',
			compensated_at = $2,
			compensation_result = $3
		WHERE id = $1
	`
	_, err := s.db.Exec(query, id, now, result)
	return err
}
