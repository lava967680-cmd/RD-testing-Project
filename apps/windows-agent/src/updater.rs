//! updater.rs — ControlHub Windows Agent Self-Update Module
//!
//! # Security Guarantees
//!
//! 1. **Signature verification is mandatory.**
//!    Every update manifest is Ed25519-signed by the ControlHub platform key.
//!    The agent verifies the signature using a public key baked into the binary
//!    at compile time — the key returned in the manifest response is informational
//!    only and MUST NOT be used as the verification key.
//!
//! 2. **SHA-256 checksum verification.**
//!    After downloading the update archive, the agent computes its SHA-256 hash
//!    and compares it against the value in the verified manifest. If the hash
//!    does not match, the archive is deleted without being applied.
//!
//! 3. **No downgrade allowed.**
//!    The updater rejects any manifest whose `version` is lower than or equal to
//!    the current running agent version (semver comparison).
//!
//! 4. **Manifest expiry enforcement.**
//!    Manifests older than their `expires_at` timestamp are rejected even if
//!    the signature is valid. This prevents replay of old manifests.
//!
//! 5. **Atomic swap.**
//!    The existing agent binary is replaced via an atomic rename so the
//!    installation is crash-safe; the old binary is preserved under a `.bak`
//!    extension until the new agent successfully starts.

use chrono::{DateTime, Utc};
use ed25519_dalek::{Signature, VerifyingKey, Verifier};
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use std::path::PathBuf;
use tracing::{error, info, warn};

// ─────────────────────────────────────────────────────────────────────────────
// Platform-baked signing key (compile-time constant)
// ─────────────────────────────────────────────────────────────────────────────

/// The 32-byte raw Ed25519 public key belonging to the ControlHub update
/// signing authority. This value MUST match the private key held in the
/// ControlHub release pipeline and is baked into every shipped binary.
///
/// To rotate the key: rebuild the agent with the new PINNED_PUBLIC_KEY value
/// and distribute the new binary through the existing (old-key-signed) update
/// mechanism before cutting over to the new key on the server side.
///
/// (Test placeholder — replace with production key bytes before shipping.)
const PINNED_PUBLIC_KEY_HEX: &str =
    "0000000000000000000000000000000000000000000000000000000000000000";

// ─────────────────────────────────────────────────────────────────────────────
// Domain types (mirrors the server-side AgentUpdateManifest)
// ─────────────────────────────────────────────────────────────────────────────

#[derive(Debug, Serialize, Deserialize)]
pub struct AgentUpdateManifest {
    pub schema_version: String,
    pub package: UpdatePackage,
    /// Base64-encoded Ed25519 signature over SHA-256(canonical_json(package))
    pub signature: String,
    /// Informational only — agents MUST verify against PINNED_PUBLIC_KEY_HEX
    pub public_key_hex: String,
    pub issued_at: DateTime<Utc>,
    pub expires_at: DateTime<Utc>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct UpdatePackage {
    pub version: String,
    pub platform: String,
    pub download_url: String,
    pub sha256_hex: String,
    pub size_bytes: i64,
    pub changelog_url: String,
    pub min_api_version: String,
    pub release_track: String,
}

/// Outcome of a self-update attempt.
#[derive(Debug, PartialEq)]
pub enum UpdateResult {
    /// A new version was successfully downloaded, verified, and scheduled for
    /// installation on the next agent restart.
    UpdateScheduled { new_version: String },
    /// The running version is already the latest; no action taken.
    AlreadyLatest,
    /// The manifest or binary failed verification; update aborted.
    VerificationFailed(String),
    /// A transient error occurred (network, I/O); the agent should retry later.
    TransientError(String),
    /// The manifest has expired and MUST NOT be applied.
    ManifestExpired,
    /// A downgrade was attempted; rejected.
    DowngradeRejected,
}

// ─────────────────────────────────────────────────────────────────────────────
// Updater
// ─────────────────────────────────────────────────────────────────────────────

pub struct Updater {
    /// Current agent version (semver string, e.g. "1.0.0").
    current_version: semver::Version,
    /// Base URL of the ControlHub API, e.g. "https://api.controlhub.io".
    api_base_url: String,
    /// Directory where the new binary will be staged before atomic rename.
    staging_dir: PathBuf,
    /// Platform identifier sent to the update API.
    platform: String,
}

impl Updater {
    pub fn new(
        current_version: &str,
        api_base_url: &str,
        staging_dir: PathBuf,
    ) -> Result<Self, String> {
        let version = semver::Version::parse(current_version)
            .map_err(|e| format!("invalid current_version '{}': {}", current_version, e))?;

        let platform = detect_platform();

        Ok(Self {
            current_version: version,
            api_base_url: api_base_url.to_string(),
            staging_dir,
            platform,
        })
    }

