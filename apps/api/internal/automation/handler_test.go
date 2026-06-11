package automation

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"schumacher-tur/api/internal/bookings"
	"schumacher-tur/api/internal/chat"
	"schumacher-tur/api/internal/payments"
	"schumacher-tur/api/internal/shared/config"
)

func TestHandleEvolutionMessagesAcceptsDirectPayload(t *testing.T) {
	chatSvc := &fakeChatIngestor{}
	handler := NewHandler(NewService(&fakeAutomationStore{}, chatSvc, config.Config{}))

	r := chi.NewRouter()
	handler.RegisterWebhooks(r)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/evolution/messages", bytes.NewBufferString(`{
		"event":"messages.upsert",
		"instance":"belle",
		"data":{
			"key":{"remoteJid":"554988709047@s.whatsapp.net","fromMe":false,"id":"MSG-1"},
			"pushName":"messias",
			"message":{"conversation":"06645648109"},
			"messageType":"conversation",
			"messageTimestamp":1772544357,
			"source":"ios"
		},
		"server_url":"https://evo.bellasoftware.com.br"
	}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rec.Code)
	}
	if chatSvc.lastInput.ContactKey != "554988709047@s.whatsapp.net" {
		t.Fatalf("unexpected contact key: %s", chatSvc.lastInput.ContactKey)
	}
	if chatSvc.lastInput.Message.Body != "06645648109" {
		t.Fatalf("unexpected message body: %s", chatSvc.lastInput.Message.Body)
	}
	if chatSvc.lastInput.Message.Kind != "TEXT" {
		t.Fatalf("unexpected kind: %s", chatSvc.lastInput.Message.Kind)
	}
	if chatSvc.lastInput.Message.ProviderMessageID != "MSG-1" {
		t.Fatalf("unexpected provider message id: %s", chatSvc.lastInput.Message.ProviderMessageID)
	}
}

func TestHandleEvolutionMessagesTranscribesAudioAndNormalizesRouteText(t *testing.T) {
	audioBytes := []byte("fake-audio-bytes")
	audioBase64 := base64.StdEncoding.EncodeToString(audioBytes)

	mediaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/getBase64FromMediaMessage/belle" {
			t.Fatalf("unexpected media path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"base64":"` + audioBase64 + `"}`))
	}))
	defer mediaServer.Close()

	openAIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/audio/transcriptions" {
			t.Fatalf("unexpected openai path: %s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			t.Fatalf("parse multipart form: %v", err)
		}
		if got := r.FormValue("model"); got != "gpt-4o-mini-transcribe" {
			t.Fatalf("unexpected model: %s", got)
		}
		if got := r.FormValue("language"); got != "pt" {
			t.Fatalf("unexpected language: %s", got)
		}
		if got := r.FormValue("prompt"); !strings.Contains(got, "Fraiburgo") || !strings.Contains(got, "Santa Inês") || !strings.Contains(got, "perguntas de reserva") || !strings.Contains(got, "só pra mim") || !strings.Contains(got, "a passagem é só pra mim") || !strings.Contains(got, "não tem criança") {
			t.Fatalf("unexpected prompt: %s", got)
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("form file: %v", err)
		}
		defer file.Close()
		if !strings.HasSuffix(header.Filename, ".ogg") {
			t.Fatalf("unexpected file extension: %s", header.Filename)
		}
		if body, err := io.ReadAll(file); err != nil {
			t.Fatalf("read audio file: %v", err)
		} else if len(body) == 0 {
			t.Fatal("expected audio file contents")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"e passagem para Freiburg."}`))
	}))
	defer openAIServer.Close()

	chatSvc := &fakeChatIngestor{}
	handler := NewHandler(NewService(&fakeAutomationStore{}, chatSvc, config.Config{
		EvolutionBaseURL:         mediaServer.URL,
		EvolutionAPIKey:          "secret",
		EvolutionInstance:        "belle",
		OpenAIAPIKey:             "sk-test",
		OpenAIBaseURL:            openAIServer.URL,
		OpenAITranscriptionModel: "gpt-4o-mini-transcribe",
	}))

	r := chi.NewRouter()
	handler.RegisterWebhooks(r)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/evolution/messages", bytes.NewBufferString(`{
		"event":"messages.upsert",
		"data":{
			"key":{"remoteJid":"554998208115@s.whatsapp.net","fromMe":false,"id":"MSG-AUDIO-1"},
			"pushName":"cliente",
			"message":{"audioMessage":{"mimetype":"audio/ogg; codecs=opus","seconds":12,"ptt":true,"url":"https://files.example.test/audio.ogg"}},
			"messageType":"audioMessage"
		}
	}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusAccepted, rec.Code, rec.Body.String())
	}
	if chatSvc.lastInput.Message.Kind != "TEXT" {
		t.Fatalf("expected transcribed audio to be stored as TEXT, got %s", chatSvc.lastInput.Message.Kind)
	}
	if chatSvc.lastInput.Message.Body != "passagem para Fraiburgo." {
		t.Fatalf("unexpected transcribed body: %s", chatSvc.lastInput.Message.Body)
	}
	if chatSvc.updateCalls != 1 {
		t.Fatalf("expected one update call, got %d", chatSvc.updateCalls)
	}
	if chatSvc.lastUpdateInput.MessageID == "" {
		t.Fatal("expected message update to target the persisted message")
	}
	if chatSvc.lastUpdateInput.Body != "passagem para Fraiburgo." {
		t.Fatalf("unexpected update body: %s", chatSvc.lastUpdateInput.Body)
	}
	if got := strings.TrimSpace(asString(chatSvc.lastUpdateInput.NormalizedPayload["current_turn_body"])); got != "" {
		t.Fatalf("expected update current_turn_body to stay empty, got %s", got)
	}
	if got := strings.TrimSpace(asString(chatSvc.lastUpdateInput.ProcessingStatus)); got != "READY_FOR_AUTOMATION" {
		t.Fatalf("unexpected update processing status: %s", got)
	}
	if got := strings.TrimSpace(asString(chatSvc.lastInput.Message.NormalizedPayload["transcription_status"])); got != "COMPLETED" {
		t.Fatalf("unexpected transcription status: %s", got)
	}
	if got := strings.TrimSpace(asString(chatSvc.lastInput.Message.NormalizedPayload["transcription_text"])); got != "passagem para Fraiburgo." {
		t.Fatalf("unexpected transcription text: %s", got)
	}
	if got := strings.TrimSpace(asString(chatSvc.lastInput.Message.NormalizedPayload["current_turn_body"])); got != "" {
		t.Fatalf("expected inbound normalized payload current_turn_body to stay empty, got %s", got)
	}
	if got := strings.TrimSpace(asString(chatSvc.lastInput.Message.NormalizedPayload["transcription_model"])); got != "gpt-4o-mini-transcribe" {
		t.Fatalf("unexpected transcription model: %s", got)
	}
	if got := strings.TrimSpace(asString(chatSvc.lastInput.Message.NormalizedPayload["audio_mimetype"])); got != "audio/ogg" {
		t.Fatalf("unexpected audio mimetype: %s", got)
	}
	if got := chatSvc.lastInput.Message.NormalizedPayload["audio_data_url"]; got == nil {
		t.Fatal("expected audio_data_url to be present")
	}
	if got := strings.TrimSpace(asString(chatSvc.lastInput.Message.NormalizedPayload["media_message_id"])); got != "MSG-AUDIO-1" {
		t.Fatalf("unexpected media message id: %s", got)
	}
}

func TestHandleEvolutionMessagesMarksAudioFetchFailureAsFailed(t *testing.T) {
	mediaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "media unavailable", http.StatusBadGateway)
	}))
	defer mediaServer.Close()

	chatSvc := &fakeChatIngestor{}
	handler := NewHandler(NewService(&fakeAutomationStore{}, chatSvc, config.Config{
		EvolutionBaseURL:  mediaServer.URL,
		EvolutionAPIKey:   "secret",
		EvolutionInstance: "belle",
		OpenAIAPIKey:      "sk-test",
	}))

	r := chi.NewRouter()
	handler.RegisterWebhooks(r)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/evolution/messages", bytes.NewBufferString(`{
		"event":"messages.upsert",
		"data":{
			"key":{"remoteJid":"554998208115@s.whatsapp.net","fromMe":false,"id":"MSG-AUDIO-2"},
			"message":{"audioMessage":{"mimetype":"audio/ogg","seconds":8,"ptt":true}},
			"messageType":"audioMessage"
		}
	}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusAccepted, rec.Code, rec.Body.String())
	}
	if chatSvc.lastInput.Message.Kind != "AUDIO" {
		t.Fatalf("expected failed audio to remain AUDIO, got %s", chatSvc.lastInput.Message.Kind)
	}
	if got := strings.TrimSpace(asString(chatSvc.lastInput.Message.NormalizedPayload["transcription_status"])); got != "FAILED" {
		t.Fatalf("unexpected transcription status: %s", got)
	}
	if got := strings.TrimSpace(asString(chatSvc.lastInput.Message.NormalizedPayload["transcription_error"])); got == "" {
		t.Fatal("expected transcription_error to be set")
	}
}

func TestHandleEvolutionMessagesMarksOpenAITranscriptionFailureAsFailed(t *testing.T) {
	mediaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"base64":"` + base64.StdEncoding.EncodeToString([]byte("fake-audio-bytes")) + `"}`))
	}))
	defer mediaServer.Close()

	openAIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"quota exceeded"}}`, http.StatusBadGateway)
	}))
	defer openAIServer.Close()

	chatSvc := &fakeChatIngestor{}
	handler := NewHandler(NewService(&fakeAutomationStore{}, chatSvc, config.Config{
		EvolutionBaseURL:         mediaServer.URL,
		EvolutionAPIKey:          "secret",
		EvolutionInstance:        "belle",
		OpenAIAPIKey:             "sk-test",
		OpenAIBaseURL:            openAIServer.URL,
		OpenAITranscriptionModel: "gpt-4o-mini-transcribe",
	}))

	r := chi.NewRouter()
	handler.RegisterWebhooks(r)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/evolution/messages", bytes.NewBufferString(`{
		"event":"messages.upsert",
		"data":{
			"key":{"remoteJid":"554998208115@s.whatsapp.net","fromMe":false,"id":"MSG-AUDIO-3"},
			"message":{"audioMessage":{"mimetype":"audio/ogg","seconds":11,"ptt":true}},
			"messageType":"audioMessage"
		}
	}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusAccepted, rec.Code, rec.Body.String())
	}
	if chatSvc.lastInput.Message.Kind != "AUDIO" {
		t.Fatalf("expected failed audio to remain AUDIO, got %s", chatSvc.lastInput.Message.Kind)
	}
	if got := strings.TrimSpace(asString(chatSvc.lastInput.Message.NormalizedPayload["audio_data_url"])); got == "" {
		t.Fatal("expected audio_data_url to be stored when fetch succeeds")
	}
	if got := strings.TrimSpace(asString(chatSvc.lastInput.Message.NormalizedPayload["transcription_status"])); got != "FAILED" {
		t.Fatalf("unexpected transcription status: %s", got)
	}
	if got := strings.TrimSpace(asString(chatSvc.lastInput.Message.NormalizedPayload["transcription_error"])); got == "" {
		t.Fatal("expected transcription_error to be set")
	}
}

