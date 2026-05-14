-- 000002_advanced_orchestration.up.sql
-- Refactor audit_logs to use partitioning
DROP TABLE IF EXISTS audit_logs;

CREATE TABLE audit_logs (
    id SERIAL,
    node_id INT REFERENCES nodes(id) ON DELETE CASCADE,
    action VARCHAR(100) NOT NULL,
    status VARCHAR(50) NOT NULL,
    output TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- Create initial partition for May 2026
CREATE TABLE audit_logs_y2026m05 PARTITION OF audit_logs
    FOR VALUES FROM ('2026-05-01') TO ('2026-06-01');

-- Orchestration Enhancements
ALTER TABLE groups ADD COLUMN IF NOT EXISTS canary_percentage INT DEFAULT 0;
ALTER TABLE groups ADD COLUMN IF NOT EXISTS is_frozen BOOLEAN DEFAULT FALSE;
ALTER TABLE groups ADD COLUMN IF NOT EXISTS maintenance_window_start TIME;
ALTER TABLE groups ADD COLUMN IF NOT EXISTS maintenance_window_end TIME;

ALTER TABLE nodes ADD COLUMN IF NOT EXISTS uuid UUID UNIQUE;
