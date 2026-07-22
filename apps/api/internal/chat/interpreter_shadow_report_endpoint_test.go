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
	store := newFakeStoreWithPassengerAuthority()
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
		openAIInterpreterAssistEndpointMessage(sessionID, string(OpenAIInterpreterAssistAccepted), string(StructuredIntentPassengerCountReply), ""),
	}

	result, err := NewService(store, config.Config{}).GetStructuredInterpreterShadowReport(context.Background(), StructuredInterpreterShadowReportFilter{
		SessionID: " " + sessionID + " ",
	})
	if err != nil {
		t.Fatalf("get report: %v", err)
	}

	if store.shadowReportCalls != 1 || store.assistReportCalls != 1 {
		t.Fatalf("expected one shadow and one assist store call, got shadow=%d assist=%d", store.shadowReportCalls, store.assistReportCalls)
	}
	if store.shadowReportFilter.Limit != 200 || store.shadowReportFilter.Offset != 0 || store.shadowReportFilter.SessionID != sessionID {
		t.Fatalf("unexpected normalized shadow store filter: %+v", store.shadowReportFilter)
	}
	if store.assistReportFilter.Limit != 200 || store.assistReportFilter.Offset != 0 || store.assistReportFilter.SessionID != sessionID {
		t.Fatalf("unexpected normalized assist store filter: %+v", store.assistReportFilter)
	}
	if result.LoadedMessageCount != 2 || result.ReportItemCount != 2 || result.AssistReportItemCount != 1 || result.Report.TotalItems != 2 {
		t.Fatalf("expected 2 loaded shadow messages, 2 shadow items and 1 assist item, got %+v", result)
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
	if result.Report.OpenAIInterpreterAssist.Total != 1 ||
		result.Report.OpenAIInterpreterAssist.Accepted != 1 ||
		result.Report.OpenAIInterpreterAssist.ByIntent[string(StructuredIntentPassengerCountReply)].Accepted != 1 {
		t.Fatalf("expected runtime assist metrics, got %+v", result.Report.OpenAIInterpreterAssist)
	}
}

func TestGetStructuredInterpreterShadowReportServiceKeepsShadowPaginationSeparateFromAssist(t *testing.T) {
	sessionID := uuid.NewString()
	base := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	shadowMessage := structuredInterpreterShadowEndpointMessage(sessionID, string(StructuredIntentGreeting), string(StructuredIntentGreeting), true, nil)
	shadowMessage.CreatedAt = base
	assistMessage := openAIInterpreterAssistEndpointMessage(sessionID, string(OpenAIInterpreterAssistAccepted), string(StructuredIntentPassengerCountReply), "")
	assistMessage.CreatedAt = base.Add(time.Minute)

	store := newFakeStoreWithPassengerAuthority()
	store.shadowReportMessages = []Message{assistMessage, shadowMessage}

	result, err := NewService(store, config.Config{}).GetStructuredInterpreterShadowReport(context.Background(), StructuredInterpreterShadowReportFilter{
		SessionID: sessionID,
		Limit:     1,
	})
	if err != nil {
		t.Fatalf("get report: %v", err)
	}

	if result.LoadedMessageCount != 1 || result.ReportItemCount != 1 || result.Report.TotalItems != 1 {
		t.Fatalf("expected shadow page to contain the matching shadow row, got %+v", result)
	}
	if result.AssistReportItemCount != 1 || result.Report.OpenAIInterpreterAssist.Total != 1 {
		t.Fatalf("expected assist page to be loaded independently, got %+v", result.Report.OpenAIInterpreterAssist)
	}
	if result.Report.IntentAgreementCount != 1 || result.Report.OpenAIValidation.Accepted != 1 {
		t.Fatalf("expected structured report metrics to survive newer assist-only rows, got %+v", result.Report)
	}
}

func TestGetStructuredInterpreterShadowReportServiceRequiresSessionID(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	_, err := NewService(store, config.Config{}).GetStructuredInterpreterShadowReport(context.Background(), StructuredInterpreterShadowReportFilter{})
	if !errors.Is(err, ErrShadowReportSessionRequired) {
		t.Fatalf("expected session required error, got %v", err)
	}
	if store.shadowReportCalls != 0 || store.assistReportCalls != 0 {
		t.Fatalf("service must not call store without session_id, got shadow=%d assist=%d calls", store.shadowReportCalls, store.assistReportCalls)
	}
}

