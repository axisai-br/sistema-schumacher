package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	AppEnv                     string
	Port                       string
	CORSOrigins                string
	DatabaseURL                string
	SupabaseURL                string
	SupabaseAnonKey            string
	SupabaseServiceRoleKey     string
	SupabaseJWKSURL            string
	SupabaseJWTSecret          string
	SupabaseIssuer             string
	SupabaseAudience           string
	SupabaseAllowMissingIssuer bool
	AuthDisabled               bool
	APIServiceTokens           []string
	PagarmeSecretKey           string
	PagarmeBaseURL             string
	PagarmeAPIBaseURL          string
	PagarmePaymentRecipientID  string
	PagarmeDebugRecipientID    string
	PagarmeWebhookBasicUser    string
	PagarmeWebhookBasicPass    string
	AbacatePayAPIKey           string
	AbacatePayWebhookSecret    string
	AbacatePayBaseURL          string
	AbacatePayPublicKey        string
	// TODO(abacatepay-domain): Use hosted frontend URLs (not localhost) in production.
	AbacatePayReturnURL string
	// TODO(abacatepay-domain): Use hosted frontend URLs (not localhost) in production.
	AbacatePayCompletionURL            string
	ChatReviewAlertWebhookURL          string
	OpenAIAPIKey                       string
	OpenAIModel                        string
	OpenAIVisionModel                  string
	OpenAITranscriptionModel           string
	OpenAIBaseURL                      string
	ChatOpenAIContinuityEnabled        bool
	EvolutionBaseURL                   string
	EvolutionAPIKey                    string
	EvolutionInstance                  string
	EvolutionWebhookSecret             string
	ChatDebounceWindowMS               int
	ChatBufferAutoFlushEnabled         bool
	ChatBufferAutoFlushIntervalSeconds int
	ChatBufferAutoFlushLimit           int
	ChatAutoSendRetryEnabled           bool
	ChatAutoSendRetryIntervalSeconds   int
	ChatAutoSendRetryLimit             int
	ChatAutoSendRetryCooldownSeconds   int
	ChatReviewSLAMinutes               int
	ChatDefaultHandoffMode             string
	ChatAgentMode                      string
	GoogleSheetsSpreadsheetID          string
	GoogleServiceAccountJSON           string
}