func TestHandleEvolutionMessagesRejectsMissingWebhookSecret(t *testing.T) {
	chatSvc := &fakeChatIngestor{}
	handler := NewHandler(NewService(&fakeAutomationStore{}, chatSvc, config.Config{
		EvolutionWebhookSecret: "cutover-secret",
	}))

	r := chi.NewRouter()
	handler.RegisterWebhooks(r)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/evolution/messages", bytes.NewBufferString(`{
		"event":"messages.upsert",
		"data":{
			"key":{"remoteJid":"554988709047@s.whatsapp.net","fromMe":false,"id":"MSG-SEC-1"},
			"message":{"conversation":"oi"},
			"messageType":"conversation"
		}
	}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
	if chatSvc.calls != 0 {
		t.Fatalf("expected chat ingest not to be called, got %d", chatSvc.calls)
	}
}

func TestHandleEvolutionMessagesAcceptsWebhookSecretHeader(t *testing.T) {
	chatSvc := &fakeChatIngestor{}
	handler := NewHandler(NewService(&fakeAutomationStore{}, chatSvc, config.Config{
		EvolutionWebhookSecret: "cutover-secret",
	}))

	r := chi.NewRouter()
	handler.RegisterWebhooks(r)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/evolution/messages", bytes.NewBufferString(`{
		"event":"messages.upsert",
		"data":{
			"key":{"remoteJid":"554988709047@s.whatsapp.net","fromMe":false,"id":"MSG-SEC-2"},
			"message":{"conversation":"oi"},
			"messageType":"conversation"
		}
	}`))
	req.Header.Set("X-Evolution-Webhook-Secret", "cutover-secret")
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rec.Code)
	}
	if chatSvc.calls != 1 {
		t.Fatalf("expected one chat ingest call, got %d", chatSvc.calls)
	}
}

func TestHandleEvolutionMessagesEndpointDispatchesStatusEvent(t *testing.T) {
	store := &fakeAutomationStore{
		result: RecordEvolutionStatusResult{MatchedChatMessages: 1, MatchedOutboundMessages: 1},
	}
	handler := NewHandler(NewService(store, &fakeChatIngestor{}, config.Config{}))

	r := chi.NewRouter()
	handler.RegisterWebhooks(r)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/evolution/messages", bytes.NewBufferString(`{
		"event":"messages.update",
		"data":{
			"key":{"id":"MSG-MULTI-STATUS-1"},
			"status":"DELIVERED",
			"messageType":"conversation",
			"messageTimestamp":1772544357
		}
	}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rec.Code)
	}
	if store.lastStatus.ProviderMessageID != "MSG-MULTI-STATUS-1" {
		t.Fatalf("unexpected provider message id: %s", store.lastStatus.ProviderMessageID)
	}
	if store.lastStatus.ProviderStatus != "DELIVERED" {
		t.Fatalf("unexpected provider status: %s", store.lastStatus.ProviderStatus)
	}
}

func TestHandleEvolutionMessagesEndpointDispatchesPresenceEvent(t *testing.T) {
	chatSvc := &fakeChatIngestor{}
	handler := NewHandler(NewService(&fakeAutomationStore{}, chatSvc, config.Config{}))

	r := chi.NewRouter()
	handler.RegisterWebhooks(r)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/evolution/messages", bytes.NewBufferString(`{
		"event":"presence.update",
		"data":{
			"id":"554998208115@s.whatsapp.net",
			"presences":{
				"554998208115@s.whatsapp.net":{"lastKnownPresence":"typing"}
			}
		}
	}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rec.Code)
	}
	if chatSvc.presenceCalls != 1 {
		t.Fatalf("expected one presence call, got %d", chatSvc.presenceCalls)
	}
	if chatSvc.lastPresenceInput.PresenceStatus != "TYPING" {
		t.Fatalf("unexpected presence status: %s", chatSvc.lastPresenceInput.PresenceStatus)
	}
}

func TestHandleEvolutionEventAliasesAcceptWebhookByEventsPaths(t *testing.T) {
	store := &fakeAutomationStore{
		result: RecordEvolutionStatusResult{MatchedChatMessages: 1, MatchedOutboundMessages: 1},
	}
	handler := NewHandler(NewService(store, &fakeChatIngestor{}, config.Config{}))

	r := chi.NewRouter()
	handler.RegisterWebhooks(r)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/evolution/messages-update", bytes.NewBufferString(`{
		"event":"messages.update",
		"data":{
			"key":{"id":"MSG-ALIAS-1"},
			"status":"READ",
			"messageType":"conversation",
			"messageTimestamp":1772544357
		}
	}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rec.Code)
	}
	if store.lastStatus.ProviderMessageID != "MSG-ALIAS-1" {
		t.Fatalf("unexpected provider message id: %s", store.lastStatus.ProviderMessageID)
	}
}

func TestHandleEvolutionMessagesAcceptsN8NEnvelope(t *testing.T) {
	chatSvc := &fakeChatIngestor{}
	handler := NewHandler(NewService(&fakeAutomationStore{}, chatSvc, config.Config{}))

	r := chi.NewRouter()
	handler.RegisterWebhooks(r)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/evolution/messages", bytes.NewBufferString(`{
		"body":{
			"event":"messages.upsert",
			"instance":"belle",
			"data":{
				"key":{"remoteJid":"554998208115@s.whatsapp.net","fromMe":false,"id":"MSG-2"},
				"pushName":"Cliente",
				"message":{"extendedTextMessage":{"text":"quero reservar"}},
				"messageType":"extendedTextMessage",
				"messageTimestamp":1772544357
			}
		}
	}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rec.Code)
	}
	if chatSvc.lastInput.Message.Body != "quero reservar" {
		t.Fatalf("unexpected extracted body: %s", chatSvc.lastInput.Message.Body)
	}
}

func TestHandleEvolutionMessagesSkipsFromMe(t *testing.T) {
	chatSvc := &fakeChatIngestor{}
	handler := NewHandler(NewService(&fakeAutomationStore{}, chatSvc, config.Config{}))

	r := chi.NewRouter()
	handler.RegisterWebhooks(r)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/evolution/messages", bytes.NewBufferString(`{
		"event":"messages.upsert",
		"data":{
			"key":{"remoteJid":"554998208115@s.whatsapp.net","fromMe":true,"id":"MSG-3"},
			"messageType":"conversation"
		}
	}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rec.Code)
	}
	if chatSvc.calls != 0 {
		t.Fatalf("expected chat ingest not to be called, got %d", chatSvc.calls)
	}
}

func TestUnimplementedAutomationRoutesRemainUnimplemented(t *testing.T) {
	handler := NewHandler(NewService(&fakeAutomationStore{}, &fakeChatIngestor{}, config.Config{}))

	r := chi.NewRouter()
	handler.RegisterWebhooks(r)
	handler.RegisterRoutes(r)

	tests := []struct {
		method string
		path   string
	}{
		{method: http.MethodPost, path: "/automation/jobs/sheet-sync/run"},
	}

	for _, tc := range tests {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()

		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotImplemented {
			t.Fatalf("expected status %d, got %d for %s", http.StatusNotImplemented, rec.Code, tc.path)
		}
	}
}

