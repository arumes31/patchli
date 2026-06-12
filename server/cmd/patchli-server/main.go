package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/arumes31/patchli/server/internal/api"
	"github.com/arumes31/patchli/server/internal/db"
	"github.com/arumes31/patchli/server/internal/orchestration"
	"github.com/arumes31/patchli/server/internal/websocket"
	"github.com/arumes31/patchli/server/static"
)

func main() {
	log.Println("Starting Patchli Server (Control Plane)...")

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
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
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
	mux.HandleFunc("/api/v1/nodes", api.HandleNodes)
	mux.HandleFunc("/api/v1/stats", api.HandleStats)
	mux.HandleFunc("/api/v1/setup", api.HandleSetup)

	// WebSocket
	mux.HandleFunc("/ws", websocket.HandleWebSocket)

	port := os.Getenv("PORT")
	if port == "" { port = "8080" }

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 15 * time.Second,
	}

	go func() {
		// #nosec G706 -- Log output is safe
		log.Printf("Server listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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
	if db.DB != nil {
		_ = db.DB.Close()
	}

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Server stopped")
}
