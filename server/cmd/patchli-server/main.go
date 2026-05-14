package main

import (
	"log"
	"net/http"
	"os"

	"github.com/patchli/server/internal/api"
	"github.com/patchli/server/internal/db"
	"github.com/patchli/server/internal/orchestration"
	"github.com/patchli/server/internal/websocket"
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

	// Static files
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("server/static"))))

	// Dashboard UI
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, "server/static/index.html")
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

	log.Printf("Server listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
