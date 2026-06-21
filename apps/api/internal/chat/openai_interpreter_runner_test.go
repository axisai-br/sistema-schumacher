package chat

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"schumacher-tur/api/internal/shared/config"
)

func TestOpenAIStructuredInterpreterRunnerEnabledRequiresConfig(t *testing.T) {
	base := config.Config{
		OpenAIAPIKey:  "sk-test",
		OpenAIModel:   "gpt-test",
		OpenAIBaseURL: "https://api.openai.com/v1",
	}

	for _, tc := range []struct {
		name   string
		mutate func(*config.Config)
	}{
		{name: "missing api key", mutate: func(cfg *config.Config) { cfg.OpenAIAPIKey = "" }},
		{name: "missing model", mutate: func(cfg *config.Config) { cfg.OpenAIModel = "" }},
		{name: "missing base url", mutate: func(cfg *config.Config) { cfg.OpenAIBaseURL = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			tc.mutate(&cfg)

			runner := NewOpenAIStructuredInterpreterRunner(cfg)
			if runner.Enabled() {
				t.Fatalf("expected runner to be disabled")
			}
		})
	}

	runner := NewOpenAIStructuredInterpreterRunner(base)
	if !runner.Enabled() {
		t.Fatalf("expected runner to be enabled")
	}
}

func TestOpenAIStructuredInterpreterRunnerDisabledErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  config.Config
	}{
		{
			name: "missing api key",
			cfg: config.Config{
				OpenAIModel:   "gpt-test",
				OpenAIBaseURL: "https://api.openai.com/v1",
			},
		},
		{
			name: "missing model",
			cfg: config.Config{
				OpenAIAPIKey:  "sk-test",
				OpenAIBaseURL: "https://api.openai.com/v1",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := NewOpenAIStructuredInterpreterRunner(tc.cfg)
			_, err := runner.InterpretStructuredTurn(context.Background(), openAIStructuredInterpreterTestRunInput("oi"))
			if !errors.Is(err, ErrOpenAIStructuredInterpreterDisabled) {
				t.Fatalf("expected disabled error, got %v", err)
			}
		})
	}
}

func TestOpenAIStructuredInterpreterRunnerRequestUsesResponsesAPI(t *testing.T) {
	var captured map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/responses" {
			t.Fatalf("expected path /responses, got %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Fatalf("expected bearer auth, got %s", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(openAIStructuredInterpreterTestResponse("resp_structured_1", validOpenAIStructuredInterpreterOutputText(t)))
	}))
	defer server.Close()

	runner := NewOpenAIStructuredInterpreterRunner(config.Config{
		OpenAIAPIKey:  "sk-test",
		OpenAIModel:   "gpt-test",
		OpenAIBaseURL: server.URL,
	})

	result, err := runner.InterpretStructuredTurn(context.Background(), openAIStructuredInterpreterTestRunInput("oi"))
	if err != nil {
		t.Fatalf("interpret structured turn: %v", err)
	}
	if result.ProviderResponseID != "resp_structured_1" {
		t.Fatalf("expected provider response id, got %s", result.ProviderResponseID)
	}
	if result.Model != "gpt-test" {
		t.Fatalf("expected model gpt-test, got %s", result.Model)
	}
	if result.Interpretation.Source != "openai_structured" {
		t.Fatalf("expected openai_structured source, got %s", result.Interpretation.Source)
	}
	assertOpenAIStructuredInterpreterRequestShape(t, captured)
}

func TestOpenAIStructuredInterpreterRunnerBaseURLWithVersionPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("expected path /v1/responses, got %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(openAIStructuredInterpreterTestResponse("resp_structured_v1", validOpenAIStructuredInterpreterOutputText(t)))
	}))
	defer server.Close()

	runner := NewOpenAIStructuredInterpreterRunner(config.Config{
		OpenAIAPIKey:  "sk-test",
		OpenAIModel:   "gpt-test",
		OpenAIBaseURL: server.URL + "/v1",
	})

	if _, err := runner.InterpretStructuredTurn(context.Background(), openAIStructuredInterpreterTestRunInput("oi")); err != nil {
		t.Fatalf("interpret structured turn: %v", err)
	}
}

