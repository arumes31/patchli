CREATE TABLE IF NOT EXISTS registration_nonces (
    nonce_hash TEXT PRIMARY KEY,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    used_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_registration_nonces_expires_at ON registration_nonces(expires_at);
