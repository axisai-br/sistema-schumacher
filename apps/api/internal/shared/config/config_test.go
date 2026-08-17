package config

import (
	"strings"
	"testing"
)

func TestLoadReadsBothSupabaseAdministrativeCredentialGenerations(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("SUPABASE_JWT_SECRET", "jwt-secret")
	t.Setenv("SUPABASE_JWKS_URL", "")
	t.Setenv("SUPABASE_ISSUER", "https://supabase.example.com/auth/v1")
	t.Setenv("SUPABASE_SECRET_KEY", "sb_secret_preferred-key")
	t.Setenv("SUPABASE_SERVICE_ROLE_KEY", "legacy-service-role-key")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.SupabaseSecretKey != "sb_secret_preferred-key" {
		t.Fatalf("expected new secret key to be loaded, got %q", cfg.SupabaseSecretKey)
	}
	if cfg.SupabaseServiceRoleKey != "legacy-service-role-key" {
		t.Fatalf("expected legacy service role key to remain loaded, got %q", cfg.SupabaseServiceRoleKey)
	}
}

func TestLoadRejectsInvalidSupabaseSecretKeyWithoutLeakingValue(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("SUPABASE_JWT_SECRET", "jwt-secret")
	t.Setenv("SUPABASE_ISSUER", "https://supabase.example.com/auth/v1")
	t.Setenv("SUPABASE_SECRET_KEY", "legacy.jwt.in-wrong-slot")
	t.Setenv("SUPABASE_SERVICE_ROLE_KEY", "legacy-service-role-key")

	_, err := Load()
	if err == nil {
		t.Fatal("expected invalid SUPABASE_SECRET_KEY to fail configuration")
	}
	if strings.Contains(err.Error(), "legacy.jwt.in-wrong-slot") {
		t.Fatal("configuration error must not include credential value")
	}
}
