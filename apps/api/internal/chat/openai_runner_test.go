package chat

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"schumacher-tur/api/internal/shared/config"
)

func TestOpenAIRunnerRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("expected path /responses, got %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Fatalf("expected bearer auth, got %s", got)
		}
		if got := r.Header.Get("X-Client-Request-Id"); got != "draft-1" {
			t.Fatalf("expected idempotency header draft-1, got %s", got)
		}

		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["model"] != "gpt-test" {
			t.Fatalf("expected model gpt-test, got %v", payload["model"])
		}
		if payload["input"] != "user" {
			t.Fatalf("expected plain text input, got %#v", payload["input"])
		}

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":          "resp_test_1",
			"output_text": "Resposta do Shabas",
		})
	}))
	defer server.Close()

	runner := NewOpenAIRunner(config.Config{
		OpenAIAPIKey: "sk-test",
		OpenAIModel:  "gpt-test",
	})
	runner.baseURL = server.URL

	result, err := runner.Run(context.Background(), RunAgentInput{
		SystemPrompt:   "system",
		UserPrompt:     "user",
		IdempotencyKey: "draft-1",
	})
	if err != nil {
		t.Fatalf("run agent: %v", err)
	}
	if result.ReplyText != "Resposta do Shabas" {
		t.Fatalf("expected reply text, got %s", result.ReplyText)
	}
	if result.ProviderResponseID != "resp_test_1" {
		t.Fatalf("expected provider response id resp_test_1, got %s", result.ProviderResponseID)
	}
}

func TestOpenAIRunnerRunIncludesPreviousResponseIDWhenConfigured(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if got := payload["previous_response_id"]; got != "resp_prev_1" {
			t.Fatalf("expected previous_response_id resp_prev_1, got %#v", got)
		}
		if _, ok := payload["conversation"]; ok {
			t.Fatalf("did not expect conversation payload when previous_response_id is available: %#v", payload["conversation"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":          "resp_test_4",
			"output_text": "Continuacao ok",
		})
	}))
	defer server.Close()

	runner := NewOpenAIRunner(config.Config{
		OpenAIAPIKey:                "sk-test",
		OpenAIModel:                 "gpt-5.4",
		ChatOpenAIContinuityEnabled: true,
	})
	runner.baseURL = server.URL

	result, err := runner.Run(context.Background(), RunAgentInput{
		Session: Session{
			Metadata: map[string]interface{}{
				"provider_response_id": "resp_prev_1",
				"provider_model":       "gpt-5.4-mini",
			},
		},
		SystemPrompt: "system",
		UserPrompt:   "user",
	})
	if err != nil {
		t.Fatalf("run agent: %v", err)
	}
	if result.ReplyText != "Continuacao ok" {
		t.Fatalf("unexpected reply text: %s", result.ReplyText)
	}
	if result.RequestPayload["previous_response_id_used"] != true {
		t.Fatalf("expected previous_response_id_used=true, got %#v", result.RequestPayload["previous_response_id_used"])
	}
}

func TestOpenAIRunnerRunRetriesWithoutPreviousResponseIDOnContinuityError(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if atomic.AddInt32(&calls, 1) == 1 {
			if got := payload["previous_response_id"]; got != "resp_prev_2" {
				t.Fatalf("expected first request to include previous_response_id, got %#v", got)
			}
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"message": "previous_response_id resp_prev_2 was not found",
				},
			})
			return
		}
		if _, ok := payload["previous_response_id"]; ok {
			t.Fatalf("expected retry to omit previous_response_id, got %#v", payload["previous_response_id"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":          "resp_test_5",
			"output_text": "retry ok",
		})
	}))
	defer server.Close()

	runner := NewOpenAIRunner(config.Config{
		OpenAIAPIKey:                "sk-test",
		OpenAIModel:                 "gpt-5.4",
		ChatOpenAIContinuityEnabled: true,
	})
	runner.baseURL = server.URL

	result, err := runner.Run(context.Background(), RunAgentInput{
		Session: Session{
			Metadata: map[string]interface{}{
				"provider_response_id": "resp_prev_2",
				"provider_model":       "gpt-5.4-mini",
			},
		},
		SystemPrompt: "system",
		UserPrompt:   "user",
	})
	if err != nil {
		t.Fatalf("run agent: %v", err)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected two requests, got %d", calls)
	}
	if result.ReplyText != "retry ok" {
		t.Fatalf("unexpected retry reply text: %s", result.ReplyText)
	}
	if result.RequestPayload["previous_response_id_used"] != true {
		t.Fatalf("expected previous_response_id_used=true after retry, got %#v", result.RequestPayload["previous_response_id_used"])
	}
	if result.RequestPayload["previous_response_id_failed"] != true {
		t.Fatalf("expected previous_response_id_failed=true after retry, got %#v", result.RequestPayload["previous_response_id_failed"])
	}
	if result.RequestPayload["retry_without_previous_response_id"] != true {
		t.Fatalf("expected retry_without_previous_response_id=true after retry, got %#v", result.RequestPayload["retry_without_previous_response_id"])
	}
}

