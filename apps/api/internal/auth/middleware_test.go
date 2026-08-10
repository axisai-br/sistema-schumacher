package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
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

func TestNewAuthenticatorValidatesHS256AndES256FromExclusiveSources(t *testing.T) {
	privateKey := mustECDSAKey(t)
	jwksServer := newECDSAJWKSServer(t, &privateKey.PublicKey, http.StatusOK)

	authenticator, err := NewAuthenticator(Config{
		JWTSecret: "legacy-secret",
		JWKSURL:   jwksServer.URL,
		Issuer:    "https://supabase.example.com/auth/v1",
		Audience:  "authenticated",
	})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	t.Cleanup(authenticator.Close)

	claims := validTestClaims()
	hsToken := mustSignToken(t, "legacy-secret", claims)
	esToken := mustSignES256Token(t, privateKey, claims)

	if status := serveAuthenticatedRequest(authenticator, hsToken); status != http.StatusNoContent {
		t.Fatalf("expected valid HS256 token status %d, got %d", http.StatusNoContent, status)
	}
	if status := serveAuthenticatedRequest(authenticator, esToken); status != http.StatusNoContent {
		t.Fatalf("expected valid ES256 token status %d, got %d", http.StatusNoContent, status)
	}
}

func TestNewAuthenticatorRejectsUnexpectedHMACAlgorithm(t *testing.T) {
	authenticator, err := NewAuthenticator(Config{JWTSecret: "legacy-secret"})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS384, validTestClaims()).SignedString([]byte("legacy-secret"))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	if status := serveAuthenticatedRequest(authenticator, token); status != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, status)
	}
}

func TestNewAuthenticatorRejectsInvalidES256Signature(t *testing.T) {
	trustedKey := mustECDSAKey(t)
	untrustedKey := mustECDSAKey(t)
	jwksServer := newECDSAJWKSServer(t, &trustedKey.PublicKey, http.StatusOK)
	authenticator, err := NewAuthenticator(Config{JWKSURL: jwksServer.URL})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	t.Cleanup(authenticator.Close)

	token := mustSignES256Token(t, untrustedKey, validTestClaims())
	if status := serveAuthenticatedRequest(authenticator, token); status != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, status)
	}
}

func TestNewAuthenticatorRejectsAsymmetricTokenWithoutJWKS(t *testing.T) {
	privateKey := mustECDSAKey(t)
	authenticator, err := NewAuthenticator(Config{JWTSecret: "legacy-secret"})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}

	token := mustSignES256Token(t, privateKey, validTestClaims())
	if status := serveAuthenticatedRequest(authenticator, token); status != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, status)
	}
}

func TestNewAuthenticatorRejectsHS256TokenWithoutJWTSecret(t *testing.T) {
	privateKey := mustECDSAKey(t)
	jwksServer := newECDSAJWKSServer(t, &privateKey.PublicKey, http.StatusOK)
	authenticator, err := NewAuthenticator(Config{JWKSURL: jwksServer.URL})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	t.Cleanup(authenticator.Close)

	token := mustSignToken(t, "legacy-secret", validTestClaims())
	if status := serveAuthenticatedRequest(authenticator, token); status != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, status)
	}
}

func TestNewAuthenticatorKeepsHS256AvailableWhenJWKSIsUnavailable(t *testing.T) {
	privateKey := mustECDSAKey(t)
	jwksServer := newECDSAJWKSServer(t, &privateKey.PublicKey, http.StatusServiceUnavailable)
	authenticator, err := NewAuthenticator(Config{
		JWTSecret: "legacy-secret",
		JWKSURL:   jwksServer.URL,
	})
	if err != nil {
		t.Fatalf("expected legacy authentication to remain available: %v", err)
	}
	t.Cleanup(authenticator.Close)

	token := mustSignToken(t, "legacy-secret", validTestClaims())
	if status := serveAuthenticatedRequest(authenticator, token); status != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, status)
	}
}