func TestRunBookingsExpireRejectsInvalidInput(t *testing.T) {
	handler := NewHandler(NewService(&fakeAutomationStore{}, &fakeChatIngestor{}, config.Config{}, &fakeBookingExpirationSource{}))

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/automation/jobs/bookings-expire/run", bytes.NewBufferString(`{"limit":-1}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestRunChatBufferFlushRejectsInvalidInput(t *testing.T) {
	handler := NewHandler(NewService(&fakeAutomationStore{}, &fakeChatIngestor{}, config.Config{}))

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/automation/jobs/chat-buffer-flush/run", bytes.NewBufferString(`{"limit":-1}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestRunChatBufferFlushProcessesDueBuffers(t *testing.T) {
	store := &fakeAutomationStore{}
	now := time.Now().UTC()
	dueAt := now.Add(-2 * time.Minute)
	futureAt := now.Add(2 * time.Minute)
	chatSvc := &fakeChatIngestor{
		sessions: []chat.Session{
			{
				ID:            "session-due",
				Channel:       "WHATSAPP",
				ContactKey:    "554998887766@s.whatsapp.net",
				Status:        "ACTIVE",
				HandoffStatus: "BOT",
				Metadata: map[string]interface{}{
					"buffer": map[string]interface{}{
						"status":        "PENDING",
						"pending_until": dueAt.Format(time.RFC3339Nano),
					},
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
			{
				ID:            "session-future",
				Channel:       "WHATSAPP",
				ContactKey:    "554997776655@s.whatsapp.net",
				Status:        "ACTIVE",
				HandoffStatus: "BOT",
				Metadata: map[string]interface{}{
					"buffer": map[string]interface{}{
						"status":        "PENDING",
						"pending_until": futureAt.Format(time.RFC3339Nano),
					},
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
			{
				ID:            "session-idle",
				Channel:       "WHATSAPP",
				ContactKey:    "554996665544@s.whatsapp.net",
				Status:        "ACTIVE",
				HandoffStatus: "BOT",
				Metadata: map[string]interface{}{
					"buffer": map[string]interface{}{
						"status": "IDLE",
					},
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
		reprocessResults: map[string]chat.ReprocessResult{
			"session-due": {
				Session: chat.Session{
					ID:            "session-due",
					Channel:       "WHATSAPP",
					ContactKey:    "554998887766@s.whatsapp.net",
					Status:        "ACTIVE",
					HandoffStatus: "BOT",
					CreatedAt:     now,
					UpdatedAt:     now,
				},
				Status: "accepted",
				Reason: "draft_generated",
				Draft: &chat.Message{
					ID:               "draft-1",
					SessionID:        "session-due",
					Direction:        "OUTBOUND",
					Kind:             "TEXT",
					ProcessingStatus: "AUTOMATION_DRAFT",
					ReceivedAt:       now,
					CreatedAt:        now,
				},
			},
		},
	}
	handler := NewHandler(NewService(store, chatSvc, config.Config{}))

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/automation/jobs/chat-buffer-flush/run", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if chatSvc.listCalls != 1 {
		t.Fatalf("expected list sessions once, got %d", chatSvc.listCalls)
	}
	if chatSvc.lastListFilter.Channel != "WHATSAPP" {
		t.Fatalf("expected channel WHATSAPP, got %s", chatSvc.lastListFilter.Channel)
	}
	if chatSvc.lastListFilter.Status != "ACTIVE" {
		t.Fatalf("expected status ACTIVE, got %s", chatSvc.lastListFilter.Status)
	}
	if chatSvc.lastListFilter.HandoffStatus != "BOT" {
		t.Fatalf("expected handoff BOT, got %s", chatSvc.lastListFilter.HandoffStatus)
	}
	if chatSvc.reprocessCalls != 1 {
		t.Fatalf("expected one reprocess call, got %d", chatSvc.reprocessCalls)
	}
	if chatSvc.reprocessInputs[0].SessionID != "session-due" {
		t.Fatalf("expected due session to be reprocessed, got %s", chatSvc.reprocessInputs[0].SessionID)
	}
	if chatSvc.reprocessInputs[0].Trigger != "SYSTEM_BUFFER_FLUSH" {
		t.Fatalf("unexpected trigger: %s", chatSvc.reprocessInputs[0].Trigger)
	}
	if len(store.jobRuns) != 1 {
		t.Fatalf("expected one job run, got %d", len(store.jobRuns))
	}
	if store.jobRuns[0].Status != "COMPLETED" {
		t.Fatalf("expected COMPLETED job run, got %s", store.jobRuns[0].Status)
	}

	var out RunChatBufferFlushResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if out.Status != "processed" {
		t.Fatalf("expected processed status, got %s", out.Status)
	}
	if out.Reason != "buffers_flushed" {
		t.Fatalf("expected buffers_flushed, got %s", out.Reason)
	}
	if out.CheckedCount != 3 {
		t.Fatalf("expected checked_count 3, got %d", out.CheckedCount)
	}
	if out.DueCount != 1 {
		t.Fatalf("expected due_count 1, got %d", out.DueCount)
	}
	if out.FlushedCount != 1 {
		t.Fatalf("expected flushed_count 1, got %d", out.FlushedCount)
	}
	if out.FailedCount != 0 {
		t.Fatalf("expected failed_count 0, got %d", out.FailedCount)
	}
	if len(out.FlushedSessions) != 1 {
		t.Fatalf("expected one flushed session, got %d", len(out.FlushedSessions))
	}
	if out.FlushedSessions[0].DraftMessageID != "draft-1" {
		t.Fatalf("expected draft-1, got %s", out.FlushedSessions[0].DraftMessageID)
	}
}

func TestRunChatBufferFlushSkipsWhenNoDueBuffers(t *testing.T) {
	store := &fakeAutomationStore{}
	now := time.Now().UTC()
	chatSvc := &fakeChatIngestor{
		sessions: []chat.Session{
			{
				ID:            "session-future",
				Channel:       "WHATSAPP",
				ContactKey:    "554997776655@s.whatsapp.net",
				Status:        "ACTIVE",
				HandoffStatus: "BOT",
				Metadata: map[string]interface{}{
					"buffer": map[string]interface{}{
						"status":        "PENDING",
						"pending_until": now.Add(5 * time.Minute).Format(time.RFC3339Nano),
					},
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
	}
	handler := NewHandler(NewService(store, chatSvc, config.Config{}))

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/automation/jobs/chat-buffer-flush/run", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if chatSvc.reprocessCalls != 0 {
		t.Fatalf("expected no reprocess calls, got %d", chatSvc.reprocessCalls)
	}
	if len(store.jobRuns) != 1 {
		t.Fatalf("expected one job run, got %d", len(store.jobRuns))
	}
	if store.jobRuns[0].Status != "SKIPPED" {
		t.Fatalf("expected SKIPPED job run, got %s", store.jobRuns[0].Status)
	}

	var out RunChatBufferFlushResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if out.Status != "skipped" {
		t.Fatalf("expected skipped status, got %s", out.Status)
	}
	if out.Reason != "no_due_buffers" {
		t.Fatalf("expected no_due_buffers, got %s", out.Reason)
	}
}

func TestStartChatBufferFlushLoopRunsSystemCycleWhenEnabled(t *testing.T) {
	store := &fakeAutomationStore{}
	now := time.Now().UTC()
	chatSvc := &fakeChatIngestor{
		sessions: []chat.Session{
			{
				ID:            "session-due",
				Channel:       "WHATSAPP",
				ContactKey:    "554998887766@s.whatsapp.net",
				Status:        "ACTIVE",
				HandoffStatus: "BOT",
				Metadata: map[string]interface{}{
					"buffer": map[string]interface{}{
						"status":        "PENDING",
						"pending_until": now.Add(-1 * time.Minute).Format(time.RFC3339Nano),
					},
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
		reprocessResults: map[string]chat.ReprocessResult{
			"session-due": {
				Session: chat.Session{
					ID:            "session-due",
					Channel:       "WHATSAPP",
					ContactKey:    "554998887766@s.whatsapp.net",
					Status:        "ACTIVE",
					HandoffStatus: "BOT",
					CreatedAt:     now,
					UpdatedAt:     now,
				},
				Status: "accepted",
				Reason: "draft_generated",
			},
		},
	}
	svc := NewService(store, chatSvc, config.Config{})
	logger := &fakeChatBufferFlushLogger{ch: make(chan struct{}, 1)}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	StartChatBufferFlushLoop(ctx, svc, config.Config{
		ChatBufferAutoFlushEnabled:         true,
		ChatBufferAutoFlushIntervalSeconds: 60,
		ChatBufferAutoFlushLimit:           10,
	}, logger)

	select {
	case <-logger.ch:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected automatic chat-buffer-flush cycle to run")
	}

	cancel()

	if len(store.jobRuns) != 1 {
		t.Fatalf("expected one system job run, got %d", len(store.jobRuns))
	}
	if store.jobRuns[0].TriggerSource != "SYSTEM" {
		t.Fatalf("expected SYSTEM trigger source, got %s", store.jobRuns[0].TriggerSource)
	}
	if chatSvc.reprocessCalls != 1 {
		t.Fatalf("expected one reprocess call, got %d", chatSvc.reprocessCalls)
	}
}

func TestStartChatBufferFlushLoopDoesNothingWhenDisabled(t *testing.T) {
	store := &fakeAutomationStore{}
	chatSvc := &fakeChatIngestor{}
	svc := NewService(store, chatSvc, config.Config{})
	logger := &fakeChatBufferFlushLogger{ch: make(chan struct{}, 1)}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	StartChatBufferFlushLoop(ctx, svc, config.Config{
		ChatBufferAutoFlushEnabled: false,
	}, logger)

	select {
	case <-logger.ch:
		t.Fatal("did not expect automatic chat-buffer-flush cycle when disabled")
	case <-time.After(100 * time.Millisecond):
	}

	if len(store.jobRuns) != 0 {
		t.Fatalf("expected no job runs, got %d", len(store.jobRuns))
	}
	if chatSvc.reprocessCalls != 0 {
		t.Fatalf("expected no reprocess calls, got %d", chatSvc.reprocessCalls)
	}
}

func TestLogChatBufferFlushSessionResultsIncludesFailureAndSuccessDiagnostics(t *testing.T) {
	logger := &fakeChatBufferFlushLogger{}

	logChatBufferFlushSessionResults(logger, RunChatBufferFlushResult{
		JobRunID: "job-123",
		FlushedSessions: []ChatBufferFlushSessionResult{
			{
				SessionID:  "session-failed",
				ContactKey: "554998887766@s.whatsapp.net",
				Status:     "failed",
				Reason:     "reprocess_failed",
				ErrorText:  "chat agent run failed: upstream error",
			},
			{
				SessionID:      "session-ok",
				ContactKey:     "554991112222@s.whatsapp.net",
				Status:         "accepted",
				Reason:         "draft_generated",
				DraftMessageID: "draft-123",
				ToolCallCount:  2,
			},
		},
	})

	if len(logger.logs) != 2 {
		t.Fatalf("expected two session logs, got %d", len(logger.logs))
	}
	if !strings.Contains(logger.logs[0], "status=failed") ||
		!strings.Contains(logger.logs[0], "session_id=session-failed") ||
		!strings.Contains(logger.logs[0], "contact_key=554998887766@s.whatsapp.net") ||
		!strings.Contains(logger.logs[0], "trigger=SYSTEM_BUFFER_FLUSH") ||
		!strings.Contains(logger.logs[0], "job_run_id=job-123") ||
		!strings.Contains(logger.logs[0], "error=chat agent run failed: upstream error") {
		t.Fatalf("failure log missing diagnostics: %s", logger.logs[0])
	}
	if !strings.Contains(logger.logs[1], "status=accepted") ||
		!strings.Contains(logger.logs[1], "session_id=session-ok") ||
		!strings.Contains(logger.logs[1], "reason=draft_generated") ||
		!strings.Contains(logger.logs[1], "has_draft=true") ||
		!strings.Contains(logger.logs[1], "tool_call_count=2") {
		t.Fatalf("success log missing diagnostics: %s", logger.logs[1])
	}
}

func TestRunChatAutoSendRetryRejectsInvalidInput(t *testing.T) {
	handler := NewHandler(NewService(&fakeAutomationStore{}, &fakeChatIngestor{}, config.Config{}))

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/automation/jobs/chat-auto-send-retry/run", bytes.NewBufferString(`{"limit":-1}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestRunChatAutoSendRetryProcessesDueDrafts(t *testing.T) {
	now := time.Now().UTC()
	store := &fakeAutomationStore{}
	chatSvc := &fakeChatIngestor{
		sessions: []chat.Session{
			{
				ID:            "session-due",
				Channel:       "WHATSAPP",
				ContactKey:    "554998887766@s.whatsapp.net",
				Status:        "ACTIVE",
				HandoffStatus: "BOT",
				Metadata: map[string]interface{}{
					"agent": map[string]interface{}{
						"auto_send_status":          "AUTO_SEND_RETRY_PENDING",
						"auto_send_last_attempt_at": now.Add(-2 * time.Minute).Format(time.RFC3339Nano),
					},
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
			{
				ID:            "session-cooldown",
				Channel:       "WHATSAPP",
				ContactKey:    "554997776655@s.whatsapp.net",
				Status:        "ACTIVE",
				HandoffStatus: "BOT",
				Metadata: map[string]interface{}{
					"agent": map[string]interface{}{
						"auto_send_status":          "AUTO_SEND_RETRY_PENDING",
						"auto_send_last_attempt_at": now.Add(-5 * time.Second).Format(time.RFC3339Nano),
					},
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
		retryDraftResults: map[string]chat.RetryDraftAutoSendResult{
			"session-due": {
				Session: chat.Session{
					ID:            "session-due",
					Channel:       "WHATSAPP",
					ContactKey:    "554998887766@s.whatsapp.net",
					Status:        "ACTIVE",
					HandoffStatus: "BOT",
					CreatedAt:     now,
					UpdatedAt:     now,
				},
				Status: "accepted",
				Reason: "draft_auto_send_retried",
				Draft: &chat.Message{
					ID:               "draft-due",
					SessionID:        "session-due",
					ProcessingStatus: "AUTOMATION_SENT",
				},
				Outbound: &chat.ReplyOutbound{
					ID:        "outbound-due",
					SessionID: "session-due",
				},
			},
		},
	}
	handler := NewHandler(NewService(store, chatSvc, config.Config{
		ChatAutoSendRetryLimit:           20,
		ChatAutoSendRetryCooldownSeconds: 30,
	}))

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/automation/jobs/chat-auto-send-retry/run", bytes.NewBufferString(`{"cooldown_seconds":30}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if chatSvc.listCalls != 1 {
		t.Fatalf("expected one list call, got %d", chatSvc.listCalls)
	}
	if chatSvc.lastListFilter.DraftAutoSendStatus != "AUTO_SEND_RETRY_PENDING" {
		t.Fatalf("expected retry-pending filter, got %s", chatSvc.lastListFilter.DraftAutoSendStatus)
	}
	if chatSvc.retryDraftCalls != 1 {
		t.Fatalf("expected one retry call, got %d", chatSvc.retryDraftCalls)
	}
	if chatSvc.retryDraftInputs[0].SessionID != "session-due" {
		t.Fatalf("expected retry for session-due, got %s", chatSvc.retryDraftInputs[0].SessionID)
	}
	if chatSvc.retryDraftInputs[0].RequestedBy != "system:auto-send-retry-loop" {
		t.Fatalf("expected system requester, got %s", chatSvc.retryDraftInputs[0].RequestedBy)
	}
	if len(store.jobRuns) != 1 {
		t.Fatalf("expected one job run, got %d", len(store.jobRuns))
	}
	if store.jobRuns[0].Status != "COMPLETED" {
		t.Fatalf("expected COMPLETED job run, got %s", store.jobRuns[0].Status)
	}

	var out RunChatAutoSendRetryResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if out.Status != "processed" {
		t.Fatalf("expected processed status, got %s", out.Status)
	}
	if out.DueCount != 1 {
		t.Fatalf("expected due_count 1, got %d", out.DueCount)
	}
	if out.RetriedCount != 1 {
		t.Fatalf("expected retried_count 1, got %d", out.RetriedCount)
	}
	if out.FailedCount != 0 {
		t.Fatalf("expected failed_count 0, got %d", out.FailedCount)
	}
}

func TestRunChatAutoSendRetrySkipsWhenNothingIsDue(t *testing.T) {
	now := time.Now().UTC()
	store := &fakeAutomationStore{}
	chatSvc := &fakeChatIngestor{
		sessions: []chat.Session{
			{
				ID:            "session-cooldown",
				Channel:       "WHATSAPP",
				ContactKey:    "554997776655@s.whatsapp.net",
				Status:        "ACTIVE",
				HandoffStatus: "BOT",
				Metadata: map[string]interface{}{
					"agent": map[string]interface{}{
						"auto_send_status":          "AUTO_SEND_RETRY_PENDING",
						"auto_send_last_attempt_at": now.Add(-5 * time.Second).Format(time.RFC3339Nano),
					},
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
	}
	handler := NewHandler(NewService(store, chatSvc, config.Config{
		ChatAutoSendRetryCooldownSeconds: 60,
	}))

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/automation/jobs/chat-auto-send-retry/run", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if chatSvc.retryDraftCalls != 0 {
		t.Fatalf("expected zero retry calls, got %d", chatSvc.retryDraftCalls)
	}
	if len(store.jobRuns) != 1 {
		t.Fatalf("expected one job run, got %d", len(store.jobRuns))
	}
	if store.jobRuns[0].Status != "SKIPPED" {
		t.Fatalf("expected SKIPPED job run, got %s", store.jobRuns[0].Status)
	}

	var out RunChatAutoSendRetryResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if out.Status != "skipped" {
		t.Fatalf("expected skipped status, got %s", out.Status)
	}
	if out.Reason != "no_due_retries" {
		t.Fatalf("expected no_due_retries reason, got %s", out.Reason)
	}
}

func TestStartChatAutoSendRetryLoopRunsSystemCycleWhenEnabled(t *testing.T) {
	now := time.Now().UTC()
	store := &fakeAutomationStore{}
	chatSvc := &fakeChatIngestor{
		sessions: []chat.Session{
			{
				ID:            "session-retry",
				Channel:       "WHATSAPP",
				ContactKey:    "554998887766@s.whatsapp.net",
				Status:        "ACTIVE",
				HandoffStatus: "BOT",
				Metadata: map[string]interface{}{
					"agent": map[string]interface{}{
						"auto_send_status":          "AUTO_SEND_RETRY_PENDING",
						"auto_send_last_attempt_at": now.Add(-2 * time.Minute).Format(time.RFC3339Nano),
					},
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
		retryDraftResults: map[string]chat.RetryDraftAutoSendResult{
			"session-retry": {
				Session: chat.Session{
					ID:            "session-retry",
					Channel:       "WHATSAPP",
					ContactKey:    "554998887766@s.whatsapp.net",
					Status:        "ACTIVE",
					HandoffStatus: "BOT",
					CreatedAt:     now,
					UpdatedAt:     now,
				},
				Status: "accepted",
				Reason: "draft_auto_send_retried",
			},
		},
	}
	svc := NewService(store, chatSvc, config.Config{})
	logger := &fakeChatBufferFlushLogger{ch: make(chan struct{}, 1)}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	StartChatAutoSendRetryLoop(ctx, svc, config.Config{
		ChatAutoSendRetryEnabled:         true,
		ChatAutoSendRetryIntervalSeconds: 60,
		ChatAutoSendRetryLimit:           10,
		ChatAutoSendRetryCooldownSeconds: 30,
	}, logger)

	select {
	case <-logger.ch:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected automatic chat-auto-send-retry cycle to run")
	}

	cancel()

	if len(store.jobRuns) != 1 {
		t.Fatalf("expected one system job run, got %d", len(store.jobRuns))
	}
	if store.jobRuns[0].TriggerSource != "SYSTEM" {
		t.Fatalf("expected SYSTEM trigger source, got %s", store.jobRuns[0].TriggerSource)
	}
	if chatSvc.retryDraftCalls != 1 {
		t.Fatalf("expected one retry call, got %d", chatSvc.retryDraftCalls)
	}
}

func TestStartChatAutoSendRetryLoopDoesNothingWhenDisabled(t *testing.T) {
	store := &fakeAutomationStore{}
	chatSvc := &fakeChatIngestor{}
	svc := NewService(store, chatSvc, config.Config{})
	logger := &fakeChatBufferFlushLogger{ch: make(chan struct{}, 1)}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	StartChatAutoSendRetryLoop(ctx, svc, config.Config{
		ChatAutoSendRetryEnabled: false,
	}, logger)

	select {
	case <-logger.ch:
		t.Fatal("did not expect automatic chat-auto-send-retry cycle when disabled")
	case <-time.After(100 * time.Millisecond):
	}

	if len(store.jobRuns) != 0 {
		t.Fatalf("expected no job runs, got %d", len(store.jobRuns))
	}
	if chatSvc.retryDraftCalls != 0 {
		t.Fatalf("expected no retry calls, got %d", chatSvc.retryDraftCalls)
	}
}

func TestRunBookingsExpireProcessesExpiredBookings(t *testing.T) {
	store := &fakeAutomationStore{}
	bookingSvc := &fakeBookingExpirationSource{
		items: []bookings.BookingListItem{
			{
				ID:              "booking-1",
				TripID:          "trip-1",
				Status:          "PENDING",
				ReservationCode: "ABCD1234",
				CreatedAt:       time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC),
				ExpiresAt:       timePointer(time.Date(2026, 4, 22, 10, 45, 0, 0, time.UTC)),
			},
			{
				ID:              "booking-2",
				TripID:          "trip-2",
				Status:          "PENDING",
				ReservationCode: "EFGH5678",
				CreatedAt:       time.Date(2026, 4, 22, 11, 30, 0, 0, time.UTC),
				ExpiresAt:       timePointer(time.Date(2099, 4, 22, 12, 0, 0, 0, time.UTC)),
			},
			{
				ID:              "booking-3",
				TripID:          "trip-3",
				Status:          "PENDING",
				ReservationCode: "IJKL9012",
				CreatedAt:       time.Date(2026, 4, 22, 9, 0, 0, 0, time.UTC),
			},
		},
	}
	handler := NewHandler(NewService(store, &fakeChatIngestor{}, config.Config{}, bookingSvc))

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/automation/jobs/bookings-expire/run", bytes.NewBufferString(`{"limit":500,"hold_minutes":50}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if bookingSvc.listCalls != 1 {
		t.Fatalf("expected list to be called once, got %d", bookingSvc.listCalls)
	}
	if bookingSvc.lastFilter.Status != "PENDING" {
		t.Fatalf("expected PENDING filter, got %s", bookingSvc.lastFilter.Status)
	}
	if len(bookingSvc.updateCalls) != 2 {
		t.Fatalf("expected two expired bookings, got %d", len(bookingSvc.updateCalls))
	}
	if bookingSvc.updateCalls[0].Status != "EXPIRED" || bookingSvc.updateCalls[1].Status != "EXPIRED" {
		t.Fatalf("expected EXPIRED updates, got %+v", bookingSvc.updateCalls)
	}
	if len(store.jobRuns) != 1 {
		t.Fatalf("expected one job run, got %d", len(store.jobRuns))
	}
	if store.jobRuns[0].Status != "COMPLETED" {
		t.Fatalf("expected COMPLETED job run, got %s", store.jobRuns[0].Status)
	}

	var out RunBookingsExpireResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if out.Status != "processed" {
		t.Fatalf("expected processed status, got %s", out.Status)
	}
	if out.ExpiredCount != 2 {
		t.Fatalf("expected expired_count 2, got %d", out.ExpiredCount)
	}
	if out.CheckedCount != 3 {
		t.Fatalf("expected checked_count 3, got %d", out.CheckedCount)
	}
}

func TestRunBookingsExpireSkipsWhenNothingExpired(t *testing.T) {
	store := &fakeAutomationStore{}
	bookingSvc := &fakeBookingExpirationSource{
		items: []bookings.BookingListItem{
			{
				ID:              "booking-1",
				TripID:          "trip-1",
				Status:          "PENDING",
				ReservationCode: "ABCD1234",
				CreatedAt:       time.Now().UTC(),
				ExpiresAt:       timePointer(time.Now().UTC().Add(30 * time.Minute)),
			},
		},
	}
	handler := NewHandler(NewService(store, &fakeChatIngestor{}, config.Config{}, bookingSvc))

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/automation/jobs/bookings-expire/run", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if len(bookingSvc.updateCalls) != 0 {
		t.Fatalf("expected no updates, got %d", len(bookingSvc.updateCalls))
	}
	if len(store.jobRuns) != 1 {
		t.Fatalf("expected one job run, got %d", len(store.jobRuns))
	}
	if store.jobRuns[0].Status != "SKIPPED" {
		t.Fatalf("expected SKIPPED job run, got %s", store.jobRuns[0].Status)
	}

	var out RunBookingsExpireResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if out.Status != "skipped" {
		t.Fatalf("expected skipped status, got %s", out.Status)
	}
	if out.Reason != "no_expired_bookings" {
		t.Fatalf("expected no_expired_bookings, got %s", out.Reason)
	}
}

func TestRunPaymentNotificationsRejectsMissingTarget(t *testing.T) {
	handler := NewHandler(NewService(&fakeAutomationStore{}, &fakeChatIngestor{}, config.Config{}, &fakePaymentNotificationSource{}))

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/automation/jobs/payment-notifications/run", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestRunPaymentNotificationsQueuesReviewDraft(t *testing.T) {
	store := &fakeAutomationStore{}
	chatSvc := &fakeChatIngestor{
		draftResult: chat.QueueAutomationDraftResult{
			Session: chat.Session{
				ID:            "session-1",
				Channel:       "WHATSAPP",
				ContactKey:    "554998887766@s.whatsapp.net",
				CustomerPhone: "554998887766",
				CustomerName:  "Joao",
				Status:        "ACTIVE",
				HandoffStatus: "BOT",
				CreatedAt:     time.Now().UTC(),
				UpdatedAt:     time.Now().UTC(),
			},
			Message: chat.Message{
				ID:               "draft-1",
				SessionID:        "session-1",
				Direction:        "OUTBOUND",
				Kind:             "TEXT",
				ProcessingStatus: "AUTOMATION_DRAFT",
				ReceivedAt:       time.Now().UTC(),
				CreatedAt:        time.Now().UTC(),
			},
		},
	}
	paymentSource := &fakePaymentNotificationSource{
		payment: payments.Payment{
			ID:        "pay-1",
			BookingID: "booking-1",
			Amount:    250,
			Method:    "PIX",
			Status:    "PAID",
		},
		notification: payments.PaymentNotificationContext{
			BookingID:       "booking-1",
			ReservationCode: "ABCD1234",
			CustomerName:    "Joao",
			CustomerPhone:   "54998887766",
			AmountTotal:     1100,
			AmountPaid:      250,
			AmountDue:       850,
			PaymentStatus:   "PARTIAL",
		},
	}
	handler := NewHandler(NewService(store, chatSvc, config.Config{}, paymentSource))

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/automation/jobs/payment-notifications/run", bytes.NewBufferString(`{"payment_id":"pay-1"}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if chatSvc.draftCalls != 1 {
		t.Fatalf("expected draft to be queued once, got %d", chatSvc.draftCalls)
	}
	if chatSvc.lastDraftInput.ContactKey != "5554998887766@s.whatsapp.net" {
		t.Fatalf("unexpected contact key: %s", chatSvc.lastDraftInput.ContactKey)
	}
	if chatSvc.lastDraftInput.IdempotencyKey != "payment-notification-pay-1" {
		t.Fatalf("unexpected draft idempotency key: %s", chatSvc.lastDraftInput.IdempotencyKey)
	}
	if !strings.Contains(chatSvc.lastDraftInput.Body, "ABCD1234") {
		t.Fatalf("expected reservation code in draft body, got %q", chatSvc.lastDraftInput.Body)
	}
	if len(store.jobRuns) != 1 {
		t.Fatalf("expected one job run, got %d", len(store.jobRuns))
	}
	if store.jobRuns[0].Status != "QUEUED" {
		t.Fatalf("expected QUEUED job run, got %s", store.jobRuns[0].Status)
	}

	var out RunPaymentNotificationsResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if out.Status != "queued" {
		t.Fatalf("expected status queued, got %s", out.Status)
	}
	if !out.DraftQueued {
		t.Fatalf("expected draft_queued true")
	}
	if out.DraftMessageID != "draft-1" {
		t.Fatalf("expected draft_message_id draft-1, got %s", out.DraftMessageID)
	}
	if out.JobRunID == "" {
		t.Fatalf("expected job_run_id to be present")
	}
}

func TestRunPaymentNotificationsSkipsWhenPhoneMissing(t *testing.T) {
	store := &fakeAutomationStore{}
	chatSvc := &fakeChatIngestor{}
	paymentSource := &fakePaymentNotificationSource{
		payment: payments.Payment{
			ID:        "pay-1",
			BookingID: "booking-1",
			Amount:    250,
			Method:    "PIX",
			Status:    "PAID",
		},
		notification: payments.PaymentNotificationContext{
			BookingID:       "booking-1",
			ReservationCode: "ABCD1234",
			CustomerName:    "Joao",
			AmountTotal:     1100,
			AmountPaid:      250,
			AmountDue:       850,
			PaymentStatus:   "PARTIAL",
		},
	}
	handler := NewHandler(NewService(store, chatSvc, config.Config{}, paymentSource))

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/automation/jobs/payment-notifications/run", bytes.NewBufferString(`{"payment_id":"pay-1"}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if chatSvc.draftCalls != 0 {
		t.Fatalf("expected no draft queue calls, got %d", chatSvc.draftCalls)
	}
	if len(store.jobRuns) != 1 {
		t.Fatalf("expected one job run, got %d", len(store.jobRuns))
	}
	if store.jobRuns[0].Status != "SKIPPED" {
		t.Fatalf("expected SKIPPED job run, got %s", store.jobRuns[0].Status)
	}

	var out RunPaymentNotificationsResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if out.Reason != "missing_customer_phone" {
		t.Fatalf("expected missing_customer_phone, got %s", out.Reason)
	}
}

func TestRunPaymentNotificationsDeduplicatesDraftByPayment(t *testing.T) {
	store := &fakeAutomationStore{}
	chatSvc := &fakeChatIngestor{
		draftResult: chat.QueueAutomationDraftResult{
			Session: chat.Session{
				ID:            "session-1",
				Channel:       "WHATSAPP",
				ContactKey:    "554998887766@s.whatsapp.net",
				CustomerPhone: "554998887766",
				Status:        "ACTIVE",
				HandoffStatus: "BOT",
				CreatedAt:     time.Now().UTC(),
				UpdatedAt:     time.Now().UTC(),
			},
			Message: chat.Message{
				ID:               "draft-1",
				SessionID:        "session-1",
				Direction:        "OUTBOUND",
				Kind:             "TEXT",
				ProcessingStatus: "AUTOMATION_DRAFT",
				ReceivedAt:       time.Now().UTC(),
				CreatedAt:        time.Now().UTC(),
			},
			Idempotent: true,
		},
	}
	paymentSource := &fakePaymentNotificationSource{
		payment: payments.Payment{
			ID:        "pay-1",
			BookingID: "booking-1",
			Amount:    250,
			Method:    "PIX",
			Status:    "PAID",
		},
		notification: payments.PaymentNotificationContext{
			BookingID:       "booking-1",
			ReservationCode: "ABCD1234",
			CustomerName:    "Joao",
			CustomerPhone:   "54998887766",
			AmountTotal:     1100,
			AmountPaid:      250,
			AmountDue:       850,
			PaymentStatus:   "PARTIAL",
		},
	}
	handler := NewHandler(NewService(store, chatSvc, config.Config{}, paymentSource))

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/automation/jobs/payment-notifications/run", bytes.NewBufferString(`{"payment_id":"pay-1"}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if len(store.jobRuns) != 1 {
		t.Fatalf("expected one job run, got %d", len(store.jobRuns))
	}
	if store.jobRuns[0].Status != "SKIPPED" {
		t.Fatalf("expected SKIPPED job run, got %s", store.jobRuns[0].Status)
	}

	var out RunPaymentNotificationsResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if out.Status != "skipped" {
		t.Fatalf("expected status skipped, got %s", out.Status)
	}
	if out.Reason != "draft_already_exists" {
		t.Fatalf("expected reason draft_already_exists, got %s", out.Reason)
	}
	if !out.Idempotent {
		t.Fatalf("expected idempotent true")
	}
}

func TestListJobRunsReturnsNormalizedJobRuns(t *testing.T) {
	store := &fakeAutomationStore{
		jobRuns: []JobRun{
			{
				ID:            uuid.NewString(),
				JobName:       "CHAT_REVIEW_ALERTS",
				TriggerSource: "MANUAL",
				Status:        "SENT",
				StartedAt:     time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC),
				CreatedAt:     time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC),
			},
			{
				ID:            uuid.NewString(),
				JobName:       "CHAT_REVIEW_ALERTS",
				TriggerSource: "SYSTEM",
				Status:        "FAILED",
				StartedAt:     time.Date(2026, 4, 22, 11, 0, 0, 0, time.UTC),
				CreatedAt:     time.Date(2026, 4, 22, 11, 0, 0, 0, time.UTC),
			},
			{
				ID:            uuid.NewString(),
				JobName:       "PAYMENT_NOTIFICATIONS",
				TriggerSource: "MANUAL",
				Status:        "SENT",
				StartedAt:     time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC),
				CreatedAt:     time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC),
			},
		},
	}
	handler := NewHandler(NewService(store, &fakeChatIngestor{}, config.Config{}))

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/automation/jobs/chat-review-alerts/runs?status=sent&limit=1", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var out ListJobRunsResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if out.JobName != "CHAT_REVIEW_ALERTS" {
		t.Fatalf("expected job name CHAT_REVIEW_ALERTS, got %s", out.JobName)
	}
	if out.Filter.Status != "SENT" {
		t.Fatalf("expected filter status SENT, got %s", out.Filter.Status)
	}
	if out.Filter.Limit != 1 {
		t.Fatalf("expected filter limit 1, got %d", out.Filter.Limit)
	}
	if out.Count != 1 || len(out.Runs) != 1 {
		t.Fatalf("expected one run, got count=%d len=%d", out.Count, len(out.Runs))
	}
	if out.Runs[0].TriggerSource != "MANUAL" {
		t.Fatalf("expected MANUAL trigger source, got %s", out.Runs[0].TriggerSource)
	}
}

func TestListJobRunsFiltersByTriggerSource(t *testing.T) {
	store := &fakeAutomationStore{
		jobRuns: []JobRun{
			{
				ID:            uuid.NewString(),
				JobName:       "CHAT_REVIEW_ALERTS",
				TriggerSource: "MANUAL",
				Status:        "SENT",
				StartedAt:     time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC),
				CreatedAt:     time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC),
			},
			{
				ID:            uuid.NewString(),
				JobName:       "CHAT_REVIEW_ALERTS",
				TriggerSource: "SYSTEM",
				Status:        "SENT",
				StartedAt:     time.Date(2026, 4, 22, 11, 0, 0, 0, time.UTC),
				CreatedAt:     time.Date(2026, 4, 22, 11, 0, 0, 0, time.UTC),
			},
		},
	}
	handler := NewHandler(NewService(store, &fakeChatIngestor{}, config.Config{}))

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/automation/jobs/chat-review-alerts/runs?trigger_source=system", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var out ListJobRunsResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if out.Count != 1 || len(out.Runs) != 1 {
		t.Fatalf("expected one filtered run, got count=%d len=%d", out.Count, len(out.Runs))
	}
	if out.Runs[0].TriggerSource != "SYSTEM" {
		t.Fatalf("expected SYSTEM trigger source, got %s", out.Runs[0].TriggerSource)
	}
}

func TestListJobRunsRejectsInvalidLimit(t *testing.T) {
	handler := NewHandler(NewService(&fakeAutomationStore{}, &fakeChatIngestor{}, config.Config{}))

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/automation/jobs/chat-review-alerts/runs?limit=nope", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestGetCutoverReadinessReturnsSnapshot(t *testing.T) {
	startedAt := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(5 * time.Second)
	store := &fakeAutomationStore{
		jobRuns: []JobRun{
			{
				ID:            "job-buffer",
				JobName:       "CHAT_BUFFER_FLUSH",
				TriggerSource: "SYSTEM",
				Status:        "COMPLETED",
				StartedAt:     startedAt,
				FinishedAt:    &finishedAt,
				CreatedAt:     startedAt,
			},
			{
				ID:            "job-retry",
				JobName:       "CHAT_AUTO_SEND_RETRY",
				TriggerSource: "SYSTEM",
				Status:        "COMPLETED",
				StartedAt:     startedAt,
				FinishedAt:    &finishedAt,
				CreatedAt:     startedAt,
			},
		},
	}
	chatSvc := &fakeChatIngestor{
		summary: chat.SessionsSummary{
			TotalCount:         4,
			PendingReviewCount: 1,
			BotOwnedCount:      4,
			ReviewSLASeconds:   900,
		},
	}
	handler := NewHandler(NewService(store, chatSvc, config.Config{
		OpenAIAPIKey:               "sk-test",
		OpenAIModel:                "gpt-5.4-mini",
		EvolutionBaseURL:           "https://evolution.example.com",
		EvolutionAPIKey:            "evo-key",
		EvolutionInstance:          "schumacher",
		EvolutionWebhookSecret:     "cutover-secret",
		ChatBufferAutoFlushEnabled: true,
		ChatAutoSendRetryEnabled:   true,
	}, &fakePaymentNotificationSource{}, &fakeBookingExpirationSource{}))

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/automation/cutover/readiness", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var out CutoverReadinessResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if out.Status != "READY" {
		t.Fatalf("expected READY status, got %s", out.Status)
	}
	if out.SessionsSummary.TotalCount != 4 {
		t.Fatalf("expected total_count 4, got %d", out.SessionsSummary.TotalCount)
	}
	if got := findCutoverCheck(out.Checks, "evolution_webhook_secret"); got == nil || got.Status != "READY" {
		t.Fatalf("expected evolution_webhook_secret check to be READY, got %#v", got)
	}
	if got := findCutoverJob(out.LatestJobs, "CHAT_BUFFER_FLUSH"); got == nil || got.Status != "COMPLETED" {
		t.Fatalf("expected CHAT_BUFFER_FLUSH job snapshot, got %#v", got)
	}
	if got := findCutoverJob(out.LatestJobs, "PAYMENT_NOTIFICATIONS"); got == nil || got.Status != "NEVER_RUN" {
		t.Fatalf("expected PAYMENT_NOTIFICATIONS as NEVER_RUN, got %#v", got)
	}
}

func TestGetCutoverReadinessFlagsAttentionWhenCriticalCheckFails(t *testing.T) {
	chatSvc := &fakeChatIngestor{
		summary: chat.SessionsSummary{
			TotalCount:       2,
			ReviewSLASeconds: 900,
		},
	}
	handler := NewHandler(NewService(&fakeAutomationStore{}, chatSvc, config.Config{
		OpenAIAPIKey:               "sk-test",
		OpenAIModel:                "gpt-5.4-mini",
		EvolutionBaseURL:           "https://evolution.example.com",
		EvolutionAPIKey:            "evo-key",
		EvolutionInstance:          "schumacher",
		ChatBufferAutoFlushEnabled: false,
		ChatAutoSendRetryEnabled:   true,
	}, &fakePaymentNotificationSource{}, &fakeBookingExpirationSource{}))

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/automation/cutover/readiness", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var out CutoverReadinessResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if out.Status != "ATTENTION_REQUIRED" {
		t.Fatalf("expected ATTENTION_REQUIRED status, got %s", out.Status)
	}
	if got := findCutoverCheck(out.Checks, "chat_buffer_auto_flush"); got == nil || got.Status != "ATTENTION_REQUIRED" {
		t.Fatalf("expected chat_buffer_auto_flush attention, got %#v", got)
	}
	if len(out.Issues) == 0 {
		t.Fatalf("expected issues to be present")
	}
}

func TestRunChatReviewAlertsSkipsWithoutActiveAlert(t *testing.T) {
	chatSvc := &fakeChatIngestor{
		summary: chat.SessionsSummary{
			TotalCount:         3,
			PendingReviewCount: 1,
			ReviewSLASeconds:   900,
		},
	}
	notifier := &fakeChatReviewAlertNotifier{enabled: true}
	handler := NewHandler(NewService(&fakeAutomationStore{}, chatSvc, config.Config{}, notifier))

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/automation/jobs/chat-review-alerts/run", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if notifier.calls != 0 {
		t.Fatalf("expected notifier not to be called, got %d", notifier.calls)
	}

	var out RunChatReviewAlertsResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if out.Status != "skipped" {
		t.Fatalf("expected status skipped, got %s", out.Status)
	}
	if out.Reason != "no_active_alert" {
		t.Fatalf("expected reason no_active_alert, got %s", out.Reason)
	}
}

func TestRunChatReviewAlertsSendsWebhookWhenAlertIsActive(t *testing.T) {
	store := &fakeAutomationStore{}
	chatSvc := &fakeChatIngestor{
		summary: chat.SessionsSummary{
			TotalCount:              5,
			PendingReviewCount:      2,
			OverdueReviewCount:      1,
			HighPriorityReviewCount: 1,
			OldestPendingAgeSeconds: 1200,
			ReviewSLASeconds:        900,
			HasReviewAlert:          true,
			ReviewAlertLevel:        "CRITICAL",
			ReviewAlertCode:         "REVIEW_QUEUE_OVERDUE",
			ReviewAlertMessage:      "Fila com drafts acima do SLA de revisao.",
			ReviewAlertSessionCount: 1,
		},
	}
	notifier := &fakeChatReviewAlertNotifier{enabled: true}
	handler := NewHandler(NewService(store, chatSvc, config.Config{}, notifier))

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/automation/jobs/chat-review-alerts/run", bytes.NewBufferString(`{"channel":"whatsapp","handoff_status":"bot"}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if notifier.calls != 1 {
		t.Fatalf("expected notifier to be called once, got %d", notifier.calls)
	}
	if notifier.lastPayload.Filter.Channel != "WHATSAPP" {
		t.Fatalf("expected filter channel WHATSAPP, got %s", notifier.lastPayload.Filter.Channel)
	}
	if notifier.lastPayload.AlertCode != "REVIEW_QUEUE_OVERDUE" {
		t.Fatalf("expected alert code REVIEW_QUEUE_OVERDUE, got %s", notifier.lastPayload.AlertCode)
	}

	var out RunChatReviewAlertsResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if out.Status != "sent" {
		t.Fatalf("expected status sent, got %s", out.Status)
	}
	if !out.NotificationTriggered {
		t.Fatalf("expected notification_triggered true")
	}
	if out.JobRunID == "" {
		t.Fatalf("expected job_run_id to be present")
	}
	if len(store.jobRuns) != 1 {
		t.Fatalf("expected one persisted job run, got %d", len(store.jobRuns))
	}
	if store.jobRuns[0].Status != "SENT" {
		t.Fatalf("expected persisted job run status SENT, got %s", store.jobRuns[0].Status)
	}
}

func TestRunChatReviewAlertsDeduplicatesSameAlertState(t *testing.T) {
	store := &fakeAutomationStore{}
	chatSvc := &fakeChatIngestor{
		summary: chat.SessionsSummary{
			TotalCount:              5,
			PendingReviewCount:      2,
			OverdueReviewCount:      1,
			HighPriorityReviewCount: 1,
			OldestPendingAgeSeconds: 1200,
			ReviewSLASeconds:        900,
			HasReviewAlert:          true,
			ReviewAlertLevel:        "CRITICAL",
			ReviewAlertCode:         "REVIEW_QUEUE_OVERDUE",
			ReviewAlertMessage:      "Fila com drafts acima do SLA de revisao.",
			ReviewAlertSessionCount: 1,
		},
	}
	notifier := &fakeChatReviewAlertNotifier{enabled: true}
	handler := NewHandler(NewService(store, chatSvc, config.Config{}, notifier))

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	for i := 0; i < 2; i++ {
		if i == 1 {
			chatSvc.summary.OldestPendingAgeSeconds = 1500
		}
		req := httptest.NewRequest(http.MethodPost, "/automation/jobs/chat-review-alerts/run", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d on run %d", http.StatusOK, rec.Code, i+1)
		}
		if i == 1 {
			var out RunChatReviewAlertsResult
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatalf("unmarshal result: %v", err)
			}
			if out.Reason != "duplicate_alert_state" {
				t.Fatalf("expected duplicate_alert_state, got %s", out.Reason)
			}
			if !out.Deduplicated {
				t.Fatalf("expected deduplicated true")
			}
		}
	}

	if notifier.calls != 1 {
		t.Fatalf("expected notifier to be called once, got %d", notifier.calls)
	}
	if len(store.jobRuns) != 1 {
		t.Fatalf("expected one persisted sent job run, got %d", len(store.jobRuns))
	}
}

func TestRunChatReviewAlertsSkipsWhenNotifierNotConfigured(t *testing.T) {
	chatSvc := &fakeChatIngestor{
		summary: chat.SessionsSummary{
			HasReviewAlert:          true,
			ReviewAlertLevel:        "WARNING",
			ReviewAlertCode:         "REVIEW_QUEUE_DUE_SOON",
			ReviewAlertSessionCount: 2,
		},
	}
	handler := NewHandler(NewService(&fakeAutomationStore{}, chatSvc, config.Config{}))

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/automation/jobs/chat-review-alerts/run", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var out RunChatReviewAlertsResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if out.Reason != "notifier_not_configured" {
		t.Fatalf("expected reason notifier_not_configured, got %s", out.Reason)
	}
	if out.WebhookConfigured {
		t.Fatalf("expected webhook_configured false")
	}
}

