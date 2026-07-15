CREATE TABLE IF NOT EXISTS refresh_tokens (
    id SERIAL PRIMARY KEY,
    mac_address VARCHAR(17) REFERENCES nodes(mac_address) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_refresh_tokens_mac_address ON refresh_tokens(mac_address);
CREATE INDEX idx_refresh_tokens_token_hash ON refresh_tokens(token_hash);
