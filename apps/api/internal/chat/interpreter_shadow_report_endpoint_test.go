package chat

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"schumacher-tur/api/internal/shared/config"
)

func TestGetStructuredInterpreterShadowReportServiceBuildsAggregatedReport(t *testing.T) {
	sessionID := uuid.NewString()
	store := newFakeStore()
	store.shadowReportMessages = []Message{
		structuredInterpreterShadowEndpointMessage(sessionID, string(StructuredIntentGreeting), string(StructuredIntentGreeting), true, nil),
		{
			ID:        uuid.NewString(),
			SessionID: sessionID,
			Body:      "body without shadow must not enter report",
			Payload: map[string]interface{}{
				"raw_prompt": "must not enter report",
			},
			NormalizedPayload: map[string]interface{}{
				"validation_errors": []string{"ignored_without_shadow"},
			},
		},
		structuredInterpreterShadowEndpointMessage(sessionID, string(StructuredIntentAvailabilitySearch), string(StructuredIntentPaymentPreference), false, []string{"json_decision_invalid"}),
	}

	result, err := NewService(store, config.Config{}).GetStructuredInterpreterShadowReport(context.Background(), StructuredInterpreterShadowReportFilter{
		SessionID: " " + sessionID + " ",
	})
	if err != nil {
		t.Fatalf("get report: %v", err)
	}

	if store.shadowReportCalls != 1 {
		t.Fatalf("expected one store call, got %d", store.shadowReportCalls)
	}
	if store.shadowReportFilter.Limit != 200 || store.shadowReportFilter.Offset != 0 || store.shadowReportFilter.SessionID != sessionID {
		t.Fatalf("unexpected normalized store filter: %+v", store.shadowReportFilter)
	}
	if result.LoadedMessageCount != 3 || result.ReportItemCount != 2 || result.Report.TotalItems != 2 {
		t.Fatalf("expected 3 loaded messages and 2 report items, got %+v", result)
	}
	if result.Report.IntentAgreementCount != 1 || result.Report.IntentDisagreementCount != 1 {
		t.Fatalf("expected one agreement and one disagreement, got %+v", result.Report)
	}
	if result.Report.ValidationErrorCount != 1 {
		t.Fatalf("expected one validation error item, got %+v", result.Report)
	}
	if result.Report.OpenAIValidation.Total != 2 ||
		result.Report.OpenAIValidation.Accepted != 1 ||
		result.Report.OpenAIValidation.Rejected != 1 {
		t.Fatalf("expected OpenAI validation metrics, got %+v", result.Report.OpenAIValidation)
	}
}

func TestGetStructuredInterpreterShadowReportServiceRequiresSessionID(t *testing.T) {
	store := newFakeStore()
	_, err := NewService(store, config.Config{}).GetStructuredInterpreterShadowReport(context.Background(), StructuredInterpreterShadowReportFilter{})
	if !errors.Is(err, ErrShadowReportSessionRequired) {
		t.Fatalf("expected session required error, got %v", err)
	}
	if store.shadowReportCalls != 0 {
		t.Fatalf("service must not call store without session_id, got %d calls", store.shadowReportCalls)
	}
}

func TestListStructuredInterpreterShadowMessagesRequiresSessionID(t *testing.T) {
	repo := &Repository{}
	_, err := repo.ListStructuredInterpreterShadowMessages(context.Background(), StructuredInterpreterShadowReportFilter{})
	if !errors.Is(err, ErrShadowReportSessionRequired) {
		t.Fatalf("expected session required error, got %v", err)
	}
}