type fakeChatIngestor struct {
	calls             int
	lastInput         chat.IngestMessageInput
	updateCalls       int
	lastUpdateInput   chat.UpdateMessageInput
	presenceCalls     int
	lastPresenceInput chat.ApplyPresenceSignalInput
	listCalls         int
	lastListFilter    chat.ListSessionsFilter
	sessions          []chat.Session
	summaryCalls      int
	lastSummaryFilter chat.ListSessionsFilter
	summary           chat.SessionsSummary
	reprocessCalls    int
	reprocessInputs   []chat.ReprocessInput
	reprocessResults  map[string]chat.ReprocessResult
	reprocessErrs     map[string]error
	retryDraftCalls   int
	retryDraftInputs  []chat.RetryDraftAutoSendInput
	retryDraftResults map[string]chat.RetryDraftAutoSendResult
	retryDraftErrs    map[string]error
	draftCalls        int
	lastDraftInput    chat.QueueAutomationDraftInput
	draftResult       chat.QueueAutomationDraftResult
}

func (f *fakeChatIngestor) Ingest(_ context.Context, input chat.IngestMessageInput) (chat.IngestMessageResult, error) {
	f.calls++
	f.lastInput = input
	now := time.Now().UTC()
	return chat.IngestMessageResult{
		Session: chat.Session{
			ID:            uuid.NewString(),
			Channel:       input.Channel,
			ContactKey:    input.ContactKey,
			CustomerPhone: input.CustomerPhone,
			CustomerName:  input.CustomerName,
			Status:        "ACTIVE",
			HandoffStatus: "BOT",
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		Message: chat.Message{
			ID:                uuid.NewString(),
			SessionID:         uuid.NewString(),
			Direction:         input.Message.Direction,
			Kind:              input.Message.Kind,
			ProviderMessageID: input.Message.ProviderMessageID,
			IdempotencyKey:    input.Message.IdempotencyKey,
			SenderName:        input.Message.SenderName,
			SenderPhone:       input.Message.SenderPhone,
			Body:              input.Message.Body,
			Payload:           input.Message.Payload,
			NormalizedPayload: input.Message.NormalizedPayload,
			ProcessingStatus:  input.Message.ProcessingStatus,
			ReceivedAt:        now,
			CreatedAt:         now,
		},
	}, nil
}

func (f *fakeChatIngestor) UpdateMessage(_ context.Context, input chat.UpdateMessageInput) (chat.Message, error) {
	f.updateCalls++
	f.lastUpdateInput = input
	message := chat.Message{
		ID:                input.MessageID,
		Body:              input.Body,
		NormalizedPayload: input.NormalizedPayload,
		ProcessingStatus:  input.ProcessingStatus,
	}
	return message, nil
}

func (f *fakeChatIngestor) ApplyPresenceSignal(_ context.Context, input chat.ApplyPresenceSignalInput) (chat.ApplyPresenceSignalResult, error) {
	f.presenceCalls++
	f.lastPresenceInput = input
	now := time.Now().UTC()
	return chat.ApplyPresenceSignalResult{
		Status:        "accepted",
		PresenceState: input.PresenceStatus,
		Reason:        "presence_extended",
		Session: chat.Session{
			ID:            uuid.NewString(),
			Channel:       input.Channel,
			ContactKey:    input.ContactKey,
			CustomerPhone: normalizePhone(input.ContactKey),
			Status:        "ACTIVE",
			HandoffStatus: "BOT",
			CreatedAt:     now,
			UpdatedAt:     now,
		},
	}, nil
}

func (f *fakeChatIngestor) ListSessions(_ context.Context, filter chat.ListSessionsFilter) ([]chat.Session, error) {
	f.listCalls++
	f.lastListFilter = filter
	return f.sessions, nil
}

func (f *fakeChatIngestor) GetSessionsSummary(_ context.Context, filter chat.ListSessionsFilter) (chat.SessionsSummary, error) {
	f.summaryCalls++
	f.lastSummaryFilter = filter
	return f.summary, nil
}

func (f *fakeChatIngestor) Reprocess(_ context.Context, input chat.ReprocessInput) (chat.ReprocessResult, error) {
	f.reprocessCalls++
	f.reprocessInputs = append(f.reprocessInputs, input)
	if err := f.reprocessErrs[input.SessionID]; err != nil {
		return chat.ReprocessResult{}, err
	}
	if result, ok := f.reprocessResults[input.SessionID]; ok {
		return result, nil
	}
	return chat.ReprocessResult{
		Session: chat.Session{
			ID:            input.SessionID,
			Status:        "ACTIVE",
			HandoffStatus: "BOT",
			CreatedAt:     time.Now().UTC(),
			UpdatedAt:     time.Now().UTC(),
		},
		Status: "accepted",
		Reason: "automation_pending",
	}, nil
}

func (f *fakeChatIngestor) RetryDraftAutoSend(_ context.Context, input chat.RetryDraftAutoSendInput) (chat.RetryDraftAutoSendResult, error) {
	f.retryDraftCalls++
	f.retryDraftInputs = append(f.retryDraftInputs, input)
	if err := f.retryDraftErrs[input.SessionID]; err != nil {
		return chat.RetryDraftAutoSendResult{}, err
	}
	if result, ok := f.retryDraftResults[input.SessionID]; ok {
		return result, nil
	}
	return chat.RetryDraftAutoSendResult{
		Session: chat.Session{
			ID:            input.SessionID,
			Status:        "ACTIVE",
			HandoffStatus: "BOT",
			CreatedAt:     time.Now().UTC(),
			UpdatedAt:     time.Now().UTC(),
		},
		Status: "accepted",
		Reason: "draft_auto_send_retried",
	}, nil
}

func (f *fakeChatIngestor) QueueAutomationDraft(_ context.Context, input chat.QueueAutomationDraftInput) (chat.QueueAutomationDraftResult, error) {
	f.draftCalls++
	f.lastDraftInput = input
	if f.draftResult.Session.ID == "" {
		now := time.Now().UTC()
		f.draftResult = chat.QueueAutomationDraftResult{
			Session: chat.Session{
				ID:            uuid.NewString(),
				Channel:       input.Channel,
				ContactKey:    input.ContactKey,
				CustomerPhone: input.CustomerPhone,
				CustomerName:  input.CustomerName,
				Status:        "ACTIVE",
				HandoffStatus: "BOT",
				CreatedAt:     now,
				UpdatedAt:     now,
			},
			Message: chat.Message{
				ID:                uuid.NewString(),
				SessionID:         uuid.NewString(),
				Direction:         "OUTBOUND",
				Kind:              "TEXT",
				IdempotencyKey:    input.IdempotencyKey,
				SenderName:        input.SenderName,
				Body:              input.Body,
				Payload:           input.Metadata,
				NormalizedPayload: input.Metadata,
				ProcessingStatus:  "AUTOMATION_DRAFT",
				ReceivedAt:        now,
				CreatedAt:         now,
			},
		}
	}
	return f.draftResult, nil
}

type fakePaymentNotificationSource struct {
	payment      payments.Payment
	list         []payments.Payment
	notification payments.PaymentNotificationContext
	getErr       error
	listErr      error
	notifyCtxErr error
}

func (f *fakePaymentNotificationSource) Get(_ context.Context, _ string) (payments.Payment, error) {
	if f.getErr != nil {
		return payments.Payment{}, f.getErr
	}
	return f.payment, nil
}

func (f *fakePaymentNotificationSource) List(_ context.Context, _ payments.PaymentListFilter) ([]payments.Payment, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.list, nil
}

func (f *fakePaymentNotificationSource) GetBookingNotificationContext(_ context.Context, _ string) (payments.PaymentNotificationContext, error) {
	if f.notifyCtxErr != nil {
		return payments.PaymentNotificationContext{}, f.notifyCtxErr
	}
	return f.notification, nil
}

type fakeBookingExpirationSource struct {
	items       []bookings.BookingListItem
	listErr     error
	updateErr   error
	listCalls   int
	lastFilter  bookings.ListFilter
	updateCalls []fakeBookingUpdateCall
}

type fakeBookingUpdateCall struct {
	ID     string
	Status string
}

func (f *fakeBookingExpirationSource) List(_ context.Context, filter bookings.ListFilter) ([]bookings.BookingListItem, error) {
	f.listCalls++
	f.lastFilter = filter
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.items, nil
}

func (f *fakeBookingExpirationSource) UpdateStatus(_ context.Context, id string, status string) (bookings.BookingDetails, error) {
	f.updateCalls = append(f.updateCalls, fakeBookingUpdateCall{ID: id, Status: status})
	if f.updateErr != nil {
		return bookings.BookingDetails{}, f.updateErr
	}

	var matched bookings.BookingListItem
	for _, item := range f.items {
		if item.ID == id {
			matched = item
			break
		}
	}

	return bookings.BookingDetails{
		Booking: bookings.Booking{
			ID:              id,
			TripID:          matched.TripID,
			Status:          status,
			ReservationCode: matched.ReservationCode,
			ExpiresAt:       matched.ExpiresAt,
			CreatedAt:       matched.CreatedAt,
		},
	}, nil
}

func TestNormalizePhone(t *testing.T) {
	if got := normalizePhone("5549988709047@s.whatsapp.net"); got != "5549988709047" {
		t.Fatalf("unexpected normalized phone: %s", got)
	}
}

func TestHandleEvolutionMessagesResponseBody(t *testing.T) {
	chatSvc := &fakeChatIngestor{}
	handler := NewHandler(NewService(&fakeAutomationStore{}, chatSvc, config.Config{}))

	r := chi.NewRouter()
	handler.RegisterWebhooks(r)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/evolution/messages", bytes.NewBufferString(`{
		"event":"messages.upsert",
		"data":{
			"key":{"remoteJid":"554998208115@s.whatsapp.net","fromMe":false,"id":"MSG-4"},
			"message":{"imageMessage":{"caption":"foto do documento","mimetype":"image/jpeg","url":"https://files.example.test/doc.jpg"}},
			"messageType":"imageMessage"
		}
	}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	var out EvolutionWebhookResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if out.Status != "accepted" {
		t.Fatalf("expected accepted status, got %s", out.Status)
	}
	if out.MessageType != "imageMessage" {
		t.Fatalf("expected imageMessage type, got %s", out.MessageType)
	}
	if chatSvc.lastInput.Message.Kind != "IMAGE" {
		t.Fatalf("unexpected kind: %s", chatSvc.lastInput.Message.Kind)
	}
	if got := chatSvc.lastInput.Message.NormalizedPayload["image_url"]; got != "https://files.example.test/doc.jpg" {
		t.Fatalf("unexpected image url: %#v", got)
	}
	if got := chatSvc.lastInput.Message.NormalizedPayload["image_mime_type"]; got != "image/jpeg" {
		t.Fatalf("unexpected image mime type: %#v", got)
	}
}

