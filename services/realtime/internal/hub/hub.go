package hub

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

type EventMessage struct {
	Event          string      `json:"event"`
	OrganizationID uuid.UUID   `json:"organization_id"`
	Timestamp      time.Time   `json:"timestamp"`
	Data           interface{} `json:"data"`
}

type AgentSession struct {
	DeviceID       uuid.UUID
	OrganizationID uuid.UUID
	Hostname       string
	LastHeartbeat  time.Time
	SendChan       chan []byte
}

type ClientSession struct {
	UserID         uuid.UUID
	OrganizationID uuid.UUID
	SendChan       chan []byte
}

type Hub struct {
	mu           sync.RWMutex
	agents       map[uuid.UUID]*AgentSession                // deviceID -> session
	clients      map[uuid.UUID]map[uuid.UUID]*ClientSession // orgID -> map[userID]session
	heartbeatTTL time.Duration
	stopChan     chan struct{}
}

func NewHub(heartbeatTTL time.Duration) *Hub {
	if heartbeatTTL <= 0 {
		heartbeatTTL = 45 * time.Second
	}

	h := &Hub{
		agents:       make(map[uuid.UUID]*AgentSession),
		clients:      make(map[uuid.UUID]map[uuid.UUID]*ClientSession),
		heartbeatTTL: heartbeatTTL,
		stopChan:     make(chan struct{}),
	}

	// Start heartbeat watchdog
	go h.watchdog()
	return h
}

func (h *Hub) RegisterAgent(session *AgentSession) {
	h.mu.Lock()
	defer h.mu.Unlock()

	session.LastHeartbeat = time.Now().UTC()
	h.agents[session.DeviceID] = session

	// Broadcast device.online event to organization
	h.broadcastToOrgLocked(session.OrganizationID, EventMessage{
		Event:          "device.online",
		OrganizationID: session.OrganizationID,
		Timestamp:      time.Now().UTC(),
		Data: map[string]interface{}{
			"device_id": session.DeviceID,
			"hostname":  session.Hostname,
			"status":    "ONLINE",
		},
	})
	log.Printf("[Realtime Hub] Agent registered: %s (Org: %s)", session.DeviceID, session.OrganizationID)
}

func (h *Hub) UnregisterAgent(deviceID uuid.UUID) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if session, exists := h.agents[deviceID]; exists {
		delete(h.agents, deviceID)
		close(session.SendChan)

		h.broadcastToOrgLocked(session.OrganizationID, EventMessage{
			Event:          "device.offline",
			OrganizationID: session.OrganizationID,
			Timestamp:      time.Now().UTC(),
			Data: map[string]interface{}{
				"device_id": session.DeviceID,
				"hostname":  session.Hostname,
				"status":    "OFFLINE",
			},
		})
		log.Printf("[Realtime Hub] Agent unregistered: %s", deviceID)
	}
}

func (h *Hub) RecordHeartbeat(deviceID uuid.UUID) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if session, exists := h.agents[deviceID]; exists {
		session.LastHeartbeat = time.Now().UTC()
	}
}

func (h *Hub) RegisterClient(session *ClientSession) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.clients[session.OrganizationID]; !exists {
		h.clients[session.OrganizationID] = make(map[uuid.UUID]*ClientSession)
	}
	h.clients[session.OrganizationID][session.UserID] = session
	log.Printf("[Realtime Hub] Client registered: %s (Org: %s)", session.UserID, session.OrganizationID)
}

func (h *Hub) UnregisterClient(orgID, userID uuid.UUID) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if orgClients, exists := h.clients[orgID]; exists {
		if session, ok := orgClients[userID]; ok {
			delete(orgClients, userID)
			close(session.SendChan)
		}
		if len(orgClients) == 0 {
			delete(h.clients, orgID)
		}
	}
	log.Printf("[Realtime Hub] Client unregistered: %s", userID)
}

func (h *Hub) BroadcastTelemetry(orgID, deviceID uuid.UUID, telemetryData interface{}) {
	h.RecordHeartbeat(deviceID)

	h.BroadcastToOrg(orgID, EventMessage{
		Event:          "device.telemetry",
		OrganizationID: orgID,
		Timestamp:      time.Now().UTC(),
		Data:           telemetryData,
	})
}

func (h *Hub) BroadcastToOrg(orgID uuid.UUID, msg EventMessage) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	h.broadcastToOrgLocked(orgID, msg)
}

func (h *Hub) broadcastToOrgLocked(orgID uuid.UUID, msg EventMessage) {
	payload, err := json.Marshal(msg)
	if err != nil {
		return
	}

	if orgClients, exists := h.clients[orgID]; exists {
		for _, client := range orgClients {
			select {
			case client.SendChan <- payload:
			default:
				log.Printf("Client buffer full, dropping message for %s", client.UserID)
			}
		}
	}
}

func (h *Hub) watchdog() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-h.stopChan:
			return
		case now := <-ticker.C:
			h.mu.Lock()
			for deviceID, session := range h.agents {
				if now.Sub(session.LastHeartbeat) > h.heartbeatTTL {
					log.Printf("[Watchdog] Device %s missed heartbeats. Marking OFFLINE.", deviceID)
					delete(h.agents, deviceID)
					close(session.SendChan)

					h.broadcastToOrgLocked(session.OrganizationID, EventMessage{
						Event:          "device.offline",
						OrganizationID: session.OrganizationID,
						Timestamp:      now.UTC(),
						Data: map[string]interface{}{
							"device_id": session.DeviceID,
							"hostname":  session.Hostname,
							"status":    "OFFLINE",
							"reason":    "heartbeat_timeout",
						},
					})
				}
			}
			h.mu.Unlock()
		}
	}
}

func (h *Hub) Stop() {
	close(h.stopChan)
}
