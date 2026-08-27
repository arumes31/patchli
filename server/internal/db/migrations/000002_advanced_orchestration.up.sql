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

CREATE OR REPLACE FUNCTION create_audit_logs_partition()
RETURNS TRIGGER AS $$
DECLARE
    partition_date TEXT;
    partition_name TEXT;
    start_date DATE;
    end_date DATE;
BEGIN
    partition_date := to_char(NEW.created_at, 'YYYY_MM');
    partition_name := 'audit_logs_y' || partition_date;
    start_date := date_trunc('month', NEW.created_at);
    end_date := start_date + interval '1 month';

    IF NOT EXISTS (SELECT 1 FROM pg_class WHERE relname = partition_name) THEN
        EXECUTE format(
            'CREATE TABLE IF NOT EXISTS %I PARTITION OF audit_logs FOR VALUES FROM (%L) TO (%L)',
            partition_name, start_date, end_date
        );
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER audit_logs_partition_trigger
BEFORE INSERT ON audit_logs
FOR EACH ROW EXECUTE FUNCTION create_audit_logs_partition();

INSERT INTO audit_logs (id, job_id, node_id, action, status, output, created_at)
SELECT id, job_id, node_id, action, status, output, created_at FROM audit_logs_old_backup;

DROP TABLE audit_logs_old_backup;

SELECT setval(pg_get_serial_sequence('audit_logs', 'id'), COALESCE((SELECT MAX(id)+1 FROM audit_logs), 1), false);

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
