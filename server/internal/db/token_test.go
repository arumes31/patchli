package db

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRefreshTokenPersistence(t *testing.T) {
	connection, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	previous := DB
	DB = connection
	t.Cleanup(func() { DB = previous; _ = connection.Close() })
	token := "opaque-refresh-credential"
	sum := sha256.Sum256([]byte(token))
	hash := hex.EncodeToString(sum[:])
	expires := time.Now().Add(time.Hour)
	mock.ExpectExec("INSERT INTO refresh_tokens").WithArgs("node", hash, expires).WillReturnResult(sqlmock.NewResult(1, 1))
	if err := StoreRefreshToken("node", token, expires); err != nil {
		t.Fatal(err)
	}
	for _, valid := range []bool{true, false} {
		mock.ExpectQuery("SELECT EXISTS").WithArgs("node", hash).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(valid))
		got, err := VerifyRefreshToken("node", token)
		if err != nil || got != valid {
			t.Fatalf("VerifyRefreshToken = %v, %v; want %v", got, err, valid)
		}
	}
	mock.ExpectExec("DELETE FROM refresh_tokens WHERE mac_address = \\$1 AND token_hash = \\$2").WithArgs("node", hash).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := DeleteRefreshToken("node", token); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec("DELETE FROM refresh_tokens WHERE mac_address = \\$1").WithArgs("node").WillReturnResult(sqlmock.NewResult(0, 2))
	if err := DeleteAllRefreshTokens("node"); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec("INSERT INTO refresh_tokens").WillReturnError(sql.ErrConnDone)
	if err := StoreRefreshToken("node", token, expires); !errors.Is(err, sql.ErrConnDone) {
		t.Fatalf("lost storage error: %v", err)
	}
	mock.ExpectQuery("SELECT EXISTS").WillReturnError(sql.ErrConnDone)
	if _, err := VerifyRefreshToken("node", token); !errors.Is(err, sql.ErrConnDone) {
		t.Fatalf("lost verification error: %v", err)
	}
	mock.ExpectExec("DELETE FROM refresh_tokens").WillReturnError(sql.ErrConnDone)
	if err := DeleteRefreshToken("node", token); !errors.Is(err, sql.ErrConnDone) {
		t.Fatalf("lost deletion error: %v", err)
	}
	mock.ExpectExec("DELETE FROM refresh_tokens").WillReturnError(sql.ErrConnDone)
	if err := DeleteAllRefreshTokens("node"); !errors.Is(err, sql.ErrConnDone) {
		t.Fatalf("lost revocation error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRefreshTokenOperationsWithoutDatabase(t *testing.T) {
	previous := DB
	DB = nil
	t.Cleanup(func() { DB = previous })
	if err := StoreRefreshToken("node", "token", time.Now()); err != nil {
		t.Fatalf("optional store: %v", err)
	}
	if valid, err := VerifyRefreshToken("node", "token"); valid || err == nil {
		t.Fatal("verification must fail closed")
	}
	if err := DeleteRefreshToken("node", "token"); err == nil {
		t.Fatal("expected unavailable database")
	}
	if err := DeleteAllRefreshTokens("node"); err == nil {
		t.Fatal("expected unavailable database")
	}
}

func TestInvalidDatabaseConfiguration(t *testing.T) {
	previous := DB
	t.Cleanup(func() { DB = previous })
	if err := Init("postgres://invalid:%zz"); err == nil {
		t.Fatal("invalid DSN accepted")
	}
	if err := runMigrations("postgres://invalid:%zz"); err == nil {
		t.Fatal("invalid migration DSN accepted")
	}
}

func tokenDatabase(t *testing.T) sqlmock.Sqlmock {
	t.Helper()
	connection, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	previous := DB
	DB = connection
	t.Cleanup(func() {
		DB = previous
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		_ = connection.Close()
	})
	return mock
}

func digest(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func expectNodeLock(mock sqlmock.Sqlmock, mac string) {
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs("patchli-node:" + mac).WillReturnResult(sqlmock.NewResult(0, 0))
}

func TestEnrollNode(t *testing.T) {
	expires := time.Now().Add(time.Hour)
	nonceExpires := time.Now().Add(time.Minute)
	t.Run("claims node", func(t *testing.T) {
		mock := tokenDatabase(t)
		expectNodeLock(mock, "node")
		mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM node_enrollments`).WithArgs("node").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectExec("DELETE FROM registration_nonces").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("INSERT INTO registration_nonces").WithArgs(digest("nonce"), nonceExpires).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec("INSERT INTO node_enrollments").WithArgs("node", expires).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec("INSERT INTO refresh_tokens").WithArgs("node", digest("token"), expires).WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()
		if err := EnrollNode("node", "nonce", nonceExpires, "token", expires); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("already enrolled keeps nonce", func(t *testing.T) {
		mock := tokenDatabase(t)
		expectNodeLock(mock, "node")
		mock.ExpectQuery("SELECT EXISTS").WithArgs("node").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectRollback()
		if err := EnrollNode("node", "nonce", nonceExpires, "token", expires); !errors.Is(err, ErrAlreadyEnrolled) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("nonce reused", func(t *testing.T) {
		mock := tokenDatabase(t)
		expectNodeLock(mock, "node")
		mock.ExpectQuery("SELECT EXISTS").WithArgs("node").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectExec("DELETE FROM registration_nonces").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("INSERT INTO registration_nonces").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectRollback()
		if err := EnrollNode("node", "nonce", nonceExpires, "token", expires); !errors.Is(err, ErrNonceUsed) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("storage failure rolls back", func(t *testing.T) {
		mock := tokenDatabase(t)
		expectNodeLock(mock, "node")
		mock.ExpectQuery("SELECT EXISTS").WithArgs("node").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectExec("DELETE FROM registration_nonces").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("INSERT INTO registration_nonces").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec("INSERT INTO node_enrollments").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec("INSERT INTO refresh_tokens").WillReturnError(sql.ErrConnDone)
		mock.ExpectRollback()
		if err := EnrollNode("node", "nonce", nonceExpires, "token", expires); !errors.Is(err, sql.ErrConnDone) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("lock failure", func(t *testing.T) {
		mock := tokenDatabase(t)
		mock.ExpectBegin()
		mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnError(sql.ErrConnDone)
		mock.ExpectRollback()
		if err := EnrollNode("node", "nonce", nonceExpires, "token", expires); !errors.Is(err, sql.ErrConnDone) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestEnrollNodeWithoutDatabase(t *testing.T) {
	previous := DB
	DB = nil
	t.Cleanup(func() { DB = previous })
	expires := time.Now().Add(time.Hour)
	nonceExpires := time.Now().Add(time.Minute)
	if err := EnrollNode("memory-node", "memory-enroll-1", nonceExpires, "token", expires); err != nil {
		t.Fatal(err)
	}
	// The claim survives without any stored refresh token.
	if err := EnrollNode("memory-node", "memory-enroll-2", nonceExpires, "token", expires); !errors.Is(err, ErrAlreadyEnrolled) {
		t.Fatalf("takeover accepted: %v", err)
	}
	if err := EnrollNode("memory-other", "memory-enroll-1", nonceExpires, "token", expires); !errors.Is(err, ErrNonceUsed) {
		t.Fatalf("nonce replay accepted: %v", err)
	}
	if err := RevokeEnrollment("memory-node"); err != nil {
		t.Fatal(err)
	}
	if err := EnrollNode("memory-node", "memory-enroll-3", nonceExpires, "token", expires); err != nil {
		t.Fatalf("re-enrollment after revocation: %v", err)
	}
	if _, err := RotateRefreshToken("memory-node", "old", "new", expires); err == nil {
		t.Fatal("rotation must fail closed without a database")
	}
}

func TestRotateRefreshToken(t *testing.T) {
	expires := time.Now().Add(time.Hour)
	t.Run("rotates and extends enrollment", func(t *testing.T) {
		mock := tokenDatabase(t)
		expectNodeLock(mock, "node")
		mock.ExpectExec(`DELETE FROM refresh_tokens WHERE mac_address = \$1 AND token_hash = \$2 AND expires_at`).WithArgs("node", digest("old")).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec("INSERT INTO refresh_tokens").WithArgs("node", digest("new"), expires).WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectExec("INSERT INTO node_enrollments").WithArgs("node", expires).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		if ok, err := RotateRefreshToken("node", "old", "new", expires); err != nil || !ok {
			t.Fatalf("RotateRefreshToken = %v, %v", ok, err)
		}
	})
	t.Run("reuse revokes node", func(t *testing.T) {
		mock := tokenDatabase(t)
		expectNodeLock(mock, "node")
		mock.ExpectExec(`DELETE FROM refresh_tokens WHERE mac_address = \$1 AND token_hash`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`DELETE FROM refresh_tokens WHERE mac_address = \$1$`).WithArgs("node").WillReturnResult(sqlmock.NewResult(0, 2))
		mock.ExpectCommit()
		if ok, err := RotateRefreshToken("node", "old", "new", expires); err != nil || ok {
			t.Fatalf("RotateRefreshToken = %v, %v", ok, err)
		}
	})
	t.Run("storage failure rolls back", func(t *testing.T) {
		mock := tokenDatabase(t)
		expectNodeLock(mock, "node")
		mock.ExpectExec("DELETE FROM refresh_tokens").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec("INSERT INTO refresh_tokens").WillReturnError(sql.ErrConnDone)
		mock.ExpectRollback()
		if ok, err := RotateRefreshToken("node", "old", "new", expires); !errors.Is(err, sql.ErrConnDone) || ok {
			t.Fatalf("RotateRefreshToken = %v, %v", ok, err)
		}
	})
}

func TestRevokeEnrollment(t *testing.T) {
	mock := tokenDatabase(t)
	expectNodeLock(mock, "node")
	mock.ExpectExec("DELETE FROM refresh_tokens").WithArgs("node").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM node_enrollments").WithArgs("node").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := RevokeEnrollment("node"); err != nil {
		t.Fatal(err)
	}
}

func TestConsumeRegistrationNonce(t *testing.T) {
	previous := DB
	DB = nil
	t.Cleanup(func() { DB = previous })
	expires := time.Now().Add(time.Minute)
	if ok, err := ConsumeRegistrationNonce("memory-nonce", expires); err != nil || !ok {
		t.Fatalf("first use rejected: %v %v", ok, err)
	}
	if ok, err := ConsumeRegistrationNonce("memory-nonce", expires); err != nil || ok {
		t.Fatalf("replay accepted: %v %v", ok, err)
	}

	connection, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	DB = connection
	t.Cleanup(func() { _ = connection.Close() })
	sum := sha256.Sum256([]byte("db-nonce"))
	hash := hex.EncodeToString(sum[:])
	for _, rows := range []int64{1, 0} {
		mock.ExpectExec("DELETE FROM registration_nonces").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("INSERT INTO registration_nonces").WithArgs(hash, expires).WillReturnResult(sqlmock.NewResult(0, rows))
		ok, err := ConsumeRegistrationNonce("db-nonce", expires)
		if err != nil || ok != (rows == 1) {
			t.Fatalf("ConsumeRegistrationNonce = %v, %v; rows %d", ok, err, rows)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