func TestNewAuthenticatorRecoversJWKSWithoutRestart(t *testing.T) {
	privateKey := mustECDSAKey(t)
	jwksServer := newMutableJWKSServer(t, http.StatusServiceUnavailable, nil)
	authenticator, err := newAuthenticator(Config{
		JWTSecret: "legacy-secret",
		JWKSURL:   jwksServer.URL(),
	}, jwksRuntimeConfig{
		refreshInterval:  time.Hour,
		refreshRateLimit: 20 * time.Millisecond,
		refreshTimeout:   100 * time.Millisecond,
		retryInitial:     10 * time.Millisecond,
		retryMaximum:     20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	t.Cleanup(authenticator.Close)

	hsToken := mustSignToken(t, "legacy-secret", validTestClaims())
	esToken := mustSignES256Token(t, privateKey, validTestClaims())
	if status := serveAuthenticatedRequest(authenticator, hsToken); status != http.StatusNoContent {
		t.Fatalf("expected HS256 during JWKS outage status %d, got %d", http.StatusNoContent, status)
	}
	if status := serveAuthenticatedRequest(authenticator, esToken); status != http.StatusUnauthorized {
		t.Fatalf("expected ES256 to fail closed during outage, got %d", status)
	}

	jwksServer.Set(http.StatusOK, []map[string]string{ecdsaTestJWK(&privateKey.PublicKey, "test-key", "ES256")})
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if serveAuthenticatedRequest(authenticator, esToken) == http.StatusNoContent {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected ES256 to recover after JWKS became available")
}

func TestJWKSRecoveryUsesSingleLoopAndStopsOnClose(t *testing.T) {
	jwksServer := newMutableJWKSServer(t, http.StatusServiceUnavailable, nil)
	authenticator, err := newAuthenticator(Config{
		JWTSecret: "legacy-secret",
		JWKSURL:   jwksServer.URL(),
	}, jwksRuntimeConfig{
		refreshInterval:  time.Hour,
		refreshRateLimit: 20 * time.Millisecond,
		refreshTimeout:   100 * time.Millisecond,
		retryInitial:     10 * time.Millisecond,
		retryMaximum:     20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}

	deadline := time.Now().Add(time.Second)
	for jwksServer.RequestCount() < 3 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if requests := jwksServer.RequestCount(); requests < 3 || requests > 4 {
		t.Fatalf("expected one bounded recovery loop, got %d requests", requests)
	}
	authenticator.Close()
	requestsAfterClose := jwksServer.RequestCount()
	time.Sleep(50 * time.Millisecond)
	if requests := jwksServer.RequestCount(); requests != requestsAfterClose {
		t.Fatalf("expected recovery to stop at close, requests changed from %d to %d", requestsAfterClose, requests)
	}
}

func TestAuthenticatorCloseWaitsForActiveUnknownKIDRefresh(t *testing.T) {
	trustedKey := mustECDSAKey(t)
	unknownKey := mustECDSAKey(t)
	refreshStarted := make(chan struct{})
	releaseRefresh := make(chan struct{})
	var requestMu sync.Mutex
	requestCount := 0
	var signalRefresh sync.Once

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestMu.Lock()
		requestCount++
		currentRequest := requestCount
		requestMu.Unlock()
		if currentRequest > 1 {
			signalRefresh.Do(func() { close(refreshStarted) })
			<-releaseRefresh
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{
			ecdsaTestJWK(&trustedKey.PublicKey, "trusted-key", "ES256"),
		}})
	}))
	t.Cleanup(server.Close)

	authenticator, err := newAuthenticator(Config{
		JWTSecret: "legacy-secret",
		JWKSURL:   server.URL,
	}, jwksRuntimeConfig{
		refreshInterval:  time.Hour,
		refreshRateLimit: time.Millisecond,
		refreshTimeout:   time.Second,
		retryInitial:     10 * time.Millisecond,
		retryMaximum:     20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}

	unknownToken := mustSignECDSAToken(t, jwt.SigningMethodES256, unknownKey, "unknown-key", validTestClaims())
	requestDone := make(chan int, 1)
	go func() {
		requestDone <- serveAuthenticatedRequest(authenticator, unknownToken)
	}()

	select {
	case <-refreshStarted:
	case <-time.After(time.Second):
		close(releaseRefresh)
		authenticator.Close()
		t.Fatal("unknown kid did not start JWKS refresh")
	}

	closeDone := make(chan struct{})
	go func() {
		authenticator.Close()
		close(closeDone)
	}()
	closeReturnedBeforeRefresh := false
	select {
	case <-closeDone:
		closeReturnedBeforeRefresh = true
	case <-time.After(25 * time.Millisecond):
	}
	close(releaseRefresh)

	select {
	case status := <-requestDone:
		if status != http.StatusUnauthorized {
			t.Fatalf("expected unknown kid to remain unauthorized, got %d", status)
		}
	case <-time.After(time.Second):
		t.Fatal("active JWKS request remained blocked")
	}
	select {
	case <-closeDone:
	case <-time.After(time.Second):
		t.Fatal("Close remained blocked after active JWKS request completed")
	}
	if closeReturnedBeforeRefresh {
		t.Fatal("Close returned while JWKS Keyfunc was still active")
	}

	trustedToken := mustSignECDSAToken(t, jwt.SigningMethodES256, trustedKey, "trusted-key", validTestClaims())
	if status := serveAuthenticatedRequest(authenticator, trustedToken); status != http.StatusUnauthorized {
		t.Fatalf("expected ES256 validation after Close to fail closed, got %d", status)
	}
	hsToken := mustSignToken(t, "legacy-secret", validTestClaims())
	if status := serveAuthenticatedRequest(authenticator, hsToken); status != http.StatusNoContent {
		t.Fatalf("expected HS256 to remain independent from JWKS lifecycle, got %d", status)
	}
}

func TestNewAuthenticatorRefreshesUnknownKIDWithoutRequestStorm(t *testing.T) {
	oldKey := mustECDSAKey(t)
	newKey := mustECDSAKey(t)
	wrongKey := mustECDSAKey(t)
	jwksServer := newMutableJWKSServer(t, http.StatusOK, []map[string]string{
		ecdsaTestJWK(&oldKey.PublicKey, "old-key", "ES256"),
	})
	authenticator, err := newAuthenticator(Config{JWKSURL: jwksServer.URL()}, jwksRuntimeConfig{
		refreshInterval:  time.Hour,
		refreshRateLimit: 50 * time.Millisecond,
		refreshTimeout:   100 * time.Millisecond,
		retryInitial:     10 * time.Millisecond,
		retryMaximum:     20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	t.Cleanup(authenticator.Close)

	jwksServer.Set(http.StatusOK, []map[string]string{
		ecdsaTestJWK(&oldKey.PublicKey, "old-key", "ES256"),
		ecdsaTestJWK(&newKey.PublicKey, "new-key", "ES256"),
	})
	newToken := mustSignECDSAToken(t, jwt.SigningMethodES256, newKey, "new-key", validTestClaims())
	if status := serveAuthenticatedRequest(authenticator, newToken); status != http.StatusNoContent {
		t.Fatalf("expected new kid to refresh and validate, got %d", status)
	}

	invalidSignature := mustSignECDSAToken(t, jwt.SigningMethodES256, wrongKey, "new-key", validTestClaims())
	if status := serveAuthenticatedRequest(authenticator, invalidSignature); status != http.StatusUnauthorized {
		t.Fatalf("expected invalid signature to remain rejected, got %d", status)
	}

	requestsBeforeAttack := jwksServer.RequestCount()
	maliciousToken := mustSignECDSAToken(t, jwt.SigningMethodES256, wrongKey, "attacker-kid", validTestClaims())
	var wait sync.WaitGroup
	for range 20 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if status := serveAuthenticatedRequest(authenticator, maliciousToken); status != http.StatusUnauthorized {
				t.Errorf("expected malicious kid to be rejected, got %d", status)
			}
		}()
	}
	wait.Wait()
	time.Sleep(100 * time.Millisecond)
	if delta := jwksServer.RequestCount() - requestsBeforeAttack; delta > 1 {
		t.Fatalf("expected unknown kid refresh to be rate limited, got %d JWKS requests", delta)
	}
}

func TestAllowedJWTSigningMethodIsExact(t *testing.T) {
	tests := []struct {
		method jwt.SigningMethod
		want   bool
	}{
		{method: jwt.SigningMethodHS256, want: true},
		{method: jwt.SigningMethodES256, want: true},
		{method: jwt.SigningMethodHS384},
		{method: jwt.SigningMethodHS512},
		{method: jwt.SigningMethodES384},
		{method: jwt.SigningMethodES512},
		{method: jwt.SigningMethodRS256},
		{method: jwt.SigningMethodRS384},
		{method: jwt.SigningMethodRS512},
		{method: jwt.SigningMethodPS256},
		{method: jwt.SigningMethodPS384},
		{method: jwt.SigningMethodPS512},
		{method: jwt.SigningMethodEdDSA},
		{method: jwt.SigningMethodNone},
		{method: unknownSigningMethod{}},
	}
	for _, test := range tests {
		t.Run(test.method.Alg(), func(t *testing.T) {
			if got := allowedJWTSigningMethod(test.method); got != test.want {
				t.Fatalf("allowedJWTSigningMethod(%s)=%t, want %t", test.method.Alg(), got, test.want)
			}
		})
	}
}

func TestNewAuthenticatorRejectsValidES384TokenFromJWKS(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ECDSA key: %v", err)
	}
	jwksServer := newMutableJWKSServer(t, http.StatusOK, []map[string]string{
		ecdsaTestJWK(&privateKey.PublicKey, "es384-key", "ES384"),
	})
	authenticator, err := NewAuthenticator(Config{JWKSURL: jwksServer.URL()})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	t.Cleanup(authenticator.Close)

	token := mustSignECDSAToken(t, jwt.SigningMethodES384, privateKey, "es384-key", validTestClaims())
	if status := serveAuthenticatedRequest(authenticator, token); status != http.StatusUnauthorized {
		t.Fatalf("expected valid ES384 signature to be rejected by exact algorithm policy, got %d", status)
	}
}