func TestHandleEvolutionMessagesEnrichesImageWithEvolutionBase64(t *testing.T) {
	type capturedMediaRequest struct {
		path   string
		apiKey string
		body   map[string]interface{}
	}

	requests := make(chan capturedMediaRequest, 1)
	mediaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		requests <- capturedMediaRequest{
			path:   r.URL.Path,
			apiKey: r.Header.Get("apikey"),
			body:   body,
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"base64":"/9j/2Q=="}`))
	}))
	defer mediaServer.Close()

	chatSvc := &fakeChatIngestor{}
	handler := NewHandler(NewService(&fakeAutomationStore{}, chatSvc, config.Config{
		EvolutionBaseURL:  mediaServer.URL,
		EvolutionAPIKey:   "secret",
		EvolutionInstance: "belle",
	}))

	r := chi.NewRouter()
	handler.RegisterWebhooks(r)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/evolution/messages", bytes.NewBufferString(`{
		"event":"messages.upsert",
		"data":{
			"key":{"remoteJid":"554998208115@s.whatsapp.net","fromMe":false,"id":"MSG-5"},
			"message":{"imageMessage":{"caption":"foto do documento","mimetype":"image/jpeg","url":"https://files.example.test/doc.jpg"}},
			"messageType":"imageMessage"
		}
	}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusAccepted, rec.Code, rec.Body.String())
	}
	var captured capturedMediaRequest
	select {
	case captured = <-requests:
	case <-time.After(time.Second):
		t.Fatal("expected evolution media request")
	}
	if captured.path != "/chat/getBase64FromMediaMessage/belle" {
		t.Fatalf("unexpected media path: %s", captured.path)
	}
	if captured.apiKey != "secret" {
		t.Fatalf("unexpected api key header: %s", captured.apiKey)
	}
	message, _ := captured.body["message"].(map[string]interface{})
	key, _ := message["key"].(map[string]interface{})
	if key["id"] != "MSG-5" {
		t.Fatalf("unexpected media message id: %#v", key["id"])
	}
	if key["remoteJid"] != "554998208115@s.whatsapp.net" {
		t.Fatalf("unexpected media remote jid: %#v", key["remoteJid"])
	}
	if key["fromMe"] != false {
		t.Fatalf("unexpected fromMe: %#v", key["fromMe"])
	}
	if got := chatSvc.lastInput.Message.NormalizedPayload["image_data_url"]; got != "data:image/jpeg;base64,/9j/2Q==" {
		t.Fatalf("unexpected image data url: %#v", got)
	}
	if got := chatSvc.lastInput.Message.NormalizedPayload["image_source"]; got != "evolution_get_base64" {
		t.Fatalf("unexpected image source: %#v", got)
	}
}

