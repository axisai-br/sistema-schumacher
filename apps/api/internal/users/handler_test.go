package users

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
