DROP INDEX IF EXISTS idx_sessions_pixlab_scope;
ALTER TABLE sessions DROP COLUMN pixlab_project_code;
DROP INDEX IF EXISTS idx_pixlab_project_bindings_status;
DROP INDEX IF EXISTS idx_pixlab_project_bindings_tenant;
DROP TABLE IF EXISTS pixlab_project_bindings;
