package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

const (
	TOTPPeriod = 30 // 30 seconds
	TOTPDigits = 6
)

// GenerateTOTPSecret generates a cryptographically secure 20-byte base32 encoded secret
func GenerateTOTPSecret() (string, error) {
	bytes := make([]byte, 20)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(bytes), nil
}

// GenerateTOTP generates a 6-digit code for a given timestamp
func GenerateTOTP(secret string, t time.Time) (string, error) {
	secret = strings.ToUpper(strings.TrimSpace(secret))
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		// Try standard padding if unpadded failed
		key, err = base32.StdEncoding.DecodeString(secret)
		if err != nil {
			return "", fmt.Errorf("invalid base32 secret: %w", err)
		}
	}

	counter := uint64(t.Unix() / TOTPPeriod)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, counter)

	mac := hmac.New(sha1.New, key)
	mac.Write(buf)
	h := mac.Sum(nil)

	// Dynamic truncation (RFC 4226 Section 5.4)
	offset := h[len(h)-1] & 0x0F
	binaryCode := (int(h[offset])&0x7F)<<24 |
		(int(h[offset+1])&0xFF)<<16 |
		(int(h[offset+2])&0xFF)<<8 |
		(int(h[offset+3]) & 0xFF)

	code := binaryCode % 1000000
	return fmt.Sprintf("%06d", code), nil
}

// ValidateTOTP verifies a 6-digit code allowing ±1 time step drift (30 seconds)
func ValidateTOTP(secret, code string, t time.Time) bool {
	code = strings.TrimSpace(code)
	if len(code) != TOTPDigits {
		return false
	}

	// Check time windows: t - 30s, t, t + 30s
	steps := []int{0, -1, 1}
	for _, step := range steps {
		checkTime := t.Add(time.Duration(step*TOTPPeriod) * time.Second)
		expectedCode, err := GenerateTOTP(secret, checkTime)
		if err == nil && expectedCode == code {
			return true
		}
	}

	return false
}
