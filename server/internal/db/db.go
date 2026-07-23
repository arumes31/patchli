package db

import (
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

var DB *sql.DB

// Init initializes the database connection and runs migrations.
func Init(dbURL string) error {
	var err error
	DB, err = sql.Open("pgx", dbURL)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	if err := DB.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	log.Println("Database connection established. Running migrations...")
	if err := runMigrations(dbURL); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	log.Println("Migrations completed successfully.")
	return nil
}

func runMigrations(dbURL string) error {
	sourceDriver, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return err
	}

	// Use pgx driver for migrations
	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		return err
	}
	defer db.Close()

	driver, err := pgx.WithInstance(db, &pgx.Config{})
	if err != nil {
		return err
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "pgx", driver)
	if err != nil {
		return err
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}

	return nil
}

// UpdateNodeStatus updates the node's heartbeat and status in the database.
func UpdateNodeStatus(mac string, hostname string, osName string, osVersion string, kernel string, status string) error {
	if DB == nil {
		return errors.New("database not initialized")
	}
	query := `
	INSERT INTO nodes (mac_address, hostname, os_name, os_version, kernel_version, status, last_heartbeat)
	VALUES ($1, $2, $3, $4, $5, $6, CURRENT_TIMESTAMP)
	ON CONFLICT (mac_address) DO UPDATE SET
		hostname = EXCLUDED.hostname,
		os_name = EXCLUDED.os_name,
		os_version = EXCLUDED.os_version,
		kernel_version = EXCLUDED.kernel_version,
		status = EXCLUDED.status,
		last_heartbeat = CURRENT_TIMESTAMP
	`
	_, err := DB.Exec(query, mac, hostname, osName, osVersion, kernel, status)
	return err
}

// IsJobRunning checks if a job is still in 'running' state.
func IsJobRunning(jobID string) (bool, error) {
	if DB == nil {
		return false, errors.New("database not initialized")
	}
	var status string
	err := DB.QueryRow("SELECT status FROM audit_logs WHERE job_id = $1 ORDER BY created_at DESC LIMIT 1", jobID).Scan(&status)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return status == "running", nil
}

func hashToken(token string) string {
	h := sha256.New()
	h.Write([]byte(token))
	return hex.EncodeToString(h.Sum(nil))
}

func StoreRefreshToken(mac, token string, expiresAt time.Time) error {
	if DB == nil {
		return nil
	}
	hash := hashToken(token)
	query := "INSERT INTO refresh_tokens (mac_address, token_hash, expires_at) VALUES ($1, $2, $3)"
	_, err := DB.Exec(query, mac, hash, expiresAt)
	return err
}

func VerifyRefreshToken(mac, token string) (bool, error) {
	if DB == nil {
		return false, errors.New("database not initialized")
	}
	hash := hashToken(token)
	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM refresh_tokens WHERE mac_address = $1 AND token_hash = $2 AND expires_at > CURRENT_TIMESTAMP)"
	err := DB.QueryRow(query, mac, hash).Scan(&exists)
	return exists, err
}

func DeleteRefreshToken(mac, token string) error {
	if DB == nil {
		return errors.New("database not initialized")
	}
	hash := hashToken(token)
	_, err := DB.Exec("DELETE FROM refresh_tokens WHERE mac_address = $1 AND token_hash = $2", mac, hash)
	return err
}

func DeleteAllRefreshTokens(mac string) error {
	if DB == nil {
		return errors.New("database not initialized")
	}
	_, err := DB.Exec("DELETE FROM refresh_tokens WHERE mac_address = $1", mac)
	return err
}
