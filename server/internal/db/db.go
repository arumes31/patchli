package db

import (
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"sync"
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
	defer func() { _ = db.Close() }()

	driver, err := pgx.WithInstance(db, &pgx.Config{})
	if err != nil {
		return err
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "pgx", driver)
	if err != nil {
		return err
	}
	defer func() { _, _ = m.Close() }()

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

var (
	// ErrAlreadyEnrolled is returned when a node identity is still claimed by
	// an earlier enrollment.
	ErrAlreadyEnrolled = errors.New("node is already enrolled")
	// ErrNonceUsed is returned when a registration nonce was already redeemed.
	ErrNonceUsed = errors.New("registration nonce already used")
)

// execer is satisfied by both *sql.DB and *sql.Tx.
type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// lockNode serializes enrollment, rotation and revocation for one node for
// the rest of tx. The advisory lock does not depend on any row existing, so
// it also orders requests that race to create the first rows of a node.
func lockNode(tx *sql.Tx, mac string) error {
	_, err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", "patchli-node:"+mac)
	return err
}

// withNodeLock runs fn in a transaction holding the node's advisory lock.
func withNodeLock(mac string, fn func(tx *sql.Tx) error) (err error) {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if err = lockNode(tx, mac); err != nil {
		return err
	}
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

var (
	memEnrollmentsMu sync.Mutex
	memEnrollments   = map[string]time.Time{}
)

// EnrollNode claims mac for a new enrollment, redeems the registration nonce
// and stores the node's first refresh token as one atomic step. The claim is
// a stable ownership record that outlives refresh token rotation: it is only
// released when it expires or an operator revokes it. Without a database the
// claims are tracked in memory.
func EnrollNode(mac, nonce string, nonceExpiresAt time.Time, refreshToken string, expiresAt time.Time) error {
	if DB == nil {
		memEnrollmentsMu.Lock()
		defer memEnrollmentsMu.Unlock()
		now := time.Now()
		for k, exp := range memEnrollments {
			if !now.Before(exp) {
				delete(memEnrollments, k)
			}
		}
		if _, ok := memEnrollments[mac]; ok {
			return ErrAlreadyEnrolled
		}
		fresh, err := ConsumeRegistrationNonce(nonce, nonceExpiresAt)
		if err != nil {
			return err
		}
		if !fresh {
			return ErrNonceUsed
		}
		memEnrollments[mac] = expiresAt
		return nil
	}
	return withNodeLock(mac, func(tx *sql.Tx) error {
		var enrolled bool
		if err := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM node_enrollments WHERE mac_address = $1 AND expires_at > CURRENT_TIMESTAMP)", mac).Scan(&enrolled); err != nil {
			return err
		}
		if enrolled {
			return ErrAlreadyEnrolled
		}
		fresh, err := consumeNonce(tx, nonce, nonceExpiresAt)
		if err != nil {
			return err
		}
		if !fresh {
			return ErrNonceUsed
		}
		if _, err := tx.Exec(`INSERT INTO node_enrollments (mac_address, enrolled_at, expires_at) VALUES ($1, CURRENT_TIMESTAMP, $2)
			ON CONFLICT (mac_address) DO UPDATE SET enrolled_at = EXCLUDED.enrolled_at, expires_at = EXCLUDED.expires_at`, mac, expiresAt); err != nil {
			return err
		}
		_, err = tx.Exec("INSERT INTO refresh_tokens (mac_address, token_hash, expires_at) VALUES ($1, $2, $3)", mac, hashToken(refreshToken), expiresAt)
		return err
	})
}

// RotateRefreshToken redeems oldToken and stores newToken in its place. It
// reports false if oldToken was not stored: a correctly signed token that is
// no longer stored has already been rotated, so every refresh token of the
// node is revoked. Rotation and revocation hold the node lock, so a
// replacement issued by a concurrent request cannot survive the revocation.
func RotateRefreshToken(mac, oldToken, newToken string, expiresAt time.Time) (bool, error) {
	if DB == nil {
		return false, errors.New("database not initialized")
	}
	var rotated bool
	err := withNodeLock(mac, func(tx *sql.Tx) error {
		res, err := tx.Exec("DELETE FROM refresh_tokens WHERE mac_address = $1 AND token_hash = $2 AND expires_at > CURRENT_TIMESTAMP", mac, hashToken(oldToken))
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			_, err := tx.Exec("DELETE FROM refresh_tokens WHERE mac_address = $1", mac)
			return err
		}
		if _, err := tx.Exec("INSERT INTO refresh_tokens (mac_address, token_hash, expires_at) VALUES ($1, $2, $3)", mac, hashToken(newToken), expiresAt); err != nil {
			return err
		}
		// Keep the enrollment claimed for as long as the node holds a token.
		if _, err := tx.Exec(`INSERT INTO node_enrollments (mac_address, expires_at) VALUES ($1, $2)
			ON CONFLICT (mac_address) DO UPDATE SET expires_at = GREATEST(node_enrollments.expires_at, EXCLUDED.expires_at)`, mac, expiresAt); err != nil {
			return err
		}
		rotated = true
		return nil
	})
	return rotated, err
}

// RevokeEnrollment releases a node's enrollment claim and revokes all of its
// refresh tokens, allowing the node to enroll again with a new grant.
func RevokeEnrollment(mac string) error {
	if DB == nil {
		memEnrollmentsMu.Lock()
		defer memEnrollmentsMu.Unlock()
		delete(memEnrollments, mac)
		return nil
	}
	return withNodeLock(mac, func(tx *sql.Tx) error {
		if _, err := tx.Exec("DELETE FROM refresh_tokens WHERE mac_address = $1", mac); err != nil {
			return err
		}
		_, err := tx.Exec("DELETE FROM node_enrollments WHERE mac_address = $1", mac)
		return err
	})
}

var (
	memNoncesMu sync.Mutex
	memNonces   = map[string]time.Time{}
)

// ConsumeRegistrationNonce marks a registration nonce as used. It returns
// false if the nonce was already redeemed. Without a database the nonces are
// tracked in memory.
func ConsumeRegistrationNonce(nonce string, expiresAt time.Time) (bool, error) {
	if DB == nil {
		hash := hashToken(nonce)
		memNoncesMu.Lock()
		defer memNoncesMu.Unlock()
		now := time.Now()
		for k, exp := range memNonces {
			if exp.Before(now) {
				delete(memNonces, k)
			}
		}
		if _, used := memNonces[hash]; used {
			return false, nil
		}
		memNonces[hash] = expiresAt
		return true, nil
	}
	return consumeNonce(DB, nonce, expiresAt)
}

func consumeNonce(ex execer, nonce string, expiresAt time.Time) (bool, error) {
	if _, err := ex.Exec("DELETE FROM registration_nonces WHERE expires_at < CURRENT_TIMESTAMP"); err != nil {
		return false, err
	}
	res, err := ex.Exec("INSERT INTO registration_nonces (nonce_hash, expires_at) VALUES ($1, $2) ON CONFLICT (nonce_hash) DO NOTHING", hashToken(nonce), expiresAt)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}