func TestHandleEvolutionMessagesAcceptsImageWithStructuredFileLength(t *testing.T) {
	chatSvc := &fakeChatIngestor{}
	handler := NewHandler(NewService(&fakeAutomationStore{}, chatSvc, config.Config{}))

	r := chi.NewRouter()
	handler.RegisterWebhooks(r)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/evolution/messages", bytes.NewBufferString(`{
		"event":"messages.upsert",
		"data":{
			"key":{"remoteJid":"554998208115@s.whatsapp.net","fromMe":false,"id":"MSG-4A"},
			"message":{"imageMessage":{
				"caption":"foto do rg",
				"mimetype":"image/jpeg",
				"url":"https://files.example.test/doc-structured.jpg",
				"fileLength":{"low":48123,"high":0,"unsigned":true}
			}},
			"messageType":"imageMessage"
		}
	}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusAccepted, rec.Code, rec.Body.String())
	}
	if chatSvc.lastInput.Message.Kind != "IMAGE" {
		t.Fatalf("unexpected kind: %s", chatSvc.lastInput.Message.Kind)
	}
	if got := chatSvc.lastInput.Message.NormalizedPayload["image_url"]; got != "https://files.example.test/doc-structured.jpg" {
		t.Fatalf("unexpected image url: %#v", got)
	}
	if _, ok := chatSvc.lastInput.Message.NormalizedPayload["image_file_length"]; ok {
		t.Fatalf("did not expect structured file length to break into invalid numeric metadata: %#v", chatSvc.lastInput.Message.NormalizedPayload["image_file_length"])
	}
}