func TestOpenAIStructuredInterpreterRunnerRedactsSensitiveDocuments(t *testing.T) {
	var bodyText string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data := map[string]interface{}{}
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		raw, err := json.Marshal(data)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		bodyText = string(raw)
		_ = json.NewEncoder(w).Encode(openAIStructuredInterpreterTestResponse("resp_structured_redacted", validOpenAIStructuredInterpreterOutputText(t)))
	}))
	defer server.Close()

	runner := NewOpenAIStructuredInterpreterRunner(config.Config{
		OpenAIAPIKey:  "sk-test",
		OpenAIModel:   "gpt-test",
		OpenAIBaseURL: server.URL,
	})

	_, err := runner.InterpretStructuredTurn(context.Background(), openAIStructuredInterpreterTestRunInput("Meu CPF e 529.982.247-25"))
	if err != nil {
		t.Fatalf("interpret structured turn: %v", err)
	}
	if strings.Contains(bodyText, "529.982.247-25") {
		t.Fatalf("request must not contain raw CPF: %s", bodyText)
	}
	if !strings.Contains(bodyText, "[DOCUMENTO_REDACTED]") {
		t.Fatalf("request should contain redacted marker: %s", bodyText)
	}
}

func TestOpenAIStructuredInterpreterRunnerAcceptsOutputContentText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "resp_structured_content_text",
			"output": []interface{}{
				map[string]interface{}{
					"content": []interface{}{
						map[string]interface{}{
							"type": "output_text",
							"text": validOpenAIStructuredInterpreterOutputText(t),
						},
					},
				},
			},
		})
	}))
	defer server.Close()

	runner := NewOpenAIStructuredInterpreterRunner(config.Config{
		OpenAIAPIKey:  "sk-test",
		OpenAIModel:   "gpt-test",
		OpenAIBaseURL: server.URL,
	})

	result, err := runner.InterpretStructuredTurn(context.Background(), openAIStructuredInterpreterTestRunInput("oi"))
	if err != nil {
		t.Fatalf("interpret structured turn: %v", err)
	}
	if result.Interpretation.Intent != StructuredIntentGreeting {
		t.Fatalf("expected greeting intent, got %s", result.Interpretation.Intent)
	}
}

func TestOpenAIStructuredInterpreterRunnerRejectsInvalidValidatedOutput(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(map[string]interface{})
	}{
		{
			name: "payment pix",
			mutate: func(payload map[string]interface{}) {
				payload["intent"] = string(StructuredIntentPaymentPreference)
				payment := payload["payment"].(map[string]interface{})
				payment["payment_preference"] = "pix"
			},
		},
		{
			name: "lowercase intent",
			mutate: func(payload map[string]interface{}) {
				payload["intent"] = "greeting"
			},
		},
		{
			name: "missing required fields",
			mutate: func(payload map[string]interface{}) {
				delete(payload, "payment")
			},
		},
		{
			name: "extra field",
			mutate: func(payload map[string]interface{}) {
				payload["action"] = "tool"
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := validOpenAIGreetingPayloadMap()
			tc.mutate(payload)

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(openAIStructuredInterpreterTestResponse("resp_structured_invalid", string(marshalOpenAITestPayload(t, payload))))
			}))
			defer server.Close()

			runner := NewOpenAIStructuredInterpreterRunner(config.Config{
				OpenAIAPIKey:  "sk-test",
				OpenAIModel:   "gpt-test",
				OpenAIBaseURL: server.URL,
			})

			result, err := runner.InterpretStructuredTurn(context.Background(), openAIStructuredInterpreterTestRunInput("oi"))
			if !errors.Is(err, ErrOpenAIStructuredInterpreterInvalidOutput) {
				t.Fatalf("expected invalid output error, got %v", err)
			}
			if result.Validation.Valid {
				t.Fatalf("expected invalid validation")
			}
		})
	}
}

func TestOpenAIStructuredInterpreterRunnerHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"type":    "server_error",
				"message": "temporary failure",
			},
		})
	}))
	defer server.Close()

	runner := NewOpenAIStructuredInterpreterRunner(config.Config{
		OpenAIAPIKey:  "sk-test",
		OpenAIModel:   "gpt-test",
		OpenAIBaseURL: server.URL,
	})

	_, err := runner.InterpretStructuredTurn(context.Background(), openAIStructuredInterpreterTestRunInput("oi"))
	if !errors.Is(err, ErrOpenAIStructuredInterpreterRequestFailed) {
		t.Fatalf("expected request failed error, got %v", err)
	}
}

