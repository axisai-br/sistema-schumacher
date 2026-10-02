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
	SupabaseSecretKey          string
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
	PagarmeWebhookSecret       string
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
	ChatOpenAIInterpreterShadowEnabled bool
	ChatOpenAIInterpreterAssistEnabled bool
	ChatOpenAITravelV2ShadowEnabled    bool
	ChatOpenAIPassengerV1ShadowEnabled bool
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
	AtendimentoV2Enabled               bool
	AtendimentoV2Telefones             []string
	AtendimentoV2Modelo                string
	AtendimentoV2ModeloVisao           string
	LLMProvedor                        string
	LLMModoJSON                        string
	LLMEsforcoRaciocinio               string
	LLMTemperatura                     *float64
	LLMModeloReserva                   string // vazio = padrao do provedor; "off" desliga
	LLMHedgeMS                         *int   // nil = padrao (15000); 0 desliga o hedge
	LLMSemRaciocinio                   *bool  // nil = padrao (true so para o modelo reserva)
	NvidiaAPIKey                       string
	NvidiaBaseURL                      string
	AtendimentoV2Concorrencia          int
	AtendimentoV2DebounceMS            int
	// AtendimentoV2EstadoExpiraH: horas sem mensagens para comecar compra nova (0 desliga).
	AtendimentoV2EstadoExpiraH   int
	AtendimentoV2SinalPorPagante float64
	AtendimentoV2Juiz            string
	// AtendimentoV2Motor: "agente" (LLM com ferramentas) ou "comandos" (LLM so
	// extrai JSON; o codigo decide e responde por template).
	AtendimentoV2Motor string
	// LLMModeloExtratorReserva: modelo tentado quando o extrator nao devolve
	// JSON valido (motor comandos); vazio desliga.
	LLMModeloExtratorReserva           string
	TypesafeAPIKey                     string
	AtendimentoV2AlertaWebhookURL      string
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
	provedor := strings.ToLower(firstNonEmpty(os.Getenv("LLM_PROVEDOR"), "nvidia"))
	cfg := Config{
		AppEnv:                             getEnv("APP_ENV", "production"),
		Port:                               getEnv("PORT", "8080"),
		CORSOrigins:                        os.Getenv("CORS_ORIGINS"),
		DatabaseURL:                        os.Getenv("DATABASE_URL"),
		SupabaseURL:                        os.Getenv("SUPABASE_URL"),
		SupabaseAnonKey:                    os.Getenv("SUPABASE_ANON_KEY"),
		SupabaseSecretKey:                  os.Getenv("SUPABASE_SECRET_KEY"),
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
		PagarmeWebhookSecret:               strings.TrimSpace(os.Getenv("PAGARME_WEBHOOK_SECRET")),
		PagarmeWebhookBasicUser:            strings.TrimSpace(os.Getenv("PAGARME_WEBHOOK_BASIC_USER")),
		PagarmeWebhookBasicPass:            strings.TrimSpace(os.Getenv("PAGARME_WEBHOOK_BASIC_PASS")),
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
		AtendimentoV2Enabled:               parseBool(os.Getenv("ATENDIMENTO_V2_ENABLED")),
		AtendimentoV2Telefones:             parsePhoneDigitsList(os.Getenv("ATENDIMENTO_V2_TELEFONES")),
		AtendimentoV2Modelo:                atendimentoV2Modelo(provedor),
		AtendimentoV2ModeloVisao:           strings.TrimSpace(os.Getenv("ATENDIMENTO_V2_MODELO_VISAO")),
		LLMProvedor:                        provedor,
		LLMModoJSON:                        strings.ToLower(strings.TrimSpace(os.Getenv("LLM_MODO_JSON"))),
		LLMEsforcoRaciocinio:               strings.ToLower(strings.TrimSpace(os.Getenv("LLM_REASONING_EFFORT"))),
		LLMTemperatura:                     optFloat("LLM_TEMPERATURA"),
		LLMModeloReserva:                   strings.TrimSpace(os.Getenv("LLM_MODELO_RESERVA")),
		LLMHedgeMS:                         optInt("LLM_HEDGE_MS"),
		LLMSemRaciocinio:                   optBool("LLM_SEM_RACIOCINIO"),
		NvidiaAPIKey:                       strings.TrimSpace(os.Getenv("NVIDIA_API_KEY")),
		NvidiaBaseURL:                      strings.TrimSpace(os.Getenv("NVIDIA_BASE_URL")),
		AtendimentoV2Concorrencia:          positiveInt(getEnvAsInt("ATENDIMENTO_V2_CONCORRENCIA", 4), 4),
		AtendimentoV2DebounceMS:            positiveInt(getEnvAsInt("ATENDIMENTO_V2_DEBOUNCE_MS", 2000), 2000),
		AtendimentoV2EstadoExpiraH:         max(getEnvAsInt("ATENDIMENTO_V2_ESTADO_EXPIRA_H", 72), 0),
		AtendimentoV2SinalPorPagante:       getEnvAsFloat("ATENDIMENTO_V2_SINAL_POR_PAGANTE", 250),
		AtendimentoV2Juiz:                  parseJuiz(os.Getenv("ATENDIMENTO_V2_JUIZ")),
		AtendimentoV2Motor:                 parseMotor(os.Getenv("ATENDIMENTO_V2_MOTOR")),
		LLMModeloExtratorReserva:           strings.TrimSpace(os.Getenv("LLM_MODELO_EXTRATOR_RESERVA")),
		TypesafeAPIKey:                     strings.TrimSpace(os.Getenv("TYPESAFE_API_KEY")),
		AtendimentoV2AlertaWebhookURL:      firstNonEmpty(os.Getenv("ATENDIMENTO_V2_ALERTA_WEBHOOK_URL"), os.Getenv("CHAT_REVIEW_ALERT_WEBHOOK_URL")),
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
		ChatOpenAIInterpreterShadowEnabled: parseBool(os.Getenv("CHAT_OPENAI_INTERPRETER_SHADOW_ENABLED")),
		ChatOpenAIInterpreterAssistEnabled: parseBool(os.Getenv("CHAT_OPENAI_INTERPRETER_ASSIST_ENABLED")),
		ChatOpenAITravelV2ShadowEnabled:    parseBool(os.Getenv("CHAT_OPENAI_TRAVEL_V2_SHADOW_ENABLED")),
		ChatOpenAIPassengerV1ShadowEnabled: parseBool(os.Getenv("CHAT_OPENAI_PASSENGER_MEANING_V1_SHADOW_ENABLED")),
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
	if err := ValidateSupabaseSecretKey(cfg.SupabaseSecretKey); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func ValidateSupabaseSecretKey(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	const prefix = "sb_secret_"
	if !strings.HasPrefix(trimmed, prefix) || len(trimmed) == len(prefix) {
		return errors.New("SUPABASE_SECRET_KEY has invalid format")
	}
	for _, char := range trimmed[len(prefix):] {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '_' || char == '-' {
			continue
		}
		return errors.New("SUPABASE_SECRET_KEY has invalid format")
	}
	return nil
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

// parsePhoneDigitsList converte "+55 (49) 9999-0000, 5549..." em telefones so com digitos.
func parsePhoneDigitsList(val string) []string {
	var out []string
	for _, part := range splitCSV(val) {
		var b strings.Builder
		for _, r := range part {
			if r >= '0' && r <= '9' {
				b.WriteRune(r)
			}
		}
		if b.Len() > 0 {
			out = append(out, b.String())
		}
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if t := strings.TrimSpace(v); t != "" {
			return t
		}
	}
	return ""
}

func positiveInt(n, fallback int) int {
	if n <= 0 {
		return fallback
	}
	return n
}

func getEnvAsFloat(key string, fallback float64) float64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

// atendimentoV2Modelo devolve ATENDIMENTO_V2_MODELO; so o provedor openai
// herda OPENAI_MODEL (um modelo da OpenAI nao serve para a NVIDIA).
func atendimentoV2Modelo(provedor string) string {
	if provedor == "openai" {
		return firstNonEmpty(os.Getenv("ATENDIMENTO_V2_MODELO"), os.Getenv("OPENAI_MODEL"))
	}
	return firstNonEmpty(os.Getenv("ATENDIMENTO_V2_MODELO"))
}

// optFloat devolve nil quando a variavel esta vazia ou invalida.
func optFloat(key string) *float64 {
	n, err := strconv.ParseFloat(strings.TrimSpace(os.Getenv(key)), 64)
	if err != nil || n < 0 {
		return nil
	}
	return &n
}

// optInt devolve nil quando a variavel esta vazia ou invalida (aceita 0).
func optInt(key string) *int {
	n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if err != nil || n < 0 {
		return nil
	}
	return &n
}

// optBool devolve nil quando a variavel esta vazia.
func optBool(key string) *bool {
	if strings.TrimSpace(os.Getenv(key)) == "" {
		return nil
	}
	b := parseBool(os.Getenv(key))
	return &b
}

// parseMotor aceita "agente" ou "comandos"; qualquer outro valor vira "agente".
func parseMotor(val string) string {
	if v := strings.ToLower(strings.TrimSpace(val)); v == "comandos" {
		return v
	}
	return "agente"
}

// parseJuiz aceita "llm", "jev" ou "off"; qualquer outro valor vira "llm".
func parseJuiz(val string) string {
	switch v := strings.ToLower(strings.TrimSpace(val)); v {
	case "llm", "jev", "off":
		return v
	default:
		return "llm"
	}
}