func TestGetStructuredInterpreterShadowReportEndpointReturnsAggregatedReport(t *testing.T) {
	sessionID := uuid.NewString()
	store := newFakeStore()
	store.shadowReportMessages = []Message{
		structuredInterpreterShadowEndpointMessage(sessionID, string(StructuredIntentGreeting), string(StructuredIntentGreeting), true, nil),
		structuredInterpreterShadowEndpointMessage(sessionID, string(StructuredIntentAvailabilitySearch), string(StructuredIntentPaymentPreference), false, []string{"json_decision_invalid"}),
		{
			ID:        uuid.NewString(),
			SessionID: sessionID,
			Body:      "CPF 529.982.247-25 should not be serialized",
			Payload: map[string]interface{}{
				"raw_prompt": "raw prompt should not be serialized",
			},
			NormalizedPayload: map[string]interface{}{
				"current_turn_body": "normalized body should not be serialized",
			},
		},
		{
			ID:        uuid.NewString(),
			SessionID: sessionID,
			Body:      "shadow metric dimensions with phone and booking ids must be sanitized",
			NormalizedPayload: map[string]interface{}{
				structuredInterpreterShadowKey: map[string]interface{}{
					"local": map[string]interface{}{
						"intent": string(StructuredIntentGreeting),
						"source": "550e8400-e29b-41d4-a716-446655440000",
					},
					"openai": map[string]interface{}{
						"status": string(StructuredInterpreterShadowValid),
						"source": "BK-ABC123456",
					},
					"openai_validation": map[string]interface{}{
						"status":            string(StructuredInterpreterShadowValidationRejected),
						"reject_reason":     "(48)99999-9999",
						"fallback_template": "R--02-23-MLK08V8Q",
					},
				},
			},
		},
		{
			ID:        uuid.NewString(),
			SessionID: sessionID,
			Body:      "shadow metric dimensions with RG must be sanitized",
			NormalizedPayload: map[string]interface{}{
				structuredInterpreterShadowKey: map[string]interface{}{
					"openai": map[string]interface{}{
						"status": string(StructuredInterpreterShadowValid),
					},
					"openai_validation": map[string]interface{}{
						"status":        string(StructuredInterpreterShadowValidationRejected),
						"reject_reason": "12.345.678-9",
					},
				},
			},
		},
	}

	rec := serveStructuredInterpreterShadowReportRequest(store, "/chat/reports/structured-interpreter-shadow?limit=5&offset=0&session_id="+sessionID)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var out StructuredInterpreterShadowReportResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal report response: %v", err)
	}
	if store.shadowReportFilter.Limit != 5 || store.shadowReportFilter.Offset != 0 || store.shadowReportFilter.SessionID != sessionID {
		t.Fatalf("unexpected parsed filter: %+v", store.shadowReportFilter)
	}
	if out.Report.TotalItems != 4 || out.Report.IntentAgreementCount != 1 || out.Report.IntentDisagreementCount != 1 {
		t.Fatalf("unexpected aggregate report: %+v", out.Report)
	}
	if out.Report.ValidationErrorCount != 1 {
		t.Fatalf("expected validation error item to be counted, got %+v", out.Report)
	}
	if out.Report.OpenAIValidation.Total != 4 ||
		out.Report.OpenAIValidation.Accepted != 1 ||
		out.Report.OpenAIValidation.Rejected != 3 ||
		out.Report.OpenAIValidation.ByRejectReason["__redacted_sensitive"] != 2 ||
		out.Report.OpenAIValidation.ByFallbackTemplate["__redacted_sensitive"] != 1 ||
		out.Report.OpenAIValidation.ByLocalSource["__redacted_sensitive"].Rejected != 1 ||
		out.Report.OpenAIValidation.ByOpenAISource["__redacted_sensitive"].Rejected != 1 {
		t.Fatalf("unexpected OpenAI validation report: %+v", out.Report.OpenAIValidation)
	}

	body := rec.Body.String()
	for _, forbidden := range []string{
		"CPF 529.982.247-25",
		"550e8400-e29b-41d4-a716-446655440000",
		"BK-ABC123456",
		"(48)99999-9999",
		"12.345.678-9",
		"R--02-23-MLK08V8Q",
		"raw_prompt",
		"raw prompt should not be serialized",
		"current_turn_body",
		"normalized body should not be serialized",
		"normalized_payload",
		"payload",
		"body",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("response leaked forbidden content %q in %s", forbidden, body)
		}
	}
}

func TestGetStructuredInterpreterShadowReportEndpointAppliesDefaultLimit(t *testing.T) {
	sessionID := uuid.NewString()
	store := newFakeStore()
	store.shadowReportMessages = []Message{}

	rec := serveStructuredInterpreterShadowReportRequest(store, "/chat/reports/structured-interpreter-shadow?session_id="+sessionID)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var out StructuredInterpreterShadowReportResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal report response: %v", err)
	}
	if store.shadowReportFilter.Limit != 200 || out.Filter.Limit != 200 {
		t.Fatalf("expected default limit 200, got store=%+v response=%+v", store.shadowReportFilter, out.Filter)
	}
	if store.shadowReportFilter.Offset != 0 || out.Filter.Offset != 0 {
		t.Fatalf("expected default offset 0, got store=%+v response=%+v", store.shadowReportFilter, out.Filter)
	}
	if store.shadowReportFilter.SessionID != sessionID || out.Filter.SessionID != sessionID {
		t.Fatalf("expected session filter %s, got store=%+v response=%+v", sessionID, store.shadowReportFilter, out.Filter)
	}
}

func TestGetStructuredInterpreterShadowReportEndpointRejectsInvalidQuery(t *testing.T) {
	for _, path := range []string{
		"/chat/reports/structured-interpreter-shadow",
		"/chat/reports/structured-interpreter-shadow?limit=0",
		"/chat/reports/structured-interpreter-shadow?limit=1001",
		"/chat/reports/structured-interpreter-shadow?limit=abc",
		"/chat/reports/structured-interpreter-shadow?offset=-1",
		"/chat/reports/structured-interpreter-shadow?offset=abc",
		"/chat/reports/structured-interpreter-shadow?session_id=not-a-uuid",
	} {
		t.Run(path, func(t *testing.T) {
			store := newFakeStore()
			rec := serveStructuredInterpreterShadowReportRequest(store, path)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
			}
			if store.shadowReportCalls != 0 {
				t.Fatalf("expected invalid query not to call store, got %d calls", store.shadowReportCalls)
			}
		})
	}
}