func TestOpenAIRunnerRunWithoutPreviousResponseIDWhenContinuityDisabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if _, ok := payload["previous_response_id"]; ok {
			t.Fatalf("did not expect previous_response_id when continuity is disabled")
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":          "resp_test_6",
			"output_text": "sem continuidade",
		})
	}))
	defer server.Close()

	runner := NewOpenAIRunner(config.Config{
		OpenAIAPIKey:                "sk-test",
		OpenAIModel:                 "gpt-5.4",
		ChatOpenAIContinuityEnabled: false,
	})
	runner.baseURL = server.URL

	result, err := runner.Run(context.Background(), RunAgentInput{
		Session: Session{
			Metadata: map[string]interface{}{
				"provider_response_id": "resp_prev_3",
				"provider_model":       "gpt-5.4-mini",
			},
		},
		SystemPrompt: "system",
		UserPrompt:   "user",
	})
	if err != nil {
		t.Fatalf("run agent: %v", err)
	}
	if result.ReplyText != "sem continuidade" {
		t.Fatalf("unexpected reply text: %s", result.ReplyText)
	}
	if result.RequestPayload["previous_response_id_used"] != false {
		t.Fatalf("expected previous_response_id_used=false, got %#v", result.RequestPayload["previous_response_id_used"])
	}
}

func TestOpenAIRunnerRunIncludesImageInputWhenCurrentTurnHasMedia(t *testing.T) {
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("fake-image-binary"))
	}))
	defer imageServer.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["model"] != "gpt-vision-test" {
			t.Fatalf("expected vision model gpt-vision-test, got %v", payload["model"])
		}

		items, ok := payload["input"].([]interface{})
		if !ok || len(items) != 1 {
			t.Fatalf("expected structured input with one message, got %#v", payload["input"])
		}
		message, ok := items[0].(map[string]interface{})
		if !ok {
			t.Fatalf("expected input message map, got %#v", items[0])
		}
		content, ok := message["content"].([]interface{})
		if !ok || len(content) != 2 {
			t.Fatalf("expected text + image content, got %#v", message["content"])
		}
		image, ok := content[1].(map[string]interface{})
		expectedDataURL := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString([]byte("fake-image-binary"))
		if !ok || image["type"] != "input_image" || image["image_url"] != expectedDataURL {
			t.Fatalf("unexpected image payload: %#v", content[1])
		}

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":          "resp_test_2",
			"output_text": "Dados extraidos",
		})
	}))
	defer server.Close()

	runner := NewOpenAIRunner(config.Config{
		OpenAIAPIKey:      "sk-test",
		OpenAIModel:       "gpt-test",
		OpenAIVisionModel: "gpt-vision-test",
	})
	runner.baseURL = server.URL

	result, err := runner.Run(context.Background(), RunAgentInput{
		SystemPrompt: "system",
		UserPrompt:   "user",
		CurrentTurnMedia: []AgentMediaInput{
			{Kind: "IMAGE", URL: imageServer.URL, MimeType: "image/jpeg"},
		},
	})
	if err != nil {
		t.Fatalf("run agent: %v", err)
	}
	if result.ReplyText != "Dados extraidos" {
		t.Fatalf("expected image reply text, got %s", result.ReplyText)
	}
	if result.Model != "gpt-vision-test" {
		t.Fatalf("expected result model gpt-vision-test, got %s", result.Model)
	}
}

