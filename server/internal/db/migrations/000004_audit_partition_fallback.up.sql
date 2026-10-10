-- Also repair databases where migration 2 was applied before this fix.
-- Never create partitions in a row trigger: routing has already happened.
DROP TRIGGER IF EXISTS audit_logs_partition_trigger ON audit_logs;
DROP FUNCTION IF EXISTS create_audit_logs_partition();
CREATE TABLE IF NOT EXISTS audit_logs_default PARTITION OF audit_logs DEFAULT;
