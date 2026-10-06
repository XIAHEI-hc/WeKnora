-- External principals such as "pixlab:<uuid>" are longer than a bare UUID.
-- Keep audit attribution lossless for workbench and future federated callers.
ALTER TABLE audit_logs
    ALTER COLUMN actor_user_id TYPE VARCHAR(512),
    ALTER COLUMN target_user_id TYPE VARCHAR(512);
