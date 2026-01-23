-- Compensation Mappings Table
CREATE TABLE IF NOT EXISTS compensation_mappings (
    id VARCHAR(36) PRIMARY KEY,
    agent_id VARCHAR(36) NOT NULL REFERENCES agents(id) ON DELETE CASCADE,

    -- Tool info
    tool_name VARCHAR(255) NOT NULL,
    tool_schema JSONB,
    tool_description TEXT DEFAULT '',

    -- Compensation info
    compensator_name VARCHAR(255),
    parameter_mapping JSONB DEFAULT '{}',

    -- Approval workflow
    status VARCHAR(50) NOT NULL DEFAULT 'pending',  -- pending, approved, rejected, no_compensation
    suggested_by VARCHAR(50) DEFAULT 'unknown',     -- heuristic, llm, manual
    confidence DECIMAL(3,2) DEFAULT 0.0,
    reasoning TEXT DEFAULT '',

    reviewed_by VARCHAR(255),
    reviewed_at TIMESTAMP,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    UNIQUE(agent_id, tool_name)
);

CREATE INDEX IF NOT EXISTS idx_compensation_mappings_agent ON compensation_mappings(agent_id);
CREATE INDEX IF NOT EXISTS idx_compensation_mappings_status ON compensation_mappings(status);

-- Transaction Log Table (for tracking tool executions)
CREATE TABLE IF NOT EXISTS transaction_logs (
    id VARCHAR(36) PRIMARY KEY,
    agent_id VARCHAR(36) NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    session_id VARCHAR(36) NOT NULL,

    -- Execution info
    tool_name VARCHAR(255) NOT NULL,
    input_params JSONB DEFAULT '{}',
    output_result JSONB DEFAULT '{}',

    -- Status
    status VARCHAR(50) NOT NULL DEFAULT 'executed',  -- executed, compensated, failed
    executed_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    compensated_at TIMESTAMP,

    -- Compensation tracking
    compensation_id VARCHAR(36),
    compensation_result JSONB,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_transaction_logs_agent ON transaction_logs(agent_id);
CREATE INDEX IF NOT EXISTS idx_transaction_logs_session ON transaction_logs(session_id);
CREATE INDEX IF NOT EXISTS idx_transaction_logs_status ON transaction_logs(status);
