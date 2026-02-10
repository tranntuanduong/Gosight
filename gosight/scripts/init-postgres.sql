-- ===========================================
-- GoSight PostgreSQL Schema
-- Metadata & User Management
-- ===========================================

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ===========================================
-- Users Table
-- Dashboard users
-- ===========================================
CREATE TABLE IF NOT EXISTS users (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email           VARCHAR(255) UNIQUE NOT NULL,
    password_hash   VARCHAR(255) NOT NULL,
    name            VARCHAR(255),
    avatar_url      VARCHAR(512),

    -- Status
    is_active       BOOLEAN DEFAULT true,
    is_verified     BOOLEAN DEFAULT false,

    -- Timestamps
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    last_login_at   TIMESTAMP WITH TIME ZONE
);

-- Index for login queries
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);

-- ===========================================
-- Projects Table
-- Websites/apps being tracked
-- ===========================================
CREATE TABLE IF NOT EXISTS projects (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name            VARCHAR(255) NOT NULL,
    domain          VARCHAR(255) NOT NULL,

    -- Owner
    owner_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Settings (JSON)
    settings        JSONB DEFAULT '{}',

    -- Status
    is_active       BOOLEAN DEFAULT true,

    -- Timestamps
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_projects_owner ON projects(owner_id);
CREATE INDEX IF NOT EXISTS idx_projects_domain ON projects(domain);

-- ===========================================
-- API Keys Table
-- For SDK authentication
-- ===========================================
CREATE TABLE IF NOT EXISTS api_keys (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,

    -- Key info
    key_hash        VARCHAR(255) NOT NULL,  -- Hashed API key
    key_prefix      VARCHAR(10) NOT NULL,   -- First 8 chars for identification
    name            VARCHAR(255) NOT NULL,

    -- Permissions
    permissions     JSONB DEFAULT '["ingest"]',  -- ingest, read, admin

    -- Rate limiting
    rate_limit      INTEGER DEFAULT 1000,  -- requests per minute

    -- Status
    is_active       BOOLEAN DEFAULT true,
    last_used_at    TIMESTAMP WITH TIME ZONE,

    -- Timestamps
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    expires_at      TIMESTAMP WITH TIME ZONE
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_api_keys_project ON api_keys(project_id);
CREATE INDEX IF NOT EXISTS idx_api_keys_prefix ON api_keys(key_prefix);

-- ===========================================
-- Team Members Table
-- Project collaborators
-- ===========================================
CREATE TABLE IF NOT EXISTS team_members (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Role: owner, admin, member, viewer
    role            VARCHAR(50) NOT NULL DEFAULT 'member',

    -- Timestamps
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    -- Unique constraint
    UNIQUE(project_id, user_id)
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_team_members_project ON team_members(project_id);
CREATE INDEX IF NOT EXISTS idx_team_members_user ON team_members(user_id);

-- ===========================================
-- Alerts Table
-- Alert configurations
-- ===========================================
CREATE TABLE IF NOT EXISTS alerts (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    created_by      UUID NOT NULL REFERENCES users(id),

    -- Alert config
    name            VARCHAR(255) NOT NULL,
    description     TEXT,

    -- Condition (JSON)
    -- Example: {"metric": "error_rate", "operator": ">", "threshold": 5, "window": "5m"}
    condition       JSONB NOT NULL,

    -- Notification channels (JSON array)
    -- Example: [{"type": "email", "to": "dev@example.com"}, {"type": "slack", "webhook": "..."}]
    channels        JSONB NOT NULL DEFAULT '[]',

    -- Status
    is_active       BOOLEAN DEFAULT true,
    last_triggered  TIMESTAMP WITH TIME ZONE,

    -- Timestamps
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_alerts_project ON alerts(project_id);

-- ===========================================
-- Alert History Table
-- Record of triggered alerts
-- ===========================================
CREATE TABLE IF NOT EXISTS alert_history (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    alert_id        UUID NOT NULL REFERENCES alerts(id) ON DELETE CASCADE,

    -- Trigger info
    triggered_at    TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    resolved_at     TIMESTAMP WITH TIME ZONE,

    -- Context (JSON)
    context         JSONB NOT NULL DEFAULT '{}',

    -- Status: triggered, acknowledged, resolved
    status          VARCHAR(50) DEFAULT 'triggered'
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_alert_history_alert ON alert_history(alert_id);
CREATE INDEX IF NOT EXISTS idx_alert_history_triggered ON alert_history(triggered_at);

-- ===========================================
-- Sessions Table (for refresh tokens)
-- ===========================================
CREATE TABLE IF NOT EXISTS refresh_tokens (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    token_hash      VARCHAR(255) NOT NULL,
    user_agent      VARCHAR(512),
    ip_address      VARCHAR(45),

    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    expires_at      TIMESTAMP WITH TIME ZONE NOT NULL,
    revoked_at      TIMESTAMP WITH TIME ZONE
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user ON refresh_tokens(user_id);

-- ===========================================
-- Updated_at trigger function
-- ===========================================
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Apply trigger to tables with updated_at
CREATE TRIGGER update_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_projects_updated_at
    BEFORE UPDATE ON projects
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_alerts_updated_at
    BEFORE UPDATE ON alerts
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
