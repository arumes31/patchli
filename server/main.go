package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	log.Println("Starting Patchli Server (Control Plane)...")

	// Initialize Worker Pool
	pool := NewWorkerPool(10) // 10 global workers
	pool.Start()
	log.Println("Worker pool started.")

	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		log.Println("Warning: DB_URL not set. In a real environment, this is required.")
	} else {
		log.Printf("Connecting to DB at: %s\n", dbURL)
		// Initialize DB connection here using database/sql or pgx
	}

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		log.Println("Warning: REDIS_URL not set.")
	} else {
		log.Printf("Connecting to Redis at: %s\n", redisURL)
		// Initialize Redis client here
	}

	// Simple Health Check Endpoint
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Dashboard and Setup UI
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, "server/static/index.html")
	})
	http.HandleFunc("/setup", ServeSetupUI)
	http.HandleFunc("/api/v1/setup", SetupHandler)
	http.HandleFunc("/ws", HandleWebSocket)

	// Example API Endpoints
	// http.HandleFunc("/api/nodes", handleNodes)
	// http.HandleFunc("/api/groups", handleGroups)

	port := "8080"
	log.Printf("Server listening on :%s\n", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
