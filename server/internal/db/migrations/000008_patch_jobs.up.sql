-- Jobs sent to agents, their results and reboot confirmation.
CREATE TABLE IF NOT EXISTS jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    node_id VARCHAR(50) NOT NULL,
    action VARCHAR(32) NOT NULL,
    state VARCHAR(16) NOT NULL DEFAULT 'sent', -- sent, rebooting, ok, failed
    trigger VARCHAR(16) NOT NULL DEFAULT 'manual', -- manual, schedule
    error TEXT,
    output TEXT,
    reboot_needed BOOLEAN,
    boot_id_before VARCHAR(64),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at TIMESTAMP WITH TIME ZONE
);
CREATE INDEX IF NOT EXISTS idx_jobs_node_created ON jobs(node_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_jobs_open ON jobs(state) WHERE state IN ('sent', 'rebooting');

ALTER TABLE nodes
    ADD COLUMN IF NOT EXISTS boot_id VARCHAR(64),
    ADD COLUMN IF NOT EXISTS agent_version VARCHAR(50),
    ADD COLUMN IF NOT EXISTS pending_updates INT NOT NULL DEFAULT -1,
    ADD COLUMN IF NOT EXISTS last_check_at TIMESTAMP WITH TIME ZONE,
    ADD COLUMN IF NOT EXISTS last_patch_at TIMESTAMP WITH TIME ZONE,
    ADD COLUMN IF NOT EXISTS auto_reboot BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS excluded BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS slot INT NOT NULL DEFAULT 100;
ALTER TABLE nodes ALTER COLUMN os_version TYPE VARCHAR(100);

-- Single-row schedule settings.
CREATE TABLE IF NOT EXISTS settings (
    id INT PRIMARY KEY CHECK (id = 1),
    window_start TIME NOT NULL DEFAULT '03:00',
    window_end TIME NOT NULL DEFAULT '05:30',
    timezone TEXT NOT NULL DEFAULT 'Europe/Vienna',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    last_run_date DATE
);
INSERT INTO settings (id) VALUES (1) ON CONFLICT DO NOTHING;
