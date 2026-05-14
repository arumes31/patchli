package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/patchli/server/db"
)

func SetupRoutes() *http.ServeMux {
	mux := http.NewServeMux()

	// Simple Health Check Endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Dashboard and Setup UI
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, "static/index.html")
	})
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "static/login.html")
	})
	mux.HandleFunc("/setup", ServeSetupUI)
	mux.HandleFunc("/groups", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "static/groups.html")
	})
	mux.HandleFunc("/settings", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "static/settings.html")
	})
	mux.HandleFunc("/api/v1/setup", SetupHandler)
	mux.HandleFunc("/ws", HandleWebSocket)

	// Example API Endpoints
	mux.HandleFunc("/api/v1/nodes", handleNodes)
	mux.HandleFunc("/api/v1/stats", handleStats)
	mux.HandleFunc("/api/v1/stream", handleStream)

	// Serve Static Assets
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("static/assets"))))

	return mux
}

func RunServer() {
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
		if err := db.Init(dbURL); err != nil {
			log.Fatalf("Failed to initialize database: %v", err)
		}
	}

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		log.Println("Warning: REDIS_URL not set.")
	} else {
		log.Printf("Connecting to Redis at: %s\n", redisURL)
	}

	mux := SetupRoutes()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	
	log.Printf("Server listening on :%s\n", port)
	
	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Could not listen on :%s: %v\n", port, err)
	}
}

func main() {
	RunServer()
}
