DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM audit_logs
        WHERE length(actor_user_id) > 36 OR length(target_user_id) > 36
    ) THEN
        RAISE EXCEPTION
            '[Migration 000116 down] BLOCKED: audit_logs contains external principal ids longer than 36 characters';
    END IF;
END $$;

ALTER TABLE audit_logs
    ALTER COLUMN actor_user_id TYPE VARCHAR(36),
    ALTER COLUMN target_user_id TYPE VARCHAR(36);
