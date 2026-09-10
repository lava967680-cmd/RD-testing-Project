package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/controlhub/controlhub/services/api/internal/config"
	"github.com/controlhub/controlhub/services/api/internal/handlers"
	"github.com/controlhub/controlhub/services/api/internal/middleware"
	"github.com/controlhub/controlhub/services/api/internal/store"
)

func main() {
	cfg := config.Load()

	// Initialize Store (In-Memory with seed data; configurable for PostgreSQL)
	st := store.NewMemoryStore()

	// Initialize Middlewares & Handlers
	authMw := middleware.NewAuthMiddleware(cfg.JWTSecret)
	authH := handlers.NewAuthHandler(st, cfg.JWTSecret)
	devH := handlers.NewDeviceHandler(st)
	enrollH := handlers.NewEnrollmentHandler(st)
	telemetryH := handlers.NewTelemetryHandler(st)
	groupH := handlers.NewGroupHandler(st)
	sessionH := handlers.NewSessionHandler(st)
	jobH := handlers.NewJobHandler(st)
	opsH := handlers.NewOperationsHandler(st)
	updateH := handlers.NewUpdateHandler(st)


	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "healthy",
			"service": "controlhub-api",
			"time":    time.Now().UTC(),
		})
	})

	// Public Auth Endpoints
	mux.HandleFunc("POST /api/v1/auth/login", authH.Login)
	mux.HandleFunc("POST /api/v1/auth/mfa/verify", authH.VerifyMFA)
	mux.HandleFunc("POST /api/v1/auth/refresh", authH.Refresh)

	// Public Agent Handshake & Execution Callbacks
	mux.HandleFunc("POST /api/v1/enrollment/handshake", enrollH.Handshake)
	mux.HandleFunc("POST /api/v1/sessions/{sessionId}/consent", sessionH.HandleConsent)
	mux.HandleFunc("POST /api/v1/agent/commands/{commandId}/result", jobH.ReportResult)

	// Protected Auth Endpoints
	mux.Handle("POST /api/v1/auth/logout", authMw.Authenticate(http.HandlerFunc(authH.Logout)))
	mux.Handle("GET /api/v1/auth/me", authMw.Authenticate(http.HandlerFunc(authH.Me)))

	// Protected Enrollment Invitation Endpoints (Requires device.enroll permission)
	mux.Handle("POST /api/v1/enrollment/invitations", authMw.Authenticate(
		authMw.RequirePermission("device.enroll", enrollH.CreateInvitation),
	))
	mux.Handle("GET /api/v1/enrollment/invitations", authMw.Authenticate(
		authMw.RequirePermission("device.enroll", enrollH.ListInvitations),
	))
	mux.Handle("DELETE /api/v1/enrollment/invitations/{id}", authMw.Authenticate(
		authMw.RequirePermission("device.enroll", enrollH.RevokeInvitation),
	))

	// Protected Device Endpoints (with granular RBAC permissions)
	mux.Handle("GET /api/v1/devices", authMw.Authenticate(
		authMw.RequirePermission("device.view", devH.ListDevices),
	))
	mux.Handle("GET /api/v1/devices/{deviceId}", authMw.Authenticate(
		authMw.RequirePermission("device.view", devH.GetDevice),
	))
	mux.Handle("POST /api/v1/devices/{deviceId}/approve", authMw.Authenticate(
		authMw.RequirePermission("device.approve", devH.ApproveDevice),
	))
	mux.Handle("POST /api/v1/devices/{deviceId}/revoke", authMw.Authenticate(
		authMw.RequirePermission("device.remove", devH.RevokeDevice),
	))

	// Telemetry Endpoints
	mux.Handle("GET /api/v1/devices/{deviceId}/telemetry", authMw.Authenticate(
		authMw.RequirePermission("device.view", telemetryH.GetDeviceTelemetry),
	))
	mux.HandleFunc("POST /api/v1/telemetry/ingest", telemetryH.IngestTelemetry)

	// Device Group Endpoints
	mux.Handle("GET /api/v1/groups", authMw.Authenticate(
		authMw.RequirePermission("group.view", groupH.ListGroups),
	))
	mux.Handle("POST /api/v1/groups", authMw.Authenticate(
		authMw.RequirePermission("group.manage", groupH.CreateGroup),
	))
	mux.Handle("GET /api/v1/groups/{id}", authMw.Authenticate(
		authMw.RequirePermission("group.view", groupH.GetGroup),
	))
	mux.Handle("PATCH /api/v1/groups/{id}", authMw.Authenticate(
		authMw.RequirePermission("group.manage", groupH.UpdateGroup),
	))
	mux.Handle("DELETE /api/v1/groups/{id}", authMw.Authenticate(
		authMw.RequirePermission("group.manage", groupH.DeleteGroup),
	))
	mux.Handle("POST /api/v1/groups/{id}/devices", authMw.Authenticate(
		authMw.RequirePermission("group.assign_device", groupH.AssignDevices),
	))
	mux.Handle("DELETE /api/v1/groups/{id}/devices/{deviceId}", authMw.Authenticate(
		authMw.RequirePermission("group.assign_device", groupH.RemoveDevice),
	))

	// Authorized Remote Desktop Sessions
	mux.Handle("POST /api/v1/devices/{deviceId}/sessions", authMw.Authenticate(
		authMw.RequirePermission("device.remote", sessionH.CreateSession),
	))
	mux.Handle("GET /api/v1/sessions", authMw.Authenticate(
		authMw.RequirePermission("device.remote", sessionH.ListSessions),
	))
	mux.Handle("GET /api/v1/sessions/{sessionId}", authMw.Authenticate(
		authMw.RequirePermission("device.remote", sessionH.GetSession),
	))
	mux.Handle("POST /api/v1/sessions/{sessionId}/terminate", authMw.Authenticate(
		authMw.RequirePermission("device.remote", sessionH.TerminateSession),
	))

	// Controlled Job & Command Pipeline
	mux.Handle("POST /api/v1/devices/{deviceId}/commands", authMw.Authenticate(
		http.HandlerFunc(jobH.CreateCommand),
	))
	mux.Handle("GET /api/v1/commands/{commandId}", authMw.Authenticate(
		authMw.RequirePermission("device.view", jobH.GetCommand),
	))
	mux.Handle("POST /api/v1/bulk/commands", authMw.Authenticate(
		authMw.RequirePermission("device.restart", jobH.CreateBulkCommand),
	))

	// Alerts Subsystem
	mux.Handle("GET /api/v1/alerts", authMw.Authenticate(
		authMw.RequirePermission("alert.view", opsH.ListAlerts),
	))
	mux.Handle("POST /api/v1/alerts/{id}/acknowledge", authMw.Authenticate(
		authMw.RequirePermission("alert.manage", opsH.AcknowledgeAlert),
	))

	// Software Inventory & Deployments
	mux.Handle("GET /api/v1/software", authMw.Authenticate(
		authMw.RequirePermission("device.software_manage", opsH.ListSoftware),
	))
	mux.Handle("POST /api/v1/software", authMw.Authenticate(
		authMw.RequirePermission("device.software_manage", opsH.CreateSoftware),
	))
	mux.Handle("POST /api/v1/software/{id}/deploy", authMw.Authenticate(
		authMw.RequirePermission("device.software_manage", opsH.DeploySoftware),
	))

	// Licensing & Subscriptions
	mux.Handle("GET /api/v1/licensing/current", authMw.Authenticate(
		authMw.RequirePermission("license.view", opsH.GetCurrentLicense),
	))

	// Compliance Audit Ledger
	mux.Handle("GET /api/v1/audit-logs", authMw.Authenticate(
		authMw.RequirePermission("audit.view", devH.ListAuditLogs),
	))

	// Agent Self-Update Manifest & Signature Verification (Stage 17)
	mux.Handle("GET /api/v1/agent/updates/latest", authMw.Authenticate(
		authMw.RequirePermission("device.view", updateH.GetLatestUpdate),
	))
	mux.Handle("GET /api/v1/agent/updates/{version}/manifest", authMw.Authenticate(
		authMw.RequirePermission("device.view", updateH.GetVersionManifest),
	))
	// Public helper — no auth required so un-enrolled agents can verify sigs
	mux.HandleFunc("POST /api/v1/agent/updates/verify-signature", updateH.VerifySignature)

	addr := fmt.Sprintf(":%s", cfg.Port)
	log.Printf("ControlHub API Service online on %s (Environment: %s)", addr, cfg.Environment)
	log.Printf("Complete Enterprise Fleet Management System Active")

	httpServer := &http.Server{
		Addr:         addr,
		Handler:      corsMiddleware(mux, cfg.AllowedOrigins),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("HTTP server error: %v", err)
	}
}

func corsMiddleware(next http.Handler, allowedOrigin string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