func TestOpenAIRunnerRunIncludesPDFInputFileWhenCurrentTurnHasMedia(t *testing.T) {
	pdfDataURL := "data:application/pdf;base64," + base64.StdEncoding.EncodeToString([]byte("%PDF-1.4 fake"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["model"] != "gpt-vision-test" {
			t.Fatalf("expected vision model gpt-vision-test, got %v", payload["model"])
		}

		items, ok := payload["input"].([]interface{})
		if !ok || len(items) != 1 {
			t.Fatalf("expected structured input with one message, got %#v", payload["input"])
		}
		message, ok := items[0].(map[string]interface{})
		if !ok {
			t.Fatalf("expected input message map, got %#v", items[0])
		}
		content, ok := message["content"].([]interface{})
		if !ok || len(content) != 2 {
			t.Fatalf("expected text + file content, got %#v", message["content"])
		}
		file, ok := content[1].(map[string]interface{})
		if !ok || file["type"] != "input_file" || file["filename"] != "rg-cliente.pdf" || file["file_data"] != pdfDataURL {
			t.Fatalf("unexpected file payload: %#v", content[1])
		}
		if _, ok := file["file_url"]; ok {
			t.Fatalf("did not expect file_url for inline PDF payload: %#v", file)
		}

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":          "resp_pdf_1",
			"output_text": `{"mode":"EXTRACTED","passengers":[],"failure_reason":""}`,
		})
	}))
	defer server.Close()

	runner := NewOpenAIRunner(config.Config{
		OpenAIAPIKey:      "sk-test",
		OpenAIModel:       "gpt-test",
		OpenAIVisionModel: "gpt-vision-test",
	})
	runner.baseURL = server.URL

	result, err := runner.Run(context.Background(), RunAgentInput{
		SystemPrompt: "system",
		UserPrompt:   "user",
		CurrentTurnMedia: []AgentMediaInput{
			{Kind: "PDF", URL: pdfDataURL, MimeType: "application/pdf", FileName: "rg-cliente.pdf"},
		},
	})
	if err != nil {
		t.Fatalf("run agent: %v", err)
	}
	if result.ProviderResponseID != "resp_pdf_1" {
		t.Fatalf("expected provider response id resp_pdf_1, got %s", result.ProviderResponseID)
	}
}

func TestOpenAIRunnerRunFallsBackToTextWhenImageRequestFails(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if atomic.AddInt32(&calls, 1) == 1 {
			if _, ok := payload["input"].([]interface{}); !ok {
				t.Fatalf("expected first request to be multimodal, got %#v", payload["input"])
			}
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": map[string]interface{}{"message": "unsupported image"}})
			return
		}
		if payload["input"] != "user" {
			t.Fatalf("expected fallback request to be text-only, got %#v", payload["input"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":          "resp_test_3",
			"output_text": "Recebi a foto do documento.",
		})
	}))
	defer server.Close()

	runner := NewOpenAIRunner(config.Config{
		OpenAIAPIKey: "sk-test",
		OpenAIModel:  "gpt-test",
	})
	runner.baseURL = server.URL

	result, err := runner.Run(context.Background(), RunAgentInput{
		SystemPrompt: "system",
		UserPrompt:   "user",
		CurrentTurnMedia: []AgentMediaInput{
			{Kind: "IMAGE", URL: "https://files.example.test/rg.jpg", MimeType: "image/jpeg"},
		},
	})
	if err != nil {
		t.Fatalf("run agent: %v", err)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected two requests after fallback, got %d", calls)
	}
	if result.ReplyText != "Recebi a foto do documento." {
		t.Fatalf("unexpected fallback reply text: %s", result.ReplyText)
	}
}

func TestCompactOpenAIErrorBodyExtractsStructuredErrorFields(t *testing.T) {
	body := []byte(`{"error":{"message":"Rate limit reached","type":"rate_limit_error","code":"rate_limit_exceeded"}}`)

	got := compactOpenAIErrorBody(body)

	want := "type=rate_limit_error code=rate_limit_exceeded message=Rate limit reached"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestOpenAIRunnerRunIncludesCompactedErrorBodyOnFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"message": "Rate limit reached",
				"type":    "rate_limit_error",
				"code":    "rate_limit_exceeded",
			},
		})
	}))
	defer server.Close()

	runner := NewOpenAIRunner(config.Config{
		OpenAIAPIKey: "sk-test",
		OpenAIModel:  "gpt-test",
	})
	runner.baseURL = server.URL

	_, err := runner.Run(context.Background(), RunAgentInput{
		SystemPrompt: "system",
		UserPrompt:   "user",
	})
	if err == nil {
		t.Fatal("expected run agent error")
	}
	if !errors.Is(err, ErrOpenAIRunFailed) {
		t.Fatalf("expected ErrOpenAIRunFailed, got %v", err)
	}
	text := err.Error()
	if !strings.Contains(text, "status 429") {
		t.Fatalf("expected status code in error, got %q", text)
	}
	if !strings.Contains(text, "type=rate_limit_error") ||
		!strings.Contains(text, "code=rate_limit_exceeded") ||
		!strings.Contains(text, "message=Rate limit reached") {
		t.Fatalf("expected compacted error body in error, got %q", text)
	}
}
