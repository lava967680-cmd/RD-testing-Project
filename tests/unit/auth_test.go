package unit_test

import (
	"testing"

	"github.com/controlhub/controlhub/services/api/internal/auth"
	"github.com/google/uuid"
)

func TestArgon2idPasswordHashing(t *testing.T) {
	password := "SecretP@ssw0rd2026!"

	hash, err := auth.HashPassword(password, nil)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	if len(hash) == 0 {
		t.Fatal("Hash output is empty")
	}

	// Verify correct password
	match, err := auth.VerifyPassword(password, hash)
	if err != nil || !match {
		t.Errorf("Expected password to verify successfully, got match=%v err=%v", match, err)
	}

	// Verify wrong password
	match, err = auth.VerifyPassword("WrongPassword123!", hash)
	if match {
		t.Errorf("Expected incorrect password verification to fail")
	}
}

func TestJWTAccessTokenLifecycle(t *testing.T) {
	userID := uuid.New()
	orgID := uuid.New()
	role := "Administrator"
	permissions := []string{"device.view", "device.enroll", "device.remote"}
	secret := "test_super_secure_jwt_secret_key_12345678"

	tokenStr, err := auth.GenerateAccessToken(userID, orgID, role, permissions, secret)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	claims, err := auth.ValidateAccessToken(tokenStr, secret)
	if err != nil {
		t.Fatalf("Failed to validate token: %v", err)
	}

	if claims.UserID != userID {
		t.Errorf("Expected UserID %v, got %v", userID, claims.UserID)
	}

	if claims.OrganizationID != orgID {
		t.Errorf("Expected OrganizationID %v, got %v", orgID, claims.OrganizationID)
	}

	if claims.Role != role {
		t.Errorf("Expected Role %v, got %v", role, claims.Role)
	}

	if len(claims.Permissions) != 3 {
		t.Errorf("Expected 3 permissions, got %d", len(claims.Permissions))
	}
}
