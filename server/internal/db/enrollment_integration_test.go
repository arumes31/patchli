//go:build integration

package db

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// The TestMigrations prefix keeps this in the CI integration run.
func TestMigrationsEnrollmentLifecycle(t *testing.T) {
	conn, m := migrationDatabase(t)
	if err := m.Up(); err != nil {
		t.Fatal(err)
	}
	previous := DB
	DB = conn
	t.Cleanup(func() { DB = previous })

	const mac = "00:11:22:33:44:55"
	if _, err := conn.Exec(`INSERT INTO nodes (hostname, mac_address) VALUES ('node', $1)`, mac); err != nil {
		t.Fatal(err)
	}
	expires := time.Now().Add(time.Hour)
	nonceExpires := time.Now().Add(10 * time.Minute)
	countTokens := func() int {
		t.Helper()
		var n int
		if err := conn.QueryRow(`SELECT count(*) FROM refresh_tokens WHERE mac_address = $1`, mac).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// Concurrent enrollments with distinct fresh grants: exactly one claims
	// the node.
	const workers = 8
	var wg sync.WaitGroup
	enrolled := make(chan string, workers)
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token := fmt.Sprintf("token-%d", i)
			switch err := EnrollNode(mac, fmt.Sprintf("race-nonce-%d", i), nonceExpires, token, expires); {
			case err == nil:
				enrolled <- token
			case !errors.Is(err, ErrAlreadyEnrolled):
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	close(enrolled)
	var winners []string
	for token := range enrolled {
		winners = append(winners, token)
	}
	if len(winners) != 1 {
		t.Fatalf("%d concurrent enrollments succeeded, want 1", len(winners))
	}
	if n := countTokens(); n != 1 {
		t.Fatalf("%d refresh tokens after enrollment, want 1", n)
	}
	// Normalize the winner's token for the rest of the test.
	if _, err := conn.Exec(`UPDATE refresh_tokens SET token_hash = $2 WHERE mac_address = $1`, mac, hashToken("token-1")); err != nil {
		t.Fatal(err)
	}
	if err := EnrollNode(mac, "nonce-2", nonceExpires, "intruder", expires); !errors.Is(err, ErrAlreadyEnrolled) {
		t.Fatalf("second enrollment: %v", err)
	}

	// Ownership survives while the node holds no refresh token at all.
	if _, err := conn.Exec(`DELETE FROM refresh_tokens WHERE mac_address = $1`, mac); err != nil {
		t.Fatal(err)
	}
	if err := EnrollNode(mac, "nonce-3", nonceExpires, "intruder", expires); !errors.Is(err, ErrAlreadyEnrolled) {
		t.Fatalf("enrollment without stored tokens: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO refresh_tokens (mac_address, token_hash, expires_at) VALUES ($1, $2, $3)`, mac, hashToken("token-1"), expires); err != nil {
		t.Fatal(err)
	}

	// Concurrent redemptions of one token: exactly one rotates, the others
	// detect reuse, and no replacement survives the revocation.
	results := make(chan bool, workers)
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := RotateRefreshToken(mac, "token-1", fmt.Sprintf("replacement-%d", i), expires)
			if err != nil {
				t.Error(err)
			}
			results <- ok
		}()
	}
	wg.Wait()
	close(results)
	rotated := 0
	for ok := range results {
		if ok {
			rotated++
		}
	}
	if rotated != 1 {
		t.Fatalf("%d concurrent rotations succeeded, want 1", rotated)
	}
	if n := countTokens(); n != 0 {
		t.Fatalf("%d refresh tokens survived reuse revocation", n)
	}
	if err := EnrollNode(mac, "nonce-4", nonceExpires, "intruder", expires); !errors.Is(err, ErrAlreadyEnrolled) {
		t.Fatalf("enrollment after reuse revocation: %v", err)
	}

	if err := RevokeEnrollment(mac); err != nil {
		t.Fatal(err)
	}
	if err := EnrollNode(mac, "nonce-5", nonceExpires, "token-2", expires); err != nil {
		t.Fatalf("re-enrollment after operator revocation: %v", err)
	}
	if ok, err := RotateRefreshToken(mac, "token-2", "token-3", expires); err != nil || !ok {
		t.Fatalf("rotation after re-enrollment = %v, %v", ok, err)
	}
	if n := countTokens(); n != 1 {
		t.Fatalf("%d refresh tokens after rotation, want 1", n)
	}
}