func TestHandleEvolutionMessagesDocumentPDFMetadata(t *testing.T) {
	chatSvc := &fakeChatIngestor{}
	handler := NewHandler(NewService(&fakeAutomationStore{}, chatSvc, config.Config{}))

	r := chi.NewRouter()
	handler.RegisterWebhooks(r)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/evolution/messages", bytes.NewBufferString(`{
		"event":"messages.upsert",
		"instance":"belle",
		"data":{
			"key":{"remoteJid":"554998208115@s.whatsapp.net","fromMe":false,"id":"MSG-PDF-1"},
			"pushName":"Cliente PDF",
			"message":{
				"documentMessage":{
					"caption":"segue o pdf do documento",
					"fileName":"rg-frente-verso.pdf",
					"mimetype":"application/pdf",
					"pageCount":2,
					"fileLength":48123,
					"url":"https://files.example.test/doc.pdf"
				}
			},
			"messageType":"documentMessage",
			"messageTimestamp":1772544357
		}
	}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rec.Code)
	}
	if chatSvc.lastInput.Message.Kind != "DOCUMENT" {
		t.Fatalf("unexpected kind: %s", chatSvc.lastInput.Message.Kind)
	}
	if chatSvc.lastInput.Message.Body != "segue o pdf do documento" {
		t.Fatalf("unexpected extracted body: %s", chatSvc.lastInput.Message.Body)
	}
	if got := chatSvc.lastInput.Message.NormalizedPayload["document_file_name"]; got != "rg-frente-verso.pdf" {
		t.Fatalf("unexpected document file name: %#v", got)
	}
	if got := chatSvc.lastInput.Message.NormalizedPayload["document_mime_type"]; got != "application/pdf" {
		t.Fatalf("unexpected document mime type: %#v", got)
	}
	if got := chatSvc.lastInput.Message.NormalizedPayload["document_page_count"]; got != 2 {
		t.Fatalf("unexpected document page count: %#v", got)
	}
	if got := chatSvc.lastInput.Message.NormalizedPayload["document_is_pdf"]; got != true {
		t.Fatalf("expected document_is_pdf=true, got %#v", got)
	}
}