func TestGetStructuredInterpreterShadowReportEndpointReturnsStoreError(t *testing.T) {
	sessionID := uuid.NewString()
	store := newFakeStore()
	store.shadowReportErr = errors.New("store unavailable")

	rec := serveStructuredInterpreterShadowReportRequest(store, "/chat/reports/structured-interpreter-shadow?session_id="+sessionID)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d: %s", http.StatusInternalServerError, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "CHAT_STRUCTURED_INTERPRETER_SHADOW_REPORT_ERROR") {
		t.Fatalf("expected report error code, got %s", rec.Body.String())
	}
}

func serveStructuredInterpreterShadowReportRequest(store *fakeStore, path string) *httptest.ResponseRecorder {
	handler := NewHandler(NewService(store, config.Config{ChatDebounceWindowMS: 1500}))
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func structuredInterpreterShadowEndpointMessage(sessionID string, localIntent string, openAIIntent string, agreement bool, validationErrors []string) Message {
	payload := map[string]interface{}{
		structuredInterpreterShadowKey: map[string]interface{}{
			"local": map[string]interface{}{
				"intent": localIntent,
			},
			"openai": map[string]interface{}{
				"status":     string(StructuredInterpreterShadowValid),
				"intent":     openAIIntent,
				"latency_ms": 100,
			},
			"openai_validation": structuredInterpreterShadowEndpointValidation(agreement),
			"agreement": map[string]interface{}{
				"intent": agreement,
			},
		},
	}
	if len(validationErrors) > 0 {
		payload["validation_errors"] = validationErrors
	}
	return Message{
		ID:                uuid.NewString(),
		SessionID:         sessionID,
		Direction:         "OUTBOUND",
		Kind:              "TEXT",
		Body:              "message body must not be serialized",
		Payload:           map[string]interface{}{"request_payload": "must not be serialized"},
		NormalizedPayload: payload,
		ProcessingStatus:  "AUTOMATION_DRAFT",
		CreatedAt:         time.Now().UTC(),
	}
}

func structuredInterpreterShadowEndpointValidation(agreement bool) map[string]interface{} {
	if agreement {
		return map[string]interface{}{
			"status":   string(StructuredInterpreterShadowValidationAccepted),
			"accepted": true,
		}
	}
	return map[string]interface{}{
		"status":            string(StructuredInterpreterShadowValidationRejected),
		"reject_reason":     "intent_not_allowed_by_active_prompt",
		"fallback_template": string(TemplateContextFallbackPaymentPreference),
	}
}

func (s *fakeStore) ListStructuredInterpreterShadowMessages(_ context.Context, filter StructuredInterpreterShadowReportFilter) ([]Message, error) {
	filter = normalizeStructuredInterpreterShadowReportFilter(filter)
	s.shadowReportCalls++
	s.shadowReportFilter = filter
	if filter.SessionID == "" {
		return nil, ErrShadowReportSessionRequired
	}
	if s.shadowReportErr != nil {
		return nil, s.shadowReportErr
	}

	if s.shadowReportMessages != nil {
		return paginateFakeStructuredInterpreterShadowMessages(filterFakeStructuredInterpreterShadowMessages(s.shadowReportMessages, filter, false), filter), nil
	}

	items := make([]Message, 0, len(s.messageOrder))
	for _, id := range s.messageOrder {
		message := s.messages[id]
		if !fakeMessageHasStructuredInterpreterShadow(message) {
			continue
		}
		if filter.SessionID != "" && message.SessionID != filter.SessionID {
			continue
		}
		items = append(items, message)
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	return paginateFakeStructuredInterpreterShadowMessages(items, filter), nil
}

func filterFakeStructuredInterpreterShadowMessages(messages []Message, filter StructuredInterpreterShadowReportFilter, requireShadow bool) []Message {
	items := make([]Message, 0, len(messages))
	for _, message := range messages {
		if filter.SessionID != "" && message.SessionID != filter.SessionID {
			continue
		}
		if requireShadow && !fakeMessageHasStructuredInterpreterShadow(message) {
			continue
		}
		items = append(items, message)
	}
	return items
}

func paginateFakeStructuredInterpreterShadowMessages(messages []Message, filter StructuredInterpreterShadowReportFilter) []Message {
	if filter.Offset >= len(messages) {
		return []Message{}
	}
	end := filter.Offset + filter.Limit
	if end > len(messages) {
		end = len(messages)
	}
	return append([]Message(nil), messages[filter.Offset:end]...)
}

func fakeMessageHasStructuredInterpreterShadow(message Message) bool {
	if _, ok := message.NormalizedPayload[structuredInterpreterShadowKey]; ok {
		return true
	}
	if _, ok := message.Payload[structuredInterpreterShadowKey]; ok {
		return true
	}
	return false
}
