//go:build integration

package db

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migrationpgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

func migrationDatabase(t *testing.T) (*sql.DB, *migrate.Migrate) {
	t.Helper()
	raw := os.Getenv("PATCHLI_MIGRATION_TEST_URL")
	if raw == "" {
		t.Fatal("PATCHLI_MIGRATION_TEST_URL must identify a disposable patchli test database")
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" && u.Hostname() != "::1" {
		t.Fatal("migration tests require a local disposable PostgreSQL instance")
	}
	if !strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "patchli") {
		t.Fatal("migration tests require a dedicated patchli database")
	}
	admin, err := sql.Open("pgx", raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	schema := fmt.Sprintf("migration_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
			t.Error(err)
		}
	})
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	conn, err := sql.Open("pgx", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	migrationConn, err := sql.Open("pgx", u.String())
	if err != nil {
		t.Fatal(err)
	}
	driver, err := migrationpgx.WithInstance(migrationConn, &migrationpgx.Config{})
	if err != nil {
		_ = migrationConn.Close()
		t.Fatal(err)
	}
	source, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	m, err := migrate.NewWithInstance("iofs", source, "pgx", driver)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = m.Close() })
	return conn, m
}

func TestMigrationsFreshAndPopulatedUpgrade(t *testing.T) {
	for _, populated := range []bool{false, true} {
		t.Run(fmt.Sprintf("populated_%t", populated), func(t *testing.T) {
			conn, m := migrationDatabase(t)
			if err := m.Steps(1); err != nil {
				t.Fatal(err)
			}
			if populated {
				_, err := conn.Exec(`INSERT INTO nodes (id,hostname,mac_address) VALUES (7,'test-node','00:11:22:33:44:55');
INSERT INTO audit_logs (id,node_id,action,status,output,created_at) VALUES
(41,7,'check_updates','success','historic output','2020-01-02T03:04:05Z'),
(42,7,'apply_updates','failed','second output','2021-12-31T23:59:59Z')`)
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := m.Up(); err != nil {
				t.Fatal(err)
			}
			if err := m.Up(); err != migrate.ErrNoChange {
				t.Fatalf("repeat migration: %v", err)
			}
			var id int
			if err := conn.QueryRow(`INSERT INTO audit_logs(action,status,created_at) VALUES ('check_updates','success','2099-05-01') RETURNING id`).Scan(&id); err != nil {
				t.Fatal(err)
			}
			expected := 1
			if populated {
				expected = 43
			}
			if id != expected {
				t.Fatalf("next audit ID %d, want %d", id, expected)
			}
			if populated {
				var count int
				if err := conn.QueryRow(`SELECT count(*) FROM audit_logs WHERE (id=41 AND output='historic output' AND created_at='2020-01-02T03:04:05Z') OR (id=42 AND output='second output' AND created_at='2021-12-31T23:59:59Z')`).Scan(&count); err != nil || count != 2 {
					t.Fatalf("history preserved count=%d err=%v", count, err)
				}
				var uuid string
				if err := conn.QueryRow(`SELECT uuid FROM nodes WHERE id=7`).Scan(&uuid); err != nil || uuid == "" {
					t.Fatalf("node UUID %q err=%v", uuid, err)
				}
			}
			if err := m.Steps(-3); err != nil {
				t.Fatal(err)
			}
			var retained int
			wantRetained := 1
			if populated {
				wantRetained = 3
			}
			if err := conn.QueryRow(`SELECT count(*) FROM audit_logs`).Scan(&retained); err != nil || retained != wantRetained {
				t.Fatalf("rollback retained %d rows, want %d; err=%v", retained, wantRetained, err)
			}
			if err := conn.QueryRow(`INSERT INTO audit_logs(action,status) VALUES ('rollback','success') RETURNING id`).Scan(&id); err != nil {
				t.Fatal(err)
			}
			if id != expected+1 {
				t.Fatalf("rollback next ID %d, want %d", id, expected+1)
			}
			if err := m.Up(); err != nil {
				t.Fatal(err)
			}
			if populated {
				if _, err := conn.Exec(`DELETE FROM nodes WHERE id=7`); err != nil {
					t.Fatal(err)
				}
				var count int
				if err := conn.QueryRow(`SELECT count(*) FROM audit_logs WHERE node_id=7`).Scan(&count); err != nil || count != 0 {
					t.Fatalf("node delete cascade count=%d err=%v", count, err)
				}
			}
		})
	}
}

func TestMigrationsRepairPreviouslyAppliedPartitioning(t *testing.T) {
	conn, m := migrationDatabase(t)
	if err := m.Migrate(3); err != nil {
		t.Fatal(err)
	}
	// Model a previously applied version with a monthly partition and the old
	// row trigger, but no catch-all partition for the next month.
	_, err := conn.Exec(`DROP TABLE audit_logs_default;
CREATE TABLE audit_logs_y2020_01 PARTITION OF audit_logs FOR VALUES FROM ('2020-01-01') TO ('2020-02-01');
INSERT INTO audit_logs(id,action,status,output,created_at) VALUES (100,'check_updates','success','existing partition','2020-01-15');
CREATE FUNCTION create_audit_logs_partition() RETURNS TRIGGER AS $$ BEGIN RAISE EXCEPTION 'legacy partition trigger must not run'; END; $$ LANGUAGE plpgsql;
CREATE TRIGGER audit_logs_partition_trigger BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION create_audit_logs_partition();`)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Up(); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO audit_logs(action,status,created_at) VALUES ('check_updates','success','2099-07-01'),('check_updates','success','2020-01-16')`); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := conn.QueryRow(`SELECT count(*) FROM audit_logs WHERE id=100 AND output='existing partition'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("existing partition data count=%d err=%v", count, err)
	}
	if err := m.Steps(-1); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(`SELECT count(*) FROM audit_logs`).Scan(&count); err != nil || count != 3 {
		t.Fatalf("forward-fix rollback lost history count=%d err=%v", count, err)
	}
}

func TestMigrationsIncompatibleHistoricalRowFailsWithoutDataLoss(t *testing.T) {
	conn, m := migrationDatabase(t)
	if err := m.Steps(1); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO audit_logs(id,action,status,output,created_at) VALUES (50,'check_updates','success','unknown timestamp',NULL)`); err != nil {
		t.Fatal(err)
	}
	if err := m.Up(); err == nil {
		t.Fatal("migration accepted NULL partition key")
	}
	// The driver executes each SQL file atomically. A rejected historical row
	// leaves version 1 data intact, while migrate marks version 2 dirty.
	var count int
	if err := conn.QueryRow(`SELECT count(*) FROM audit_logs WHERE id=50 AND output='unknown timestamp' AND created_at IS NULL`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("failed migration lost original row count=%d err=%v", count, err)
	}
	var kind string
	if err := conn.QueryRow(`SELECT relkind::text FROM pg_class WHERE oid='audit_logs'::regclass`).Scan(&kind); err != nil || kind != "r" {
		t.Fatalf("original table changed kind=%s err=%v", kind, err)
	}
	version, dirty, err := m.Version()
	if err != nil || version != 2 || !dirty {
		t.Fatalf("version=%d dirty=%t err=%v", version, dirty, err)
	}
}
