-- 000002_advanced_orchestration.up.sql
-- Refactor audit_logs to use partitioning
ALTER TABLE audit_logs RENAME TO audit_logs_old_backup;

CREATE TABLE audit_logs (
    id SERIAL,
    job_id VARCHAR(255),
    node_id INT REFERENCES nodes(id) ON DELETE CASCADE,
    action VARCHAR(100) NOT NULL,
    status VARCHAR(50) NOT NULL,
    output TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- Routing happens before row triggers. A default partition accepts historical
-- rows and future months without attempting DDL during an INSERT.
CREATE TABLE audit_logs_default PARTITION OF audit_logs DEFAULT;

INSERT INTO audit_logs (id, job_id, node_id, action, status, output, created_at)
SELECT id, job_id, node_id, action, status, output, created_at FROM audit_logs_old_backup;

DROP TABLE audit_logs_old_backup;

SELECT setval(pg_get_serial_sequence('audit_logs', 'id'), COALESCE((SELECT MAX(id)::bigint + 1 FROM audit_logs), 1), false);

-- Orchestration Enhancements
ALTER TABLE groups ADD COLUMN IF NOT EXISTS canary_percentage INT DEFAULT 0;
ALTER TABLE groups ADD COLUMN IF NOT EXISTS is_frozen BOOLEAN DEFAULT FALSE;
ALTER TABLE groups ADD COLUMN IF NOT EXISTS maintenance_window_start TIME;
ALTER TABLE groups ADD COLUMN IF NOT EXISTS maintenance_window_end TIME;

CREATE EXTENSION IF NOT EXISTS pgcrypto;
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS uuid UUID UNIQUE;
UPDATE nodes SET uuid = gen_random_uuid() WHERE uuid IS NULL;
ALTER TABLE nodes ALTER COLUMN uuid SET DEFAULT gen_random_uuid();
ALTER TABLE nodes ALTER COLUMN uuid SET NOT NULL;
