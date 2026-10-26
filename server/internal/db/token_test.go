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

func TestConsumeRefreshToken(t *testing.T) {
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
	for _, rows := range []int64{1, 0} {
		mock.ExpectExec("DELETE FROM refresh_tokens WHERE mac_address = \\$1 AND token_hash = \\$2 AND expires_at").WithArgs("node", hash).WillReturnResult(sqlmock.NewResult(0, rows))
		got, err := ConsumeRefreshToken("node", token)
		if err != nil || got != (rows == 1) {
			t.Fatalf("ConsumeRefreshToken = %v, %v; rows %d", got, err, rows)
		}
	}
	mock.ExpectExec("DELETE FROM refresh_tokens").WillReturnError(sql.ErrConnDone)
	if _, err := ConsumeRefreshToken("node", token); !errors.Is(err, sql.ErrConnDone) {
		t.Fatalf("lost consume error: %v", err)
	}
	mock.ExpectQuery("SELECT EXISTS").WithArgs("node").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	if active, err := HasActiveRefreshToken("node"); err != nil || !active {
		t.Fatalf("HasActiveRefreshToken = %v, %v", active, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
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
