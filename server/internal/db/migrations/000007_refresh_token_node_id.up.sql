-- Agent node IDs are UUIDs (36 characters); match nodes.mac_address.
ALTER TABLE refresh_tokens ALTER COLUMN mac_address TYPE VARCHAR(50);