type unknownSigningMethod struct{}

func (unknownSigningMethod) Alg() string { return "UNKNOWN" }

func (unknownSigningMethod) Verify(string, string, interface{}) error {
	return errors.New("unsupported")
}

func (unknownSigningMethod) Sign(string, interface{}) (string, error) {
	return "", errors.New("unsupported")
}

func TestNewAuthenticatorRequiresAtLeastOneJWTSource(t *testing.T) {
	if _, err := NewAuthenticator(Config{}); err == nil {
		t.Fatal("expected missing JWT sources to fail")
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

func validTestClaims() jwt.MapClaims {
	return jwt.MapClaims{
		"iss":  "https://supabase.example.com/auth/v1",
		"aud":  "authenticated",
		"sub":  "user-123",
		"role": "authenticated",
		"exp":  time.Now().Add(time.Hour).Unix(),
	}
}

func mustECDSAKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ECDSA key: %v", err)
	}
	return privateKey
}

func mustSignES256Token(t *testing.T, privateKey *ecdsa.PrivateKey, claims jwt.MapClaims) string {
	return mustSignECDSAToken(t, jwt.SigningMethodES256, privateKey, "test-key", claims)
}

func mustSignECDSAToken(t *testing.T, method *jwt.SigningMethodECDSA, privateKey *ecdsa.PrivateKey, kid string, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(method, claims)
	token.Header["kid"] = kid
	signed, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatalf("sign ES256 token: %v", err)
	}
	return signed
}

