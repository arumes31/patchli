package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func handleNodes(w http.ResponseWriter, r *http.Request) {
	nodes := manager.GetNodes()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(nodes)
}

func handleStats(w http.ResponseWriter, r *http.Request) {
	nodes := manager.GetNodes()
	
	stats := struct {
		Vitality   int    `json:"vitality"`
		Immune     string `json:"immune"`
		Recovery   int    `json:"recovery"`
	}{
		Vitality: len(nodes),
		Immune:   "92%", // Mocking for now
		Recovery: 2,     // Mocking for now
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func handleStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// In a real app, you'd use a channel to broadcast logs.
	// For now, just send a periodic keep-alive or initial messages.
	fmt.Fprintf(w, "data: %s\n\n", `{"message": "System stream initialized", "level": "system"}`)
	w.(http.Flusher).Flush()

	// Wait for connection to close
	<-r.Context().Done()
}
