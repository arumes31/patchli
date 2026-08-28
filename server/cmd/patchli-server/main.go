package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/arumes31/patchli/server/internal/api"
	"github.com/arumes31/patchli/server/internal/auth"
	"github.com/arumes31/patchli/server/internal/db"
	"github.com/arumes31/patchli/server/internal/orchestration"
	"github.com/arumes31/patchli/server/internal/websocket"
	"github.com/arumes31/patchli/server/static"
)

type serverConfig struct {
	port     string
	certFile string
	keyFile  string
	dbURL    string
}

func loadConfig(getenv func(string) string) (serverConfig, error) {
	config := serverConfig{
		port:     strings.TrimSpace(getenv("PORT")),
		certFile: strings.TrimSpace(getenv("TLS_CERT_FILE")),
		keyFile:  strings.TrimSpace(getenv("TLS_KEY_FILE")),
		dbURL:    strings.TrimSpace(getenv("DB_URL")),
	}
	if config.port == "" {
		config.port = "8443"
	}
	if config.certFile == "" || config.keyFile == "" || config.certFile == config.keyFile {
		return serverConfig{}, errors.New("TLS_CERT_FILE and TLS_KEY_FILE must reference distinct certificate and key files")
	}
	for name, path := range map[string]string{"TLS_CERT_FILE": config.certFile, "TLS_KEY_FILE": config.keyFile} {
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return serverConfig{}, fmt.Errorf("%s must reference a readable regular file", name)
		}
	}
	publicURL, err := url.Parse(strings.TrimSpace(getenv("BASE_URL")))
	if err != nil || publicURL.Scheme != "https" || publicURL.Host == "" || publicURL.User != nil || publicURL.RawQuery != "" || publicURL.Fragment != "" {
		return serverConfig{}, errors.New("BASE_URL must be an https origin")
	}
	if len(getenv("ADMIN_TOKEN")) < 32 {
		return serverConfig{}, errors.New("ADMIN_TOKEN must contain at least 32 bytes")
	}
	if config.dbURL == "" {
		return serverConfig{}, errors.New("DB_URL is required")
	}
	return config, nil
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func main() {
	log.Println("Starting Patchli Server (Control Plane)...")
	config, err := loadConfig(os.Getenv)
	if err != nil {
		log.Fatal(err)
	}
	if err := auth.InitSecrets(); err != nil {
		log.Fatal(err)
	}

	// Initialize Worker Pool
	pool := orchestration.NewWorkerPool(10)
	if err := pool.Start(); err != nil {
		log.Fatal(err)
	}
	log.Println("Worker pool started.")

	log.Printf("Connecting to DB...")
	if err := db.Init(config.dbURL); err != nil {
		log.Fatalf("DB Init failed: %v", err)
	}

	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

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

	// Agent Auth
	mux.HandleFunc("/api/v1/auth/login", api.HandleAgentLogin)
	mux.HandleFunc("/api/v1/auth/refresh", api.HandleAgentRefresh)

	// WebSocket
	mux.HandleFunc("/ws", websocket.HandleWebSocket)

	srv := &http.Server{
		Addr:              ":" + config.port,
		Handler:           securityHeaders(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}

	go func() {
		log.Printf("Server listening with TLS on :%s", config.port)
		if err := srv.ListenAndServeTLS(config.certFile, config.keyFile); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down server gracefully...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("HTTP shutdown: %v", err)
	}
	pool.Stop()
	pool.Wait()
	if db.DB != nil {
		_ = db.DB.Close()
	}
	log.Println("Server stopped")
}