func TestListStructuredInterpreterShadowMessagesRequiresSessionID(t *testing.T) {
	repo := &Repository{}
	_, err := repo.ListStructuredInterpreterShadowMessages(context.Background(), StructuredInterpreterShadowReportFilter{})
	if !errors.Is(err, ErrShadowReportSessionRequired) {
		t.Fatalf("expected session required error, got %v", err)
	}
}

func TestListOpenAIInterpreterAssistMessagesRequiresSessionID(t *testing.T) {
	repo := &Repository{}
	_, err := repo.ListOpenAIInterpreterAssistMessages(context.Background(), StructuredInterpreterShadowReportFilter{})
	if !errors.Is(err, ErrShadowReportSessionRequired) {
		t.Fatalf("expected session required error, got %v", err)
	}
}

func TestGetStructuredInterpreterShadowReportEndpointReturnsAggregatedReport(t *testing.T) {
	sessionID := uuid.NewString()
	store := newFakeStoreWithPassengerAuthority()
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
		{
			ID:        uuid.NewString(),
			SessionID: sessionID,
			Body:      "assist body with CPF 529.982.247-25 must not be serialized",
			NormalizedPayload: map[string]interface{}{
				openAIInterpreterAssistMetadataKey: map[string]interface{}{
					"openai_assist_status": string(OpenAIInterpreterAssistRejected),
					"considered":           true,
					"openai_intent":        string(StructuredIntentPaymentPreference),
					"openai_confidence":    0.77,
					"reject_reason":        "telefone_(48)99999-9999",
					"fallback_template":    "booking_550e8400-e29b-41d4-a716-446655440000",
					"source":               "Joao Vitor Messias",
					"current_turn_body":    "raw customer text should not be serialized",
				},
			},
		},
	}

	rec := serveStructuredInterpreterShadowReportRequest(store, "/chat/reports/structured-interpreter-shadow?limit=10&offset=0&session_id="+sessionID)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var out StructuredInterpreterShadowReportResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal report response: %v", err)
	}
	if store.shadowReportFilter.Limit != 10 || store.shadowReportFilter.Offset != 0 || store.shadowReportFilter.SessionID != sessionID {
		t.Fatalf("unexpected parsed shadow filter: %+v", store.shadowReportFilter)
	}
	if store.assistReportFilter.Limit != 10 || store.assistReportFilter.Offset != 0 || store.assistReportFilter.SessionID != sessionID {
		t.Fatalf("unexpected parsed assist filter: %+v", store.assistReportFilter)
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
	if out.Report.OpenAIInterpreterAssist.Total != 1 ||
		out.Report.OpenAIInterpreterAssist.Rejected != 1 ||
		out.Report.OpenAIInterpreterAssist.ByRejectReason["__redacted_sensitive"] != 1 ||
		out.Report.OpenAIInterpreterAssist.ByFallbackTemplate["__redacted_sensitive"] != 1 ||
		out.Report.OpenAIInterpreterAssist.BySource["__redacted_sensitive"].Rejected != 1 {
		t.Fatalf("unexpected runtime assist report: %+v", out.Report.OpenAIInterpreterAssist)
	}

	body := rec.Body.String()
	for _, forbidden := range []string{
		"CPF 529.982.247-25",
		"assist body with CPF",
		"550e8400-e29b-41d4-a716-446655440000",
		"booking_550e8400-e29b-41d4-a716-446655440000",
		"BK-ABC123456",
		"(48)99999-9999",
		"telefone_(48)99999-9999",
		"12.345.678-9",
		"R--02-23-MLK08V8Q",
		"Joao Vitor Messias",
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
	store := newFakeStoreWithPassengerAuthority()
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
	if store.assistReportFilter.Limit != 200 {
		t.Fatalf("expected default assist limit 200, got %+v", store.assistReportFilter)
	}
	if store.shadowReportFilter.Offset != 0 || out.Filter.Offset != 0 {
		t.Fatalf("expected default offset 0, got store=%+v response=%+v", store.shadowReportFilter, out.Filter)
	}
	if store.assistReportFilter.Offset != 0 {
		t.Fatalf("expected default assist offset 0, got %+v", store.assistReportFilter)
	}
	if store.shadowReportFilter.SessionID != sessionID || out.Filter.SessionID != sessionID {
		t.Fatalf("expected session filter %s, got store=%+v response=%+v", sessionID, store.shadowReportFilter, out.Filter)
	}
	if store.assistReportFilter.SessionID != sessionID {
		t.Fatalf("expected assist session filter %s, got %+v", sessionID, store.assistReportFilter)
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
			store := newFakeStoreWithPassengerAuthority()
			rec := serveStructuredInterpreterShadowReportRequest(store, path)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
			}
			if store.shadowReportCalls != 0 || store.assistReportCalls != 0 {
				t.Fatalf("expected invalid query not to call store, got shadow=%d assist=%d calls", store.shadowReportCalls, store.assistReportCalls)
			}
		})
	}
}

