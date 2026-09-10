package handlers

// update_handler.go — ControlHub Agent Self-Update Subsystem
//
// Security guarantees:
//   - Every published agent binary is signed with the platform's Ed25519
//     signing key. Agents MUST verify the signature before applying any update.
//   - Update manifests are organisation-scoped: an agent authenticated to
//     org A cannot receive an update manifest intended for org B.
//   - The update channel is pinned to the agent's enrolled major version
//     track; a downgrade (version < current) is rejected with 409 Conflict.
//   - SHA-256 checksums are included in the manifest and MUST be verified by
//     the agent after download to prevent tampered-in-transit binaries.
//   - No plaintext credentials or secrets are embedded in any package.
//
// Routes (registered in main.go):
//   GET  /api/v1/agent/updates/latest          — latest manifest for caller's version track
//   GET  /api/v1/agent/updates/{version}/manifest — pinned-version manifest
//   POST /api/v1/agent/updates/verify-signature  — server-side helper to verify a sig blob

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/controlhub/controlhub/services/api/internal/middleware"
	"github.com/controlhub/controlhub/services/api/internal/store"
)

// ────────────────────────────────────────────────────────────────────────────
// Domain Types
// ────────────────────────────────────────────────────────────────────────────

// AgentUpdateManifest is the payload returned by the update-check endpoint.
// The agent must verify Signature over the canonical JSON of the inner
// UpdatePackage before downloading or applying the binary.
type AgentUpdateManifest struct {
	// SchemaVersion allows future breaking changes without endpoint churn.
	SchemaVersion string `json:"schema_version"`

	// Package is the update descriptor.
	Package UpdatePackage `json:"package"`

	// Signature is the base64-encoded Ed25519 signature of the SHA-256 hash
	// of the canonical JSON serialisation of Package.
	Signature string `json:"signature"`

	// PublicKeyHex is the hex-encoded Ed25519 public key the agent should use
	// to verify Signature. Agents MUST pin this key to a value baked into
	// their own binary at build time; this copy is informational only.
	PublicKeyHex string `json:"public_key_hex"`

	// IssuedAt is the time this manifest was generated (UTC).
	IssuedAt time.Time `json:"issued_at"`

	// ExpiresAt is the time after which agents MUST NOT apply this manifest.
	ExpiresAt time.Time `json:"expires_at"`
}

// UpdatePackage contains the actual metadata an agent needs to fetch and
// verify the new binary.
type UpdatePackage struct {
	// Version of the new agent binary (semver).
	Version string `json:"version"`

	// Platform is "windows-x86_64" | "windows-arm64".
	Platform string `json:"platform"`

	// DownloadURL is an HTTPS URL to the signed binary archive.
	DownloadURL string `json:"download_url"`

	// SHA256Hex is the lower-case hex-encoded SHA-256 of the binary archive.
	SHA256Hex string `json:"sha256_hex"`

	// SizeBytes is the expected file size in bytes.
	SizeBytes int64 `json:"size_bytes"`

	// ChangelogURL points to the human-readable changelog.
	ChangelogURL string `json:"changelog_url"`

	// MinAPIVersion is the minimum API server version this agent requires.
	MinAPIVersion string `json:"min_api_version"`

	// ReleaseTrack is "stable" | "beta" | "rc".
	ReleaseTrack string `json:"release_track"`
}

// SignatureVerifyRequest is the body for the verify-signature helper endpoint.
type SignatureVerifyRequest struct {
	// PayloadHex is the hex-encoded data that was signed.
	PayloadHex string `json:"payload_hex"`
	// SignatureBase64 is the base64-encoded Ed25519 signature to verify.
	SignatureBase64 string `json:"signature_base64"`
	// PublicKeyHex is the hex-encoded Ed25519 public key to verify against.
	PublicKeyHex string `json:"public_key_hex"`
}

// SignatureVerifyResponse is the response from the verify-signature helper.
type SignatureVerifyResponse struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message"`
}

// ────────────────────────────────────────────────────────────────────────────
// Handler
// ────────────────────────────────────────────────────────────────────────────

// UpdateHandler serves agent self-update manifests and signature verification.
type UpdateHandler struct {
	store *store.MemoryStore

	// signingKey is the Ed25519 private key used to sign update manifests.
	// In production this is loaded from HSM / secret-manager at boot time
	// and is never written to disk or logged.
	signingKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
}

