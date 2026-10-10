-- Preserve audit history when returning to the nonpartitioned schema.
-- Retain additive orchestration columns and node UUIDs: dropping them would
-- discard operator configuration and invalidate existing node identities.
DROP TRIGGER IF EXISTS audit_logs_partition_trigger ON audit_logs;
DROP FUNCTION IF EXISTS create_audit_logs_partition();
ALTER TABLE audit_logs RENAME TO audit_logs_partitioned_backup;
CREATE TABLE audit_logs (
    id SERIAL PRIMARY KEY,
    job_id VARCHAR(255),
    node_id INT REFERENCES nodes(id) ON DELETE CASCADE,
    action VARCHAR(100) NOT NULL,
    status VARCHAR(50) NOT NULL,
    output TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO audit_logs (id, job_id, node_id, action, status, output, created_at)
SELECT id, job_id, node_id, action, status, output, created_at FROM audit_logs_partitioned_backup;
DROP TABLE audit_logs_partitioned_backup;
SELECT setval(pg_get_serial_sequence('audit_logs', 'id'), COALESCE((SELECT MAX(id)::bigint + 1 FROM audit_logs), 1), false);
