CREATE TABLE IF NOT EXISTS pixlab_project_bindings (
    project_code TEXT PRIMARY KEY,
    project_name TEXT NOT NULL DEFAULT '',
    tenant_id INTEGER NOT NULL,
    knowledge_base_id TEXT NOT NULL UNIQUE,
    agent_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'provisioning',
    revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_pixlab_project_bindings_tenant
    ON pixlab_project_bindings (tenant_id);
CREATE INDEX IF NOT EXISTS idx_pixlab_project_bindings_status
    ON pixlab_project_bindings (status);

ALTER TABLE sessions ADD COLUMN pixlab_project_code TEXT DEFAULT NULL;

CREATE INDEX IF NOT EXISTS idx_sessions_pixlab_scope
    ON sessions (tenant_id, pixlab_project_code, user_id, updated_at DESC)
    WHERE pixlab_project_code IS NOT NULL;