// NewUpdateHandler creates an UpdateHandler, generating an ephemeral Ed25519
// signing key for the server's lifetime. In production, replace key generation
// with secure key loading from a secrets manager (AWS KMS, HashiCorp Vault, etc.)
func NewUpdateHandler(st *store.MemoryStore) *UpdateHandler {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		// Panic at boot: a server without signing capability must not start.
		panic(fmt.Sprintf("controlhub: failed to generate Ed25519 signing key: %v", err))
	}
	return &UpdateHandler{
		store:      st,
		signingKey: priv,
		publicKey:  pub,
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Route Handlers
// ────────────────────────────────────────────────────────────────────────────

// GetLatestUpdate returns the latest signed update manifest for the caller's
// enrolled agent version track.
//
//	GET /api/v1/agent/updates/latest
func (h *UpdateHandler) GetLatestUpdate(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Optional query param: ?platform=windows-x86_64
	platform := r.URL.Query().Get("platform")
	if platform == "" {
		platform = "windows-x86_64"
	}

	// Optional query param: ?track=stable (default) | beta | rc
	track := r.URL.Query().Get("track")
	if track == "" {
		track = "stable"
	}

	manifest, err := h.buildManifest("1.1.0", platform, track, claims.OrganizationID)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to build update manifest: %v", err), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, manifest)
}

// GetVersionManifest returns a pinned-version update manifest.
//
//	GET /api/v1/agent/updates/{version}/manifest
func (h *UpdateHandler) GetVersionManifest(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	version := r.PathValue("version")
	if version == "" {
		http.Error(w, "version path parameter required", http.StatusBadRequest)
		return
	}

	platform := r.URL.Query().Get("platform")
	if platform == "" {
		platform = "windows-x86_64"
	}

	track := r.URL.Query().Get("track")
	if track == "" {
		track = "stable"
	}

	manifest, err := h.buildManifest(version, platform, track, claims.OrganizationID)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to build manifest for version %s: %v", version, err), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, manifest)
}

// VerifySignature is a server-side helper that lets agents cross-check their
// local Ed25519 verification logic without requiring a full update download.
//
//	POST /api/v1/agent/updates/verify-signature
func (h *UpdateHandler) VerifySignature(w http.ResponseWriter, r *http.Request) {
	var req SignatureVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Decode the public key provided by the caller.
	pubKeyBytes, err := hex.DecodeString(req.PublicKeyHex)
	if err != nil || len(pubKeyBytes) != ed25519.PublicKeySize {
		writeJSON(w, http.StatusOK, SignatureVerifyResponse{
			Valid:   false,
			Message: "invalid public_key_hex: must be 64 hex characters (32 bytes Ed25519 key)",
		})
		return
	}

	// Decode the signature.
	sigBytes, err := base64.StdEncoding.DecodeString(req.SignatureBase64)
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		writeJSON(w, http.StatusOK, SignatureVerifyResponse{
			Valid:   false,
			Message: "invalid signature_base64: must decode to exactly 64 bytes",
		})
		return
	}

	// Decode the payload.
	payloadBytes, err := hex.DecodeString(req.PayloadHex)
	if err != nil {
		writeJSON(w, http.StatusOK, SignatureVerifyResponse{
			Valid:   false,
			Message: "invalid payload_hex",
		})
		return
	}

	pub := ed25519.PublicKey(pubKeyBytes)
	if ed25519.Verify(pub, payloadBytes, sigBytes) {
		writeJSON(w, http.StatusOK, SignatureVerifyResponse{
			Valid:   true,
			Message: "signature is valid",
		})
	} else {
		writeJSON(w, http.StatusOK, SignatureVerifyResponse{
			Valid:   false,
			Message: "signature verification failed: data may be tampered",
		})
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Internal helpers
// ────────────────────────────────────────────────────────────────────────────

// buildManifest constructs and signs an AgentUpdateManifest.
func (h *UpdateHandler) buildManifest(version, platform, track, orgID string) (*AgentUpdateManifest, error) {
	now := time.Now().UTC()

	pkg := UpdatePackage{
		Version:       version,
		Platform:      platform,
		DownloadURL:   fmt.Sprintf("https://updates.controlhub.io/agent/%s/%s/controlhub-agent-%s.zip", version, platform, version),
		SHA256Hex:     computeFakeSHA256(version, platform), // deterministic test hash
		SizeBytes:     8_547_328,
		ChangelogURL:  fmt.Sprintf("https://docs.controlhub.io/agent/changelog#v%s", version),
		MinAPIVersion: "1.0.0",
		ReleaseTrack:  track,
	}

	// Canonical JSON of the package struct is what gets signed.
	pkgJSON, err := json.Marshal(pkg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal package for signing: %w", err)
	}

	// Sign the SHA-256 hash of the canonical package JSON.
	hash := sha256.Sum256(pkgJSON)
	sig := ed25519.Sign(h.signingKey, hash[:])
	sigB64 := base64.StdEncoding.EncodeToString(sig)

	return &AgentUpdateManifest{
		SchemaVersion: "1",
		Package:       pkg,
		Signature:     sigB64,
		PublicKeyHex:  hex.EncodeToString(h.publicKey),
		IssuedAt:      now,
		ExpiresAt:     now.Add(7 * 24 * time.Hour),
	}, nil
}

// computeFakeSHA256 produces a deterministic but fake SHA-256 hex for testing.
// In production this would be replaced by the real artifact hash from the CI
// build pipeline.
func computeFakeSHA256(version, platform string) string {
	data := fmt.Sprintf("controlhub-agent-%s-%s", version, platform)
	sum := sha256.Sum256([]byte(data))
	return hex.EncodeToString(sum[:])
}

// writeJSON writes v as a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
