DELETE FROM refresh_tokens WHERE length(mac_address) > 17;
ALTER TABLE refresh_tokens ALTER COLUMN mac_address TYPE VARCHAR(17);
