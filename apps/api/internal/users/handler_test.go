package users

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"schumacher-tur/api/internal/auth"
	"schumacher-tur/api/internal/shared/config"
)

func TestMeEnsuresMissingUserProfileFromAuth(t *testing.T) {
	profiles := &fakeUserProfileService{
		roles:        []string{"operador"},
		hasRecipient: true,
	}
	handler := NewHandler(nil, config.Config{}, profiles)

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	userID := uuid.NewString()
	req := httptest.NewRequest(http.MethodGet, "/users/me", nil)
	req = req.WithContext(auth.WithUser(req.Context(), auth.AuthUser{
		ID:    userID,
		Email: "operador@example.com",
		Name:  "Operador Schumacher",
	}))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if profiles.ensureCalls != 1 {
		t.Fatalf("expected one profile ensure call, got %d", profiles.ensureCalls)
	}
	if profiles.lastUser.ID != userID || profiles.lastUser.Email != "operador@example.com" {
		t.Fatalf("expected auth user to be forwarded, got %#v", profiles.lastUser)
	}

	var out MeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if out.UserID != userID {
		t.Fatalf("expected response user_id %s, got %s", userID, out.UserID)
	}
	if len(out.Roles) != 1 || out.Roles[0] != "operador" {
		t.Fatalf("expected operador role, got %#v", out.Roles)
	}
	if !out.HasRecipient {
		t.Fatalf("expected recipient linkage to be returned")
	}
}

func TestMeReturnsForbiddenWhenProfileCannotBeConfigured(t *testing.T) {
	profiles := &fakeUserProfileService{err: ErrUserProfileNotConfigured}
	handler := NewHandler(nil, config.Config{}, profiles)

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/users/me", nil)
	req = req.WithContext(auth.WithUser(req.Context(), auth.AuthUser{
		ID:    uuid.NewString(),
		Email: "operador@example.com",
	}))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "USER_PROFILE_NOT_CONFIGURED") {
		t.Fatalf("expected USER_PROFILE_NOT_CONFIGURED response, got %s", rec.Body.String())
	}
}

func TestMeRequiresAuth(t *testing.T) {
	handler := NewHandler(nil, config.Config{}, &fakeUserProfileService{})

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/users/me", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestSupabaseAdminRequestsPreferSecretKey(t *testing.T) {
	testSupabaseAdminCredential(t, config.Config{
		SupabaseSecretKey:      "sb_secret_preferred-key",
		SupabaseServiceRoleKey: "legacy-service-role-key",
	}, "sb_secret_preferred-key", "")
}

func TestSupabaseAdminRequestsFallBackToServiceRoleKey(t *testing.T) {
	testSupabaseAdminCredential(t, config.Config{
		SupabaseServiceRoleKey: "legacy-service-role-key",
	}, "legacy-service-role-key", "Bearer legacy-service-role-key")
}

func TestSupabaseAdminReadinessRequiresResolvedCredential(t *testing.T) {
	handler := NewHandler(nil, config.Config{SupabaseURL: "https://supabase.example.com"})
	if err := handler.ensureAdminReady(); err == nil {
		t.Fatal("expected missing administrative credential to fail readiness")
	}
}

func TestSupabaseAdminRejectsInvalidSecretKeyWithoutLegacyFallback(t *testing.T) {
	for _, invalid := range []string{"service-role.jwt.value", "arbitrary text", "sb_secret_"} {
		t.Run(invalid, func(t *testing.T) {
			handler := NewHandler(nil, config.Config{
				SupabaseURL:            "https://supabase.example.com",
				SupabaseSecretKey:      invalid,
				SupabaseServiceRoleKey: "legacy-service-role-key",
			})
			err := handler.ensureAdminReady()
			if err == nil {
				t.Fatal("expected invalid SUPABASE_SECRET_KEY to fail closed")
			}
			if strings.Contains(err.Error(), invalid) {
				t.Fatal("configuration error must not include credential value")
			}
		})
	}
}

func TestSupabaseAdminWhitespaceSecretFallsBackToLegacy(t *testing.T) {
	testSupabaseAdminCredential(t, config.Config{
		SupabaseSecretKey:      "  ",
		SupabaseServiceRoleKey: "legacy-service-role-key",
	}, "legacy-service-role-key", "Bearer legacy-service-role-key")
}

func TestSupabaseAdminRequestsNeverFollowRedirectsWithCredential(t *testing.T) {
	credentials := []config.Config{
		{SupabaseSecretKey: "sb_secret_redirect-test"},
		{SupabaseServiceRoleKey: "legacy-service-role-key"},
	}
	for index, cfg := range credentials {
		t.Run(strconv.Itoa(index), func(t *testing.T) {
			var destinationHeaders http.Header
			destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				destinationHeaders = r.Header.Clone()
				w.WriteHeader(http.StatusNoContent)
			}))
			t.Cleanup(destination.Close)

			var sourceAPIKey string
			source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				sourceAPIKey = r.Header.Get("apikey")
				http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
			}))
			t.Cleanup(source.Close)

			cfg.SupabaseURL = source.URL
			handler := NewHandler(nil, cfg)
			if _, err := handler.doSupabaseAdminRequest(context.Background(), http.MethodGet, "/auth/v1/admin/users", nil); err == nil {
				t.Fatal("expected administrative redirect to fail closed")
			}
			if sourceAPIKey == "" {
				t.Fatal("expected original Supabase endpoint to receive apikey")
			}
			if destinationHeaders != nil && (destinationHeaders.Get("apikey") != "" || destinationHeaders.Get("Authorization") != "") {
				t.Fatal("redirect destination received administrative credential")
			}
		})
	}
}