func Load() (Config, error) {
	cfg := Config{
		AppEnv:                             getEnv("APP_ENV", "production"),
		Port:                               getEnv("PORT", "8080"),
		CORSOrigins:                        os.Getenv("CORS_ORIGINS"),
		DatabaseURL:                        os.Getenv("DATABASE_URL"),
		SupabaseURL:                        os.Getenv("SUPABASE_URL"),
		SupabaseAnonKey:                    os.Getenv("SUPABASE_ANON_KEY"),
		SupabaseServiceRoleKey:             os.Getenv("SUPABASE_SERVICE_ROLE_KEY"),
		SupabaseJWKSURL:                    os.Getenv("SUPABASE_JWKS_URL"),
		SupabaseJWTSecret:                  os.Getenv("SUPABASE_JWT_SECRET"),
		SupabaseIssuer:                     os.Getenv("SUPABASE_ISSUER"),
		SupabaseAudience:                   getEnv("SUPABASE_AUDIENCE", "authenticated"),
		SupabaseAllowMissingIssuer:         parseBool(os.Getenv("SUPABASE_ALLOW_MISSING_ISSUER")),
		AuthDisabled:                       parseBool(os.Getenv("AUTH_DISABLED")),
		APIServiceTokens:                   splitCSV(os.Getenv("API_SERVICE_TOKENS")),
		PagarmeSecretKey:                   os.Getenv("PAGARME_SECRET_KEY"),
		PagarmeBaseURL:                     os.Getenv("PAGARME_BASE_URL"),
		PagarmeAPIBaseURL:                  getEnv("PAGARME_API_BASE_URL", getEnv("PAGARME_BASE_URL", "https://api.pagar.me/core/v5")),
		PagarmePaymentRecipientID:          strings.TrimSpace(os.Getenv("PAGARME_PAYMENT_RECIPIENT_ID")),
		PagarmeDebugRecipientID:            strings.TrimSpace(os.Getenv("PAGARME_DEBUG_RECIPIENT_ID")),
		PagarmeWebhookBasicUser:            os.Getenv("PAGARME_WEBHOOK_BASIC_USER"),
		PagarmeWebhookBasicPass:            os.Getenv("PAGARME_WEBHOOK_BASIC_PASS"),
		AbacatePayAPIKey:                   os.Getenv("ABACATEPAY_API_KEY"),
		AbacatePayWebhookSecret:            os.Getenv("ABACATEPAY_WEBHOOK_SECRET"),
		AbacatePayBaseURL:                  os.Getenv("ABACATEPAY_BASE_URL"),
		AbacatePayPublicKey:                os.Getenv("ABACATEPAY_PUBLIC_KEY"),
		AbacatePayReturnURL:                os.Getenv("ABACATEPAY_RETURN_URL"),
		AbacatePayCompletionURL:            os.Getenv("ABACATEPAY_COMPLETION_URL"),
		ChatReviewAlertWebhookURL:          strings.TrimSpace(os.Getenv("CHAT_REVIEW_ALERT_WEBHOOK_URL")),
		OpenAIAPIKey:                       strings.TrimSpace(os.Getenv("OPENAI_API_KEY")),
		OpenAIModel:                        strings.TrimSpace(os.Getenv("OPENAI_MODEL")),
		OpenAIVisionModel:                  strings.TrimSpace(os.Getenv("OPENAI_VISION_MODEL")),
		OpenAITranscriptionModel:           strings.TrimSpace(os.Getenv("OPENAI_TRANSCRIPTION_MODEL")),
		OpenAIBaseURL:                      getEnv("OPENAI_BASE_URL", "https://api.openai.com/v1"),
		ChatOpenAIContinuityEnabled:        parseBool(os.Getenv("CHAT_OPENAI_CONTINUITY_ENABLED")),
		EvolutionBaseURL:                   strings.TrimSpace(os.Getenv("EVOLUTION_BASE_URL")),
		EvolutionAPIKey:                    strings.TrimSpace(os.Getenv("EVOLUTION_API_KEY")),
		EvolutionInstance:                  strings.TrimSpace(os.Getenv("EVOLUTION_INSTANCE")),
		EvolutionWebhookSecret:             strings.TrimSpace(os.Getenv("EVOLUTION_WEBHOOK_SECRET")),
		ChatDebounceWindowMS:               getEnvAsInt("CHAT_DEBOUNCE_WINDOW_MS", 1500),
		ChatBufferAutoFlushEnabled:         parseBool(os.Getenv("CHAT_BUFFER_AUTO_FLUSH_ENABLED")),
		ChatBufferAutoFlushIntervalSeconds: getEnvAsInt("CHAT_BUFFER_AUTO_FLUSH_INTERVAL_SECONDS", 2),
		ChatBufferAutoFlushLimit:           getEnvAsInt("CHAT_BUFFER_AUTO_FLUSH_LIMIT", 100),
		ChatAutoSendRetryEnabled:           parseBool(os.Getenv("CHAT_AUTO_SEND_RETRY_ENABLED")),
		ChatAutoSendRetryIntervalSeconds:   getEnvAsInt("CHAT_AUTO_SEND_RETRY_INTERVAL_SECONDS", 10),
		ChatAutoSendRetryLimit:             getEnvAsInt("CHAT_AUTO_SEND_RETRY_LIMIT", 20),
		ChatAutoSendRetryCooldownSeconds:   getEnvAsInt("CHAT_AUTO_SEND_RETRY_COOLDOWN_SECONDS", 30),
		ChatReviewSLAMinutes:               getEnvAsInt("CHAT_REVIEW_SLA_MINUTES", 15),
		ChatDefaultHandoffMode:             getEnv("CHAT_DEFAULT_HANDOFF_MODE", "BOT"),
		ChatAgentMode:                      getEnv("CHAT_AGENT_MODE", "legacy"),
		GoogleSheetsSpreadsheetID:          strings.TrimSpace(os.Getenv("GOOGLE_SHEETS_SPREADSHEET_ID")),
		GoogleServiceAccountJSON:           strings.TrimSpace(os.Getenv("GOOGLE_SERVICE_ACCOUNT_JSON")),
	}

	if cfg.DatabaseURL == "" {
		return cfg, errors.New("DATABASE_URL is required")
	}
	if cfg.SupabaseJWTSecret == "" && cfg.SupabaseJWKSURL == "" {
		return cfg, errors.New("SUPABASE_JWT_SECRET or SUPABASE_JWKS_URL is required")
	}
	if cfg.SupabaseIssuer == "" {
		return cfg, errors.New("SUPABASE_ISSUER is required")
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseBool(val string) bool {
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func splitCSV(val string) []string {
	if strings.TrimSpace(val) == "" {
		return nil
	}

	parts := strings.Split(val, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func getEnvAsInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}

	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}
