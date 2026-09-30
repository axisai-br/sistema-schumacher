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

func TestLoadPassengerMeaningV1ShadowFlagDefaultsOffAndParsesExplicitOn(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("SUPABASE_JWT_SECRET", "jwt-secret")
	t.Setenv("SUPABASE_ISSUER", "https://supabase.example.com/auth/v1")
	t.Setenv("CHAT_OPENAI_PASSENGER_MEANING_V1_SHADOW_ENABLED", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load default passenger meaning config: %v", err)
	}
	if cfg.ChatOpenAIPassengerV1ShadowEnabled {
		t.Fatal("passenger meaning v1 shadow must default off")
	}

	t.Setenv("CHAT_OPENAI_PASSENGER_MEANING_V1_SHADOW_ENABLED", "true")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("load enabled passenger meaning config: %v", err)
	}
	if !cfg.ChatOpenAIPassengerV1ShadowEnabled {
		t.Fatal("explicit passenger meaning v1 shadow flag must parse on")
	}
}

func TestLoadAtendimentoV2Defaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("SUPABASE_JWT_SECRET", "jwt-secret")
	t.Setenv("SUPABASE_ISSUER", "https://supabase.example.com/auth/v1")
	for _, k := range []string{"ATENDIMENTO_V2_ENABLED", "ATENDIMENTO_V2_TELEFONES", "ATENDIMENTO_V2_MODELO", "ATENDIMENTO_V2_CONCORRENCIA", "ATENDIMENTO_V2_DEBOUNCE_MS", "ATENDIMENTO_V2_SINAL_POR_PAGANTE", "ATENDIMENTO_V2_JUIZ", "TYPESAFE_API_KEY", "ATENDIMENTO_V2_ALERTA_WEBHOOK_URL", "LLM_PROVEDOR", "LLM_MODO_JSON", "LLM_REASONING_EFFORT", "LLM_TEMPERATURA", "NVIDIA_API_KEY", "NVIDIA_BASE_URL", "ATENDIMENTO_V2_MODELO_VISAO"} {
		t.Setenv(k, "")
	}
	t.Setenv("OPENAI_MODEL", "gpt-fallback")
	t.Setenv("CHAT_REVIEW_ALERT_WEBHOOK_URL", "https://alerta.example.com")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.AtendimentoV2Enabled {
		t.Fatal("atendimento v2 must default off")
	}
	if len(cfg.AtendimentoV2Telefones) != 0 {
		t.Fatalf("expected empty allowlist, got %v", cfg.AtendimentoV2Telefones)
	}
	if cfg.LLMProvedor != "nvidia" || cfg.NvidiaAPIKey != "" || cfg.NvidiaBaseURL != "" || cfg.LLMModoJSON != "" || cfg.LLMTemperatura != nil {
		t.Fatalf("unexpected llm defaults: %q %q %q %v", cfg.LLMProvedor, cfg.NvidiaAPIKey, cfg.LLMModoJSON, cfg.LLMTemperatura)
	}
	if cfg.AtendimentoV2Modelo != "" {
		t.Fatalf("nvidia must not inherit OPENAI_MODEL, got %q", cfg.AtendimentoV2Modelo)
	}
	t.Setenv("LLM_PROVEDOR", "OpenAI")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.LLMProvedor != "openai" || cfg.AtendimentoV2Modelo != "gpt-fallback" {
		t.Fatalf("expected OPENAI_MODEL fallback for openai, got %q %q", cfg.LLMProvedor, cfg.AtendimentoV2Modelo)
	}
	if cfg.AtendimentoV2Concorrencia != 4 || cfg.AtendimentoV2DebounceMS != 2000 || cfg.AtendimentoV2SinalPorPagante != 250 {
		t.Fatalf("unexpected defaults: %d %d %v", cfg.AtendimentoV2Concorrencia, cfg.AtendimentoV2DebounceMS, cfg.AtendimentoV2SinalPorPagante)
	}
	if cfg.AtendimentoV2Juiz != "llm" {
		t.Fatalf("expected judge llm, got %q", cfg.AtendimentoV2Juiz)
	}
	if cfg.AtendimentoV2AlertaWebhookURL != "https://alerta.example.com" {
		t.Fatalf("expected alert webhook fallback, got %q", cfg.AtendimentoV2AlertaWebhookURL)
	}
}

func TestLoadAtendimentoV2ReadsExplicitValues(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("SUPABASE_JWT_SECRET", "jwt-secret")
	t.Setenv("SUPABASE_ISSUER", "https://supabase.example.com/auth/v1")
	t.Setenv("OPENAI_MODEL", "gpt-fallback")
	t.Setenv("ATENDIMENTO_V2_ENABLED", "true")
	t.Setenv("ATENDIMENTO_V2_TELEFONES", "+55 (49) 98870-9047, 554988709048,, abc")
	t.Setenv("ATENDIMENTO_V2_MODELO", "gpt-v2")
	t.Setenv("ATENDIMENTO_V2_CONCORRENCIA", "8")
	t.Setenv("ATENDIMENTO_V2_DEBOUNCE_MS", "500")
	t.Setenv("ATENDIMENTO_V2_SINAL_POR_PAGANTE", "300.5")
	t.Setenv("ATENDIMENTO_V2_JUIZ", "JEV")
	t.Setenv("TYPESAFE_API_KEY", "ts-key")
	t.Setenv("ATENDIMENTO_V2_ALERTA_WEBHOOK_URL", "https://v2.example.com")
	t.Setenv("LLM_PROVEDOR", "OpenAI")
	t.Setenv("NVIDIA_API_KEY", "nv-key")
	t.Setenv("NVIDIA_BASE_URL", "https://nim.example.com/v1")
	t.Setenv("LLM_MODO_JSON", "Response_Format")
	t.Setenv("LLM_REASONING_EFFORT", "HIGH")
	t.Setenv("LLM_TEMPERATURA", "0.3")
	t.Setenv("ATENDIMENTO_V2_MODELO_VISAO", "nvidia/vlm")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.AtendimentoV2Enabled || cfg.AtendimentoV2Modelo != "gpt-v2" || cfg.AtendimentoV2Concorrencia != 8 ||
		cfg.AtendimentoV2DebounceMS != 500 || cfg.AtendimentoV2SinalPorPagante != 300.5 ||
		cfg.AtendimentoV2Juiz != "jev" || cfg.TypesafeAPIKey != "ts-key" || cfg.AtendimentoV2AlertaWebhookURL != "https://v2.example.com" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if cfg.LLMProvedor != "openai" || cfg.NvidiaAPIKey != "nv-key" || cfg.NvidiaBaseURL != "https://nim.example.com/v1" ||
		cfg.LLMModoJSON != "response_format" || cfg.LLMEsforcoRaciocinio != "high" || cfg.LLMTemperatura == nil || *cfg.LLMTemperatura != 0.3 ||
		cfg.AtendimentoV2ModeloVisao != "nvidia/vlm" {
		t.Fatalf("unexpected llm config: %+v", cfg)
	}
	want := []string{"5549988709047", "554988709048"}
	if len(cfg.AtendimentoV2Telefones) != 2 || cfg.AtendimentoV2Telefones[0] != want[0] || cfg.AtendimentoV2Telefones[1] != want[1] {
		t.Fatalf("unexpected phones: %v", cfg.AtendimentoV2Telefones)
	}
}