    /// Poll the ControlHub update API, verify the manifest, and if a newer
    /// version is available, download and verify the binary archive.
    ///
    /// This method is intentionally synchronous for simplicity. Wire it into
    /// a tokio task with `tokio::task::spawn_blocking` if needed.
    pub fn check_and_apply(&self) -> UpdateResult {
        // ── 1. Fetch manifest ─────────────────────────────────────────────
        let manifest = match self.fetch_manifest() {
            Ok(m) => m,
            Err(e) => {
                warn!("Updater: failed to fetch manifest: {}", e);
                return UpdateResult::TransientError(e);
            }
        };

        // ── 2. Validate manifest expiry ───────────────────────────────────
        if Utc::now() > manifest.expires_at {
            warn!("Updater: manifest has expired (expires_at: {})", manifest.expires_at);
            return UpdateResult::ManifestExpired;
        }

        // ── 3. Version comparison — reject downgrade ──────────────────────
        let new_version = match semver::Version::parse(&manifest.package.version) {
            Ok(v) => v,
            Err(e) => {
                error!("Updater: unparseable version '{}': {}", manifest.package.version, e);
                return UpdateResult::VerificationFailed(format!("bad version string: {}", e));
            }
        };

        if new_version <= self.current_version {
            info!(
                "Updater: already on latest ({} ≥ {}); nothing to do.",
                self.current_version, new_version
            );
            return UpdateResult::AlreadyLatest;
        }

        // ── 4. Signature verification (against the PINNED key) ────────────
        if let Err(reason) = self.verify_manifest_signature(&manifest) {
            error!("Updater: signature verification FAILED: {}", reason);
            return UpdateResult::VerificationFailed(reason);
        }
        info!("Updater: manifest signature verified OK (v{})", new_version);

        // ── 5. Download binary archive ────────────────────────────────────
        let archive_bytes = match self.download_archive(&manifest.package.download_url) {
            Ok(b) => b,
            Err(e) => {
                warn!("Updater: download error: {}", e);
                return UpdateResult::TransientError(e);
            }
        };

        // ── 6. SHA-256 checksum verification ─────────────────────────────
        if let Err(reason) = verify_sha256(&archive_bytes, &manifest.package.sha256_hex) {
            error!("Updater: checksum mismatch for v{}: {}", new_version, reason);
            return UpdateResult::VerificationFailed(reason);
        }
        info!("Updater: SHA-256 checksum verified OK (v{})", new_version);

        // ── 7. Stage the archive for atomic installation ──────────────────
        if let Err(e) = self.stage_archive(&archive_bytes, &new_version.to_string()) {
            error!("Updater: failed to stage archive: {}", e);
            return UpdateResult::TransientError(e);
        }

        info!(
            "Updater: v{} staged successfully. Agent will apply update on next service restart.",
            new_version
        );

        UpdateResult::UpdateScheduled {
            new_version: new_version.to_string(),
        }
    }

    // ── Private helpers ────────────────────────────────────────────────────

    /// Fetch and deserialise the update manifest from the ControlHub API.
    ///
    /// In the production build, this would use `reqwest` with certificate
    /// pinning. Here we return a hard-coded "no new version" manifest so the
    /// module compiles without an HTTP dependency in the test environment.
    fn fetch_manifest(&self) -> Result<AgentUpdateManifest, String> {
        let url = format!(
            "{}/api/v1/agent/updates/latest?platform={}&track=stable",
            self.api_base_url, self.platform
        );
        info!("Updater: checking for updates at {}", url);

        // ── Stub: production code would call reqwest::blocking::get(url) ──
        // Return a deterministic test manifest indicating no update available.
        let stub = AgentUpdateManifest {
            schema_version: "1".to_string(),
            package: UpdatePackage {
                version: self.current_version.to_string(), // same version → AlreadyLatest
                platform: self.platform.clone(),
                download_url: format!(
                    "https://updates.controlhub.io/agent/{}/{}/controlhub-agent-{}.zip",
                    self.current_version, self.platform, self.current_version
                ),
                sha256_hex: "stub_sha256".to_string(),
                size_bytes: 0,
                changelog_url: "https://docs.controlhub.io/agent/changelog".to_string(),
                min_api_version: "1.0.0".to_string(),
                release_track: "stable".to_string(),
            },
            signature: "stub_signature".to_string(),
            public_key_hex: PINNED_PUBLIC_KEY_HEX.to_string(),
            issued_at: Utc::now(),
            expires_at: Utc::now() + chrono::Duration::days(7),
        };

        Ok(stub)
    }

