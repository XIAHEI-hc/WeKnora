-- Migration 000115: PixLab project-scoped knowledge workbench bindings.
CREATE TABLE IF NOT EXISTS pixlab_project_bindings (
    project_code VARCHAR(64) PRIMARY KEY,
    project_name VARCHAR(255) NOT NULL DEFAULT '',
    tenant_id BIGINT NOT NULL,
    knowledge_base_id VARCHAR(64) NOT NULL UNIQUE,
    agent_id VARCHAR(64) NOT NULL,
    status VARCHAR(24) NOT NULL DEFAULT 'provisioning',
    revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_pixlab_project_bindings_tenant
    ON pixlab_project_bindings (tenant_id);
CREATE INDEX IF NOT EXISTS idx_pixlab_project_bindings_status
    ON pixlab_project_bindings (status);

ALTER TABLE sessions
    ADD COLUMN IF NOT EXISTS pixlab_project_code VARCHAR(64) DEFAULT NULL;

CREATE INDEX IF NOT EXISTS idx_sessions_pixlab_scope
    ON sessions (tenant_id, pixlab_project_code, user_id, updated_at DESC)
    WHERE pixlab_project_code IS NOT NULL;

COMMENT ON TABLE pixlab_project_bindings IS
    'Server-managed mapping from a PixLab project to one WeKnora tenant, knowledge base and agent';
COMMENT ON COLUMN sessions.pixlab_project_code IS
    'PixLab project scope for workbench sessions; NULL for ordinary WeKnora sessions';
