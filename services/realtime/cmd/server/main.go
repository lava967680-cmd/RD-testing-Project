package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/controlhub/controlhub/services/realtime/internal/hub"
	"github.com/google/uuid"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	h := hub.NewHub(45 * time.Second)
	defer h.Stop()

	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "healthy",
			"service": "controlhub-realtime",
			"time":    time.Now().UTC(),
		})
	})

	// Client SSE stream for real-time telemetry updates (HTTP Server-Sent Events)
	mux.HandleFunc("GET /api/v1/events/stream", func(w http.ResponseWriter, r *http.Request) {
		orgIDStr := r.URL.Query().Get("org_id")
		orgID, err := uuid.Parse(orgIDStr)
		if err != nil {
			http.Error(w, "Invalid org_id parameter", http.StatusBadRequest)
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		clientSession := &hub.ClientSession{
			UserID:         uuid.New(),
			OrganizationID: orgID,
			SendChan:       make(chan []byte, 32),
		}
		h.RegisterClient(clientSession)
		defer h.UnregisterClient(orgID, clientSession.UserID)

		// Send initial connection event
		fmt.Fprintf(w, "data: {\"event\":\"connected\",\"time\":\"%s\"}\n\n", time.Now().UTC().Format(time.RFC3339))
		flusher.Flush()

		notify := r.Context().Done()
		for {
			select {
			case <-notify:
				return
			case msg, ok := <-clientSession.SendChan:
				if !ok {
					return
				}
				fmt.Fprintf(w, "data: %s\n\n", string(msg))
				flusher.Flush()
			}
		}
	})

	// Agent Telemetry Ingest Webhook (dispatches to Hub)
	mux.HandleFunc("POST /api/v1/agent/telemetry", func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			DeviceID       uuid.UUID   `json:"device_id"`
			OrganizationID uuid.UUID   `json:"organization_id"`
			Metrics        interface{} `json:"metrics"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "Invalid JSON body", http.StatusBadRequest)
			return
		}

		h.BroadcastTelemetry(payload.OrganizationID, payload.DeviceID, payload)
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprintf(w, `{"status":"telemetry_routed"}`)
	})

	addr := fmt.Sprintf(":%s", port)
	log.Printf("ControlHub Realtime Gateway listening on %s", addr)
	log.Printf("Real-time SSE and Watchdog active (Heartbeat TTL: 45s)")

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Realtime server fatal error: %v", err)
	}
}
