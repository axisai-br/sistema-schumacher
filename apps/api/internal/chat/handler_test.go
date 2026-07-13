package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"schumacher-tur/api/internal/auth"
	"schumacher-tur/api/internal/shared/config"
)

func TestIngestMessageCreatesSessionAndMessage(t *testing.T) {
	store := newFakeStore()
	handler := NewHandler(NewService(store, config.Config{ChatDebounceWindowMS: 1500}))

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	body := bytes.NewBufferString(`{
		"channel":"whatsapp",
		"contact_key":"5511999999999",
		"customer_phone":"5511999999999",
		"customer_name":"Cliente Teste",
		"metadata":{"source":"shadow"},
		"message":{
			"direction":"inbound",
			"provider_message_id":"msg-1",
			"idempotency_key":"idem-1",
			"body":"oi",
			"payload":{"raw":"ok"}
		}
	}`)

	req := httptest.NewRequest(http.MethodPost, "/chat/messages/ingest", body)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, rec.Code)
	}

	var out IngestMessageResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if out.Idempotent {
		t.Fatalf("expected new message, got idempotent response")
	}
	if out.Session.ContactKey != "5511999999999" {
		t.Fatalf("expected contact key to be persisted")
	}
	if out.Message.ProviderMessageID != "msg-1" {
		t.Fatalf("expected provider message id to be persisted")
	}
	buffer, ok := out.Session.Metadata["buffer"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected buffer metadata to be present")
	}
	if got := asString(buffer["status"]); got != bufferStatusPending {
		t.Fatalf("expected buffer status %s, got %s", bufferStatusPending, got)
	}
	if got := asInt(buffer["message_count"]); got != 1 {
		t.Fatalf("expected one buffered message, got %d", got)
	}
	if out.Message.ProcessingStatus != "BUFFERED_PENDING" {
		t.Fatalf("expected buffered processing status, got %s", out.Message.ProcessingStatus)
	}
	if len(store.sessions) != 1 || len(store.messages) != 1 {
		t.Fatalf("expected one session and one message, got %d sessions and %d messages", len(store.sessions), len(store.messages))
	}
}

func TestIngestMessageReturnsExistingOnIdempotency(t *testing.T) {
	store := newFakeStore()
	handler := NewHandler(NewService(store, config.Config{ChatDebounceWindowMS: 1500}))

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	firstReq := httptest.NewRequest(http.MethodPost, "/chat/messages/ingest", bytes.NewBufferString(`{
		"contact_key":"5511999999999",
		"message":{"direction":"INBOUND","provider_message_id":"msg-dup","idempotency_key":"idem-dup","body":"oi"}
	}`))
	firstRec := httptest.NewRecorder()
	r.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusCreated {
		t.Fatalf("expected first call to create message, got %d", firstRec.Code)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/chat/messages/ingest", bytes.NewBufferString(`{
		"contact_key":"5511999999999",
		"message":{"direction":"INBOUND","provider_message_id":"msg-dup","idempotency_key":"idem-dup","body":"oi de novo"}
	}`))
	secondRec := httptest.NewRecorder()
	r.ServeHTTP(secondRec, secondReq)

	if secondRec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, secondRec.Code)
	}

	var out IngestMessageResult
	if err := json.Unmarshal(secondRec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !out.Idempotent {
		t.Fatalf("expected idempotent response")
	}
	if len(store.messages) != 1 {
		t.Fatalf("expected only one persisted message, got %d", len(store.messages))
	}
}

func TestListAndGetSessionAndMessages(t *testing.T) {
	store := newFakeStore()
	session, message := store.seedSessionWithMessage("5511888888888", "ola")
	handler := NewHandler(NewService(store, config.Config{ChatDebounceWindowMS: 1500}))

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	listReq := httptest.NewRequest(http.MethodGet, "/chat/sessions?channel=whatsapp&limit=10", nil)
	listRec := httptest.NewRecorder()
	r.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, listRec.Code)
	}

	var sessions []Session
	if err := json.Unmarshal(listRec.Body.Bytes(), &sessions); err != nil {
		t.Fatalf("unmarshal sessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != session.ID {
		t.Fatalf("expected listed session %s", session.ID)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/chat/sessions/"+session.ID, nil)
	getRec := httptest.NewRecorder()
	r.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, getRec.Code)
	}

	msgReq := httptest.NewRequest(http.MethodGet, "/chat/sessions/"+session.ID+"/messages?limit=5", nil)
	msgRec := httptest.NewRecorder()
	r.ServeHTTP(msgRec, msgReq)
	if msgRec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, msgRec.Code)
	}

	var messages []Message
	if err := json.Unmarshal(msgRec.Body.Bytes(), &messages); err != nil {
		t.Fatalf("unmarshal messages: %v", err)
	}
	if len(messages) != 1 || messages[0].ID != message.ID {
		t.Fatalf("expected listed message %s", message.ID)
	}
}

func TestGetCurrentDraftReturnsNotFoundWhenSessionHasNoDraft(t *testing.T) {
	store := newFakeStore()
	session, _ := store.seedSessionWithMessage("5511999999999", "oi")
	handler := NewHandler(NewService(store, config.Config{ChatDebounceWindowMS: 1500}))

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/chat/sessions/"+session.ID+"/draft", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}
}

func TestGetCurrentDraftReturnsObservabilityForGeneratedDraft(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Encontrei uma opcao para esse trecho.",
			Model:              "gpt-test",
			ProviderResponseID: "resp-draft-view-1",
		},
	}
	searcher := &fakeAvailabilitySearcher{
		enabled: true,
		result: AvailabilitySearchResult{
			Results: []AvailabilitySearchItem{
				{
					SegmentID:              "seg-1",
					TripID:                 "trip-1",
					RouteID:                "route-1",
					BoardStopID:            "board-1",
					AlightStopID:           "alight-1",
					OriginStopID:           "stop-origin-1",
					DestinationStopID:      "stop-destination-1",
					OriginDisplayName:      "Videira/SC",
					DestinationDisplayName: "Sao Luis/MA",
					OriginDepartTime:       "18:30",
					TripDate:               "2026-05-20",
					SeatsAvailable:         12,
					Price:                  250,
					Currency:               "BRL",
					Status:                 "ACTIVE",
					TripStatus:             "SCHEDULED",
				},
			},
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-draft-view-1",
			IdempotencyKey:    "idem-draft-view-1",
			Body:              "qual o valor de Videira/SC para Sao Luis/MA em 10/05?",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}
	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess message: %v", err)
	}
	if reprocessed.Draft == nil {
		t.Fatalf("expected draft to be generated")
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/chat/sessions/"+ingested.Session.ID+"/draft", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var out CurrentDraftResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal current draft: %v", err)
	}
	if out.Draft.ID != reprocessed.Draft.ID {
		t.Fatalf("expected draft id %s, got %s", reprocessed.Draft.ID, out.Draft.ID)
	}
	if out.DraftStatus != messageStatusAutomationDraft {
		t.Fatalf("expected draft status %s, got %s", messageStatusAutomationDraft, out.DraftStatus)
	}
	if out.AgentStatus != agentStatusDraftGenerated {
		t.Fatalf("expected agent status %s, got %s", agentStatusDraftGenerated, out.AgentStatus)
	}
	if out.Model != "gpt-test" {
		t.Fatalf("expected model gpt-test, got %s", out.Model)
	}
	if out.ProviderResponseID != "resp-draft-view-1" {
		t.Fatalf("expected provider response id resp-draft-view-1, got %s", out.ProviderResponseID)
	}
	if out.AutoSendStatus != draftAutoSendStatusEligible {
		t.Fatalf("expected auto_send_status %s, got %s", draftAutoSendStatusEligible, out.AutoSendStatus)
	}
	if len(out.AutoSendReasons) != 0 {
		t.Fatalf("expected no auto_send_reasons for safe availability draft, got %+v", out.AutoSendReasons)
	}
	if len(out.CurrentTurnMessageIDs) != 1 || out.CurrentTurnMessageIDs[0] == "" {
		t.Fatalf("expected current_turn_message_ids, got %+v", out.CurrentTurnMessageIDs)
	}
	if len(out.ToolNames) != 1 || out.ToolNames[0] != toolNameAvailabilitySearch {
		t.Fatalf("expected tool name %s, got %+v", toolNameAvailabilitySearch, out.ToolNames)
	}
	if out.ToolCallCount != 1 {
		t.Fatalf("expected tool call count 1, got %d", out.ToolCallCount)
	}
	if out.GeneratedAt == nil {
		t.Fatalf("expected generated_at to be present")
	}
	if out.LinkedReply != nil {
		t.Fatalf("expected no linked reply for generated draft")
	}
}

func TestBlockAutoSendOperationalTimePreferenceWithoutAvailabilityTool(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Confirmando: Fraiburgo → Monção em 18/05. Qual horário prefere — manhã, tarde ou noite?",
			Model:              "gpt-test",
			ProviderResponseID: "resp-operational-no-tool",
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-operational-no-tool",
			IdempotencyKey:    "idem-operational-no-tool",
			Body:              "Fraiburgo para monção 18/05",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}
	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess: %v", err)
	}
	if out.Draft == nil {
		t.Fatalf("expected draft")
	}
	if got := readInt(out.Draft.NormalizedPayload["tool_call_count"]); got != 0 {
		t.Fatalf("expected tool_call_count=0, got %d", got)
	}
	if got := readDraftAutoSendStatus(*out.Draft); got != draftAutoSendStatusReviewNeeded {
		t.Fatalf("expected auto_send_status %s, got %s", draftAutoSendStatusReviewNeeded, got)
	}
	reasons := readDraftAutoSendReasons(*out.Draft)
	if len(reasons) != 1 || reasons[0] != draftAutoSendReasonOperationalNoTool {
		t.Fatalf("expected auto_send reason %s, got %+v", draftAutoSendReasonOperationalNoTool, reasons)
	}
}

func TestGetCurrentDraftReturnsLinkedReplyForReviewedDraft(t *testing.T) {
	store := newFakeStore()
	ownerUserID := uuid.NewString()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Posso seguir com sua reserva.",
			Model:              "gpt-test",
			ProviderResponseID: "resp-draft-view-2",
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511888888888",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-draft-view-2",
			IdempotencyKey:    "idem-draft-view-2",
			Body:              "quero continuar minha reserva",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}
	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess message: %v", err)
	}
	if reprocessed.Draft == nil {
		t.Fatalf("expected draft to be generated")
	}
	if _, err := svc.RequestHandoff(context.Background(), RequestHandoffInput{
		SessionID:      ingested.Session.ID,
		RequestedBy:    "dashboard",
		AssignedUserID: ownerUserID,
	}); err != nil {
		t.Fatalf("request handoff: %v", err)
	}
	replied, err := svc.Reply(context.Background(), ReplyInput{
		SessionID:      ingested.Session.ID,
		OwnerUserID:    ownerUserID,
		DraftMessageID: reprocessed.Draft.ID,
		IdempotencyKey: "idem-reviewed-draft-1",
	})
	if err != nil {
		t.Fatalf("reply message: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/chat/sessions/"+ingested.Session.ID+"/draft", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var out CurrentDraftResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal current draft: %v", err)
	}
	if out.Draft.ID != reprocessed.Draft.ID {
		t.Fatalf("expected reviewed draft id %s, got %s", reprocessed.Draft.ID, out.Draft.ID)
	}
	if out.DraftStatus != messageStatusAutomationReviewed {
		t.Fatalf("expected draft status %s, got %s", messageStatusAutomationReviewed, out.DraftStatus)
	}
	if out.AgentStatus != agentStatusDraftReviewed {
		t.Fatalf("expected agent status %s, got %s", agentStatusDraftReviewed, out.AgentStatus)
	}
	if out.ReviewMode != "CONTROLLED" {
		t.Fatalf("expected review mode CONTROLLED, got %s", out.ReviewMode)
	}
	if out.ReviewAction != "APPROVED_AS_IS" {
		t.Fatalf("expected review action APPROVED_AS_IS, got %s", out.ReviewAction)
	}
	if out.ReviewedByUserID != ownerUserID {
		t.Fatalf("expected reviewed_by_user_id %s, got %s", ownerUserID, out.ReviewedByUserID)
	}
	if out.ReviewedAt == nil {
		t.Fatalf("expected reviewed_at to be present")
	}
	if out.AutoSendStatus != draftAutoSendStatusEligible {
		t.Fatalf("expected auto_send_status %s, got %s", draftAutoSendStatusEligible, out.AutoSendStatus)
	}
	if len(out.AutoSendReasons) != 0 {
		t.Fatalf("expected no auto_send_reasons for simple text draft, got %+v", out.AutoSendReasons)
	}
	if out.LinkedReply == nil {
		t.Fatalf("expected linked reply to be present")
	}
	if out.LinkedReply.ID != replied.Message.ID {
		t.Fatalf("expected linked reply id %s, got %s", replied.Message.ID, out.LinkedReply.ID)
	}
}

func TestGetCurrentDraftReturnsReviewRequiredForDocumentTurn(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Recebi seu documento e vou analisar.",
			Model:              "gpt-test",
			ProviderResponseID: "resp-draft-view-document-1",
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511777000000",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			Kind:              "DOCUMENT",
			ProviderMessageID: "msg-draft-view-document-1",
			IdempotencyKey:    "idem-draft-view-document-1",
			Body:              "RG frente",
			NormalizedPayload: map[string]interface{}{
				"document_file_name": "rg-frente.pdf",
				"document_mime_type": "application/pdf",
			},
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}
	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess message: %v", err)
	}
	if reprocessed.Draft == nil {
		t.Fatalf("expected draft to be generated")
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/chat/sessions/"+ingested.Session.ID+"/draft", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var out CurrentDraftResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal current draft: %v", err)
	}
	if out.AutoSendStatus != draftAutoSendStatusReviewNeeded {
		t.Fatalf("expected auto_send_status %s, got %s", draftAutoSendStatusReviewNeeded, out.AutoSendStatus)
	}
	if len(out.AutoSendReasons) != 1 || out.AutoSendReasons[0] != draftAutoSendReasonNonTextTurn {
		t.Fatalf("expected auto_send_reasons [%s], got %+v", draftAutoSendReasonNonTextTurn, out.AutoSendReasons)
	}
}

func TestGetCurrentDraftAllowsAutoSendForImageTurn(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Consegui identificar estes dados. Eles conferem?",
			Model:              "gpt-test",
			ProviderResponseID: "resp-draft-view-image-1",
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511777000000",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			Kind:              "IMAGE",
			ProviderMessageID: "msg-draft-view-image-1",
			IdempotencyKey:    "idem-draft-view-image-1",
			Body:              "",
			NormalizedPayload: map[string]interface{}{
				"image_url":       "https://files.example.test/rg-frente.jpg",
				"image_mime_type": "image/jpeg",
			},
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}
	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess message: %v", err)
	}
	if reprocessed.Draft == nil {
		t.Fatalf("expected draft to be generated")
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/chat/sessions/"+ingested.Session.ID+"/draft", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var out CurrentDraftResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal current draft: %v", err)
	}
	if out.AutoSendStatus != draftAutoSendStatusEligible {
		t.Fatalf("expected auto_send_status %s, got %s", draftAutoSendStatusEligible, out.AutoSendStatus)
	}
	if len(out.AutoSendReasons) != 0 {
		t.Fatalf("expected no auto_send_reasons for image turn, got %+v", out.AutoSendReasons)
	}
}

func TestReprocessExtractsDocumentImageBeforeGenericReply(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          `{"mode":"EXTRACTED","passengers":[{"name":"Joao Vitor Messias","document_type":"CPF","document":"066.456.481-03","confidence":0.91}]}`,
			Model:              "gpt-vision-test",
			ProviderResponseID: "resp-document-extract-1",
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5549988709047",
		Message: IngestMessagePayload{
			Direction:         "OUTBOUND",
			ProviderMessageID: "msg-doc-out-1",
			IdempotencyKey:    "idem-doc-out-1",
			Body:              "Pode enviar seu nome completo e o documento. Se for foto, envie frente e verso.",
		},
	}); err != nil {
		t.Fatalf("ingest outbound: %v", err)
	}
	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5549988709047",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			Kind:              "IMAGE",
			ProviderMessageID: "msg-doc-img-1",
			IdempotencyKey:    "idem-doc-img-1",
			NormalizedPayload: map[string]interface{}{
				"image_url":       "https://files.example.test/documento.jpg",
				"image_mime_type": "image/jpeg",
			},
		},
	})
	if err != nil {
		t.Fatalf("ingest image: %v", err)
	}

	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess message: %v", err)
	}
	if reprocessed.Draft == nil {
		t.Fatalf("expected draft to be generated")
	}
	if runner.calls != 1 {
		t.Fatalf("expected only document extraction run, got %d calls", runner.calls)
	}
	if len(runner.lastInput.CurrentTurnMedia) != 1 || runner.lastInput.CurrentTurnMedia[0].Kind != "IMAGE" {
		t.Fatalf("expected one image media item for document extraction, got %+v", runner.lastInput.CurrentTurnMedia)
	}
	if !strings.Contains(reprocessed.Draft.Body, "so para voce ou vai mais alguem") {
		t.Fatalf("expected passenger quantity clarification after extracting the CPF, got %q", reprocessed.Draft.Body)
	}
	if got := strings.TrimSpace(asString(reprocessed.Draft.NormalizedPayload["template_name"])); got == string(TemplateConfirmDocument) {
		t.Fatalf("unknown passenger quantity must not emit document confirmation, got %q", reprocessed.Draft.Body)
	}
	if len(reprocessed.ToolCalls) != 1 || reprocessed.ToolCalls[0].ToolName != toolNameDocumentExtract {
		t.Fatalf("expected document_extract tool call, got %+v", reprocessed.ToolCalls)
	}
	if readDraftAutoSendStatus(*reprocessed.Draft) != draftAutoSendStatusEligible {
		t.Fatalf("expected image document confirmation to be auto-send eligible, got %s", readDraftAutoSendStatus(*reprocessed.Draft))
	}
}

func TestReprocessExtractsDocumentImageDataURLBeforeGenericReply(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          `{"mode":"EXTRACTED","passengers":[{"name":"Maria Silva","document_type":"RG","document":"1234567","confidence":0.9}]}`,
			Model:              "gpt-vision-test",
			ProviderResponseID: "resp-document-extract-data-url-1",
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5549988709050",
		Message: IngestMessagePayload{
			Direction:         "OUTBOUND",
			ProviderMessageID: "msg-doc-data-out-1",
			IdempotencyKey:    "idem-doc-data-out-1",
			Body:              "Pode enviar seu nome completo e o documento. Se for foto, envie frente e verso.",
		},
	}); err != nil {
		t.Fatalf("ingest outbound: %v", err)
	}
	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5549988709050",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			Kind:              "IMAGE",
			ProviderMessageID: "msg-doc-data-img-1",
			IdempotencyKey:    "idem-doc-data-img-1",
			NormalizedPayload: map[string]interface{}{
				"image_data_url":  "data:image/jpeg;base64,/9j/2Q==",
				"image_mime_type": "image/jpeg",
			},
		},
	})
	if err != nil {
		t.Fatalf("ingest image: %v", err)
	}

	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess message: %v", err)
	}
	if reprocessed.Draft == nil {
		t.Fatalf("expected draft to be generated")
	}
	if runner.calls != 1 {
		t.Fatalf("expected only document extraction run, got %d calls", runner.calls)
	}
	if len(runner.lastInput.CurrentTurnMedia) != 1 || runner.lastInput.CurrentTurnMedia[0].URL != "data:image/jpeg;base64,/9j/2Q==" {
		t.Fatalf("expected one image data URL media item, got %+v", runner.lastInput.CurrentTurnMedia)
	}
	if len(reprocessed.ToolCalls) != 1 || reprocessed.ToolCalls[0].ToolName != toolNameDocumentExtract {
		t.Fatalf("expected document_extract tool call, got %+v", reprocessed.ToolCalls)
	}
	if got := readInt(reprocessed.Draft.NormalizedPayload["tool_call_count"]); got != 1 {
		t.Fatalf("expected tool_call_count=1, got %d", got)
	}
	if got := strings.TrimSpace(asString(reprocessed.Draft.NormalizedPayload["template_name"])); got == string(TemplateAskDocuments) {
		t.Fatalf("did not expect generic document request template, got %q", got)
	}
	if !strings.Contains(reprocessed.Draft.Body, "so para voce ou vai mais alguem") {
		t.Fatalf("expected passenger quantity clarification after extracting the RG, got %q", reprocessed.Draft.Body)
	}
	if got := strings.TrimSpace(asString(reprocessed.Draft.NormalizedPayload["template_name"])); got == string(TemplateConfirmDocument) {
		t.Fatalf("unknown passenger quantity must not emit document confirmation, got %q", reprocessed.Draft.Body)
	}
}

func TestReprocessExtractsDocumentImageDataURLDuringBookingDocumentCollection(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          `{"mode":"EXTRACTED","passengers":[{"name":"Claudecir Schumacher","document_type":"CPF","document":"529.982.247-25","confidence":0.93}]}`,
			Model:              "gpt-vision-test",
			ProviderResponseID: "resp-document-extract-booking-flow-1",
		},
	}
	creator := &fakeBookingCreator{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, creator)
	session := seedDocumentCollectionBookingHistory(t, store, "5549988709056")

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			Kind:              "IMAGE",
			ProviderMessageID: "msg-doc-booking-flow-img-1",
			IdempotencyKey:    "idem-doc-booking-flow-img-1",
			NormalizedPayload: map[string]interface{}{
				"image_data_url":  "data:image/jpeg;base64,/9j/2Q==",
				"image_mime_type": "image/jpeg",
			},
		},
	})
	if err != nil {
		t.Fatalf("ingest image: %v", err)
	}

	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess image document: %v", err)
	}
	if reprocessed.Draft == nil {
		t.Fatalf("expected draft to be generated")
	}
	if runner.calls != 1 {
		t.Fatalf("expected document extraction to run once, got %d calls", runner.calls)
	}
	if len(runner.lastInput.CurrentTurnMedia) != 1 || runner.lastInput.CurrentTurnMedia[0].URL != "data:image/jpeg;base64,/9j/2Q==" {
		t.Fatalf("expected image data URL media in document_extract, got %+v", runner.lastInput.CurrentTurnMedia)
	}
	if got := readInt(reprocessed.Draft.NormalizedPayload["tool_call_count"]); got != 1 {
		t.Fatalf("expected tool_call_count=1, got %d", got)
	}
	if len(reprocessed.ToolCalls) != 1 || reprocessed.ToolCalls[0].ToolName != toolNameDocumentExtract {
		t.Fatalf("expected one document_extract tool call, got %+v", reprocessed.ToolCalls)
	}
	if got := strings.TrimSpace(asString(reprocessed.Draft.NormalizedPayload["template_name"])); got == string(TemplateAskDocuments) {
		t.Fatalf("did not expect generic ask documents template, got %q", got)
	}
	if creator.calls != 0 {
		t.Fatalf("expected booking_create not to be called before document confirmation, got %d calls", creator.calls)
	}
	for _, call := range reprocessed.ToolCalls {
		if call.ToolName == toolNameBookingCreate {
			t.Fatalf("did not expect booking_create tool call on document image turn, got %+v", reprocessed.ToolCalls)
		}
	}
	if !strings.Contains(reprocessed.Draft.Body, "Claudecir Schumacher | CPF | 529.***.***-25") {
		t.Fatalf("expected extracted document confirmation, got %q", reprocessed.Draft.Body)
	}
}

func TestReprocessDocumentImageWithoutExtractableMediaReturnsObjectiveFallback(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: "generic LLM fallback", Model: "gpt-test"},
	}
	creator := &fakeBookingCreator{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, creator)
	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5549988709051",
		Message: IngestMessagePayload{
			Direction:         "OUTBOUND",
			ProviderMessageID: "msg-doc-unavailable-out-1",
			IdempotencyKey:    "idem-doc-unavailable-out-1",
			Body:              "Pode enviar seu nome completo e o documento. Se for foto, envie frente e verso.",
		},
	}); err != nil {
		t.Fatalf("ingest outbound: %v", err)
	}
	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5549988709051",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			Kind:              "IMAGE",
			ProviderMessageID: "msg-doc-unavailable-img-1",
			IdempotencyKey:    "idem-doc-unavailable-img-1",
			NormalizedPayload: map[string]interface{}{
				"image_mime_type":    "image/jpeg",
				"image_source":       "evolution_url_fallback",
				"image_base64_error": "evolution get base64 failed",
			},
		},
	})
	if err != nil {
		t.Fatalf("ingest image: %v", err)
	}

	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess message: %v", err)
	}
	if reprocessed.Draft == nil {
		t.Fatalf("expected draft to be generated")
	}
	if runner.calls != 0 {
		t.Fatalf("expected generic runner not to be called, got %d calls", runner.calls)
	}
	if creator.calls != 0 {
		t.Fatalf("expected booking_create not to be called, got %d calls", creator.calls)
	}
	if len(reprocessed.ToolCalls) != 0 {
		t.Fatalf("did not expect stored document_extract tool call for inaccessible media, got %+v", reprocessed.ToolCalls)
	}
	if got := readInt(reprocessed.Draft.NormalizedPayload["tool_call_count"]); got != 0 {
		t.Fatalf("expected tool_call_count=0, got %d", got)
	}
	if !strings.Contains(reprocessed.Draft.Body, "reenviar uma foto") || !strings.Contains(reprocessed.Draft.Body, "nome completo e CPF, RG ou CNH completo") {
		t.Fatalf("expected objective inaccessible-media fallback, got %q", reprocessed.Draft.Body)
	}
	toolContext := asMap(reprocessed.Draft.Payload["tool_context"])
	extract := asMap(toolContext[toolNameDocumentExtract])
	if got := strings.TrimSpace(asString(extract["failure_reason"])); got != "media_unavailable" {
		t.Fatalf("expected media_unavailable failure reason, got %q in %#v", got, extract)
	}
	if got := strings.TrimSpace(asString(reprocessed.Draft.NormalizedPayload["template_name"])); got == string(TemplateAskDocuments) {
		t.Fatalf("did not expect generic document request template, got %q", got)
	}
}

func TestReprocessImageAfterNewRouteQuestionDoesNotUseOldDocumentFallback(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: "Me diga a cidade de destino para eu verificar as opcoes.", Model: "gpt-test"},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5549988709057",
		Message: IngestMessagePayload{
			Direction:         "OUTBOUND",
			ProviderMessageID: "msg-old-doc-out-1",
			IdempotencyKey:    "idem-old-doc-out-1",
			Body:              "Pode enviar seu nome completo e o documento. Se for foto, envie frente e verso.",
		},
	}); err != nil {
		t.Fatalf("ingest old document request: %v", err)
	}
	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5549988709057",
		Message: IngestMessagePayload{
			Direction:         "OUTBOUND",
			ProviderMessageID: "msg-new-route-out-1",
			IdempotencyKey:    "idem-new-route-out-1",
			Body:              "Para qual cidade no Maranhao voce vai?",
		},
	}); err != nil {
		t.Fatalf("ingest newer route question: %v", err)
	}
	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5549988709057",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			Kind:              "IMAGE",
			ProviderMessageID: "msg-image-after-route-1",
			IdempotencyKey:    "idem-image-after-route-1",
			NormalizedPayload: map[string]interface{}{
				"image_mime_type": "image/jpeg",
			},
		},
	})
	if err != nil {
		t.Fatalf("ingest image after route question: %v", err)
	}

	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess image after route question: %v", err)
	}
	if reprocessed.Draft == nil {
		t.Fatalf("expected generic draft to be generated")
	}
	if len(reprocessed.ToolCalls) != 0 {
		t.Fatalf("did not expect document_extract tool call from stale document request, got %+v", reprocessed.ToolCalls)
	}
	if len(store.toolCallOrder) != 0 {
		t.Fatalf("did not expect stored tool calls from stale document request, got %+v", store.toolCallOrder)
	}
	toolContext := asMap(reprocessed.Draft.Payload["tool_context"])
	if extract := asMap(toolContext[toolNameDocumentExtract]); len(extract) != 0 {
		t.Fatalf("did not expect document_extract fallback context, got %#v", extract)
	}
	body := strings.Join(strings.Fields(foldChatText(reprocessed.Draft.Body)), " ")
	if strings.Contains(body, "reenviar uma foto") ||
		strings.Contains(body, "nome completo e cpf ou rg") ||
		strings.Contains(body, "documento") {
		t.Fatalf("did not expect document fallback reply after newer route question, got %q", reprocessed.Draft.Body)
	}
	if runner.calls > 0 && strings.Contains(runner.lastInput.SystemPrompt, "extrai dados de documentos brasileiros") {
		t.Fatalf("did not expect document_extract runner prompt, got %q", runner.lastInput.SystemPrompt)
	}
}

func TestReprocessRejectsPDFDocumentInsteadOfExtracting(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          `{"mode":"EXTRACTED","passengers":[{"name":"Maria Silva","document_type":"CPF","document":"123.456.789-09","confidence":0.94}]}`,
			Model:              "gpt-vision-test",
			ProviderResponseID: "resp-document-extract-pdf-1",
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5549988709048",
		Message: IngestMessagePayload{
			Direction:         "OUTBOUND",
			ProviderMessageID: "msg-pdf-route-out-1",
			IdempotencyKey:    "idem-pdf-route-out-1",
			Body:              "Para qual cidade no Maranhao voce vai?",
		},
	}); err != nil {
		t.Fatalf("ingest route outbound: %v", err)
	}
	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5549988709048",
		Message: IngestMessagePayload{
			Direction:         "OUTBOUND",
			ProviderMessageID: "msg-pdf-doc-out-1",
			IdempotencyKey:    "idem-pdf-doc-out-1",
			Body:              "Pode enviar seu nome completo e o documento. Se for foto, envie frente e verso.",
		},
	}); err != nil {
		t.Fatalf("ingest document request outbound: %v", err)
	}
	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5549988709048",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			Kind:              "DOCUMENT",
			ProviderMessageID: "msg-pdf-doc-1",
			IdempotencyKey:    "idem-pdf-doc-1",
			Body:              "Salvador",
			NormalizedPayload: map[string]interface{}{
				"document_data_url":  "data:application/pdf;base64,JVBERi0xLjQ=",
				"document_file_name": "rg-salvador.pdf",
				"document_mime_type": "application/pdf",
			},
		},
	})
	if err != nil {
		t.Fatalf("ingest PDF: %v", err)
	}

	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess message: %v", err)
	}
	if reprocessed.Draft == nil {
		t.Fatalf("expected draft to be generated")
	}
	if runner.calls != 0 {
		t.Fatalf("expected PDF not to be sent to document extraction runner, got %d calls", runner.calls)
	}
	if reprocessed.Draft.Body != unsupportedPDFDocumentReply {
		t.Fatalf("expected PDF rejection with photo/text alternatives, got %q", reprocessed.Draft.Body)
	}
	if strings.Contains(reprocessed.Draft.Body, unsupportedPackageSupportPhone) {
		t.Fatalf("did not expect unsupported package reply, got %q", reprocessed.Draft.Body)
	}
	if len(reprocessed.ToolCalls) != 0 {
		t.Fatalf("did not expect document_extract tool call for PDF, got %+v", reprocessed.ToolCalls)
	}
	if len(store.toolCallOrder) != 0 {
		t.Fatalf("did not expect stored tool_call for PDF, got %+v", store.toolCallOrder)
	}
	if readDraftAutoSendStatus(*reprocessed.Draft) != draftAutoSendStatusEligible {
		t.Fatalf("expected PDF guidance to be auto-send eligible, got %s", readDraftAutoSendStatus(*reprocessed.Draft))
	}
}

func TestReprocessRejectsPDFDocumentWithoutExtractableMedia(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          `{"mode":"EXTRACTED","passengers":[{"name":"Maria Silva","document_type":"CPF","document":"123.456.789-09","confidence":0.94}]}`,
			Model:              "gpt-vision-test",
			ProviderResponseID: "resp-document-extract-pdf-no-media-1",
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5549988709058",
		Message: IngestMessagePayload{
			Direction:         "OUTBOUND",
			ProviderMessageID: "msg-pdf-no-media-doc-out-1",
			IdempotencyKey:    "idem-pdf-no-media-doc-out-1",
			Body:              "Pode enviar seu nome completo e o documento. Se for foto, envie frente e verso.",
		},
	}); err != nil {
		t.Fatalf("ingest document request outbound: %v", err)
	}
	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5549988709058",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			Kind:              "DOCUMENT",
			ProviderMessageID: "msg-pdf-no-media-doc-1",
			IdempotencyKey:    "idem-pdf-no-media-doc-1",
			NormalizedPayload: map[string]interface{}{
				"document_file_name": "rg.pdf",
				"document_mime_type": "application/pdf",
			},
		},
	})
	if err != nil {
		t.Fatalf("ingest PDF without extractable media: %v", err)
	}

	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess PDF without extractable media: %v", err)
	}
	if reprocessed.Draft == nil {
		t.Fatalf("expected draft to be generated")
	}
	if runner.calls != 0 {
		t.Fatalf("expected PDF without extractable media not to call runner, got %d calls", runner.calls)
	}
	if len(reprocessed.ToolCalls) != 0 {
		t.Fatalf("did not expect stored document_extract tool call for PDF guardrail, got %+v", reprocessed.ToolCalls)
	}
	if len(store.toolCallOrder) != 0 {
		t.Fatalf("did not expect stored tool_call for PDF guardrail, got %+v", store.toolCallOrder)
	}
	if reprocessed.Draft.Body != unsupportedPDFDocumentReply {
		t.Fatalf("expected unsupported PDF reply, got %q", reprocessed.Draft.Body)
	}
	toolContext := asMap(reprocessed.Draft.Payload["tool_context"])
	extract := asMap(toolContext[toolNameDocumentExtract])
	if got := strings.TrimSpace(asString(extract["failure_reason"])); got != "unsupported_pdf" {
		t.Fatalf("expected unsupported_pdf failure reason, got %q in %#v", got, extract)
	}
	if got := strings.TrimSpace(asString(extract["failure_reason"])); got == "media_unavailable" {
		t.Fatalf("did not expect media_unavailable fallback for PDF, got %#v", extract)
	}
}

func TestReprocessConfirmsTextPassengerDocumentBeforeGenericRunner(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          buildUnsupportedPackageReply(),
			Model:              "gpt-test",
			ProviderResponseID: "resp-text-document-should-not-run",
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	session := seedDocumentCollectionBookingHistory(t, store, "5549988709049")

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-text-doc-1",
			IdempotencyKey:    "idem-text-doc-1",
			Body:              "Claudecir Schumacher 52998224725",
		},
	})
	if err != nil {
		t.Fatalf("ingest text document: %v", err)
	}

	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess text document: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected text passenger document to be handled before generic runner, got %d calls", runner.calls)
	}
	if reprocessed.Draft == nil {
		t.Fatalf("expected document confirmation draft")
	}
	if !strings.Contains(reprocessed.Draft.Body, "Claudecir Schumacher | CPF | 529.***.***-25") {
		t.Fatalf("expected passenger document confirmation, got %q", reprocessed.Draft.Body)
	}
	if strings.Contains(reprocessed.Draft.Body, unsupportedPackageSupportPhone) {
		t.Fatalf("did not expect unsupported package reply, got %q", reprocessed.Draft.Body)
	}
	if got := strings.TrimSpace(asString(reprocessed.Draft.NormalizedPayload["intent"])); got != string(IntentPassengerDocumentsProvided) {
		t.Fatalf("expected passenger documents intent, got %q", got)
	}
	if got := readDraftAutoSendStatus(*reprocessed.Draft); got != draftAutoSendStatusEligible {
		t.Fatalf("expected text document confirmation to be auto-send eligible, got %s", got)
	}
}

func TestReprocessAdministrativeNotesDuringPassengerDocumentsRoutesSupport(t *testing.T) {
	cases := []string{
		"queria verificar com você com relação à baixa das notas",
		"nota fiscal",
		"notas",
		"baixa das notas",
		"faturamento",
		"financeiro",
		"emissão de nota",
		"comprovante fiscal",
	}
	for i, body := range cases {
		t.Run(body, func(t *testing.T) {
			store := newFakeStore()
			runner := &fakeAgentRunner{
				enabled: true,
				result:  RunAgentResult{ReplyText: buildUnsupportedPackageReply(), Model: "gpt-test"},
			}
			creator := &fakeBookingCreator{enabled: true}
			paymentSearcher := &fakePaymentStatusSearcher{enabled: true}
			paymentCreator := &fakePaymentCreator{enabled: true}
			svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, creator, paymentSearcher, paymentCreator)
			session := seedDocumentCollectionBookingHistory(t, store, fmt.Sprintf("55499887093%02d", i))

			ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
				ContactKey: session.ContactKey,
				Message: IngestMessagePayload{
					Direction:         "INBOUND",
					ProviderMessageID: fmt.Sprintf("msg-admin-notes-%d", i),
					IdempotencyKey:    fmt.Sprintf("idem-admin-notes-%d", i),
					Body:              body,
				},
			})
			if err != nil {
				t.Fatalf("ingest administrative notes turn: %v", err)
			}

			reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
			if err != nil {
				t.Fatalf("reprocess administrative notes turn: %v", err)
			}

			assertAdministrativeNotesSupportDraft(t, reprocessed)
			if runner.calls != 0 {
				t.Fatalf("expected no runner/document_extract call, got %d", runner.calls)
			}
			if creator.calls != 0 {
				t.Fatalf("expected no booking_create call, got %d", creator.calls)
			}
			if paymentSearcher.calls != 0 {
				t.Fatalf("expected no payment_status call, got %d", paymentSearcher.calls)
			}
			if paymentCreator.calls != 0 {
				t.Fatalf("expected no payment_create call, got %d", paymentCreator.calls)
			}
			if len(reprocessed.ToolCalls) != 0 || len(store.toolCallOrder) != 0 {
				t.Fatalf("expected no tool calls, got result=%+v stored=%+v", reprocessed.ToolCalls, store.toolCallOrder)
			}
		})
	}
}

func TestReprocessAdministrativeNotesInCleanSessionRoutesSupportWithoutRunner(t *testing.T) {
	store := newFakeStore()
	logger := &fakeChatLogger{}
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: "Claro. Pode me informar os números das notas?", Model: "gpt-test"},
	}
	availability := &fakeAvailabilitySearcher{enabled: true}
	creator := &fakeBookingCreator{enabled: true}
	paymentSearcher := &fakePaymentStatusSearcher{enabled: true}
	paymentCreator := &fakePaymentCreator{enabled: true}
	svc := NewService(
		store,
		config.Config{ChatDebounceWindowMS: 1500},
		logger,
		runner,
		availability,
		creator,
		paymentSearcher,
		paymentCreator,
	)
	session, hello := store.seedSessionWithMessage("5549988709370", "oi")
	if _, err := store.UpdateMessage(context.Background(), UpdateMessageInput{
		MessageID:        hello.ID,
		ProcessingStatus: "PROCESSED",
	}); err != nil {
		t.Fatalf("mark hello processed: %v", err)
	}

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-admin-notes-clean",
			IdempotencyKey:    "idem-admin-notes-clean",
			Body:              "queria verificar com você com relação à baixa das notas",
		},
	})
	if err != nil {
		t.Fatalf("ingest clean-session administrative notes turn: %v", err)
	}

	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess clean-session administrative notes turn: %v", err)
	}

	assertAdministrativeNotesSupportDraft(t, reprocessed)
	if runner.calls != 0 {
		t.Fatalf("expected no free-form runner call, got %d", runner.calls)
	}
	if availability.calls != 0 {
		t.Fatalf("expected no availability_search call, got %d", availability.calls)
	}
	if creator.calls != 0 {
		t.Fatalf("expected no booking_create call, got %d", creator.calls)
	}
	if paymentSearcher.calls != 0 {
		t.Fatalf("expected no payment_status call, got %d", paymentSearcher.calls)
	}
	if paymentCreator.calls != 0 {
		t.Fatalf("expected no payment_create call, got %d", paymentCreator.calls)
	}
	if len(reprocessed.ToolCalls) != 0 || len(store.toolCallOrder) != 0 {
		t.Fatalf("expected no tool calls, got result=%+v stored=%+v", reprocessed.ToolCalls, store.toolCallOrder)
	}
	if !logger.contains("event=intent_router_decision") ||
		!logger.contains("intent_source="+administrativeNotesSupportDecisionSource) ||
		!logger.contains("template_name="+string(TemplateHumanSupportInfo)) {
		t.Fatalf("expected administrative intent_router_decision log, got %+v", logger.entries)
	}
}

func TestReprocessAdministrativeNotesDuringBookingPendingRoutesSupport(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: buildUnsupportedPackageReply(), Model: "gpt-test"},
	}
	creator := &fakeBookingCreator{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, creator)
	session := seedBookingPendingPhase(t, store)

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-admin-notes-booking-pending",
			IdempotencyKey:    "idem-admin-notes-booking-pending",
			Body:              "queria verificar a nota fiscal",
		},
	})
	if err != nil {
		t.Fatalf("ingest booking-pending administrative notes turn: %v", err)
	}

	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess booking-pending administrative notes turn: %v", err)
	}

	assertAdministrativeNotesSupportDraft(t, reprocessed)
	if runner.calls != 0 {
		t.Fatalf("expected no runner/document_extract call, got %d", runner.calls)
	}
	if creator.calls != 0 {
		t.Fatalf("expected no booking_create call, got %d", creator.calls)
	}
	if len(reprocessed.ToolCalls) != 0 || len(store.toolCallOrder) != 0 {
		t.Fatalf("expected no tool calls, got result=%+v stored=%+v", reprocessed.ToolCalls, store.toolCallOrder)
	}
}

func TestReprocessPassengerDocumentTextStillUsesDocumentFlowWithAdministrativeGate(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: buildUnsupportedPackageReply(), Model: "gpt-test"},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	session := seedDocumentCollectionBookingHistory(t, store, "5549988709360")

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-invalid-text-doc-admin-gate",
			IdempotencyKey:    "idem-invalid-text-doc-admin-gate",
			Body:              "João Silva CPF 00000000000",
		},
	})
	if err != nil {
		t.Fatalf("ingest invalid passenger document text: %v", err)
	}

	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess invalid passenger document text: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected text document flow before generic runner, got %d calls", runner.calls)
	}
	if reprocessed.Draft == nil {
		t.Fatal("expected passenger document draft")
	}
	if got := strings.TrimSpace(asString(reprocessed.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskDocuments) {
		t.Fatalf("expected ask documents template, got %q payload=%+v body=%q", got, reprocessed.Draft.NormalizedPayload, reprocessed.Draft.Body)
	}
	if got := strings.TrimSpace(asString(reprocessed.Draft.NormalizedPayload["action"])); got != "ask_valid_passenger_cpf" {
		t.Fatalf("expected invalid CPF document action, got %q payload=%+v body=%q", got, reprocessed.Draft.NormalizedPayload, reprocessed.Draft.Body)
	}
	if strings.Contains(reprocessed.Draft.Body, "notas ou financeiro") ||
		strings.TrimSpace(asString(reprocessed.Draft.NormalizedPayload["template_name"])) == string(TemplateHumanSupportInfo) {
		t.Fatalf("did not expect administrative support reply for passenger document text: payload=%+v body=%q", reprocessed.Draft.NormalizedPayload, reprocessed.Draft.Body)
	}
}

func TestReprocessAlreadySentStillUsesPassengerDocumentFallbackWithAdministrativeGate(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: buildUnsupportedPackageReply(), Model: "gpt-test"},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	session := seedDocumentCollectionBookingHistory(t, store, "5549988709361")

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-already-sent-admin-gate",
			IdempotencyKey:    "idem-already-sent-admin-gate",
			Body:              "já mandei acima",
		},
	})
	if err != nil {
		t.Fatalf("ingest already-sent document reply: %v", err)
	}

	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess already-sent document reply: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected already-sent fallback before generic runner, got %d calls", runner.calls)
	}
	if reprocessed.Draft == nil {
		t.Fatal("expected passenger document fallback draft")
	}
	body := strings.Join(strings.Fields(foldChatText(reprocessed.Draft.Body)), " ")
	if !strings.Contains(body, "nao consegui identificar os dados do passageiro") {
		t.Fatalf("expected already-sent document fallback, got %q", reprocessed.Draft.Body)
	}
	if got := strings.TrimSpace(asString(reprocessed.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskDocuments) {
		t.Fatalf("expected ask documents template, got %q payload=%+v body=%q", got, reprocessed.Draft.NormalizedPayload, reprocessed.Draft.Body)
	}
}

func TestReprocessAdministrativeNotesMediaDuringPassengerDocumentsRunsDocumentExtract(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          `{"mode":"EXTRACTED","passengers":[{"name":"Claudecir Schumacher","document_type":"CPF","document":"529.982.247-25","confidence":0.93}]}`,
			Model:              "gpt-vision-test",
			ProviderResponseID: "resp-document-admin-notes-media",
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	session := seedDocumentCollectionBookingHistory(t, store, "5549988709362")

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			Kind:              "IMAGE",
			ProviderMessageID: "msg-admin-notes-image-doc",
			IdempotencyKey:    "idem-admin-notes-image-doc",
			Body:              "nota fiscal",
			NormalizedPayload: map[string]interface{}{
				"image_data_url":  "data:image/jpeg;base64,/9j/2Q==",
				"image_mime_type": "image/jpeg",
			},
		},
	})
	if err != nil {
		t.Fatalf("ingest administrative notes media turn: %v", err)
	}

	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess administrative notes media turn: %v", err)
	}
	if runner.calls != 1 {
		t.Fatalf("expected document_extract runner call for media, got %d", runner.calls)
	}
	if len(reprocessed.ToolCalls) != 1 || reprocessed.ToolCalls[0].ToolName != toolNameDocumentExtract {
		t.Fatalf("expected document_extract tool call, got %+v", reprocessed.ToolCalls)
	}
	if reprocessed.Draft == nil {
		t.Fatal("expected document extraction draft")
	}
	if got := strings.TrimSpace(asString(reprocessed.Draft.NormalizedPayload["template_name"])); got == string(TemplateHumanSupportInfo) {
		t.Fatalf("media document turn must not become administrative support: payload=%+v body=%q", reprocessed.Draft.NormalizedPayload, reprocessed.Draft.Body)
	}
	if !strings.Contains(reprocessed.Draft.Body, "Claudecir Schumacher | CPF | 529.***.***-25") {
		t.Fatalf("expected extracted document confirmation, got %q", reprocessed.Draft.Body)
	}
}

func assertAdministrativeNotesSupportDraft(t *testing.T, out ReprocessResult) {
	t.Helper()
	if out.Draft == nil {
		t.Fatal("expected administrative support draft")
	}
	if !strings.Contains(out.Draft.Body, "notas ou financeiro") ||
		!strings.Contains(out.Draft.Body, "suporte da Schumacher Tur") ||
		strings.Contains(out.Draft.Body, "Recebi os dados do passageiro") {
		t.Fatalf("unexpected administrative support body: %q", out.Draft.Body)
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got != string(TemplateHumanSupportInfo) {
		t.Fatalf("expected human support info template, got %q payload=%+v body=%q", got, out.Draft.NormalizedPayload, out.Draft.Body)
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["intent"])); got != string(IntentHumanSupportInfoQuestion) {
		t.Fatalf("expected human support info intent, got %q payload=%+v body=%q", got, out.Draft.NormalizedPayload, out.Draft.Body)
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["action"])); got == string(BookingNextAskPassengerDocuments) {
		t.Fatalf("did not expect ask passenger documents action, got payload=%+v body=%q", out.Draft.NormalizedPayload, out.Draft.Body)
	}
	if _, ok := out.Draft.Payload["selected_option_index"]; ok {
		t.Fatalf("did not expect selected_option_index in request payload: %+v", out.Draft.Payload)
	}
	if _, ok := out.Draft.NormalizedPayload["selected_option_index"]; ok {
		t.Fatalf("did not expect selected_option_index in response payload: %+v", out.Draft.NormalizedPayload)
	}
	if _, ok := out.Draft.Payload[selectedAvailabilityResultPayloadKey]; ok {
		t.Fatalf("did not expect selected availability in request payload: %+v", out.Draft.Payload)
	}
	if _, ok := out.Draft.NormalizedPayload[selectedAvailabilityResultPayloadKey]; ok {
		t.Fatalf("did not expect selected availability in response payload: %+v", out.Draft.NormalizedPayload)
	}
}

func TestReprocessInlinePassengerDocumentsWithIncompleteSecondNameAsksResendWithNameAndCPF(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: buildUnsupportedPackageReply(), Model: "gpt-test"},
	}
	creator := &fakeBookingCreator{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, creator)
	session := seedInlinePassengerDocumentBookingHistory(t, store, "5549988709060")

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-inline-docs-partial-1",
			IdempotencyKey:    "idem-inline-docs-partial-1",
			Body:              "joão vitor messias 06645648103 ivoneide 46643591104",
		},
	})
	if err != nil {
		t.Fatalf("ingest inline passenger documents: %v", err)
	}

	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess inline passenger documents: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected inline documents to be handled before generic runner, got %d calls", runner.calls)
	}
	if creator.calls != 0 {
		t.Fatalf("expected booking_create not to be called before confirmation, got %d calls", creator.calls)
	}
	if reprocessed.Draft == nil {
		t.Fatalf("expected partial passenger document draft")
	}
	body := strings.Join(strings.Fields(strings.ToLower(reprocessed.Draft.Body)), " ")
	if !strings.Contains(body, "joão vitor messias") ||
		!strings.Contains(body, "reenvie ivoneide") ||
		!strings.Contains(body, "nome completo + cpf") ||
		!strings.Contains(body, "mesma linha") ||
		strings.Contains(body, "documentos dos 2 passageiros faltantes") ||
		strings.Contains(body, "tem crianca") {
		t.Fatalf("unexpected partial passenger reply: %q", reprocessed.Draft.Body)
	}
	if got := strings.TrimSpace(asString(reprocessed.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskDocuments) {
		t.Fatalf("expected ask documents template, got %q", got)
	}
}

func TestReprocessInlinePassengerDocumentsWithOnlyOneOfTwoAsksMissingBeforeConfirmation(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: buildUnsupportedPackageReply(), Model: "gpt-test"},
	}
	creator := &fakeBookingCreator{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, creator)
	session := seedInlinePassengerDocumentBookingHistory(t, store, "5549988709062")

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-inline-docs-one-of-two-1",
			IdempotencyKey:    "idem-inline-docs-one-of-two-1",
			Body:              "João Vitor Messias 06645648103",
		},
	})
	if err != nil {
		t.Fatalf("ingest partial passenger documents: %v", err)
	}

	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess partial passenger documents: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected partial documents to be handled before generic runner, got %d calls", runner.calls)
	}
	if creator.calls != 0 {
		t.Fatalf("expected booking_create not to be called before confirmation, got %d calls", creator.calls)
	}
	if reprocessed.Draft == nil {
		t.Fatalf("expected missing passenger document draft")
	}
	body := strings.Join(strings.Fields(foldChatText(reprocessed.Draft.Body)), " ")
	if !strings.Contains(body, "documento de 1 passageiro") ||
		strings.Contains(body, "documentos dos 2 passageiros") ||
		strings.Contains(body, "consegui identificar estes dados") ||
		strings.Contains(body, "posso prosseguir e criar a reserva") {
		t.Fatalf("unexpected missing passenger reply: %q", reprocessed.Draft.Body)
	}
	if got := strings.TrimSpace(asString(reprocessed.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskDocuments) {
		t.Fatalf("expected ask documents template, got %q", got)
	}
}

func TestReprocessInlinePassengerDocumentsWithLapChildPendingAsksAssignmentBeforeConfirmation(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: buildUnsupportedPackageReply(), Model: "gpt-test"},
	}
	creator := &fakeBookingCreator{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, creator)
	session := seedInlinePassengerDocumentBookingHistory(t, store, "5549988709063")

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-inline-docs-lap-pending-1",
			IdempotencyKey:    "idem-inline-docs-lap-pending-1",
			Body:              "João Vitor Messias 06645648103 Ivoneide Pereira 46643591104",
		},
	})
	if err != nil {
		t.Fatalf("ingest complete passenger documents with lap child pending: %v", err)
	}

	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess complete passenger documents with lap child pending: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected lap child assignment to be handled before generic runner, got %d calls", runner.calls)
	}
	if creator.calls != 0 {
		t.Fatalf("expected booking_create not to be called before lap child assignment, got %d calls", creator.calls)
	}
	if reprocessed.Draft == nil {
		t.Fatalf("expected lap child assignment draft")
	}
	body := strings.Join(strings.Fields(foldChatText(reprocessed.Draft.Body)), " ")
	if !strings.Contains(body, "qual deles e a crianca de ate 5 anos") ||
		!strings.Contains(body, "joao vitor messias") ||
		!strings.Contains(body, "ivoneide pereira") ||
		strings.Contains(body, "consegui identificar estes dados") ||
		strings.Contains(body, "eles conferem") {
		t.Fatalf("unexpected lap child assignment reply: %q", reprocessed.Draft.Body)
	}
	if got := strings.TrimSpace(asString(reprocessed.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskLapChildAssignment) {
		t.Fatalf("expected lap child assignment template, got %q", got)
	}
}

func TestReprocessLapChildAssignmentDraftsDocumentConfirmationBeforeBookingCreate(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: buildUnsupportedPackageReply(), Model: "gpt-test"},
	}
	creator := &fakeBookingCreator{
		enabled: true,
		result: BookingCreateResult{
			Mode:            "created",
			BookingID:       "BK-LAP123",
			ReservationCode: "LAP12345",
			Status:          "PENDING",
			TotalAmount:     950,
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, creator)
	session := seedInlinePassengerDocumentBookingHistory(t, store, "5549988709065")

	ingestedDocs, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-inline-docs-lap-flow-1",
			IdempotencyKey:    "idem-inline-docs-lap-flow-1",
			Body:              "João Vitor Messias 06645648103 Ivoneide Pereira 46643591104",
		},
	})
	if err != nil {
		t.Fatalf("ingest complete passenger documents with lap child pending: %v", err)
	}
	reprocessedDocs, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingestedDocs.Session.ID})
	if err != nil {
		t.Fatalf("reprocess complete passenger documents with lap child pending: %v", err)
	}
	if reprocessedDocs.Draft == nil {
		t.Fatalf("expected lap child assignment draft")
	}
	if got := strings.TrimSpace(asString(reprocessedDocs.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskLapChildAssignment) {
		t.Fatalf("expected lap child assignment template, got %q", got)
	}
	if creator.calls != 0 {
		t.Fatalf("expected booking_create not to be called before lap child assignment, got %d calls", creator.calls)
	}
	seedSentOutboundFromDraft(t, store, ingestedDocs.Session.ID, reprocessedDocs.Draft)

	ingestedAssignment, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-inline-docs-lap-flow-2",
			IdempotencyKey:    "idem-inline-docs-lap-flow-2",
			Body:              "2",
		},
	})
	if err != nil {
		t.Fatalf("ingest lap child assignment: %v", err)
	}
	reprocessedAssignment, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingestedAssignment.Session.ID})
	if err != nil {
		t.Fatalf("reprocess lap child assignment: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected lap child flow to be handled before generic runner, got %d calls", runner.calls)
	}
	if creator.calls != 0 {
		t.Fatalf("expected booking_create not to be called on lap child assignment turn, got %d calls", creator.calls)
	}
	if len(reprocessedAssignment.ToolCalls) != 0 {
		t.Fatalf("expected no tool calls on lap child assignment turn, got %+v", reprocessedAssignment.ToolCalls)
	}
	if reprocessedAssignment.Draft == nil {
		t.Fatalf("expected document confirmation draft after lap child assignment")
	}
	body := strings.Join(strings.Fields(strings.ToLower(reprocessedAssignment.Draft.Body)), " ")
	if !strings.Contains(body, "consegui identificar estes dados") ||
		!strings.Contains(body, "joão vitor messias") ||
		!strings.Contains(body, "2. ivoneide pereira") ||
		!strings.Contains(body, "crianca de ate 5 anos") {
		t.Fatalf("unexpected document confirmation after lap child assignment: %q", reprocessedAssignment.Draft.Body)
	}
	if got := strings.TrimSpace(asString(reprocessedAssignment.Draft.NormalizedPayload["template_name"])); got != string(TemplateConfirmDocument) {
		t.Fatalf("expected confirm document template after lap child assignment, got %q", got)
	}
	assignmentSnapshot := assertDraftBookingPassengerSnapshotPersistence(t, reprocessedAssignment.Draft)
	if len(assignmentSnapshot.Passengers) != 2 ||
		assignmentSnapshot.Passengers[0].Passenger.IsLapChild ||
		!assignmentSnapshot.Passengers[1].Passenger.IsLapChild ||
		assignmentSnapshot.Passengers[0].LapChildSource != bookingPassengerLapChildSourceExplicitAssignment ||
		assignmentSnapshot.Passengers[1].LapChildSource != bookingPassengerLapChildSourceExplicitAssignment {
		t.Fatalf("expected explicit passenger-2 assignment in canonical snapshot, got %+v", assignmentSnapshot)
	}
	seedSentOutboundFromDraft(t, store, ingestedAssignment.Session.ID, reprocessedAssignment.Draft)
	messagesAfterAssignment, err := store.ListMessages(context.Background(), ingestedAssignment.Session.ID, ListMessagesFilter{})
	if err != nil {
		t.Fatalf("list messages after lap child assignment: %v", err)
	}
	if !lastAssistantAskedDocumentConfirmation(messagesAfterAssignment) {
		bodies := make([]string, 0, len(messagesAfterAssignment))
		for _, message := range messagesAfterAssignment {
			bodies = append(bodies, strings.TrimSpace(message.Direction)+":"+strings.TrimSpace(message.Body))
		}
		t.Fatalf("expected saved document confirmation before final confirmation, got history=%q", strings.Join(bodies, " | "))
	}

	ingestedConfirmation, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-inline-docs-lap-flow-3",
			IdempotencyKey:    "idem-inline-docs-lap-flow-3",
			Body:              "sim",
		},
	})
	if err != nil {
		t.Fatalf("ingest document confirmation: %v", err)
	}
	reprocessedConfirmation, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingestedConfirmation.Session.ID})
	if err != nil {
		t.Fatalf("reprocess document confirmation: %v", err)
	}
	if creator.calls != 1 {
		var draftBody string
		if reprocessedConfirmation.Draft != nil {
			draftBody = reprocessedConfirmation.Draft.Body
		}
		t.Fatalf("expected booking_create to be called once after document confirmation, got %d calls; draft=%q tool_calls=%+v", creator.calls, draftBody, reprocessedConfirmation.ToolCalls)
	}
	if len(reprocessedConfirmation.ToolCalls) != 1 || reprocessedConfirmation.ToolCalls[0].ToolName != toolNameBookingCreate {
		t.Fatalf("expected one booking_create tool call after confirmation, got %+v", reprocessedConfirmation.ToolCalls)
	}
	if len(creator.lastInput.Passengers) != 2 ||
		creator.lastInput.Passengers[0].IsLapChild ||
		!creator.lastInput.Passengers[1].IsLapChild {
		t.Fatalf("expected passenger 2 marked as lap child on booking_create input, got %+v", creator.lastInput.Passengers)
	}
	if creator.lastInput.SelectedOptionIndex != 1 ||
		creator.lastInput.TripID != "trip-inline-docs-1" ||
		creator.lastInput.BoardStopID != "board-inline-docs-1" ||
		creator.lastInput.AlightStopID != "alight-inline-docs-1" {
		t.Fatalf("expected booking_create to keep original option 1 after lap child assignment, got %+v", creator.lastInput)
	}
}

func TestReprocessInlinePassengerDocumentsWithIncompleteFirstNameDoesNotUseWrongOrdinal(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: buildUnsupportedPackageReply(), Model: "gpt-test"},
	}
	creator := &fakeBookingCreator{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, creator)
	session := seedInlinePassengerDocumentBookingHistory(t, store, "5549988709064")

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-inline-docs-partial-first-1",
			IdempotencyKey:    "idem-inline-docs-partial-first-1",
			Body:              "joão 06645648103 Ivoneide Pereira 46643591104",
		},
	})
	if err != nil {
		t.Fatalf("ingest partial-first inline passenger documents: %v", err)
	}

	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess partial-first inline passenger documents: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected partial-first documents to be handled before generic runner, got %d calls", runner.calls)
	}
	if creator.calls != 0 {
		t.Fatalf("expected booking_create not to be called before complete passenger details, got %d calls", creator.calls)
	}
	if reprocessed.Draft == nil {
		t.Fatalf("expected partial passenger document draft")
	}
	body := strings.Join(strings.Fields(foldChatText(reprocessed.Draft.Body)), " ")
	if !strings.Contains(body, "ivoneide pereira") ||
		!strings.Contains(body, "reenvie joao") ||
		!strings.Contains(body, "nome completo + cpf") ||
		!strings.Contains(body, "mesma linha") ||
		strings.Contains(body, "passageiro 2") {
		t.Fatalf("unexpected partial-first reply: %q", reprocessed.Draft.Body)
	}
	if got := strings.TrimSpace(asString(reprocessed.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskDocuments) {
		t.Fatalf("expected ask documents template, got %q", got)
	}
}

func TestReprocessInlinePassengerDocumentsAfterPartialResendDraftsConfirmation(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: buildUnsupportedPackageReply(), Model: "gpt-test"},
	}
	creator := &fakeBookingCreator{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, creator)
	session := seedInlinePassengerDocumentBookingHistoryNoChild(t, store, "5549988709066")

	ingestedPartial, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-inline-docs-partial-resend-1",
			IdempotencyKey:    "idem-inline-docs-partial-resend-1",
			Body:              "joão 06645648103 Ivoneide Pereira 46643591104",
		},
	})
	if err != nil {
		t.Fatalf("ingest partial inline passenger documents: %v", err)
	}
	reprocessedPartial, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingestedPartial.Session.ID})
	if err != nil {
		t.Fatalf("reprocess partial inline passenger documents: %v", err)
	}
	if creator.calls != 0 {
		t.Fatalf("expected booking_create not to be called before complete passenger details, got %d calls", creator.calls)
	}
	if reprocessedPartial.Draft == nil {
		t.Fatalf("expected partial passenger document draft")
	}
	partialBody := strings.Join(strings.Fields(foldChatText(reprocessedPartial.Draft.Body)), " ")
	if !strings.Contains(partialBody, "reenvie joao") ||
		!strings.Contains(partialBody, "nome completo + cpf") ||
		strings.Contains(partialBody, "passageiro 2") {
		t.Fatalf("unexpected partial resend reply: %q", reprocessedPartial.Draft.Body)
	}
	seedSentOutboundFromDraft(t, store, ingestedPartial.Session.ID, reprocessedPartial.Draft)

	ingestedResend, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-inline-docs-partial-resend-2",
			IdempotencyKey:    "idem-inline-docs-partial-resend-2",
			Body:              "João Vitor Messias 06645648103",
		},
	})
	if err != nil {
		t.Fatalf("ingest completed partial passenger: %v", err)
	}
	reprocessedResend, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingestedResend.Session.ID})
	if err != nil {
		t.Fatalf("reprocess completed partial passenger: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected completed partial passenger to be handled before generic runner, got %d calls", runner.calls)
	}
	if creator.calls != 0 {
		t.Fatalf("expected booking_create not to be called before document confirmation, got %d calls", creator.calls)
	}
	if len(reprocessedResend.ToolCalls) != 0 {
		t.Fatalf("expected no tool calls before document confirmation, got %+v", reprocessedResend.ToolCalls)
	}
	if reprocessedResend.Draft == nil {
		t.Fatalf("expected document confirmation draft")
	}
	confirmationBody := strings.Join(strings.Fields(foldChatText(reprocessedResend.Draft.Body)), " ")
	if !strings.Contains(confirmationBody, "consegui identificar estes dados") ||
		!strings.Contains(confirmationBody, "joao vitor messias") ||
		!strings.Contains(confirmationBody, "ivoneide pereira") ||
		strings.Contains(confirmationBody, "documentos dos 2 passageiros faltantes") {
		t.Fatalf("unexpected confirmation after completed partial passenger: %q", reprocessedResend.Draft.Body)
	}
	if got := strings.TrimSpace(asString(reprocessedResend.Draft.NormalizedPayload["template_name"])); got != string(TemplateConfirmDocument) {
		t.Fatalf("expected confirm document template, got %q", got)
	}
}

func TestReprocessAlreadySentReevaluatesPreviousInlinePassengerDocuments(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: buildUnsupportedPackageReply(), Model: "gpt-test"},
	}
	creator := &fakeBookingCreator{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, creator)
	session := seedInlinePassengerDocumentBookingHistory(t, store, "5549988709061")
	now := time.Now().UTC()
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        session.ID,
		Direction:        "INBOUND",
		Kind:             "TEXT",
		ProcessingStatus: "PROCESSED",
		ReceivedAt:       now.Add(-90 * time.Second),
		Body:             "joão vitor messias 06645648103 ivoneide pereira 46643591104",
	}); err != nil {
		t.Fatalf("seed previous inline documents: %v", err)
	}
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        session.ID,
		Direction:        "OUTBOUND",
		Kind:             "TEXT",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       now.Add(-60 * time.Second),
		Body:             "Agora pode enviar os nomes completos e os documentos dos 2 passageiros faltantes.",
	}); err != nil {
		t.Fatalf("seed repeated generic document request: %v", err)
	}

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-inline-docs-already-sent-1",
			IdempotencyKey:    "idem-inline-docs-already-sent-1",
			Body:              "mas já enviei",
		},
	})
	if err != nil {
		t.Fatalf("ingest already sent reply: %v", err)
	}

	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess already sent reply: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected already-sent reply to be handled before generic runner, got %d calls", runner.calls)
	}
	if creator.calls != 0 {
		t.Fatalf("expected booking_create not to be called before confirmation, got %d calls", creator.calls)
	}
	if reprocessed.Draft == nil {
		t.Fatalf("expected lap child assignment draft")
	}
	body := strings.Join(strings.Fields(foldChatText(reprocessed.Draft.Body)), " ")
	if !strings.Contains(body, "qual deles e a crianca de ate 5 anos") ||
		!strings.Contains(body, "joao vitor messias") ||
		!strings.Contains(body, "ivoneide pereira") ||
		strings.Contains(body, "consegui identificar estes dados") ||
		strings.Contains(body, "eles conferem") ||
		strings.Contains(body, "documentos dos 2 passageiros faltantes") {
		t.Fatalf("unexpected already-sent lap child reply: %q", reprocessed.Draft.Body)
	}
	if got := strings.TrimSpace(asString(reprocessed.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskLapChildAssignment) {
		t.Fatalf("expected lap child assignment template, got %q", got)
	}
}

func TestReprocessCPFOnlyPassengerDocumentAsksNameBeforeGenericRunner(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          buildUnsupportedPackageReply(),
			Model:              "gpt-test",
			ProviderResponseID: "resp-cpf-only-should-not-run",
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	session := seedDocumentCollectionBookingHistory(t, store, "5549988709052")

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-cpf-only-1",
			IdempotencyKey:    "idem-cpf-only-1",
			Body:              "cpf 52998224725",
		},
	})
	if err != nil {
		t.Fatalf("ingest cpf-only document: %v", err)
	}

	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess cpf-only document: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected cpf-only passenger document to be handled before generic runner, got %d calls", runner.calls)
	}
	if reprocessed.Draft == nil {
		t.Fatalf("expected draft asking passenger name")
	}
	if !strings.Contains(strings.ToLower(reprocessed.Draft.Body), "nome completo") {
		t.Fatalf("expected CPF-only reply to ask for passenger name, got %q", reprocessed.Draft.Body)
	}
	if strings.Contains(reprocessed.Draft.Body, unsupportedPackageSupportPhone) {
		t.Fatalf("did not expect unsupported package reply, got %q", reprocessed.Draft.Body)
	}
}

func TestReprocessConfirmsCorrectedCPFBeforeGenericRunner(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          buildUnsupportedPackageReply(),
			Model:              "gpt-test",
			ProviderResponseID: "resp-correction-should-not-run",
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	session := seedDocumentCorrectionBookingHistory(t, store, "5549988709050")

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-text-correction-1",
			IdempotencyKey:    "idem-text-correction-1",
			Body:              "o nome está certo, mas quero que use o cpf 52998224725",
		},
	})
	if err != nil {
		t.Fatalf("ingest correction: %v", err)
	}

	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess correction: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected correction to be handled before generic runner, got %d calls", runner.calls)
	}
	if reprocessed.Draft == nil {
		t.Fatalf("expected corrected document confirmation draft")
	}
	if !strings.Contains(reprocessed.Draft.Body, "Claudecir Schumacher | CPF | 529.***.***-25") {
		t.Fatalf("expected corrected CPF confirmation, got %q", reprocessed.Draft.Body)
	}
	if strings.Contains(reprocessed.Draft.Body, "2817314") || strings.Contains(reprocessed.Draft.Body, unsupportedPackageSupportPhone) {
		t.Fatalf("expected old RG and unsupported reply to be absent, got %q", reprocessed.Draft.Body)
	}
}

func TestReprocessInvalidCorrectedCPFAsksCorrectionBeforeGenericRunner(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          buildUnsupportedPackageReply(),
			Model:              "gpt-test",
			ProviderResponseID: "resp-invalid-cpf-should-not-run",
		},
	}
	creator := &fakeBookingCreator{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, creator)
	session := seedDocumentCorrectionBookingHistory(t, store, "5549988709053")

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-invalid-cpf-correction-1",
			IdempotencyKey:    "idem-invalid-cpf-correction-1",
			Body:              "o nome está certo, mas quero que use o cpf 12345678901",
		},
	})
	if err != nil {
		t.Fatalf("ingest invalid correction: %v", err)
	}

	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess invalid correction: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected invalid correction to be handled before generic runner, got %d calls", runner.calls)
	}
	if creator.calls != 0 {
		t.Fatalf("expected invalid correction not to create booking, got %d calls", creator.calls)
	}
	if reprocessed.Draft == nil {
		t.Fatalf("expected invalid CPF correction draft")
	}
	body := strings.Join(strings.Fields(strings.ToLower(reprocessed.Draft.Body)), " ")
	if !strings.Contains(body, "cpf") || !strings.Contains(body, "invalido") {
		t.Fatalf("expected invalid CPF correction reply, got %q", reprocessed.Draft.Body)
	}
	if strings.Contains(reprocessed.Draft.Body, unsupportedPackageSupportPhone) {
		t.Fatalf("did not expect unsupported package reply, got %q", reprocessed.Draft.Body)
	}
}

func TestReprocessCreatesBookingAfterCorrectedDocumentConfirmation(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"},
	}
	creator := &fakeBookingCreator{
		enabled: true,
		result: BookingCreateResult{
			Mode:            "created",
			BookingID:       "BK-CORRECTED",
			ReservationCode: "CORR1234",
			Status:          "PENDING",
			TotalAmount:     950,
			RemainderAmount: 950,
			Passengers: []BookingCreatePassengerResult{
				{Name: "Claudecir Schumacher", Document: "52998224725", DocumentType: "CPF", Phone: "5549988709051"},
			},
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, creator)
	session := seedDocumentCorrectionBookingHistory(t, store, "5549988709051")
	now := time.Now().UTC()
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        session.ID,
		Direction:        "INBOUND",
		Kind:             "TEXT",
		ProcessingStatus: "PROCESSED",
		ReceivedAt:       now.Add(-70 * time.Second),
		Body:             "Claudecir Schumacher 52998224725",
	}); err != nil {
		t.Fatalf("seed correction inbound: %v", err)
	}
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        session.ID,
		Direction:        "OUTBOUND",
		Kind:             "TEXT",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       now.Add(-60 * time.Second),
		Body:             "Consegui identificar estes dados. Eles conferem? Posso prosseguir e criar a reserva?\n1. Claudecir Schumacher | CPF | 529.***.***-25",
	}); err != nil {
		t.Fatalf("seed corrected confirmation outbound: %v", err)
	}

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-correction-confirm-1",
			IdempotencyKey:    "idem-correction-confirm-1",
			Body:              "sim",
		},
	})
	if err != nil {
		t.Fatalf("ingest correction confirmation: %v", err)
	}

	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess correction confirmation: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected booking create template before generic runner, got %d calls", runner.calls)
	}
	if creator.calls != 1 {
		t.Fatalf("expected one booking create call, got %d", creator.calls)
	}
	if len(creator.lastInput.Passengers) != 1 ||
		creator.lastInput.Passengers[0].DocumentType != "CPF" ||
		creator.lastInput.Passengers[0].Document != "52998224725" {
		t.Fatalf("expected corrected CPF passenger on booking input, got %+v", creator.lastInput.Passengers)
	}
	if reprocessed.Draft == nil || !strings.Contains(reprocessed.Draft.Body, "valor integral") {
		t.Fatalf("expected payment preference draft after booking creation, got %+v", reprocessed.Draft)
	}
}

func TestReprocessCreatesBookingAfterPartialDocumentConfirmationWithVisibleCPF(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"},
	}
	creator := &fakeBookingCreator{
		enabled: true,
		result: BookingCreateResult{
			Mode:            "created",
			BookingID:       "BK-PARTIAL",
			ReservationCode: "PART1234",
			Status:          "PENDING",
			TotalAmount:     950,
			RemainderAmount: 950,
			Passengers: []BookingCreatePassengerResult{
				{Name: "Joao Vitor Messias", Document: syntheticValidCPFForTests(), DocumentType: "CPF", Phone: "5549988709056"},
			},
		},
	}
	openAI := &fakeOpenAIInterpreter{
		enabled: true,
		result: OpenAIStructuredInterpreterRunResult{
			Interpretation: StructuredInterpretation{
				Intent:      StructuredIntentBookingCancelRequest,
				TurnMeaning: TurnMeaningNewRequest,
				Confidence:  0.99,
				Source:      "openai_structured",
			},
			ProviderResponseID: "resp_shadow_booking_divergent",
		},
	}
	svc := NewService(store, config.Config{
		ChatDebounceWindowMS:               1500,
		ChatOpenAIInterpreterShadowEnabled: true,
	}, runner, creator, openAI)
	session := seedDocumentCollectionBookingHistory(t, store, "5549988709056")
	now := time.Now().UTC()
	result := DocumentExtractResult{
		Mode:                   "PARTIAL",
		ExpectedPassengerCount: 1,
		MediaCount:             1,
		Passengers: []DocumentExtractPassenger{
			{
				Name:         "Joao Vitor Messias",
				DocumentType: "RG",
				Document:     "numero nao identificado",
				CPF:          syntheticValidCPFForTests(),
				Confidence:   0.72,
			},
		},
	}
	toolContext := map[string]interface{}{
		toolNameDocumentExtract: buildDocumentExtractResponsePayload(result),
	}
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        session.ID,
		Direction:        "OUTBOUND",
		Kind:             "TEXT",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       now.Add(-1 * time.Minute),
		Body:             buildConfirmExtractedDocumentReply(result),
		Payload: map[string]interface{}{
			"tool_context": toolContext,
		},
		NormalizedPayload: map[string]interface{}{
			"tool_context": toolContext,
		},
	}); err != nil {
		t.Fatalf("seed partial document confirmation outbound: %v", err)
	}

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-partial-confirm-1",
			IdempotencyKey:    "idem-partial-confirm-1",
			Body:              "ta certo",
		},
	})
	if err != nil {
		t.Fatalf("ingest partial confirmation: %v", err)
	}

	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess partial confirmation: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected booking create template before generic runner, got %d calls", runner.calls)
	}
	if creator.calls != 1 {
		t.Fatalf("expected booking_create after partial confirmation, got %d calls", creator.calls)
	}
	if len(creator.lastInput.Passengers) != 1 ||
		creator.lastInput.Passengers[0].DocumentType != "CPF" ||
		creator.lastInput.Passengers[0].Document != syntheticValidCPFForTests() ||
		creator.lastInput.Passengers[0].CPF != syntheticValidCPFForTests() {
		t.Fatalf("expected visible CPF as booking passenger document, got %+v", creator.lastInput.Passengers)
	}
	if reprocessed.Draft == nil {
		t.Fatalf("expected payment preference draft after booking creation")
	}
	if strings.Contains(reprocessed.Draft.Body, "Ainda falta o documento") {
		t.Fatalf("did not expect ask-documents fallback after confirmed partial document, got %q", reprocessed.Draft.Body)
	}
	if len(reprocessed.ToolCalls) != 1 || reprocessed.ToolCalls[0].ToolName != toolNameBookingCreate {
		t.Fatalf("expected one booking_create tool call, got %+v", reprocessed.ToolCalls)
	}
	if openAI.calls != 1 {
		t.Fatalf("expected OpenAI shadow to be called once, got %d", openAI.calls)
	}
	shadow := mustStructuredInterpreterShadowMap(t, reprocessed.Memory[structuredInterpreterShadowKey])
	openAIShadow := mustNestedMap(t, shadow, "openai")
	if got := asString(openAIShadow["status"]); got != string(StructuredInterpreterShadowValid) {
		t.Fatalf("expected shadow status %q, got %q", StructuredInterpreterShadowValid, got)
	}
	agreement := mustNestedMap(t, shadow, "agreement")
	if got, ok := agreement["intent"].(bool); !ok || got {
		t.Fatalf("expected divergent OpenAI intent not to agree, got %#v", agreement["intent"])
	}
}

func TestReprocessPartialDocumentConfirmationWithoutUsableDocumentAsksObjectiveDocument(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"},
	}
	creator := &fakeBookingCreator{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, creator)
	session := seedDocumentCollectionBookingHistory(t, store, "5549988709057")
	now := time.Now().UTC()
	result := DocumentExtractResult{
		Mode:                   "PARTIAL",
		ExpectedPassengerCount: 1,
		MediaCount:             1,
		Passengers: []DocumentExtractPassenger{
			{
				Name:         "Joao Vitor Messias",
				DocumentType: "RG",
				Document:     "numero nao identificado",
				Confidence:   0.72,
			},
		},
	}
	toolContext := map[string]interface{}{
		toolNameDocumentExtract: buildDocumentExtractResponsePayload(result),
	}
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        session.ID,
		Direction:        "OUTBOUND",
		Kind:             "TEXT",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       now.Add(-1 * time.Minute),
		Body:             buildConfirmExtractedDocumentReply(result),
		Payload: map[string]interface{}{
			"tool_context": toolContext,
		},
		NormalizedPayload: map[string]interface{}{
			"tool_context": toolContext,
		},
	}); err != nil {
		t.Fatalf("seed incomplete partial document confirmation outbound: %v", err)
	}

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-partial-incomplete-confirm-1",
			IdempotencyKey:    "idem-partial-incomplete-confirm-1",
			Body:              "ta certo",
		},
	})
	if err != nil {
		t.Fatalf("ingest incomplete partial confirmation: %v", err)
	}

	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess incomplete partial confirmation: %v", err)
	}
	if creator.calls != 0 {
		t.Fatalf("expected incomplete partial document not to call booking_create, got %d calls", creator.calls)
	}
	if len(reprocessed.ToolCalls) != 0 {
		t.Fatalf("did not expect tool calls for incomplete partial document confirmation, got %+v", reprocessed.ToolCalls)
	}
	if reprocessed.Draft == nil {
		t.Fatalf("expected objective document request draft")
	}
	body := strings.Join(strings.Fields(reprocessed.Draft.Body), " ")
	if !strings.Contains(body, "CPF, RG ou CNH completo") {
		t.Fatalf("expected objective CPF/RG/CNH request, got %q", reprocessed.Draft.Body)
	}
	if strings.Contains(strings.Join(strings.Fields(foldChatText(reprocessed.Draft.Body)), " "), "confirme") {
		t.Fatalf("did not expect confirmation-only guidance for unsafe partial document, got %q", reprocessed.Draft.Body)
	}
}

func TestReprocessCreatesBookingAfterTranscribedAudioDocumentConfirmation(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: buildUnsupportedPackageReply(), Model: "gpt-test"},
	}
	creator := &fakeBookingCreator{
		enabled: true,
		result: BookingCreateResult{
			Mode:            "created",
			BookingID:       "BK-AUDIO",
			ReservationCode: "AUD1234",
			Status:          "PENDING",
			TotalAmount:     950,
			RemainderAmount: 950,
			Passengers: []BookingCreatePassengerResult{
				{Name: "Claudecir Schumacher", Document: "52998224725", DocumentType: "CPF", Phone: "5549988709054"},
			},
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, creator)
	session := seedDocumentCollectionBookingHistory(t, store, "5549988709054")
	now := time.Now().UTC()
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        session.ID,
		Direction:        "INBOUND",
		Kind:             "TEXT",
		ProcessingStatus: "PROCESSED",
		ReceivedAt:       now.Add(-70 * time.Second),
		Body:             "Claudecir Schumacher 52998224725",
	}); err != nil {
		t.Fatalf("seed passenger document inbound: %v", err)
	}
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        session.ID,
		Direction:        "OUTBOUND",
		Kind:             "TEXT",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       now.Add(-60 * time.Second),
		Body:             "Consegui identificar estes dados. Eles conferem? Posso prosseguir e criar a reserva?\n1. Claudecir Schumacher | CPF | 529.***.***-25",
	}); err != nil {
		t.Fatalf("seed document confirmation outbound: %v", err)
	}

	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:         session.ID,
		Direction:         "INBOUND",
		Kind:              "AUDIO",
		ProcessingStatus:  "RECEIVED",
		ProviderMessageID: "msg-audio-booking-confirm-1",
		IdempotencyKey:    "idem-audio-booking-confirm-1",
		ReceivedAt:        now,
		Body:              "",
		NormalizedPayload: map[string]interface{}{
			"transcription_status": "COMPLETED",
			"transcription_text":   "Sim, tá certo.",
		},
	}); err != nil {
		t.Fatalf("seed audio confirmation inbound: %v", err)
	}

	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess audio confirmation: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected booking create template before generic runner, got %d calls", runner.calls)
	}
	if creator.calls != 1 {
		t.Fatalf("expected one booking create call, got %d", creator.calls)
	}
	if reprocessed.Draft == nil || strings.Contains(reprocessed.Draft.Body, unsupportedPackageSupportPhone) {
		t.Fatalf("expected booking-created draft without unsupported package reply, got %+v", reprocessed.Draft)
	}
	if got := strings.TrimSpace(creator.lastInput.TripID); got != "trip-doc-text-1" {
		t.Fatalf("expected booking trip from pending draft, got %+v", creator.lastInput)
	}
}

func TestReprocessBookingPendingAlreadySentStaysInDocumentFlow(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: buildUnsupportedPackageReply(), Model: "gpt-test"},
	}
	creator := &fakeBookingCreator{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, creator)
	session := seedDocumentCollectionBookingHistory(t, store, "5549988709055")
	now := time.Now().UTC()
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        session.ID,
		Direction:        "INBOUND",
		Kind:             "TEXT",
		ProcessingStatus: "PROCESSED",
		ReceivedAt:       now.Add(-70 * time.Second),
		Body:             "Joao Vitor Messias 06645648103",
	}); err != nil {
		t.Fatalf("seed passenger document inbound: %v", err)
	}
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        session.ID,
		Direction:        "OUTBOUND",
		Kind:             "TEXT",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       now.Add(-60 * time.Second),
		Body:             "Consegui identificar estes dados. Eles conferem? Posso prosseguir e criar a reserva?\n1. Joao Vitor Messias | CPF | 066.***.***-03",
	}); err != nil {
		t.Fatalf("seed document confirmation outbound: %v", err)
	}

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-already-sent-1",
			IdempotencyKey:    "idem-already-sent-1",
			Body:              "Mas eu já enviei.",
		},
	})
	if err != nil {
		t.Fatalf("ingest already sent reply: %v", err)
	}

	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess already sent reply: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected booking document flow before generic runner, got %d calls", runner.calls)
	}
	if creator.calls != 0 {
		t.Fatalf("expected no booking create without explicit confirmation, got %d calls", creator.calls)
	}
	if reprocessed.Draft == nil {
		t.Fatalf("expected document-flow draft")
	}
	if strings.Contains(reprocessed.Draft.Body, unsupportedPackageSupportPhone) {
		t.Fatalf("did not expect unsupported package reply, got %q", reprocessed.Draft.Body)
	}
	folded := strings.Join(strings.Fields(foldChatText(reprocessed.Draft.Body)), " ")
	if !strings.Contains(folded, "falta confirmar") || !strings.Contains(folded, "criar a reserva") {
		t.Fatalf("expected confirmation-missing reply, got %q", reprocessed.Draft.Body)
	}
}

func seedDocumentCollectionBookingHistory(t *testing.T, store *fakeStore, contactKey string) Session {
	t.Helper()
	now := time.Now().UTC()
	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:       "WHATSAPP",
		ContactKey:    contactKey,
		CustomerPhone: contactKey,
		CustomerName:  "Claudecir",
		LastMessageAt: &now,
		LastInboundAt: &now,
	})
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}

	availabilityPayload := buildAvailabilityToolResponsePayload(AvailabilitySearchResult{
		Filter: AvailabilitySearchInput{
			Origin:      "Moncao/MA",
			Destination: "Fraiburgo/SC",
			Qty:         1,
			Limit:       5,
		},
		Results: []AvailabilitySearchItem{
			{
				TripID:                 "trip-doc-text-1",
				BoardStopID:            "board-doc-text-1",
				AlightStopID:           "alight-doc-text-1",
				OriginDisplayName:      "Moncao/MA",
				DestinationDisplayName: "Fraiburgo/SC",
				OriginDepartTime:       "09:00",
				TripDate:               "2026-05-11",
				Price:                  950,
				Currency:               "BRL",
				PackageName:            packageToSantaCatarina,
			},
		},
	})
	if _, err := store.SaveAgentDraft(context.Background(), SaveAgentDraftInput{
		SessionID:        session.ID,
		IdempotencyKey:   "draft-doc-text-availability-" + contactKey,
		Body:             "Encontrei uma opcao para Moncao/MA -> Fraiburgo/SC.",
		SenderName:       "SHABAS",
		ProcessingStatus: messageStatusAutomationSent,
		Payload: map[string]interface{}{
			"tool_context": map[string]interface{}{
				toolNameAvailabilitySearch: availabilityPayload,
			},
		},
		NormalizedPayload: map[string]interface{}{
			"tool_context": map[string]interface{}{
				toolNameAvailabilitySearch: availabilityPayload,
			},
		},
		RecordedAt: now.Add(-8 * time.Minute),
	}); err != nil {
		t.Fatalf("seed availability draft: %v", err)
	}

	messages := []CreateMessageInput{
		{SessionID: session.ID, Direction: "INBOUND", Kind: "TEXT", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-7 * time.Minute), Body: "1"},
		{SessionID: session.ID, Direction: "OUTBOUND", Kind: "TEXT", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-6 * time.Minute), Body: "A passagem e so para voce ou vai mais alguem junto? Tem crianca de 5 anos ou menos?"},
		{SessionID: session.ID, Direction: "INBOUND", Kind: "TEXT", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-5 * time.Minute), Body: "so eu"},
		{SessionID: session.ID, Direction: "OUTBOUND", Kind: "TEXT", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-4 * time.Minute), Body: "Tem crianca de 5 anos ou menos viajando?"},
		{SessionID: session.ID, Direction: "INBOUND", Kind: "TEXT", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-3 * time.Minute), Body: "nao"},
		{SessionID: session.ID, Direction: "OUTBOUND", Kind: "TEXT", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-2 * time.Minute), Body: "Perfeito. Agora pode enviar seu nome completo e o documento. Se preferir, pode mandar foto legivel do documento."},
	}
	for _, message := range messages {
		if _, err := store.CreateMessage(context.Background(), message); err != nil {
			t.Fatalf("seed message: %v", err)
		}
	}
	return session
}

func seedSentOutboundFromDraft(t *testing.T, store *fakeStore, sessionID string, draft *Message) {
	t.Helper()
	if draft == nil {
		t.Fatalf("expected draft to seed sent outbound")
	}
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:         sessionID,
		Direction:         "OUTBOUND",
		Kind:              "TEXT",
		ProcessingStatus:  messageStatusAutomationSent,
		ReceivedAt:        time.Now().UTC(),
		Body:              draft.Body,
		Payload:           draft.Payload,
		NormalizedPayload: draft.NormalizedPayload,
	}); err != nil {
		t.Fatalf("seed sent outbound from draft: %v", err)
	}
}

func seedInlinePassengerDocumentBookingHistory(t *testing.T, store *fakeStore, contactKey string) Session {
	t.Helper()
	return seedInlinePassengerDocumentBookingHistoryWithPassengerReply(t, store, contactKey, "eu e minha filha de 4 anos")
}

func seedInlinePassengerDocumentBookingHistoryNoChild(t *testing.T, store *fakeStore, contactKey string) Session {
	t.Helper()
	return seedInlinePassengerDocumentBookingHistoryWithPassengerReply(t, store, contactKey, "2 pessoas e nenhuma crianca")
}

func seedInlinePassengerDocumentBookingHistoryWithPassengerReply(t *testing.T, store *fakeStore, contactKey string, passengerReply string) Session {
	t.Helper()
	now := time.Now().UTC()
	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:        "WHATSAPP",
		ContactKey:     contactKey,
		CustomerPhone:  contactKey,
		CustomerName:   "Joao",
		LastMessageAt:  &now,
		LastOutboundAt: &now,
	})
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}

	availabilityPayload := buildAvailabilityToolResponsePayload(AvailabilitySearchResult{
		Filter: AvailabilitySearchInput{
			Origin:      "Santa Ines/MA",
			Destination: "Fraiburgo/SC",
			Qty:         2,
			Limit:       5,
		},
		Results: []AvailabilitySearchItem{
			{
				TripID:                 "trip-inline-docs-1",
				BoardStopID:            "board-inline-docs-1",
				AlightStopID:           "alight-inline-docs-1",
				OriginDisplayName:      "Santa Ines/MA",
				DestinationDisplayName: "Fraiburgo/SC",
				OriginDepartTime:       "12:00",
				TripDate:               "2026-06-22",
				Price:                  950,
				Currency:               "BRL",
				PackageName:            packageToSantaCatarina,
			},
			{
				TripID:                 "trip-inline-docs-2",
				BoardStopID:            "board-inline-docs-2",
				AlightStopID:           "alight-inline-docs-2",
				OriginDisplayName:      "Santa Ines/MA",
				DestinationDisplayName: "Fraiburgo/SC",
				OriginDepartTime:       "18:00",
				TripDate:               "2026-06-24",
				Price:                  950,
				Currency:               "BRL",
				PackageName:            packageToSantaCatarina,
			},
		},
	})
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        session.ID,
		Direction:        "OUTBOUND",
		Kind:             "TEXT",
		Body:             "Encontrei estas opcoes para Santa Ines/MA -> Fraiburgo/SC.",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       now.Add(-6 * time.Minute),
		Payload: map[string]interface{}{
			"tool_context": map[string]interface{}{
				toolNameAvailabilitySearch: availabilityPayload,
			},
		},
		NormalizedPayload: map[string]interface{}{
			"tool_context": map[string]interface{}{
				toolNameAvailabilitySearch: availabilityPayload,
			},
		},
	}); err != nil {
		t.Fatalf("seed availability: %v", err)
	}

	messages := []CreateMessageInput{
		{SessionID: session.ID, Direction: "INBOUND", Kind: "TEXT", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-5 * time.Minute), Body: "primeira opcao"},
		{SessionID: session.ID, Direction: "OUTBOUND", Kind: "TEXT", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-4 * time.Minute), Body: "A passagem e so para voce ou vai mais alguem junto? Tem crianca de 5 anos ou menos?"},
		{SessionID: session.ID, Direction: "INBOUND", Kind: "TEXT", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-3 * time.Minute), Body: passengerReply},
		{SessionID: session.ID, Direction: "OUTBOUND", Kind: "TEXT", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-2 * time.Minute), Body: "Perfeito. Agora pode enviar os nomes completos e os documentos dos 2 passageiros faltantes (CPF ou RG)."},
	}
	for _, message := range messages {
		if _, err := store.CreateMessage(context.Background(), message); err != nil {
			t.Fatalf("seed message: %v", err)
		}
	}
	return session
}

func seedSequentialSoloChildDocumentBookingHistory(t *testing.T, store *fakeStore, contactKey string) Session {
	t.Helper()
	now := time.Now().UTC()
	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:        "WHATSAPP",
		ContactKey:     contactKey,
		CustomerPhone:  contactKey,
		CustomerName:   "Joao",
		LastMessageAt:  &now,
		LastOutboundAt: &now,
	})
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}

	availabilityPayload := buildAvailabilityToolResponsePayload(AvailabilitySearchResult{
		Filter: AvailabilitySearchInput{Origin: "Santa Ines/MA", Destination: "Fraiburgo/SC", Qty: 1, Limit: 5},
		Results: []AvailabilitySearchItem{{
			TripID: "trip-sequential-docs-1", BoardStopID: "board-sequential-docs-1", AlightStopID: "alight-sequential-docs-1",
			OriginDisplayName: "Santa Ines/MA", DestinationDisplayName: "Fraiburgo/SC", OriginDepartTime: "12:00", TripDate: "2026-06-22", Price: 950, Currency: "BRL",
		}},
	})
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID: session.ID, Direction: "OUTBOUND", Kind: "TEXT", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-8 * time.Minute),
		Body:              "Encontrei esta opcao para Santa Ines/MA -> Fraiburgo/SC.",
		Payload:           map[string]interface{}{"tool_context": map[string]interface{}{toolNameAvailabilitySearch: availabilityPayload}},
		NormalizedPayload: map[string]interface{}{"tool_context": map[string]interface{}{toolNameAvailabilitySearch: availabilityPayload}},
	}); err != nil {
		t.Fatalf("seed availability: %v", err)
	}

	messages := []CreateMessageInput{
		{SessionID: session.ID, Direction: "INBOUND", Kind: "TEXT", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-7 * time.Minute), Body: "primeira opcao"},
		{SessionID: session.ID, Direction: "OUTBOUND", Kind: "TEXT", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-6 * time.Minute), Body: "A passagem e so para voce ou vai mais alguem junto?"},
		{SessionID: session.ID, Direction: "INBOUND", Kind: "TEXT", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-5 * time.Minute), Body: "so pra mim"},
		{SessionID: session.ID, Direction: "OUTBOUND", Kind: "TEXT", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-4 * time.Minute), Body: askChildUnder5Reply},
		{SessionID: session.ID, Direction: "INBOUND", Kind: "TEXT", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-3 * time.Minute), Body: "sim"},
		{SessionID: session.ID, Direction: "OUTBOUND", Kind: "TEXT", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-2 * time.Minute), Body: "Perfeito. Agora pode enviar os nomes completos e os documentos dos 2 passageiros faltantes (CPF, RG ou CNH completos)."},
	}
	for _, message := range messages {
		if _, err := store.CreateMessage(context.Background(), message); err != nil {
			t.Fatalf("seed message: %v", err)
		}
	}
	return session
}

func reprocessSequentialDocumentImage(t *testing.T, svc *Service, contactKey string, suffix string) ReprocessResult {
	t.Helper()
	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: contactKey,
		Message: IngestMessagePayload{
			Direction: "INBOUND", Kind: "IMAGE", ProviderMessageID: "msg-sequential-document-" + suffix, IdempotencyKey: "idem-sequential-document-" + suffix,
			NormalizedPayload: map[string]interface{}{"image_url": "https://files.example.test/" + suffix + ".jpg", "image_mime_type": "image/jpeg"},
		},
	})
	if err != nil {
		t.Fatalf("ingest sequential document image: %v", err)
	}
	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess sequential document image: %v", err)
	}
	if reprocessed.Draft == nil {
		t.Fatalf("expected sequential document draft")
	}
	return reprocessed
}

func assertSequentialDraftDocumentCount(t *testing.T, draft *Message, expected int) DocumentExtractResult {
	t.Helper()
	snapshot := assertDraftBookingPassengerSnapshotPersistence(t, draft)
	passengers := bookingPassengersFromCanonicalSnapshot(*snapshot)
	if len(passengers) != expected {
		t.Fatalf("expected %d canonical passengers, got %+v", expected, passengers)
	}
	result := documentExtractResultFromBookingDraft(BookingDraftContext{
		PassengerDetails:      passengers,
		PassengerDetailsCount: len(passengers),
	})
	result.ExpectedPassengerCount = snapshot.ExpectedDocumentCount
	return result
}

func assertDraftBookingPassengerSnapshotPersistence(t *testing.T, draft *Message) *bookingPassengerSnapshot {
	t.Helper()
	if draft == nil {
		t.Fatal("expected draft with canonical passenger snapshot")
	}
	var parsed *bookingPassengerSnapshot
	for label, payload := range map[string]map[string]interface{}{
		"payload":            draft.Payload,
		"normalized_payload": draft.NormalizedPayload,
	} {
		toolContext := asMap(payload["tool_context"])
		snapshotPayload := asMap(toolContext[toolNameBookingPassengerSnapshot])
		snapshot, ok := parseBookingPassengerSnapshotPayload(snapshotPayload)
		if !ok {
			t.Fatalf("expected valid booking_passenger_snapshot in %s, got %+v", label, payload)
		}
		if parsed == nil {
			copy := snapshot
			parsed = &copy
			continue
		}
		if len(parsed.Passengers) != len(snapshot.Passengers) || parsed.ExpectedDocumentCount != snapshot.ExpectedDocumentCount {
			t.Fatalf("payload snapshots diverged: first=%+v %s=%+v", parsed, label, snapshot)
		}
		for index := range parsed.Passengers {
			if parsed.Passengers[index] != snapshot.Passengers[index] {
				t.Fatalf("payload passenger snapshot diverged at %d: first=%+v %s=%+v", index, parsed.Passengers[index], label, snapshot.Passengers[index])
			}
		}
	}
	return parsed
}

func seedDocumentCorrectionBookingHistory(t *testing.T, store *fakeStore, contactKey string) Session {
	t.Helper()
	session := seedDocumentCollectionBookingHistory(t, store, contactKey)
	now := time.Now().UTC()
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        session.ID,
		Direction:        "OUTBOUND",
		Kind:             "TEXT",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       now.Add(-90 * time.Second),
		Body:             "Consegui identificar estes dados. Eles conferem? Posso prosseguir e criar a reserva?\n1. Claudecir Schumacher | RG | 2817314",
		Payload: map[string]interface{}{
			"tool_context": map[string]interface{}{
				toolNameDocumentExtract: buildDocumentExtractResponsePayload(DocumentExtractResult{
					Mode:                   "EXTRACTED",
					ExpectedPassengerCount: 1,
					MediaCount:             1,
					Passengers: []DocumentExtractPassenger{
						{Name: "Claudecir Schumacher", DocumentType: "RG", Document: "2817314", RG: "2817314", Confidence: 0.9},
					},
				}),
			},
		},
	}); err != nil {
		t.Fatalf("seed document confirmation: %v", err)
	}
	return session
}

func TestReprocessAsksOnlyForMissingPassengerDocumentAfterImageExtract(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          `{"mode":"EXTRACTED","passengers":[{"name":"Joao Vitor Messias","cpf":"06645648103","confidence":0.92}]}`,
			Model:              "gpt-vision-test",
			ProviderResponseID: "resp-document-extract-2",
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	availabilityPayload := buildAvailabilityToolResponsePayload(AvailabilitySearchResult{
		Filter: AvailabilitySearchInput{Origin: "Santa Ines/MA", Destination: "Fraiburgo/SC", Qty: 2, Limit: 5},
		Results: []AvailabilitySearchItem{{
			TripID: "trip-missing-doc", BoardStopID: "board-missing-doc", AlightStopID: "alight-missing-doc",
			OriginDisplayName: "Santa Ines/MA", DestinationDisplayName: "Fraiburgo/SC", OriginDepartTime: "12:00", TripDate: "2026-08-25", Price: 950, Currency: "BRL",
		}},
	})
	messages := []IngestMessagePayload{
		{Direction: "OUTBOUND", ProviderMessageID: "msg-missing-availability", IdempotencyKey: "idem-missing-availability", Body: "Encontrei uma opcao para Santa Ines/MA -> Fraiburgo/SC.", Payload: map[string]interface{}{"tool_context": map[string]interface{}{toolNameAvailabilitySearch: availabilityPayload}}, NormalizedPayload: map[string]interface{}{"tool_context": map[string]interface{}{toolNameAvailabilitySearch: availabilityPayload}}},
		{Direction: "OUTBOUND", ProviderMessageID: "msg-missing-out-1", IdempotencyKey: "idem-missing-out-1", Body: "A passagem e so para voce ou tem mais alguem? Ha crianca de ate 5 anos viajando?"},
		{Direction: "INBOUND", ProviderMessageID: "msg-missing-in-1", IdempotencyKey: "idem-missing-in-1", Body: "eu e minha filha de 4 anos"},
		{Direction: "OUTBOUND", ProviderMessageID: "msg-missing-out-2", IdempotencyKey: "idem-missing-out-2", Body: "Pode enviar os nomes completos e os documentos dos dois. Se preferir, pode mandar foto legivel do documento."},
	}
	for _, message := range messages {
		if _, err := svc.Ingest(context.Background(), IngestMessageInput{
			ContactKey: "5549988709047",
			Message:    message,
		}); err != nil {
			t.Fatalf("ingest history: %v", err)
		}
	}
	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5549988709047",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			Kind:              "IMAGE",
			ProviderMessageID: "msg-missing-img-1",
			IdempotencyKey:    "idem-missing-img-1",
			NormalizedPayload: map[string]interface{}{
				"image_url":       "https://files.example.test/documento.jpg",
				"image_mime_type": "image/jpeg",
			},
		},
	})
	if err != nil {
		t.Fatalf("ingest image: %v", err)
	}

	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess message: %v", err)
	}
	if reprocessed.Draft == nil {
		t.Fatalf("expected draft to be generated")
	}
	extract := findLatestDocumentExtractContext([]Message{*reprocessed.Draft})
	if extract == nil || len(extract.Passengers) != 1 || extract.Passengers[0].Name != "Joao Vitor Messias" {
		t.Fatalf("expected the extracted passenger snapshot to be preserved, got %+v", extract)
	}
	if !strings.Contains(reprocessed.Draft.Body, "Ainda falta o documento de 1 passageiro") {
		t.Fatalf("expected only missing passenger document request, got %q", reprocessed.Draft.Body)
	}
}

func TestReprocessSequentialTextThenDocumentExtractMergesPassengers(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{
		ReplyText: `{"mode":"EXTRACTED","passengers":[{"name":"Maria Messias","document_type":"RG","document":"1234567","confidence":0.93}]}`,
		Model:     "gpt-vision-test",
	}}
	creator := &fakeBookingCreator{enabled: true, result: BookingCreateResult{
		Mode: "created", BookingID: "BK-TEXT-IMAGE", ReservationCode: "TXTIMG12", Status: "PENDING", TotalAmount: 950,
	}}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, creator)
	session := seedSequentialSoloChildDocumentBookingHistory(t, store, "5549988709101")

	ingestedAdult, err := svc.Ingest(context.Background(), IngestMessageInput{ContactKey: session.ContactKey, Message: IngestMessagePayload{
		Direction: "INBOUND", ProviderMessageID: "msg-sequential-text-adult", IdempotencyKey: "idem-sequential-text-adult", Body: "Joao Vitor Messias 52998224725",
	}})
	if err != nil {
		t.Fatalf("ingest adult text document: %v", err)
	}
	adultResult, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingestedAdult.Session.ID})
	if err != nil {
		t.Fatalf("reprocess adult text document: %v", err)
	}
	if adultResult.Draft == nil || !strings.Contains(strings.Join(strings.Fields(foldChatText(adultResult.Draft.Body)), " "), "falta o documento da crianca") {
		t.Fatalf("expected missing child document after adult text, got %+v", adultResult.Draft)
	}
	seedSentOutboundFromDraft(t, store, session.ID, adultResult.Draft)

	childResult := reprocessSequentialDocumentImage(t, svc, session.ContactKey, "text-then-child")
	if got := strings.TrimSpace(asString(childResult.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskLapChildAssignment) {
		t.Fatalf("expected lap child assignment after second document, got %q body=%q", got, childResult.Draft.Body)
	}
	if !strings.Contains(childResult.Draft.Body, "Joao Vitor Messias") || !strings.Contains(childResult.Draft.Body, "Maria Messias") {
		t.Fatalf("expected both sequential passengers in assignment, got %q", childResult.Draft.Body)
	}
	assertSequentialDraftDocumentCount(t, childResult.Draft, 2)
	seedSentOutboundFromDraft(t, store, session.ID, childResult.Draft)

	history, err := store.ListMessages(context.Background(), session.ID, ListMessagesFilter{})
	if err != nil {
		t.Fatalf("list text then image history: %v", err)
	}
	restoredDraft := collectBookingDraftContext(session, history, "")
	if restoredDraft.PassengerDetailsCount != 2 || len(restoredDraft.PassengerDetails) != 2 {
		t.Fatalf("expected combined document snapshot to survive reconstruction, got %+v", restoredDraft)
	}
	if action := decideNextBookingStep(restoredDraft); action != BookingNextAskLapChildAssignment {
		t.Fatalf("expected reconstructed context to remain on lap child assignment, action=%s context=%+v", action, restoredDraft)
	}

	ingestedAssignment, err := svc.Ingest(context.Background(), IngestMessageInput{ContactKey: session.ContactKey, Message: IngestMessagePayload{
		Direction: "INBOUND", ProviderMessageID: "msg-text-image-assignment", IdempotencyKey: "idem-text-image-assignment", Body: "2",
	}})
	if err != nil {
		t.Fatalf("ingest text then image lap child assignment: %v", err)
	}
	assignmentResult, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingestedAssignment.Session.ID})
	if err != nil {
		t.Fatalf("reprocess text then image lap child assignment: %v", err)
	}
	if assignmentResult.Draft == nil {
		t.Fatalf("expected document confirmation after lap child assignment")
	}
	if got := strings.TrimSpace(asString(assignmentResult.Draft.NormalizedPayload["template_name"])); got != string(TemplateConfirmDocument) {
		t.Fatalf("expected document confirmation after assignment, got %q body=%q", got, assignmentResult.Draft.Body)
	}
	if !strings.Contains(assignmentResult.Draft.Body, "Joao Vitor Messias") ||
		!strings.Contains(assignmentResult.Draft.Body, "Maria Messias") {
		t.Fatalf("expected confirmation to keep both restored passengers, got %q", assignmentResult.Draft.Body)
	}
	if creator.calls != 0 {
		t.Fatalf("booking_create must remain blocked before confirmation, got %d calls", creator.calls)
	}
	seedSentOutboundFromDraft(t, store, session.ID, assignmentResult.Draft)
	history, err = store.ListMessages(context.Background(), session.ID, ListMessagesFilter{})
	if err != nil {
		t.Fatalf("list text then image history after assignment: %v", err)
	}
	assignedDraft := collectBookingDraftContext(session, history, "sim")
	if assignedDraft.PassengerDetailsCount != 2 ||
		len(assignedDraft.PassengerDetails) != 2 ||
		!assignedDraft.PassengerDetails[1].IsLapChild ||
		assignedDraft.PassengerDetails[0].IsLapChild {
		t.Fatalf("expected reconstructed assignment to preserve both passengers and mark passenger 2 as child, got %+v", assignedDraft)
	}

	ingestedConfirmation, err := svc.Ingest(context.Background(), IngestMessageInput{ContactKey: session.ContactKey, Message: IngestMessagePayload{
		Direction: "INBOUND", ProviderMessageID: "msg-text-image-confirmation", IdempotencyKey: "idem-text-image-confirmation", Body: "sim",
	}})
	if err != nil {
		t.Fatalf("ingest text then image document confirmation: %v", err)
	}
	if _, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingestedConfirmation.Session.ID}); err != nil {
		t.Fatalf("reprocess text then image document confirmation: %v", err)
	}
	if creator.calls != 1 || len(creator.lastInput.Passengers) != 2 {
		t.Fatalf("expected booking_create once with restored adult and child, calls=%d input=%+v", creator.calls, creator.lastInput)
	}
	if !creator.lastInput.Passengers[1].IsLapChild || creator.lastInput.Passengers[0].IsLapChild {
		t.Fatalf("expected booking_create to preserve passenger 2 as lap child, got %+v", creator.lastInput.Passengers)
	}
}

func TestReprocessSequentialDocumentImageThenTextUsesMergedPassengers(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{
		ReplyText: `{"mode":"EXTRACTED","passengers":[{"name":"Joao Vitor Messias","document_type":"CPF","document":"52998224725","confidence":0.93}]}`,
		Model:     "gpt-vision-test",
	}}
	creator := &fakeBookingCreator{enabled: true, result: BookingCreateResult{
		Mode: "created", BookingID: "BK-IMAGE-TEXT", ReservationCode: "IMGTXT12", Status: "PENDING", TotalAmount: 950,
	}}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, creator)
	session := seedSequentialSoloChildDocumentBookingHistory(t, store, "5549988709204")

	adultResult := reprocessSequentialDocumentImage(t, svc, session.ContactKey, "image-then-text-adult")
	assertSequentialDraftDocumentCount(t, adultResult.Draft, 1)
	if !strings.Contains(strings.Join(strings.Fields(foldChatText(adultResult.Draft.Body)), " "), "falta o documento da crianca") {
		t.Fatalf("expected missing child document after adult image, got %q", adultResult.Draft.Body)
	}
	seedSentOutboundFromDraft(t, store, session.ID, adultResult.Draft)

	ingestedChild, err := svc.Ingest(context.Background(), IngestMessageInput{ContactKey: session.ContactKey, Message: IngestMessagePayload{
		Direction: "INBOUND", ProviderMessageID: "msg-image-text-child", IdempotencyKey: "idem-image-text-child", Body: "Maria Messias RG 1234567",
	}})
	if err != nil {
		t.Fatalf("ingest child text document: %v", err)
	}
	childResult, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingestedChild.Session.ID})
	if err != nil {
		t.Fatalf("reprocess child text document: %v", err)
	}
	if childResult.Draft == nil {
		t.Fatalf("expected lap child assignment after image and text documents")
	}
	if got := strings.TrimSpace(asString(childResult.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskLapChildAssignment) {
		t.Fatalf("expected lap child assignment after image then text, got %q body=%q", got, childResult.Draft.Body)
	}
	if !strings.Contains(childResult.Draft.Body, "Joao Vitor Messias") || !strings.Contains(childResult.Draft.Body, "Maria Messias") {
		t.Fatalf("expected both merged passengers in assignment, got %q", childResult.Draft.Body)
	}
	if creator.calls != 0 {
		t.Fatalf("booking_create must remain blocked before assignment and confirmation, got %d calls", creator.calls)
	}
	seedSentOutboundFromDraft(t, store, session.ID, childResult.Draft)

	ingestedAssignment, err := svc.Ingest(context.Background(), IngestMessageInput{ContactKey: session.ContactKey, Message: IngestMessagePayload{
		Direction: "INBOUND", ProviderMessageID: "msg-image-text-assignment", IdempotencyKey: "idem-image-text-assignment", Body: "2",
	}})
	if err != nil {
		t.Fatalf("ingest image then text lap child assignment: %v", err)
	}
	assignmentResult, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingestedAssignment.Session.ID})
	if err != nil {
		t.Fatalf("reprocess image then text lap child assignment: %v", err)
	}
	if assignmentResult.Draft == nil {
		t.Fatalf("expected document confirmation after image then text assignment")
	}
	if got := strings.TrimSpace(asString(assignmentResult.Draft.NormalizedPayload["template_name"])); got != string(TemplateConfirmDocument) {
		t.Fatalf("expected document confirmation after assignment, got %q body=%q", got, assignmentResult.Draft.Body)
	}
	if !strings.Contains(assignmentResult.Draft.Body, "Joao Vitor Messias") || !strings.Contains(assignmentResult.Draft.Body, "Maria Messias") {
		t.Fatalf("expected merged passengers in confirmation, got %q", assignmentResult.Draft.Body)
	}
	seedSentOutboundFromDraft(t, store, session.ID, assignmentResult.Draft)

	history, err := store.ListMessages(context.Background(), session.ID, ListMessagesFilter{})
	if err != nil {
		t.Fatalf("list image then text history: %v", err)
	}
	extract := findLatestDocumentExtractContext(history)
	if extract == nil || len(extract.Passengers) != 1 {
		t.Fatalf("expected latest isolated extract to remain at one passenger, got %+v", extract)
	}
	bookingDraft := collectBookingDraftContext(session, history, "sim")
	if bookingDraft.PassengerDetailsCount != 2 || len(bookingDraft.PassengerDetails) != 2 {
		t.Fatalf("expected complete merged context before confirmation, got %+v", bookingDraft)
	}
	if input, ok := parseBookingCreateFromDocumentConfirmation(session, history, "sim"); !ok || len(input.Passengers) != 2 {
		t.Fatalf("expected merged image and text confirmation to authorize two passengers, ok=%v input=%+v", ok, input)
	}

	ingestedConfirmation, err := svc.Ingest(context.Background(), IngestMessageInput{ContactKey: session.ContactKey, Message: IngestMessagePayload{
		Direction: "INBOUND", ProviderMessageID: "msg-image-text-confirmation", IdempotencyKey: "idem-image-text-confirmation", Body: "sim",
	}})
	if err != nil {
		t.Fatalf("ingest image then text confirmation: %v", err)
	}
	if _, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingestedConfirmation.Session.ID}); err != nil {
		t.Fatalf("reprocess image then text confirmation: %v", err)
	}
	if creator.calls != 1 || len(creator.lastInput.Passengers) != 2 {
		t.Fatalf("expected booking_create once with the merged passengers, calls=%d input=%+v", creator.calls, creator.lastInput)
	}
	if creator.lastInput.Passengers[0].IsLapChild || !creator.lastInput.Passengers[1].IsLapChild {
		t.Fatalf("expected passenger 2 to remain the lap child, got %+v", creator.lastInput.Passengers)
	}
}

func TestReprocessSequentialDocumentExtractsMergePassengers(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{
		ReplyText: `{"mode":"EXTRACTED","passengers":[{"name":"Joao Vitor Messias","document_type":"CPF","document":"52998224725","confidence":0.93}]}`,
		Model:     "gpt-vision-test",
	}}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	session := seedSequentialSoloChildDocumentBookingHistory(t, store, "5549988709102")

	adultResult := reprocessSequentialDocumentImage(t, svc, session.ContactKey, "adult-first")
	assertSequentialDraftDocumentCount(t, adultResult.Draft, 1)
	seedSentOutboundFromDraft(t, store, session.ID, adultResult.Draft)
	runner.result = RunAgentResult{ReplyText: `{"mode":"EXTRACTED","passengers":[{"name":"Maria Messias","document_type":"RG","document":"1234567","confidence":0.93}]}`, Model: "gpt-vision-test"}

	childResult := reprocessSequentialDocumentImage(t, svc, session.ContactKey, "child-second")
	if got := strings.TrimSpace(asString(childResult.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskLapChildAssignment) {
		t.Fatalf("expected lap child assignment after two sequential extracts, got %q body=%q", got, childResult.Draft.Body)
	}
	assertSequentialDraftDocumentCount(t, childResult.Draft, 2)
}

func TestReprocessSequentialChildThenAdultDocumentExtractMergesPassengers(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{
		ReplyText: `{"mode":"EXTRACTED","passengers":[{"name":"Maria Messias","document_type":"RG","document":"1234567","birth_date":"2022-03-04","confidence":0.93}]}`,
		Model:     "gpt-vision-test",
	}}
	creator := &fakeBookingCreator{enabled: true, result: BookingCreateResult{
		Mode: "created", BookingID: "BK-SEQUENTIAL", ReservationCode: "SEQ12345", Status: "PENDING", TotalAmount: 950,
	}}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, creator)
	session := seedSequentialSoloChildDocumentBookingHistory(t, store, "5549988709103")

	childResult := reprocessSequentialDocumentImage(t, svc, session.ContactKey, "child-first")
	assertSequentialDraftDocumentCount(t, childResult.Draft, 1)
	if creator.calls != 0 {
		t.Fatalf("booking_create must remain blocked after only the first document, got %d calls", creator.calls)
	}
	seedSentOutboundFromDraft(t, store, session.ID, childResult.Draft)
	runner.result = RunAgentResult{ReplyText: `{"mode":"EXTRACTED","passengers":[{"name":"Joao Vitor Messias","document_type":"CPF","document":"52998224725","confidence":0.93}]}`, Model: "gpt-vision-test"}

	adultResult := reprocessSequentialDocumentImage(t, svc, session.ContactKey, "adult-second")
	if got := strings.TrimSpace(asString(adultResult.Draft.NormalizedPayload["template_name"])); got != string(TemplateConfirmDocument) {
		t.Fatalf("expected combined document confirmation when child assignment is known, got %q body=%q", got, adultResult.Draft.Body)
	}
	if !strings.Contains(adultResult.Draft.Body, "Maria Messias") || !strings.Contains(adultResult.Draft.Body, "Joao Vitor Messias") {
		t.Fatalf("expected both sequential passengers in confirmation, got %q", adultResult.Draft.Body)
	}
	assertSequentialDraftDocumentCount(t, adultResult.Draft, 2)
	seedSentOutboundFromDraft(t, store, session.ID, adultResult.Draft)
	history, err := store.ListMessages(context.Background(), session.ID, ListMessagesFilter{})
	if err != nil {
		t.Fatalf("list sequential booking history: %v", err)
	}
	bookingDraft := collectBookingDraftContext(session, history, "sim")
	if action := decideNextBookingStep(bookingDraft); action != BookingNextCallCreate {
		t.Fatalf("expected sequential merged context ready to create, action=%s context=%+v", action, bookingDraft)
	}
	if input, ok := parseBookingCreateFromDocumentConfirmation(session, history, "sim"); !ok {
		t.Fatalf("expected sequential merged documents to authorize booking_create, context=%+v extract=%+v", bookingDraft, findLatestDocumentExtractContext(history))
	} else if len(input.Passengers) != 2 {
		t.Fatalf("expected two passengers in sequential booking input, got %+v", input)
	}

	ingestedConfirmation, err := svc.Ingest(context.Background(), IngestMessageInput{ContactKey: session.ContactKey, Message: IngestMessagePayload{
		Direction: "INBOUND", ProviderMessageID: "msg-sequential-confirm", IdempotencyKey: "idem-sequential-confirm", Body: "sim",
	}})
	if err != nil {
		t.Fatalf("ingest sequential document confirmation: %v", err)
	}
	historyWithConfirmation, err := store.ListMessages(context.Background(), session.ID, ListMessagesFilter{})
	if err != nil {
		t.Fatalf("list sequential booking history with confirmation: %v", err)
	}
	bookingDraftWithConfirmation := collectBookingDraftContext(session, historyWithConfirmation, "sim")
	if action := decideNextBookingStep(bookingDraftWithConfirmation); action != BookingNextCallCreate {
		t.Fatalf("expected persisted confirmation context ready to create, action=%s context=%+v", action, bookingDraftWithConfirmation)
	}
	if input, ok := parseBookingCreateFromDocumentConfirmation(session, historyWithConfirmation, "sim"); !ok {
		t.Fatalf("expected persisted confirmation turn to authorize booking_create, context=%+v extract=%+v", bookingDraftWithConfirmation, findLatestDocumentExtractContext(historyWithConfirmation))
	} else if len(input.Passengers) != 2 {
		t.Fatalf("expected two passengers after persisted confirmation turn, got %+v", input)
	}
	confirmationResult, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingestedConfirmation.Session.ID})
	if err != nil {
		t.Fatalf("reprocess sequential document confirmation: %v", err)
	}
	if creator.calls != 1 || len(creator.lastInput.Passengers) != 2 {
		body := ""
		if confirmationResult.Draft != nil {
			body = confirmationResult.Draft.Body
		}
		t.Fatalf("expected booking_create once with two merged passengers, calls=%d input=%+v draft=%q", creator.calls, creator.lastInput, body)
	}
	if !creator.lastInput.Passengers[0].IsLapChild || creator.lastInput.Passengers[1].IsLapChild {
		t.Fatalf("expected only the first passenger marked as lap child, got %+v", creator.lastInput.Passengers)
	}
}

func TestReprocessSequentialDuplicateDocumentDoesNotIncreaseCount(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{
		ReplyText: `{"mode":"EXTRACTED","passengers":[{"name":"Joao Vitor Messias","document_type":"CPF","document":"52998224725","confidence":0.93}]}`,
		Model:     "gpt-vision-test",
	}}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	session := seedSequentialSoloChildDocumentBookingHistory(t, store, "5549988709104")

	firstResult := reprocessSequentialDocumentImage(t, svc, session.ContactKey, "duplicate-first")
	seedSentOutboundFromDraft(t, store, session.ID, firstResult.Draft)
	secondResult := reprocessSequentialDocumentImage(t, svc, session.ContactKey, "duplicate-second")

	assertSequentialDraftDocumentCount(t, secondResult.Draft, 1)
	if got := strings.TrimSpace(asString(secondResult.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskDocuments) {
		t.Fatalf("duplicate document must keep asking for the missing child, got %q body=%q", got, secondResult.Draft.Body)
	}
}

func TestReprocessLatestDocumentExtractionWinsOverAccumulatedSnapshot(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{
		ReplyText: `{"mode":"EXTRACTED","passengers":[{"name":"Nome Antigo","document_type":"CPF","document":"52998224725","birth_date":"1990-01-01","birth_city":"Cidade Antiga","confidence":0.93}]}`,
		Model:     "gpt-vision-test",
	}}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	session := seedSequentialSoloChildDocumentBookingHistory(t, store, "5549988709106")

	firstResult := reprocessSequentialDocumentImage(t, svc, session.ContactKey, "latest-wins-old")
	assertSequentialDraftDocumentCount(t, firstResult.Draft, 1)
	seedSentOutboundFromDraft(t, store, session.ID, firstResult.Draft)

	runner.result = RunAgentResult{
		ReplyText: `{"mode":"EXTRACTED","passengers":[{"name":"Nome Corrigido","document_type":"CPF","document":"52998224725","birth_date":"1991-02-02","birth_city":"Cidade Nova","confidence":0.97}]}`,
		Model:     "gpt-vision-test",
	}
	latestResult := reprocessSequentialDocumentImage(t, svc, session.ContactKey, "latest-wins-new")
	extract := assertSequentialDraftDocumentCount(t, latestResult.Draft, 1)
	passenger := extract.Passengers[0]
	if passenger.Name != "Nome Corrigido" || passenger.BirthDate != "1991-02-02" || passenger.BirthCity != "Cidade Nova" {
		t.Fatalf("expected current extraction to replace older non-empty fields, got %+v", passenger)
	}
	if got := strings.TrimSpace(asString(latestResult.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskDocuments) {
		t.Fatalf("same document must remain one passenger and keep asking for the child, got %q body=%q", got, latestResult.Draft.Body)
	}
}

func TestReprocessDocumentReextractReplacesUniquePassengerWhenIdentifierChanges(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{
		ReplyText: `{"mode":"EXTRACTED","passengers":[{"name":"Joao Vitor Messias","document_type":"CPF","document":"52998224725","confidence":0.93},{"name":"Maria Messias","document_type":"RG","document":"1234567","confidence":0.93}]}`,
		Model:     "gpt-vision-test",
	}}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	session := seedSequentialSoloChildDocumentBookingHistory(t, store, "5549988709203")

	firstResult := reprocessSequentialDocumentImage(t, svc, session.ContactKey, "replace-rg")
	assertSequentialDraftDocumentCount(t, firstResult.Draft, 2)
	replacementRequest := *firstResult.Draft
	replacementRequest.Body = "Reenvie o documento da Maria para corrigir os dados, por favor."
	replacementRequest.Payload = cloneMap(firstResult.Draft.Payload)
	replacementRequest.NormalizedPayload = cloneMap(firstResult.Draft.NormalizedPayload)
	replacementRequest.Payload["template_name"] = string(TemplateAskDocuments)
	replacementRequest.NormalizedPayload["template_name"] = string(TemplateAskDocuments)
	seedSentOutboundFromDraft(t, store, session.ID, &replacementRequest)

	runner.result = RunAgentResult{
		ReplyText: `{"mode":"EXTRACTED","passengers":[{"name":"Maria Messias","document_type":"CNH","document":"12345678901","confidence":0.97}]}`,
		Model:     "gpt-vision-test",
	}
	latestResult := reprocessSequentialDocumentImage(t, svc, session.ContactKey, "replace-cnh")
	extract := assertSequentialDraftDocumentCount(t, latestResult.Draft, 2)
	passenger := extract.Passengers[1]
	if passenger.Name != "Maria Messias" || passenger.DocumentType != "CNH" || passenger.Document != "12345678901" {
		t.Fatalf("expected the unique passenger slot to use the latest CNH, got %+v", passenger)
	}
	if extract.Passengers[0].Document != "52998224725" {
		t.Fatalf("expected the other passenger slot to remain unchanged, got %+v", extract.Passengers)
	}
	if got := strings.TrimSpace(asString(latestResult.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskLapChildAssignment) {
		t.Fatalf("document replacement must keep the two-passenger assignment step, got %q body=%q", got, latestResult.Draft.Body)
	}
}

func TestReprocessDocumentMergeKeepsTwoPassengersFromSameExtraction(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{
		ReplyText: `{"mode":"EXTRACTED","passengers":[{"name":"Joao Vitor Messias","document_type":"CPF","document":"52998224725","confidence":0.93},{"name":"Maria Messias","document_type":"RG","document":"1234567","confidence":0.93}]}`,
		Model:     "gpt-vision-test",
	}}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	session := seedSequentialSoloChildDocumentBookingHistory(t, store, "5549988709105")

	result := reprocessSequentialDocumentImage(t, svc, session.ContactKey, "same-extraction")
	if got := strings.TrimSpace(asString(result.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskLapChildAssignment) {
		t.Fatalf("expected lap child assignment for two passengers in one extraction, got %q body=%q", got, result.Draft.Body)
	}
	assertSequentialDraftDocumentCount(t, result.Draft, 2)
}

func TestReprocessLapChildMismatchDoesNotConfirmDocument(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{
		ReplyText: `{"mode":"EXTRACTED","passengers":[{"name":"Joao Vitor Messias","document_type":"CPF","document":"52998224725","is_lap_child":true,"confidence":0.93},{"name":"Maria Messias","document_type":"RG","document":"1234567","is_lap_child":true,"confidence":0.93}]}`,
		Model:     "gpt-vision-test",
	}}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	session := seedSequentialSoloChildDocumentBookingHistory(t, store, "5549988709222")

	result := reprocessSequentialDocumentImage(t, svc, session.ContactKey, "lap-child-mismatch")
	if got := strings.TrimSpace(asString(result.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskLapChildAssignment) {
		t.Fatalf("two marked lap children for one expected must ask assignment, got %q body=%q", got, result.Draft.Body)
	}
	if got := strings.TrimSpace(asString(result.Draft.NormalizedPayload["template_name"])); got == string(TemplateConfirmDocument) {
		t.Fatalf("lap-child mismatch must never emit CONFIRM_DOCUMENT, body=%q", result.Draft.Body)
	}
	extract := assertSequentialDraftDocumentCount(t, result.Draft, 2)
	if !extract.Passengers[0].IsLapChild || !extract.Passengers[1].IsLapChild {
		t.Fatalf("test must preserve both conflicting lap-child marks, got %+v", extract.Passengers)
	}
}

func TestReprocessLatestLapChildAgeAdultExtractDoesNotAskAssignment(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{
		ReplyText: `{"mode":"EXTRACTED","passengers":[{"name":"Maria Messias","document_type":"CPF","document":"52998224725","cpf":"52998224725","birth_date":"1990-03-12","confidence":0.98}]}`,
		Model:     "gpt-vision-test",
	}}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	session := seedDocumentCollectionBookingHistory(t, store, "5549988709224")

	oldExtract := DocumentExtractResult{
		Mode:                   "EXTRACTED",
		ExpectedPassengerCount: 1,
		Passengers: []DocumentExtractPassenger{{
			Name: "Maria Messias", DocumentType: "CPF", Document: "52998224725", CPF: "52998224725",
			BirthDate: "2022-05-10", IsLapChild: true, Confidence: 0.98,
		}},
	}
	toolContext := map[string]interface{}{toolNameDocumentExtract: buildDocumentExtractResponsePayload(oldExtract)}
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID: session.ID, Direction: "OUTBOUND", Kind: "TEXT", ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:        time.Now().UTC().Add(-time.Minute),
		Body:              buildConfirmExtractedDocumentReply(oldExtract),
		Payload:           map[string]interface{}{"tool_context": toolContext},
		NormalizedPayload: map[string]interface{}{"tool_context": toolContext},
	}); err != nil {
		t.Fatalf("seed old lap-child extract: %v", err)
	}

	result := reprocessSequentialDocumentImage(t, svc, session.ContactKey, "latest-adult-age")
	if got := strings.TrimSpace(asString(result.Draft.NormalizedPayload["template_name"])); got != string(TemplateConfirmDocument) {
		t.Fatalf("new adult age must clear the obsolete marker instead of asking assignment, got %q body=%q", got, result.Draft.Body)
	}
	extract := assertSequentialDraftDocumentCount(t, result.Draft, 1)
	if extract.Passengers[0].BirthDate != "1990-03-12" || extract.Passengers[0].IsLapChild {
		t.Fatalf("combined extract must persist the latest adult age and cleared marker, got %+v", extract.Passengers[0])
	}
}

func TestReprocessLapChildNameFallbackPreservesOldMarker(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{
		ReplyText: `{"mode":"EXTRACTED","passengers":[{"name":"Maria Messias","document_type":"CNH","document":"12345678901","cnh":"12345678901","birth_date":"1990-03-12","confidence":0.98}]}`,
		Model:     "gpt-vision-test",
	}}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	session := seedDocumentCollectionBookingHistory(t, store, "5549988709225")

	oldExtract := DocumentExtractResult{
		Mode:                   "EXTRACTED",
		ExpectedPassengerCount: 1,
		Passengers: []DocumentExtractPassenger{{
			Name: "Maria Messias", DocumentType: "RG", Document: "1234567", RG: "1234567",
			BirthDate: "2022-05-10", IsLapChild: true, Confidence: 0.98,
		}},
	}
	toolContext := map[string]interface{}{toolNameDocumentExtract: buildDocumentExtractResponsePayload(oldExtract)}
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID: session.ID, Direction: "OUTBOUND", Kind: "TEXT", ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:        time.Now().UTC().Add(-time.Minute),
		Body:              buildConfirmExtractedDocumentReply(oldExtract),
		Payload:           map[string]interface{}{"tool_context": toolContext},
		NormalizedPayload: map[string]interface{}{"tool_context": toolContext},
	}); err != nil {
		t.Fatalf("seed old RG lap-child extract: %v", err)
	}

	result := reprocessSequentialDocumentImage(t, svc, session.ContactKey, "name-fallback-adult-age")
	if got := strings.TrimSpace(asString(result.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskLapChildAssignment) {
		t.Fatalf("name-only RG to CNH fallback must preserve the old marker in Service, got %q body=%q", got, result.Draft.Body)
	}
	extract := assertSequentialDraftDocumentCount(t, result.Draft, 1)
	if extract.Passengers[0].DocumentType != "CNH" || extract.Passengers[0].Document != "12345678901" || !extract.Passengers[0].IsLapChild {
		t.Fatalf("combined Service extract must update the document and preserve the old marker, got %+v", extract.Passengers[0])
	}
	snapshot := assertDraftBookingPassengerSnapshotPersistence(t, result.Draft)
	if snapshot.Passengers[0].LapChildSource != bookingPassengerLapChildSourceNameFallbackPreserved {
		t.Fatalf("expected persisted nominal fallback provenance, got %+v", snapshot.Passengers[0])
	}
	rawExtract := findLatestDocumentExtractContext([]Message{*result.Draft})
	if rawExtract == nil || len(rawExtract.Passengers) != 1 || rawExtract.Passengers[0].DocumentType != "CNH" || rawExtract.Passengers[0].RG != "" {
		t.Fatalf("document_extract must remain the raw current CNH evidence, got %+v", rawExtract)
	}
}

func TestReprocessLapChildNameFallbackAdultToChildSnapshotReplayPreservesAdult(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{
		ReplyText: `{"mode":"EXTRACTED","passengers":[{"name":"Maria Messias","document_type":"CNH","document":"12345678901","cnh":"12345678901","birth_date":"2022-05-10","confidence":0.98}]}`,
		Model:     "gpt-vision-test",
	}}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	session := seedDocumentCollectionBookingHistory(t, store, "5549988709226")

	oldExtract := DocumentExtractResult{
		Mode:                   "EXTRACTED",
		ExpectedPassengerCount: 1,
		Passengers: []DocumentExtractPassenger{{
			Name: "Maria Messias", DocumentType: "RG", Document: "1234567", RG: "1234567",
			BirthDate: "1990-03-12", Confidence: 0.98,
		}},
	}
	toolContext := map[string]interface{}{toolNameDocumentExtract: buildDocumentExtractResponsePayload(oldExtract)}
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID: session.ID, Direction: "OUTBOUND", Kind: "TEXT", ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:        time.Now().UTC().Add(-time.Minute),
		Body:              buildConfirmExtractedDocumentReply(oldExtract),
		Payload:           map[string]interface{}{"tool_context": toolContext},
		NormalizedPayload: map[string]interface{}{"tool_context": toolContext},
	}); err != nil {
		t.Fatalf("seed old RG adult extract: %v", err)
	}

	result := reprocessSequentialDocumentImage(t, svc, session.ContactKey, "name-fallback-child-age")
	if got := strings.TrimSpace(asString(result.Draft.NormalizedPayload["template_name"])); got != string(TemplateConfirmDocument) {
		t.Fatalf("name-only RG to CNH fallback must preserve adult readiness, got %q body=%q", got, result.Draft.Body)
	}
	snapshot := assertDraftBookingPassengerSnapshotPersistence(t, result.Draft)
	if len(snapshot.Passengers) != 1 || snapshot.Passengers[0].Passenger.IsLapChild || snapshot.Passengers[0].LapChildSource != bookingPassengerLapChildSourceNameFallbackPreserved {
		t.Fatalf("expected persisted adult nominal fallback snapshot, got %+v", snapshot)
	}
	rawExtract := findLatestDocumentExtractContext([]Message{*result.Draft})
	if rawExtract == nil || len(rawExtract.Passengers) != 1 || rawExtract.Passengers[0].BirthDate != "2022-05-10" {
		t.Fatalf("document_extract must remain the raw child-age CNH evidence, got %+v", rawExtract)
	}

	replayMessage := *result.Draft
	for replay := 1; replay <= 2; replay++ {
		restored := collectBookingDraftContext(session, []Message{replayMessage}, "")
		if len(restored.PassengerDetails) != 1 || restored.PassengerDetails[0].IsLapChild {
			t.Fatalf("replay %d changed adult classification: %+v", replay, restored.PassengerDetails)
		}
		replayMessage = bookingPassengerSnapshotReplayMessage(restored.PassengerSnapshot, *rawExtract)
	}
}

func TestUnknownAvailabilityQtyServiceDoesNotConfirmDocument(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{
		ReplyText: `{"mode":"EXTRACTED","passengers":[{"name":"Joao Vitor Messias","document_type":"CPF","document":"52998224725","confidence":0.93}]}`,
		Model:     "gpt-vision-test",
	}}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	now := time.Now().UTC()
	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel: "WHATSAPP", ContactKey: "5549988709223", CustomerPhone: "5549988709223", CustomerName: "Joao", LastMessageAt: &now, LastOutboundAt: &now,
	})
	if err != nil {
		t.Fatalf("seed unknown-quantity session: %v", err)
	}
	availability := buildAvailabilityToolResponsePayload(AvailabilitySearchResult{
		Filter: AvailabilitySearchInput{Origin: "Santa Ines/MA", Destination: "Fraiburgo/SC", Qty: 1, Limit: 5},
		Results: []AvailabilitySearchItem{{
			TripID: "trip-unknown-qty", BoardStopID: "board-unknown-qty", AlightStopID: "alight-unknown-qty",
			OriginDisplayName: "Santa Ines/MA", DestinationDisplayName: "Fraiburgo/SC", OriginDepartTime: "12:00", TripDate: "2026-08-25", Price: 950, Currency: "BRL",
		}},
	})
	seed := []CreateMessageInput{
		{SessionID: session.ID, Direction: "OUTBOUND", Kind: "TEXT", Body: "Encontrei esta opcao para Santa Ines/MA -> Fraiburgo/SC.", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-4 * time.Minute), Payload: map[string]interface{}{"tool_context": map[string]interface{}{toolNameAvailabilitySearch: availability}}, NormalizedPayload: map[string]interface{}{"tool_context": map[string]interface{}{toolNameAvailabilitySearch: availability}}},
		{SessionID: session.ID, Direction: "INBOUND", Kind: "TEXT", Body: "primeira opcao", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-3 * time.Minute)},
		{SessionID: session.ID, Direction: "OUTBOUND", Kind: "TEXT", Body: askPassengerCountReply, ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-2 * time.Minute)},
		{SessionID: session.ID, Direction: "OUTBOUND", Kind: "TEXT", Body: "Pode enviar seu nome completo e CPF.", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-time.Minute)},
	}
	for _, message := range seed {
		if _, err := store.CreateMessage(context.Background(), message); err != nil {
			t.Fatalf("seed unknown-quantity history: %v", err)
		}
	}

	result := reprocessSequentialDocumentImage(t, svc, session.ContactKey, "unknown-availability-qty")
	if got := strings.TrimSpace(asString(result.Draft.NormalizedPayload["template_name"])); got == string(TemplateConfirmDocument) {
		t.Fatalf("unknown passenger quantity must never emit CONFIRM_DOCUMENT, body=%q", result.Draft.Body)
	}
	if !strings.Contains(result.Draft.Body, "so para voce ou vai mais alguem") {
		t.Fatalf("default availability qty must keep quantity UNKNOWN and ask clarification, body=%q payload=%+v", result.Draft.Body, result.Draft.NormalizedPayload)
	}
}

func TestGetCurrentDraftReturnsAutoSendRetryDetails(t *testing.T) {
	store := newFakeStore()
	now := time.Now().UTC()
	session, _ := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:       "WHATSAPP",
		ContactKey:    "5511888000000",
		CustomerPhone: "5511888000000",
		LastMessageAt: timePointer(now),
		Metadata: map[string]interface{}{
			"agent": map[string]interface{}{
				"status":                    agentStatusDraftGenerated,
				"draft_generated_at":        now.Add(-5 * time.Minute).Format(time.RFC3339Nano),
				"auto_send_status":          draftAutoSendStatusRetryPending,
				"auto_send_reasons":         []string{draftAutoSendReasonDeliveryFail},
				"auto_send_last_attempt_at": now.Add(-1 * time.Minute).Format(time.RFC3339Nano),
				"auto_send_last_error_text": "gateway timeout",
			},
		},
	})

	draft := Message{
		ID:             uuid.NewString(),
		SessionID:      session.ID,
		Direction:      "OUTBOUND",
		Kind:           "TEXT",
		IdempotencyKey: "chat-agent-draft-retry-1",
		Body:           "Posso seguir com seu atendimento.",
		Payload: map[string]interface{}{
			"mode":                            "AUTOMATION_DRAFT",
			"draft_idempotency_key":           "chat-agent-draft-retry-1",
			"auto_send_status":                draftAutoSendStatusRetryPending,
			"auto_send_reasons":               []string{draftAutoSendReasonDeliveryFail},
			"auto_send_last_attempt_at":       now.Add(-1 * time.Minute).Format(time.RFC3339Nano),
			"auto_send_retry_pending_at":      now.Add(-1 * time.Minute).Format(time.RFC3339Nano),
			"auto_send_last_error_text":       "gateway timeout",
			"auto_send_last_reply_message_id": "reply-retry-1",
			"auto_send_last_outbound_id":      "outbound-retry-1",
		},
		NormalizedPayload: map[string]interface{}{
			"mode":                            "AUTOMATION_DRAFT",
			"draft_idempotency_key":           "chat-agent-draft-retry-1",
			"auto_send_status":                draftAutoSendStatusRetryPending,
			"auto_send_reasons":               []string{draftAutoSendReasonDeliveryFail},
			"auto_send_last_attempt_at":       now.Add(-1 * time.Minute).Format(time.RFC3339Nano),
			"auto_send_retry_pending_at":      now.Add(-1 * time.Minute).Format(time.RFC3339Nano),
			"auto_send_last_error_text":       "gateway timeout",
			"auto_send_last_reply_message_id": "reply-retry-1",
			"auto_send_last_outbound_id":      "outbound-retry-1",
		},
		ProcessingStatus: messageStatusAutomationDraft,
		ReceivedAt:       now.Add(-5 * time.Minute),
		CreatedAt:        now.Add(-5 * time.Minute),
	}
	store.messages[draft.ID] = draft
	store.messageOrder = append(store.messageOrder, draft.ID)
	store.byIdempotencyKey[draft.IdempotencyKey] = draft.ID

	handler := NewHandler(NewService(store, config.Config{ChatDebounceWindowMS: 1500}))
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/chat/sessions/"+session.ID+"/draft", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var out CurrentDraftResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal current draft: %v", err)
	}
	if out.AutoSendStatus != draftAutoSendStatusRetryPending {
		t.Fatalf("expected auto_send_status %s, got %s", draftAutoSendStatusRetryPending, out.AutoSendStatus)
	}
	if !out.AutoSendIssueActive {
		t.Fatalf("expected auto_send_issue_active true")
	}
	if out.AutoSendLastErrorText != "gateway timeout" {
		t.Fatalf("expected auto_send_last_error_text gateway timeout, got %s", out.AutoSendLastErrorText)
	}
	if out.AutoSendLastReplyID != "reply-retry-1" {
		t.Fatalf("expected auto_send_last_reply_message_id reply-retry-1, got %s", out.AutoSendLastReplyID)
	}
	if out.AutoSendLastOutboundID != "outbound-retry-1" {
		t.Fatalf("expected auto_send_last_outbound_id outbound-retry-1, got %s", out.AutoSendLastOutboundID)
	}
	if out.AutoSendLastAttemptAt == nil || out.AutoSendRetryAt == nil {
		t.Fatalf("expected auto-send retry timestamps to be populated")
	}
}

func TestListSessionsIncludesDraftReviewSummary(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Encontrei uma opcao para esse trecho.",
			Model:              "gpt-test",
			ProviderResponseID: "resp-session-summary-1",
		},
	}
	searcher := &fakeAvailabilitySearcher{
		enabled: true,
		result: AvailabilitySearchResult{
			Results: []AvailabilitySearchItem{
				{
					SegmentID:              "seg-1",
					TripID:                 "trip-1",
					RouteID:                "route-1",
					BoardStopID:            "board-1",
					AlightStopID:           "alight-1",
					OriginStopID:           "stop-origin-1",
					DestinationStopID:      "stop-destination-1",
					OriginDisplayName:      "Videira/SC",
					DestinationDisplayName: "Sao Luis/MA",
					OriginDepartTime:       "18:30",
					TripDate:               "2026-05-10",
					SeatsAvailable:         12,
					Price:                  250,
					Currency:               "BRL",
					Status:                 "ACTIVE",
					TripStatus:             "SCHEDULED",
				},
			},
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511777777777",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-session-summary-1",
			IdempotencyKey:    "idem-session-summary-1",
			Body:              "qual o valor de Videira/SC para Sao Luis/MA em 10/05?",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}
	if _, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID}); err != nil {
		t.Fatalf("reprocess message: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/chat/sessions?channel=whatsapp&limit=10", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var sessions []Session
	if err := json.Unmarshal(rec.Body.Bytes(), &sessions); err != nil {
		t.Fatalf("unmarshal sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected one session, got %d", len(sessions))
	}
	if sessions[0].AgentStatus != agentStatusDraftGenerated {
		t.Fatalf("expected agent status %s, got %s", agentStatusDraftGenerated, sessions[0].AgentStatus)
	}
	if !sessions[0].HasAutomationDraft {
		t.Fatalf("expected has_automation_draft to be true")
	}
	if sessions[0].DraftReviewStatus != "PENDING_REVIEW" {
		t.Fatalf("expected draft review status PENDING_REVIEW, got %s", sessions[0].DraftReviewStatus)
	}
	if sessions[0].DraftGeneratedAt == nil {
		t.Fatalf("expected draft_generated_at to be present")
	}
	if sessions[0].DraftReviewSLASeconds != 15*60 {
		t.Fatalf("expected draft review SLA 900 seconds, got %d", sessions[0].DraftReviewSLASeconds)
	}
	if sessions[0].DraftPendingAgeBucket != "FRESH" {
		t.Fatalf("expected draft pending age bucket FRESH, got %s", sessions[0].DraftPendingAgeBucket)
	}
	if sessions[0].DraftReviewPriority != "LOW" {
		t.Fatalf("expected draft review priority LOW, got %s", sessions[0].DraftReviewPriority)
	}
	if sessions[0].DraftReviewAlertActive {
		t.Fatalf("expected draft review alert inactive for fresh draft")
	}
	if sessions[0].DraftReviewOverdue {
		t.Fatalf("expected draft review overdue false")
	}
	if len(sessions[0].DraftToolNames) != 1 || sessions[0].DraftToolNames[0] != toolNameAvailabilitySearch {
		t.Fatalf("expected draft tool names to include %s, got %+v", toolNameAvailabilitySearch, sessions[0].DraftToolNames)
	}
	if sessions[0].DraftToolCallCount != 1 {
		t.Fatalf("expected draft tool call count 1, got %d", sessions[0].DraftToolCallCount)
	}
	if sessions[0].DraftModel != "gpt-test" {
		t.Fatalf("expected draft model gpt-test, got %s", sessions[0].DraftModel)
	}
	if sessions[0].DraftProviderResponseID != "resp-session-summary-1" {
		t.Fatalf("expected provider response id resp-session-summary-1, got %s", sessions[0].DraftProviderResponseID)
	}
	if sessions[0].DraftAutoSendStatus != draftAutoSendStatusEligible {
		t.Fatalf("expected draft auto send status %s, got %s", draftAutoSendStatusEligible, sessions[0].DraftAutoSendStatus)
	}
	if len(sessions[0].DraftAutoSendReasons) != 0 {
		t.Fatalf("expected no draft auto send reasons for safe availability draft, got %+v", sessions[0].DraftAutoSendReasons)
	}
}

func TestGetSessionIncludesReviewedDraftSummary(t *testing.T) {
	store := newFakeStore()
	ownerUserID := uuid.NewString()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Posso seguir com sua reserva.",
			Model:              "gpt-test",
			ProviderResponseID: "resp-session-summary-2",
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511666666666",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-session-summary-2",
			IdempotencyKey:    "idem-session-summary-2",
			Body:              "quero continuar minha reserva",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}
	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess message: %v", err)
	}
	if _, err := svc.RequestHandoff(context.Background(), RequestHandoffInput{
		SessionID:      ingested.Session.ID,
		RequestedBy:    "dashboard",
		AssignedUserID: ownerUserID,
	}); err != nil {
		t.Fatalf("request handoff: %v", err)
	}
	if _, err := svc.Reply(context.Background(), ReplyInput{
		SessionID:      ingested.Session.ID,
		OwnerUserID:    ownerUserID,
		DraftMessageID: reprocessed.Draft.ID,
		IdempotencyKey: "idem-session-reviewed-1",
	}); err != nil {
		t.Fatalf("reply message: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/chat/sessions/"+ingested.Session.ID, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var out Session
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal session: %v", err)
	}
	if out.AgentStatus != agentStatusDraftReviewed {
		t.Fatalf("expected agent status %s, got %s", agentStatusDraftReviewed, out.AgentStatus)
	}
	if !out.HasAutomationDraft {
		t.Fatalf("expected has_automation_draft to be true")
	}
	if out.DraftReviewStatus != "REVIEWED" {
		t.Fatalf("expected draft review status REVIEWED, got %s", out.DraftReviewStatus)
	}
	if out.DraftReviewedAt == nil {
		t.Fatalf("expected draft_reviewed_at to be present")
	}
	if out.DraftReviewedByUserID != ownerUserID {
		t.Fatalf("expected draft_reviewed_by_user_id %s, got %s", ownerUserID, out.DraftReviewedByUserID)
	}
	if out.DraftReviewAction != "APPROVED_AS_IS" {
		t.Fatalf("expected draft review action APPROVED_AS_IS, got %s", out.DraftReviewAction)
	}
	if out.DraftAutoSendStatus != draftAutoSendStatusEligible {
		t.Fatalf("expected draft auto send status %s, got %s", draftAutoSendStatusEligible, out.DraftAutoSendStatus)
	}
	if len(out.DraftAutoSendReasons) != 0 {
		t.Fatalf("expected no draft auto send reasons, got %+v", out.DraftAutoSendReasons)
	}
}

func TestListSessionsFiltersByDraftReviewStatus(t *testing.T) {
	store := newFakeStore()
	ownerUserID := uuid.NewString()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Posso seguir com sua reserva.",
			Model:              "gpt-test",
			ProviderResponseID: "resp-filter-review",
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)

	pending, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511555555555",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-filter-review-1",
			IdempotencyKey:    "idem-filter-review-1",
			Body:              "quero continuar minha reserva",
		},
	})
	if err != nil {
		t.Fatalf("ingest pending session: %v", err)
	}
	if _, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: pending.Session.ID}); err != nil {
		t.Fatalf("reprocess pending session: %v", err)
	}

	reviewed, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511444444444",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-filter-review-2",
			IdempotencyKey:    "idem-filter-review-2",
			Body:              "quero continuar minha reserva tambem",
		},
	})
	if err != nil {
		t.Fatalf("ingest reviewed session: %v", err)
	}
	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: reviewed.Session.ID})
	if err != nil {
		t.Fatalf("reprocess reviewed session: %v", err)
	}
	if _, err := svc.RequestHandoff(context.Background(), RequestHandoffInput{
		SessionID:      reviewed.Session.ID,
		RequestedBy:    "dashboard",
		AssignedUserID: ownerUserID,
	}); err != nil {
		t.Fatalf("request handoff: %v", err)
	}
	if _, err := svc.Reply(context.Background(), ReplyInput{
		SessionID:      reviewed.Session.ID,
		OwnerUserID:    ownerUserID,
		DraftMessageID: reprocessed.Draft.ID,
		IdempotencyKey: "idem-filter-review-3",
	}); err != nil {
		t.Fatalf("reply reviewed session: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/chat/sessions?draft_review_status=pending_review&limit=10", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var sessions []Session
	if err := json.Unmarshal(rec.Body.Bytes(), &sessions); err != nil {
		t.Fatalf("unmarshal sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected one pending review session, got %d", len(sessions))
	}
	if sessions[0].ID != pending.Session.ID {
		t.Fatalf("expected pending session %s, got %s", pending.Session.ID, sessions[0].ID)
	}
	if sessions[0].DraftReviewStatus != "PENDING_REVIEW" {
		t.Fatalf("expected draft review status PENDING_REVIEW, got %s", sessions[0].DraftReviewStatus)
	}
}

func TestListSessionsFiltersByDraftAutoSendStatus(t *testing.T) {
	store := newFakeStore()
	now := time.Now().UTC()

	retryPending, _ := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:       "WHATSAPP",
		ContactKey:    "5511666666661",
		CustomerPhone: "5511666666661",
		LastMessageAt: timePointer(now.Add(-3 * time.Minute)),
		Metadata: map[string]interface{}{
			"agent": map[string]interface{}{
				"status":             agentStatusDraftGenerated,
				"draft_generated_at": now.Add(-10 * time.Minute).Format(time.RFC3339Nano),
				"auto_send_status":   draftAutoSendStatusRetryPending,
				"auto_send_reasons":  []string{draftAutoSendReasonDeliveryFail},
			},
		},
	})
	_, _ = store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:       "WHATSAPP",
		ContactKey:    "5511666666662",
		CustomerPhone: "5511666666662",
		LastMessageAt: timePointer(now.Add(-2 * time.Minute)),
		Metadata: map[string]interface{}{
			"agent": map[string]interface{}{
				"status":             agentStatusDraftGenerated,
				"draft_generated_at": now.Add(-9 * time.Minute).Format(time.RFC3339Nano),
				"auto_send_status":   draftAutoSendStatusBlockedHuman,
				"auto_send_reasons":  []string{draftAutoSendReasonHumanHandoff},
			},
		},
	})

	handler := NewHandler(NewService(store, config.Config{ChatDebounceWindowMS: 1500}))
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/chat/sessions?draft_auto_send_status=auto_send_retry_pending&limit=10", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var sessions []Session
	if err := json.Unmarshal(rec.Body.Bytes(), &sessions); err != nil {
		t.Fatalf("unmarshal sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected one retry-pending session, got %d", len(sessions))
	}
	if sessions[0].ID != retryPending.ID {
		t.Fatalf("expected retry-pending session %s, got %s", retryPending.ID, sessions[0].ID)
	}
	if sessions[0].DraftAutoSendStatus != draftAutoSendStatusRetryPending {
		t.Fatalf("expected draft auto send status %s, got %s", draftAutoSendStatusRetryPending, sessions[0].DraftAutoSendStatus)
	}
}

func TestListSessionsOrdersByReviewPriority(t *testing.T) {
	store := newFakeStore()
	now := time.Now().UTC()
	overdue, _ := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:       "WHATSAPP",
		ContactKey:    "5511333333333",
		CustomerPhone: "5511333333333",
		LastMessageAt: timePointer(now.Add(-20 * time.Minute)),
		Metadata: map[string]interface{}{
			"agent": map[string]interface{}{
				"status":             "DRAFT_GENERATED",
				"draft_generated_at": now.Add(-20 * time.Minute).Format(time.RFC3339Nano),
			},
		},
	})
	dueSoon, _ := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:       "WHATSAPP",
		ContactKey:    "5511222222222",
		CustomerPhone: "5511222222222",
		LastMessageAt: timePointer(now.Add(-10 * time.Minute)),
		Metadata: map[string]interface{}{
			"agent": map[string]interface{}{
				"status":             "DRAFT_GENERATED",
				"draft_generated_at": now.Add(-10 * time.Minute).Format(time.RFC3339Nano),
			},
		},
	})
	fresh, _ := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:       "WHATSAPP",
		ContactKey:    "5511111111111",
		CustomerPhone: "5511111111111",
		LastMessageAt: timePointer(now.Add(-2 * time.Minute)),
		Metadata: map[string]interface{}{
			"agent": map[string]interface{}{
				"status":             "DRAFT_GENERATED",
				"draft_generated_at": now.Add(-2 * time.Minute).Format(time.RFC3339Nano),
			},
		},
	})
	reviewed, _ := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:       "WHATSAPP",
		ContactKey:    "5511444444444",
		CustomerPhone: "5511444444444",
		LastMessageAt: timePointer(now.Add(-1 * time.Minute)),
		Metadata: map[string]interface{}{
			"agent": map[string]interface{}{
				"status": "DRAFT_REVIEWED",
			},
		},
	})
	normal, _ := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:       "WHATSAPP",
		ContactKey:    "5511555555555",
		CustomerPhone: "5511555555555",
		LastMessageAt: timePointer(now),
	})

	handler := NewHandler(NewService(store, config.Config{ChatDebounceWindowMS: 1500}))
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/chat/sessions?order_by=review_priority&limit=10", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var sessions []Session
	if err := json.Unmarshal(rec.Body.Bytes(), &sessions); err != nil {
		t.Fatalf("unmarshal sessions: %v", err)
	}
	if len(sessions) != 5 {
		t.Fatalf("expected five sessions, got %d", len(sessions))
	}
	if sessions[0].ID != overdue.ID {
		t.Fatalf("expected overdue draft session first, got %s", sessions[0].ID)
	}
	if sessions[1].ID != dueSoon.ID {
		t.Fatalf("expected due soon draft session second, got %s", sessions[1].ID)
	}
	if sessions[2].ID != fresh.ID {
		t.Fatalf("expected fresh draft session third, got %s", sessions[2].ID)
	}
	if sessions[3].ID != reviewed.ID {
		t.Fatalf("expected reviewed draft session fourth, got %s", sessions[3].ID)
	}
	if sessions[4].ID != normal.ID {
		t.Fatalf("expected normal session fifth, got %s", sessions[4].ID)
	}
}

func TestGetSessionsSummaryReturnsReviewCounters(t *testing.T) {
	store := newFakeStore()
	ownerUserID := uuid.NewString()
	now := time.Now().UTC()

	pending, _ := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:       "WHATSAPP",
		ContactKey:    "5511000000001",
		CustomerPhone: "5511000000001",
		LastMessageAt: timePointer(now.Add(-3 * time.Minute)),
		Metadata: map[string]interface{}{
			"agent": map[string]interface{}{
				"status": "DRAFT_GENERATED",
			},
		},
	})
	reviewed, _ := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:       "WHATSAPP",
		ContactKey:    "5511000000002",
		CustomerPhone: "5511000000002",
		LastMessageAt: timePointer(now.Add(-2 * time.Minute)),
		Metadata: map[string]interface{}{
			"agent": map[string]interface{}{
				"status":              "DRAFT_REVIEWED",
				"reviewed_by_user_id": ownerUserID,
			},
		},
	})
	item := store.sessions[reviewed.ID]
	item.HandoffStatus = "HUMAN"
	item.CurrentOwnerUserID = ownerUserID
	store.sessions[reviewed.ID] = item

	_, _ = store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:       "WHATSAPP",
		ContactKey:    "5511000000004",
		CustomerPhone: "5511000000004",
		LastMessageAt: timePointer(now.Add(-90 * time.Second)),
		Metadata: map[string]interface{}{
			"agent": map[string]interface{}{
				"status":             "DRAFT_GENERATED",
				"draft_generated_at": now.Add(-6 * time.Minute).Format(time.RFC3339Nano),
				"auto_send_status":   draftAutoSendStatusRetryPending,
				"auto_send_reasons":  []string{draftAutoSendReasonDeliveryFail},
			},
		},
	})
	_, _ = store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:       "WHATSAPP",
		ContactKey:    "5511000000005",
		CustomerPhone: "5511000000005",
		LastMessageAt: timePointer(now.Add(-30 * time.Second)),
		Metadata: map[string]interface{}{
			"agent": map[string]interface{}{
				"status":             "DRAFT_GENERATED",
				"draft_generated_at": now.Add(-4 * time.Minute).Format(time.RFC3339Nano),
				"auto_send_status":   draftAutoSendStatusBlockedHuman,
				"auto_send_reasons":  []string{draftAutoSendReasonHumanHandoff},
			},
		},
	})
	_, _ = store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:       "WHATSAPP",
		ContactKey:    "5511000000003",
		CustomerPhone: "5511000000003",
		LastMessageAt: timePointer(now.Add(-1 * time.Minute)),
	})

	handler := NewHandler(NewService(store, config.Config{ChatDebounceWindowMS: 1500}))
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/chat/sessions/summary?channel=whatsapp&draft_review_status=pending_review", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var out SessionsSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal summary: %v", err)
	}
	if out.TotalCount != 5 {
		t.Fatalf("expected total_count 5, got %d", out.TotalCount)
	}
	if out.ReviewSLASeconds != 15*60 {
		t.Fatalf("expected review_sla_seconds 900, got %d", out.ReviewSLASeconds)
	}
	if out.PendingReviewCount != 3 {
		t.Fatalf("expected pending_review_count 3, got %d", out.PendingReviewCount)
	}
	if out.ReviewedCount != 1 {
		t.Fatalf("expected reviewed_count 1, got %d", out.ReviewedCount)
	}
	if out.NoDraftCount != 1 {
		t.Fatalf("expected no_draft_count 1, got %d", out.NoDraftCount)
	}
	if out.HumanOwnedCount != 1 {
		t.Fatalf("expected human_owned_count 1, got %d", out.HumanOwnedCount)
	}
	if out.BotOwnedCount != 4 {
		t.Fatalf("expected bot_owned_count 4, got %d", out.BotOwnedCount)
	}
	if out.DueSoonReviewCount != 0 {
		t.Fatalf("expected due_soon_review_count 0, got %d", out.DueSoonReviewCount)
	}
	if out.OverdueReviewCount != 0 {
		t.Fatalf("expected overdue_review_count 0, got %d", out.OverdueReviewCount)
	}
	if out.HighPriorityReviewCount != 0 {
		t.Fatalf("expected high_priority_review_count 0, got %d", out.HighPriorityReviewCount)
	}
	if out.MediumPriorityReviewCount != 0 {
		t.Fatalf("expected medium_priority_review_count 0, got %d", out.MediumPriorityReviewCount)
	}
	if out.LowPriorityReviewCount != 3 {
		t.Fatalf("expected low_priority_review_count 3, got %d", out.LowPriorityReviewCount)
	}
	if out.AutoSendRetryPendingCount != 1 {
		t.Fatalf("expected auto_send_retry_pending_count 1, got %d", out.AutoSendRetryPendingCount)
	}
	if out.AutoSendBlockedHumanCount != 1 {
		t.Fatalf("expected auto_send_blocked_human_count 1, got %d", out.AutoSendBlockedHumanCount)
	}
	if out.AutoSendIssueCount != 2 {
		t.Fatalf("expected auto_send_issue_count 2, got %d", out.AutoSendIssueCount)
	}
	if out.HasReviewAlert {
		t.Fatalf("expected has_review_alert false, got true")
	}
	if out.OldestPendingAgeSeconds < 0 {
		t.Fatalf("expected oldest_pending_age_seconds >= 0, got %d", out.OldestPendingAgeSeconds)
	}
	_ = pending
}

func TestGetSessionsSummaryReturnsWarningAlertForDueSoonQueue(t *testing.T) {
	store := newFakeStore()
	now := time.Now().UTC()

	_, _ = store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:       "WHATSAPP",
		ContactKey:    "5511000000009",
		CustomerPhone: "5511000000009",
		LastMessageAt: timePointer(now.Add(-10 * time.Minute)),
		Metadata: map[string]interface{}{
			"agent": map[string]interface{}{
				"status":             "DRAFT_GENERATED",
				"draft_generated_at": now.Add(-10 * time.Minute).Format(time.RFC3339Nano),
			},
		},
	})

	handler := NewHandler(NewService(store, config.Config{ChatDebounceWindowMS: 1500, ChatReviewSLAMinutes: 15}))
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/chat/sessions/summary?channel=whatsapp", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var out SessionsSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal summary: %v", err)
	}
	if !out.HasReviewAlert {
		t.Fatalf("expected has_review_alert true")
	}
	if out.ReviewAlertLevel != "WARNING" {
		t.Fatalf("expected review_alert_level WARNING, got %s", out.ReviewAlertLevel)
	}
	if out.ReviewAlertCode != "REVIEW_QUEUE_DUE_SOON" {
		t.Fatalf("expected review_alert_code REVIEW_QUEUE_DUE_SOON, got %s", out.ReviewAlertCode)
	}
	if out.ReviewAlertSessionCount != 1 {
		t.Fatalf("expected review_alert_session_count 1, got %d", out.ReviewAlertSessionCount)
	}
}

func TestListSessionsAndSummaryExposeReviewAging(t *testing.T) {
	store := newFakeStore()
	now := time.Now().UTC()
	_, _ = store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:       "WHATSAPP",
		ContactKey:    "5511000000010",
		CustomerPhone: "5511000000010",
		LastMessageAt: timePointer(now.Add(-20 * time.Minute)),
		Metadata: map[string]interface{}{
			"agent": map[string]interface{}{
				"status":             "DRAFT_GENERATED",
				"draft_generated_at": now.Add(-20 * time.Minute).Format(time.RFC3339Nano),
			},
		},
	})
	_, _ = store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:       "WHATSAPP",
		ContactKey:    "5511000000011",
		CustomerPhone: "5511000000011",
		LastMessageAt: timePointer(now.Add(-10 * time.Minute)),
		Metadata: map[string]interface{}{
			"agent": map[string]interface{}{
				"status":             "DRAFT_GENERATED",
				"draft_generated_at": now.Add(-10 * time.Minute).Format(time.RFC3339Nano),
			},
		},
	})
	_, _ = store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:       "WHATSAPP",
		ContactKey:    "5511000000012",
		CustomerPhone: "5511000000012",
		LastMessageAt: timePointer(now.Add(-2 * time.Minute)),
		Metadata: map[string]interface{}{
			"agent": map[string]interface{}{
				"status":             "DRAFT_GENERATED",
				"draft_generated_at": now.Add(-2 * time.Minute).Format(time.RFC3339Nano),
			},
		},
	})

	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500, ChatReviewSLAMinutes: 15})
	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	listReq := httptest.NewRequest(http.MethodGet, "/chat/sessions?order_by=review_priority&limit=10", nil)
	listRec := httptest.NewRecorder()
	r.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, listRec.Code)
	}

	var sessions []Session
	if err := json.Unmarshal(listRec.Body.Bytes(), &sessions); err != nil {
		t.Fatalf("unmarshal sessions: %v", err)
	}
	if len(sessions) != 3 {
		t.Fatalf("expected three sessions, got %d", len(sessions))
	}
	if sessions[0].ContactKey != "5511000000010" {
		t.Fatalf("expected overdue session first, got %s", sessions[0].ContactKey)
	}
	if sessions[1].ContactKey != "5511000000011" {
		t.Fatalf("expected due soon session second, got %s", sessions[1].ContactKey)
	}
	if sessions[2].ContactKey != "5511000000012" {
		t.Fatalf("expected fresh session third, got %s", sessions[2].ContactKey)
	}
	byContactKey := map[string]Session{}
	for _, item := range sessions {
		byContactKey[item.ContactKey] = item
	}
	if byContactKey["5511000000010"].DraftPendingAgeBucket != "OVERDUE" {
		t.Fatalf("expected overdue bucket for 5511000000010, got %s", byContactKey["5511000000010"].DraftPendingAgeBucket)
	}
	if !byContactKey["5511000000010"].DraftReviewOverdue {
		t.Fatalf("expected overdue session to be flagged")
	}
	if byContactKey["5511000000010"].DraftReviewPriority != "HIGH" {
		t.Fatalf("expected HIGH priority for 5511000000010, got %s", byContactKey["5511000000010"].DraftReviewPriority)
	}
	if !byContactKey["5511000000010"].DraftReviewAlertActive {
		t.Fatalf("expected active alert for 5511000000010")
	}
	if byContactKey["5511000000010"].DraftReviewAlertLevel != "CRITICAL" {
		t.Fatalf("expected CRITICAL alert for 5511000000010, got %s", byContactKey["5511000000010"].DraftReviewAlertLevel)
	}
	if byContactKey["5511000000011"].DraftPendingAgeBucket != "DUE_SOON" {
		t.Fatalf("expected due soon bucket for 5511000000011, got %s", byContactKey["5511000000011"].DraftPendingAgeBucket)
	}
	if byContactKey["5511000000011"].DraftReviewOverdue {
		t.Fatalf("expected due soon session not to be overdue")
	}
	if byContactKey["5511000000011"].DraftReviewPriority != "MEDIUM" {
		t.Fatalf("expected MEDIUM priority for 5511000000011, got %s", byContactKey["5511000000011"].DraftReviewPriority)
	}
	if !byContactKey["5511000000011"].DraftReviewAlertActive {
		t.Fatalf("expected active alert for 5511000000011")
	}
	if byContactKey["5511000000011"].DraftReviewAlertLevel != "WARNING" {
		t.Fatalf("expected WARNING alert for 5511000000011, got %s", byContactKey["5511000000011"].DraftReviewAlertLevel)
	}
	if byContactKey["5511000000012"].DraftPendingAgeBucket != "FRESH" {
		t.Fatalf("expected fresh bucket for 5511000000012, got %s", byContactKey["5511000000012"].DraftPendingAgeBucket)
	}
	if byContactKey["5511000000012"].DraftReviewPriority != "LOW" {
		t.Fatalf("expected LOW priority for 5511000000012, got %s", byContactKey["5511000000012"].DraftReviewPriority)
	}
	if byContactKey["5511000000012"].DraftReviewAlertActive {
		t.Fatalf("expected inactive alert for 5511000000012")
	}

	summaryReq := httptest.NewRequest(http.MethodGet, "/chat/sessions/summary?channel=whatsapp", nil)
	summaryRec := httptest.NewRecorder()
	r.ServeHTTP(summaryRec, summaryReq)

	if summaryRec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, summaryRec.Code)
	}

	var summary SessionsSummary
	if err := json.Unmarshal(summaryRec.Body.Bytes(), &summary); err != nil {
		t.Fatalf("unmarshal summary: %v", err)
	}
	if summary.PendingReviewCount != 3 {
		t.Fatalf("expected pending_review_count 3, got %d", summary.PendingReviewCount)
	}
	if summary.DueSoonReviewCount != 1 {
		t.Fatalf("expected due_soon_review_count 1, got %d", summary.DueSoonReviewCount)
	}
	if summary.OverdueReviewCount != 1 {
		t.Fatalf("expected overdue_review_count 1, got %d", summary.OverdueReviewCount)
	}
	if summary.HighPriorityReviewCount != 1 {
		t.Fatalf("expected high_priority_review_count 1, got %d", summary.HighPriorityReviewCount)
	}
	if summary.MediumPriorityReviewCount != 1 {
		t.Fatalf("expected medium_priority_review_count 1, got %d", summary.MediumPriorityReviewCount)
	}
	if summary.LowPriorityReviewCount != 1 {
		t.Fatalf("expected low_priority_review_count 1, got %d", summary.LowPriorityReviewCount)
	}
	if !summary.HasReviewAlert {
		t.Fatalf("expected has_review_alert true")
	}
	if summary.ReviewAlertLevel != "CRITICAL" {
		t.Fatalf("expected review_alert_level CRITICAL, got %s", summary.ReviewAlertLevel)
	}
	if summary.ReviewAlertCode != "REVIEW_QUEUE_OVERDUE" {
		t.Fatalf("expected review_alert_code REVIEW_QUEUE_OVERDUE, got %s", summary.ReviewAlertCode)
	}
	if summary.ReviewAlertSessionCount != 1 {
		t.Fatalf("expected review_alert_session_count 1, got %d", summary.ReviewAlertSessionCount)
	}
	if summary.OldestPendingAgeSeconds < 20*60-5 {
		t.Fatalf("expected oldest_pending_age_seconds around 1200, got %d", summary.OldestPendingAgeSeconds)
	}
}

func TestReplyCreatesOutboundRecordForHumanOwner(t *testing.T) {
	store := newFakeStore()
	session, _ := store.seedSessionWithMessage("5511888888888", "ola")
	ownerID := uuid.NewString()
	item := store.sessions[session.ID]
	item.HandoffStatus = "HUMAN"
	item.CurrentOwnerUserID = ownerID
	store.sessions[session.ID] = item

	handler := NewHandler(NewService(store, config.Config{ChatDebounceWindowMS: 1500}))
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+session.ID+"/reply", bytes.NewBufferString(`{
		"owner_user_id":"`+ownerID+`",
		"body":"posso te ajudar com mais algo?",
		"sender_name":"Operador",
		"idempotency_key":"reply-1",
		"metadata":{"source":"dashboard"}
	}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, rec.Code)
	}

	var out ReplyResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if out.Idempotent {
		t.Fatalf("expected new reply, got idempotent response")
	}
	if out.Session.HandoffStatus != "HUMAN" {
		t.Fatalf("expected session to remain HUMAN, got %s", out.Session.HandoffStatus)
	}
	if out.Message.Direction != "OUTBOUND" {
		t.Fatalf("expected outbound message, got %s", out.Message.Direction)
	}
	if out.Message.ProcessingStatus != "MANUAL_PENDING" {
		t.Fatalf("expected MANUAL_PENDING processing status, got %s", out.Message.ProcessingStatus)
	}
	if out.Outbound.Status != "MANUAL_PENDING" {
		t.Fatalf("expected MANUAL_PENDING outbound status, got %s", out.Outbound.Status)
	}
	if out.Outbound.Recipient != session.ContactKey {
		t.Fatalf("expected outbound recipient %s, got %s", session.ContactKey, out.Outbound.Recipient)
	}
	buffer, ok := out.Session.Metadata["buffer"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected buffer metadata to be present")
	}
	if got := asString(buffer["status"]); got != bufferStatusIdle {
		t.Fatalf("expected buffer status %s, got %s", bufferStatusIdle, got)
	}
	if len(store.outbounds) != 1 {
		t.Fatalf("expected one outbound record, got %d", len(store.outbounds))
	}
}

func TestReplyDeliversImmediatelyWhenSenderIsEnabled(t *testing.T) {
	store := newFakeStore()
	session, _ := store.seedSessionWithMessage("5511888888888@s.whatsapp.net", "ola")
	ownerID := uuid.NewString()
	item := store.sessions[session.ID]
	item.HandoffStatus = "HUMAN"
	item.CurrentOwnerUserID = ownerID
	store.sessions[session.ID] = item

	sender := &fakeReplySender{
		enabled: true,
		result: SendReplyResult{
			ProviderMessageID: "MSG-SENT-1",
			ProviderStatus:    "SENT",
			Payload:           map[string]interface{}{"provider": "EVOLUTION"},
			SentAt:            time.Now().UTC(),
		},
	}
	handler := NewHandler(NewService(store, config.Config{ChatDebounceWindowMS: 1500}, sender))
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+session.ID+"/reply", bytes.NewBufferString(`{
		"owner_user_id":"`+ownerID+`",
		"body":"posso te ajudar com mais algo?",
		"sender_name":"Operador",
		"idempotency_key":"reply-send-1"
	}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, rec.Code)
	}

	var out ReplyResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if out.Outbound.Status != "SENT" {
		t.Fatalf("expected outbound status SENT, got %s", out.Outbound.Status)
	}
	if out.Outbound.ProviderMessageID != "MSG-SENT-1" {
		t.Fatalf("expected provider message id MSG-SENT-1, got %s", out.Outbound.ProviderMessageID)
	}
	if out.Message.ProcessingStatus != "SENT" {
		t.Fatalf("expected processing status SENT, got %s", out.Message.ProcessingStatus)
	}
	if sender.calls != 1 {
		t.Fatalf("expected one sender call, got %d", sender.calls)
	}
}

func TestReplyRejectsWhenHumanOwnerMismatch(t *testing.T) {
	store := newFakeStore()
	session, _ := store.seedSessionWithMessage("5511888888888", "ola")
	item := store.sessions[session.ID]
	item.HandoffStatus = "HUMAN"
	item.CurrentOwnerUserID = uuid.NewString()
	store.sessions[session.ID] = item

	handler := NewHandler(NewService(store, config.Config{ChatDebounceWindowMS: 1500}))
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+session.ID+"/reply", bytes.NewBufferString(`{
		"owner_user_id":"`+uuid.NewString()+`",
		"body":"resposta"
	}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, rec.Code)
	}
}

func TestReplyRejectsWhenSessionHasNoActiveHumanOwner(t *testing.T) {
	store := newFakeStore()
	session, _ := store.seedSessionWithMessage("5511888888888", "ola")

	handler := NewHandler(NewService(store, config.Config{ChatDebounceWindowMS: 1500}))
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+session.ID+"/reply", bytes.NewBufferString(`{
		"owner_user_id":"`+uuid.NewString()+`",
		"body":"resposta"
	}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, rec.Code)
	}
}

func TestReplyReturnsExistingOnIdempotency(t *testing.T) {
	store := newFakeStore()
	session, _ := store.seedSessionWithMessage("5511888888888", "ola")
	ownerID := uuid.NewString()
	item := store.sessions[session.ID]
	item.HandoffStatus = "HUMAN"
	item.CurrentOwnerUserID = ownerID
	store.sessions[session.ID] = item

	handler := NewHandler(NewService(store, config.Config{ChatDebounceWindowMS: 1500}))
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	body := `{
		"owner_user_id":"` + ownerID + `",
		"body":"resposta assistida",
		"idempotency_key":"reply-idem-1"
	}`

	firstReq := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+session.ID+"/reply", bytes.NewBufferString(body))
	firstRec := httptest.NewRecorder()
	r.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusCreated {
		t.Fatalf("expected first status %d, got %d", http.StatusCreated, firstRec.Code)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+session.ID+"/reply", bytes.NewBufferString(body))
	secondRec := httptest.NewRecorder()
	r.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusOK {
		t.Fatalf("expected second status %d, got %d", http.StatusOK, secondRec.Code)
	}

	var out ReplyResult
	if err := json.Unmarshal(secondRec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !out.Idempotent {
		t.Fatalf("expected idempotent response")
	}
	if len(store.outbounds) != 1 {
		t.Fatalf("expected one outbound record, got %d", len(store.outbounds))
	}
}

func TestReplyRetriesDeliveryOnSameIdempotencyKeyAfterFailure(t *testing.T) {
	store := newFakeStore()
	session, _ := store.seedSessionWithMessage("5511888888888@s.whatsapp.net", "ola")
	ownerID := uuid.NewString()
	item := store.sessions[session.ID]
	item.HandoffStatus = "HUMAN"
	item.CurrentOwnerUserID = ownerID
	store.sessions[session.ID] = item

	sender := &fakeReplySender{
		enabled: true,
		errs: []error{
			errors.New("gateway timeout"),
			nil,
		},
		results: []SendReplyResult{
			{},
			{
				ProviderMessageID: "MSG-RETRY-1",
				ProviderStatus:    "SENT",
				Payload:           map[string]interface{}{"provider": "EVOLUTION"},
				SentAt:            time.Now().UTC(),
			},
		},
	}
	handler := NewHandler(NewService(store, config.Config{ChatDebounceWindowMS: 1500}, sender))
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	body := `{
		"owner_user_id":"` + ownerID + `",
		"body":"resposta assistida",
		"idempotency_key":"reply-send-retry-1"
	}`

	firstReq := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+session.ID+"/reply", bytes.NewBufferString(body))
	firstRec := httptest.NewRecorder()
	r.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusBadGateway {
		t.Fatalf("expected first status %d, got %d", http.StatusBadGateway, firstRec.Code)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+session.ID+"/reply", bytes.NewBufferString(body))
	secondRec := httptest.NewRecorder()
	r.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusOK {
		t.Fatalf("expected second status %d, got %d", http.StatusOK, secondRec.Code)
	}

	var out ReplyResult
	if err := json.Unmarshal(secondRec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !out.Idempotent {
		t.Fatalf("expected idempotent response on retry")
	}
	if out.Outbound.Status != "SENT" {
		t.Fatalf("expected outbound status SENT after retry, got %s", out.Outbound.Status)
	}
	if out.Outbound.ProviderMessageID != "MSG-RETRY-1" {
		t.Fatalf("expected provider message id MSG-RETRY-1, got %s", out.Outbound.ProviderMessageID)
	}
	if sender.calls != 2 {
		t.Fatalf("expected two sender calls, got %d", sender.calls)
	}
}

func TestReplyApprovesAutomationDraftWhenDraftMessageIDIsProvided(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Temos vaga para esse trecho.",
			Model:              "gpt-test",
			ProviderResponseID: "resp-review-1",
		},
	})

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511888888888",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-review-1",
			IdempotencyKey:    "idem-review-1",
			Body:              "quero uma passagem",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}

	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess message: %v", err)
	}
	if reprocessed.Draft == nil {
		t.Fatalf("expected draft to be created")
	}

	ownerID := uuid.NewString()
	session := store.sessions[ingested.Session.ID]
	session.HandoffStatus = "HUMAN"
	session.CurrentOwnerUserID = ownerID
	store.sessions[session.ID] = session

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+session.ID+"/reply", bytes.NewBufferString(`{
		"owner_user_id":"`+ownerID+`",
		"draft_message_id":"`+reprocessed.Draft.ID+`",
		"idempotency_key":"reply-review-1"
	}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, rec.Code)
	}

	var out ReplyResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if out.Draft == nil {
		t.Fatalf("expected reviewed draft in response")
	}
	if out.Draft.ProcessingStatus != messageStatusAutomationReviewed {
		t.Fatalf("expected reviewed draft status %s, got %s", messageStatusAutomationReviewed, out.Draft.ProcessingStatus)
	}
	if out.Message.Body != reprocessed.Draft.Body {
		t.Fatalf("expected reply body to reuse draft body")
	}
	if got := asString(out.Message.Payload["draft_message_id"]); got != reprocessed.Draft.ID {
		t.Fatalf("expected draft_message_id %s, got %s", reprocessed.Draft.ID, got)
	}
	agent, ok := out.Session.Metadata["agent"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected agent metadata")
	}
	if got := asString(agent["status"]); got != agentStatusDraftReviewed {
		t.Fatalf("expected agent status %s, got %s", agentStatusDraftReviewed, got)
	}
}

func TestReplyCanEditAutomationDraftBeforeSending(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Temos vaga para esse trecho.",
			Model:              "gpt-test",
			ProviderResponseID: "resp-review-2",
		},
	})

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511888888888",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-review-2",
			IdempotencyKey:    "idem-review-2",
			Body:              "quero uma passagem",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}

	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess message: %v", err)
	}
	if reprocessed.Draft == nil {
		t.Fatalf("expected draft to be created")
	}

	ownerID := uuid.NewString()
	session := store.sessions[ingested.Session.ID]
	session.HandoffStatus = "HUMAN"
	session.CurrentOwnerUserID = ownerID
	store.sessions[session.ID] = session

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+session.ID+"/reply", bytes.NewBufferString(`{
		"owner_user_id":"`+ownerID+`",
		"draft_message_id":"`+reprocessed.Draft.ID+`",
		"body":"Temos vaga. Quer que eu te passe as datas disponiveis?",
		"idempotency_key":"reply-review-2"
	}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, rec.Code)
	}

	var out ReplyResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if out.Message.Body != "Temos vaga. Quer que eu te passe as datas disponiveis?" {
		t.Fatalf("expected edited reply body, got %q", out.Message.Body)
	}
	if out.Draft == nil {
		t.Fatalf("expected reviewed draft")
	}
	if got := asString(out.Draft.NormalizedPayload["review_action"]); got != "EDITED" {
		t.Fatalf("expected review_action EDITED, got %s", got)
	}
}

func TestReplyRejectsDraftReviewWhenDraftIsNotActiveAutomationDraft(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Temos vaga para esse trecho.",
			Model:              "gpt-test",
			ProviderResponseID: "resp-review-3",
		},
	})

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511888888888",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-review-3",
			IdempotencyKey:    "idem-review-3",
			Body:              "quero uma passagem",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}

	reprocessed, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess message: %v", err)
	}
	if reprocessed.Draft == nil {
		t.Fatalf("expected draft to be created")
	}

	ownerID := uuid.NewString()
	session := store.sessions[ingested.Session.ID]
	session.HandoffStatus = "HUMAN"
	session.CurrentOwnerUserID = ownerID
	store.sessions[session.ID] = session

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	firstReq := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+session.ID+"/reply", bytes.NewBufferString(`{
		"owner_user_id":"`+ownerID+`",
		"draft_message_id":"`+reprocessed.Draft.ID+`",
		"idempotency_key":"reply-review-3a"
	}`))
	firstRec := httptest.NewRecorder()
	r.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusCreated {
		t.Fatalf("expected first status %d, got %d", http.StatusCreated, firstRec.Code)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+session.ID+"/reply", bytes.NewBufferString(`{
		"owner_user_id":"`+ownerID+`",
		"draft_message_id":"`+reprocessed.Draft.ID+`",
		"idempotency_key":"reply-review-3b"
	}`))
	secondRec := httptest.NewRecorder()
	r.ServeHTTP(secondRec, secondReq)

	if secondRec.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, secondRec.Code)
	}
}

func TestReplyRejectsInvalidDraftMessageID(t *testing.T) {
	store := newFakeStore()
	session, _ := store.seedSessionWithMessage("5511888888888", "ola")
	ownerID := uuid.NewString()
	item := store.sessions[session.ID]
	item.HandoffStatus = "HUMAN"
	item.CurrentOwnerUserID = ownerID
	store.sessions[session.ID] = item

	handler := NewHandler(NewService(store, config.Config{ChatDebounceWindowMS: 1500}))
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+session.ID+"/reply", bytes.NewBufferString(`{
		"owner_user_id":"`+ownerID+`",
		"draft_message_id":"not-a-uuid"
	}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestReplyMediaCreatesOutboundRecordForHumanOwner(t *testing.T) {
	store := newFakeStore()
	session, _ := store.seedSessionWithMessage("5511888888888", "ola")
	ownerID := uuid.NewString()
	item := store.sessions[session.ID]
	item.HandoffStatus = "HUMAN"
	item.CurrentOwnerUserID = ownerID
	store.sessions[session.ID] = item

	handler := NewHandler(NewService(store, config.Config{ChatDebounceWindowMS: 1500}))
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("owner_user_id", ownerID); err != nil {
		t.Fatalf("write owner_user_id: %v", err)
	}
	if err := writer.WriteField("caption", "Segue o documento"); err != nil {
		t.Fatalf("write caption: %v", err)
	}
	if err := writer.WriteField("media_type", "DOCUMENT"); err != nil {
		t.Fatalf("write media_type: %v", err)
	}
	part, err := writer.CreateFormFile("file", "rg.pdf")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte("fake-pdf-content")); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+session.ID+"/reply/media", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusCreated, rec.Code, rec.Body.String())
	}

	var out ReplyResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if out.Message.Kind != "DOCUMENT" {
		t.Fatalf("expected message kind DOCUMENT, got %s", out.Message.Kind)
	}
	if got := asString(out.Message.Payload["media_kind"]); got != "document" {
		t.Fatalf("expected media_kind document, got %s", got)
	}
	if got := asString(out.Message.Payload["media_file_name"]); got != "rg.pdf" {
		t.Fatalf("expected media_file_name rg.pdf, got %s", got)
	}
	if got := asString(out.Message.Payload["media_base64"]); got == "" {
		t.Fatalf("expected media_base64 payload")
	}
}

func TestIngestMessageWithHumanOwnerBlocksAgentProcessing(t *testing.T) {
	store := newFakeStore()
	session, _ := store.seedSessionWithMessage("5511888888888", "ola")
	ownerID := uuid.NewString()
	item := store.sessions[session.ID]
	item.HandoffStatus = "HUMAN"
	item.CurrentOwnerUserID = ownerID
	store.sessions[session.ID] = item

	handler := NewHandler(NewService(store, config.Config{ChatDebounceWindowMS: 1500}))
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/messages/ingest", bytes.NewBufferString(`{
		"contact_key":"5511888888888",
		"message":{"direction":"INBOUND","provider_message_id":"msg-human-1","idempotency_key":"idem-human-1","body":"preciso falar com atendente","processing_status":"RECEIVED"}
	}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, rec.Code)
	}

	var out IngestMessageResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if out.Message.ProcessingStatus != "HUMAN_OWNED_PENDING" {
		t.Fatalf("expected HUMAN_OWNED_PENDING, got %s", out.Message.ProcessingStatus)
	}
	if got := asString(out.Message.NormalizedPayload["agent_block_reason"]); got != agentBlockReasonHumanOwnerActive {
		t.Fatalf("expected agent block reason %s, got %s", agentBlockReasonHumanOwnerActive, got)
	}
	if got := asString(out.Message.NormalizedPayload["current_owner_user_id"]); got != ownerID {
		t.Fatalf("expected current owner %s, got %s", ownerID, got)
	}
	buffer, ok := out.Session.Metadata["buffer"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected buffer metadata to be present")
	}
	if got := asString(buffer["status"]); got != bufferStatusIdle {
		t.Fatalf("expected blocked buffer to remain %s, got %s", bufferStatusIdle, got)
	}
	if got := asString(buffer["agent_block_reason"]); got != agentBlockReasonHumanOwnerActive {
		t.Fatalf("expected buffer block reason %s, got %s", agentBlockReasonHumanOwnerActive, got)
	}
}

func TestPresenceSignalIsSkippedWhenHumanOwnerIsActive(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500})

	session, _ := store.seedSessionWithMessage("5511888888888", "ola")
	ownerID := uuid.NewString()
	item := store.sessions[session.ID]
	item.HandoffStatus = "HUMAN"
	item.CurrentOwnerUserID = ownerID
	item.Metadata["buffer"] = map[string]interface{}{
		"status":        bufferStatusPending,
		"pending_until": time.Now().UTC().Add(1 * time.Second).Format(time.RFC3339Nano),
	}
	store.sessions[session.ID] = item

	result, err := svc.ApplyPresenceSignal(context.Background(), ApplyPresenceSignalInput{
		ContactKey:     "5511888888888",
		PresenceStatus: "typing",
	})
	if err != nil {
		t.Fatalf("apply presence: %v", err)
	}
	if result.Status != "skipped" {
		t.Fatalf("expected skipped status, got %s", result.Status)
	}
	if result.Reason != agentBlockReasonHumanOwnerActive {
		t.Fatalf("expected reason %s, got %s", agentBlockReasonHumanOwnerActive, result.Reason)
	}
}

func TestRequestHandoffCreatesRecordAndUpdatesSession(t *testing.T) {
	store := newFakeStore()
	session, _ := store.seedSessionWithMessage("5511888888888", "ola")
	profiles := &fakeProfileEnsurer{}
	handler := NewHandler(NewService(store, config.Config{ChatDebounceWindowMS: 1500}, profiles))

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	userID := uuid.NewString()
	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+session.ID+"/handoff", bytes.NewBufferString(`{
		"requested_by":"operator",
		"reason":"cliente pediu atendimento humano",
		"metadata":{"source":"dashboard"}
	}`))
	req = req.WithContext(auth.WithUser(req.Context(), auth.AuthUser{ID: userID, Email: "operador@example.com"}))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, rec.Code)
	}

	var out RequestHandoffResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if out.Session.HandoffStatus != "HUMAN" {
		t.Fatalf("expected session handoff status HUMAN, got %s", out.Session.HandoffStatus)
	}
	if out.Handoff.Status != "REQUESTED" {
		t.Fatalf("expected handoff status REQUESTED, got %s", out.Handoff.Status)
	}
	if out.Handoff.RequestedBy != "OPERATOR" {
		t.Fatalf("expected requested_by to be normalized, got %s", out.Handoff.RequestedBy)
	}
	if len(store.handoffs) != 1 {
		t.Fatalf("expected one persisted handoff, got %d", len(store.handoffs))
	}
	if out.Session.CurrentOwnerUserID != userID {
		t.Fatalf("expected current owner %s, got %s", userID, out.Session.CurrentOwnerUserID)
	}
	if out.Handoff.AssignedUserID != userID {
		t.Fatalf("expected handoff assigned user %s, got %s", userID, out.Handoff.AssignedUserID)
	}
	if profiles.calls != 1 || profiles.lastUser.ID != userID {
		t.Fatalf("expected profile ensure for authenticated user, got calls=%d user=%s", profiles.calls, profiles.lastUser.ID)
	}
}

func TestRequestHandoffIgnoresBodyAssignedUserID(t *testing.T) {
	store := newFakeStore()
	session, _ := store.seedSessionWithMessage("5511888888888", "ola")
	profiles := &fakeProfileEnsurer{}
	handler := NewHandler(NewService(store, config.Config{ChatDebounceWindowMS: 1500}, profiles))

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	bodyOwnerID := uuid.NewString()
	authOwnerID := uuid.NewString()
	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+session.ID+"/handoff", bytes.NewBufferString(`{
		"requested_by":"operator",
		"assigned_user_id":"`+bodyOwnerID+`",
		"reason":"cliente pediu atendimento humano"
	}`))
	req = req.WithContext(auth.WithUser(req.Context(), auth.AuthUser{ID: authOwnerID, Email: "operador@example.com"}))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, rec.Code)
	}

	var out RequestHandoffResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if out.Session.HandoffStatus != "HUMAN" {
		t.Fatalf("expected session handoff status HUMAN, got %s", out.Session.HandoffStatus)
	}
	if out.Session.CurrentOwnerUserID != authOwnerID {
		t.Fatalf("expected current owner from auth %s, got %s", authOwnerID, out.Session.CurrentOwnerUserID)
	}
	if out.Handoff.AssignedUserID != authOwnerID {
		t.Fatalf("expected handoff assigned user from auth %s, got %s", authOwnerID, out.Handoff.AssignedUserID)
	}
}

func TestRequestHandoffRequiresAuthenticatedUser(t *testing.T) {
	store := newFakeStore()
	session, _ := store.seedSessionWithMessage("5511888888888", "ola")
	handler := NewHandler(NewService(store, config.Config{ChatDebounceWindowMS: 1500}))

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+session.ID+"/handoff", bytes.NewBufferString(`{
		"assigned_user_id":"not-a-uuid"
	}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestRequestHandoffRejectsWhenAlreadyActive(t *testing.T) {
	store := newFakeStore()
	session, _ := store.seedSessionWithMessage("5511888888888", "ola")
	item := store.sessions[session.ID]
	item.HandoffStatus = "HUMAN_REQUESTED"
	store.sessions[session.ID] = item
	handler := NewHandler(NewService(store, config.Config{ChatDebounceWindowMS: 1500}))

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+session.ID+"/handoff", bytes.NewBufferString(`{}`))
	req = req.WithContext(auth.WithUser(req.Context(), auth.AuthUser{ID: uuid.NewString(), Email: "operador@example.com"}))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, rec.Code)
	}
}

func TestRequestHandoffProfileFailureReturnsForbidden(t *testing.T) {
	store := newFakeStore()
	session, _ := store.seedSessionWithMessage("5511888888888", "ola")
	profiles := &fakeProfileEnsurer{err: ErrUserProfileNotConfigured}
	handler := NewHandler(NewService(store, config.Config{ChatDebounceWindowMS: 1500}, profiles))

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+session.ID+"/handoff", bytes.NewBufferString(`{}`))
	req = req.WithContext(auth.WithUser(req.Context(), auth.AuthUser{ID: uuid.NewString(), Email: "operador@example.com"}))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "USER_PROFILE_NOT_CONFIGURED") {
		t.Fatalf("expected USER_PROFILE_NOT_CONFIGURED response, got %s", rec.Body.String())
	}
}

func TestRequestHandoffForeignKeyFailureReturnsForbidden(t *testing.T) {
	store := newFakeStore()
	session, _ := store.seedSessionWithMessage("5511888888888", "ola")
	store.requestHandoffErr = &pgconn.PgError{Code: "23503", ConstraintName: "chat_handoffs_assigned_user_id_fkey"}
	handler := NewHandler(NewService(store, config.Config{ChatDebounceWindowMS: 1500}))

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+session.ID+"/handoff", bytes.NewBufferString(`{}`))
	req = req.WithContext(auth.WithUser(req.Context(), auth.AuthUser{ID: uuid.NewString(), Email: "operador@example.com"}))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "USER_PROFILE_NOT_CONFIGURED") {
		t.Fatalf("expected USER_PROFILE_NOT_CONFIGURED response, got %s", rec.Body.String())
	}
}

func TestResumeSessionResolvesActiveHandoff(t *testing.T) {
	store := newFakeStore()
	session, _ := store.seedSessionWithMessage("5511888888888", "ola")
	ownerID := uuid.NewString()
	_, err := store.RequestHandoff(context.Background(), RequestHandoffInput{
		SessionID:      session.ID,
		RequestedBy:    "OPERATOR",
		Reason:         "cliente pediu atendimento humano",
		AssignedUserID: ownerID,
		Metadata:       map[string]interface{}{"source": "dashboard"},
	})
	if err != nil {
		t.Fatalf("seed handoff: %v", err)
	}

	handler := NewHandler(NewService(store, config.Config{ChatDebounceWindowMS: 1500}))
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+session.ID+"/resume", bytes.NewBufferString(`{
		"resumed_by":"operator",
		"reason":"atendimento finalizado",
		"metadata":{"source":"dashboard"}
	}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var out ResumeSessionResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if out.Session.HandoffStatus != "BOT" {
		t.Fatalf("expected session handoff status BOT, got %s", out.Session.HandoffStatus)
	}
	if out.Session.CurrentOwnerUserID != "" {
		t.Fatalf("expected current owner to be cleared, got %s", out.Session.CurrentOwnerUserID)
	}
	if out.Handoff.Status != "RESOLVED" {
		t.Fatalf("expected handoff status RESOLVED, got %s", out.Handoff.Status)
	}
	if out.Handoff.AssignedUserID != ownerID {
		t.Fatalf("expected assigned user %s, got %s", ownerID, out.Handoff.AssignedUserID)
	}
	if out.Handoff.ResolvedAt == nil {
		t.Fatalf("expected resolved_at to be set")
	}
	if got := asString(out.Handoff.Metadata["resumed_by"]); got != "OPERATOR" {
		t.Fatalf("expected resumed_by metadata OPERATOR, got %s", got)
	}
}

func TestResumeSessionRejectsWhenNoActiveHandoff(t *testing.T) {
	store := newFakeStore()
	session, _ := store.seedSessionWithMessage("5511888888888", "ola")
	handler := NewHandler(NewService(store, config.Config{ChatDebounceWindowMS: 1500}))

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+session.ID+"/resume", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, rec.Code)
	}
}

func TestIngestMessageAggregatesWithinDebounceWindow(t *testing.T) {
	store := newFakeStore()
	handler := NewHandler(NewService(store, config.Config{ChatDebounceWindowMS: 2000}))

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	firstReq := httptest.NewRequest(http.MethodPost, "/chat/messages/ingest", bytes.NewBufferString(`{
		"contact_key":"5511999999999",
		"message":{"direction":"INBOUND","provider_message_id":"msg-buffer-1","idempotency_key":"idem-buffer-1","body":"primeira"}
	}`))
	firstRec := httptest.NewRecorder()
	r.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusCreated {
		t.Fatalf("expected first call to create message, got %d", firstRec.Code)
	}

	time.Sleep(10 * time.Millisecond)

	secondReq := httptest.NewRequest(http.MethodPost, "/chat/messages/ingest", bytes.NewBufferString(`{
		"contact_key":"5511999999999",
		"message":{"direction":"INBOUND","provider_message_id":"msg-buffer-2","idempotency_key":"idem-buffer-2","body":"segunda"}
	}`))
	secondRec := httptest.NewRecorder()
	r.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusCreated {
		t.Fatalf("expected second call to create message, got %d", secondRec.Code)
	}

	var out IngestMessageResult
	if err := json.Unmarshal(secondRec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	buffer, ok := out.Session.Metadata["buffer"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected buffer metadata to be present")
	}
	if got := asInt(buffer["message_count"]); got != 2 {
		t.Fatalf("expected two buffered messages, got %d", got)
	}
	if asString(buffer["last_message_body"]) != "segunda" {
		t.Fatalf("expected latest buffered body to be tracked")
	}
}

func TestOutboundMessageClearsPendingBuffer(t *testing.T) {
	store := newFakeStore()
	handler := NewHandler(NewService(store, config.Config{ChatDebounceWindowMS: 1500}))

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	inboundReq := httptest.NewRequest(http.MethodPost, "/chat/messages/ingest", bytes.NewBufferString(`{
		"contact_key":"5511999999999",
		"message":{"direction":"INBOUND","provider_message_id":"msg-inbound","idempotency_key":"idem-inbound","body":"oi"}
	}`))
	inboundRec := httptest.NewRecorder()
	r.ServeHTTP(inboundRec, inboundReq)
	if inboundRec.Code != http.StatusCreated {
		t.Fatalf("expected inbound call to create message, got %d", inboundRec.Code)
	}

	outboundReq := httptest.NewRequest(http.MethodPost, "/chat/messages/ingest", bytes.NewBufferString(`{
		"contact_key":"5511999999999",
		"message":{"direction":"OUTBOUND","provider_message_id":"msg-outbound","idempotency_key":"idem-outbound","body":"resposta"}
	}`))
	outboundRec := httptest.NewRecorder()
	r.ServeHTTP(outboundRec, outboundReq)
	if outboundRec.Code != http.StatusCreated {
		t.Fatalf("expected outbound call to create message, got %d", outboundRec.Code)
	}

	var out IngestMessageResult
	if err := json.Unmarshal(outboundRec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	buffer, ok := out.Session.Metadata["buffer"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected buffer metadata to be present")
	}
	if got := asString(buffer["status"]); got != bufferStatusIdle {
		t.Fatalf("expected buffer status %s, got %s", bufferStatusIdle, got)
	}
	if got := asInt(buffer["message_count"]); got != 0 {
		t.Fatalf("expected cleared buffer count, got %d", got)
	}
}

func TestPresenceTypingExtendsPendingBuffer(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 2000})

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-presence-1",
			IdempotencyKey:    "idem-presence-1",
			Body:              "oi",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}

	before := parseBufferTime(ingested.Session.Metadata["buffer"].(map[string]interface{})["pending_until"])
	if before == nil {
		t.Fatalf("expected initial pending_until")
	}

	observedAt := before.Add(-500 * time.Millisecond)
	result, err := svc.ApplyPresenceSignal(context.Background(), ApplyPresenceSignalInput{
		ContactKey:     "5511999999999",
		PresenceStatus: "typing",
		ObservedAt:     &observedAt,
	})
	if err != nil {
		t.Fatalf("apply presence: %v", err)
	}

	after := parseBufferTime(result.Session.Metadata["buffer"].(map[string]interface{})["pending_until"])
	if after == nil || !after.After(*before) {
		t.Fatalf("expected pending_until to be extended")
	}
	if result.Status != "accepted" {
		t.Fatalf("expected accepted status, got %s", result.Status)
	}
}

func TestPresencePausedShortensPendingBuffer(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 2000})

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-presence-2",
			IdempotencyKey:    "idem-presence-2",
			Body:              "oi",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}

	before := parseBufferTime(ingested.Session.Metadata["buffer"].(map[string]interface{})["pending_until"])
	if before == nil {
		t.Fatalf("expected initial pending_until")
	}

	observedAt := ingested.Message.ReceivedAt.Add(500 * time.Millisecond)
	result, err := svc.ApplyPresenceSignal(context.Background(), ApplyPresenceSignalInput{
		ContactKey:     "5511999999999",
		PresenceStatus: "paused",
		ObservedAt:     &observedAt,
	})
	if err != nil {
		t.Fatalf("apply presence: %v", err)
	}

	after := parseBufferTime(result.Session.Metadata["buffer"].(map[string]interface{})["pending_until"])
	if after == nil || !after.Before(*before) {
		t.Fatalf("expected pending_until to be shortened")
	}
}

func TestPresenceWithoutPendingBufferIsSkipped(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500})

	_, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "OUTBOUND",
			ProviderMessageID: "msg-presence-3",
			IdempotencyKey:    "idem-presence-3",
			Body:              "resposta",
		},
	})
	if err != nil {
		t.Fatalf("ingest outbound message: %v", err)
	}

	result, err := svc.ApplyPresenceSignal(context.Background(), ApplyPresenceSignalInput{
		ContactKey:     "5511999999999",
		PresenceStatus: "typing",
	})
	if err != nil {
		t.Fatalf("apply presence: %v", err)
	}
	if result.Status != "skipped" {
		t.Fatalf("expected skipped status, got %s", result.Status)
	}
	if result.Reason != "no_pending_buffer" {
		t.Fatalf("expected no_pending_buffer, got %s", result.Reason)
	}
}

func TestReprocessBuildsMemoryAndMarksMessagesAutomationPending(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500})

	first, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-reprocess-1",
			IdempotencyKey:    "idem-reprocess-1",
			Body:              "quero saber o valor para Santa Catarina",
		},
	})
	if err != nil {
		t.Fatalf("ingest first message: %v", err)
	}
	_, err = svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-reprocess-2",
			IdempotencyKey:    "idem-reprocess-2",
			Body:              "saindo de Sao Luis",
		},
	})
	if err != nil {
		t.Fatalf("ingest second message: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+first.Session.ID+"/reprocess", bytes.NewBufferString(`{
		"trigger":"dashboard",
		"metadata":{"source":"ops"}
	}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var out ReprocessResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if out.Status != "accepted" {
		t.Fatalf("expected accepted status, got %s", out.Status)
	}
	if len(out.Messages) != 2 {
		t.Fatalf("expected two reprocessed messages, got %d", len(out.Messages))
	}
	for _, item := range out.Messages {
		if item.ProcessingStatus != messageStatusAutomationPending {
			t.Fatalf("expected processing status %s, got %s", messageStatusAutomationPending, item.ProcessingStatus)
		}
	}
	currentTurnBody := asString(out.Memory["current_turn_body"])
	if currentTurnBody == "" || !strings.Contains(currentTurnBody, "Santa Catarina") || !strings.Contains(currentTurnBody, "Sao Luis") {
		t.Fatalf("expected current_turn_body to include buffered messages, got %q", currentTurnBody)
	}
	buffer, ok := out.Session.Metadata["buffer"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected buffer metadata to be present")
	}
	if got := asString(buffer["status"]); got != bufferStatusIdle {
		t.Fatalf("expected buffer status %s, got %s", bufferStatusIdle, got)
	}
	agent, ok := out.Session.Metadata["agent"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected agent metadata to be present")
	}
	if got := asString(agent["status"]); got != agentStatusReadyForAutomation {
		t.Fatalf("expected agent status %s, got %s", agentStatusReadyForAutomation, got)
	}
	if got := asString(agent["trigger"]); got != "DASHBOARD" {
		t.Fatalf("expected agent trigger DASHBOARD, got %s", got)
	}
}

func TestReprocessRejectsWhenHumanOwnerIsActive(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500})

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-reprocess-human-1",
			IdempotencyKey:    "idem-reprocess-human-1",
			Body:              "oi",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}

	item := store.sessions[ingested.Session.ID]
	item.HandoffStatus = "HUMAN"
	item.CurrentOwnerUserID = uuid.NewString()
	store.sessions[item.ID] = item

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+ingested.Session.ID+"/reprocess", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, rec.Code)
	}
}

func TestReprocessGeneratesAgentDraftWhenRunnerIsEnabled(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Temos saidas para Santa Catarina. Qual cidade de destino voce quer consultar?",
			Model:              "gpt-test",
			ProviderResponseID: "resp_123",
			RequestPayload:     map[string]interface{}{"model": "gpt-test"},
			ResponsePayload:    map[string]interface{}{"id": "resp_123"},
		},
	})

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-agent-1",
			IdempotencyKey:    "idem-agent-1",
			Body:              "quero ir para Santa Catarina",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+ingested.Session.ID+"/reprocess", bytes.NewBufferString(`{"trigger":"dashboard"}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var out ReprocessResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if out.Draft == nil {
		t.Fatalf("expected draft to be present")
	}
	if out.Draft.ProcessingStatus != messageStatusAutomationDraft {
		t.Fatalf("expected draft status %s, got %s", messageStatusAutomationDraft, out.Draft.ProcessingStatus)
	}
	agent, ok := out.Session.Metadata["agent"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected agent metadata")
	}
	if got := asString(agent["status"]); got != agentStatusDraftGenerated {
		t.Fatalf("expected agent status %s, got %s", agentStatusDraftGenerated, got)
	}
}

func TestReprocessStructuredInterpreterShadowDisabledDoesNotCallOpenAI(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Temos saidas para Santa Catarina. Qual cidade de destino voce quer consultar?",
			Model:              "gpt-test",
			ProviderResponseID: "resp_shadow_disabled",
		},
	}
	openAI := &fakeOpenAIInterpreter{
		enabled: true,
		result: OpenAIStructuredInterpreterRunResult{
			Interpretation: StructuredInterpretation{
				Intent:      StructuredIntentBookingCancelRequest,
				TurnMeaning: TurnMeaningNewRequest,
				Confidence:  0.99,
				Source:      "openai_structured",
			},
			ProviderResponseID: "resp_shadow_should_not_run",
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, openAI)

	out := ingestAndReprocessShadowDraft(t, svc, "5511900000101", "oi")

	if openAI.calls != 0 {
		t.Fatalf("expected OpenAI shadow not to be called when disabled, got %d calls", openAI.calls)
	}
	if out.Draft == nil {
		t.Fatalf("expected draft to be generated")
	}
	if got := strings.TrimSpace(out.Draft.Body); got != runner.result.ReplyText {
		t.Fatalf("expected real draft body to remain %q, got %q", runner.result.ReplyText, got)
	}
	if len(out.ToolCalls) != 0 {
		t.Fatalf("expected no tool calls, got %+v", out.ToolCalls)
	}
	shadow := mustStructuredInterpreterShadowMap(t, out.Memory[structuredInterpreterShadowKey])
	openAIShadow := mustNestedMap(t, shadow, "openai")
	if got := asString(openAIShadow["status"]); got != string(StructuredInterpreterShadowDisabled) {
		t.Fatalf("expected shadow status %q, got %q", StructuredInterpreterShadowDisabled, got)
	}
}

func TestReprocessStructuredInterpreterShadowEnabledRunnerDisabled(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Vou verificar as opcoes para voce.",
			Model:              "gpt-test",
			ProviderResponseID: "resp_shadow_runner_disabled",
		},
	}
	openAI := &fakeOpenAIInterpreter{enabled: false}
	svc := NewService(store, config.Config{
		ChatDebounceWindowMS:               1500,
		ChatOpenAIInterpreterShadowEnabled: true,
	}, runner, openAI)

	out := ingestAndReprocessShadowDraft(t, svc, "5511900000102", "quero viajar")

	if openAI.calls != 0 {
		t.Fatalf("expected disabled OpenAI runner not to be called, got %d calls", openAI.calls)
	}
	if out.Draft == nil {
		t.Fatalf("expected draft to be generated")
	}
	shadow := mustStructuredInterpreterShadowMap(t, out.Memory[structuredInterpreterShadowKey])
	openAIShadow := mustNestedMap(t, shadow, "openai")
	if got := asString(openAIShadow["status"]); got != string(StructuredInterpreterShadowOpenAIDisabled) {
		t.Fatalf("expected shadow status %q, got %q", StructuredInterpreterShadowOpenAIDisabled, got)
	}
}

func TestReprocessStructuredInterpreterShadowEnabledValidDoesNotChangeDraftOrTools(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Temos saidas para Santa Catarina. Qual cidade de destino voce quer consultar?",
			Model:              "gpt-test",
			ProviderResponseID: "resp_shadow_valid",
		},
	}
	openAI := &fakeOpenAIInterpreter{
		enabled: true,
		result: OpenAIStructuredInterpreterRunResult{
			Interpretation: StructuredInterpretation{
				Intent:      StructuredIntentGreeting,
				TurnMeaning: TurnMeaningGreeting,
				Confidence:  0.88,
				Source:      "openai_structured",
			},
			ProviderResponseID: "resp_shadow_valid_openai",
		},
	}
	svc := NewService(store, config.Config{
		ChatDebounceWindowMS:               1500,
		ChatOpenAIInterpreterShadowEnabled: true,
	}, runner, openAI)

	out := ingestAndReprocessShadowDraft(t, svc, "5511900000103", "oi")

	if openAI.calls != 1 {
		t.Fatalf("expected OpenAI shadow to be called once, got %d", openAI.calls)
	}
	if strings.TrimSpace(openAI.lastInput.IdempotencyKey) == "" {
		t.Fatalf("expected shadow idempotency key")
	}
	if out.Draft == nil {
		t.Fatalf("expected draft to be generated")
	}
	if got := strings.TrimSpace(out.Draft.Body); got != runner.result.ReplyText {
		t.Fatalf("expected real draft body to remain %q, got %q", runner.result.ReplyText, got)
	}
	if len(out.ToolCalls) != 0 {
		t.Fatalf("expected no tool calls, got %+v", out.ToolCalls)
	}
	shadow := mustStructuredInterpreterShadowMap(t, out.Memory[structuredInterpreterShadowKey])
	openAIShadow := mustNestedMap(t, shadow, "openai")
	if got := asString(openAIShadow["status"]); got != string(StructuredInterpreterShadowValid) {
		t.Fatalf("expected shadow status %q, got %q", StructuredInterpreterShadowValid, got)
	}
	if got := asString(openAIShadow["provider_response_id"]); got != "resp_shadow_valid_openai" {
		t.Fatalf("expected provider response id, got %q", got)
	}
	agreement := mustNestedMap(t, shadow, "agreement")
	if got, ok := agreement["intent"].(bool); !ok || !got {
		t.Fatalf("expected intent agreement, got %#v", agreement["intent"])
	}
	if got, ok := agreement["turn_meaning"].(bool); !ok || !got {
		t.Fatalf("expected turn meaning agreement, got %#v", agreement["turn_meaning"])
	}
	validation := mustNestedMap(t, shadow, "openai_validation")
	if got := asString(validation["status"]); got != string(StructuredInterpreterShadowValidationAccepted) {
		t.Fatalf("expected OpenAI validation status %q, got %q", StructuredInterpreterShadowValidationAccepted, got)
	}
	if got, ok := validation["accepted"].(bool); !ok || !got {
		t.Fatalf("expected OpenAI validation accepted, got %#v", validation["accepted"])
	}
	draftShadow := mustStructuredInterpreterShadowMap(t, out.Draft.NormalizedPayload[structuredInterpreterShadowKey])
	draftOpenAIShadow := mustNestedMap(t, draftShadow, "openai")
	if got := asString(draftOpenAIShadow["status"]); got != string(StructuredInterpreterShadowValid) {
		t.Fatalf("expected draft shadow status %q, got %q", StructuredInterpreterShadowValid, got)
	}
	draftValidation := mustNestedMap(t, draftShadow, "openai_validation")
	if got := asString(draftValidation["status"]); got != string(StructuredInterpreterShadowValidationAccepted) {
		t.Fatalf("expected draft validation status %q, got %q", StructuredInterpreterShadowValidationAccepted, got)
	}
}

func TestReprocessStructuredInterpreterShadowValidationDoesNotChangeDraftToolsOrCanonicalState(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Temos saidas para Santa Catarina. Qual cidade de destino voce quer consultar?",
			Model:              "gpt-test",
			ProviderResponseID: "resp_shadow_validation_runtime",
		},
	}
	proposal := validationProposal(StructuredIntentPaymentPreference, TurnMeaningAnswerToQuestion)
	proposal.Payment.PaymentPreference = "sinal"
	openAI := &fakeOpenAIInterpreter{
		enabled: true,
		result: OpenAIStructuredInterpreterRunResult{
			Interpretation:     proposal,
			ProviderResponseID: "resp_shadow_validation_runtime_openai",
		},
	}
	svc := NewService(store, config.Config{
		ChatDebounceWindowMS:               1500,
		ChatOpenAIInterpreterShadowEnabled: true,
	}, runner, openAI)

	out := ingestAndReprocessShadowDraft(t, svc, "5511900000106", "oi")

	if openAI.calls != 1 {
		t.Fatalf("expected OpenAI shadow to be called once, got %d", openAI.calls)
	}
	if out.Draft == nil {
		t.Fatalf("expected draft to be generated")
	}
	if got := strings.TrimSpace(out.Draft.Body); got != runner.result.ReplyText {
		t.Fatalf("expected real draft body to remain %q, got %q", runner.result.ReplyText, got)
	}
	if len(out.ToolCalls) != 0 {
		t.Fatalf("validation-only OpenAI proposal must not create tool calls, got %+v", out.ToolCalls)
	}

	agent := asMap(out.Session.Metadata["agent"])
	state, ok := agent["canonical_state"].(CanonicalConversationState)
	if !ok {
		t.Fatalf("expected canonical_state in agent metadata, got %#v", agent["canonical_state"])
	}
	if state.Phase != ConversationPhaseDiscovery {
		t.Fatalf("expected real canonical phase to remain %s, got %+v", ConversationPhaseDiscovery, state)
	}
	if strings.TrimSpace(state.Payment.Preference) != "" {
		t.Fatalf("OpenAI validation must not set payment preference in canonical state, got %+v", state.Payment)
	}

	shadow := mustStructuredInterpreterShadowMap(t, out.Memory[structuredInterpreterShadowKey])
	validation := mustNestedMap(t, shadow, "openai_validation")
	if got := asString(validation["status"]); got != string(StructuredInterpreterShadowValidationRejected) {
		t.Fatalf("expected OpenAI validation status %q, got %q", StructuredInterpreterShadowValidationRejected, got)
	}
	if got, ok := validation["accepted"].(bool); !ok || got {
		t.Fatalf("expected OpenAI validation rejected, got %#v", validation["accepted"])
	}
	if got := asString(validation["reject_reason"]); got != "active_prompt_required" {
		t.Fatalf("expected reject reason active_prompt_required, got %q", got)
	}
	if got := asString(validation["fallback_template"]); got != string(TemplateContextFallbackPaymentPreference) {
		t.Fatalf("expected payment preference fallback, got %q", got)
	}

	draftShadow := mustStructuredInterpreterShadowMap(t, out.Draft.NormalizedPayload[structuredInterpreterShadowKey])
	draftValidation := mustNestedMap(t, draftShadow, "openai_validation")
	if got := asString(draftValidation["status"]); got != string(StructuredInterpreterShadowValidationRejected) {
		t.Fatalf("expected draft validation status %q, got %q", StructuredInterpreterShadowValidationRejected, got)
	}
}

func TestReprocessStructuredInterpreterShadowEnabledErrorContinues(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Vou verificar as opcoes para voce.",
			Model:              "gpt-test",
			ProviderResponseID: "resp_shadow_error",
		},
	}
	openAI := &fakeOpenAIInterpreter{
		enabled: true,
		err:     errors.New("provider failed with CPF 52998224725 and raw_output"),
	}
	svc := NewService(store, config.Config{
		ChatDebounceWindowMS:               1500,
		ChatOpenAIInterpreterShadowEnabled: true,
	}, runner, openAI)

	out := ingestAndReprocessShadowDraft(t, svc, "5511900000104", "quero viajar")

	if openAI.calls != 1 {
		t.Fatalf("expected OpenAI shadow to be called once, got %d", openAI.calls)
	}
	if out.Draft == nil {
		t.Fatalf("expected draft to be generated")
	}
	if got := strings.TrimSpace(out.Draft.Body); got != runner.result.ReplyText {
		t.Fatalf("expected real draft body to remain %q, got %q", runner.result.ReplyText, got)
	}
	shadow := mustStructuredInterpreterShadowMap(t, out.Memory[structuredInterpreterShadowKey])
	openAIShadow := mustNestedMap(t, shadow, "openai")
	if got := asString(openAIShadow["status"]); got != string(StructuredInterpreterShadowError) {
		t.Fatalf("expected shadow status %q, got %q", StructuredInterpreterShadowError, got)
	}
	if got := asString(openAIShadow["error_code"]); got != "openai_structured_interpreter_error" {
		t.Fatalf("expected sanitized error code, got %q", got)
	}
	raw, err := json.Marshal(shadow)
	if err != nil {
		t.Fatalf("marshal shadow: %v", err)
	}
	if strings.Contains(string(raw), "52998224725") || strings.Contains(string(raw), "raw_output") {
		t.Fatalf("shadow error leaked sensitive/raw data: %s", string(raw))
	}
}

func TestReprocessStructuredInterpreterShadowDoesNotPersistSensitiveData(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Certo, vou conferir.",
			Model:              "gpt-test",
			ProviderResponseID: "resp_shadow_sensitive",
		},
	}
	openAI := &fakeOpenAIInterpreter{
		enabled: true,
		result: OpenAIStructuredInterpreterRunResult{
			Interpretation: StructuredInterpretation{
				Intent:      StructuredIntentUnknown,
				TurnMeaning: TurnMeaningUnknown,
				Confidence:  0.4,
				Source:      "openai_structured",
			},
			ProviderResponseID: "resp_shadow_sensitive_openai",
			RawOutput:          `{"cpf":"52998224725","raw_output":"forbidden"}`,
		},
	}
	svc := NewService(store, config.Config{
		ChatDebounceWindowMS:               1500,
		ChatOpenAIInterpreterShadowEnabled: true,
	}, runner, openAI)

	out := ingestAndReprocessShadowDraft(t, svc, "5511900000105", "Meu CPF e 529.982.247-25, RG 123456 e data:image/png;base64,AAAA")

	shadow := mustStructuredInterpreterShadowMap(t, out.Memory[structuredInterpreterShadowKey])
	raw, err := json.Marshal(shadow)
	if err != nil {
		t.Fatalf("marshal shadow: %v", err)
	}
	text := string(raw)
	for _, forbidden := range []string{
		"52998224725",
		"529.982.247-25",
		"123456",
		"data:image",
		"raw_output",
		"RawOutput",
		"prompt",
		"compact",
		"request_payload",
		"response_payload",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("shadow summary leaked forbidden value %q in %s", forbidden, text)
		}
	}
}

func TestReprocessAutoSendsEligibleDraftWhenSenderIsEnabled(t *testing.T) {
	store := newFakeStore()
	sender := &fakeReplySender{
		enabled: true,
		result: SendReplyResult{
			ProviderMessageID: "MSG-AUTO-1",
			ProviderStatus:    "SENT",
			Payload:           map[string]interface{}{"provider": "EVOLUTION"},
			SentAt:            time.Now().UTC(),
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, sender, &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Posso te ajudar com a proxima etapa da reserva.",
			Model:              "gpt-test",
			ProviderResponseID: "resp_auto_send_1",
		},
	})

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999@s.whatsapp.net",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-auto-send-1",
			IdempotencyKey:    "idem-auto-send-1",
			Body:              "oi",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+ingested.Session.ID+"/reprocess", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var out ReprocessResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if out.Reason != "draft_auto_sent" {
		t.Fatalf("expected reason draft_auto_sent, got %s", out.Reason)
	}
	if out.Draft == nil {
		t.Fatalf("expected draft to be present")
	}
	if out.Draft.ProcessingStatus != messageStatusAutomationSent {
		t.Fatalf("expected draft status %s, got %s", messageStatusAutomationSent, out.Draft.ProcessingStatus)
	}
	agent, ok := out.Session.Metadata["agent"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected agent metadata")
	}
	if got := asString(agent["status"]); got != agentStatusDraftAutoSent {
		t.Fatalf("expected agent status %s, got %s", agentStatusDraftAutoSent, got)
	}
	if sender.calls != 1 {
		t.Fatalf("expected one sender call, got %d", sender.calls)
	}
	if len(store.outbounds) != 1 {
		t.Fatalf("expected one outbound record, got %d", len(store.outbounds))
	}
	for _, outbound := range store.outbounds {
		if outbound.Status != "SENT" {
			t.Fatalf("expected outbound status SENT, got %s", outbound.Status)
		}
		if asString(outbound.Payload["mode"]) != "BOT_AUTO_REPLY" {
			t.Fatalf("expected BOT_AUTO_REPLY mode, got %s", asString(outbound.Payload["mode"]))
		}
	}
}

func TestReprocessAutoSendsPricingQuoteDraftWhenSenderIsEnabled(t *testing.T) {
	store := newFakeStore()
	sender := &fakeReplySender{
		enabled: true,
		result: SendReplyResult{
			ProviderMessageID: "MSG-AUTO-SKIP-1",
			ProviderStatus:    "SENT",
			Payload:           map[string]interface{}{"provider": "EVOLUTION"},
			SentAt:            time.Now().UTC(),
		},
	}
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Encontrei o valor confirmado para esse trecho.",
			Model:              "gpt-test",
			ProviderResponseID: "resp_tool_auto_skip_1",
		},
	}
	searcher := &fakeAvailabilitySearcher{
		enabled: true,
		result: AvailabilitySearchResult{
			Results: []AvailabilitySearchItem{{
				SegmentID:              "seg-1",
				TripID:                 "trip-1",
				RouteID:                "route-1",
				OriginDisplayName:      "Videira/SC",
				DestinationDisplayName: "Sao Luis/MA",
				OriginDepartTime:       "18:30",
				TripDate:               "2026-05-10",
				SeatsAvailable:         12,
				Price:                  250,
				Currency:               "BRL",
				Status:                 "ACTIVE",
				TripStatus:             "SCHEDULED",
			}},
		},
	}
	pricingSearcher := &fakePricingQuoteSearcher{
		enabled: true,
		result: PricingQuoteResult{
			Filter: PricingQuoteInput{FareMode: "AUTO"},
			Results: []PricingQuoteItem{{
				TripID:                 "trip-1",
				RouteID:                "route-1",
				BoardStopID:            "board-1",
				AlightStopID:           "alight-1",
				OriginStopID:           "origin-1",
				DestinationStopID:      "destination-1",
				OriginDisplayName:      "Videira/SC",
				DestinationDisplayName: "Sao Luis/MA",
				OriginDepartTime:       "18:30",
				TripDate:               "2026-05-10",
				BaseAmount:             250,
				CalcAmount:             250,
				FinalAmount:            250,
				Currency:               "BRL",
				FareMode:               "AUTO",
			}},
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, sender, runner, searcher, pricingSearcher)

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-auto-send-skip-1",
			IdempotencyKey:    "idem-auto-send-skip-1",
			Body:              "qual o valor de Videira/SC para Sao Luis/MA em 10/05?",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+ingested.Session.ID+"/reprocess", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var out ReprocessResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if out.Draft == nil {
		t.Fatalf("expected draft to be present")
	}
	if out.Reason != "draft_auto_sent" {
		t.Fatalf("expected reason draft_auto_sent, got %s", out.Reason)
	}
	if out.Draft.ProcessingStatus != messageStatusAutomationSent {
		t.Fatalf("expected draft status %s, got %s", messageStatusAutomationSent, out.Draft.ProcessingStatus)
	}
	if got := readDraftAutoSendStatus(*out.Draft); got != draftAutoSendStatusEligible {
		t.Fatalf("expected draft auto_send_status %s, got %s", draftAutoSendStatusEligible, got)
	}
	if reasons := readDraftAutoSendReasons(*out.Draft); len(reasons) != 0 {
		t.Fatalf("expected no draft auto_send_reasons, got %+v", reasons)
	}
	if sender.calls != 1 {
		t.Fatalf("expected one sender call, got %d", sender.calls)
	}
	if len(store.outbounds) != 1 {
		t.Fatalf("expected one outbound record, got %d", len(store.outbounds))
	}
}

func TestReprocessAutoSendsAvailabilitySearchDraftWhenSenderIsEnabled(t *testing.T) {
	store := newFakeStore()
	sender := &fakeReplySender{
		enabled: true,
		result: SendReplyResult{
			ProviderMessageID: "MSG-AUTO-AVAIL-1",
			ProviderStatus:    "SENT",
			Payload:           map[string]interface{}{"provider": "EVOLUTION"},
			SentAt:            time.Now().UTC(),
		},
	}
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Tenho datas disponiveis para Concordia.",
			Model:              "gpt-test",
			ProviderResponseID: "resp_auto_availability_1",
		},
	}
	searcher := &fakeAvailabilitySearcher{
		enabled: true,
		result: AvailabilitySearchResult{
			Results: []AvailabilitySearchItem{{
				SegmentID:              "seg-sc-auto-1",
				TripID:                 "trip-sc-auto-1",
				RouteID:                "route-sc-auto-1",
				OriginDisplayName:      "Santa Ines/MA",
				DestinationDisplayName: "Concordia/SC",
				OriginDepartTime:       "18:30",
				TripDate:               "2026-05-10",
				SeatsAvailable:         8,
				Price:                  1100,
				Currency:               "BRL",
				Status:                 "ACTIVE",
				TripStatus:             "SCHEDULED",
				PackageName:            packageToSantaCatarina,
			}},
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, sender, runner, searcher)

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999@s.whatsapp.net",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-auto-send-availability-1",
			IdempotencyKey:    "idem-auto-send-availability-1",
			Body:              "quero para concordia",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}

	store.sessions[ingested.Session.ID] = Session{
		ID:            ingested.Session.ID,
		Channel:       ingested.Session.Channel,
		ContactKey:    ingested.Session.ContactKey,
		CustomerPhone: ingested.Session.CustomerPhone,
		CustomerName:  ingested.Session.CustomerName,
		Status:        ingested.Session.Status,
		HandoffStatus: ingested.Session.HandoffStatus,
		Metadata: map[string]interface{}{
			"memory": map[string]interface{}{
				"recent_messages": []map[string]interface{}{
					{"direction": "INBOUND", "body": "quero passagem para sc"},
					{"direction": "OUTBOUND", "body": "Fraiburgo R$ 950; Concordia R$ 1100."},
				},
			},
		},
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+ingested.Session.ID+"/reprocess", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var out ReprocessResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if out.Reason != "draft_auto_sent" {
		t.Fatalf("expected reason draft_auto_sent, got %s", out.Reason)
	}
	if out.Draft == nil {
		t.Fatalf("expected draft to be present")
	}
	if out.Draft.ProcessingStatus != messageStatusAutomationSent {
		t.Fatalf("expected draft status %s, got %s", messageStatusAutomationSent, out.Draft.ProcessingStatus)
	}
	if got := readDraftAutoSendStatus(*out.Draft); got != draftAutoSendStatusEligible {
		t.Fatalf("expected draft auto_send_status %s, got %s", draftAutoSendStatusEligible, got)
	}
	if sender.calls != 1 {
		t.Fatalf("expected one sender call, got %d", sender.calls)
	}
	if len(store.outbounds) != 1 {
		t.Fatalf("expected one outbound record, got %d", len(store.outbounds))
	}
}

func TestReprocessRetriesAutoSendOnSameDraftAfterFailure(t *testing.T) {
	store := newFakeStore()
	sender := &fakeReplySender{
		enabled: true,
		errs:    []error{errors.New("gateway timeout"), nil},
		results: []SendReplyResult{
			{},
			{
				ProviderMessageID: "MSG-AUTO-RETRY-1",
				ProviderStatus:    "SENT",
				Payload:           map[string]interface{}{"provider": "EVOLUTION"},
				SentAt:            time.Now().UTC(),
			},
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, sender, &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Posso seguir com seu atendimento.",
			Model:              "gpt-test",
			ProviderResponseID: "resp_auto_retry_1",
		},
	})

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999@s.whatsapp.net",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-auto-retry-1",
			IdempotencyKey:    "idem-auto-retry-1",
			Body:              "oi",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+ingested.Session.ID+"/reprocess", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected first status %d, got %d", http.StatusBadGateway, rec.Code)
	}
	var failedDraft *Message
	for _, message := range store.messages {
		if message.SessionID == ingested.Session.ID && message.Direction == "OUTBOUND" && message.ProcessingStatus == messageStatusAutomationDraft {
			item := message
			failedDraft = &item
			break
		}
	}
	if failedDraft == nil {
		t.Fatalf("expected failed draft to remain available")
	}
	if got := readDraftAutoSendStatus(*failedDraft); got != draftAutoSendStatusRetryPending {
		t.Fatalf("expected failed draft auto_send_status %s, got %s", draftAutoSendStatusRetryPending, got)
	}
	if reasons := readDraftAutoSendReasons(*failedDraft); len(reasons) != 1 || reasons[0] != draftAutoSendReasonDeliveryFail {
		t.Fatalf("expected retry reason %s, got %#v", draftAutoSendReasonDeliveryFail, reasons)
	}

	req = httptest.NewRequest(http.MethodPost, "/chat/sessions/"+ingested.Session.ID+"/reprocess", bytes.NewBufferString(`{}`))
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected second status %d, got %d", http.StatusOK, rec.Code)
	}

	var out ReprocessResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !out.Idempotent {
		t.Fatalf("expected idempotent response on retry")
	}
	if out.Draft == nil || out.Draft.ProcessingStatus != messageStatusAutomationSent {
		t.Fatalf("expected auto-sent draft on retry, got %+v", out.Draft)
	}
	if sender.calls != 2 {
		t.Fatalf("expected two sender calls, got %d", sender.calls)
	}
	if len(store.outbounds) != 1 {
		t.Fatalf("expected one outbound record, got %d", len(store.outbounds))
	}
}

func TestMaybeAutoSendDraftBlocksWhenSessionMovesToHuman(t *testing.T) {
	store := newFakeStore()
	sender := &fakeReplySender{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, sender)

	session := Session{
		ID:                 uuid.NewString(),
		Channel:            "WHATSAPP",
		ContactKey:         "5511999999999@s.whatsapp.net",
		Status:             "ACTIVE",
		HandoffStatus:      "HUMAN",
		CurrentOwnerUserID: uuid.NewString(),
		Metadata: map[string]interface{}{
			"agent": map[string]interface{}{
				"status":             agentStatusDraftGenerated,
				"draft_generated_at": time.Now().UTC().Format(time.RFC3339Nano),
				"auto_send_status":   draftAutoSendStatusEligible,
			},
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	store.sessions[session.ID] = session
	store.sessionsByKey[session.Channel+"|"+session.ContactKey] = session.ID

	draft := Message{
		ID:             uuid.NewString(),
		SessionID:      session.ID,
		Direction:      "OUTBOUND",
		Kind:           "TEXT",
		IdempotencyKey: "chat-agent-draft-blocked",
		Body:           "Posso seguir com seu atendimento.",
		Payload: map[string]interface{}{
			"mode":             "AUTOMATION_DRAFT",
			"auto_send_status": draftAutoSendStatusEligible,
		},
		NormalizedPayload: map[string]interface{}{
			"mode":             "AUTOMATION_DRAFT",
			"auto_send_status": draftAutoSendStatusEligible,
		},
		ProcessingStatus: messageStatusAutomationDraft,
		ReceivedAt:       time.Now().UTC(),
		CreatedAt:        time.Now().UTC(),
	}
	store.messages[draft.ID] = draft
	store.messageOrder = append(store.messageOrder, draft.ID)
	store.byIdempotencyKey[draft.IdempotencyKey] = draft.ID

	out, err := svc.maybeAutoSendDraft(context.Background(), ReprocessResult{
		Session: session,
		Draft:   &draft,
		Status:  "accepted",
		Reason:  "draft_already_generated",
	})
	if err != nil {
		t.Fatalf("maybe auto-send draft: %v", err)
	}
	if out.Reason != "draft_auto_send_blocked_human" {
		t.Fatalf("expected reason draft_auto_send_blocked_human, got %s", out.Reason)
	}
	if out.Draft == nil {
		t.Fatalf("expected updated draft")
	}
	if got := readDraftAutoSendStatus(*out.Draft); got != draftAutoSendStatusBlockedHuman {
		t.Fatalf("expected blocked draft status %s, got %s", draftAutoSendStatusBlockedHuman, got)
	}
	if sender.calls != 0 {
		t.Fatalf("expected zero sender calls, got %d", sender.calls)
	}
	agent := asMap(out.Session.Metadata["agent"])
	if got := asString(agent["auto_send_status"]); got != draftAutoSendStatusBlockedHuman {
		t.Fatalf("expected session auto_send_status %s, got %s", draftAutoSendStatusBlockedHuman, got)
	}
}

func TestRetryDraftAutoSendRetriesFailedDraftWithoutRerun(t *testing.T) {
	store := newFakeStore()
	sender := &fakeReplySender{
		enabled: true,
		errs: []error{
			errors.New("gateway timeout"),
			nil,
		},
		results: []SendReplyResult{
			{},
			{
				ProviderMessageID: "msg-auto-retry-manual-1",
				ProviderStatus:    "SENT",
				Payload: map[string]interface{}{
					"provider": "evolution",
				},
				SentAt: time.Now().UTC(),
			},
		},
	}
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Posso seguir com seu atendimento.",
			Model:              "gpt-test",
			ProviderResponseID: "resp_auto_retry_manual_1",
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, sender, runner)

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999@s.whatsapp.net",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-auto-manual-1",
			IdempotencyKey:    "idem-auto-manual-1",
			Body:              "oi",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+ingested.Session.ID+"/reprocess", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected first status %d, got %d", http.StatusBadGateway, rec.Code)
	}
	if runner.calls != 1 {
		t.Fatalf("expected one runner call after failed auto-send, got %d", runner.calls)
	}

	req = httptest.NewRequest(http.MethodPost, "/chat/sessions/"+ingested.Session.ID+"/draft/retry-auto-send", bytes.NewBufferString(`{
		"requested_by":"ops-bot",
		"reason":"manual_retry_dashboard",
		"metadata":{"source":"dashboard"}
	}`))
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected retry status %d, got %d", http.StatusOK, rec.Code)
	}

	var out RetryDraftAutoSendResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if out.Status != "accepted" {
		t.Fatalf("expected status accepted, got %s", out.Status)
	}
	if out.Reason != "draft_auto_send_retried" {
		t.Fatalf("expected reason draft_auto_send_retried, got %s", out.Reason)
	}
	if out.Draft == nil || out.Draft.ProcessingStatus != messageStatusAutomationSent {
		t.Fatalf("expected sent draft after manual retry, got %+v", out.Draft)
	}
	if got := readDraftAutoSendStatus(*out.Draft); got != draftAutoSendStatusEligible {
		t.Fatalf("expected draft auto_send_status %s after successful retry, got %s", draftAutoSendStatusEligible, got)
	}
	if got := asString(out.Draft.Payload["auto_send_retry_requested_by"]); got != "ops-bot" {
		t.Fatalf("expected auto_send_retry_requested_by ops-bot, got %s", got)
	}
	if got := asString(out.Draft.Payload["auto_send_retry_request_reason"]); got != "manual_retry_dashboard" {
		t.Fatalf("expected retry request reason manual_retry_dashboard, got %s", got)
	}
	if got := asInt(out.Draft.Payload["auto_send_retry_request_count"]); got != 1 {
		t.Fatalf("expected retry request count 1, got %d", got)
	}
	if out.Message == nil || out.Outbound == nil {
		t.Fatalf("expected linked message and outbound in retry result")
	}
	if sender.calls != 2 {
		t.Fatalf("expected two sender calls after manual retry, got %d", sender.calls)
	}
	if runner.calls != 1 {
		t.Fatalf("expected manual retry to avoid rerunning the agent, got %d runner calls", runner.calls)
	}
	if len(store.outbounds) != 1 {
		t.Fatalf("expected one outbound record reused for retry, got %d", len(store.outbounds))
	}
}

func TestRetryDraftAutoSendBlocksWhenSessionMovesToHuman(t *testing.T) {
	store := newFakeStore()
	sender := &fakeReplySender{
		enabled: true,
		errs:    []error{errors.New("gateway timeout")},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, sender, &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Posso seguir com seu atendimento.",
			Model:              "gpt-test",
			ProviderResponseID: "resp_auto_retry_manual_block_1",
		},
	})

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999@s.whatsapp.net",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-auto-manual-block-1",
			IdempotencyKey:    "idem-auto-manual-block-1",
			Body:              "oi",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+ingested.Session.ID+"/reprocess", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected first status %d, got %d", http.StatusBadGateway, rec.Code)
	}

	session := store.sessions[ingested.Session.ID]
	session.HandoffStatus = "HUMAN"
	session.CurrentOwnerUserID = uuid.NewString()
	if session.Metadata == nil {
		session.Metadata = map[string]interface{}{}
	}
	store.sessions[session.ID] = session

	req = httptest.NewRequest(http.MethodPost, "/chat/sessions/"+ingested.Session.ID+"/draft/retry-auto-send", bytes.NewBufferString(`{"requested_by":"ops-human"}`))
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected blocked retry status %d, got %d", http.StatusOK, rec.Code)
	}

	var out RetryDraftAutoSendResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if out.Status != "blocked" {
		t.Fatalf("expected status blocked, got %s", out.Status)
	}
	if out.Reason != "draft_auto_send_blocked_human" {
		t.Fatalf("expected reason draft_auto_send_blocked_human, got %s", out.Reason)
	}
	if out.Draft == nil {
		t.Fatalf("expected blocked draft in response")
	}
	if got := readDraftAutoSendStatus(*out.Draft); got != draftAutoSendStatusBlockedHuman {
		t.Fatalf("expected blocked draft auto_send_status %s, got %s", draftAutoSendStatusBlockedHuman, got)
	}
	if sender.calls != 1 {
		t.Fatalf("expected no extra sender call after human block, got %d", sender.calls)
	}
}

func TestRetryDraftAutoSendRejectsDraftOutsideRetryQueue(t *testing.T) {
	store := newFakeStore()
	sender := &fakeReplySender{
		enabled: true,
		result: SendReplyResult{
			ProviderMessageID: "msg-auto-direct-1",
			ProviderStatus:    "SENT",
			SentAt:            time.Now().UTC(),
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, sender, &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Posso seguir com seu atendimento.",
			Model:              "gpt-test",
			ProviderResponseID: "resp_auto_direct_1",
		},
	})

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999@s.whatsapp.net",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-auto-direct-req-1",
			IdempotencyKey:    "idem-auto-direct-req-1",
			Body:              "oi",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+ingested.Session.ID+"/reprocess", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected reprocess status %d, got %d", http.StatusOK, rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/chat/sessions/"+ingested.Session.ID+"/draft/retry-auto-send", bytes.NewBufferString(`{}`))
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected conflict status %d, got %d", http.StatusConflict, rec.Code)
	}
}

func TestReprocessUsesAvailabilityToolWhenTurnHasStructuredRoute(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Encontrei uma opcao para esse trecho.",
			Model:              "gpt-test",
			ProviderResponseID: "resp_tool_1",
		},
	}
	searcher := &fakeAvailabilitySearcher{
		enabled: true,
		result: AvailabilitySearchResult{
			Results: []AvailabilitySearchItem{
				{
					SegmentID:              "seg-1",
					TripID:                 "trip-1",
					RouteID:                "route-1",
					OriginDisplayName:      "Videira/SC",
					DestinationDisplayName: "Sao Luis/MA",
					OriginDepartTime:       "18:30",
					TripDate:               "2026-05-10",
					SeatsAvailable:         12,
					Price:                  250,
					Currency:               "BRL",
					Status:                 "ACTIVE",
					TripStatus:             "SCHEDULED",
					PackageName:            "Pacote p/ Maranhao",
				},
			},
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-tool-1",
			IdempotencyKey:    "idem-tool-1",
			Body:              "quais horarios e o valor de Videira/SC para Sao Luis/MA em 20/05 para 2 pessoas?",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+ingested.Session.ID+"/reprocess", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var out ReprocessResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(out.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(out.ToolCalls))
	}
	if out.ToolCalls[0].ToolName != toolNameAvailabilitySearch {
		t.Fatalf("expected tool name %s, got %s", toolNameAvailabilitySearch, out.ToolCalls[0].ToolName)
	}
	if searcher.calls != 1 {
		t.Fatalf("expected one availability search, got %d", searcher.calls)
	}
	if runner.calls != 1 {
		t.Fatalf("expected one runner call, got %d", runner.calls)
	}
	if searcher.lastInput.Origin != "Videira/SC" || searcher.lastInput.Destination != "Sao Luis/MA" {
		t.Fatalf("unexpected normalized route: %+v", searcher.lastInput)
	}
	if searcher.lastInput.Qty != 2 {
		t.Fatalf("expected quantity 2, got %d", searcher.lastInput.Qty)
	}
	if searcher.lastInput.TripDate == nil || searcher.lastInput.TripDate.UTC().Format("2006-01-02") != "2026-05-20" {
		t.Fatalf("expected inferred date 2026-05-20, got %+v", searcher.lastInput.TripDate)
	}
	if !strings.Contains(runner.lastInput.UserPrompt, "last_validated_tool_facts") {
		t.Fatalf("expected prompt to include validated tool facts")
	}
	if !strings.Contains(runner.lastInput.UserPrompt, "Videira/SC") || !strings.Contains(runner.lastInput.UserPrompt, "Sao Luis/MA") {
		t.Fatalf("expected prompt to include normalized route")
	}
}

func TestReprocessSkipsAvailabilityToolWhenTurnIsBroad(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Fraiburgo: R$ 950. Monte Carlo: R$ 950. Videira: R$ 950.",
			Model:              "gpt-test",
			ProviderResponseID: "resp_tool_2",
		},
	}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-tool-2",
			IdempotencyKey:    "idem-tool-2",
			Body:              "quero ir para Santa Catarina",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+ingested.Session.ID+"/reprocess", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var out ReprocessResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(out.ToolCalls) != 0 {
		t.Fatalf("expected no tool calls, got %d", len(out.ToolCalls))
	}
	if searcher.calls != 0 {
		t.Fatalf("expected no availability search, got %d", searcher.calls)
	}
	if runner.calls != 0 {
		t.Fatalf("expected broad SC template to avoid LLM, got %d calls", runner.calls)
	}
	if out.Draft == nil {
		t.Fatalf("expected deterministic broad SC draft")
	}
	if !strings.Contains(out.Draft.Body, "Fraiburgo: R$ 950") || !strings.Contains(out.Draft.Body, "Ituporanga: R$ 1100") {
		t.Fatalf("expected public SC table, got %q", out.Draft.Body)
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["model"])); got != "template_realizer" {
		t.Fatalf("expected template model, got %q", got)
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["run_mode"])); got != "TEMPLATE_FIRST_REPLY" {
		t.Fatalf("expected template run mode, got %q", got)
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got != string(TemplatePublicSCTable) {
		t.Fatalf("expected template %s, got %q", TemplatePublicSCTable, got)
	}
}

func TestReprocessUsesPackageAvailabilityForBroadStateDateLookup(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Tenho datas disponiveis para Santa Catarina.",
			Model:              "gpt-test",
			ProviderResponseID: "resp_tool_sc_dates_1",
		},
	}
	searcher := &fakeAvailabilitySearcher{
		enabled: true,
		result: AvailabilitySearchResult{
			Results: []AvailabilitySearchItem{
				{
					SegmentID:              "seg-sc-1",
					TripID:                 "trip-sc-1",
					RouteID:                "route-sc-1",
					OriginDisplayName:      "Santa Ines/MA",
					DestinationDisplayName: "Videira/SC",
					OriginDepartTime:       "18:30",
					TripDate:               "2026-05-10",
					SeatsAvailable:         9,
					Price:                  950,
					Currency:               "BRL",
					Status:                 "ACTIVE",
					TripStatus:             "SCHEDULED",
					PackageName:            packageToSantaCatarina,
				},
			},
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-tool-sc-dates-1",
			IdempotencyKey:    "idem-tool-sc-dates-1",
			Body:              "quais datas para Santa Catarina?",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+ingested.Session.ID+"/reprocess", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var out ReprocessResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(out.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(out.ToolCalls))
	}
	if searcher.calls != 1 {
		t.Fatalf("expected one availability search, got %d", searcher.calls)
	}
	if searcher.lastInput.PackageName != packageToSantaCatarina {
		t.Fatalf("expected package search %q, got %+v", packageToSantaCatarina, searcher.lastInput)
	}
	if searcher.lastInput.Origin != "" || searcher.lastInput.Destination != "" {
		t.Fatalf("expected package-level search without fixed route, got %+v", searcher.lastInput)
	}
	if !strings.Contains(runner.lastInput.UserPrompt, `"package_name":"`+packageToSantaCatarina+`"`) {
		t.Fatalf("expected prompt to expose package-level availability context")
	}
	if strings.Contains(runner.lastInput.UserPrompt, "priorize listar ate 5 datas futuras") {
		t.Fatalf("expected prompt not to include legacy date-listing prose")
	}
}

func TestReprocessGeneratesSupportDraftForUnsupportedExplicitDestinations(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "broad_state", body: "quero passagem para bahia"},
		{name: "unsupported_city", body: "quero passagem pra ilheus"},
		{name: "bare_from_to", body: "jaragua do sul para ilheus"},
		{name: "travel_availability", body: "tem viagem para salvador"},
		{name: "go_to_city", body: "quero ir para curitiba"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeStore()
			runner := &fakeAgentRunner{
				enabled: true,
				result: RunAgentResult{
					ReplyText:          "Qual cidade voce quer consultar?",
					Model:              "gpt-test",
					ProviderResponseID: "resp-should-not-run",
				},
			}
			searcher := &fakeAvailabilitySearcher{enabled: true}
			pricingSearcher := &fakePricingQuoteSearcher{enabled: true}
			svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher, pricingSearcher)

			ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
				ContactKey: "5511999999999",
				Message: IngestMessagePayload{
					Direction:         "INBOUND",
					ProviderMessageID: "msg-unsupported-" + tc.name,
					IdempotencyKey:    "idem-unsupported-" + tc.name,
					Body:              tc.body,
				},
			})
			if err != nil {
				t.Fatalf("ingest message: %v", err)
			}

			out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
			if err != nil {
				t.Fatalf("reprocess: %v", err)
			}
			if runner.calls != 0 {
				t.Fatalf("expected deterministic support draft without agent run, got %d calls", runner.calls)
			}
			if searcher.calls != 0 {
				t.Fatalf("expected no availability search for unsupported destination, got %d calls", searcher.calls)
			}
			if pricingSearcher.calls != 0 {
				t.Fatalf("expected no pricing search for unsupported destination, got %d calls", pricingSearcher.calls)
			}
			if len(out.ToolCalls) != 0 {
				t.Fatalf("expected no tool calls for unsupported destination, got %d", len(out.ToolCalls))
			}
			if out.Draft == nil {
				t.Fatalf("expected support draft")
			}
			if !strings.Contains(out.Draft.Body, "atendemos apenas viagens dos pacotes Santa Catarina e Maranhao") {
				t.Fatalf("expected supported packages message, got %q", out.Draft.Body)
			}
			if !strings.Contains(out.Draft.Body, unsupportedPackageSupportPhone) {
				t.Fatalf("expected support phone in message, got %q", out.Draft.Body)
			}
			for _, blockedTerm := range []string{"data", "cidade", "mes", "disponibilidade"} {
				if strings.Contains(foldChatText(out.Draft.Body), " "+blockedTerm+" ") {
					t.Fatalf("support draft should not ask about %s: %q", blockedTerm, out.Draft.Body)
				}
			}
		})
	}
}

func TestInferUnsupportedPackageQueryAllowsGenericAndSupportedDestinations(t *testing.T) {
	cases := []struct {
		text        string
		unsupported bool
	}{
		{text: "quero passagem"},
		{text: "tem viagem?"},
		{text: "quero viajar"},
		{text: "quero passagem para Santa Catarina"},
		{text: "quero passagem para SC"},
		{text: "quero passagem para Maranhao"},
		{text: "quero passagem para MA"},
		{text: "quero passagem para Chapeco"},
		{text: "quero passagem para Fraiburgo"},
		{text: "quero passagem para Santa Ines"},
		{text: "qual o valor de Videira/SC para Sao Luis/MA em 10/05 para 2 pessoas?"},
		{text: "quero reagendar a reserva ABC12345 para 12/06/2026"},
		{text: "quero passagem para Bahia", unsupported: true},
		{text: "quero passagem pra Ilheus", unsupported: true},
		{text: "Jaragua do Sul para Ilheus", unsupported: true},
		{text: "tem viagem para Salvador", unsupported: true},
		{text: "quero ir para Curitiba", unsupported: true},
	}

	for _, tc := range cases {
		t.Run(tc.text, func(t *testing.T) {
			_, unsupported := inferUnsupportedPackageQuery(tc.text)
			if unsupported != tc.unsupported {
				t.Fatalf("expected unsupported=%v for %q, got %v", tc.unsupported, tc.text, unsupported)
			}
		})
	}
}

func TestReprocessDoesNotFallbackUnsupportedForNormalizedSupportedCity(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Você sai de qual cidade do Maranhão?",
			Model:              "gpt-test",
			ProviderResponseID: "resp-fraiburgo-normalized",
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)

	session, _ := store.seedSessionWithMessage("5511999999999", "oi")
	now := time.Now().UTC()
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        session.ID,
		Direction:        "OUTBOUND",
		Kind:             "TEXT",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       now.Add(-2 * time.Minute),
		Body:             "Fraiburgo R$ 950; Monte Carlo R$ 950; Videira R$ 950.",
	}); err != nil {
		t.Fatalf("create outbound table: %v", err)
	}

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-fraiburgo-city-only",
			IdempotencyKey:    "idem-fraiburgo-city-only",
			Body:              "passagem para Fraiburgo",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess: %v", err)
	}
	if runner.calls != 1 {
		t.Fatalf("expected agent runner to be called once, got %d", runner.calls)
	}
	if out.Draft == nil {
		t.Fatalf("expected agent draft")
	}
	if strings.Contains(out.Draft.Body, "atendemos apenas viagens dos pacotes Santa Catarina e Maranhao") {
		t.Fatalf("expected supported city flow, got %q", out.Draft.Body)
	}
	if !strings.Contains(runner.lastInput.UserPrompt, `"destination":"Fraiburgo/SC"`) {
		t.Fatalf("expected prompt to infer Fraiburgo/SC, got %q", runner.lastInput.UserPrompt)
	}
	if strings.Contains(runner.lastInput.UserPrompt, "Guardrail de direcao") {
		t.Fatalf("expected prompt not to include legacy direction guardrail prose, got %q", runner.lastInput.UserPrompt)
	}
}

func TestReprocessAsksOriginAfterBroadStateCitySelection(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Você sai de qual cidade do Maranhão?",
			Model:              "gpt-test",
			ProviderResponseID: "resp_tool_sc_city_1",
		},
	}
	searcher := &fakeAvailabilitySearcher{
		enabled: true,
		result: AvailabilitySearchResult{
			Results: []AvailabilitySearchItem{
				{
					SegmentID:              "seg-sc-city-1",
					TripID:                 "trip-sc-city-1",
					RouteID:                "route-sc-city-1",
					OriginDisplayName:      "Santa Ines/MA",
					DestinationDisplayName: "Seara/SC",
					OriginDepartTime:       "18:30",
					TripDate:               "2026-05-10",
					SeatsAvailable:         6,
					Price:                  1100,
					Currency:               "BRL",
					Status:                 "ACTIVE",
					TripStatus:             "SCHEDULED",
					PackageName:            packageToSantaCatarina,
				},
			},
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)

	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-tool-sc-city-ctx-1",
			IdempotencyKey:    "idem-tool-sc-city-ctx-1",
			Body:              "quero passagem para sc",
		},
	}); err != nil {
		t.Fatalf("ingest broad state message: %v", err)
	}

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-tool-sc-city-1",
			IdempotencyKey:    "idem-tool-sc-city-1",
			Body:              "para Seara",
		},
	})
	if err != nil {
		t.Fatalf("ingest city selection message: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+ingested.Session.ID+"/reprocess", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var out ReprocessResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(out.ToolCalls) != 0 {
		t.Fatalf("expected no tool calls before origin is known, got %d", len(out.ToolCalls))
	}
	if searcher.calls != 0 {
		t.Fatalf("expected no availability search before origin is known, got %d", searcher.calls)
	}
	if !strings.Contains(runner.lastInput.UserPrompt, `"destination":"Seara/SC"`) {
		t.Fatalf("expected prompt to include inferred destination, got %q", runner.lastInput.UserPrompt)
	}
	if strings.Contains(runner.lastInput.UserPrompt, "Guardrail de direcao") {
		t.Fatalf("expected prompt not to include legacy direction guardrail prose, got %q", runner.lastInput.UserPrompt)
	}
}

func TestReprocessStillAsksOriginWhenDateArrivesBeforeOrigin(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Para 10/05 tenho estas saidas do Maranhao para Seara.",
			Model:              "gpt-test",
			ProviderResponseID: "resp_tool_sc_date_choice_1",
		},
	}
	searcher := &fakeAvailabilitySearcher{
		enabled: true,
		result: AvailabilitySearchResult{
			Results: []AvailabilitySearchItem{
				{
					SegmentID:              "seg-sc-date-1",
					TripID:                 "trip-sc-date-1",
					RouteID:                "route-sc-date-1",
					OriginDisplayName:      "Santa Ines/MA",
					DestinationDisplayName: "Seara/SC",
					OriginDepartTime:       "18:30",
					TripDate:               "2026-05-10",
					SeatsAvailable:         6,
					Price:                  1100,
					Currency:               "BRL",
					Status:                 "ACTIVE",
					TripStatus:             "SCHEDULED",
					PackageName:            packageToSantaCatarina,
				},
				{
					SegmentID:              "seg-sc-date-2",
					TripID:                 "trip-sc-date-2",
					RouteID:                "route-sc-date-2",
					OriginDisplayName:      "Moncao/MA",
					DestinationDisplayName: "Seara/SC",
					OriginDepartTime:       "20:15",
					TripDate:               "2026-05-10",
					SeatsAvailable:         4,
					Price:                  1100,
					Currency:               "BRL",
					Status:                 "ACTIVE",
					TripStatus:             "SCHEDULED",
					PackageName:            packageToSantaCatarina,
				},
			},
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)

	messages := []IngestMessagePayload{
		{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-tool-sc-date-ctx-1",
			IdempotencyKey:    "idem-tool-sc-date-ctx-1",
			Body:              "quero passagem para sc",
		},
		{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-tool-sc-date-ctx-2",
			IdempotencyKey:    "idem-tool-sc-date-ctx-2",
			Body:              "para Seara",
		},
		{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-tool-sc-date-1",
			IdempotencyKey:    "idem-tool-sc-date-1",
			Body:              "10/05",
		},
	}

	var ingested IngestMessageResult
	var err error
	for _, message := range messages {
		ingested, err = svc.Ingest(context.Background(), IngestMessageInput{
			ContactKey: "5511999999999",
			Message:    message,
		})
		if err != nil {
			t.Fatalf("ingest message %q: %v", message.Body, err)
		}
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+ingested.Session.ID+"/reprocess", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var out ReprocessResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(out.ToolCalls) != 0 {
		t.Fatalf("expected no tool calls before origin is known, got %d", len(out.ToolCalls))
	}
	if searcher.calls != 0 {
		t.Fatalf("expected no availability search before origin is known, got %d", searcher.calls)
	}
	if !strings.Contains(runner.lastInput.UserPrompt, `"destination":"Seara/SC"`) {
		t.Fatalf("expected prompt to keep inferred destination, got %q", runner.lastInput.UserPrompt)
	}
	if strings.Contains(runner.lastInput.UserPrompt, "Guardrail de direcao") {
		t.Fatalf("expected prompt not to include legacy direction guardrail prose, got %q", runner.lastInput.UserPrompt)
	}
}

func TestReprocessUsesTranscribedAudioAsText(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Tenho datas disponiveis.",
			Model:              "gpt-test",
			ProviderResponseID: "resp_audio_text_1",
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)

	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:       "WHATSAPP",
		ContactKey:    "5511999999999",
		CustomerPhone: "5511999999999",
	})
	if err != nil {
		t.Fatalf("upsert session: %v", err)
	}
	now := time.Now().UTC()
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:         session.ID,
		Direction:         "INBOUND",
		Kind:              "AUDIO",
		ProviderMessageID: "msg-audio-text-1",
		IdempotencyKey:    "idem-audio-text-1",
		Body:              "quais datas disponiveis",
		NormalizedPayload: map[string]interface{}{
			"transcription_status": "COMPLETED",
			"transcription_text":   "quais datas disponiveis",
		},
		ProcessingStatus: "RECEIVED",
		ReceivedAt:       now,
	}); err != nil {
		t.Fatalf("create audio message: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+session.ID+"/reprocess", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if runner.calls != 1 {
		t.Fatalf("expected runner to be called once, got %d", runner.calls)
	}
	if !strings.Contains(runner.lastInput.UserPrompt, "quais datas disponiveis") {
		t.Fatalf("expected runner prompt to include transcribed text, got %q", runner.lastInput.UserPrompt)
	}
	if got := len(runner.lastInput.CurrentTurnMedia); got != 0 {
		t.Fatalf("expected no media inputs for transcribed audio, got %d", got)
	}
}

func TestReprocessIgnoresOlderFailedAudioWhenNewAudioIsTranscribed(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Tenho datas disponiveis.",
			Model:              "gpt-test",
			ProviderResponseID: "resp_audio_text_2",
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)

	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:       "WHATSAPP",
		ContactKey:    "5511999999999",
		CustomerPhone: "5511999999999",
	})
	if err != nil {
		t.Fatalf("upsert session: %v", err)
	}
	now := time.Now().UTC()
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:         session.ID,
		Direction:         "INBOUND",
		Kind:              "AUDIO",
		ProviderMessageID: "msg-audio-old-failed",
		IdempotencyKey:    "idem-audio-old-failed",
		NormalizedPayload: map[string]interface{}{
			"transcription_status": "FAILED",
		},
		ProcessingStatus: "REVIEW_REQUIRED",
		ReceivedAt:       now.Add(-2 * time.Minute),
	}); err != nil {
		t.Fatalf("create failed audio message: %v", err)
	}
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:         session.ID,
		Direction:         "INBOUND",
		Kind:              "AUDIO",
		ProviderMessageID: "msg-audio-new-completed",
		IdempotencyKey:    "idem-audio-new-completed",
		Body:              "quais datas disponiveis",
		NormalizedPayload: map[string]interface{}{
			"transcription_status": "COMPLETED",
			"transcription_text":   "quais datas disponiveis",
		},
		ProcessingStatus: "READY_FOR_AUTOMATION",
		ReceivedAt:       now.Add(-1 * time.Minute),
	}); err != nil {
		t.Fatalf("create completed audio message: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+session.ID+"/reprocess", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if runner.calls != 1 {
		t.Fatalf("expected runner to be called once, got %d", runner.calls)
	}
	if !strings.Contains(runner.lastInput.UserPrompt, "quais datas disponiveis") {
		t.Fatalf("expected runner prompt to include transcribed text, got %q", runner.lastInput.UserPrompt)
	}
	if strings.Contains(runner.lastInput.UserPrompt, "msg-audio-old-failed") {
		t.Fatalf("expected old failed audio not to be part of the prompt, got %q", runner.lastInput.UserPrompt)
	}
	var out ReprocessResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got := asString(out.Memory["current_turn_body"]); got != "quais datas disponiveis" {
		t.Fatalf("expected current_turn_body to ignore failed audio, got %q", got)
	}
}

func TestReprocessPassengerReplyContinuesBookingFlow(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Perfeito, vamos seguir com os documentos.",
			Model:              "gpt-test",
			ProviderResponseID: "resp-passenger-reply-1",
		},
	}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)

	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:       "WHATSAPP",
		ContactKey:    "5511999999999",
		CustomerPhone: "5511999999999",
	})
	if err != nil {
		t.Fatalf("upsert session: %v", err)
	}
	now := time.Now().UTC()
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        session.ID,
		Direction:        "OUTBOUND",
		Kind:             "TEXT",
		Body:             "Perfeito, Messias — a passagem e so para voce ou tem mais alguem? Ha crianca de ate 5 anos viajando?",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       now.Add(-2 * time.Minute),
	}); err != nil {
		t.Fatalf("create passenger question: %v", err)
	}
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID: session.ID,
		Direction: "INBOUND",
		Kind:      "AUDIO",
		Body:      "é só para mim",
		NormalizedPayload: map[string]interface{}{
			"transcription_status": "COMPLETED",
			"transcription_text":   "é só para mim",
		},
		ProcessingStatus: "READY_FOR_AUTOMATION",
		ReceivedAt:       now.Add(-1 * time.Minute),
	}); err != nil {
		t.Fatalf("create passenger reply: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+session.ID+"/reprocess", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if runner.calls != 0 {
		t.Fatalf("expected passenger slot reply to avoid LLM, got %d runner calls", runner.calls)
	}
	if searcher.calls != 0 {
		t.Fatalf("expected no availability search for passenger reply, got %d", searcher.calls)
	}
	draft := latestAutomationDraftForSession(t, store, session.ID)
	if got := strings.TrimSpace(draft.Body); got != "Tem crianca de 5 anos ou menos viajando?" {
		t.Fatalf("expected child-only deterministic reply, got %q", got)
	}
}

func TestPassengerCountContextDoesNotCallLLMForPraMim(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	session := seedPassengerCountContext(t, store, "Perfeito. A passagem e so para voce ou vai mais alguem junto? Tem crianca de 5 anos ou menos?")

	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-pra-mim",
			IdempotencyKey:    "idem-pra-mim",
			Body:              "pra mim",
		},
	}); err != nil {
		t.Fatalf("ingest passenger reply: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected passenger count context to avoid LLM, got %d calls", runner.calls)
	}
	if got := strings.TrimSpace(out.Draft.Body); got != "Tem crianca de 5 anos ou menos viajando?" {
		t.Fatalf("expected child-only deterministic reply, got %q", got)
	}
}

func TestPassengerCountContextDoesNotRepeatComboQuestionWhenPassengerKnown(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	session := seedPassengerCountContext(t, store, "Perfeito. A passagem e so para voce ou vai mais alguem junto? Tem crianca de 5 anos ou menos?")

	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-so-eu",
			IdempotencyKey:    "idem-so-eu",
			Body:              "so eu",
		},
	}); err != nil {
		t.Fatalf("ingest passenger reply: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected passenger count context to avoid LLM, got %d calls", runner.calls)
	}
	if strings.Contains(out.Draft.Body, "vai mais alguem junto") {
		t.Fatalf("expected not to repeat combo question, got %q", out.Draft.Body)
	}
	if got := strings.TrimSpace(out.Draft.Body); got != "Tem crianca de 5 anos ou menos viajando?" {
		t.Fatalf("expected child-only deterministic reply, got %q", got)
	}
}

func TestPassengerCountContextParsesTranscribedAudioSoloEle(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	session := seedPassengerCountContext(t, store, "Perfeito. A passagem e so para voce ou vai mais alguem junto? Tem crianca de 5 anos ou menos?")

	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			Kind:              "AUDIO",
			ProviderMessageID: "msg-audio-solo-ele",
			IdempotencyKey:    "idem-audio-solo-ele",
			Body:              "",
			NormalizedPayload: map[string]interface{}{
				"transcription_status": "COMPLETED",
				"transcription_text":   "A passagem é só para ele mesmo.",
			},
		},
	}); err != nil {
		t.Fatalf("ingest passenger reply: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected transcribed passenger audio to avoid LLM, got %d calls", runner.calls)
	}
	if got := asInt(out.Memory["passenger_count"]); got != 1 {
		t.Fatalf("expected passenger_count=1, got %d", got)
	}
	if got := strings.TrimSpace(out.Draft.Body); got != "Tem crianca de 5 anos ou menos viajando?" {
		t.Fatalf("expected child-only deterministic reply, got %q", got)
	}
}

func TestPassengerCountContextParsesTranscribedAudioSoloMim(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	session := seedPassengerCountContext(t, store, "Perfeito. A passagem e so para voce ou vai mais alguem junto? Tem crianca de 5 anos ou menos?")

	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			Kind:              "TEXT",
			ProviderMessageID: "msg-audio-solo-mim",
			IdempotencyKey:    "idem-audio-solo-mim",
			Body:              "A passagem é só pra mim.",
			NormalizedPayload: map[string]interface{}{
				"transcription_status": "COMPLETED",
				"transcription_text":   "A passagem é só pra mim.",
				"message_text":         "A passagem é só pra mim.",
			},
			ProcessingStatus: "READY_FOR_AUTOMATION",
		},
	}); err != nil {
		t.Fatalf("ingest passenger reply: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected transcribed passenger audio to avoid LLM, got %d calls", runner.calls)
	}
	if got := asInt(out.Memory["passenger_count"]); got != 1 {
		t.Fatalf("expected passenger_count=1, got %d", got)
	}
	if got := strings.TrimSpace(out.Draft.Body); got != "Tem crianca de 5 anos ou menos viajando?" {
		t.Fatalf("expected child-only deterministic reply, got %q", got)
	}
	if strings.Contains(out.Draft.Body, "A passagem e so para voce ou vai mais alguem junto") {
		t.Fatalf("expected not to repeat passenger question, got %q", out.Draft.Body)
	}
}

func TestPassengerCountContextParsesTranscribedAudioSoloNoChildAsksDocuments(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	session := seedPassengerCountContextWithAvailability(t, store, "Perfeito. A passagem e so para voce ou vai mais alguem junto? Tem crianca de 5 anos ou menos?")

	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			Kind:              "TEXT",
			ProviderMessageID: "msg-audio-solo-no-child",
			IdempotencyKey:    "idem-audio-solo-no-child",
			Body:              "A passagem é só pra mim, não tem criança.",
			NormalizedPayload: map[string]interface{}{
				"transcription_status": "COMPLETED",
				"transcription_text":   "A passagem é só pra mim, não tem criança.",
				"message_text":         "A passagem é só pra mim, não tem criança.",
			},
			ProcessingStatus: "READY_FOR_AUTOMATION",
		},
	}); err != nil {
		t.Fatalf("ingest passenger reply: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected transcribed passenger audio to avoid LLM, got %d calls", runner.calls)
	}
	if got := asInt(out.Memory["passenger_count"]); got != 1 {
		t.Fatalf("expected passenger_count=1, got %d", got)
	}
	if got := asInt(out.Memory["child_under_5_count"]); got != 0 {
		t.Fatalf("expected child_under_5_count=0, got %d", got)
	}
	if got := strings.TrimSpace(out.Draft.Body); !strings.Contains(got, "documento de 1 passageiro") {
		t.Fatalf("expected document request, got %q", got)
	}
	if strings.Contains(out.Draft.Body, "A passagem e so para voce ou vai mais alguem junto") {
		t.Fatalf("expected not to repeat passenger question, got %q", out.Draft.Body)
	}
}

func TestPassengerCountContextTranscribedAudioAmbiguousUsesFallback(t *testing.T) {
	cases := []string{"sim", "ok", "hum"}

	for _, text := range cases {
		t.Run(text, func(t *testing.T) {
			store := newFakeStore()
			runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
			svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
			session := seedPassengerCountContext(t, store, "Perfeito. A passagem e so para voce ou vai mais alguem junto? Tem crianca de 5 anos ou menos?")

			if _, err := svc.Ingest(context.Background(), IngestMessageInput{
				ContactKey: session.ContactKey,
				Message: IngestMessagePayload{
					Direction:         "INBOUND",
					Kind:              "TEXT",
					ProviderMessageID: "msg-audio-ambiguous-" + text,
					IdempotencyKey:    "idem-audio-ambiguous-" + text,
					Body:              text,
					NormalizedPayload: map[string]interface{}{
						"transcription_status": "COMPLETED",
						"transcription_text":   text,
						"message_text":         text,
					},
					ProcessingStatus: "READY_FOR_AUTOMATION",
				},
			}); err != nil {
				t.Fatalf("ingest passenger reply: %v", err)
			}

			out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
			if err != nil {
				t.Fatalf("reprocess: %v", err)
			}
			if runner.calls != 0 {
				t.Fatalf("expected ambiguous transcribed audio to avoid LLM, got %d calls", runner.calls)
			}
			if got := asInt(out.Memory["passenger_count"]); got != 0 {
				t.Fatalf("expected not to infer passenger_count, got %d", got)
			}
			if got, _ := out.Memory["child_under_5_count_known"].(bool); got {
				t.Fatalf("expected not to infer child_under_5_count from ambiguous audio")
			}
			if got := strings.TrimSpace(out.Draft.Body); !strings.Contains(got, "Nao consegui entender o audio com seguranca") || !strings.Contains(got, "Exemplo: so eu ou eu e mais uma pessoa") {
				t.Fatalf("expected audio clarification fallback, got %q", got)
			}
			if got := strings.TrimSpace(out.Draft.Body); got == "Perfeito. A passagem e so para voce ou vai mais alguem junto? Tem crianca de 5 anos ou menos?" {
				t.Fatalf("expected not to repeat exact passenger question, got %q", got)
			}
		})
	}
}

func TestSCDestinationFollowUpAsksMAOrigin(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
	session := seedPublicSCTableContext(t, store)

	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-sc-followup-ituporanga",
			IdempotencyKey:    "idem-sc-followup-ituporanga",
			Body:              "Quero pra Ituporanga.",
		},
	}); err != nil {
		t.Fatalf("ingest SC follow-up: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess: %v", err)
	}
	if out.Draft == nil {
		t.Fatalf("expected draft reply")
	}
	if got := strings.TrimSpace(out.Draft.Body); got != "Perfeito — Ituporanga/SC. De qual cidade do Maranhao voce vai sair?" {
		t.Fatalf("expected MA origin prompt, got %q", got)
	}
	if runner.calls != 0 {
		t.Fatalf("expected deterministic follow-up to avoid LLM, got %d calls", runner.calls)
	}
}

func TestSCDestinationFollowUpDoesNotCallAvailabilityYet(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
	session := seedPublicSCTableContext(t, store)

	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-sc-followup-ituporanga-no-tool",
			IdempotencyKey:    "idem-sc-followup-ituporanga-no-tool",
			Body:              "Quero pra Ituporanga.",
		},
	}); err != nil {
		t.Fatalf("ingest SC follow-up: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess: %v", err)
	}
	if out.Draft == nil {
		t.Fatalf("expected draft reply")
	}
	if len(out.ToolCalls) != 0 {
		t.Fatalf("expected no tool calls yet, got %+v", out.ToolCalls)
	}
	if searcher.calls != 0 {
		t.Fatalf("expected no availability search yet, got %d", searcher.calls)
	}
}

func TestOperationalUnsupportedLLMDraftStillBlocked(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "A rota para Ituporanga nao esta disponivel no momento.",
			Model:              "gpt-test",
			ProviderResponseID: "resp-unsupported-llm-block",
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:       "WHATSAPP",
		ContactKey:    "5511999999999",
		CustomerPhone: "5511999999999",
	})
	if err != nil {
		t.Fatalf("upsert session: %v", err)
	}

	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-unsupported-llm-block",
			IdempotencyKey:    "idem-unsupported-llm-block",
			Body:              "Quero pra Ituporanga.",
		},
	}); err != nil {
		t.Fatalf("ingest message: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess: %v", err)
	}
	if out.Draft == nil {
		t.Fatalf("expected LLM draft to be persisted")
	}
	if got := readDraftAutoSendStatus(*out.Draft); got != draftAutoSendStatusReviewNeeded {
		t.Fatalf("expected auto_send_status %s, got %s", draftAutoSendStatusReviewNeeded, got)
	}
	reasons := readDraftAutoSendReasons(*out.Draft)
	if len(reasons) != 1 || reasons[0] != draftAutoSendReasonOperationalClaimWithoutTool {
		t.Fatalf("expected review reason %q, got %+v", draftAutoSendReasonOperationalClaimWithoutTool, reasons)
	}
}

func seedPassengerCountContext(t *testing.T, store *fakeStore, question string) Session {
	t.Helper()
	now := time.Now().UTC()
	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:        "WHATSAPP",
		ContactKey:     "5511999999999",
		CustomerPhone:  "5511999999999",
		LastMessageAt:  &now,
		LastOutboundAt: &now,
	})
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        session.ID,
		Direction:        "OUTBOUND",
		Kind:             "TEXT",
		Body:             question,
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       now.Add(-2 * time.Minute),
	}); err != nil {
		t.Fatalf("seed passenger question: %v", err)
	}
	return session
}

func seedPassengerCountContextWithAvailability(t *testing.T, store *fakeStore, question string) Session {
	t.Helper()
	now := time.Now().UTC()
	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:        "WHATSAPP",
		ContactKey:     "5511999999999",
		CustomerPhone:  "5511999999999",
		LastMessageAt:  &now,
		LastOutboundAt: &now,
	})
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        session.ID,
		Direction:        "OUTBOUND",
		Kind:             "TEXT",
		Body:             "Encontrei estas opcoes para Santa Ines/MA -> Fraiburgo/SC.",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       now.Add(-4 * time.Minute),
		Payload: map[string]interface{}{
			"tool_context": map[string]interface{}{
				toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(AvailabilitySearchResult{
					Filter: AvailabilitySearchInput{
						Origin:      "Santa Ines/MA",
						Destination: "Fraiburgo/SC",
						Qty:         1,
						Limit:       5,
					},
					Results: []AvailabilitySearchItem{{
						TripID:                 "trip-1",
						BoardStopID:            "board-1",
						AlightStopID:           "alight-1",
						OriginDisplayName:      "Santa Ines/MA",
						DestinationDisplayName: "Fraiburgo/SC",
						OriginDepartTime:       "12:00",
						TripDate:               "2026-05-25",
						Price:                  950,
						Currency:               "BRL",
					}},
				}),
			},
		},
	}); err != nil {
		t.Fatalf("seed availability: %v", err)
	}
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        session.ID,
		Direction:        "INBOUND",
		Kind:             "TEXT",
		Body:             "primeira opcao",
		ProcessingStatus: "PROCESSED",
		ReceivedAt:       now.Add(-3 * time.Minute),
	}); err != nil {
		t.Fatalf("seed selected option: %v", err)
	}
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        session.ID,
		Direction:        "OUTBOUND",
		Kind:             "TEXT",
		Body:             question,
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       now.Add(-2 * time.Minute),
	}); err != nil {
		t.Fatalf("seed passenger question: %v", err)
	}
	return session
}

func seedPublicSCTableContext(t *testing.T, store *fakeStore) Session {
	t.Helper()
	now := time.Now().UTC()
	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:        "WHATSAPP",
		ContactKey:     "5511999999999",
		CustomerPhone:  "5511999999999",
		LastMessageAt:  &now,
		LastOutboundAt: &now,
	})
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        session.ID,
		Direction:        "OUTBOUND",
		Kind:             "TEXT",
		Body:             "Oi Messias, temos sim. Valores por cidade em Santa Catarina:\nFraiburgo R$ 950\nVideira R$ 950\nItuporanga R$ 1100",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       now.Add(-2 * time.Minute),
	}); err != nil {
		t.Fatalf("seed public SC table: %v", err)
	}
	return session
}

func latestAutomationDraftForSession(t *testing.T, store *fakeStore, sessionID string) Message {
	t.Helper()
	var latest *Message
	for _, messageID := range store.messageOrder {
		message := store.messages[messageID]
		if message.SessionID != sessionID ||
			message.Direction != "OUTBOUND" ||
			message.ProcessingStatus != messageStatusAutomationDraft {
			continue
		}
		item := message
		latest = &item
	}
	if latest == nil {
		t.Fatalf("expected automation draft for session %s", sessionID)
	}
	return *latest
}

func TestReprocessSkipsRunnerForUntranscribedAudio(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "draft",
			Model:              "gpt-test",
			ProviderResponseID: "resp_audio_skip_1",
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)

	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:       "WHATSAPP",
		ContactKey:    "5511999999999",
		CustomerPhone: "5511999999999",
	})
	if err != nil {
		t.Fatalf("upsert session: %v", err)
	}
	now := time.Now().UTC()
	message, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:         session.ID,
		Direction:         "INBOUND",
		Kind:              "AUDIO",
		ProviderMessageID: "msg-audio-skip-1",
		IdempotencyKey:    "idem-audio-skip-1",
		ProcessingStatus:  "RECEIVED",
		ReceivedAt:        now,
	})
	if err != nil {
		t.Fatalf("create audio message: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+session.ID+"/reprocess", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if runner.calls != 0 {
		t.Fatalf("expected runner to be skipped, got %d calls", runner.calls)
	}
	var out ReprocessResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if out.Draft != nil {
		t.Fatalf("expected no draft when audio is untranscribed, got %+v", out.Draft)
	}
	if out.Reason != "review_required" {
		t.Fatalf("expected review_required reason, got %s", out.Reason)
	}
	updated := store.messages[message.ID]
	if got := strings.TrimSpace(asString(updated.NormalizedPayload["auto_send_status"])); got != draftAutoSendStatusReviewNeeded {
		t.Fatalf("expected auto_send_status %s, got %s", draftAutoSendStatusReviewNeeded, got)
	}
	reasons := asStringSlice(updated.NormalizedPayload["auto_send_reasons"])
	if len(reasons) != 1 || reasons[0] != draftAutoSendReasonNonTextTurn {
		t.Fatalf("expected non_text_turn review reason, got %+v", reasons)
	}
}

func TestReprocessReturnsBadGatewayWhenAvailabilityToolFails(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "draft",
			Model:              "gpt-test",
			ProviderResponseID: "resp_tool_3",
		},
	}
	searcher := &fakeAvailabilitySearcher{
		enabled: true,
		err:     errors.New("timeout"),
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-tool-3",
			IdempotencyKey:    "idem-tool-3",
			Body:              "preciso de vagas de Videira/SC para Sao Luis/MA",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+ingested.Session.ID+"/reprocess", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected status %d, got %d", http.StatusBadGateway, rec.Code)
	}
	if runner.calls != 0 {
		t.Fatalf("expected no runner call after tool failure, got %d", runner.calls)
	}
	if len(store.toolCalls) != 1 {
		t.Fatalf("expected one stored tool call, got %d", len(store.toolCalls))
	}
}

func TestReprocessUsesPricingQuoteToolAfterAvailabilityForPriceIntent(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Encontrei os valores confirmados para esse trecho.",
			Model:              "gpt-test",
			ProviderResponseID: "resp_pricing_1",
		},
	}
	availabilitySearcher := &fakeAvailabilitySearcher{
		enabled: true,
		result: AvailabilitySearchResult{
			Results: []AvailabilitySearchItem{
				{
					SegmentID:              "seg-1",
					TripID:                 "trip-1",
					RouteID:                "route-1",
					BoardStopID:            "board-1",
					AlightStopID:           "alight-1",
					OriginStopID:           "stop-origin-1",
					DestinationStopID:      "stop-destination-1",
					OriginDisplayName:      "Videira/SC",
					DestinationDisplayName: "Sao Luis/MA",
					OriginDepartTime:       "18:30",
					TripDate:               "2026-05-10",
					SeatsAvailable:         12,
					Price:                  250,
					Currency:               "BRL",
					Status:                 "ACTIVE",
					TripStatus:             "SCHEDULED",
					PackageName:            "Pacote p/ Maranhao",
				},
			},
		},
	}
	pricingSearcher := &fakePricingQuoteSearcher{
		enabled: true,
		result: PricingQuoteResult{
			Filter: PricingQuoteInput{FareMode: "AUTO"},
			Results: []PricingQuoteItem{
				{
					TripID:                 "trip-1",
					RouteID:                "route-1",
					BoardStopID:            "board-1",
					AlightStopID:           "alight-1",
					OriginStopID:           "stop-origin-1",
					DestinationStopID:      "stop-destination-1",
					OriginDisplayName:      "Videira/SC",
					DestinationDisplayName: "Sao Luis/MA",
					OriginDepartTime:       "18:30",
					TripDate:               "2026-05-10",
					BaseAmount:             250,
					CalcAmount:             250,
					FinalAmount:            250,
					Currency:               "BRL",
					FareMode:               "AUTO",
				},
			},
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, availabilitySearcher, pricingSearcher)

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-pricing-tool-1",
			IdempotencyKey:    "idem-pricing-tool-1",
			Body:              "qual o valor de Videira/SC para Sao Luis/MA em 10/05 para 2 pessoas?",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+ingested.Session.ID+"/reprocess", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var out ReprocessResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(out.ToolCalls) != 2 {
		t.Fatalf("expected two tool calls, got %d", len(out.ToolCalls))
	}
	if out.ToolCalls[0].ToolName != toolNameAvailabilitySearch {
		t.Fatalf("expected first tool name %s, got %s", toolNameAvailabilitySearch, out.ToolCalls[0].ToolName)
	}
	if out.ToolCalls[1].ToolName != toolNamePricingQuote {
		t.Fatalf("expected second tool name %s, got %s", toolNamePricingQuote, out.ToolCalls[1].ToolName)
	}
	if availabilitySearcher.calls != 1 {
		t.Fatalf("expected one availability search, got %d", availabilitySearcher.calls)
	}
	if pricingSearcher.calls != 1 {
		t.Fatalf("expected one pricing quote search, got %d", pricingSearcher.calls)
	}
	if len(pricingSearcher.lastInput.Candidates) != 1 {
		t.Fatalf("expected one pricing candidate, got %d", len(pricingSearcher.lastInput.Candidates))
	}
	if pricingSearcher.lastInput.Candidates[0].TripID != "trip-1" || pricingSearcher.lastInput.Candidates[0].BoardStopID != "board-1" {
		t.Fatalf("unexpected pricing candidate: %+v", pricingSearcher.lastInput.Candidates[0])
	}
	if !strings.Contains(runner.lastInput.UserPrompt, toolNamePricingQuote) {
		t.Fatalf("expected prompt to include pricing quote section")
	}
	if !strings.Contains(runner.lastInput.UserPrompt, `"final_amount":"R$ 250.00"`) {
		t.Fatalf("expected prompt to include quoted final amount")
	}
}

func TestReprocessReturnsBadGatewayWhenPricingQuoteToolFails(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "draft",
			Model:              "gpt-test",
			ProviderResponseID: "resp_pricing_2",
		},
	}
	availabilitySearcher := &fakeAvailabilitySearcher{
		enabled: true,
		result: AvailabilitySearchResult{
			Results: []AvailabilitySearchItem{
				{
					SegmentID:              "seg-1",
					TripID:                 "trip-1",
					RouteID:                "route-1",
					BoardStopID:            "board-1",
					AlightStopID:           "alight-1",
					OriginStopID:           "stop-origin-1",
					DestinationStopID:      "stop-destination-1",
					OriginDisplayName:      "Videira/SC",
					DestinationDisplayName: "Sao Luis/MA",
					OriginDepartTime:       "18:30",
					TripDate:               "2026-05-10",
					Price:                  250,
					Currency:               "BRL",
					Status:                 "ACTIVE",
					TripStatus:             "SCHEDULED",
				},
			},
		},
	}
	pricingSearcher := &fakePricingQuoteSearcher{
		enabled: true,
		err:     errors.New("pricing timeout"),
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, availabilitySearcher, pricingSearcher)

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-pricing-tool-2",
			IdempotencyKey:    "idem-pricing-tool-2",
			Body:              "qual o preco de Videira/SC para Sao Luis/MA?",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+ingested.Session.ID+"/reprocess", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected status %d, got %d", http.StatusBadGateway, rec.Code)
	}
	if runner.calls != 0 {
		t.Fatalf("expected no runner call after pricing tool failure, got %d", runner.calls)
	}
	if len(store.toolCalls) != 2 {
		t.Fatalf("expected two stored tool calls, got %d", len(store.toolCalls))
	}
	if store.toolCalls[store.toolCallOrder[1]].ToolName != toolNamePricingQuote {
		t.Fatalf("expected second stored tool call to be %s", toolNamePricingQuote)
	}
}

func TestReprocessUsesBookingLookupToolWhenTurnHasReservationCode(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Encontrei a sua reserva e ela segue pendente.",
			Model:              "gpt-test",
			ProviderResponseID: "resp_booking_1",
		},
	}
	searcher := &fakeBookingLookupSearcher{
		enabled: true,
		result: BookingLookupResult{
			Results: []BookingLookupItem{
				{
					ID:              "BK-ABC123456",
					TripID:          "trip-1",
					Status:          "PENDING",
					ReservationCode: "ABC12345",
					TotalAmount:     950,
					DepositAmount:   300,
					RemainderAmount: 650,
					PassengerName:   "Maria",
					PassengerPhone:  "48999999999",
					SeatNumber:      12,
					CreatedAt:       time.Now().UTC(),
				},
			},
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-booking-tool-1",
			IdempotencyKey:    "idem-booking-tool-1",
			Body:              "qual o status da minha reserva ABC12345?",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+ingested.Session.ID+"/reprocess", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var out ReprocessResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(out.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(out.ToolCalls))
	}
	if out.ToolCalls[0].ToolName != toolNameBookingLookup {
		t.Fatalf("expected tool name %s, got %s", toolNameBookingLookup, out.ToolCalls[0].ToolName)
	}
	if searcher.calls != 1 {
		t.Fatalf("expected one booking lookup, got %d", searcher.calls)
	}
	if searcher.lastInput.ReservationCode != "ABC12345" {
		t.Fatalf("expected reservation_code ABC12345, got %s", searcher.lastInput.ReservationCode)
	}
	if !strings.Contains(runner.lastInput.UserPrompt, "codigo ABC12345") && !strings.Contains(runner.lastInput.UserPrompt, "ABC12345") {
		t.Fatalf("expected prompt to include reservation code")
	}
	if !strings.Contains(runner.lastInput.UserPrompt, "PENDING") {
		t.Fatalf("expected prompt to include booking status")
	}
}

func TestReprocessReturnsBadGatewayWhenBookingLookupToolFails(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "draft",
			Model:              "gpt-test",
			ProviderResponseID: "resp_booking_2",
		},
	}
	searcher := &fakeBookingLookupSearcher{
		enabled: true,
		err:     errors.New("db timeout"),
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-booking-tool-2",
			IdempotencyKey:    "idem-booking-tool-2",
			Body:              "me diz o status da reserva ABC12345",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+ingested.Session.ID+"/reprocess", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected status %d, got %d", http.StatusBadGateway, rec.Code)
	}
	if runner.calls != 0 {
		t.Fatalf("expected no runner call after booking tool failure, got %d", runner.calls)
	}
	if len(store.toolCalls) != 1 {
		t.Fatalf("expected one stored tool call, got %d", len(store.toolCalls))
	}
}

func TestReprocessUsesRescheduleLookupToolWhenTurnRequestsReschedule(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Encontrei opcoes para a nova data e vou encaminhar para revisao.",
			Model:              "gpt-test",
			ProviderResponseID: "resp_reschedule_1",
		},
	}
	searcher := &fakeRescheduleAssistSearcher{
		enabled: true,
		result: RescheduleAssistResult{
			Mode:                "manual_review_required_with_options",
			NextStep:            "revisao_humana",
			HumanReviewRequired: true,
			Booking: &RescheduleAssistBooking{
				ID:              "BK-ABC123456",
				ReservationCode: "ABC12345",
				Status:          "PENDING",
			},
			Current: RescheduleAssistRoute{
				Origin:         "Videira/SC",
				Destination:    "Sao Luis/MA",
				TripDate:       "2026-06-10",
				PassengerCount: 2,
			},
			Requested: RescheduleAssistRequest{
				Origin:      "Videira/SC",
				Destination: "Sao Luis/MA",
				TripDate:    "2026-06-12",
				Qty:         2,
			},
			Options: []RescheduleAssistOption{
				{
					TripID:         "trip-2",
					TripDate:       "2026-06-12",
					DepartureTime:  "18:30",
					Origin:         "Videira/SC",
					Destination:    "Sao Luis/MA",
					BoardStopID:    "stop-1",
					AlightStopID:   "stop-2",
					SeatsAvailable: 6,
					Price:          950,
					Currency:       "BRL",
					PackageName:    "Convencional",
				},
			},
			FieldsRequiredForManualCompletion: []string{"trip_id", "board_stop_id", "alight_stop_id"},
			MessageForAgent:                   "Nao confirme reagendamento como concluido; a troca depende de revisao humana.",
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-reschedule-tool-1",
			IdempotencyKey:    "idem-reschedule-tool-1",
			Body:              "quero reagendar a reserva ABC12345 para 12/06/2026",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+ingested.Session.ID+"/reprocess", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var out ReprocessResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(out.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(out.ToolCalls))
	}
	if out.ToolCalls[0].ToolName != toolNameRescheduleLookup {
		t.Fatalf("expected tool name %s, got %s", toolNameRescheduleLookup, out.ToolCalls[0].ToolName)
	}
	if searcher.calls != 1 {
		t.Fatalf("expected one reschedule lookup, got %d", searcher.calls)
	}
	if searcher.lastInput.ReservationCode != "ABC12345" {
		t.Fatalf("expected reservation_code ABC12345, got %s", searcher.lastInput.ReservationCode)
	}
	if searcher.lastInput.RequestedTripDate == nil || searcher.lastInput.RequestedTripDate.UTC().Format("2006-01-02") != "2026-06-12" {
		t.Fatalf("expected requested trip date 2026-06-12")
	}
	if !strings.Contains(runner.lastInput.UserPrompt, toolNameRescheduleLookup) {
		t.Fatalf("expected prompt to include reschedule tool section")
	}
	if !strings.Contains(runner.lastInput.UserPrompt, "Nunca confirme o reagendamento como concluido") &&
		!strings.Contains(runner.lastInput.UserPrompt, "revisao humana") {
		t.Fatalf("expected prompt to include manual review guardrail")
	}
}

func TestReprocessReturnsBadGatewayWhenRescheduleLookupToolFails(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "draft",
			Model:              "gpt-test",
			ProviderResponseID: "resp_reschedule_2",
		},
	}
	searcher := &fakeRescheduleAssistSearcher{
		enabled: true,
		err:     errors.New("reports timeout"),
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-reschedule-tool-2",
			IdempotencyKey:    "idem-reschedule-tool-2",
			Body:              "preciso reagendar a reserva ABC12345 para 14/06/2026",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+ingested.Session.ID+"/reprocess", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected status %d, got %d", http.StatusBadGateway, rec.Code)
	}
	if runner.calls != 0 {
		t.Fatalf("expected no runner call after reschedule tool failure, got %d", runner.calls)
	}
	if len(store.toolCalls) != 1 {
		t.Fatalf("expected one stored tool call, got %d", len(store.toolCalls))
	}
	if store.toolCalls[store.toolCallOrder[0]].ToolName != toolNameRescheduleLookup {
		t.Fatalf("expected stored tool call to be %s", toolNameRescheduleLookup)
	}
}

func TestReprocessUsesPaymentStatusToolWhenTurnAsksAboutPix(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Vi aqui que o PIX ainda esta pendente.",
			Model:              "gpt-test",
			ProviderResponseID: "resp_payment_1",
		},
	}
	bookingSearcher := &fakeBookingLookupSearcher{
		enabled: true,
		result: BookingLookupResult{
			Results: []BookingLookupItem{
				{
					ID:              "BK-ABC123456",
					TripID:          "trip-1",
					Status:          "PENDING",
					ReservationCode: "ABC12345",
					TotalAmount:     950,
					DepositAmount:   300,
					RemainderAmount: 650,
					PassengerName:   "Maria",
					PassengerPhone:  "48999999999",
					SeatNumber:      12,
					CreatedAt:       time.Now().UTC(),
				},
			},
		},
	}
	paymentSearcher := &fakePaymentStatusSearcher{
		enabled: true,
		result: PaymentStatusResult{
			Results: []PaymentStatusItem{
				{
					ID:          "pay-1",
					BookingID:   "BK-ABC123456",
					Amount:      300,
					Method:      "PIX",
					Status:      "PENDING",
					Provider:    "PAGARME",
					ProviderRef: "charge-1",
					CreatedAt:   time.Now().UTC(),
				},
			},
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, bookingSearcher, paymentSearcher)

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-payment-tool-1",
			IdempotencyKey:    "idem-payment-tool-1",
			Body:              "o pix da reserva ABC12345 ja foi pago?",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+ingested.Session.ID+"/reprocess", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var out ReprocessResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(out.ToolCalls) != 2 {
		t.Fatalf("expected two tool calls, got %d", len(out.ToolCalls))
	}
	if out.ToolCalls[0].ToolName != toolNameBookingLookup {
		t.Fatalf("expected first tool name %s, got %s", toolNameBookingLookup, out.ToolCalls[0].ToolName)
	}
	if out.ToolCalls[1].ToolName != toolNamePaymentStatus {
		t.Fatalf("expected second tool name %s, got %s", toolNamePaymentStatus, out.ToolCalls[1].ToolName)
	}
	if bookingSearcher.calls != 1 {
		t.Fatalf("expected one booking lookup, got %d", bookingSearcher.calls)
	}
	if paymentSearcher.calls != 1 {
		t.Fatalf("expected one payment lookup, got %d", paymentSearcher.calls)
	}
	if paymentSearcher.lastInput.BookingID != "BK-ABC123456" {
		t.Fatalf("expected booking_id BK-ABC123456, got %s", paymentSearcher.lastInput.BookingID)
	}
	if !strings.Contains(runner.lastInput.UserPrompt, toolNamePaymentStatus) {
		t.Fatalf("expected prompt to include payment tool section")
	}
	if !strings.Contains(runner.lastInput.UserPrompt, `"payments":[`) {
		t.Fatalf("expected prompt to include payment item")
	}
	if !strings.Contains(runner.lastInput.UserPrompt, "PENDING") {
		t.Fatalf("expected prompt to include payment status")
	}
}

func TestReprocessReturnsBadGatewayWhenPaymentStatusToolFails(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "draft",
			Model:              "gpt-test",
			ProviderResponseID: "resp_payment_2",
		},
	}
	bookingSearcher := &fakeBookingLookupSearcher{
		enabled: true,
		result: BookingLookupResult{
			Results: []BookingLookupItem{
				{
					ID:              "BK-ABC123456",
					Status:          "PENDING",
					ReservationCode: "ABC12345",
					CreatedAt:       time.Now().UTC(),
				},
			},
		},
	}
	paymentSearcher := &fakePaymentStatusSearcher{
		enabled: true,
		err:     errors.New("payments timeout"),
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, bookingSearcher, paymentSearcher)

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-payment-tool-2",
			IdempotencyKey:    "idem-payment-tool-2",
			Body:              "confere o pagamento da reserva ABC12345",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+ingested.Session.ID+"/reprocess", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected status %d, got %d", http.StatusBadGateway, rec.Code)
	}
	if runner.calls != 0 {
		t.Fatalf("expected no runner call after payment tool failure, got %d", runner.calls)
	}
	if len(store.toolCalls) != 2 {
		t.Fatalf("expected two stored tool calls, got %d", len(store.toolCalls))
	}
	if store.toolCalls[store.toolCallOrder[1]].ToolName != toolNamePaymentStatus {
		t.Fatalf("expected second stored tool call to be %s", toolNamePaymentStatus)
	}
}

func TestReprocessAsksPassengerCountWhenCustomerChoosesPreviousOption(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Reserva criada com sucesso. Seu codigo e ABC12345.",
			Model:              "gpt-test",
			ProviderResponseID: "resp_booking_create_1",
		},
	}
	creator := &fakeBookingCreator{
		enabled: true,
		result: BookingCreateResult{
			Mode:            "created",
			BookingID:       "BK-ABC123456",
			ReservationCode: "ABC12345",
			Status:          "PENDING",
			TotalAmount:     950,
			DepositAmount:   0,
			RemainderAmount: 950,
			Passengers: []BookingCreatePassengerResult{
				{
					Name:         "Joao Vitor Messias da Cruz Damasio",
					Document:     "06645648105",
					DocumentType: "CPF",
					Phone:        "5511999999999",
					SeatID:       "12",
				},
			},
			MessageForAgent: "Reserva criada com sucesso. Informe o codigo e siga para pagamento.",
		},
	}
	availabilitySearcher := &fakeAvailabilitySearcher{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, availabilitySearcher, creator)

	now := time.Now().UTC()
	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:       "WHATSAPP",
		ContactKey:    "5511999999999",
		CustomerPhone: "5511999999999",
		CustomerName:  "Joao Vitor Messias da Cruz Damasio",
		LastMessageAt: &now,
		LastInboundAt: &now,
	})
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        session.ID,
		Direction:        "INBOUND",
		Kind:             "TEXT",
		ProcessingStatus: "PROCESSED",
		ReceivedAt:       now.Add(-2 * time.Minute),
		Body:             "quais horarios de Videira/SC para Sao Luis/MA em 10/05?",
	}); err != nil {
		t.Fatalf("seed prior inbound: %v", err)
	}
	if _, err := store.SaveAgentDraft(context.Background(), SaveAgentDraftInput{
		SessionID:        session.ID,
		IdempotencyKey:   "draft-prev-booking-create",
		Body:             "Tenho duas opcoes para essa data.",
		SenderName:       "SHABAS",
		ProcessingStatus: messageStatusAutomationSent,
		Payload: map[string]interface{}{
			"tool_context": map[string]interface{}{
				toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(AvailabilitySearchResult{
					Filter: AvailabilitySearchInput{
						Origin:      "Videira/SC",
						Destination: "Sao Luis/MA",
						Qty:         1,
						Limit:       5,
					},
					Results: []AvailabilitySearchItem{
						{
							TripID:                 "trip-1",
							BoardStopID:            "board-1",
							AlightStopID:           "alight-1",
							OriginStopID:           "origin-1",
							DestinationStopID:      "destination-1",
							OriginDisplayName:      "Videira/SC",
							DestinationDisplayName: "Sao Luis/MA",
							OriginDepartTime:       "18:30",
							TripDate:               "2026-05-10",
							SeatsAvailable:         8,
							Price:                  950,
							Currency:               "BRL",
							Status:                 "ACTIVE",
							TripStatus:             "SCHEDULED",
						},
						{
							TripID:                 "trip-2",
							BoardStopID:            "board-2",
							AlightStopID:           "alight-2",
							OriginStopID:           "origin-2",
							DestinationStopID:      "destination-2",
							OriginDisplayName:      "Videira/SC",
							DestinationDisplayName: "Sao Luis/MA",
							OriginDepartTime:       "20:15",
							TripDate:               "2026-05-10",
							SeatsAvailable:         5,
							Price:                  980,
							Currency:               "BRL",
							Status:                 "ACTIVE",
							TripStatus:             "SCHEDULED",
						},
					},
				}),
			},
		},
		NormalizedPayload: map[string]interface{}{"mode": "AUTOMATION_DRAFT"},
		Agent:             map[string]interface{}{"status": agentStatusDraftGenerated},
		Buffer:            map[string]interface{}{},
		RecordedAt:        now.Add(-90 * time.Second),
	}); err != nil {
		t.Fatalf("seed prior draft: %v", err)
	}

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey:    "5511999999999",
		CustomerPhone: "5511999999999",
		CustomerName:  "Joao Vitor Messias da Cruz Damasio",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-booking-create-1",
			IdempotencyKey:    "idem-booking-create-1",
			Body:              "quero reservar a opcao 1. nome: Joao Vitor Messias da Cruz Damasio cpf: 06645648105",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+ingested.Session.ID+"/reprocess", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var out ReprocessResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(out.ToolCalls) != 0 {
		t.Fatalf("expected no booking tool call before passenger flow, got %d", len(out.ToolCalls))
	}
	if creator.calls != 0 {
		t.Fatalf("expected booking create not to be called before passenger flow, got %d", creator.calls)
	}
	if runner.calls != 0 {
		t.Fatalf("expected passenger-count template to avoid LLM, got %d calls", runner.calls)
	}
	if out.Draft == nil || strings.TrimSpace(out.Draft.Body) != askPassengerCountReply {
		t.Fatalf("expected passenger-count template, got %+v", out.Draft)
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskPassengerCount) {
		t.Fatalf("expected template %s, got %q", TemplateAskPassengerCount, got)
	}
}

func TestReprocessUsesPaymentCreateToolWhenCustomerAsksToGeneratePix(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Segue o PIX copia e cola para a sua reserva.",
			Model:              "gpt-test",
			ProviderResponseID: "resp_payment_create_1",
		},
	}
	bookingSearcher := &fakeBookingLookupSearcher{
		enabled: true,
		result: BookingLookupResult{
			Results: []BookingLookupItem{
				{
					ID:              "BK-ABC123456",
					Status:          "PENDING",
					ReservationCode: "ABC12345",
					TotalAmount:     950,
					DepositAmount:   0,
					RemainderAmount: 950,
					PassengerName:   "Maria Silva",
					PassengerPhone:  "48999999999",
					CreatedAt:       time.Now().UTC(),
				},
			},
		},
	}
	paymentCreator := &fakePaymentCreator{
		enabled: true,
		result: PaymentCreateResult{
			Mode:            "pix_sent",
			BookingID:       "BK-ABC123456",
			ReservationCode: "ABC12345",
			BookingStatus:   "PENDING",
			PaymentType:     "sinal",
			Stage:           "deposit",
			AmountTotal:     950,
			AmountPaid:      0,
			AmountDue:       250,
			PaymentID:       "pay-1",
			PaymentStatus:   "PENDING",
			Provider:        "PAGARME",
			ProviderRef:     "charge-1",
			PixCode:         "000201PIXCODE",
			MessageForAgent: "PIX gerado com sucesso. Envie somente o copia e cola ao cliente, sem link do provedor.",
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, bookingSearcher, paymentCreator)

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey:    "5511999999999",
		CustomerPhone: "5511999999999",
		CustomerName:  "Maria Silva",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-payment-create-1",
			IdempotencyKey:    "idem-payment-create-1",
			Body:              "gera o pix da reserva ABC12345",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+ingested.Session.ID+"/reprocess", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var out ReprocessResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(out.ToolCalls) != 2 {
		t.Fatalf("expected two tool calls, got %d", len(out.ToolCalls))
	}
	if out.ToolCalls[0].ToolName != toolNameBookingLookup {
		t.Fatalf("expected first tool name %s, got %s", toolNameBookingLookup, out.ToolCalls[0].ToolName)
	}
	if out.ToolCalls[1].ToolName != toolNamePaymentCreate {
		t.Fatalf("expected second tool name %s, got %s", toolNamePaymentCreate, out.ToolCalls[1].ToolName)
	}
	if paymentCreator.calls != 1 {
		t.Fatalf("expected one payment create call, got %d", paymentCreator.calls)
	}
	if paymentCreator.lastInput.BookingID != "BK-ABC123456" {
		t.Fatalf("expected booking_id BK-ABC123456, got %s", paymentCreator.lastInput.BookingID)
	}
	if !strings.Contains(runner.lastInput.UserPrompt, toolNamePaymentCreate) {
		t.Fatalf("expected prompt to include payment create tool section")
	}
	if !strings.Contains(runner.lastInput.UserPrompt, "000201PIXCODE") {
		t.Fatalf("expected prompt to include pix code")
	}
}

func TestReprocessUsesPaymentCreateToolFromPreviousBookingCreateContext(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Aqui esta o PIX copia e cola.",
			Model:              "gpt-test",
			ProviderResponseID: "resp_payment_create_2",
		},
	}
	paymentCreator := &fakePaymentCreator{
		enabled: true,
		result: PaymentCreateResult{
			Mode:            "pix_sent",
			BookingID:       "BK-ABC123456",
			ReservationCode: "ABC12345",
			BookingStatus:   "PENDING",
			PaymentType:     "sinal",
			Stage:           "deposit",
			AmountTotal:     950,
			AmountPaid:      0,
			AmountDue:       250,
			PaymentID:       "pay-2",
			PaymentStatus:   "PENDING",
			Provider:        "PAGARME",
			ProviderRef:     "charge-2",
			PixCode:         "000201PIXCODE2",
			MessageForAgent: "PIX gerado com sucesso. Envie somente o copia e cola ao cliente, sem link do provedor.",
		},
	}
	openAI := &fakeOpenAIInterpreter{
		enabled: true,
		result: OpenAIStructuredInterpreterRunResult{
			Interpretation: StructuredInterpretation{
				Intent:      StructuredIntentBookingCancelRequest,
				TurnMeaning: TurnMeaningNewRequest,
				Confidence:  0.99,
				Source:      "openai_structured",
			},
			ProviderResponseID: "resp_shadow_payment_divergent",
		},
	}
	availabilitySearcher := &fakeAvailabilitySearcher{enabled: true}
	svc := NewService(store, config.Config{
		ChatDebounceWindowMS:               1500,
		ChatOpenAIInterpreterShadowEnabled: true,
	}, runner, availabilitySearcher, paymentCreator, openAI)

	now := time.Now().UTC()
	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:       "WHATSAPP",
		ContactKey:    "5511999999999",
		CustomerPhone: "5511999999999",
		CustomerName:  "Maria Silva",
		LastMessageAt: &now,
		LastInboundAt: &now,
	})
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if _, err := store.SaveAgentDraft(context.Background(), SaveAgentDraftInput{
		SessionID:        session.ID,
		IdempotencyKey:   "draft-prev-payment-create",
		Body:             "Reserva criada com sucesso.",
		SenderName:       "SHABAS",
		ProcessingStatus: messageStatusAutomationSent,
		Payload: map[string]interface{}{
			"tool_context": map[string]interface{}{
				toolNameBookingCreate: buildBookingCreateResponsePayload(BookingCreateResult{
					Mode:            "created",
					BookingID:       "BK-ABC123456",
					ReservationCode: "ABC12345",
					Status:          "PENDING",
					TotalAmount:     950,
					RemainderAmount: 950,
					Passengers: []BookingCreatePassengerResult{
						{
							Name:         "Maria Silva",
							Document:     "RG123456",
							DocumentType: "RG",
							Phone:        "48999999999",
						},
					},
				}),
			},
		},
		NormalizedPayload: map[string]interface{}{"mode": "AUTOMATION_DRAFT"},
		Agent:             map[string]interface{}{"status": agentStatusDraftGenerated},
		Buffer:            map[string]interface{}{},
		RecordedAt:        now.Add(-90 * time.Second),
	}); err != nil {
		t.Fatalf("seed prior draft: %v", err)
	}

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey:    "5511999999999",
		CustomerPhone: "5511999999999",
		CustomerName:  "Maria Silva",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-payment-create-2",
			IdempotencyKey:    "idem-payment-create-2",
			Body:              "pode gerar o pix? cpf: 52998224725",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+ingested.Session.ID+"/reprocess", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var out ReprocessResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(out.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(out.ToolCalls))
	}
	if out.ToolCalls[0].ToolName != toolNamePaymentCreate {
		t.Fatalf("expected tool name %s, got %s", toolNamePaymentCreate, out.ToolCalls[0].ToolName)
	}
	if openAI.calls != 1 {
		t.Fatalf("expected OpenAI shadow to be called once, got %d", openAI.calls)
	}
	shadow := mustStructuredInterpreterShadowMap(t, out.Memory[structuredInterpreterShadowKey])
	openAIShadow := mustNestedMap(t, shadow, "openai")
	if got := asString(openAIShadow["status"]); got != string(StructuredInterpreterShadowValid) {
		t.Fatalf("expected shadow status %q, got %q", StructuredInterpreterShadowValid, got)
	}
	if paymentCreator.lastInput.BookingID != "BK-ABC123456" {
		t.Fatalf("expected booking id from previous booking create context, got %s", paymentCreator.lastInput.BookingID)
	}
	if paymentCreator.lastInput.CustomerDocument != "52998224725" {
		t.Fatalf("expected explicit cpf propagated, got %s", paymentCreator.lastInput.CustomerDocument)
	}
}

func TestReprocessUsesBookingCancelToolWhenCustomerAsksToCancelReservation(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Sua reserva foi cancelada com sucesso.",
			Model:              "gpt-test",
			ProviderResponseID: "resp_booking_cancel_1",
		},
	}
	bookingSearcher := &fakeBookingLookupSearcher{
		enabled: true,
		result: BookingLookupResult{
			Results: []BookingLookupItem{
				{
					ID:              "BK-ABC123456",
					Status:          "PENDING",
					ReservationCode: "ABC12345",
					TotalAmount:     950,
					RemainderAmount: 950,
					PassengerName:   "Maria Silva",
					PassengerPhone:  "48999999999",
					CreatedAt:       time.Now().UTC(),
				},
			},
		},
	}
	bookingCanceler := &fakeBookingCanceler{
		enabled: true,
		result: BookingCancelResult{
			Mode:            "cancel",
			BookingID:       "BK-ABC123456",
			ReservationCode: "ABC12345",
			TripID:          "trip-1",
			PreviousStatus:  "PENDING",
			BookingStatus:   "CANCELLED",
			Reason:          "customer_requested",
			Actor:           "CUSTOMER",
			PassengerCount:  1,
			MessageForAgent: "Cancelamento aplicado com sucesso. Confirme ao cliente que a reserva foi cancelada.",
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, bookingSearcher, bookingCanceler)

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey:    "5511999999999",
		CustomerPhone: "5511999999999",
		CustomerName:  "Maria Silva",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-booking-cancel-1",
			IdempotencyKey:    "idem-booking-cancel-1",
			Body:              "quero cancelar a reserva ABC12345",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+ingested.Session.ID+"/reprocess", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var out ReprocessResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(out.ToolCalls) != 2 {
		t.Fatalf("expected two tool calls, got %d", len(out.ToolCalls))
	}
	if out.ToolCalls[0].ToolName != toolNameBookingLookup {
		t.Fatalf("expected first tool name %s, got %s", toolNameBookingLookup, out.ToolCalls[0].ToolName)
	}
	if out.ToolCalls[1].ToolName != toolNameBookingCancel {
		t.Fatalf("expected second tool name %s, got %s", toolNameBookingCancel, out.ToolCalls[1].ToolName)
	}
	if bookingCanceler.calls != 1 {
		t.Fatalf("expected one booking cancel call, got %d", bookingCanceler.calls)
	}
	if bookingCanceler.lastInput.BookingID != "BK-ABC123456" {
		t.Fatalf("expected booking_id BK-ABC123456, got %s", bookingCanceler.lastInput.BookingID)
	}
	if !strings.Contains(runner.lastInput.UserPrompt, toolNameBookingCancel) {
		t.Fatalf("expected prompt to include booking cancel tool section")
	}
	if !strings.Contains(runner.lastInput.UserPrompt, "CANCELLED") {
		t.Fatalf("expected prompt to include cancelled status")
	}
}

func TestReprocessUsesBookingCancelToolFromPreviousBookingCreateContext(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Sua reserva foi cancelada.",
			Model:              "gpt-test",
			ProviderResponseID: "resp_booking_cancel_2",
		},
	}
	availabilitySearcher := &fakeAvailabilitySearcher{enabled: true}
	bookingCanceler := &fakeBookingCanceler{
		enabled: true,
		result: BookingCancelResult{
			Mode:            "cancel",
			BookingID:       "BK-ABC123456",
			ReservationCode: "ABC12345",
			TripID:          "trip-1",
			PreviousStatus:  "PENDING",
			BookingStatus:   "CANCELLED",
			Reason:          "customer_requested",
			Actor:           "CUSTOMER",
			PassengerCount:  1,
			MessageForAgent: "Cancelamento aplicado com sucesso. Confirme ao cliente que a reserva foi cancelada.",
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, availabilitySearcher, bookingCanceler)

	now := time.Now().UTC()
	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:       "WHATSAPP",
		ContactKey:    "5511999999999",
		CustomerPhone: "5511999999999",
		CustomerName:  "Maria Silva",
		LastMessageAt: &now,
		LastInboundAt: &now,
	})
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if _, err := store.SaveAgentDraft(context.Background(), SaveAgentDraftInput{
		SessionID:        session.ID,
		IdempotencyKey:   "draft-prev-booking-cancel",
		Body:             "Reserva criada com sucesso.",
		SenderName:       "SHABAS",
		ProcessingStatus: messageStatusAutomationSent,
		Payload: map[string]interface{}{
			"tool_context": map[string]interface{}{
				toolNameBookingCreate: buildBookingCreateResponsePayload(BookingCreateResult{
					Mode:            "created",
					BookingID:       "BK-ABC123456",
					ReservationCode: "ABC12345",
					Status:          "PENDING",
					TotalAmount:     950,
					RemainderAmount: 950,
				}),
			},
		},
		NormalizedPayload: map[string]interface{}{"mode": "AUTOMATION_DRAFT"},
		Agent:             map[string]interface{}{"status": agentStatusDraftGenerated},
		Buffer:            map[string]interface{}{},
		RecordedAt:        now.Add(-90 * time.Second),
	}); err != nil {
		t.Fatalf("seed prior draft: %v", err)
	}

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey:    "5511999999999",
		CustomerPhone: "5511999999999",
		CustomerName:  "Maria Silva",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-booking-cancel-2",
			IdempotencyKey:    "idem-booking-cancel-2",
			Body:              "quero cancelar minha reserva",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+ingested.Session.ID+"/reprocess", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var out ReprocessResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(out.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(out.ToolCalls))
	}
	if out.ToolCalls[0].ToolName != toolNameBookingCancel {
		t.Fatalf("expected tool name %s, got %s", toolNameBookingCancel, out.ToolCalls[0].ToolName)
	}
	if bookingCanceler.lastInput.BookingID != "BK-ABC123456" {
		t.Fatalf("expected booking id from previous booking context, got %s", bookingCanceler.lastInput.BookingID)
	}
}

func TestReprocessDraftIsIdempotentPerTurn(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Qual cidade de destino voce quer consultar?",
			Model:              "gpt-test",
			ProviderResponseID: "resp_456",
			RequestPayload:     map[string]interface{}{"model": "gpt-test"},
			ResponsePayload:    map[string]interface{}{"id": "resp_456"},
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-agent-2",
			IdempotencyKey:    "idem-agent-2",
			Body:              "quais datas para Santa Catarina?",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	path := "/chat/sessions/" + ingested.Session.ID + "/reprocess"
	firstReq := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(`{}`))
	firstRec := httptest.NewRecorder()
	r.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("expected first status %d, got %d", http.StatusOK, firstRec.Code)
	}

	secondReq := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(`{}`))
	secondRec := httptest.NewRecorder()
	r.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusOK {
		t.Fatalf("expected second status %d, got %d", http.StatusOK, secondRec.Code)
	}

	var out ReprocessResult
	if err := json.Unmarshal(secondRec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !out.Idempotent {
		t.Fatalf("expected idempotent reprocess result")
	}
	if runner.calls != 1 {
		t.Fatalf("expected one runner call, got %d", runner.calls)
	}
}

func TestReprocessReturnsBadGatewayWhenAgentRunnerFails(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, &fakeAgentRunner{
		enabled: true,
		err:     errors.New("upstream timeout"),
	})

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-agent-3",
			IdempotencyKey:    "idem-agent-3",
			Body:              "preciso saber os horarios",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}

	handler := NewHandler(svc)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+ingested.Session.ID+"/reprocess", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected status %d, got %d", http.StatusBadGateway, rec.Code)
	}
}

func TestReprocessRejectsWhenNoPendingMessagesExist(t *testing.T) {
	store := newFakeStore()
	session, _ := store.seedSessionWithMessage("5511999999999", "oi")
	_, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        session.ID,
		Direction:        "OUTBOUND",
		Kind:             "TEXT",
		ProcessingStatus: "SENT",
		ReceivedAt:       time.Now().UTC(),
		Body:             "resposta",
	})
	if err != nil {
		t.Fatalf("seed outbound message: %v", err)
	}

	handler := NewHandler(NewService(store, config.Config{ChatDebounceWindowMS: 1500}))
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/chat/sessions/"+session.ID+"/reprocess", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, rec.Code)
	}
}

func TestResolveSessionSuccess(t *testing.T) {
	store := newFakeStore()
	session, _ := store.seedSessionWithMessage("5511991111111", "ola")
	ownerUserID := uuid.NewString()
	if _, err := store.RequestHandoff(context.Background(), RequestHandoffInput{
		SessionID:      session.ID,
		AssignedUserID: ownerUserID,
	}); err != nil {
		t.Fatalf("request handoff: %v", err)
	}

	handler := NewHandler(NewService(store, config.Config{ChatDebounceWindowMS: 1500}))
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(
		http.MethodPost,
		"/chat/sessions/"+session.ID+"/resolve",
		bytes.NewBufferString(`{"resolved_by_user_id":"`+ownerUserID+`","reason":"atendimento concluido"}`),
	)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var out ResolveSessionResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if out.Session.Status != "RESOLVED" {
		t.Fatalf("expected status RESOLVED, got %s", out.Session.Status)
	}
	if out.Session.HandoffStatus != "BOT" {
		t.Fatalf("expected handoff BOT, got %s", out.Session.HandoffStatus)
	}
	if out.Session.CurrentOwnerUserID != "" {
		t.Fatalf("expected owner to be cleared")
	}
	if got := strings.TrimSpace(asString(out.Session.Metadata["resolved_by_user_id"])); got != ownerUserID {
		t.Fatalf("expected resolved_by_user_id %s, got %s", ownerUserID, got)
	}
	if got := strings.TrimSpace(asString(out.Session.Metadata["resolve_reason"])); got != "atendimento concluido" {
		t.Fatalf("expected resolve reason to be stored, got %s", got)
	}
}

func TestResolveSessionRejectsWithoutHumanOwnership(t *testing.T) {
	store := newFakeStore()
	session, _ := store.seedSessionWithMessage("5511992222222", "oi")
	ownerUserID := uuid.NewString()

	handler := NewHandler(NewService(store, config.Config{ChatDebounceWindowMS: 1500}))
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest(
		http.MethodPost,
		"/chat/sessions/"+session.ID+"/resolve",
		bytes.NewBufferString(`{"resolved_by_user_id":"`+ownerUserID+`"}`),
	)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, rec.Code)
	}
}

func TestIngestReopensResolvedSessionAfterInbound(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500})
	session, _ := store.seedSessionWithMessage("5511993333333", "bom dia")
	ownerUserID := uuid.NewString()

	if _, err := svc.RequestHandoff(context.Background(), RequestHandoffInput{
		SessionID:      session.ID,
		AssignedUserID: ownerUserID,
	}); err != nil {
		t.Fatalf("request handoff: %v", err)
	}
	if _, err := svc.ResolveSession(context.Background(), ResolveSessionInput{
		SessionID:        session.ID,
		ResolvedByUserID: ownerUserID,
		ResolveReason:    "finalizado",
	}); err != nil {
		t.Fatalf("resolve session: %v", err)
	}

	out, err := svc.Ingest(context.Background(), IngestMessageInput{
		Channel:       "WHATSAPP",
		ContactKey:    "5511993333333",
		CustomerPhone: "5511993333333",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-reopen-1",
			IdempotencyKey:    "idem-reopen-1",
			Body:              "voltei, preciso de ajuda",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}

	if out.Session.Status != "ACTIVE" {
		t.Fatalf("expected reopened status ACTIVE, got %s", out.Session.Status)
	}
	if out.Session.HandoffStatus != "BOT" {
		t.Fatalf("expected reopened handoff BOT, got %s", out.Session.HandoffStatus)
	}
	if out.Session.CurrentOwnerUserID != "" {
		t.Fatalf("expected reopened session to clear owner")
	}
}

func TestListSessionsSupportsOperationalTabs(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500})

	naoAtendido, _ := store.seedSessionWithMessage("5511980000001", "oi")
	aberto, _ := store.seedSessionWithMessage("5511980000002", "oi")
	finalizado, _ := store.seedSessionWithMessage("5511980000003", "oi")

	ownerUserID := uuid.NewString()
	if _, err := store.RequestHandoff(context.Background(), RequestHandoffInput{
		SessionID:      aberto.ID,
		AssignedUserID: ownerUserID,
	}); err != nil {
		t.Fatalf("request handoff aberto: %v", err)
	}
	if _, err := svc.ResolveSession(context.Background(), ResolveSessionInput{
		SessionID:        finalizado.ID,
		ResolvedByUserID: ownerUserID,
	}); err == nil {
		t.Fatalf("expected resolve without ownership to fail")
	}
	if _, err := store.RequestHandoff(context.Background(), RequestHandoffInput{
		SessionID:      finalizado.ID,
		AssignedUserID: ownerUserID,
	}); err != nil {
		t.Fatalf("request handoff finalizado: %v", err)
	}
	if _, err := svc.ResolveSession(context.Background(), ResolveSessionInput{
		SessionID:        finalizado.ID,
		ResolvedByUserID: ownerUserID,
		ResolveReason:    "concluido",
	}); err != nil {
		t.Fatalf("resolve finalizado: %v", err)
	}

	nonAttended, err := svc.ListSessions(context.Background(), ListSessionsFilter{
		Channel:       "WHATSAPP",
		Status:        "ACTIVE",
		HandoffStatus: "BOT",
	})
	if err != nil {
		t.Fatalf("list non attended: %v", err)
	}
	if len(nonAttended) != 1 || nonAttended[0].ID != naoAtendido.ID {
		t.Fatalf("expected only non attended session")
	}

	openSessions, err := svc.ListSessions(context.Background(), ListSessionsFilter{
		Channel:       "WHATSAPP",
		Status:        "ACTIVE",
		HandoffStatus: "HUMAN",
	})
	if err != nil {
		t.Fatalf("list open sessions: %v", err)
	}
	if len(openSessions) != 1 || openSessions[0].ID != aberto.ID {
		t.Fatalf("expected only open human session")
	}

	resolvedSessions, err := svc.ListSessions(context.Background(), ListSessionsFilter{
		Channel: "WHATSAPP",
		Status:  "RESOLVED",
	})
	if err != nil {
		t.Fatalf("list resolved sessions: %v", err)
	}
	if len(resolvedSessions) != 1 || resolvedSessions[0].ID != finalizado.ID {
		t.Fatalf("expected only resolved session")
	}
}

type fakeStore struct {
	sessions             map[string]Session
	sessionsByKey        map[string]string
	messages             map[string]Message
	messageOrder         []string
	byProviderID         map[string]string
	byIdempotencyKey     map[string]string
	handoffs             map[string]Handoff
	outbounds            map[string]ReplyOutbound
	toolCalls            map[string]ToolCall
	toolCallOrder        []string
	requestHandoffErr    error
	shadowReportMessages []Message
	shadowReportErr      error
	shadowReportCalls    int
	shadowReportFilter   StructuredInterpreterShadowReportFilter
	assistReportErr      error
	assistReportCalls    int
	assistReportFilter   StructuredInterpreterShadowReportFilter
}

type fakeProfileEnsurer struct {
	calls    int
	lastUser auth.AuthUser
	err      error
}

func (f *fakeProfileEnsurer) EnsureUserProfileIDFromAuth(_ context.Context, user auth.AuthUser) (string, error) {
	f.calls++
	f.lastUser = user
	if f.err != nil {
		return "", f.err
	}
	return user.ID, nil
}

type fakeReplySender struct {
	enabled    bool
	calls      int
	result     SendReplyResult
	results    []SendReplyResult
	err        error
	errs       []error
	beforeSend func(SendReplyInput)
}

func ingestAndReprocessShadowDraft(t *testing.T, svc *Service, contactKey string, body string) ReprocessResult {
	t.Helper()
	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: contactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-shadow-" + contactKey,
			IdempotencyKey:    "idem-shadow-" + contactKey,
			Body:              body,
		},
	})
	if err != nil {
		t.Fatalf("ingest shadow turn: %v", err)
	}
	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess shadow turn: %v", err)
	}
	return out
}

func mustStructuredInterpreterShadowMap(t *testing.T, value interface{}) map[string]interface{} {
	t.Helper()
	if value == nil {
		t.Fatalf("expected structured interpreter shadow summary")
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal structured interpreter shadow summary: %v", err)
	}
	out := map[string]interface{}{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal structured interpreter shadow summary: %v", err)
	}
	return out
}

func mustNestedMap(t *testing.T, parent map[string]interface{}, key string) map[string]interface{} {
	t.Helper()
	child := asMap(parent[key])
	if child == nil {
		t.Fatalf("expected nested map %q in %#v", key, parent)
	}
	return child
}

type fakeAgentRunner struct {
	enabled   bool
	calls     int
	result    RunAgentResult
	results   []RunAgentResult
	err       error
	errs      []error
	lastInput RunAgentInput
	inputs    []RunAgentInput
}

type fakeAvailabilitySearcher struct {
	enabled   bool
	calls     int
	result    AvailabilitySearchResult
	results   []AvailabilitySearchResult
	err       error
	errs      []error
	lastInput AvailabilitySearchInput
}

type fakePricingQuoteSearcher struct {
	enabled   bool
	calls     int
	result    PricingQuoteResult
	results   []PricingQuoteResult
	err       error
	errs      []error
	lastInput PricingQuoteInput
}

type fakeBookingLookupSearcher struct {
	enabled   bool
	calls     int
	result    BookingLookupResult
	results   []BookingLookupResult
	err       error
	errs      []error
	lastInput BookingLookupInput
}

type fakeBookingCreator struct {
	enabled   bool
	calls     int
	result    BookingCreateResult
	results   []BookingCreateResult
	err       error
	errs      []error
	lastInput BookingCreateInput
}

type fakeRescheduleAssistSearcher struct {
	enabled   bool
	calls     int
	result    RescheduleAssistResult
	results   []RescheduleAssistResult
	err       error
	errs      []error
	lastInput RescheduleAssistInput
}

type fakePaymentStatusSearcher struct {
	enabled   bool
	calls     int
	result    PaymentStatusResult
	results   []PaymentStatusResult
	err       error
	errs      []error
	lastInput PaymentStatusInput
}

type fakePaymentCreator struct {
	enabled   bool
	calls     int
	result    PaymentCreateResult
	results   []PaymentCreateResult
	err       error
	errs      []error
	lastInput PaymentCreateInput
}

type fakeChatLogger struct {
	entries []string
}

func (f *fakeChatLogger) Printf(format string, v ...interface{}) {
	f.entries = append(f.entries, fmt.Sprintf(format, v...))
}

func (f *fakeChatLogger) contains(substr string) bool {
	for _, entry := range f.entries {
		if strings.Contains(entry, substr) {
			return true
		}
	}
	return false
}

type fakeBookingCanceler struct {
	enabled   bool
	calls     int
	result    BookingCancelResult
	results   []BookingCancelResult
	err       error
	errs      []error
	lastInput BookingCancelInput
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		sessions:         map[string]Session{},
		sessionsByKey:    map[string]string{},
		messages:         map[string]Message{},
		messageOrder:     []string{},
		byProviderID:     map[string]string{},
		byIdempotencyKey: map[string]string{},
		handoffs:         map[string]Handoff{},
		outbounds:        map[string]ReplyOutbound{},
		toolCalls:        map[string]ToolCall{},
		toolCallOrder:    []string{},
	}
}

func (f *fakeReplySender) Enabled() bool {
	return f != nil && f.enabled
}

func (f *fakeReplySender) SendReply(_ context.Context, input SendReplyInput) (SendReplyResult, error) {
	if f.beforeSend != nil {
		f.beforeSend(input)
	}
	f.calls++
	if len(f.errs) > 0 {
		err := f.errs[0]
		f.errs = f.errs[1:]
		var result SendReplyResult
		if len(f.results) > 0 {
			result = f.results[0]
			f.results = f.results[1:]
		}
		if err != nil {
			return result, err
		}
		return result, nil
	}
	if f.err != nil {
		return SendReplyResult{}, f.err
	}
	return f.result, nil
}

func (f *fakeAgentRunner) Enabled() bool {
	return f != nil && f.enabled
}

func (f *fakeAgentRunner) Run(_ context.Context, input RunAgentInput) (RunAgentResult, error) {
	f.calls++
	f.lastInput = input
	f.inputs = append(f.inputs, input)
	if len(f.errs) > 0 {
		err := f.errs[0]
		f.errs = f.errs[1:]
		var result RunAgentResult
		if len(f.results) > 0 {
			result = f.results[0]
			f.results = f.results[1:]
		}
		if err != nil {
			return result, err
		}
		return result, nil
	}
	if f.err != nil {
		return RunAgentResult{}, f.err
	}
	return f.result, nil
}

func (f *fakeAvailabilitySearcher) Enabled() bool {
	return f != nil && f.enabled
}

func (f *fakeAvailabilitySearcher) Search(_ context.Context, input AvailabilitySearchInput) (AvailabilitySearchResult, error) {
	f.calls++
	f.lastInput = input
	if len(f.errs) > 0 {
		err := f.errs[0]
		f.errs = f.errs[1:]
		var result AvailabilitySearchResult
		if len(f.results) > 0 {
			result = f.results[0]
			f.results = f.results[1:]
		}
		if err != nil {
			return result, err
		}
		return result, nil
	}
	if f.err != nil {
		return AvailabilitySearchResult{}, f.err
	}
	result := f.result
	if result.Filter == (AvailabilitySearchInput{}) {
		result.Filter = input
	}
	return result, nil
}

func (f *fakePricingQuoteSearcher) Enabled() bool {
	return f != nil && f.enabled
}

func (f *fakePricingQuoteSearcher) Search(_ context.Context, input PricingQuoteInput) (PricingQuoteResult, error) {
	f.calls++
	f.lastInput = input
	if len(f.errs) > 0 {
		err := f.errs[0]
		f.errs = f.errs[1:]
		var result PricingQuoteResult
		if len(f.results) > 0 {
			result = f.results[0]
			f.results = f.results[1:]
		}
		if err != nil {
			return result, err
		}
		return result, nil
	}
	if f.err != nil {
		return PricingQuoteResult{}, f.err
	}
	return f.result, nil
}

func (f *fakeBookingLookupSearcher) Enabled() bool {
	return f != nil && f.enabled
}

func (f *fakeBookingLookupSearcher) Search(_ context.Context, input BookingLookupInput) (BookingLookupResult, error) {
	f.calls++
	f.lastInput = input
	if len(f.errs) > 0 {
		err := f.errs[0]
		f.errs = f.errs[1:]
		var result BookingLookupResult
		if len(f.results) > 0 {
			result = f.results[0]
			f.results = f.results[1:]
		}
		if err != nil {
			return result, err
		}
		return result, nil
	}
	if f.err != nil {
		return BookingLookupResult{}, f.err
	}
	return f.result, nil
}

func (f *fakeBookingCreator) Enabled() bool {
	return f != nil && f.enabled
}

func (f *fakeBookingCreator) Create(_ context.Context, input BookingCreateInput) (BookingCreateResult, error) {
	f.calls++
	f.lastInput = input
	if len(f.errs) > 0 {
		err := f.errs[0]
		f.errs = f.errs[1:]
		var result BookingCreateResult
		if len(f.results) > 0 {
			result = f.results[0]
			f.results = f.results[1:]
		}
		if err != nil {
			return result, err
		}
		return result, nil
	}
	if f.err != nil {
		return BookingCreateResult{}, f.err
	}
	return f.result, nil
}

func (f *fakePaymentStatusSearcher) Enabled() bool {
	return f != nil && f.enabled
}

func (f *fakePaymentCreator) Enabled() bool {
	return f != nil && f.enabled
}

func (f *fakeBookingCanceler) Enabled() bool {
	return f != nil && f.enabled
}

func (f *fakeRescheduleAssistSearcher) Enabled() bool {
	return f != nil && f.enabled
}

func (f *fakePaymentStatusSearcher) Search(_ context.Context, input PaymentStatusInput) (PaymentStatusResult, error) {
	f.calls++
	f.lastInput = input
	if len(f.errs) > 0 {
		err := f.errs[0]
		f.errs = f.errs[1:]
		var result PaymentStatusResult
		if len(f.results) > 0 {
			result = f.results[0]
			f.results = f.results[1:]
		}
		if err != nil {
			return result, err
		}
		return result, nil
	}
	if f.err != nil {
		return PaymentStatusResult{}, f.err
	}
	return f.result, nil
}

func (f *fakePaymentCreator) Create(_ context.Context, input PaymentCreateInput) (PaymentCreateResult, error) {
	f.calls++
	f.lastInput = input
	if len(f.errs) > 0 {
		err := f.errs[0]
		f.errs = f.errs[1:]
		var result PaymentCreateResult
		if len(f.results) > 0 {
			result = f.results[0]
			f.results = f.results[1:]
		}
		if err != nil {
			return result, err
		}
		return result, nil
	}
	if f.err != nil {
		return PaymentCreateResult{}, f.err
	}
	return f.result, nil
}

func (f *fakeBookingCanceler) Cancel(_ context.Context, input BookingCancelInput) (BookingCancelResult, error) {
	f.calls++
	f.lastInput = input
	if len(f.errs) > 0 {
		err := f.errs[0]
		f.errs = f.errs[1:]
		var result BookingCancelResult
		if len(f.results) > 0 {
			result = f.results[0]
			f.results = f.results[1:]
		}
		if err != nil {
			return result, err
		}
		return result, nil
	}
	if f.err != nil {
		return BookingCancelResult{}, f.err
	}
	return f.result, nil
}

func (f *fakeRescheduleAssistSearcher) Search(_ context.Context, input RescheduleAssistInput) (RescheduleAssistResult, error) {
	f.calls++
	f.lastInput = input
	if len(f.errs) > 0 {
		err := f.errs[0]
		f.errs = f.errs[1:]
		var result RescheduleAssistResult
		if len(f.results) > 0 {
			result = f.results[0]
			f.results = f.results[1:]
		}
		if err != nil {
			return result, err
		}
		return result, nil
	}
	if f.err != nil {
		return RescheduleAssistResult{}, f.err
	}
	return f.result, nil
}

func (s *fakeStore) FindMessageByKeys(_ context.Context, providerMessageID string, idempotencyKey string) (*Message, error) {
	if providerMessageID != "" {
		if id, ok := s.byProviderID[providerMessageID]; ok {
			item := s.messages[id]
			return &item, nil
		}
	}
	if idempotencyKey != "" {
		if id, ok := s.byIdempotencyKey[idempotencyKey]; ok {
			item := s.messages[id]
			return &item, nil
		}
	}
	return nil, nil
}

func (s *fakeStore) UpsertSession(_ context.Context, input UpsertSessionInput) (Session, error) {
	key := input.Channel + "::" + input.ContactKey
	if existingID, ok := s.sessionsByKey[key]; ok {
		item := s.sessions[existingID]
		if strings.EqualFold(strings.TrimSpace(item.Status), "RESOLVED") && input.LastInboundAt != nil {
			item.Status = "ACTIVE"
			item.HandoffStatus = "BOT"
			item.CurrentOwnerUserID = ""
		}
		if input.CustomerPhone != "" {
			item.CustomerPhone = input.CustomerPhone
		}
		if input.CustomerName != "" {
			item.CustomerName = input.CustomerName
		}
		if input.LastMessageAt != nil {
			item.LastMessageAt = input.LastMessageAt
		}
		if input.LastInboundAt != nil {
			item.LastInboundAt = input.LastInboundAt
		}
		if input.LastOutboundAt != nil {
			item.LastOutboundAt = input.LastOutboundAt
		}
		if len(input.Metadata) > 0 {
			if item.Metadata == nil {
				item.Metadata = map[string]interface{}{}
			}
			for k, v := range input.Metadata {
				item.Metadata[k] = v
			}
		}
		item.UpdatedAt = time.Now().UTC()
		s.sessions[item.ID] = item
		return item, nil
	}

	now := time.Now().UTC()
	item := Session{
		ID:             uuid.NewString(),
		Channel:        input.Channel,
		ContactKey:     input.ContactKey,
		CustomerPhone:  input.CustomerPhone,
		CustomerName:   input.CustomerName,
		Status:         "ACTIVE",
		HandoffStatus:  "BOT",
		LastMessageAt:  input.LastMessageAt,
		LastInboundAt:  input.LastInboundAt,
		LastOutboundAt: input.LastOutboundAt,
		Metadata:       map[string]interface{}{},
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	for k, v := range input.Metadata {
		item.Metadata[k] = v
	}

	s.sessions[item.ID] = item
	s.sessionsByKey[key] = item.ID
	return item, nil
}

func (s *fakeStore) CreateMessage(_ context.Context, input CreateMessageInput) (Message, error) {
	now := time.Now().UTC()
	item := Message{
		ID:                uuid.NewString(),
		SessionID:         input.SessionID,
		Direction:         input.Direction,
		Kind:              input.Kind,
		ProviderMessageID: input.ProviderMessageID,
		IdempotencyKey:    input.IdempotencyKey,
		SenderName:        input.SenderName,
		SenderPhone:       input.SenderPhone,
		Body:              input.Body,
		Payload:           input.Payload,
		NormalizedPayload: input.NormalizedPayload,
		ProcessingStatus:  input.ProcessingStatus,
		ReceivedAt:        input.ReceivedAt,
		SentAt:            input.SentAt,
		CreatedAt:         now,
	}

	s.messages[item.ID] = item
	s.messageOrder = append(s.messageOrder, item.ID)
	if item.ProviderMessageID != "" {
		s.byProviderID[item.ProviderMessageID] = item.ID
	}
	if item.IdempotencyKey != "" {
		s.byIdempotencyKey[item.IdempotencyKey] = item.ID
	}

	return item, nil
}

func (s *fakeStore) UpdateMessage(_ context.Context, input UpdateMessageInput) (Message, error) {
	item, ok := s.messages[input.MessageID]
	if !ok {
		return Message{}, ErrSessionNotFound
	}
	if input.Body != "" {
		item.Body = input.Body
	}
	if item.NormalizedPayload == nil {
		item.NormalizedPayload = map[string]interface{}{}
	}
	for key, value := range input.NormalizedPayload {
		item.NormalizedPayload[key] = value
	}
	item.ProcessingStatus = input.ProcessingStatus
	s.messages[item.ID] = item
	return item, nil
}

func (s *fakeStore) CreateToolCall(_ context.Context, input CreateToolCallInput) (ToolCall, error) {
	now := time.Now().UTC()
	item := ToolCall{
		ID:              uuid.NewString(),
		SessionID:       input.SessionID,
		MessageID:       input.MessageID,
		ToolName:        input.ToolName,
		RequestPayload:  input.RequestPayload,
		ResponsePayload: input.ResponsePayload,
		Status:          input.Status,
		ErrorCode:       input.ErrorCode,
		ErrorMessage:    input.ErrorMessage,
		StartedAt:       input.StartedAt,
		FinishedAt:      input.FinishedAt,
		CreatedAt:       now,
	}
	s.toolCalls[item.ID] = item
	s.toolCallOrder = append(s.toolCallOrder, item.ID)
	return item, nil
}

func (s *fakeStore) UpdateSessionBufferState(_ context.Context, input UpdateSessionBufferStateInput) (Session, error) {
	item := s.sessions[input.SessionID]
	if item.Metadata == nil {
		item.Metadata = map[string]interface{}{}
	}
	item.Metadata["buffer"] = input.Buffer
	item.UpdatedAt = time.Now().UTC()
	s.sessions[item.ID] = item
	return item, nil
}

func (s *fakeStore) UpdateSessionMetadata(_ context.Context, input UpdateSessionMetadataInput) (Session, error) {
	item := s.sessions[input.SessionID]
	if item.Metadata == nil {
		item.Metadata = map[string]interface{}{}
	}
	for key, value := range input.Metadata {
		item.Metadata[key] = value
	}
	item.UpdatedAt = time.Now().UTC()
	s.sessions[item.ID] = item
	return item, nil
}

func (s *fakeStore) RequestHandoff(_ context.Context, input RequestHandoffInput) (RequestHandoffResult, error) {
	if s.requestHandoffErr != nil {
		return RequestHandoffResult{}, s.requestHandoffErr
	}
	item, ok := s.sessions[input.SessionID]
	if !ok {
		return RequestHandoffResult{}, ErrSessionNotFound
	}

	now := time.Now().UTC()
	handoff := Handoff{
		ID:             uuid.NewString(),
		SessionID:      input.SessionID,
		RequestedBy:    input.RequestedBy,
		Reason:         input.Reason,
		Status:         "REQUESTED",
		AssignedUserID: input.AssignedUserID,
		RequestedAt:    now,
		Metadata:       map[string]interface{}{},
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	for key, value := range input.Metadata {
		handoff.Metadata[key] = value
	}
	s.handoffs[handoff.ID] = handoff

	item.HandoffStatus = "HUMAN_REQUESTED"
	item.CurrentOwnerUserID = ""
	if input.AssignedUserID != "" {
		item.HandoffStatus = "HUMAN"
		item.CurrentOwnerUserID = input.AssignedUserID
	}
	item.UpdatedAt = now
	s.sessions[item.ID] = item

	return RequestHandoffResult{
		Session: item,
		Handoff: handoff,
	}, nil
}

func (s *fakeStore) ResumeSession(_ context.Context, input ResumeSessionInput) (ResumeSessionResult, error) {
	item, ok := s.sessions[input.SessionID]
	if !ok {
		return ResumeSessionResult{}, ErrSessionNotFound
	}

	var selected Handoff
	found := false
	for _, handoff := range s.handoffs {
		if handoff.SessionID == input.SessionID && handoff.Status == "REQUESTED" && handoff.ResolvedAt == nil {
			if !found || handoff.RequestedAt.After(selected.RequestedAt) {
				selected = handoff
				found = true
			}
		}
	}
	if !found {
		return ResumeSessionResult{}, ErrNoActiveHandoff
	}

	now := time.Now().UTC()
	if selected.Metadata == nil {
		selected.Metadata = map[string]interface{}{}
	}
	for key, value := range input.Metadata {
		selected.Metadata[key] = value
	}
	if input.ResumedBy != "" {
		selected.Metadata["resumed_by"] = input.ResumedBy
	}
	if input.Reason != "" {
		selected.Metadata["resume_reason"] = input.Reason
	}
	selected.Status = "RESOLVED"
	selected.ResolvedAt = &now
	selected.UpdatedAt = now
	s.handoffs[selected.ID] = selected

	item.HandoffStatus = "BOT"
	item.CurrentOwnerUserID = ""
	item.UpdatedAt = now
	s.sessions[item.ID] = item

	return ResumeSessionResult{
		Session: item,
		Handoff: selected,
	}, nil
}

func (s *fakeStore) ResolveSession(_ context.Context, input ResolveSessionInput) (ResolveSessionResult, error) {
	item, ok := s.sessions[input.SessionID]
	if !ok {
		return ResolveSessionResult{}, ErrSessionNotFound
	}
	if strings.EqualFold(strings.TrimSpace(item.Status), "RESOLVED") {
		return ResolveSessionResult{}, ErrSessionAlreadyResolved
	}
	if item.HandoffStatus != "HUMAN" || strings.TrimSpace(item.CurrentOwnerUserID) == "" {
		return ResolveSessionResult{}, ErrResolveRequiresHuman
	}
	if strings.TrimSpace(item.CurrentOwnerUserID) != strings.TrimSpace(input.ResolvedByUserID) {
		return ResolveSessionResult{}, ErrResolveOwnerMismatch
	}

	if item.Metadata == nil {
		item.Metadata = map[string]interface{}{}
	}
	for key, value := range input.Metadata {
		item.Metadata[key] = value
	}

	now := time.Now().UTC()
	item.Metadata["resolved_at"] = now.Format(time.RFC3339Nano)
	item.Metadata["resolved_by_user_id"] = strings.TrimSpace(input.ResolvedByUserID)
	if reason := strings.TrimSpace(input.ResolveReason); reason != "" {
		item.Metadata["resolve_reason"] = reason
	}
	item.Status = "RESOLVED"
	item.HandoffStatus = "BOT"
	item.CurrentOwnerUserID = ""
	item.UpdatedAt = now
	s.sessions[item.ID] = item

	return ResolveSessionResult{
		Session: item,
		Status:  "resolved",
		Reason:  "manual_resolve",
	}, nil
}

func (s *fakeStore) FindReplyByIdempotency(_ context.Context, sessionID string, idempotencyKey string) (*ReplyResult, error) {
	messageID, ok := s.byIdempotencyKey[idempotencyKey]
	if !ok {
		return nil, nil
	}
	message := s.messages[messageID]
	if message.SessionID != sessionID || message.Direction != "OUTBOUND" {
		return nil, nil
	}
	session := s.sessions[sessionID]
	for _, outbound := range s.outbounds {
		if outbound.SessionID == sessionID && outbound.IdempotencyKey == idempotencyKey {
			result := ReplyResult{
				Session:  session,
				Message:  message,
				Outbound: outbound,
			}
			return &result, nil
		}
	}
	return nil, nil
}

func (s *fakeStore) CreateReply(_ context.Context, input ReplyInput, debounceWindow time.Duration) (ReplyResult, error) {
	session, ok := s.sessions[input.SessionID]
	if !ok {
		return ReplyResult{}, ErrSessionNotFound
	}

	now := time.Now().UTC()
	replyBody := strings.TrimSpace(input.Body)
	replyMode := "ASSISTED_REPLY"
	reviewAction := ""
	replyKind := "TEXT"
	if mediaKind := strings.ToUpper(strings.TrimSpace(asString(input.Metadata["media_kind"]))); mediaKind == "IMAGE" || mediaKind == "AUDIO" || mediaKind == "DOCUMENT" {
		replyKind = mediaKind
	}
	var reviewedDraft *Message
	if input.DraftMessageID != "" {
		draft, ok := s.messages[input.DraftMessageID]
		if !ok || draft.SessionID != input.SessionID || draft.Direction != "OUTBOUND" || !strings.EqualFold(draft.ProcessingStatus, messageStatusAutomationDraft) {
			return ReplyResult{}, ErrReplyDraftNotAllowed
		}
		if replyBody == "" {
			replyBody = strings.TrimSpace(draft.Body)
		}
		if replyBody == "" {
			return ReplyResult{}, ErrReplyBodyRequired
		}
		reviewAction = "APPROVED_AS_IS"
		if strings.TrimSpace(replyBody) != strings.TrimSpace(draft.Body) {
			reviewAction = "EDITED"
		}
		if draft.Payload == nil {
			draft.Payload = map[string]interface{}{}
		}
		if draft.NormalizedPayload == nil {
			draft.NormalizedPayload = map[string]interface{}{}
		}
		draft.Payload["review_mode"] = "CONTROLLED"
		draft.Payload["review_status"] = "APPROVED"
		draft.Payload["review_action"] = reviewAction
		draft.Payload["reviewed_at"] = now.Format(time.RFC3339Nano)
		draft.Payload["reviewed_by_user_id"] = input.OwnerUserID
		draft.NormalizedPayload["review_mode"] = "CONTROLLED"
		draft.NormalizedPayload["review_status"] = "APPROVED"
		draft.NormalizedPayload["review_action"] = reviewAction
		draft.NormalizedPayload["reviewed_at"] = now.Format(time.RFC3339Nano)
		draft.NormalizedPayload["reviewed_by_user_id"] = input.OwnerUserID
		draft.ProcessingStatus = messageStatusAutomationReviewed
		s.messages[draft.ID] = draft
		reviewedDraft = &draft
		replyMode = "DRAFT_REVIEW"
	}

	message := Message{
		ID:             uuid.NewString(),
		SessionID:      input.SessionID,
		Direction:      "OUTBOUND",
		Kind:           replyKind,
		IdempotencyKey: input.IdempotencyKey,
		SenderName:     input.SenderName,
		Body:           replyBody,
		Payload: map[string]interface{}{
			"mode":          replyMode,
			"shadow_mode":   true,
			"owner_user_id": input.OwnerUserID,
			"sender_name":   input.SenderName,
		},
		NormalizedPayload: map[string]interface{}{
			"mode":          replyMode,
			"shadow_mode":   true,
			"owner_user_id": input.OwnerUserID,
			"sender_name":   input.SenderName,
		},
		ProcessingStatus: "MANUAL_PENDING",
		ReceivedAt:       now,
		CreatedAt:        now,
	}
	if reviewedDraft != nil {
		message.Payload["draft_message_id"] = reviewedDraft.ID
		message.Payload["review_mode"] = "CONTROLLED"
		message.Payload["review_action"] = reviewAction
		message.Payload["draft_reviewed"] = true
		message.NormalizedPayload["draft_message_id"] = reviewedDraft.ID
		message.NormalizedPayload["review_mode"] = "CONTROLLED"
		message.NormalizedPayload["review_action"] = reviewAction
		message.NormalizedPayload["draft_reviewed"] = true
	}
	for key, value := range input.Metadata {
		message.Payload[key] = value
		message.NormalizedPayload[key] = value
	}
	s.messages[message.ID] = message
	s.messageOrder = append(s.messageOrder, message.ID)
	s.byIdempotencyKey[message.IdempotencyKey] = message.ID

	if session.Metadata == nil {
		session.Metadata = map[string]interface{}{}
	}
	session.LastMessageAt = &now
	session.LastOutboundAt = &now
	session.Metadata["buffer"] = buildBufferState(session.Metadata, message, debounceWindow)
	if reviewedDraft != nil {
		session.Metadata["agent"] = buildDraftReviewedAgentState(session.Metadata, *reviewedDraft, input.OwnerUserID, reviewAction, now)
	}
	session.UpdatedAt = now
	s.sessions[session.ID] = session

	outbound := ReplyOutbound{
		ID:             uuid.NewString(),
		SessionID:      session.ID,
		Channel:        session.Channel,
		Recipient:      session.ContactKey,
		Payload:        map[string]interface{}{"body": replyBody, "owner_user_id": input.OwnerUserID, "mode": replyMode, "shadow_mode": true, "sender_name": input.SenderName, "message_id": message.ID},
		Provider:       "EVOLUTION",
		IdempotencyKey: input.IdempotencyKey,
		Status:         "MANUAL_PENDING",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if reviewedDraft != nil {
		outbound.Payload["draft_message_id"] = reviewedDraft.ID
		outbound.Payload["review_mode"] = "CONTROLLED"
		outbound.Payload["review_action"] = reviewAction
		outbound.Payload["draft_reviewed"] = true
	}
	for key, value := range input.Metadata {
		outbound.Payload[key] = value
	}
	s.outbounds[outbound.ID] = outbound

	return ReplyResult{
		Session:  session,
		Message:  message,
		Outbound: outbound,
		Draft:    reviewedDraft,
	}, nil
}

func (s *fakeStore) CreateAutomationReply(_ context.Context, input CreateAutomationReplyInput, debounceWindow time.Duration) (ReplyResult, error) {
	session, ok := s.sessions[input.SessionID]
	if !ok {
		return ReplyResult{}, ErrSessionNotFound
	}
	if session.HandoffStatus != "BOT" || strings.TrimSpace(session.CurrentOwnerUserID) != "" {
		return ReplyResult{}, ErrReprocessRequiresBot
	}

	draft, ok := s.messages[input.DraftMessageID]
	if !ok || draft.SessionID != input.SessionID || draft.Direction != "OUTBOUND" || !strings.EqualFold(draft.ProcessingStatus, messageStatusAutomationDraft) {
		return ReplyResult{}, ErrReplyDraftNotAllowed
	}

	now := time.Now().UTC()
	senderName := strings.TrimSpace(input.SenderName)
	if senderName == "" {
		senderName = firstNonEmpty(strings.TrimSpace(draft.SenderName), "SHABAS")
	}

	message := Message{
		ID:             uuid.NewString(),
		SessionID:      input.SessionID,
		Direction:      "OUTBOUND",
		Kind:           "TEXT",
		IdempotencyKey: input.IdempotencyKey,
		SenderName:     senderName,
		Body:           strings.TrimSpace(draft.Body),
		Payload: map[string]interface{}{
			"mode":             "BOT_AUTO_REPLY",
			"draft_message_id": draft.ID,
			"draft_auto_sent":  true,
			"sender_name":      senderName,
			"auto_send_status": firstNonEmpty(asString(draft.NormalizedPayload["auto_send_status"]), asString(draft.Payload["auto_send_status"]), draftAutoSendStatusEligible),
		},
		NormalizedPayload: map[string]interface{}{
			"mode":             "BOT_AUTO_REPLY",
			"draft_message_id": draft.ID,
			"draft_auto_sent":  true,
			"sender_name":      senderName,
			"auto_send_status": firstNonEmpty(asString(draft.NormalizedPayload["auto_send_status"]), asString(draft.Payload["auto_send_status"]), draftAutoSendStatusEligible),
		},
		ProcessingStatus: "AUTOMATION_PENDING",
		ReceivedAt:       now,
		CreatedAt:        now,
	}
	if reasons := firstNonEmptyStringSlice(asStringSlice(draft.NormalizedPayload["auto_send_reasons"]), asStringSlice(draft.Payload["auto_send_reasons"])); len(reasons) > 0 {
		message.Payload["auto_send_reasons"] = reasons
		message.NormalizedPayload["auto_send_reasons"] = reasons
	}
	for key, value := range input.Metadata {
		message.Payload[key] = value
		message.NormalizedPayload[key] = value
	}
	s.messages[message.ID] = message
	s.messageOrder = append(s.messageOrder, message.ID)
	s.byIdempotencyKey[message.IdempotencyKey] = message.ID

	if session.Metadata == nil {
		session.Metadata = map[string]interface{}{}
	}
	session.LastMessageAt = &now
	session.LastOutboundAt = &now
	session.Metadata["buffer"] = buildBufferState(session.Metadata, message, debounceWindow)
	session.UpdatedAt = now
	s.sessions[session.ID] = session

	outbound := ReplyOutbound{
		ID:             uuid.NewString(),
		SessionID:      session.ID,
		Channel:        session.Channel,
		Recipient:      session.ContactKey,
		Payload:        map[string]interface{}{"body": message.Body, "mode": "BOT_AUTO_REPLY", "draft_message_id": draft.ID, "draft_auto_sent": true, "sender_name": senderName, "message_id": message.ID, "auto_send_status": message.Payload["auto_send_status"]},
		Provider:       "EVOLUTION",
		IdempotencyKey: input.IdempotencyKey,
		Status:         "AUTOMATION_PENDING",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if reasons, ok := message.Payload["auto_send_reasons"]; ok {
		outbound.Payload["auto_send_reasons"] = reasons
	}
	for key, value := range input.Metadata {
		outbound.Payload[key] = value
	}
	s.outbounds[outbound.ID] = outbound

	return ReplyResult{
		Session:  session,
		Message:  message,
		Outbound: outbound,
		Draft:    &draft,
	}, nil
}

func (s *fakeStore) UpdateDraftAutoSendState(_ context.Context, input UpdateDraftAutoSendStateInput) (SaveAgentDraftResult, error) {
	session, ok := s.sessions[input.SessionID]
	if !ok {
		return SaveAgentDraftResult{}, ErrSessionNotFound
	}

	draft, ok := s.messages[input.DraftMessageID]
	if !ok || draft.SessionID != input.SessionID || draft.Direction != "OUTBOUND" || !isAutomationDraftStatus(draft.ProcessingStatus) {
		return SaveAgentDraftResult{}, ErrReplyDraftNotAllowed
	}

	if draft.Payload == nil {
		draft.Payload = map[string]interface{}{}
	}
	if draft.NormalizedPayload == nil {
		draft.NormalizedPayload = map[string]interface{}{}
	}
	for key, value := range input.Payload {
		draft.Payload[key] = value
		draft.NormalizedPayload[key] = value
	}
	draft.Payload["auto_send_status"] = input.AutoSendStatus
	draft.NormalizedPayload["auto_send_status"] = input.AutoSendStatus
	if reasons := mergeDistinctStrings(readDraftAutoSendReasons(draft), input.AutoSendReasons...); len(reasons) > 0 {
		draft.Payload["auto_send_reasons"] = reasons
		draft.NormalizedPayload["auto_send_reasons"] = reasons
	}
	s.messages[draft.ID] = draft

	if len(input.Agent) > 0 {
		if session.Metadata == nil {
			session.Metadata = map[string]interface{}{}
		}
		providerResponseID := strings.TrimSpace(asString(input.Agent["provider_response_id"]))
		providerConversationID := strings.TrimSpace(asString(input.Agent["provider_conversation_id"]))
		if providerResponseID != "" {
			session.Metadata["provider_response_id"] = providerResponseID
		}
		if providerConversationID != "" {
			session.Metadata["provider_conversation_id"] = providerConversationID
		}
		if providerResponseID != "" || providerConversationID != "" {
			if providerModel := strings.TrimSpace(asString(input.Agent["draft_model"])); providerModel != "" {
				session.Metadata["provider_model"] = providerModel
			} else if providerModel := strings.TrimSpace(asString(input.Agent["model"])); providerModel != "" {
				session.Metadata["provider_model"] = providerModel
			}
		}
		session.Metadata["agent"] = input.Agent
	}
	session.UpdatedAt = time.Now().UTC()
	s.sessions[session.ID] = session

	return SaveAgentDraftResult{
		Session: session,
		Message: draft,
	}, nil
}

func (s *fakeStore) MarkReplyDeliverySent(_ context.Context, input MarkReplyDeliverySentInput) (ReplyResult, error) {
	session := s.sessions[input.SessionID]
	message := s.messages[input.MessageID]
	outbound := s.outbounds[input.OutboundID]

	if message.NormalizedPayload == nil {
		message.NormalizedPayload = map[string]interface{}{}
	}
	for key, value := range input.Payload {
		message.NormalizedPayload[key] = value
	}
	message.ProviderMessageID = input.ProviderMessageID
	message.ProcessingStatus = input.ProviderStatus
	message.SentAt = timePointer(input.SentAt)
	s.messages[message.ID] = message
	if input.ProviderMessageID != "" {
		s.byProviderID[input.ProviderMessageID] = message.ID
	}

	if outbound.Payload == nil {
		outbound.Payload = map[string]interface{}{}
	}
	for key, value := range input.Payload {
		outbound.Payload[key] = value
	}
	outbound.ProviderMessageID = input.ProviderMessageID
	outbound.Status = input.ProviderStatus
	outbound.SentAt = timePointer(input.SentAt)
	outbound.UpdatedAt = time.Now().UTC()
	s.outbounds[outbound.ID] = outbound

	var draft *Message
	if strings.EqualFold(asString(message.Payload["mode"]), "BOT_AUTO_REPLY") {
		if draftID := strings.TrimSpace(firstNonEmpty(asString(message.Payload["draft_message_id"]), asString(message.NormalizedPayload["draft_message_id"]))); draftID != "" {
			item := s.messages[draftID]
			if item.Payload == nil {
				item.Payload = map[string]interface{}{}
			}
			if item.NormalizedPayload == nil {
				item.NormalizedPayload = map[string]interface{}{}
			}
			item.Payload["auto_sent"] = true
			item.Payload["auto_send_status"] = draftAutoSendStatusEligible
			item.Payload["auto_sent_at"] = input.SentAt.UTC().Format(time.RFC3339Nano)
			item.Payload["auto_sent_message_id"] = message.ID
			item.Payload["auto_sent_outbound_id"] = outbound.ID
			item.Payload["auto_sent_provider_message_id"] = input.ProviderMessageID
			item.Payload["auto_send_last_error_text"] = nil
			item.Payload["auto_send_retry_pending_at"] = nil
			item.NormalizedPayload["auto_sent"] = true
			item.NormalizedPayload["auto_send_status"] = draftAutoSendStatusEligible
			item.NormalizedPayload["auto_sent_at"] = input.SentAt.UTC().Format(time.RFC3339Nano)
			item.NormalizedPayload["auto_sent_message_id"] = message.ID
			item.NormalizedPayload["auto_sent_outbound_id"] = outbound.ID
			item.NormalizedPayload["auto_sent_provider_message_id"] = input.ProviderMessageID
			item.NormalizedPayload["auto_send_last_error_text"] = nil
			item.NormalizedPayload["auto_send_retry_pending_at"] = nil
			item.ProcessingStatus = messageStatusAutomationSent
			s.messages[item.ID] = item
			draft = &item

			if session.Metadata == nil {
				session.Metadata = map[string]interface{}{}
			}
			session.Metadata["agent"] = buildDraftAutoSentAgentState(session.Metadata, item, message, outbound, input.SentAt)
			s.sessions[session.ID] = session
		}
	}

	return ReplyResult{
		Session:  session,
		Message:  message,
		Outbound: outbound,
		Draft:    draft,
	}, nil
}

func (s *fakeStore) MarkReplyDeliveryFailure(_ context.Context, input MarkReplyDeliveryFailureInput) (ReplyResult, error) {
	session := s.sessions[input.SessionID]
	message := s.messages[input.MessageID]
	outbound := s.outbounds[input.OutboundID]

	if message.NormalizedPayload == nil {
		message.NormalizedPayload = map[string]interface{}{}
	}
	message.NormalizedPayload["delivery_error_text"] = input.ErrorText
	message.ProcessingStatus = "SEND_FAILED"
	s.messages[message.ID] = message

	if outbound.Payload == nil {
		outbound.Payload = map[string]interface{}{}
	}
	outbound.Payload["delivery_error_text"] = input.ErrorText
	outbound.Status = "SEND_FAILED"
	outbound.UpdatedAt = time.Now().UTC()
	s.outbounds[outbound.ID] = outbound

	var draft *Message
	if strings.EqualFold(asString(message.Payload["mode"]), "BOT_AUTO_REPLY") {
		if draftID := strings.TrimSpace(firstNonEmpty(asString(message.Payload["draft_message_id"]), asString(message.NormalizedPayload["draft_message_id"]))); draftID != "" {
			item := s.messages[draftID]
			if item.Payload == nil {
				item.Payload = map[string]interface{}{}
			}
			if item.NormalizedPayload == nil {
				item.NormalizedPayload = map[string]interface{}{}
			}
			observedAt := time.Now().UTC()
			reasons := mergeDistinctStrings(readDraftAutoSendReasons(item), draftAutoSendReasonDeliveryFail)
			item.Payload["auto_send_status"] = draftAutoSendStatusRetryPending
			item.Payload["auto_send_reasons"] = reasons
			item.Payload["auto_send_last_attempt_at"] = observedAt.Format(time.RFC3339Nano)
			item.Payload["auto_send_retry_pending_at"] = observedAt.Format(time.RFC3339Nano)
			item.Payload["auto_send_last_error_text"] = input.ErrorText
			item.Payload["auto_send_last_reply_message_id"] = message.ID
			item.Payload["auto_send_last_outbound_id"] = outbound.ID
			item.NormalizedPayload["auto_send_status"] = draftAutoSendStatusRetryPending
			item.NormalizedPayload["auto_send_reasons"] = reasons
			item.NormalizedPayload["auto_send_last_attempt_at"] = observedAt.Format(time.RFC3339Nano)
			item.NormalizedPayload["auto_send_retry_pending_at"] = observedAt.Format(time.RFC3339Nano)
			item.NormalizedPayload["auto_send_last_error_text"] = input.ErrorText
			item.NormalizedPayload["auto_send_last_reply_message_id"] = message.ID
			item.NormalizedPayload["auto_send_last_outbound_id"] = outbound.ID
			s.messages[item.ID] = item
			draft = &item

			if session.Metadata == nil {
				session.Metadata = map[string]interface{}{}
			}
			session.Metadata["agent"] = buildDraftAutoSendRetryPendingAgentState(session.Metadata, item, message, outbound, input.ErrorText, observedAt)
			s.sessions[session.ID] = session
		}
	}

	return ReplyResult{
		Session:  session,
		Message:  message,
		Outbound: outbound,
		Draft:    draft,
	}, nil
}

func (s *fakeStore) SaveReprocessSnapshot(_ context.Context, input SaveReprocessSnapshotInput) (SaveReprocessSnapshotResult, error) {
	session := s.sessions[input.SessionID]
	if session.Metadata == nil {
		session.Metadata = map[string]interface{}{}
	}
	session.Metadata["memory"] = input.Memory
	session.Metadata["agent"] = input.Agent
	session.Metadata["buffer"] = input.Buffer
	session.UpdatedAt = time.Now().UTC()
	s.sessions[session.ID] = session

	updated := make([]Message, 0, len(input.MessageIDs))
	for _, messageID := range input.MessageIDs {
		message := s.messages[messageID]
		if message.NormalizedPayload == nil {
			message.NormalizedPayload = map[string]interface{}{}
		}
		for key, value := range input.MessageMetadata {
			message.NormalizedPayload[key] = value
		}
		message.ProcessingStatus = input.MessageStatus
		s.messages[message.ID] = message
		updated = append(updated, message)
	}

	return SaveReprocessSnapshotResult{
		Session:  session,
		Messages: updated,
	}, nil
}

func (s *fakeStore) SaveAgentDraft(_ context.Context, input SaveAgentDraftInput) (SaveAgentDraftResult, error) {
	session := s.sessions[input.SessionID]
	if session.Metadata == nil {
		session.Metadata = map[string]interface{}{}
	}
	session.Metadata["agent"] = input.Agent
	session.Metadata["buffer"] = input.Buffer
	session.UpdatedAt = time.Now().UTC()
	s.sessions[session.ID] = session

	message := Message{
		ID:                uuid.NewString(),
		SessionID:         input.SessionID,
		Direction:         "OUTBOUND",
		Kind:              "TEXT",
		IdempotencyKey:    input.IdempotencyKey,
		SenderName:        input.SenderName,
		Body:              input.Body,
		Payload:           input.Payload,
		NormalizedPayload: input.NormalizedPayload,
		ProcessingStatus:  input.ProcessingStatus,
		ReceivedAt:        input.RecordedAt,
		CreatedAt:         input.RecordedAt,
	}
	s.messages[message.ID] = message
	s.messageOrder = append(s.messageOrder, message.ID)
	s.byIdempotencyKey[message.IdempotencyKey] = message.ID

	return SaveAgentDraftResult{
		Session: session,
		Message: message,
	}, nil
}

func (s *fakeStore) ListSessions(_ context.Context, filter ListSessionsFilter) ([]Session, error) {
	items := []Session{}
	reviewSLASeconds := filter.ReviewSLASeconds
	if reviewSLASeconds <= 0 {
		reviewSLASeconds = 15 * 60
	}
	for _, item := range s.sessions {
		if filter.Channel != "" && item.Channel != filter.Channel {
			continue
		}
		if filter.Status != "" && item.Status != filter.Status {
			continue
		}
		if filter.HandoffStatus != "" && item.HandoffStatus != filter.HandoffStatus {
			continue
		}
		if filter.ContactKey != "" && item.ContactKey != filter.ContactKey {
			continue
		}
		agent := asMap(item.Metadata["agent"])
		agentStatus := strings.ToUpper(strings.TrimSpace(asString(agent["status"])))
		if filter.AgentStatus != "" && agentStatus != filter.AgentStatus {
			continue
		}
		autoSendStatus := strings.ToUpper(strings.TrimSpace(asString(agent["auto_send_status"])))
		if filter.DraftAutoSendStatus != "" && autoSendStatus != filter.DraftAutoSendStatus {
			continue
		}
		switch filter.DraftReviewStatus {
		case "PENDING_REVIEW":
			if agentStatus != agentStatusDraftGenerated {
				continue
			}
		case "REVIEWED":
			if agentStatus != agentStatusDraftReviewed {
				continue
			}
		}
		items = append(items, item)
	}
	observedAt := time.Now().UTC()
	sort.Slice(items, func(i, j int) bool {
		if filter.OrderBy == "REVIEW_PRIORITY" {
			priority := func(session Session) int {
				decorated := decorateSessionDraftSummary(session, reviewSLASeconds, observedAt)
				switch decorated.DraftReviewPriority {
				case "HIGH":
					return 0
				case "MEDIUM":
					return 1
				case "LOW":
					return 2
				case "REVIEWED":
					return 3
				default:
					if decorated.DraftReviewStatus == "REVIEWED" {
						return 3
					}
					return 4
				}
			}
			left := priority(items[i])
			right := priority(items[j])
			if left != right {
				return left < right
			}
			leftDecorated := decorateSessionDraftSummary(items[i], reviewSLASeconds, observedAt)
			rightDecorated := decorateSessionDraftSummary(items[j], reviewSLASeconds, observedAt)
			if leftDecorated.DraftPendingAgeSeconds != rightDecorated.DraftPendingAgeSeconds {
				return leftDecorated.DraftPendingAgeSeconds > rightDecorated.DraftPendingAgeSeconds
			}
		}
		left := items[i].LastMessageAt
		right := items[j].LastMessageAt
		if left == nil {
			return false
		}
		if right == nil {
			return true
		}
		return left.After(*right)
	})
	return items, nil
}

func (s *fakeStore) CountSessionsSummary(_ context.Context, filter ListSessionsFilter, reviewSLASeconds int) (SessionsSummary, error) {
	var summary SessionsSummary
	for _, item := range s.sessions {
		if filter.Channel != "" && item.Channel != filter.Channel {
			continue
		}
		if filter.Status != "" && item.Status != filter.Status {
			continue
		}
		if filter.HandoffStatus != "" && item.HandoffStatus != filter.HandoffStatus {
			continue
		}
		if filter.ContactKey != "" && item.ContactKey != filter.ContactKey {
			continue
		}

		decorated := decorateSessionDraftSummary(item, reviewSLASeconds, time.Now().UTC())
		summary.TotalCount++
		switch decorated.DraftReviewStatus {
		case "PENDING_REVIEW":
			summary.PendingReviewCount++
		case "REVIEWED":
			summary.ReviewedCount++
		default:
			summary.NoDraftCount++
		}
		if decorated.HandoffStatus == "HUMAN" && strings.TrimSpace(decorated.CurrentOwnerUserID) != "" {
			summary.HumanOwnedCount++
		}
		if decorated.HandoffStatus == "BOT" && strings.TrimSpace(decorated.CurrentOwnerUserID) == "" {
			summary.BotOwnedCount++
		}
		switch decorated.DraftPendingAgeBucket {
		case "DUE_SOON":
			summary.DueSoonReviewCount++
		case "OVERDUE":
			summary.OverdueReviewCount++
		}
		switch decorated.DraftReviewPriority {
		case "HIGH":
			summary.HighPriorityReviewCount++
		case "MEDIUM":
			summary.MediumPriorityReviewCount++
		case "LOW":
			summary.LowPriorityReviewCount++
		}
		if decorated.DraftPendingAgeSeconds > summary.OldestPendingAgeSeconds {
			summary.OldestPendingAgeSeconds = decorated.DraftPendingAgeSeconds
		}
		switch strings.ToUpper(strings.TrimSpace(decorated.DraftAutoSendStatus)) {
		case draftAutoSendStatusRetryPending:
			summary.AutoSendRetryPendingCount++
			summary.AutoSendIssueCount++
		case draftAutoSendStatusBlockedHuman:
			summary.AutoSendBlockedHumanCount++
			summary.AutoSendIssueCount++
		}
	}
	summary.ReviewSLASeconds = reviewSLASeconds
	return summary, nil
}

func (s *fakeStore) GetSession(_ context.Context, id string) (Session, error) {
	item, ok := s.sessions[id]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	return item, nil
}

func (s *fakeStore) ListMessages(_ context.Context, sessionID string, _ ListMessagesFilter) ([]Message, error) {
	items := []Message{}
	for _, id := range s.messageOrder {
		item := s.messages[id]
		if item.SessionID == sessionID {
			items = append(items, item)
		}
	}
	return items, nil
}

func (s *fakeStore) seedSessionWithMessage(contactKey string, body string) (Session, Message) {
	now := time.Now().UTC()
	session, _ := s.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:       "WHATSAPP",
		ContactKey:    contactKey,
		CustomerPhone: contactKey,
		LastMessageAt: &now,
		LastInboundAt: &now,
	})
	message, _ := s.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        session.ID,
		Direction:        "INBOUND",
		Kind:             "TEXT",
		ProcessingStatus: "RECEIVED",
		ReceivedAt:       now,
		Body:             body,
	})
	return session, message
}

func timePointer(value time.Time) *time.Time {
	utc := value.UTC()
	return &utc
}