func TestGetStructuredInterpreterShadowReportEndpointReturnsStoreError(t *testing.T) {
	sessionID := uuid.NewString()
	store := newFakeStoreWithPassengerAuthority()
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

func openAIInterpreterAssistEndpointMessage(sessionID string, status string, openAIIntent string, rejectReason string) Message {
	payload := map[string]interface{}{
		openAIInterpreterAssistMetadataKey: map[string]interface{}{
			"openai_assist_status": status,
			"validation_status":    status,
			"considered":           status != string(OpenAIInterpreterAssistSkipped),
			"accepted":             status == string(OpenAIInterpreterAssistAccepted),
			"local_intent":         string(StructuredIntentUnknown),
			"local_confidence":     0.4,
			"openai_intent":        openAIIntent,
			"openai_confidence":    0.76,
			"decision_source":      "openai_interpreter_assist",
		},
	}
	if rejectReason != "" {
		assist := payload[openAIInterpreterAssistMetadataKey].(map[string]interface{})
		assist["reject_reason"] = rejectReason
		assist["controlled_reason_codes"] = []string{rejectReason}
		if status == string(OpenAIInterpreterAssistSkipped) {
			assist["skip_reason"] = rejectReason
		}
	}
	return Message{
		ID:                uuid.NewString(),
		SessionID:         sessionID,
		Direction:         "OUTBOUND",
		Kind:              "TEXT",
		Body:              "runtime assist message body must not be serialized",
		NormalizedPayload: payload,
		ProcessingStatus:  "AUTOMATION_DRAFT",
		CreatedAt:         time.Now().UTC(),
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

	return s.fakeReportMessages(filter, fakeMessageHasStructuredInterpreterShadow), nil
}

func (s *fakeStore) ListOpenAIInterpreterAssistMessages(_ context.Context, filter StructuredInterpreterShadowReportFilter) ([]Message, error) {
	filter = normalizeStructuredInterpreterShadowReportFilter(filter)
	s.assistReportCalls++
	s.assistReportFilter = filter
	if filter.SessionID == "" {
		return nil, ErrShadowReportSessionRequired
	}
	if s.assistReportErr != nil {
		return nil, s.assistReportErr
	}

	return s.fakeReportMessages(filter, fakeMessageHasOpenAIInterpreterAssist), nil
}

func (s *fakeStore) fakeReportMessages(filter StructuredInterpreterShadowReportFilter, include func(Message) bool) []Message {
	if s.shadowReportMessages != nil {
		return paginateFakeReportMessages(filterFakeReportMessages(s.shadowReportMessages, filter, include), filter)
	}

	items := make([]Message, 0, len(s.messageOrder))
	for _, id := range s.messageOrder {
		message := s.messages[id]
		if !include(message) {
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
	return paginateFakeReportMessages(items, filter)
}

func filterFakeReportMessages(messages []Message, filter StructuredInterpreterShadowReportFilter, include func(Message) bool) []Message {
	items := make([]Message, 0, len(messages))
	for _, message := range messages {
		if filter.SessionID != "" && message.SessionID != filter.SessionID {
			continue
		}
		if !include(message) {
			continue
		}
		items = append(items, message)
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	return items
}

func paginateFakeReportMessages(messages []Message, filter StructuredInterpreterShadowReportFilter) []Message {
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

func fakeMessageHasOpenAIInterpreterAssist(message Message) bool {
	if _, ok := message.NormalizedPayload[openAIInterpreterAssistMetadataKey]; ok {
		return true
	}
	if _, ok := message.Payload[openAIInterpreterAssistMetadataKey]; ok {
		return true
	}
	return false
}