type mutableJWKSServer struct {
	server       *httptest.Server
	mu           sync.RWMutex
	status       int
	keys         []map[string]string
	requestCount int
}

func newMutableJWKSServer(t *testing.T, status int, keys []map[string]string) *mutableJWKSServer {
	t.Helper()
	state := &mutableJWKSServer{status: status, keys: keys}
	state.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		state.mu.Lock()
		state.requestCount++
		status := state.status
		keys := append([]map[string]string(nil), state.keys...)
		state.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status == http.StatusOK {
			if err := json.NewEncoder(w).Encode(map[string]any{"keys": keys}); err != nil {
				t.Errorf("encode JWKS: %v", err)
			}
		}
	}))
	t.Cleanup(state.server.Close)
	return state
}

func (s *mutableJWKSServer) URL() string {
	return s.server.URL
}

func (s *mutableJWKSServer) Set(status int, keys []map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = status
	s.keys = keys
}

func (s *mutableJWKSServer) RequestCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.requestCount
}

func ecdsaTestJWK(publicKey *ecdsa.PublicKey, kid, alg string) map[string]string {
	curve, size := "P-256", 32
	switch publicKey.Curve {
	case elliptic.P384():
		curve, size = "P-384", 48
	case elliptic.P521():
		curve, size = "P-521", 66
	}
	return map[string]string{
		"kty": "EC",
		"use": "sig",
		"alg": alg,
		"kid": kid,
		"crv": curve,
		"x":   base64.RawURLEncoding.EncodeToString(publicKey.X.FillBytes(make([]byte, size))),
		"y":   base64.RawURLEncoding.EncodeToString(publicKey.Y.FillBytes(make([]byte, size))),
	}
}

func newECDSAJWKSServer(t *testing.T, publicKey *ecdsa.PublicKey, status int) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status != http.StatusOK {
			return
		}
		payload := map[string]any{"keys": []map[string]string{ecdsaTestJWK(publicKey, "test-key", "ES256")}}
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			t.Errorf("encode JWKS: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func serveAuthenticatedRequest(authenticator *Authenticator, token string) int {
	handler := authenticator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	handler.ServeHTTP(recorder, request)
	return recorder.Code
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
