DROP TABLE IF EXISTS settings;
ALTER TABLE nodes
    DROP COLUMN IF EXISTS boot_id,
    DROP COLUMN IF EXISTS agent_version,
    DROP COLUMN IF EXISTS pending_updates,
    DROP COLUMN IF EXISTS last_check_at,
    DROP COLUMN IF EXISTS last_patch_at,
    DROP COLUMN IF EXISTS auto_reboot,
    DROP COLUMN IF EXISTS excluded,
    DROP COLUMN IF EXISTS slot;
DROP TABLE IF EXISTS jobs;