func TestHandleEvolutionMessagesDocumentFallsBackToFileName(t *testing.T) {
	chatSvc := &fakeChatIngestor{}
	handler := NewHandler(NewService(&fakeAutomationStore{}, chatSvc, config.Config{}))

	r := chi.NewRouter()
	handler.RegisterWebhooks(r)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/evolution/messages", bytes.NewBufferString(`{
		"event":"messages.upsert",
		"data":{
			"key":{"remoteJid":"554998208115@s.whatsapp.net","fromMe":false,"id":"MSG-PDF-2"},
			"message":{
				"documentMessage":{
					"fileName":"cpf-cliente.pdf",
					"mimetype":"application/pdf"
				}
			},
			"messageType":"documentMessage"
		}
	}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rec.Code)
	}
	if chatSvc.lastInput.Message.Body != "cpf-cliente.pdf" {
		t.Fatalf("expected fallback body from file name, got %s", chatSvc.lastInput.Message.Body)
	}
}

func TestHandleEvolutionStatusAcceptsDirectPayload(t *testing.T) {
	store := &fakeAutomationStore{
		result: RecordEvolutionStatusResult{MatchedChatMessages: 1, MatchedOutboundMessages: 2},
	}
	handler := NewHandler(NewService(store, &fakeChatIngestor{}, config.Config{}))

	r := chi.NewRouter()
	handler.RegisterWebhooks(r)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/evolution/status", bytes.NewBufferString(`{
		"event":"messages.update",
		"data":{
			"key":{"id":"MSG-STATUS-1"},
			"status":"READ",
			"messageType":"conversation",
			"messageTimestamp":1772544357
		}
	}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rec.Code)
	}
	if store.lastStatus.ProviderMessageID != "MSG-STATUS-1" {
		t.Fatalf("unexpected provider message id: %s", store.lastStatus.ProviderMessageID)
	}
	if store.lastStatus.ProviderStatus != "READ" {
		t.Fatalf("unexpected provider status: %s", store.lastStatus.ProviderStatus)
	}
}

func TestHandleEvolutionStatusAcceptsN8NEnvelope(t *testing.T) {
	store := &fakeAutomationStore{
		result: RecordEvolutionStatusResult{MatchedChatMessages: 0, MatchedOutboundMessages: 1},
	}
	handler := NewHandler(NewService(store, &fakeChatIngestor{}, config.Config{}))

	r := chi.NewRouter()
	handler.RegisterWebhooks(r)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/evolution/status", bytes.NewBufferString(`{
		"body":{
			"event":"messages.update",
			"data":{
				"key":{"id":"MSG-STATUS-2"},
				"status":"DELIVERY_ACK",
				"messageTimestamp":1772544357
			}
		}
	}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rec.Code)
	}
	if store.lastStatus.ProviderStatus != "DELIVERY_ACK" {
		t.Fatalf("unexpected provider status: %s", store.lastStatus.ProviderStatus)
	}
}

func TestHandleEvolutionStatusRejectsMissingStatus(t *testing.T) {
	handler := NewHandler(NewService(&fakeAutomationStore{}, &fakeChatIngestor{}, config.Config{}))

	r := chi.NewRouter()
	handler.RegisterWebhooks(r)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/evolution/status", bytes.NewBufferString(`{
		"event":"messages.update",
		"data":{"key":{"id":"MSG-STATUS-3"}}
	}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestHandleEvolutionPresenceAcceptsDirectPayload(t *testing.T) {
	chatSvc := &fakeChatIngestor{}
	handler := NewHandler(NewService(&fakeAutomationStore{}, chatSvc, config.Config{}))

	r := chi.NewRouter()
	handler.RegisterWebhooks(r)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/evolution/presence", bytes.NewBufferString(`{
		"event":"presence.update",
		"instance":"belle",
		"data":{
			"id":"554998208115@s.whatsapp.net",
			"presences":{
				"554998208115@s.whatsapp.net":{"lastKnownPresence":"typing"}
			}
		},
		"date_time":"2026-04-21T14:00:00Z"
	}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rec.Code)
	}
	if chatSvc.presenceCalls != 1 {
		t.Fatalf("expected one presence call, got %d", chatSvc.presenceCalls)
	}
	if chatSvc.lastPresenceInput.ContactKey != "554998208115@s.whatsapp.net" {
		t.Fatalf("unexpected contact key: %s", chatSvc.lastPresenceInput.ContactKey)
	}
	if chatSvc.lastPresenceInput.PresenceStatus != "TYPING" {
		t.Fatalf("unexpected presence status: %s", chatSvc.lastPresenceInput.PresenceStatus)
	}
}

func TestHandleEvolutionPresenceAcceptsN8NEnvelope(t *testing.T) {
	chatSvc := &fakeChatIngestor{}
	handler := NewHandler(NewService(&fakeAutomationStore{}, chatSvc, config.Config{}))

	r := chi.NewRouter()
	handler.RegisterWebhooks(r)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/evolution/presence", bytes.NewBufferString(`{
		"body":{
			"event":"presence.update",
			"data":{
				"id":"554998208115@s.whatsapp.net",
				"presences":{
					"554998208115@s.whatsapp.net":{"lastKnownPresence":"paused"}
				}
			}
		}
	}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rec.Code)
	}
	if chatSvc.lastPresenceInput.PresenceStatus != "PAUSED" {
		t.Fatalf("unexpected presence status: %s", chatSvc.lastPresenceInput.PresenceStatus)
	}
}

func TestHandleEvolutionPresenceRejectsMissingPresence(t *testing.T) {
	handler := NewHandler(NewService(&fakeAutomationStore{}, &fakeChatIngestor{}, config.Config{}))

	r := chi.NewRouter()
	handler.RegisterWebhooks(r)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/evolution/presence", bytes.NewBufferString(`{
		"event":"presence.update",
		"data":{"id":"554998208115@s.whatsapp.net","presences":{}}
	}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

type fakeAutomationStore struct {
	lastStatus RecordEvolutionStatusInput
	result     RecordEvolutionStatusResult
	jobRuns    []JobRun
}

func (f *fakeAutomationStore) RecordEvolutionStatus(_ context.Context, input RecordEvolutionStatusInput) (RecordEvolutionStatusResult, error) {
	f.lastStatus = input
	return f.result, nil
}

func (f *fakeAutomationStore) CreateJobRun(_ context.Context, input CreateJobRunInput) (JobRun, error) {
	finishedAt := input.FinishedAt
	item := JobRun{
		ID:            uuid.NewString(),
		JobName:       input.JobName,
		TriggerSource: input.TriggerSource,
		Status:        input.Status,
		InputPayload:  input.InputPayload,
		ResultPayload: input.ResultPayload,
		ErrorText:     input.ErrorText,
		StartedAt:     input.StartedAt,
		FinishedAt:    finishedAt,
		CreatedAt:     time.Now().UTC(),
	}
	f.jobRuns = append(f.jobRuns, item)
	return item, nil
}

func (f *fakeAutomationStore) UpdateJobRun(_ context.Context, input UpdateJobRunInput) (JobRun, error) {
	for i := range f.jobRuns {
		if f.jobRuns[i].ID != input.ID {
			continue
		}
		f.jobRuns[i].Status = input.Status
		f.jobRuns[i].ResultPayload = input.ResultPayload
		f.jobRuns[i].ErrorText = input.ErrorText
		finishedAt := input.FinishedAt
		f.jobRuns[i].FinishedAt = &finishedAt
		return f.jobRuns[i], nil
	}
	return JobRun{}, nil
}

func (f *fakeAutomationStore) FindLatestJobRunByKey(_ context.Context, jobName string, alertSignature string) (*JobRun, error) {
	for i := len(f.jobRuns) - 1; i >= 0; i-- {
		item := f.jobRuns[i]
		if item.JobName != jobName || item.Status != "SENT" {
			continue
		}
		if asString(item.ResultPayload["alert_signature"]) != alertSignature {
			continue
		}
		copy := item
		return &copy, nil
	}
	return nil, nil
}

func (f *fakeAutomationStore) ListJobRuns(_ context.Context, input ListJobRunsInput) ([]JobRun, error) {
	items := make([]JobRun, 0, len(f.jobRuns))
	for _, item := range f.jobRuns {
		if item.JobName != input.JobName {
			continue
		}
		if input.Status != "" && item.Status != input.Status {
			continue
		}
		if input.TriggerSource != "" && item.TriggerSource != input.TriggerSource {
			continue
		}
		items = append(items, item)
		if input.Limit > 0 && len(items) >= input.Limit {
			break
		}
	}
	return items, nil
}

type fakeChatReviewAlertNotifier struct {
	enabled     bool
	calls       int
	lastPayload ChatReviewAlertNotificationPayload
}

type fakeChatBufferFlushLogger struct {
	calls int
	ch    chan struct{}
	logs  []string
}

func findCutoverCheck(items []CutoverReadinessCheck, key string) *CutoverReadinessCheck {
	for i := range items {
		if items[i].Key == key {
			return &items[i]
		}
	}
	return nil
}

func findCutoverJob(items []CutoverReadinessJob, jobName string) *CutoverReadinessJob {
	for i := range items {
		if items[i].JobName == jobName {
			return &items[i]
		}
	}
	return nil
}

func (f *fakeChatReviewAlertNotifier) Enabled() bool {
	return f != nil && f.enabled
}

func (f *fakeChatReviewAlertNotifier) NotifyReviewAlert(_ context.Context, payload ChatReviewAlertNotificationPayload) error {
	f.calls++
	f.lastPayload = payload
	return nil
}

func (f *fakeChatBufferFlushLogger) Printf(format string, v ...interface{}) {
	f.calls++
	f.logs = append(f.logs, fmt.Sprintf(format, v...))
	if f.ch == nil {
		return
	}
	select {
	case f.ch <- struct{}{}:
	default:
	}
}
