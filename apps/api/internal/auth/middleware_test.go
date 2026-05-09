package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

func TestAudMatchesString(t *testing.T) {
	if !audMatches("authenticated", "authenticated") {
		t.Fatalf("expected audience match")
	}
	if audMatches("authenticated", "other") {
		t.Fatalf("expected audience mismatch")
	}
}

func TestAudMatchesSlice(t *testing.T) {
	aud := []interface{}{"other", "authenticated"}
	if !audMatches("authenticated", aud) {
		t.Fatalf("expected audience match in slice")
	}
}

func TestMiddlewareSkip(t *testing.T) {
	auth, err := NewAuthenticator(Config{Skip: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected %d, got %d", http.StatusNoContent, rr.Code)
	}
}

func TestMiddlewareHS256Token(t *testing.T) {
	auth := &Authenticator{
		jwtSecret: []byte("secret"),
		issuer:    "https://supabase.example.com/auth/v1",
		audience:  "authenticated",
	}

	var gotUserID string
	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := UserIDFromContext(r.Context())
		if !ok {
			t.Fatalf("expected user id in context")
		}
		gotUserID = userID
		w.WriteHeader(http.StatusNoContent)
	}))

	token := mustSignToken(t, "secret", jwt.MapClaims{
		"iss":  "https://supabase.example.com/auth/v1",
		"aud":  "authenticated",
		"sub":  "user-123",
		"role": "authenticated",
		"exp":  time.Now().Add(time.Hour).Unix(),
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected %d, got %d", http.StatusNoContent, rr.Code)
	}
	if gotUserID != "user-123" {
		t.Fatalf("expected user id %q, got %q", "user-123", gotUserID)
	}
}

func TestMiddlewareRejectsInvalidHS256Signature(t *testing.T) {
	auth := &Authenticator{
		jwtSecret: []byte("secret"),
		issuer:    "https://supabase.example.com/auth/v1",
		audience:  "authenticated",
	}
	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	token := mustSignToken(t, "other-secret", jwt.MapClaims{
		"iss":  "https://supabase.example.com/auth/v1",
		"aud":  "authenticated",
		"sub":  "user-123",
		"role": "authenticated",
		"exp":  time.Now().Add(time.Hour).Unix(),
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d", http.StatusUnauthorized, rr.Code)
	}
}

func TestMiddlewareRejectsMissingIssuerByDefault(t *testing.T) {
	auth := &Authenticator{
		jwtSecret: []byte("secret"),
		issuer:    "https://supabase.example.com/auth/v1",
		audience:  "authenticated",
	}
	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	token := mustSignToken(t, "secret", jwt.MapClaims{
		"aud":  "authenticated",
		"sub":  "user-123",
		"role": "authenticated",
		"exp":  time.Now().Add(time.Hour).Unix(),
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d", http.StatusUnauthorized, rr.Code)
	}
}

func TestMiddlewareAllowsMissingIssuerWhenCompatibilityEnabled(t *testing.T) {
	auth := &Authenticator{
		jwtSecret:          []byte("secret"),
		issuer:             "https://supabase.example.com/auth/v1",
		audience:           "authenticated",
		allowMissingIssuer: true,
	}
	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	token := mustSignToken(t, "secret", jwt.MapClaims{
		"aud":  "authenticated",
		"sub":  "user-123",
		"role": "authenticated",
		"exp":  time.Now().Add(time.Hour).Unix(),
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected %d, got %d", http.StatusNoContent, rr.Code)
	}
}

func mustSignToken(t *testing.T, secret string, claims jwt.MapClaims) string {
	t.Helper()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return token
}

func TestMiddlewareServiceToken(t *testing.T) {
	subject := serviceSubject("service-token-123")
	auth := &Authenticator{
		services: map[string]string{
			"service-token-123": subject,
		},
	}

	var gotUserID string
	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := UserIDFromContext(r.Context())
		if !ok {
			t.Fatalf("expected user id in context")
		}
		gotUserID = userID
		w.WriteHeader(http.StatusNoContent)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer service-token-123")
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected %d, got %d", http.StatusNoContent, rr.Code)
	}
	if gotUserID != subject {
		t.Fatalf("expected subject %q, got %q", subject, gotUserID)
	}
}
