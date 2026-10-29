-- Stable record of which node identity has been claimed by an enrollment.
-- Unlike refresh_tokens, the row survives token rotation and reuse
-- revocation, so a fresh registration grant cannot take over an enrolled node.
CREATE TABLE IF NOT EXISTS node_enrollments (
    mac_address VARCHAR(50) PRIMARY KEY,
    enrolled_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL
);

INSERT INTO node_enrollments (mac_address, expires_at)
SELECT mac_address, MAX(expires_at)
FROM refresh_tokens
WHERE mac_address IS NOT NULL AND expires_at > CURRENT_TIMESTAMP
GROUP BY mac_address
ON CONFLICT (mac_address) DO NOTHING;