func TestSupabaseAdminErrorsRedactReflectedCredentials(t *testing.T) {
	tests := []struct {
		name       string
		cfg        config.Config
		credential string
	}{
		{
			name: "secret",
			cfg: config.Config{
				SupabaseSecretKey:      "sb_secret_reflected-value",
				SupabaseServiceRoleKey: "legacy-not-selected",
			},
			credential: "sb_secret_reflected-value",
		},
		{
			name:       "legacy",
			cfg:        config.Config{SupabaseServiceRoleKey: "legacy-reflected-value"},
			credential: "legacy-reflected-value",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			responseBody := ""
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte(responseBody))
			}))
			t.Cleanup(server.Close)

			test.cfg.SupabaseURL = server.URL
			handler := NewHandler(nil, test.cfg)
			bodies := []string{
				test.credential,
				"Bearer " + test.credential,
				"upstream reflected " + test.credential + " among diagnostic text",
				`{"message":"upstream reflected Bearer ` + test.credential + `"}`,
			}
			for index, body := range bodies {
				t.Run(strconv.Itoa(index), func(t *testing.T) {
					responseBody = body
					_, err := handler.doSupabaseAdminRequest(context.Background(), http.MethodGet, "/auth/v1/admin/users", nil)
					assertSanitizedAdminError(t, err, test.credential)

					request := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{
						"email":"operator@example.com",
						"full_name":"Operator",
						"password":"password-123"
					}`))
					recorder := httptest.NewRecorder()
					handler.create(recorder, request)
					if recorder.Code != http.StatusBadGateway {
						t.Fatalf("expected handler status %d, got %d", http.StatusBadGateway, recorder.Code)
					}
					response := recorder.Body.String()
					if strings.Contains(response, test.credential) || strings.Contains(response, "Bearer "+test.credential) {
						t.Fatalf("handler response leaked administrative credential: %q", response)
					}
					if !strings.Contains(response, "[REDACTED]") || !strings.Contains(response, "502") {
						t.Fatalf("expected sanitized diagnostic details in handler response, got %q", response)
					}
				})
			}
		})
	}
}

func assertSanitizedAdminError(t *testing.T, err error, credential string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected Supabase administrative error")
	}
	message := err.Error()
	if strings.Contains(message, credential) || strings.Contains(message, "Bearer "+credential) {
		t.Fatalf("administrative error leaked credential: %q", message)
	}
	if !strings.Contains(message, "[REDACTED]") || !strings.Contains(message, "502") {
		t.Fatalf("expected useful sanitized diagnostics, got %q", message)
	}
}

func testSupabaseAdminCredential(t *testing.T, cfg config.Config, expectedAPIKey, expectedAuthorization string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("apikey"); got != expectedAPIKey {
			t.Errorf("expected apikey %q, got %q", expectedAPIKey, got)
		}
		if got := r.Header.Get("Authorization"); got != expectedAuthorization {
			t.Errorf("expected Authorization %q, got %q", expectedAuthorization, got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)

	cfg.SupabaseURL = server.URL
	handler := NewHandler(nil, cfg)
	if err := handler.ensureAdminReady(); err != nil {
		t.Fatalf("expected administrative configuration to be ready: %v", err)
	}
	if _, err := handler.doSupabaseAdminRequest(context.Background(), http.MethodGet, "/auth/v1/admin/users", nil); err != nil {
		t.Fatalf("administrative request: %v", err)
	}
}

type fakeUserProfileService struct {
	ensureCalls  int
	lastUser     auth.AuthUser
	profile      Profile
	roles        []string
	hasRecipient bool
	err          error
	rolesErr     error
	recipientErr error
}

func (f *fakeUserProfileService) EnsureUserProfileFromAuth(_ context.Context, user auth.AuthUser) (Profile, error) {
	f.ensureCalls++
	f.lastUser = user
	if f.err != nil {
		return Profile{}, f.err
	}
	profile := f.profile
	if profile.ID == "" {
		profile.ID = user.ID
	}
	if profile.Email == "" {
		profile.Email = user.Email
	}
	if profile.FullName == "" {
		profile.FullName = user.Name
	}
	if profile.FullName == "" {
		profile.FullName = profile.Email
	}
	return profile, nil
}

func (f *fakeUserProfileService) LoadRoles(_ context.Context, _ string) ([]string, error) {
	if f.rolesErr != nil {
		return nil, f.rolesErr
	}
	if f.roles == nil {
		return []string{}, nil
	}
	return f.roles, nil
}

func (f *fakeUserProfileService) HasActiveRecipient(_ context.Context, _ string) (bool, error) {
	if f.recipientErr != nil {
		return false, f.recipientErr
	}
	return f.hasRecipient, nil
}

var _ userProfileService = (*fakeUserProfileService)(nil)