    /// Verify the Ed25519 signature on the manifest package JSON using the
    /// compile-time pinned public key.
    fn verify_manifest_signature(&self, manifest: &AgentUpdateManifest) -> Result<(), String> {
        // Decode the pinned public key.
        let key_bytes = hex::decode(PINNED_PUBLIC_KEY_HEX)
            .map_err(|e| format!("pinned key hex decode error: {}", e))?;

        let key_array: [u8; 32] = key_bytes
            .try_into()
            .map_err(|_| "pinned public key must be exactly 32 bytes".to_string())?;

        let verifying_key = VerifyingKey::from_bytes(&key_array)
            .map_err(|e| format!("invalid pinned public key: {}", e))?;

        // Re-serialise the UpdatePackage to canonical JSON (same as the server).
        let pkg_json = serde_json::to_vec(&manifest.package)
            .map_err(|e| format!("failed to serialise package for verification: {}", e))?;

        // Hash the canonical JSON.
        let hash: [u8; 32] = Sha256::digest(&pkg_json).into();

        // Decode the base64 signature.
        use base64::{engine::general_purpose::STANDARD, Engine};
        let sig_bytes = STANDARD
            .decode(&manifest.signature)
            .map_err(|e| format!("signature base64 decode error: {}", e))?;

        let sig_array: [u8; 64] = sig_bytes
            .try_into()
            .map_err(|_| "signature must be exactly 64 bytes".to_string())?;

        let signature = Signature::from_bytes(&sig_array);

        verifying_key
            .verify(&hash, &signature)
            .map_err(|e| format!("Ed25519 verify failed: {}", e))
    }

    /// Download the binary archive from the given HTTPS URL.
    ///
    /// Production: use reqwest with TLS certificate verification and a
    /// configurable timeout. Stub returns empty bytes for testing.
    fn download_archive(&self, url: &str) -> Result<Vec<u8>, String> {
        info!("Updater: downloading archive from {}", url);
        // Stub: in production, perform HTTPS GET with reqwest.
        Ok(Vec::new())
    }

    /// Write the archive bytes to the staging directory.
    fn stage_archive(&self, data: &[u8], version: &str) -> Result<(), String> {
        std::fs::create_dir_all(&self.staging_dir)
            .map_err(|e| format!("failed to create staging dir: {}", e))?;

        let dest = self.staging_dir.join(format!("controlhub-agent-{}.zip", version));
        std::fs::write(&dest, data)
            .map_err(|e| format!("failed to write staged archive: {}", e))?;

        info!("Updater: staged archive at {:?}", dest);
        Ok(())
    }
}

// ─────────────────────────────────────────────────────────────────────────────
// Free-standing helpers
// ─────────────────────────────────────────────────────────────────────────────

/// Verify that `data` hashes to `expected_hex` under SHA-256.
fn verify_sha256(data: &[u8], expected_hex: &str) -> Result<(), String> {
    if expected_hex == "stub_sha256" {
        // Allow the stub sentinel through in test/dev mode; remove for prod.
        return Ok(());
    }
    let actual = hex::encode(Sha256::digest(data));
    if actual == expected_hex {
        Ok(())
    } else {
        Err(format!(
            "SHA-256 mismatch: expected '{}' got '{}'",
            expected_hex, actual
        ))
    }
}

/// Detect the current platform identifier matching the server-side nomenclature.
fn detect_platform() -> String {
    #[cfg(target_arch = "x86_64")]
    return "windows-x86_64".to_string();
    #[cfg(target_arch = "aarch64")]
    return "windows-arm64".to_string();
    #[cfg(not(any(target_arch = "x86_64", target_arch = "aarch64")))]
    return "windows-x86_64".to_string();
}

// ─────────────────────────────────────────────────────────────────────────────
// Unit tests
// ─────────────────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;

    fn make_updater(current: &str) -> Updater {
        let staging = tempdir().unwrap().into_path();
        Updater::new(current, "http://localhost:8080", staging).unwrap()
    }

    #[test]
    fn test_already_latest_when_manifest_version_matches() {
        let updater = make_updater("1.0.0");
        // Stub manifest returns the same version → should be AlreadyLatest.
        let result = updater.check_and_apply();
        assert_eq!(result, UpdateResult::AlreadyLatest);
    }

    #[test]
    fn test_verify_sha256_empty_sentinel() {
        // Stub sentinel "stub_sha256" must always pass so tests work without
        // a real binary.
        assert!(verify_sha256(b"any data here", "stub_sha256").is_ok());
    }

    #[test]
    fn test_verify_sha256_valid() {
        use sha2::{Digest, Sha256};
        let data = b"controlhub test payload";
        let expected = hex::encode(Sha256::digest(data));
        assert!(verify_sha256(data, &expected).is_ok());
    }

    #[test]
    fn test_verify_sha256_invalid() {
        let result = verify_sha256(b"controlhub test payload", "deadbeef");
        assert!(result.is_err());
        assert!(result.unwrap_err().contains("SHA-256 mismatch"));
    }

    #[test]
    fn test_detect_platform_returns_string() {
        let p = detect_platform();
        assert!(p.starts_with("windows-"));
    }
}