func TestOpenAIStructuredInterpreterRunnerEmptyOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":          "resp_structured_empty",
			"output_text": " ",
		})
	}))
	defer server.Close()

	runner := NewOpenAIStructuredInterpreterRunner(config.Config{
		OpenAIAPIKey:  "sk-test",
		OpenAIModel:   "gpt-test",
		OpenAIBaseURL: server.URL,
	})

	_, err := runner.InterpretStructuredTurn(context.Background(), openAIStructuredInterpreterTestRunInput("oi"))
	if !errors.Is(err, ErrOpenAIStructuredInterpreterEmptyOutput) {
		t.Fatalf("expected empty output error, got %v", err)
	}
}

func TestOpenAIStructuredInterpreterRunnerSendsIdempotencyKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Client-Request-Id"); got != "structured-1" {
			t.Fatalf("expected idempotency header structured-1, got %s", got)
		}
		_ = json.NewEncoder(w).Encode(openAIStructuredInterpreterTestResponse("resp_structured_idempotent", validOpenAIStructuredInterpreterOutputText(t)))
	}))
	defer server.Close()

	runner := NewOpenAIStructuredInterpreterRunner(config.Config{
		OpenAIAPIKey:  "sk-test",
		OpenAIModel:   "gpt-test",
		OpenAIBaseURL: server.URL,
	})
	input := openAIStructuredInterpreterTestRunInput("oi")
	input.IdempotencyKey = "structured-1"

	if _, err := runner.InterpretStructuredTurn(context.Background(), input); err != nil {
		t.Fatalf("interpret structured turn: %v", err)
	}
}

func assertOpenAIStructuredInterpreterRequestShape(t *testing.T, payload map[string]interface{}) {
	t.Helper()

	if payload["model"] != "gpt-test" {
		t.Fatalf("expected model gpt-test, got %#v", payload["model"])
	}
	if _, ok := payload["instructions"].(string); !ok {
		t.Fatalf("expected instructions string, got %#v", payload["instructions"])
	}
	if _, ok := payload["input"].(string); !ok {
		t.Fatalf("expected input string, got %#v", payload["input"])
	}
	if payload["store"] != false {
		t.Fatalf("expected store=false, got %#v", payload["store"])
	}
	tools, ok := payload["tools"].([]interface{})
	if !ok {
		t.Fatalf("expected tools array, got %#v", payload["tools"])
	}
	if len(tools) != 0 {
		t.Fatalf("expected empty tools array, got %#v", tools)
	}
	for _, forbidden := range []string{"conversation", "previous_response_id", "web_search", "tool_choice", "temperature"} {
		if _, ok := payload[forbidden]; ok {
			t.Fatalf("request must not contain %s: %#v", forbidden, payload[forbidden])
		}
	}

	text, ok := payload["text"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected text object, got %#v", payload["text"])
	}
	format, ok := text["format"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected text.format object, got %#v", text["format"])
	}
	if format["type"] != "json_schema" {
		t.Fatalf("expected json_schema format, got %#v", format["type"])
	}
	if format["name"] != "openai_structured_interpretation" {
		t.Fatalf("expected structured schema name, got %#v", format["name"])
	}
	if format["strict"] != true {
		t.Fatalf("expected strict=true, got %#v", format["strict"])
	}
	schema, ok := format["schema"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected schema object, got %#v", format["schema"])
	}
	if schema["additionalProperties"] != false {
		t.Fatalf("expected schema additionalProperties=false, got %#v", schema["additionalProperties"])
	}
}

func openAIStructuredInterpreterTestRunInput(currentTurn string) OpenAIStructuredInterpreterRunInput {
	local := openAIStructuredInterpretationToStructured(validOpenAIGreetingCandidate())
	return OpenAIStructuredInterpreterRunInput{
		StructuredInput: StructuredInterpreterInput{
			CurrentTurn: currentTurn,
		},
		LocalInterpretation: local,
	}
}

func validOpenAIStructuredInterpreterOutputText(t *testing.T) string {
	t.Helper()
	return string(marshalOpenAITestPayload(t, validOpenAIGreetingPayloadMap()))
}

func openAIStructuredInterpreterTestResponse(id string, outputText string) map[string]interface{} {
	return map[string]interface{}{
		"id":          id,
		"output_text": outputText,
	}
}
