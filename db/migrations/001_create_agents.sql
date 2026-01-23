CREATE TABLE IF NOT EXISTS agents (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    repo_url TEXT NOT NULL,
    branch VARCHAR(255) DEFAULT 'main',
    type VARCHAR(50) DEFAULT 'unknown',
    status VARCHAR(50) DEFAULT 'pending',
    port INTEGER DEFAULT 0,
    container_id VARCHAR(255) DEFAULT '',
    image_id VARCHAR(255) DEFAULT '',
    env_content TEXT DEFAULT '',
    error TEXT DEFAULT '',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_agents_status ON agents(status);
CREATE INDEX IF NOT EXISTS idx_agents_name ON agents(name);
