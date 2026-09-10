package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/controlhub/controlhub/services/api/internal/auth"
)

type contextKey string

const (
	ClaimsContextKey contextKey = "user_claims"
)

type AuthMiddleware struct {
	jwtSecret string
}

func NewAuthMiddleware(jwtSecret string) *AuthMiddleware {
	return &AuthMiddleware{jwtSecret: jwtSecret}
}

// Authenticate verifies the JWT token and attaches user claims to context
func (m *AuthMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			respondError(w, http.StatusUnauthorized, "Missing Authorization header")
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			respondError(w, http.StatusUnauthorized, "Invalid Authorization header format. Expected 'Bearer <token>'")
			return
		}

		tokenStr := parts[1]
		claims, err := auth.ValidateAccessToken(tokenStr, m.jwtSecret)
		if err != nil {
			respondError(w, http.StatusUnauthorized, "Invalid or expired access token")
			return
		}

		ctx := context.WithValue(r.Context(), ClaimsContextKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequirePermission checks if authenticated user possesses a specific RBAC permission
func (m *AuthMiddleware) RequirePermission(requiredPerm string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := r.Context().Value(ClaimsContextKey).(*auth.Claims)
		if !ok || claims == nil {
			respondError(w, http.StatusUnauthorized, "Authentication required")
			return
		}

		hasPerm := false
		for _, perm := range claims.Permissions {
			if perm == requiredPerm || perm == "*" {
				hasPerm = true
				break
			}
		}

		if !hasPerm {
			respondError(w, http.StatusForbidden, "Access forbidden: insufficient permissions")
			return
		}

		next(w, r)
	}
}

// GetClaims extracts Claims from context
func GetClaims(ctx context.Context) (*auth.Claims, bool) {
	claims, ok := ctx.Value(ClaimsContextKey).(*auth.Claims)
	return claims, ok
}

func respondError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}
