package main

import (
	"context"
	"crypto/tls"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/arumes31/patchli/server/internal/api"
	"github.com/arumes31/patchli/server/internal/auth"
	"github.com/arumes31/patchli/server/internal/cmdsign"
	"github.com/arumes31/patchli/server/internal/db"
	"github.com/arumes31/patchli/server/internal/orchestration"
	"github.com/arumes31/patchli/server/internal/websocket"
	"github.com/arumes31/patchli/server/static"
)

func main() {
	log.Println("Starting Patchli Server (Control Plane)...")
	auth.InitSecrets()
	websocket.InitSecrets()
	if err := cmdsign.Init(); err != nil {
		log.Fatalf("Invalid command signing key: %v", err)
	}
	if cmdsign.Enabled() {
		log.Println("Agent commands will be signed (COMMAND_SIGNING_KEY).")
	} else {
		log.Println("WARNING: COMMAND_SIGNING_KEY is not set; agent commands are sent unsigned.")
	}

	// Initialize Worker Pool
	pool := orchestration.NewWorkerPool(10)
	pool.Start()
	log.Println("Worker pool started.")

	dbURL := os.Getenv("DB_URL")
	if dbURL != "" {
		log.Printf("Connecting to DB...")
		if err := db.Init(dbURL); err != nil {
			log.Fatalf("DB Init failed: %v", err)
		}
	}

	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/health", handleHealth)

	// Use embedded FS for static files
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(static.FS))))

	// Dashboard UI
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		data, err := static.FS.ReadFile("index.html")
		if err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write(data)
	})
	mux.HandleFunc("/setup", api.ServeSetupUI)

	// API v1
	mux.HandleFunc("/api/v1/nodes", api.AuthMiddleware(api.HandleNodes))
	mux.HandleFunc("/api/v1/stats", api.AuthMiddleware(api.HandleStats))
	mux.HandleFunc("/api/v1/setup", api.AuthMiddleware(api.HandleSetup))
	mux.HandleFunc("/api/v1/nodes/revoke", api.AuthMiddleware(api.HandleRevokeEnrollment))

	// Agent Auth (rate limited per client IP)
	authLimiter := api.NewRateLimiter(30, time.Minute)
	mux.HandleFunc("/api/v1/auth/login", authLimiter.Middleware(api.HandleAgentLogin))
	mux.HandleFunc("/api/v1/auth/refresh", authLimiter.Middleware(api.HandleAgentRefresh))

	// WebSocket
	mux.HandleFunc("/ws", websocket.HandleWebSocket)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	if os.Getenv("BASE_URL") == "" {
		log.Println("WARNING: BASE_URL is not set; setup URLs are derived from the request Host header. Set BASE_URL in production.")
	}

	// WebSocket connections are hijacked, which clears these deadlines; the
	// WebSocket handler manages its own read deadline.
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           api.SecurityHeaders(mux),
		ReadHeaderTimeout: 15 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	// Agents require a TLS-authenticated controller. Terminate TLS here or in
	// a reverse proxy in front of the server.
	certFile, keyFile := os.Getenv("TLS_CERT_FILE"), os.Getenv("TLS_KEY_FILE")
	if (certFile == "") != (keyFile == "") {
		log.Fatal("TLS_CERT_FILE and TLS_KEY_FILE must be set together")
	}
	if certFile != "" {
		srv.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	go func() {
		log.Printf("Server listening on port %q (TLS: %t)", port, certFile != "") // #nosec G706 -- %q escapes control characters.
		var err error
		if certFile != "" {
			err = srv.ListenAndServeTLS(certFile, keyFile)
		} else {
			err = srv.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down server gracefully...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool.Stop()
	pool.Wait()
	if db.DB != nil {
		_ = db.DB.Close()
	}

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Server stopped")
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	if db.DB != nil {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if err := db.DB.PingContext(ctx); err != nil {
			http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}
