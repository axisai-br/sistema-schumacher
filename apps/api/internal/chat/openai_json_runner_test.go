package chat

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"schumacher-tur/api/internal/shared/config"
)

func TestOpenAIJSONRunnerBuildsStrictJSONSchemaPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("expected path /responses, got %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Fatalf("expected bearer auth, got %s", got)
		}
		if got := r.Header.Get("X-Client-Request-Id"); got != "json-1" {
			t.Fatalf("expected idempotency header json-1, got %s", got)
		}

		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
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
		required, ok := schema["required"].([]interface{})
		if !ok || len(required) == 0 {
			t.Fatalf("expected explicit required fields, got %#v", schema["required"])
		}

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "resp_json_1",
			"output_text": `{
				"domain":"scheduling",
				"intent":"AVAILABILITY_SEARCH",
				"action":"tool",
				"confidence":0.91,
				"missing_fields":[],
				"safe_next_step":"call availability_search",
				"selected_option_index":null,
				"availability_input":{"tool_name":"availability_search","origin":"Chapeco/SC","destination":"Santa Ines/MA","package_name":"Pacote p/ Maranhão","trip_date":"","qty":1,"limit":5},
				"payment_input":null,
				"booking_input":null
			}`,
		})
	}))
	defer server.Close()

	runner := NewOpenAIJSONRunner(config.Config{
		OpenAIAPIKey: "sk-test",
		OpenAIModel:  "gpt-test",
	})
	runner.baseURL = server.URL

	decision, result, err := runner.RunIntentDecision(context.Background(), RunJSONDecisionInput{
		SystemPrompt:   "system",
		CompactInput:   `{"current_turn":"Santa Inês"}`,
		IdempotencyKey: "json-1",
	})
	if err != nil {
		t.Fatalf("run json decision: %v", err)
	}
	if decision.Intent != string(IntentAvailabilitySearch) {
		t.Fatalf("expected availability intent, got %s", decision.Intent)
	}
	if result.ProviderResponseID != "resp_json_1" {
		t.Fatalf("expected provider response id, got %s", result.ProviderResponseID)
	}
}

func TestOpenAIJSONRunnerRejectsInvalidJSONOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":          "resp_json_invalid",
			"output_text": "not-json",
		})
	}))
	defer server.Close()

	runner := NewOpenAIJSONRunner(config.Config{
		OpenAIAPIKey: "sk-test",
		OpenAIModel:  "gpt-test",
	})
	runner.baseURL = server.URL

	var decision IntentDecisionJSON
	_, err := runner.Run(context.Background(), RunJSONDecisionInput{
		SystemPrompt: "system",
		CompactInput: "{}",
		SchemaName:   "intent_decision",
		Schema:       intentDecisionJSONSchema(),
	}, &decision)
	if err == nil {
		t.Fatal("expected invalid JSON output error")
	}
	if !errors.Is(err, ErrOpenAIJSONInvalidOutput) {
		t.Fatalf("expected ErrOpenAIJSONInvalidOutput, got %v", err)
	}
}
