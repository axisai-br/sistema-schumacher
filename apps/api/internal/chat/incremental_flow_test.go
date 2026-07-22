package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"schumacher-tur/api/internal/shared/config"
	"strings"
	"testing"
	"time"
)

func TestExtractSelectedOptionIndexBarePrimeira(t *testing.T) {
	cases := map[string]int{
		"primeira": 1,
		"primeiro": 1,
		"1":        1,
		"01":       1,
		"opção 1":  1,
		"opcao 1":  1,
		"segunda":  2,
		"02":       2,
		"terceiro": 3,
		"03":       3,
		"quarta":   4,
		"04":       4,
		"quinto":   5,
		"05":       5,
	}
	for input, want := range cases {
		if got := extractSelectedOptionIndex(input); got != want {
			t.Fatalf("extractSelectedOptionIndex(%q) = %d, want %d", input, got, want)
		}
	}
}

func TestSelectAvailabilityOptionPrimeiraContinuesFromStructuredPassengerState(t *testing.T) {
	out, runner, searcher := reprocessAvailabilitySelection(t, "primeira")
	if out.Draft == nil {
		t.Fatal("expected draft")
	}
	if got := foldChatText(out.Draft.Body); !strings.Contains(got, "documento de 1 passageiro") {
		t.Fatalf("expected the next missing document after selection, got %q", out.Draft.Body)
	}
	if readDraftAutoSendStatus(*out.Draft) != draftAutoSendStatusEligible {
		t.Fatalf("expected auto-send eligible, got %s", readDraftAutoSendStatus(*out.Draft))
	}
	if runner.calls != 0 {
		t.Fatalf("expected LLM not to be called, got %d", runner.calls)
	}
	if searcher.calls != 0 {
		t.Fatalf("expected availability not to be called again, got %d", searcher.calls)
	}
}

func TestSelectAvailabilityOptionDoesNotCallLLM(t *testing.T) {
	_, runner, _ := reprocessAvailabilitySelection(t, "opcao 1")
	if runner.calls != 0 {
		t.Fatalf("expected LLM not to be called, got %d", runner.calls)
	}
}

func TestSelectAvailabilityOptionWithIntentPhraseContinuesFromStructuredPassengerState(t *testing.T) {
	out, runner, searcher, creator := reprocessAvailabilitySelectionWithBookingCreator(t, "quero a primeira opcao")
	if out.Draft == nil {
		t.Fatal("expected draft")
	}
	if got := foldChatText(out.Draft.Body); !strings.Contains(got, "documento de 1 passageiro") {
		t.Fatalf("expected the next missing document after selection, got %q", out.Draft.Body)
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskDocuments) {
		t.Fatalf("expected template %s, got %q", TemplateAskDocuments, got)
	}
	if runner.calls != 0 {
		t.Fatalf("expected LLM not to be called, got %d", runner.calls)
	}
	if searcher.calls != 0 {
		t.Fatalf("expected availability not to be called again, got %d", searcher.calls)
	}
	if creator.calls != 0 {
		t.Fatalf("expected booking_create not to be called, got %d", creator.calls)
	}
}

func TestSelectAvailabilityOptionContextualConfirmationsUseStructuredPassengerState(t *testing.T) {
	for _, text := range []string{"isso msm", "essa msm"} {
		t.Run(text, func(t *testing.T) {
			assertContextualAvailabilitySelectionUsesStructuredPassengerState(t, text)
		})
	}
}

func TestAvailabilityOptionEssaMsmRenderedSingleOptionWithStaleFactsUsesFallback(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"},
	}
	searcher := &fakeAvailabilitySearcher{
		enabled: true,
		result:  availabilityDateSelectionTestResult(),
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)

	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
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
	seedInboundSent(t, store, session.ID, "oi queria me informar pra saber que dia o onibus sai daqui de videira pra santa ines no maranhao", now.Add(-5*time.Minute))
	if _, err := store.SaveAgentDraft(context.Background(), SaveAgentDraftInput{
		SessionID:        session.ID,
		IdempotencyKey:   "draft-stale-availability-selection",
		Body:             "Encontrei estas opcoes antigas.",
		SenderName:       "SHABAS",
		ProcessingStatus: messageStatusAutomationSent,
		Payload: map[string]interface{}{
			"tool_context": map[string]interface{}{
				toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availabilityDateSelectionFiveOptionsTestResult()),
			},
		},
		RecordedAt: now.Add(-4 * time.Minute),
	}); err != nil {
		t.Fatalf("seed stale availability draft: %v", err)
	}
	seedInboundSent(t, store, session.ID, "13/07", now.Add(-3*time.Minute))
	if _, err := store.SaveAgentDraft(context.Background(), SaveAgentDraftInput{
		SessionID:        session.ID,
		IdempotencyKey:   "draft-current-rendered-availability-selection",
		Body:             "Encontrei estas opcoes:\n1. Videira/SC para Santa Ines/MA, 2026-07-13, saida 13:00, R$ 950\n\nQual opcao voce prefere?",
		SenderName:       "SHABAS",
		ProcessingStatus: messageStatusAutomationSent,
		RecordedAt:       now.Add(-2 * time.Minute),
	}); err != nil {
		t.Fatalf("seed current rendered availability draft: %v", err)
	}

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-essa-msm-rendered-single-option",
			IdempotencyKey:    "idem-essa-msm-rendered-single-option",
			Body:              "essa msm",
		},
	})
	if err != nil {
		t.Fatalf("ingest selection: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess selection: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected deterministic selection to avoid LLM, got %d calls", runner.calls)
	}
	if searcher.calls != 0 || len(out.ToolCalls) != 0 {
		t.Fatalf("expected no new availability search, searcher=%d tool_calls=%d", searcher.calls, len(out.ToolCalls))
	}
	if out.Draft == nil {
		t.Fatal("expected draft")
	}
	reply, ok := realizeResponseTemplate(TemplateContextFallbackAvailabilityOption)
	if !ok {
		t.Fatal("expected availability option fallback template")
	}
	if got := strings.TrimSpace(out.Draft.Body); got != reply {
		t.Fatalf("expected contextual fallback reply %q, got %q", reply, got)
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got != string(TemplateContextFallbackAvailabilityOption) {
		t.Fatalf("expected template %s, got %q payload=%+v", TemplateContextFallbackAvailabilityOption, got, out.Draft.NormalizedPayload)
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got == string(TemplateAskPassengerCount) {
		t.Fatalf("stale rendered selection must not advance to passenger count, got %+v", out.Draft.NormalizedPayload)
	}
	intentDecision := asMap(out.Memory["intent_decision"])
	if got := asInt(intentDecision["selected_option_index"]); got != 0 {
		t.Fatalf("stale rendered selection must not select option 1, got %d memory=%+v", got, intentDecision)
	}
}

func assertContextualAvailabilitySelectionUsesStructuredPassengerState(t *testing.T, text string) {
	t.Helper()
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"},
	}
	searcher := &fakeAvailabilitySearcher{
		enabled: true,
		result:  availabilityDateSelectionTestResult(),
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)

	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
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
	seedPassengerClarificationStateV1ForTest(store, session.ID, completePassengerStateForTest(1, 0))
	seedOutboundSent(t, store, session.ID, "De qual cidade do Maranhao voce vai sair?", now.Add(-3*time.Minute))
	if _, err := store.SaveAgentDraft(context.Background(), SaveAgentDraftInput{
		SessionID:        session.ID,
		IdempotencyKey:   "draft-contextual-availability-selection",
		Body:             "Encontrei estas opcoes:\n1. Monção/MA para Videira/SC, 2026-07-06, saida 09:00, R$ 950\n\nQual opcao voce prefere?",
		SenderName:       "SHABAS",
		ProcessingStatus: messageStatusAutomationSent,
		Payload: map[string]interface{}{
			"tool_context": map[string]interface{}{
				toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(AvailabilitySearchResult{
					Filter: AvailabilitySearchInput{
						Origin:      "Monção/MA",
						Destination: "Videira/SC",
						PackageName: packageToSantaCatarina,
						Qty:         1,
						Limit:       5,
					},
					Results: []AvailabilitySearchItem{{
						TripID:                 "trip-2026-07-06",
						BoardStopID:            "board-2026-07-06",
						AlightStopID:           "alight-2026-07-06",
						OriginDisplayName:      "Monção/MA",
						DestinationDisplayName: "Videira/SC",
						OriginDepartTime:       "09:00",
						TripDate:               "2026-07-06",
						SeatsAvailable:         5,
						Price:                  950,
						Currency:               "BRL",
						Status:                 "ACTIVE",
						TripStatus:             "SCHEDULED",
						PackageName:            packageToSantaCatarina,
					}},
				}),
			},
		},
		RecordedAt: now.Add(-2 * time.Minute),
	}); err != nil {
		t.Fatalf("seed availability draft: %v", err)
	}

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-contextual-availability-selection",
			IdempotencyKey:    "idem-contextual-availability-selection",
			Body:              text,
		},
	})
	if err != nil {
		t.Fatalf("ingest selection: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess selection: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected deterministic selection to avoid LLM, got %d calls", runner.calls)
	}
	if searcher.calls != 0 || len(out.ToolCalls) != 0 {
		t.Fatalf("expected no new availability search, searcher=%d tool_calls=%d", searcher.calls, len(out.ToolCalls))
	}
	if out.Draft == nil {
		t.Fatal("expected draft")
	}
	if got := foldChatText(out.Draft.Body); !strings.Contains(got, "documento de 1 passageiro") {
		t.Fatalf("expected the next missing document after selection, got %q", out.Draft.Body)
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["intent"])); got != string(IntentSelectAvailabilityOption) {
		t.Fatalf("expected selected availability intent, got %q payload=%+v", got, out.Draft.NormalizedPayload)
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskDocuments) {
		t.Fatalf("expected template %s, got %q", TemplateAskDocuments, got)
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got == string(TemplateUnsupportedPackage) {
		t.Fatalf("contextual selection must not become unsupported package, got %+v", out.Draft.NormalizedPayload)
	}
	if strings.TrimSpace(out.Draft.Body) == buildUnsupportedPackageReply() {
		t.Fatalf("contextual selection must not use unsupported package reply")
	}
	if reasons := readDraftAutoSendReasons(*out.Draft); containsString(reasons, draftAutoSendReasonOutOfScopeDuringBooking) {
		t.Fatalf("did not expect out-of-scope booking auto-send reason, got %+v", reasons)
	}
	intentDecision := asMap(out.Memory["intent_decision"])
	if got := asInt(intentDecision["selected_option_index"]); got != 1 {
		t.Fatalf("expected selected option index 1, got %d memory=%+v", got, intentDecision)
	}
}

func TestAvailabilityOptionMultipleAmbiguousReplyUsesContextualFallback(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"},
	}
	searcher := &fakeAvailabilitySearcher{
		enabled: true,
		result:  availabilityDateSelectionFiveOptionsTestResult(),
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)

	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:        "WHATSAPP",
		ContactKey:     "5511888888888",
		CustomerPhone:  "5511888888888",
		LastMessageAt:  &now,
		LastOutboundAt: &now,
	})
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}
	seedPassengerClarificationStateV1ForTest(store, session.ID, completePassengerStateForTest(1, 0))
	seedOutboundSent(t, store, session.ID, "De qual cidade do Maranhao voce vai sair?", now.Add(-3*time.Minute))
	seedInboundSent(t, store, session.ID, "opcao 2", now.Add(-150*time.Second))
	if _, err := store.SaveAgentDraft(context.Background(), SaveAgentDraftInput{
		SessionID:        session.ID,
		IdempotencyKey:   "draft-contextual-availability-multiple-options",
		Body:             buildAvailabilityListReply(availabilityDateSelectionFiveOptionsTestResult()),
		SenderName:       "SHABAS",
		ProcessingStatus: messageStatusAutomationSent,
		Payload: map[string]interface{}{
			"tool_context": map[string]interface{}{
				toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availabilityDateSelectionFiveOptionsTestResult()),
			},
		},
		RecordedAt: now.Add(-2 * time.Minute),
	}); err != nil {
		t.Fatalf("seed availability draft: %v", err)
	}

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-contextual-availability-multiple-ok",
			IdempotencyKey:    "idem-contextual-availability-multiple-ok",
			Body:              "ok",
		},
	})
	if err != nil {
		t.Fatalf("ingest ambiguous selection: %v", err)
	}
	historyBeforeReprocess, err := store.ListMessages(context.Background(), ingested.Session.ID, ListMessagesFilter{})
	if err != nil {
		t.Fatalf("list messages before reprocess: %v", err)
	}
	canonicalStateBeforeReprocess := deriveCanonicalConversationState(ingested.Session, historyBeforeReprocess, "ok")
	if canonicalStateBeforeReprocess.Phase == ConversationPhaseRouteSelection || canonicalStateBeforeReprocess.Route.SelectedOptionIndex == 0 {
		t.Fatalf("test setup must start from selected trip state, got %+v", canonicalStateBeforeReprocess)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess ambiguous selection: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected deterministic fallback to avoid LLM, got %d calls", runner.calls)
	}
	if searcher.calls != 0 || len(out.ToolCalls) != 0 {
		t.Fatalf("expected no new availability search, searcher=%d tool_calls=%d", searcher.calls, len(out.ToolCalls))
	}
	if out.Draft == nil {
		t.Fatal("expected draft")
	}
	reply, ok := realizeResponseTemplate(TemplateContextFallbackAvailabilityOption)
	if !ok {
		t.Fatal("expected availability option fallback template")
	}
	if got := strings.TrimSpace(out.Draft.Body); got != reply {
		t.Fatalf("expected contextual fallback reply %q, got %q", reply, got)
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got != string(TemplateContextFallbackAvailabilityOption) {
		t.Fatalf("expected template %s, got %q payload=%+v", TemplateContextFallbackAvailabilityOption, got, out.Draft.NormalizedPayload)
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got == string(TemplateAskPassengerCount) {
		t.Fatalf("ambiguous reply must not advance to passenger count, got %+v", out.Draft.NormalizedPayload)
	}
	intentDecision := asMap(out.Memory["intent_decision"])
	if got := asInt(intentDecision["selected_option_index"]); got != 0 {
		t.Fatalf("ambiguous reply must not select option 1, got %d memory=%+v", got, intentDecision)
	}
	canonicalStateAfterReprocess, ok := out.Memory["canonical_state"].(CanonicalConversationState)
	if !ok {
		t.Fatalf("expected canonical_state in memory, got %#v", out.Memory["canonical_state"])
	}
	if !reflect.DeepEqual(canonicalStateAfterReprocess, canonicalStateBeforeReprocess) {
		t.Fatalf("contextual fallback must not mutate canonical_state\nbefore=%+v\nafter=%+v", canonicalStateBeforeReprocess, canonicalStateAfterReprocess)
	}
}

func TestAvailabilityDateSelectionAfterListDoesNotBecomeUnsupportedPackage(t *testing.T) {
	for _, text := range []string{"06/7", "6/7", "6/07", "06/07"} {
		t.Run(text, func(t *testing.T) {
			store := newFakeStoreWithPassengerAuthority()
			runner := &fakeAgentRunner{
				enabled: true,
				result:  RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"},
			}
			searcher := &fakeAvailabilitySearcher{
				enabled: true,
				result:  availabilityDateSelectionTestResult(),
			}
			svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)

			now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
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
			seedOutboundSent(t, store, session.ID, "De qual cidade do Maranhao voce vai sair?", now.Add(-3*time.Minute))
			if _, err := store.SaveAgentDraft(context.Background(), SaveAgentDraftInput{
				SessionID:        session.ID,
				IdempotencyKey:   "draft-availability-date-selection",
				Body:             "Encontrei estas opcoes:\n1. Santa Ines/MA para Videira/SC, 2026-07-06, saida 08:00, R$ 950\n\nQual opcao voce prefere?",
				SenderName:       "SHABAS",
				ProcessingStatus: messageStatusAutomationSent,
				Payload: map[string]interface{}{
					"tool_context": map[string]interface{}{
						toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availabilityDateSelectionTestResult()),
					},
				},
				RecordedAt: now.Add(-2 * time.Minute),
			}); err != nil {
				t.Fatalf("seed availability draft: %v", err)
			}

			ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
				ContactKey: session.ContactKey,
				Message: IngestMessagePayload{
					Direction:         "INBOUND",
					ProviderMessageID: "msg-availability-date-selection",
					IdempotencyKey:    "idem-availability-date-selection",
					Body:              text,
				},
			})
			if err != nil {
				t.Fatalf("ingest date selection: %v", err)
			}

			out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
			if err != nil {
				t.Fatalf("reprocess date selection: %v", err)
			}
			if runner.calls != 0 {
				t.Fatalf("expected deterministic availability date selection to avoid LLM, got %d calls", runner.calls)
			}
			if searcher.calls != 1 {
				t.Fatalf("expected one availability search, got %d", searcher.calls)
			}
			if searcher.lastInput.TripDate == nil || searcher.lastInput.TripDate.UTC().Format("2006-01-02") != "2026-07-06" {
				t.Fatalf("expected trip date 2026-07-06, got %+v", searcher.lastInput)
			}
			if out.Draft == nil {
				t.Fatal("expected availability draft")
			}
			if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got == string(TemplateUnsupportedPackage) {
				t.Fatalf("date selection must not become unsupported package, got %+v", out.Draft.NormalizedPayload)
			}
			if strings.TrimSpace(out.Draft.Body) == buildUnsupportedPackageReply() {
				t.Fatalf("date selection must not use unsupported package reply")
			}
			if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["intent"])); got != string(IntentAvailabilitySearch) {
				t.Fatalf("expected availability intent, got %q payload=%+v", got, out.Draft.NormalizedPayload)
			}
			if reasons := readDraftAutoSendReasons(*out.Draft); containsString(reasons, draftAutoSendReasonOutOfScopeDuringBooking) {
				t.Fatalf("did not expect out-of-scope booking auto-send reason, got %+v", reasons)
			}
		})
	}
}

func TestIncrementalFlowUsesActivePromptForAvailabilityOptionAfterDate(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"},
	}
	searcher := &fakeAvailabilitySearcher{
		enabled: true,
		result:  activePromptFlowAvailabilityResult(),
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
	contactKey := "5511999999999"
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)

	first := ingestAndReprocessActivePromptFlowTurn(t, svc, contactKey, "active-prompt-flow-start", "quero saber como faço uma reserva")
	if first.Draft == nil {
		t.Fatal("expected reservation start draft")
	}
	sessionID := first.Session.ID
	seedOutboundSent(t, store, sessionID, first.Draft.Body, now.Add(1*time.Minute))

	availability := activePromptFlowAvailabilityResult()
	if _, err := store.SaveAgentDraft(context.Background(), SaveAgentDraftInput{
		SessionID:        sessionID,
		IdempotencyKey:   "active-prompt-flow-availability",
		Body:             buildAvailabilityListReply(availability),
		SenderName:       "SHABAS",
		ProcessingStatus: messageStatusAutomationSent,
		Payload: map[string]interface{}{
			"tool_context": map[string]interface{}{
				toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availability),
			},
		},
		RecordedAt: now.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("seed availability prompt: %v", err)
	}

	out := ingestAndReprocessActivePromptFlowTurn(t, svc, contactKey, "active-prompt-flow-option", "essa mesmo")
	if out.Draft == nil {
		t.Fatal("expected booking continuation draft")
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["intent"])); got != string(IntentSelectAvailabilityOption) {
		t.Fatalf("expected selected availability intent, got %q payload=%+v", got, out.Draft.NormalizedPayload)
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskDocuments) {
		t.Fatalf("expected template %s, got %q", TemplateAskDocuments, got)
	}
	if got := strings.TrimSpace(out.Draft.Body); !strings.Contains(got, "documento de 1 passageiro") {
		t.Fatalf("expected document request, got %q", got)
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got == string(TemplateUnsupportedPackage) {
		t.Fatalf("active prompt selection must not become unsupported package, got %+v", out.Draft.NormalizedPayload)
	}
	if strings.TrimSpace(out.Draft.Body) == buildUnsupportedPackageReply() {
		t.Fatalf("active prompt selection must not use unsupported package reply")
	}
	if reasons := readDraftAutoSendReasons(*out.Draft); containsString(reasons, draftAutoSendReasonOutOfScopeDuringBooking) {
		t.Fatalf("did not expect out-of-scope booking auto-send reason, got %+v", reasons)
	}
	intentDecision := asMap(out.Memory["intent_decision"])
	if got := asInt(intentDecision["selected_option_index"]); got != 1 {
		t.Fatalf("expected selected option index 1, got %d memory=%+v", got, intentDecision)
	}
}

func TestReservationHowToProceedAsksRouteToSCWithoutPassengerCollection(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-reservation-start-1",
			IdempotencyKey:    "idem-reservation-start-1",
			Body:              "como faço pra fazer uma reserva?",
		},
	})
	if err != nil {
		t.Fatalf("ingest reservation start: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess reservation start: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected reservation start template to avoid LLM, got %d calls", runner.calls)
	}
	if searcher.calls != 0 || len(out.ToolCalls) != 0 {
		t.Fatalf("expected no availability search before route, searcher=%d tool_calls=%d", searcher.calls, len(out.ToolCalls))
	}
	if out.Draft == nil {
		t.Fatal("expected draft")
	}
	if got := strings.TrimSpace(out.Draft.Body); got != askReservationRouteSCReply {
		t.Fatalf("expected reservation start reply %q, got %q", askReservationRouteSCReply, got)
	}
	if strings.TrimSpace(out.Draft.Body) == buildUnsupportedPackageReply() {
		t.Fatalf("reservation start must not use unsupported package reply")
	}
	folded := foldChatText(out.Draft.Body)
	for _, want := range []string{"de qual cidade", "para qual cidade", "santa catarina"} {
		if !strings.Contains(folded, strings.TrimSpace(foldChatText(want))) {
			t.Fatalf("expected route-to-SC reply to contain %q, got %q", want, out.Draft.Body)
		}
	}
	for _, notWant := range []string{"passageiro", "crianca", "documento", "pagamento"} {
		if strings.Contains(folded, strings.TrimSpace(foldChatText(notWant))) {
			t.Fatalf("expected reservation start not to ask %q, got %q", notWant, out.Draft.Body)
		}
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskReservationRouteSC) {
		t.Fatalf("expected template %s, got %q", TemplateAskReservationRouteSC, got)
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got == string(TemplateUnsupportedPackage) {
		t.Fatalf("reservation start must not use unsupported package template, got %+v", out.Draft.NormalizedPayload)
	}

	if got := readDraftAutoSendStatus(*out.Draft); got != draftAutoSendStatusEligible {
		t.Fatalf("expected reservation start draft to be auto-send eligible, got %s reasons=%v", got, readDraftAutoSendReasons(*out.Draft))
	}
	if reasons := readDraftAutoSendReasons(*out.Draft); len(reasons) != 0 {
		t.Fatalf("expected no auto-send block reasons, got %v", reasons)
	}
}

func TestReservationHelpWithUnsupportedDestinationReturnsSupportWithoutOpenAI(t *testing.T) {
	for _, text := range []string{
		"como faço pra reservar passagem para Bahia",
		"como faço pra fazer uma reserva para Bahia",
	} {
		t.Run(text, func(t *testing.T) {
			store := newFakeStoreWithPassengerAuthority()
			runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
			searcher := &fakeAvailabilitySearcher{enabled: true}
			svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)

			ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
				ContactKey: "5511999999999",
				Message: IngestMessagePayload{
					Direction:         "INBOUND",
					ProviderMessageID: "msg-reservation-start-unsupported-bahia-1",
					IdempotencyKey:    "idem-reservation-start-unsupported-bahia-1",
					Body:              text,
				},
			})
			if err != nil {
				t.Fatalf("ingest unsupported reservation help: %v", err)
			}

			out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
			if err != nil {
				t.Fatalf("reprocess unsupported reservation help: %v", err)
			}
			assertUnsupportedPackageDraftWithoutTools(t, out, runner, searcher)
		})
	}
}

func TestReservationPhraseInPassengerCollectionDoesNotResetRoute(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
	session := seedPassengerCollectionPhase(t, store)

	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-reservation-active-1",
			IdempotencyKey:    "idem-reservation-active-1",
			Body:              "quero reservar",
		},
	}); err != nil {
		t.Fatalf("ingest active reservation phrase: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess active reservation phrase: %v", err)
	}
	if out.Draft == nil {
		t.Fatal("expected draft")
	}
	folded := foldChatText(out.Draft.Body)
	if strings.Contains(folded, "cidade de saida") ||
		strings.Contains(folded, "santa catarina") ||
		strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])) == string(TemplateAskReservationRouteSC) {
		t.Fatalf("expected active booking flow not to reset to route template, got %q", out.Draft.Body)
	}
	if !strings.Contains(folded, "passagem e so para voce") {
		t.Fatalf("expected passenger collection to remain active, got %q", out.Draft.Body)
	}
	if searcher.calls != 0 {
		t.Fatalf("expected no availability search reset, got %d", searcher.calls)
	}
}

func TestBroadMaranhaoQueryAsksSCOriginWithoutOpenAI(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-broad-ma-1",
			IdempotencyKey:    "idem-broad-ma-1",
			Body:              "Passagem para o Maranhão",
		},
	})
	if err != nil {
		t.Fatalf("ingest broad MA query: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess broad MA query: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected broad MA template to avoid LLM, got %d calls", runner.calls)
	}
	if searcher.calls != 0 || len(out.ToolCalls) != 0 {
		t.Fatalf("expected no tool calls for broad MA query, searcher=%d tool_calls=%d", searcher.calls, len(out.ToolCalls))
	}
	if out.Draft == nil || !strings.Contains(out.Draft.Body, "cidade de Santa Catarina") {
		t.Fatalf("expected SC origin question, got %+v", out.Draft)
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskSCOriginForMA) {
		t.Fatalf("expected template %s, got %q", TemplateAskSCOriginForMA, got)
	}
}

func TestSCOriginAfterMaranhaoQueryAsksMADestinationWithoutOpenAI(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
	session := seedMaranhaoOriginQuestionFromBroadQuery(t, svc)

	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-ma-origin-chapeco-1",
			IdempotencyKey:    "idem-ma-origin-chapeco-1",
			Body:              "Saída de Chapecó",
		},
	}); err != nil {
		t.Fatalf("ingest SC origin: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess SC origin: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected SC origin template to avoid LLM, got %d calls", runner.calls)
	}
	if searcher.calls != 0 || len(out.ToolCalls) != 0 {
		t.Fatalf("expected no availability search before MA destination, searcher=%d tool_calls=%d", searcher.calls, len(out.ToolCalls))
	}
	if out.Draft == nil || strings.TrimSpace(out.Draft.Body) != "Perfeito — Chapeco/SC. Para qual cidade do Maranhao voce quer ir?" {
		t.Fatalf("expected MA destination question, got %+v", out.Draft)
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskMADestination) {
		t.Fatalf("expected template %s, got %q", TemplateAskMADestination, got)
	}
}

func TestBroadSantaCatarinaQueryReturnsPublicTableWithoutOpenAI(t *testing.T) {
	broadSantaCatarinaQueryReturnsPublicTableWithoutOpenAI(t, "Tem passagem para Santa Catarina?")
}

func TestBroadSantaCatarinaAbbreviationsReturnPublicTableWithoutOpenAI(t *testing.T) {
	for _, body := range []string{
		"quero passagem pra sc",
		"quero passagem para sc",
		"quero passagem p/ sc",
		"passagem sc",
		"quero passagem para Santa Catarina",
	} {
		t.Run(body, func(t *testing.T) {
			broadSantaCatarinaQueryReturnsPublicTableWithoutOpenAI(t, body)
		})
	}
}

func broadSantaCatarinaQueryReturnsPublicTableWithoutOpenAI(t *testing.T, body string) {
	t.Helper()
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)

	messageKey := strings.NewReplacer(" ", "-", "/", "-").Replace(strings.ToLower(body))
	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-broad-sc-" + messageKey,
			IdempotencyKey:    "idem-broad-sc-" + messageKey,
			Body:              body,
		},
	})
	if err != nil {
		t.Fatalf("ingest broad SC query: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess broad SC query: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected broad SC template to avoid LLM, got %d calls", runner.calls)
	}
	if searcher.calls != 0 || len(out.ToolCalls) != 0 {
		t.Fatalf("expected no tool calls for public table, searcher=%d tool_calls=%d", searcher.calls, len(out.ToolCalls))
	}
	if out.Draft == nil || !strings.Contains(out.Draft.Body, "Fraiburgo: R$ 950") || !strings.Contains(out.Draft.Body, "Seara: R$ 1100") {
		t.Fatalf("expected public SC table, got %+v", out.Draft)
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got != string(TemplatePublicSCTable) {
		t.Fatalf("expected template %s, got %q", TemplatePublicSCTable, got)
	}
}

func TestSCDestinationAfterPublicTableAsksMAOriginWithoutOpenAI(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
	session := seedPublicSCTableContext(t, store)

	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-sc-destination-videira-1",
			IdempotencyKey:    "idem-sc-destination-videira-1",
			Body:              "quero ir para videira",
		},
	}); err != nil {
		t.Fatalf("ingest SC destination: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess SC destination: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected SC destination template to avoid LLM, got %d calls", runner.calls)
	}
	if searcher.calls != 0 || len(out.ToolCalls) != 0 {
		t.Fatalf("expected no availability search before MA origin, searcher=%d tool_calls=%d", searcher.calls, len(out.ToolCalls))
	}
	if out.Draft == nil || strings.TrimSpace(out.Draft.Body) != "Perfeito — Videira/SC. De qual cidade do Maranhao voce vai sair?" {
		t.Fatalf("expected MA origin question, got %+v", out.Draft)
	}
	if strings.Contains(strings.ToLower(out.Draft.Body), "passage") && strings.Contains(strings.ToLower(out.Draft.Body), "voce") {
		t.Fatalf("expected route follow-up, not passenger question: %q", out.Draft.Body)
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskMAOrigin) {
		t.Fatalf("expected template %s, got %q", TemplateAskMAOrigin, got)
	}
}

func TestUnsupportedMAOriginFollowUpReturnsSupportWithoutOpenAI(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
	session := seedMAOriginQuestionAfterPublicSCTable(t, store)

	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-unsupported-ma-origin-bh-1",
			IdempotencyKey:    "idem-unsupported-ma-origin-bh-1",
			Body:              "Belo Horizonte",
		},
	}); err != nil {
		t.Fatalf("ingest unsupported MA origin: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess unsupported MA origin: %v", err)
	}
	assertUnsupportedPackageDraftWithoutTools(t, out, runner, searcher)
}

func TestUnsupportedMADestinationFollowUpReturnsSupportWithoutOpenAI(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
	session := prepareMaranhaoDestinationFollowUp(t, svc, store)

	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-unsupported-ma-destination-curitiba-1",
			IdempotencyKey:    "idem-unsupported-ma-destination-curitiba-1",
			Body:              "Curitiba",
		},
	}); err != nil {
		t.Fatalf("ingest unsupported MA destination: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess unsupported MA destination: %v", err)
	}
	assertUnsupportedPackageDraftWithoutTools(t, out, runner, searcher)
}

func TestSupportedMAOriginFollowUpStillRunsAvailabilityWithoutOpenAI(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{
		enabled: true,
		result: AvailabilitySearchResult{
			Results: []AvailabilitySearchItem{{
				TripID:                 "trip-1",
				BoardStopID:            "board-1",
				AlightStopID:           "alight-1",
				OriginDisplayName:      "Moncao/MA",
				DestinationDisplayName: "Videira/SC",
				OriginDepartTime:       "09:00",
				TripDate:               "2026-05-11",
				SeatsAvailable:         6,
				Price:                  950,
				Currency:               "BRL",
				Status:                 "ACTIVE",
				TripStatus:             "SCHEDULED",
				PackageName:            packageToSantaCatarina,
			}},
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
	session := seedMAOriginQuestionAfterPublicSCTable(t, store)

	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-supported-ma-origin-moncao-1",
			IdempotencyKey:    "idem-supported-ma-origin-moncao-1",
			Body:              "Moncao",
		},
	}); err != nil {
		t.Fatalf("ingest supported MA origin: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess supported MA origin: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected deterministic flow to avoid LLM, got %d calls", runner.calls)
	}
	if searcher.calls != 1 || len(out.ToolCalls) != 1 {
		t.Fatalf("expected one availability call, searcher=%d tool_calls=%d", searcher.calls, len(out.ToolCalls))
	}
	if searcher.lastInput.Origin != "Moncao/MA" || searcher.lastInput.Destination != "Videira/SC" {
		t.Fatalf("unexpected availability input: %+v", searcher.lastInput)
	}
}

func TestMADestinationAfterSCOriginExecutesAvailabilityTool(t *testing.T) {
	t.Setenv("CHAT_LEGACY_PROMPT_FALLBACK_ENABLED", "false")

	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"},
	}
	searcher := &fakeAvailabilitySearcher{
		enabled: true,
		result: AvailabilitySearchResult{
			Results: []AvailabilitySearchItem{{
				TripID:                 "trip-1",
				BoardStopID:            "board-1",
				AlightStopID:           "alight-1",
				OriginDisplayName:      "Chapeco/SC",
				DestinationDisplayName: "Moncao/MA",
				OriginDepartTime:       "18:30",
				TripDate:               "2026-05-10",
				SeatsAvailable:         6,
				Price:                  950,
				Currency:               "BRL",
				Status:                 "ACTIVE",
				TripStatus:             "SCHEDULED",
				PackageName:            packageToMaranhao,
			}},
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
	session := prepareMaranhaoDestinationFollowUp(t, svc, store)

	_, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-ma-destination-1",
			IdempotencyKey:    "idem-ma-destination-1",
			Body:              "Moncao",
		},
	})
	if err != nil {
		t.Fatalf("ingest destination: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess destination: %v", err)
	}
	if searcher.calls != 1 {
		t.Fatalf("expected one availability search, got %d", searcher.calls)
	}
	if runner.calls != 0 {
		t.Fatalf("expected availability template to avoid LLM, got %d", runner.calls)
	}
	if out.Draft == nil {
		t.Fatal("expected draft after availability search")
	}
	assertAvailabilityTemplateDraft(t, *out.Draft)
	if len(out.ToolCalls) != 1 || out.ToolCalls[0].ToolName != toolNameAvailabilitySearch {
		t.Fatalf("expected one availability tool call, got %+v", out.ToolCalls)
	}
	if searcher.lastInput.Origin != "Chapeco/SC" || searcher.lastInput.Destination != "Moncao/MA" {
		t.Fatalf("unexpected availability search input: %+v", searcher.lastInput)
	}
}

func TestSantaInesAfterChapecoCallsAvailabilitySearchOnceWithoutOpenAI(t *testing.T) {
	t.Setenv("CHAT_LEGACY_PROMPT_FALLBACK_ENABLED", "false")

	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{
		enabled: true,
		result: AvailabilitySearchResult{
			Results: []AvailabilitySearchItem{{
				TripID:                 "trip-1",
				BoardStopID:            "board-1",
				AlightStopID:           "alight-1",
				OriginDisplayName:      "Chapeco/SC",
				DestinationDisplayName: "Santa Ines/MA",
				OriginDepartTime:       "18:30",
				TripDate:               "2026-05-10",
				SeatsAvailable:         6,
				Price:                  950,
				Currency:               "BRL",
				Status:                 "ACTIVE",
				TripStatus:             "SCHEDULED",
				PackageName:            packageToMaranhao,
			}},
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
	session := prepareMaranhaoDestinationFollowUp(t, svc, store)

	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-ma-destination-santa-ines-1",
			IdempotencyKey:    "idem-ma-destination-santa-ines-1",
			Body:              "Santa Inês",
		},
	}); err != nil {
		t.Fatalf("ingest MA destination: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess MA destination: %v", err)
	}
	if searcher.calls != 1 {
		t.Fatalf("expected availability search exactly once, got %d", searcher.calls)
	}
	if runner.calls != 0 {
		t.Fatalf("expected availability template to avoid LLM, got %d calls", runner.calls)
	}
	if searcher.lastInput.Origin != "Chapeco/SC" || searcher.lastInput.Destination != "Santa Ines/MA" {
		t.Fatalf("unexpected availability input: %+v", searcher.lastInput)
	}
	if out.Draft == nil || !strings.Contains(out.Draft.Body, "Santa Ines/MA") {
		t.Fatalf("expected availability result template, got %+v", out.Draft)
	}
}

func TestMADestinationAfterSCOriginSkipsResolveAgentToolContext(t *testing.T) {
	t.Setenv("CHAT_LEGACY_PROMPT_FALLBACK_ENABLED", "false")

	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"},
	}
	searcher := &fakeAvailabilitySearcher{
		enabled: true,
		result: AvailabilitySearchResult{
			Results: []AvailabilitySearchItem{{
				TripID:                 "trip-1",
				BoardStopID:            "board-1",
				AlightStopID:           "alight-1",
				OriginDisplayName:      "Chapeco/SC",
				DestinationDisplayName: "Moncao/MA",
				OriginDepartTime:       "18:30",
				TripDate:               "2026-05-10",
				SeatsAvailable:         6,
				Price:                  950,
				Currency:               "BRL",
				Status:                 "ACTIVE",
				TripStatus:             "SCHEDULED",
				PackageName:            packageToMaranhao,
			}},
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
	session := prepareMaranhaoDestinationFollowUp(t, svc, store)

	_, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-ma-destination-2",
			IdempotencyKey:    "idem-ma-destination-2",
			Body:              "Moncao",
		},
	})
	if err != nil {
		t.Fatalf("ingest destination: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess destination: %v", err)
	}
	if searcher.calls != 1 {
		t.Fatalf("expected one availability search, got %d", searcher.calls)
	}
	if runner.calls != 0 {
		t.Fatalf("expected availability template to avoid LLM, got %d", runner.calls)
	}
	if out.Draft == nil {
		t.Fatal("expected draft after availability search")
	}
	assertAvailabilityTemplateDraft(t, *out.Draft)
	if len(out.ToolCalls) != 1 {
		t.Fatalf("expected one tool call after deterministic tool handling, got %+v", out.ToolCalls)
	}
}

func TestMADestinationAfterSCOriginDoesNotCallLLMBeforeTool(t *testing.T) {
	t.Setenv("CHAT_LEGACY_PROMPT_FALLBACK_ENABLED", "false")

	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"},
	}
	searcher := &availabilitySearchOrderGuard{
		t:       t,
		runner:  runner,
		enabled: true,
		result: AvailabilitySearchResult{
			Results: []AvailabilitySearchItem{{
				TripID:                 "trip-1",
				BoardStopID:            "board-1",
				AlightStopID:           "alight-1",
				OriginDisplayName:      "Chapeco/SC",
				DestinationDisplayName: "Moncao/MA",
				OriginDepartTime:       "18:30",
				TripDate:               "2026-05-10",
				SeatsAvailable:         6,
				Price:                  950,
				Currency:               "BRL",
				Status:                 "ACTIVE",
				TripStatus:             "SCHEDULED",
				PackageName:            packageToMaranhao,
			}},
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
	session := prepareMaranhaoDestinationFollowUp(t, svc, store)

	_, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-ma-destination-3",
			IdempotencyKey:    "idem-ma-destination-3",
			Body:              "Moncao",
		},
	})
	if err != nil {
		t.Fatalf("ingest destination: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess destination: %v", err)
	}
	if searcher.calls != 1 {
		t.Fatalf("expected one availability search, got %d", searcher.calls)
	}
	if runner.calls != 0 {
		t.Fatalf("expected no LLM call before or after deterministic tool execution, got %d", runner.calls)
	}
	if out.Draft == nil {
		t.Fatal("expected draft after availability search")
	}
	assertAvailabilityTemplateDraft(t, *out.Draft)
	if len(out.ToolCalls) != 1 || out.ToolCalls[0].ToolName != toolNameAvailabilitySearch {
		t.Fatalf("expected availability tool call, got %+v", out.ToolCalls)
	}
}

func TestMaranhaoFlowChapecoThenMoncaoCallsAvailabilitySearchOnce(t *testing.T) {
	t.Setenv("CHAT_LEGACY_PROMPT_FALLBACK_ENABLED", "false")

	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"},
	}
	searcher := &fakeAvailabilitySearcher{
		enabled: true,
		result: AvailabilitySearchResult{
			Results: []AvailabilitySearchItem{{
				TripID:                 "trip-1",
				BoardStopID:            "board-1",
				AlightStopID:           "alight-1",
				OriginDisplayName:      "Chapeco/SC",
				DestinationDisplayName: "Moncao/MA",
				OriginDepartTime:       "18:30",
				TripDate:               "2026-05-10",
				SeatsAvailable:         6,
				Price:                  950,
				Currency:               "BRL",
				Status:                 "ACTIVE",
				TripStatus:             "SCHEDULED",
				PackageName:            packageToMaranhao,
			}},
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
	session := prepareMaranhaoDestinationFollowUp(t, svc, store)

	_, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-ma-destination-4",
			IdempotencyKey:    "idem-ma-destination-4",
			Body:              "Moncao",
		},
	})
	if err != nil {
		t.Fatalf("ingest destination: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess destination: %v", err)
	}
	if searcher.calls != 1 {
		t.Fatalf("expected availability search once across Chapeco then Moncao flow, got %d", searcher.calls)
	}
	if runner.calls != 0 {
		t.Fatalf("expected deterministic availability template to avoid LLM, got %d", runner.calls)
	}
	if out.Draft == nil {
		t.Fatal("expected draft after availability search")
	}
	assertAvailabilityTemplateDraft(t, *out.Draft)
}

func TestBlockAutoSendOperationalClaimWithoutTool(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText: "Tem disponibilidade para 18/05 as 09:00 por R$ 950.",
			Model:     "gpt-test",
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-operational-claim-1",
			IdempotencyKey:    "idem-operational-claim-1",
			Body:              "quero passagem",
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
		t.Fatal("expected draft")
	}
	if readDraftAutoSendStatus(*out.Draft) != draftAutoSendStatusReviewNeeded {
		t.Fatalf("expected review required, got %s", readDraftAutoSendStatus(*out.Draft))
	}
	if reasons := readDraftAutoSendReasons(*out.Draft); len(reasons) != 1 || reasons[0] != draftAutoSendReasonOperationalClaimWithoutTool {
		t.Fatalf("expected operational claim reason, got %+v", reasons)
	}
}

func TestUnsupportedCargoDoesNotCallAvailability(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{enabled: true}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-cargo-1",
			IdempotencyKey:    "idem-cargo-1",
			Body:              "quero levar uma moto",
		},
	})
	if err != nil {
		t.Fatalf("ingest message: %v", err)
	}
	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess: %v", err)
	}
	if searcher.calls != 0 {
		t.Fatalf("expected availability not to be called, got %d", searcher.calls)
	}
	if out.Draft == nil || !strings.Contains(out.Draft.Body, "+55 49 9886-2222") {
		t.Fatalf("expected support draft, got %#v", out.Draft)
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got != string(TemplateUnsupportedCargo) {
		t.Fatalf("expected unsupported cargo template, got %q", got)
	}
}

func TestSoEuFillsPassengerCountWithoutOpenAI(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	session := seedPassengerCountContext(t, store, "Perfeito. A passagem e so para voce ou vai mais alguem junto? Tem crianca de 5 anos ou menos?")

	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-so-eu-accent-1",
			IdempotencyKey:    "idem-so-eu-accent-1",
			Body:              "só eu",
			NormalizedPayload: map[string]interface{}{
				passengerClarificationEventsV1MessageKey: []PassengerClarificationEventV1{{
					Type: PassengerClarificationEventPassengerCountSet, Slot: PassengerClarificationSlotPassenger,
					Value: 1, ValueKnown: true, PassengerProvenance: PassengerCountProvenanceSoloSpeaker,
				}},
			},
		},
	}); err != nil {
		t.Fatalf("ingest passenger reply: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess passenger reply: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected passenger template to avoid LLM, got %d calls", runner.calls)
	}
	if got := asInt(out.Memory["passenger_count"]); got != 1 {
		t.Fatalf("expected passenger_count=1, got %d", got)
	}
	if out.Draft == nil || strings.TrimSpace(out.Draft.Body) != askChildUnder5Reply {
		t.Fatalf("expected child question, got %+v", out.Draft)
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskChildUnder5) {
		t.Fatalf("expected template %s, got %q", TemplateAskChildUnder5, got)
	}
}

func TestSemCriancaFillsChildUnder5CountWithoutOpenAI(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	session := seedPassengerCountContext(t, store, "Tem crianca de 5 anos ou menos viajando?")
	passengerKnown := ReducePassengerClarificationEventsV1(newPassengerClarificationStateV1(), []PassengerClarificationEventV1{{
		Type: PassengerClarificationEventPassengerCountSet, Slot: PassengerClarificationSlotPassenger,
		MessageID: "fixture-passenger-known", Value: 1, ValueKnown: true,
		PassengerProvenance: PassengerCountProvenanceSoloSpeaker,
	}})
	passengerKnown.BootstrapCompleted = true
	seedPassengerClarificationStateV1ForTest(store, session.ID, passengerKnown)

	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-sem-crianca-1",
			IdempotencyKey:    "idem-sem-crianca-1",
			Body:              "não, sem criança",
			NormalizedPayload: map[string]interface{}{
				passengerClarificationEventsV1MessageKey: []PassengerClarificationEventV1{{
					Type: PassengerClarificationEventChildCountSet, Slot: PassengerClarificationSlotChild,
					Value: 0, ValueKnown: true,
				}},
			},
		},
	}); err != nil {
		t.Fatalf("ingest child reply: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess child reply: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected child reply template to avoid LLM, got %d calls", runner.calls)
	}
	if got := asInt(out.Memory["child_under_5_count"]); got != 0 {
		t.Fatalf("expected child_under_5_count=0, got %d", got)
	}
	if known := out.Memory["child_under_5_count_known"]; known != true {
		t.Fatalf("expected child_under_5_count_known=true, got %#v", known)
	}
}

func TestPaymentCannotStartWithoutBooking(t *testing.T) {
	input, ok := parsePaymentCreateInput(Session{}, nil, "manda o pix", nil, nil)
	if ok {
		t.Fatalf("expected payment create to be blocked without booking, got %+v", input)
	}
}

func TestPaymentInfoQuestionBeforeBookingUsesClosedTemplateWithoutTool(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	paymentSearcher := &fakePaymentStatusSearcher{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, paymentSearcher)

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-payment-info-before-booking",
			IdempotencyKey:    "idem-payment-info-before-booking",
			Body:              "dia 13/07, ai o pagamento faz logo ou só no dia mesmo?",
		},
	})
	if err != nil {
		t.Fatalf("ingest payment info question: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess payment info question: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected payment info template to avoid LLM, got %d calls", runner.calls)
	}
	if paymentSearcher.calls != 0 || len(out.ToolCalls) != 0 {
		t.Fatalf("expected no payment status tool, calls=%d tool_calls=%+v", paymentSearcher.calls, out.ToolCalls)
	}
	if out.Draft == nil {
		t.Fatal("expected draft")
	}
	if got := strings.TrimSpace(out.Draft.Body); got != paymentOptionsInfoReply {
		t.Fatalf("expected payment info reply %q, got %q", paymentOptionsInfoReply, got)
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got != string(TemplatePaymentOptionsInfo) {
		t.Fatalf("expected template %s, got %q payload=%+v", TemplatePaymentOptionsInfo, got, out.Draft.NormalizedPayload)
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["intent"])); got != string(IntentPaymentInfoQuestion) {
		t.Fatalf("expected payment info intent, got %q payload=%+v", got, out.Draft.NormalizedPayload)
	}
	if got := readDraftAutoSendStatus(*out.Draft); got != draftAutoSendStatusEligible {
		t.Fatalf("expected payment info draft to be auto-send eligible, got %s reasons=%v", got, readDraftAutoSendReasons(*out.Draft))
	}
	if reasons := readDraftAutoSendReasons(*out.Draft); len(reasons) != 0 {
		t.Fatalf("expected no auto-send reasons for payment info template, got %+v", reasons)
	}
}

func TestPaymentInfoQuestionDuringAvailabilitySelectionPreservesPromptAndAutoSends(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	paymentSearcher := &fakePaymentStatusSearcher{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher, paymentSearcher)

	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
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
	availability := availabilityOptionPromptFutureResult()
	if _, err := store.SaveAgentDraft(context.Background(), SaveAgentDraftInput{
		SessionID:        session.ID,
		IdempotencyKey:   "draft-availability-out-of-turn-payment-info",
		Body:             buildAvailabilityListReply(availability),
		SenderName:       "SHABAS",
		ProcessingStatus: messageStatusAutomationSent,
		Payload: map[string]interface{}{
			"tool_context": map[string]interface{}{
				toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availability),
			},
		},
		RecordedAt: now.Add(-1 * time.Minute),
	}); err != nil {
		t.Fatalf("seed availability draft: %v", err)
	}

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-payment-info-during-availability-selection",
			IdempotencyKey:    "idem-payment-info-during-availability-selection",
			Body:              "ai o pagamento eu faço logo ou só no dia mesmo?",
		},
	})
	if err != nil {
		t.Fatalf("ingest payment info question: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess payment info question: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected deterministic payment info template to avoid LLM, got %d calls", runner.calls)
	}
	if searcher.calls != 0 || paymentSearcher.calls != 0 || len(out.ToolCalls) != 0 {
		t.Fatalf("expected no tool calls, availability=%d payment_status=%d tool_calls=%+v", searcher.calls, paymentSearcher.calls, out.ToolCalls)
	}
	if out.Draft == nil {
		t.Fatal("expected draft")
	}
	folded := foldChatText(out.Draft.Body)
	if !strings.Contains(folded, "pagamento pode ser realizado") ||
		!strings.Contains(folded, "para continuar") ||
		!strings.Contains(folded, "qual opcao voce prefere") {
		t.Fatalf("expected payment info with pending availability prompt, got %q", out.Draft.Body)
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got != string(TemplatePaymentOptionsInfo) {
		t.Fatalf("expected template %s, got %q payload=%+v", TemplatePaymentOptionsInfo, got, out.Draft.NormalizedPayload)
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["intent"])); got != string(IntentPaymentInfoQuestion) {
		t.Fatalf("expected payment info intent, got %q payload=%+v", got, out.Draft.NormalizedPayload)
	}
	if got := readDraftAutoSendStatus(*out.Draft); got != draftAutoSendStatusEligible {
		t.Fatalf("expected payment info draft to be auto-send eligible, got %s reasons=%v", got, readDraftAutoSendReasons(*out.Draft))
	}
	if reasons := readDraftAutoSendReasons(*out.Draft); len(reasons) != 0 {
		t.Fatalf("expected no auto-send reasons for payment info template, got %+v", reasons)
	}
	toolContext := asMap(out.Draft.Payload["tool_context"])
	if len(asMap(toolContext[toolNameAvailabilitySearch])) == 0 {
		t.Fatalf("expected out-of-turn reminder draft to preserve availability tool_context, got %+v", out.Draft.Payload)
	}

	intentDecision := asMap(out.Memory["intent_decision"])
	if got := asInt(intentDecision["selected_option_index"]); got != 0 {
		t.Fatalf("out-of-turn info must not select availability option, got %d memory=%+v", got, intentDecision)
	}
	templateData := asMap(intentDecision["template_data"])
	if got := strings.TrimSpace(asString(templateData[outOfTurnPendingPromptTemplateDataKey])); got != string(TemplateContextFallbackAvailabilityOption) {
		t.Fatalf("expected pending availability option template, got %+v", templateData)
	}
}

func TestRejectedAvailabilityOutOfTurnPaymentReminderDoesNotAttachAvailabilityContext(t *testing.T) {
	cases := []string{
		"não quero essa, paga agora?",
		"não quero opção 1, paga agora?",
		"1 não serve, paga agora?",
		"13/07 não serve, paga agora?",
		"essa não dá, paga agora?",
	}
	for _, text := range cases {
		t.Run(text, func(t *testing.T) {
			store := newFakeStoreWithPassengerAuthority()
			runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
			searcher := &fakeAvailabilitySearcher{enabled: true}
			paymentSearcher := &fakePaymentStatusSearcher{enabled: true}
			svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher, paymentSearcher)

			now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
			session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
				Channel:        "WHATSAPP",
				ContactKey:     "5511999999999",
				CustomerPhone:  "5511999999999",
				CustomerName:   "Joao Vitor Messias",
				LastMessageAt:  &now,
				LastOutboundAt: &now,
			})
			if err != nil {
				t.Fatalf("seed session: %v", err)
			}
			availability := availabilityOptionPromptFutureResult()
			if _, err := store.SaveAgentDraft(context.Background(), SaveAgentDraftInput{
				SessionID:        session.ID,
				IdempotencyKey:   "draft-rejected-availability-out-of-turn-" + strings.ReplaceAll(foldChatText(text), " ", "-"),
				Body:             buildAvailabilityListReply(availability),
				SenderName:       "SHABAS",
				ProcessingStatus: messageStatusAutomationSent,
				Payload: map[string]interface{}{
					"tool_context": map[string]interface{}{
						toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availability),
					},
				},
				RecordedAt: now.Add(-1 * time.Minute),
			}); err != nil {
				t.Fatalf("seed availability draft: %v", err)
			}

			out := ingestAndReprocessActivePromptFlowTurn(t, svc, session.ContactKey, "rejected-out-of-turn-payment-"+strings.ReplaceAll(foldChatText(text), " ", "-"), text)
			if out.Draft == nil {
				t.Fatal("expected payment info draft")
			}
			if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got != string(TemplatePaymentOptionsInfo) {
				t.Fatalf("expected payment info template, got %q payload=%+v", got, out.Draft.NormalizedPayload)
			}
			folded := foldChatText(out.Draft.Body)
			if !strings.Contains(folded, "pagamento pode ser realizado") ||
				!strings.Contains(folded, "para continuar") ||
				!strings.Contains(folded, "qual opcao voce prefere") {
				t.Fatalf("expected payment info with pending availability prompt, got %q", out.Draft.Body)
			}
			if availabilityContext := asMap(asMap(out.Draft.Payload["tool_context"])[toolNameAvailabilitySearch]); len(availabilityContext) != 0 {
				t.Fatalf("rejected availability reminder must not attach availability context, got %+v", out.Draft.Payload)
			}
			if availabilityContext := asMap(asMap(out.Draft.NormalizedPayload["tool_context"])[toolNameAvailabilitySearch]); len(availabilityContext) != 0 {
				t.Fatalf("rejected availability reminder must not normalize availability context, got %+v", out.Draft.NormalizedPayload)
			}
			if index := payloadSelectedOptionIndex(out.Draft.NormalizedPayload); index != 0 {
				t.Fatalf("rejected availability reminder must not persist selected option index, got %d payload=%+v", index, out.Draft.NormalizedPayload)
			}
			intentDecision := asMap(out.Memory["intent_decision"])
			templateData := asMap(intentDecision["template_data"])
			if !templateDataBool(templateData, outOfTurnRejectedAvailabilityDataKey) {
				t.Fatalf("expected rejected availability flag in template data, got %+v", templateData)
			}

			history, err := store.ListMessages(context.Background(), session.ID, ListMessagesFilter{})
			if err != nil {
				t.Fatalf("list messages: %v", err)
			}
			if input, ok := parseBookingCreateInput(session, history, "quero reservar\nJoao Vitor Messias | CPF | 84960815086", nil); ok {
				t.Fatalf("expected rejected availability not to be reused for booking_create, got %+v", input)
			}

			freshAvailability := singleAvailabilitySearchHistory(now, AvailabilitySearchItem{
				TripID:                 "trip-2026-07-14",
				BoardStopID:            "board-2026-07-14",
				AlightStopID:           "alight-2026-07-14",
				OriginDisplayName:      "Videira/SC",
				DestinationDisplayName: "Santa Ines/MA",
				OriginDepartTime:       "14:00",
				TripDate:               "2026-07-14",
				Price:                  980,
				Currency:               "BRL",
			})[0]
			freshAvailability.SessionID = session.ID
			freshAvailability.ReceivedAt = now.Add(1 * time.Minute)
			history = append(history, freshAvailability)
			history = appendExplicitSoloPassengerDeclaration(history, now.Add(2*time.Minute))

			input, ok := parseBookingCreateInput(session, history, "quero reservar opção 1\nJoao Vitor Messias | CPF | 84960815086", nil)
			if !ok {
				t.Fatalf("expected fresh availability after rejected reminder to build booking input")
			}
			if input.TripID != "trip-2026-07-14" ||
				input.BoardStopID != "board-2026-07-14" ||
				input.AlightStopID != "alight-2026-07-14" {
				t.Fatalf("expected fresh availability trip after rejected reminder, got %+v", input)
			}
		})
	}
}

func TestAvailabilitySelectionAfterSpecificRejectedOptionOutOfTurnPayment(t *testing.T) {
	for _, reply := range []string{"2", "1"} {
		t.Run(reply, func(t *testing.T) {
			store := newFakeStoreWithPassengerAuthority()
			runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
			searcher := &fakeAvailabilitySearcher{enabled: true}
			paymentSearcher := &fakePaymentStatusSearcher{enabled: true}
			svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher, paymentSearcher)

			now := availabilityTestObservedAt()
			session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
				Channel:        "WHATSAPP",
				ContactKey:     "5511999999999",
				CustomerPhone:  "5511999999999",
				CustomerName:   "Joao Vitor Messias",
				LastMessageAt:  &now,
				LastOutboundAt: &now,
			})
			if err != nil {
				t.Fatalf("seed session: %v", err)
			}
			availability := availabilityOptionPromptFiveOptionsFutureResultAt(now)
			optionTwo := availability.Results[1]
			if _, err := store.SaveAgentDraft(context.Background(), SaveAgentDraftInput{
				SessionID:        session.ID,
				IdempotencyKey:   "draft-specific-rejected-availability-out-of-turn-" + reply,
				Body:             buildAvailabilityListReply(availability),
				SenderName:       "SHABAS",
				ProcessingStatus: messageStatusAutomationSent,
				Payload: map[string]interface{}{
					"tool_context": map[string]interface{}{
						toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availability),
					},
				},
				RecordedAt: now.Add(-1 * time.Minute),
			}); err != nil {
				t.Fatalf("seed availability draft: %v", err)
			}

			first := ingestAndReprocessActivePromptFlowTurn(t, svc, session.ContactKey, "specific-rejected-out-of-turn-payment-"+reply, "não quero essa 1, paga agora?")
			if first.Draft == nil {
				t.Fatal("expected payment info draft")
			}
			if got := strings.TrimSpace(asString(first.Draft.NormalizedPayload["template_name"])); got != string(TemplatePaymentOptionsInfo) {
				t.Fatalf("expected payment info template, got %q payload=%+v", got, first.Draft.NormalizedPayload)
			}
			if index := payloadSelectedOptionIndex(first.Draft.NormalizedPayload); index != 0 {
				t.Fatalf("payment reminder must not select option, got %d payload=%+v", index, first.Draft.NormalizedPayload)
			}
			if availabilityContext := asMap(asMap(first.Draft.Payload["tool_context"])[toolNameAvailabilitySearch]); len(availabilityContext) == 0 {
				t.Fatalf("specific rejected option in multi-option list must preserve availability context, got %+v", first.Draft.Payload)
			}
			templateData := asMap(first.Draft.NormalizedPayload["template_data"])
			if !templateDataBool(templateData, outOfTurnRejectedAvailabilityDataKey) ||
				templateDataBool(templateData, outOfTurnRejectedWholeContextDataKey) {
				t.Fatalf("expected specific rejected availability metadata, got %+v", templateData)
			}
			if got := availabilityRejectedOptionIndexesFromMetadata(templateData[outOfTurnRejectedOptionIndexesDataKey]); !sameIntSlice(got, []int{1}) {
				t.Fatalf("expected rejected option indexes [1], got %+v metadata=%+v", got, templateData)
			}

			markSessionMessagesAutomationSent(t, store, session.ID)
			second := ingestAndReprocessActivePromptFlowTurn(t, svc, session.ContactKey, "selection-after-specific-rejection-"+reply, reply)
			if second.Draft == nil {
				t.Fatal("expected second draft")
			}
			intentDecision := asMap(second.Memory["intent_decision"])
			if reply == "2" {
				if got := strings.TrimSpace(second.Draft.Body); !strings.Contains(got, "documento de 1 passageiro") {
					t.Fatalf("expected document request after option 2, got %q", got)
				}
				if got := strings.TrimSpace(asString(second.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskDocuments) {
					t.Fatalf("expected template %s, got %q payload=%+v", TemplateAskDocuments, got, second.Draft.NormalizedPayload)
				}
				if got := asInt(intentDecision["selected_option_index"]); got != 2 {
					t.Fatalf("expected selected option index 2, got %d memory=%+v", got, intentDecision)
				}
				snapshot := asMap(second.Draft.NormalizedPayload[selectedAvailabilityResultPayloadKey])
				if got := strings.TrimSpace(asString(snapshot["trip_id"])); got != optionTwo.TripID {
					t.Fatalf("expected option 2 trip snapshot, got %q snapshot=%+v", got, snapshot)
				}
				return
			}
			if second.Draft.NormalizedPayload["template_name"] == string(TemplateAskPassengerCount) ||
				asInt(intentDecision["selected_option_index"]) != 0 {
				t.Fatalf("rejected option 1 must not select, draft=%+v memory=%+v", second.Draft.NormalizedPayload, intentDecision)
			}
		})
	}
}

func TestAvailabilitySelectionAfterSpecificRejectedDateOutOfTurnPayment(t *testing.T) {
	for _, reply := range []string{"2", "1"} {
		t.Run(reply, func(t *testing.T) {
			store := newFakeStoreWithPassengerAuthority()
			runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
			searcher := &fakeAvailabilitySearcher{enabled: true}
			paymentSearcher := &fakePaymentStatusSearcher{enabled: true}
			svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher, paymentSearcher)

			now := availabilityTestObservedAt()
			session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
				Channel:        "WHATSAPP",
				ContactKey:     "5511999999999",
				CustomerPhone:  "5511999999999",
				CustomerName:   "Joao Vitor Messias",
				LastMessageAt:  &now,
				LastOutboundAt: &now,
			})
			if err != nil {
				t.Fatalf("seed session: %v", err)
			}
			availability := availabilityOptionPromptFiveOptionsFutureResultAt(now)
			rejectedDateInput := availabilityTestDateInput(t, availability.Results[0].TripDate)
			rejectedDateMetadata := availabilityTestDayMonthMetadata(t, availability.Results[0].TripDate)
			optionTwo := availability.Results[1]
			if _, err := store.SaveAgentDraft(context.Background(), SaveAgentDraftInput{
				SessionID:        session.ID,
				IdempotencyKey:   "draft-specific-rejected-date-out-of-turn-" + reply,
				Body:             buildAvailabilityListReply(availability),
				SenderName:       "SHABAS",
				ProcessingStatus: messageStatusAutomationSent,
				Payload: map[string]interface{}{
					"tool_context": map[string]interface{}{
						toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availability),
					},
				},
				RecordedAt: now.Add(-1 * time.Minute),
			}); err != nil {
				t.Fatalf("seed availability draft: %v", err)
			}

			first := ingestAndReprocessActivePromptFlowTurn(t, svc, session.ContactKey, "specific-rejected-date-out-of-turn-payment-"+reply, "não quero "+rejectedDateInput+", paga agora?")
			if first.Draft == nil {
				t.Fatal("expected payment info draft")
			}
			if got := strings.TrimSpace(asString(first.Draft.NormalizedPayload["template_name"])); got != string(TemplatePaymentOptionsInfo) {
				t.Fatalf("expected payment info template, got %q payload=%+v", got, first.Draft.NormalizedPayload)
			}
			if index := payloadSelectedOptionIndex(first.Draft.NormalizedPayload); index != 0 {
				t.Fatalf("payment reminder must not select option, got %d payload=%+v", index, first.Draft.NormalizedPayload)
			}
			if availabilityContext := asMap(asMap(first.Draft.Payload["tool_context"])[toolNameAvailabilitySearch]); len(availabilityContext) == 0 {
				t.Fatalf("specific rejected date in multi-option list must preserve availability context, got %+v", first.Draft.Payload)
			}
			templateData := asMap(first.Draft.NormalizedPayload["template_data"])
			if !templateDataBool(templateData, outOfTurnRejectedAvailabilityDataKey) ||
				templateDataBool(templateData, outOfTurnRejectedWholeContextDataKey) {
				t.Fatalf("expected specific rejected date metadata, got %+v", templateData)
			}
			if got := availabilityRejectedTripDatesFromMetadata(templateData[outOfTurnRejectedTripDatesDataKey]); !sameStringSlice(got, []string{rejectedDateMetadata}) {
				t.Fatalf("expected rejected trip date [%s], got %+v metadata=%+v", rejectedDateMetadata, got, templateData)
			}

			markSessionMessagesAutomationSent(t, store, session.ID)
			second := ingestAndReprocessActivePromptFlowTurn(t, svc, session.ContactKey, "selection-after-specific-date-rejection-"+reply, reply)
			if second.Draft == nil {
				t.Fatal("expected second draft")
			}
			intentDecision := asMap(second.Memory["intent_decision"])
			if reply == "2" {
				if got := strings.TrimSpace(second.Draft.Body); !strings.Contains(got, "documento de 1 passageiro") {
					t.Fatalf("expected document request after option 2, got %q", got)
				}
				if got := strings.TrimSpace(asString(second.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskDocuments) {
					t.Fatalf("expected template %s, got %q payload=%+v", TemplateAskDocuments, got, second.Draft.NormalizedPayload)
				}
				if got := asInt(intentDecision["selected_option_index"]); got != 2 {
					t.Fatalf("expected selected option index 2, got %d memory=%+v", got, intentDecision)
				}
				snapshot := asMap(second.Draft.NormalizedPayload[selectedAvailabilityResultPayloadKey])
				if got := strings.TrimSpace(asString(snapshot["trip_id"])); got != optionTwo.TripID {
					t.Fatalf("expected option 2 trip snapshot, got %q snapshot=%+v", got, snapshot)
				}
				return
			}
			if second.Draft.NormalizedPayload["template_name"] == string(TemplateAskPassengerCount) ||
				asInt(intentDecision["selected_option_index"]) != 0 {
				t.Fatalf("rejected date option 1 must not select, draft=%+v memory=%+v", second.Draft.NormalizedPayload, intentDecision)
			}
			if snapshot := asMap(second.Draft.NormalizedPayload[selectedAvailabilityResultPayloadKey]); len(snapshot) != 0 {
				t.Fatalf("rejected date option 1 must not persist selected availability, got %+v", snapshot)
			}
		})
	}
}

func TestPassengerFailClosedAvailabilityRejectsStaleInvisibleBlockedAndIncompleteFacts(t *testing.T) {
	testCases := []struct {
		name       string
		secondTurn bool
		seed       func(t *testing.T, store *fakeStore, session Session, now time.Time)
	}{
		{
			name:       "stale_facts",
			secondTurn: true,
			seed: func(t *testing.T, store *fakeStore, session Session, now time.Time) {
				stale := availabilityOptionPromptFutureResultAt(now)
				seedAvailabilitySelectionPromptForFailClosedTest(t, store, session.ID, "stale-facts", stale, messageStatusAutomationSent, true, now.Add(-2*time.Minute))
				seedAvailabilitySelectionPromptForFailClosedTest(t, store, session.ID, "current-without-facts", stale, messageStatusAutomationSent, false, now.Add(-time.Minute))
			},
		},
		{
			name: "invisible_outbound",
			seed: func(t *testing.T, store *fakeStore, session Session, now time.Time) {
				current := availabilityOptionPromptFutureResultAt(now)
				seedAvailabilitySelectionPromptForFailClosedTest(t, store, session.ID, "invisible", current, messageStatusAutomationDraft, true, now.Add(-time.Minute))
			},
		},
		{
			name: "posterior_blocker",
			seed: func(t *testing.T, store *fakeStore, session Session, now time.Time) {
				current := availabilityOptionPromptFutureResultAt(now)
				seedAvailabilitySelectionPromptForFailClosedTest(t, store, session.ID, "before-blocker", current, messageStatusAutomationSent, true, now.Add(-2*time.Minute))
				if _, err := store.SaveAgentDraft(context.Background(), SaveAgentDraftInput{
					SessionID: session.ID, IdempotencyKey: "metadata-only-selection-blocker",
					Body: "Entendi a escolha e ainda preciso confirmar os dados.", SenderName: "SHABAS", ProcessingStatus: messageStatusAutomationSent,
					Payload: map[string]interface{}{"selected_option_index": 1}, RecordedAt: now.Add(-time.Minute),
				}); err != nil {
					t.Fatalf("seed posterior metadata-only blocker: %v", err)
				}
			},
		},
		{
			name: "incomplete_current_item",
			seed: func(t *testing.T, store *fakeStore, session Session, now time.Time) {
				current := availabilityOptionPromptFutureResultAt(now)
				current.Results = append([]AvailabilitySearchItem(nil), current.Results...)
				current.Results[0].BoardStopID = ""
				seedAvailabilitySelectionPromptForFailClosedTest(t, store, session.ID, "incomplete", current, messageStatusAutomationSent, true, now.Add(-time.Minute))
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			base := newFakeStore()
			store := newFakeTravelQueryV2ShadowClaimStore()
			store.fakeStore = base
			runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "must not run", Model: "gpt-test"}}
			jsonRunner := &fakeJSONDecisionRunner{enabled: true}
			openAI := &fakeOpenAIInterpreter{enabled: true}
			travel := &fakeOpenAITravelQueryV2Interpreter{enabled: true}
			searcher := &fakeAvailabilitySearcher{enabled: true}
			booking := &fakeBookingCreator{enabled: true}
			payment := &fakePaymentCreator{enabled: true}
			paymentSearcher := &fakePaymentStatusSearcher{enabled: true}
			svc := NewService(store, config.Config{
				ChatDebounceWindowMS:               1500,
				ChatOpenAIInterpreterShadowEnabled: true,
				ChatOpenAITravelV2ShadowEnabled:    true,
				ChatAgentMode:                      chatAgentModeHybridJSON,
			}, runner, jsonRunner, openAI, travel, searcher, booking, payment, paymentSearcher)

			now := availabilityTestObservedAt()
			session, err := base.UpsertSession(context.Background(), UpsertSessionInput{
				Channel: "WHATSAPP", ContactKey: "5511999999997", CustomerPhone: "5511999999997",
				LastMessageAt: &now, LastOutboundAt: &now,
			})
			if err != nil {
				t.Fatalf("seed fail-closed session: %v", err)
			}
			testCase.seed(t, base, session, now)

			ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
				ContactKey: session.ContactKey,
				Message: IngestMessagePayload{
					Direction: "INBOUND", ProviderMessageID: "fail-closed-" + testCase.name,
					IdempotencyKey: "fail-closed-" + testCase.name, Body: "opcao 1",
				},
			})
			if err != nil {
				t.Fatalf("ingest fail-closed selection: %v", err)
			}
			out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
			if err != nil {
				t.Fatalf("reprocess fail-closed selection: %v", err)
			}
			if out.Draft == nil {
				t.Fatal("expected safe passenger clarification draft")
			}
			if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskPassengerCount) {
				t.Fatalf("expected safe %s, got %q payload=%+v", TemplateAskPassengerCount, got, out.Draft.NormalizedPayload)
			}
			for label, payload := range map[string]map[string]interface{}{
				"payload": out.Draft.Payload, "normalized_payload": out.Draft.NormalizedPayload,
			} {
				if index := payloadSelectedOptionIndex(payload); index != 0 {
					t.Fatalf("%s leaked selected_option_index=%d: %+v", label, index, payload)
				}
				if snapshot := asMap(payload[selectedAvailabilityResultPayloadKey]); len(snapshot) != 0 {
					t.Fatalf("%s leaked selected availability snapshot: %+v", label, snapshot)
				}
			}
			if facts := asMap(asMap(out.Draft.Payload["tool_context"])[toolNameAvailabilitySearch]); len(facts) != 0 {
				t.Fatalf("fail-closed draft leaked stale/invisible/blocked/incomplete facts: %+v", facts)
			}
			canonicalState, ok := out.Memory["canonical_state"].(CanonicalConversationState)
			if !ok {
				t.Fatalf("expected sanitized canonical_state in fail-closed memory, got %#v", out.Memory["canonical_state"])
			}
			assertCanonicalAvailabilityInvalidatedForTest(t, canonicalState, "fail-closed memory")

			reloaded, err := base.GetSession(context.Background(), session.ID)
			if err != nil {
				t.Fatalf("reload fail-closed session: %v", err)
			}
			persistedCanonical := canonicalConversationStateFromTestValue(
				t,
				asMap(reloaded.Metadata["agent"])["canonical_state"],
				"metadata.agent.canonical_state",
			)
			assertCanonicalAvailabilityInvalidatedForTest(t, persistedCanonical, "persisted agent state")
			persistedBoundary := canonicalAvailabilityFactsInvalidationBoundaryInMetadata(reloaded.Metadata)
			if !canonicalAvailabilityFactsInvalidatedInMetadata(reloaded.Metadata) ||
				persistedBoundary.AfterMessageID != ingested.Message.ID ||
				!persistedBoundary.AfterCreatedAt.Equal(canonicalAvailabilityHistoryMessageTime(ingested.Message)) {
				t.Fatalf("fail-closed did not persist the causal inbound boundary: marker=%t boundary=%+v inbound=%+v",
					canonicalAvailabilityFactsInvalidatedInMetadata(reloaded.Metadata), persistedBoundary, ingested.Message)
			}
			if runner.calls != 0 || jsonRunner.calls != 0 || openAI.calls != 0 || travel.calls != 0 ||
				searcher.calls != 0 || booking.calls != 0 || payment.calls != 0 || paymentSearcher.calls != 0 || len(out.ToolCalls) != 0 {
				t.Fatalf("fail-closed selection dispatched external work: runner=%d json=%d shadow=%d travel=%d availability=%d booking=%d payment=%d payment_status=%d tools=%+v",
					runner.calls, jsonRunner.calls, openAI.calls, travel.calls, searcher.calls, booking.calls, payment.calls, paymentSearcher.calls, out.ToolCalls)
			}
			select {
			case <-store.claimAttempts:
				t.Fatal("fail-closed selection scheduled Travel V2 shadow")
			case <-time.After(25 * time.Millisecond):
			}

			if testCase.secondTurn {
				markSessionMessagesAutomationSent(t, base, session.ID)
				secondTurnMessage, err := base.CreateMessage(context.Background(), CreateMessageInput{
					SessionID: session.ID, Direction: "INBOUND", Kind: "TEXT", Body: "talvez",
					ProcessingStatus: "BUFFERED_PENDING", ReceivedAt: now.Add(time.Minute),
					NormalizedPayload: map[string]interface{}{
						passengerClarificationEventsV1MessageKey: []PassengerClarificationEventV1{
							{
								Type: PassengerClarificationEventPassengerCountSet, Slot: PassengerClarificationSlotPassenger,
								Value: 1, ValueKnown: true,
								PassengerProvenance: PassengerCountProvenanceAbsoluteTotal,
							},
							{
								Type: PassengerClarificationEventChildCountSet, Slot: PassengerClarificationSlotChild,
								Value: 0, ValueKnown: true,
							},
						},
					},
				})
				if err != nil {
					t.Fatalf("seed structured passenger authority: %v", err)
				}
				second, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: secondTurnMessage.SessionID})
				if err != nil {
					t.Fatalf("reprocess safe turn after canonical invalidation: %v", err)
				}
				secondCanonical, ok := second.Memory["canonical_state"].(CanonicalConversationState)
				if !ok {
					t.Fatalf("expected canonical_state in safe second turn, got %#v", second.Memory["canonical_state"])
				}
				assertCanonicalAvailabilityInvalidatedForTest(t, secondCanonical, "safe second-turn memory")
				if openAI.calls != 1 {
					state, _ := passengerClarificationStateV1FromSession(second.Session)
					t.Fatalf("expected V1 interpreter shadow once on safe second turn, got %d passenger_state=%+v draft=%+v", openAI.calls, state, second.Draft)
				}
				assertCanonicalAvailabilityInvalidatedForTest(t, openAI.lastInput.StructuredInput.State, "V1 interpreter input")
				if jsonRunner.calls > 0 {
					var compact JSONDecisionCompactInput
					if err := json.Unmarshal([]byte(jsonRunner.lastInput.CompactInput), &compact); err != nil {
						t.Fatalf("decode JSON interpreter input: %v", err)
					}
					if compact.State.SelectedOptionIndex != 0 || compact.State.TripID != "" ||
						compact.State.TripDate != "" || compact.State.PackageName != "" || compact.State.HasAvailabilityFacts {
						t.Fatalf("JSON interpreter received stale availability state: %+v", compact.State)
					}
				}
				select {
				case <-store.completed:
					if travel.calls != 1 {
						t.Fatalf("expected Travel V2 interpreter once on safe second turn, got %d", travel.calls)
					}
					assertCanonicalAvailabilityInvalidatedForTest(t, travel.lastInput.StructuredInput.State, "Travel V2 interpreter input")
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for Travel V2 safe second-turn completion")
				}
			}
		})
	}
}

func TestCanonicalAvailabilityInvalidatedBeforeRouterForMarkedPassengerState(t *testing.T) {
	testCases := []struct {
		name        string
		currentTurn string
		seed        func(t *testing.T, store *fakeStore, session Session, now time.Time)
	}{
		{
			name:        "stale_facts",
			currentTurn: "1",
			seed: func(t *testing.T, store *fakeStore, session Session, now time.Time) {
				stale := availabilityOptionPromptFutureResultAt(now)
				seedAvailabilitySelectionPromptForFailClosedTest(t, store, session.ID, "before-router-stale", stale, messageStatusAutomationSent, true, now.Add(-3*time.Minute))
			},
		},
		{
			name:        "stale_facts_contextual",
			currentTurn: "essa msm",
			seed: func(t *testing.T, store *fakeStore, session Session, now time.Time) {
				stale := availabilityOptionPromptFutureResultAt(now)
				seedAvailabilitySelectionPromptForFailClosedTest(t, store, session.ID, "before-router-stale-contextual", stale, messageStatusAutomationSent, true, now.Add(-3*time.Minute))
			},
		},
		{
			name:        "invisible_outbound",
			currentTurn: "1",
			seed: func(t *testing.T, store *fakeStore, session Session, now time.Time) {
				stale := availabilityOptionPromptFutureResultAt(now)
				seedAvailabilitySelectionPromptForFailClosedTest(t, store, session.ID, "before-router-invisible", stale, messageStatusAutomationDraft, true, now.Add(-3*time.Minute))
				seedAvailabilitySelectionPromptForFailClosedTest(t, store, session.ID, "before-router-invisible-current", stale, messageStatusAutomationSent, false, now.Add(-time.Minute))
			},
		},
		{
			name:        "posterior_blocker",
			currentTurn: "1",
			seed: func(t *testing.T, store *fakeStore, session Session, now time.Time) {
				stale := availabilityOptionPromptFutureResultAt(now)
				seedAvailabilitySelectionPromptForFailClosedTest(t, store, session.ID, "before-router-blocked", stale, messageStatusAutomationSent, true, now.Add(-3*time.Minute))
				if _, err := store.SaveAgentDraft(context.Background(), SaveAgentDraftInput{
					SessionID: session.ID, IdempotencyKey: "before-router-blocker",
					Body: "Entendi a escolha e ainda preciso confirmar os dados.", SenderName: "SHABAS",
					ProcessingStatus: messageStatusAutomationSent, Payload: map[string]interface{}{"selected_option_index": 1}, RecordedAt: now.Add(-2 * time.Minute),
				}); err != nil {
					t.Fatalf("seed posterior blocker: %v", err)
				}
				seedAvailabilitySelectionPromptForFailClosedTest(t, store, session.ID, "before-router-blocked-current", stale, messageStatusAutomationSent, false, now.Add(-time.Minute))
			},
		},
		{
			name:        "incomplete_item",
			currentTurn: "1",
			seed: func(t *testing.T, store *fakeStore, session Session, now time.Time) {
				stale := availabilityOptionPromptFutureResultAt(now)
				stale.Results = append([]AvailabilitySearchItem(nil), stale.Results...)
				stale.Results[0].BoardStopID = ""
				seedAvailabilitySelectionPromptForFailClosedTest(t, store, session.ID, "before-router-incomplete", stale, messageStatusAutomationSent, true, now.Add(-3*time.Minute))
				seedAvailabilitySelectionPromptForFailClosedTest(t, store, session.ID, "before-router-incomplete-current", stale, messageStatusAutomationSent, false, now.Add(-time.Minute))
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			base := newFakeStore()
			store := newFakeTravelQueryV2ShadowClaimStore()
			store.fakeStore = base
			runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "must not run", Model: "gpt-test"}}
			jsonRunner := &fakeJSONDecisionRunner{enabled: true}
			openAI := &fakeOpenAIInterpreter{enabled: true}
			travel := &fakeOpenAITravelQueryV2Interpreter{enabled: true}
			searcher := &fakeAvailabilitySearcher{enabled: true}
			booking := &fakeBookingCreator{enabled: true}
			payment := &fakePaymentCreator{enabled: true}
			paymentSearcher := &fakePaymentStatusSearcher{enabled: true}
			svc := NewService(store, config.Config{
				ChatDebounceWindowMS:               1500,
				ChatOpenAIInterpreterShadowEnabled: true,
				ChatOpenAITravelV2ShadowEnabled:    true,
				ChatAgentMode:                      chatAgentModeHybridJSON,
			},
				runner, jsonRunner, openAI, travel, searcher, booking, payment, paymentSearcher)

			now := availabilityTestObservedAt()
			session, err := base.UpsertSession(context.Background(), UpsertSessionInput{
				Channel: "WHATSAPP", ContactKey: "5511888" + testCase.name, CustomerPhone: "5511888" + testCase.name,
				LastMessageAt: &now, LastOutboundAt: &now,
			})
			if err != nil {
				t.Fatalf("seed marked session: %v", err)
			}
			seedPassengerClarificationStateV1ForTest(base, session.ID, completePassengerStateForTest(1, 0))
			testCase.seed(t, base, session, now)
			markCanonicalAvailabilityInvalidatedForTest(base, session.ID)

			routerCalls := 0
			routerHistory := []Message(nil)
			routerState := CanonicalConversationState{}
			routerActivePrompt := ActivePromptContext{}
			routerDecision := IntentDecision{}
			svc.deterministicRouter = func(history []Message, currentTurn string, state CanonicalConversationState, observedAt time.Time) IntentDecision {
				routerCalls++
				routerHistory = append([]Message(nil), history...)
				routerState = canonicalConversationStateFromTestValue(t, state, "router input capture")
				routerActivePrompt = InferActivePromptContext(history, state)
				routerDecision = routeDeterministicIntent(history, currentTurn, state, observedAt)
				return routerDecision
			}

			ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
				ContactKey: session.ContactKey,
				Message: IngestMessagePayload{Direction: "INBOUND", ProviderMessageID: "before-router-" + testCase.name,
					IdempotencyKey: "before-router-" + testCase.name, Body: testCase.currentTurn},
			})
			if err != nil {
				t.Fatalf("ingest marked selection: %v", err)
			}
			out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
			if err != nil {
				t.Fatalf("reprocess marked selection: %v", err)
			}
			if routerCalls != 1 {
				t.Fatalf("router calls=%d, want 1", routerCalls)
			}
			assertAvailabilityInvalidatedRouterHistoryForTest(t, routerHistory)
			assertCanonicalAvailabilityInvalidatedForTest(t, routerState, "router input")
			if routerActivePrompt.Kind == ActivePromptAvailabilityOptionChoice {
				t.Fatalf("router received stale availability active prompt: %+v", routerActivePrompt)
			}
			if routerDecision.Intent == IntentSelectAvailabilityOption {
				t.Fatalf("router selected invalidated availability: %+v", routerDecision)
			}
			if intentDecision := asMap(out.Memory["intent_decision"]); strings.TrimSpace(asString(intentDecision["intent"])) == string(IntentSelectAvailabilityOption) {
				t.Fatalf("intent_decision selected invalidated availability: %+v", intentDecision)
			}
			if out.Draft == nil {
				t.Fatal("expected deterministic safe draft")
			}
			for label, payload := range map[string]map[string]interface{}{"payload": out.Draft.Payload, "normalized_payload": out.Draft.NormalizedPayload} {
				if index := payloadSelectedOptionIndex(payload); index != 0 {
					t.Fatalf("%s leaked invalidated index=%d: %+v", label, index, payload)
				}
				if snapshot := asMap(payload[selectedAvailabilityResultPayloadKey]); len(snapshot) != 0 {
					t.Fatalf("%s leaked invalidated snapshot: %+v", label, snapshot)
				}
				assertNoRouteIDKeysAtAnyDepthForTest(t, payload, "draft."+label)
			}
			reloaded, err := base.GetSession(context.Background(), session.ID)
			if err != nil {
				t.Fatalf("reload marked invalidation: %v", err)
			}
			if !canonicalAvailabilityFactsInvalidatedInMetadata(reloaded.Metadata) {
				t.Fatalf("invalid selection cleared the persisted marker: %+v", reloaded.Metadata)
			}
			persistedBoundary := canonicalAvailabilityFactsInvalidationBoundaryInMetadata(reloaded.Metadata)
			if persistedBoundary.AfterMessageID != ingested.Message.ID ||
				!persistedBoundary.AfterCreatedAt.Equal(canonicalAvailabilityHistoryMessageTime(ingested.Message)) {
				t.Fatalf("legacy marker did not acquire a conservative current-turn boundary: got=%+v inbound=%+v",
					persistedBoundary, ingested.Message)
			}
			if runner.calls != 0 || jsonRunner.calls != 0 || openAI.calls != 0 || travel.calls != 0 ||
				searcher.calls != 0 || booking.calls != 0 || payment.calls != 0 || paymentSearcher.calls != 0 || len(out.ToolCalls) != 0 {
				t.Fatalf("marked invalidation dispatched external work: runner=%d json=%d shadow=%d travel=%d availability=%d booking=%d payment=%d payment_status=%d tools=%+v",
					runner.calls, jsonRunner.calls, openAI.calls, travel.calls, searcher.calls, booking.calls, payment.calls, paymentSearcher.calls, out.ToolCalls)
			}
			select {
			case <-store.claimAttempts:
				t.Fatal("marked invalidation scheduled Travel V2 shadow")
			case <-time.After(25 * time.Millisecond):
			}
			if travel.calls != 0 {
				t.Fatalf("Travel V2 interpreter ran after the negative claim window: calls=%d", travel.calls)
			}
			if openAI.calls != 0 {
				t.Fatalf("V1 interpreter shadow ran after the negative claim window: calls=%d", openAI.calls)
			}
		})
	}
}

func TestPreInvalidationAvailabilityHistoryBoundaryDoesNotReachRouter(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "must not run", Model: "gpt-test"}}
	jsonRunner := &fakeJSONDecisionRunner{enabled: true}
	openAI := &fakeOpenAIInterpreter{enabled: true}
	travel := &fakeOpenAITravelQueryV2Interpreter{enabled: true}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	booking := &fakeBookingCreator{enabled: true}
	payment := &fakePaymentCreator{enabled: true}
	paymentSearcher := &fakePaymentStatusSearcher{enabled: true}
	svc := NewService(store, config.Config{
		ChatDebounceWindowMS:               1500,
		ChatOpenAIInterpreterShadowEnabled: true,
		ChatOpenAITravelV2ShadowEnabled:    true,
		ChatAgentMode:                      chatAgentModeHybridJSON,
	}, runner, jsonRunner, openAI, travel, searcher, booking, payment, paymentSearcher)

	now := availabilityTestObservedAt()
	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel: "WHATSAPP", ContactKey: "5511888000101", CustomerPhone: "5511888000101",
		LastMessageAt: &now, LastOutboundAt: &now,
	})
	if err != nil {
		t.Fatalf("seed boundary session: %v", err)
	}
	seedPassengerClarificationStateV1ForTest(store, session.ID, completePassengerStateForTest(1, 0))
	stale := availabilityOptionPromptFutureResultAt(now)
	seedAvailabilitySelectionPromptForFailClosedTest(
		t, store, session.ID, "pre-boundary-sent", stale, messageStatusAutomationSent, true, now.Add(-4*time.Minute),
	)
	boundary, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID: session.ID, Direction: "INBOUND", Kind: "TEXT", Body: "opcao antiga invalida",
		ProcessingStatus: "AUTOMATION_PROCESSED", ReceivedAt: now.Add(-3 * time.Minute),
	})
	if err != nil {
		t.Fatalf("seed invalidation boundary: %v", err)
	}
	seedAvailabilitySelectionPromptForFailClosedTest(
		t, store, session.ID, "post-boundary-draft", stale, messageStatusAutomationDraft, true, now.Add(-2*time.Minute),
	)
	markCanonicalAvailabilityInvalidatedAfterMessageForTest(store, session.ID, boundary)

	routerDecision := IntentDecision{}
	svc.deterministicRouter = func(history []Message, currentTurn string, state CanonicalConversationState, observedAt time.Time) IntentDecision {
		routerDecision = routeDeterministicIntent(history, currentTurn, state, observedAt)
		return routerDecision
	}
	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{Direction: "INBOUND", ProviderMessageID: "pre-boundary-router",
			IdempotencyKey: "pre-boundary-router", Body: "essa msm"},
	})
	if err != nil {
		t.Fatalf("ingest boundary selection: %v", err)
	}
	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess boundary selection: %v", err)
	}
	if routerDecision.Intent == IntentSelectAvailabilityOption {
		t.Fatalf("router selected pre-boundary availability: %+v", routerDecision)
	}
	if out.Draft == nil {
		t.Fatal("expected safe deterministic draft")
	}
	for label, payload := range map[string]map[string]interface{}{
		"payload": out.Draft.Payload, "normalized_payload": out.Draft.NormalizedPayload,
	} {
		if index := payloadSelectedOptionIndex(payload); index != 0 {
			t.Fatalf("%s leaked pre-boundary index=%d: %+v", label, index, payload)
		}
		if snapshot := asMap(payload[selectedAvailabilityResultPayloadKey]); len(snapshot) != 0 {
			t.Fatalf("%s leaked pre-boundary snapshot: %+v", label, snapshot)
		}
	}
	reloaded, err := store.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("reload boundary session: %v", err)
	}
	if !canonicalAvailabilityFactsInvalidatedInMetadata(reloaded.Metadata) {
		t.Fatalf("pre-boundary selection cleared invalidation marker: %+v", reloaded.Metadata)
	}
	if runner.calls != 0 || jsonRunner.calls != 0 || openAI.calls != 0 || travel.calls != 0 ||
		searcher.calls != 0 || booking.calls != 0 || payment.calls != 0 || paymentSearcher.calls != 0 || len(out.ToolCalls) != 0 {
		t.Fatalf("boundary fail-closed dispatched external work: runner=%d json=%d shadow=%d travel=%d availability=%d booking=%d payment=%d payment_status=%d tools=%+v",
			runner.calls, jsonRunner.calls, openAI.calls, travel.calls, searcher.calls, booking.calls, payment.calls, paymentSearcher.calls, out.ToolCalls)
	}
}

func TestAvailabilityInvalidationHistoryBoundaryRejectsUntrustedPostBoundaryAvailability(t *testing.T) {
	testCases := []struct {
		name             string
		processingStatus string
		incomplete       bool
		botAutoReply     bool
	}{
		{name: "draft", processingStatus: messageStatusAutomationDraft},
		{name: "blocked", processingStatus: "BLOCKED_BY_REVIEW"},
		{name: "manual_pending", processingStatus: "MANUAL_PENDING"},
		{name: "send_failed", processingStatus: "SEND_FAILED"},
		{name: "incomplete_item", processingStatus: messageStatusAutomationSent, incomplete: true},
		{name: "bot_auto_reply_without_reliable_source", processingStatus: "PENDING", botAutoReply: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			store := newFakeStore()
			runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "must not run", Model: "gpt-test"}}
			jsonRunner := &fakeJSONDecisionRunner{enabled: true}
			openAI := &fakeOpenAIInterpreter{enabled: true}
			travel := &fakeOpenAITravelQueryV2Interpreter{enabled: true}
			searcher := &fakeAvailabilitySearcher{enabled: true}
			booking := &fakeBookingCreator{enabled: true}
			payment := &fakePaymentCreator{enabled: true}
			paymentSearcher := &fakePaymentStatusSearcher{enabled: true}
			svc := NewService(store, config.Config{
				ChatDebounceWindowMS:               1500,
				ChatOpenAIInterpreterShadowEnabled: true,
				ChatOpenAITravelV2ShadowEnabled:    true,
				ChatAgentMode:                      chatAgentModeHybridJSON,
			}, runner, jsonRunner, openAI, travel, searcher, booking, payment, paymentSearcher)

			now := availabilityTestObservedAt()
			session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
				Channel: "WHATSAPP", ContactKey: "boundary-" + testCase.name, CustomerPhone: "boundary-" + testCase.name,
				LastMessageAt: &now, LastOutboundAt: &now,
			})
			if err != nil {
				t.Fatalf("seed boundary session: %v", err)
			}
			seedPassengerClarificationStateV1ForTest(store, session.ID, completePassengerStateForTest(1, 0))
			oldAvailability := availabilityOptionPromptFutureResultAt(now)
			seedAvailabilitySelectionPromptForFailClosedTest(
				t, store, session.ID, "old-sent-"+testCase.name, oldAvailability, messageStatusAutomationSent, true, now.Add(-5*time.Minute),
			)
			boundary, err := store.CreateMessage(context.Background(), CreateMessageInput{
				SessionID: session.ID, Direction: "INBOUND", Kind: "TEXT", Body: "invalidar availability anterior",
				ProcessingStatus: "AUTOMATION_PROCESSED", ReceivedAt: now.Add(-4 * time.Minute),
			})
			if err != nil {
				t.Fatalf("seed invalidation boundary: %v", err)
			}
			postBoundaryAvailability := availabilityOptionPromptFutureResultAt(now.Add(time.Hour))
			if testCase.incomplete {
				postBoundaryAvailability.Results = append([]AvailabilitySearchItem(nil), postBoundaryAvailability.Results...)
				postBoundaryAvailability.Results[0].BoardStopID = ""
			}
			payload := map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(postBoundaryAvailability),
				},
			}
			if testCase.botAutoReply {
				payload["mode"] = "BOT_AUTO_REPLY"
				payload["draft_message_id"] = "missing-unreliable-source"
			}
			if _, err := store.SaveAgentDraft(context.Background(), SaveAgentDraftInput{
				SessionID: session.ID, IdempotencyKey: "post-boundary-" + testCase.name,
				Body: buildAvailabilityListReply(postBoundaryAvailability), SenderName: "SHABAS",
				ProcessingStatus: testCase.processingStatus, Payload: payload, RecordedAt: now.Add(-2 * time.Minute),
			}); err != nil {
				t.Fatalf("seed post-boundary availability: %v", err)
			}
			markCanonicalAvailabilityInvalidatedAfterMessageForTest(store, session.ID, boundary)

			routerDecision := IntentDecision{}
			svc.deterministicRouter = func(history []Message, currentTurn string, state CanonicalConversationState, observedAt time.Time) IntentDecision {
				routerDecision = routeDeterministicIntent(history, currentTurn, state, observedAt)
				return routerDecision
			}
			ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
				ContactKey: session.ContactKey,
				Message: IngestMessagePayload{Direction: "INBOUND", ProviderMessageID: "boundary-current-" + testCase.name,
					IdempotencyKey: "boundary-current-" + testCase.name, Body: "essa msm"},
			})
			if err != nil {
				t.Fatalf("ingest boundary reply: %v", err)
			}
			out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
			if err != nil {
				t.Fatalf("reprocess boundary reply: %v", err)
			}
			if routerDecision.Intent == IntentSelectAvailabilityOption {
				t.Fatalf("router selected untrusted post-boundary availability: %+v", routerDecision)
			}
			if out.Draft == nil {
				t.Fatal("expected safe deterministic draft")
			}
			for label, draftPayload := range map[string]map[string]interface{}{
				"payload": out.Draft.Payload, "normalized_payload": out.Draft.NormalizedPayload,
			} {
				if index := payloadSelectedOptionIndex(draftPayload); index != 0 {
					t.Fatalf("%s leaked old index=%d: %+v", label, index, draftPayload)
				}
				if snapshot := asMap(draftPayload[selectedAvailabilityResultPayloadKey]); len(snapshot) != 0 {
					t.Fatalf("%s leaked old snapshot: %+v", label, snapshot)
				}
				if facts := asMap(asMap(draftPayload["tool_context"])[toolNameAvailabilitySearch]); len(facts) != 0 {
					t.Fatalf("%s leaked old trip/stop facts: %+v", label, facts)
				}
			}
			canonical, ok := out.Memory["canonical_state"].(CanonicalConversationState)
			if !ok {
				t.Fatalf("missing canonical state in fail-closed memory: %#v", out.Memory["canonical_state"])
			}
			assertCanonicalAvailabilityInvalidatedForTest(t, canonical, "boundary fail-closed memory")
			reloaded, err := store.GetSession(context.Background(), session.ID)
			if err != nil {
				t.Fatalf("reload boundary session: %v", err)
			}
			persistedBoundary := canonicalAvailabilityFactsInvalidationBoundaryInMetadata(reloaded.Metadata)
			if !canonicalAvailabilityFactsInvalidatedInMetadata(reloaded.Metadata) ||
				persistedBoundary.AfterMessageID != boundary.ID ||
				!persistedBoundary.AfterCreatedAt.Equal(canonicalAvailabilityHistoryMessageTime(boundary)) {
				t.Fatalf("marker/boundary changed after rejected selection: marker=%t boundary=%+v metadata=%+v",
					canonicalAvailabilityFactsInvalidatedInMetadata(reloaded.Metadata), persistedBoundary, reloaded.Metadata)
			}
			if runner.calls != 0 || jsonRunner.calls != 0 || openAI.calls != 0 || travel.calls != 0 ||
				searcher.calls != 0 || booking.calls != 0 || payment.calls != 0 || paymentSearcher.calls != 0 || len(out.ToolCalls) != 0 {
				t.Fatalf("boundary fail-closed dispatched external work: runner=%d json=%d shadow=%d travel=%d availability=%d booking=%d payment=%d payment_status=%d tools=%+v",
					runner.calls, jsonRunner.calls, openAI.calls, travel.calls, searcher.calls, booking.calls, payment.calls, paymentSearcher.calls, out.ToolCalls)
			}
		})
	}
}

func TestPostInvalidationHistoryBoundaryFreshAvailabilityClearsAfterRouter(t *testing.T) {
	assertPostInvalidationHistoryBoundaryFreshAvailabilityClearsAfterRouter(t, "1")
}

func TestPostInvalidationHistoryBoundaryFreshAvailabilityContextualSelectionClearsAfterRouter(t *testing.T) {
	assertPostInvalidationHistoryBoundaryFreshAvailabilityClearsAfterRouter(t, "essa msm")
}

func assertPostInvalidationHistoryBoundaryFreshAvailabilityClearsAfterRouter(t *testing.T, currentTurn string) {
	t.Helper()
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "must not run", Model: "gpt-test"}}
	jsonRunner := &fakeJSONDecisionRunner{enabled: true}
	openAI := &fakeOpenAIInterpreter{enabled: true}
	travel := &fakeOpenAITravelQueryV2Interpreter{enabled: true}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	booking := &fakeBookingCreator{enabled: true}
	payment := &fakePaymentCreator{enabled: true}
	paymentSearcher := &fakePaymentStatusSearcher{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500, ChatAgentMode: chatAgentModeHybridJSON},
		runner, jsonRunner, openAI, travel, searcher, booking, payment, paymentSearcher)

	now := availabilityTestObservedAt()
	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel: "WHATSAPP", ContactKey: "5511888777777", CustomerPhone: "5511888777777",
		LastMessageAt: &now, LastOutboundAt: &now,
	})
	if err != nil {
		t.Fatalf("seed fresh selection session: %v", err)
	}
	seedPassengerClarificationStateV1ForTest(store, session.ID, completePassengerStateForTest(1, 0))
	stale := availabilityOptionPromptFutureResultAt(now)
	seedAvailabilitySelectionPromptForFailClosedTest(t, store, session.ID, "fresh-replaces-stale", stale, messageStatusAutomationSent, true, now.Add(-3*time.Minute))
	seedAvailabilitySelectionPromptForFailClosedTest(t, store, session.ID, "fresh-replaces-stale-blocker", stale, messageStatusAutomationSent, false, now.Add(-2*time.Minute))
	boundary, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID: session.ID, Direction: "INBOUND", Kind: "TEXT", Body: "invalidar availability anterior",
		ProcessingStatus: "AUTOMATION_PROCESSED", ReceivedAt: now.Add(-90 * time.Second),
	})
	if err != nil {
		t.Fatalf("seed fresh availability boundary: %v", err)
	}
	fresh := availabilityOptionPromptFutureResultAt(now.AddDate(0, 0, 1))
	if stale.Results[0].TripID == fresh.Results[0].TripID ||
		stale.Results[0].BoardStopID == fresh.Results[0].BoardStopID ||
		stale.Results[0].AlightStopID == fresh.Results[0].AlightStopID {
		t.Fatalf("fresh control must use route IDs distinct from stale facts: stale=%+v fresh=%+v", stale.Results[0], fresh.Results[0])
	}
	seedAvailabilitySelectionPromptForFailClosedTest(t, store, session.ID, "fresh-current", fresh, messageStatusAutomationSent, true, now.Add(-time.Minute))
	markCanonicalAvailabilityInvalidatedAfterMessageForTest(store, session.ID, boundary)

	routerState := CanonicalConversationState{}
	routerDecision := IntentDecision{}
	svc.deterministicRouter = func(history []Message, currentTurn string, state CanonicalConversationState, observedAt time.Time) IntentDecision {
		routerState = canonicalConversationStateFromTestValue(t, state, "fresh router input capture")
		routerDecision = routeDeterministicIntent(history, currentTurn, state, observedAt)
		return routerDecision
	}
	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{Direction: "INBOUND", ProviderMessageID: "fresh-clears-marker",
			IdempotencyKey: "fresh-clears-marker", Body: currentTurn},
	})
	if err != nil {
		t.Fatalf("ingest fresh selection: %v", err)
	}
	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess fresh selection: %v", err)
	}
	assertCanonicalAvailabilityInvalidatedForTest(t, routerState, "fresh selection router input")
	if routerDecision.Intent != IntentSelectAvailabilityOption || routerDecision.SelectedOptionIndex != 1 {
		t.Fatalf("fresh current availability was not selected: %+v", routerDecision)
	}
	if out.Draft == nil {
		t.Fatal("expected fresh selection draft")
	}
	expectedSnapshot := selectedAvailabilityResultPayloadFromAvailability(&fresh, 1)
	for label, payload := range map[string]map[string]interface{}{"payload": out.Draft.Payload, "normalized_payload": out.Draft.NormalizedPayload} {
		if index := payloadSelectedOptionIndex(payload); index != 1 {
			t.Fatalf("%s selected index=%d, want 1: %+v", label, index, payload)
		}
		if snapshot := asMap(payload[selectedAvailabilityResultPayloadKey]); !reflect.DeepEqual(snapshot, expectedSnapshot) {
			t.Fatalf("%s fresh snapshot mismatch: got=%+v want=%+v", label, snapshot, expectedSnapshot)
		}
		assertStringValuesAbsentAtAnyDepthForTest(t, payload, "fresh draft."+label,
			stale.Results[0].TripID, stale.Results[0].BoardStopID, stale.Results[0].AlightStopID)
	}
	reloaded, err := store.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("reload fresh selection: %v", err)
	}
	if canonicalAvailabilityFactsInvalidatedInMetadata(reloaded.Metadata) {
		t.Fatalf("fresh complete availability did not clear invalidation marker: %+v", reloaded.Metadata)
	}
	if boundary := canonicalAvailabilityFactsInvalidationBoundaryInMetadata(reloaded.Metadata); boundary.known() {
		t.Fatalf("fresh complete availability did not clear invalidation boundary: %+v", boundary)
	}
	persistedCanonical := canonicalConversationStateFromTestValue(t, asMap(reloaded.Metadata["agent"])["canonical_state"], "fresh metadata.agent.canonical_state")
	if persistedCanonical.Route.SelectedOptionIndex != 1 || persistedCanonical.Route.TripID != fresh.Results[0].TripID ||
		persistedCanonical.Route.BoardStopID != fresh.Results[0].BoardStopID || persistedCanonical.Route.AlightStopID != fresh.Results[0].AlightStopID {
		t.Fatalf("fresh complete route was not persisted: %+v", persistedCanonical.Route)
	}
	if facts := asMap(persistedCanonical.LastToolFacts[toolNameAvailabilitySearch]); len(facts) == 0 {
		t.Fatalf("fresh availability facts were not persisted: %+v", persistedCanonical.LastToolFacts)
	} else {
		assertStringValuesAbsentAtAnyDepthForTest(t, facts, "fresh persisted availability facts",
			stale.Results[0].TripID, stale.Results[0].BoardStopID, stale.Results[0].AlightStopID)
	}
	if runner.calls != 0 || jsonRunner.calls != 0 || openAI.calls != 0 || travel.calls != 0 ||
		searcher.calls != 0 || booking.calls != 0 || payment.calls != 0 || paymentSearcher.calls != 0 || len(out.ToolCalls) != 0 {
		t.Fatalf("fresh deterministic selection dispatched external work: runner=%d json=%d shadow=%d travel=%d availability=%d booking=%d payment=%d payment_status=%d tools=%+v",
			runner.calls, jsonRunner.calls, openAI.calls, travel.calls, searcher.calls, booking.calls, payment.calls, paymentSearcher.calls, out.ToolCalls)
	}
}

func markCanonicalAvailabilityInvalidatedForTest(store *fakeStore, sessionID string) {
	session := store.sessions[sessionID]
	metadata := cloneMap(session.Metadata)
	agent := cloneMap(asMap(metadata["agent"]))
	if agent == nil {
		agent = map[string]interface{}{}
	}
	agent[canonicalAvailabilityFactsInvalidatedMetadataKey] = true
	metadata["agent"] = agent
	memory := cloneMap(asMap(metadata["memory"]))
	if memory == nil {
		memory = map[string]interface{}{}
	}
	memory[canonicalAvailabilityFactsInvalidatedMetadataKey] = true
	metadata["memory"] = memory
	session.Metadata = metadata
	store.sessions[sessionID] = session
}

func markCanonicalAvailabilityInvalidatedAfterMessageForTest(store *fakeStore, sessionID string, boundary Message) {
	markCanonicalAvailabilityInvalidatedForTest(store, sessionID)
	session := store.sessions[sessionID]
	metadata := cloneMap(session.Metadata)
	for _, key := range []string{"agent", "memory"} {
		root := cloneMap(asMap(metadata[key]))
		root[canonicalAvailabilityFactsInvalidatedAfterMessageIDMetadataKey] = boundary.ID
		root[canonicalAvailabilityFactsInvalidatedAfterCreatedAtMetadataKey] = canonicalAvailabilityHistoryMessageTime(boundary).Format(time.RFC3339Nano)
		metadata[key] = root
	}
	session.Metadata = metadata
	store.sessions[sessionID] = session
}

func assertCanonicalAvailabilityInvalidatedForTest(t *testing.T, state CanonicalConversationState, label string) {
	t.Helper()
	if state.Route.SelectedOptionIndex != 0 ||
		strings.TrimSpace(state.Route.TripID) != "" ||
		strings.TrimSpace(state.Route.BoardStopID) != "" ||
		strings.TrimSpace(state.Route.AlightStopID) != "" ||
		strings.TrimSpace(state.Route.TripDate) != "" ||
		strings.TrimSpace(state.Route.DepartureTime) != "" ||
		state.Route.Price != 0 ||
		strings.TrimSpace(state.Route.Currency) != "" ||
		strings.TrimSpace(state.Route.PackageName) != "" {
		t.Fatalf("%s retained stale selected route: %+v", label, state.Route)
	}
	if availability := asMap(state.LastToolFacts[toolNameAvailabilitySearch]); len(availability) != 0 {
		t.Fatalf("%s retained stale availability facts: %+v", label, availability)
	}
}

func assertAvailabilityInvalidatedRouterHistoryForTest(t *testing.T, history []Message) {
	t.Helper()
	for i, message := range history {
		if messageLooksLikeAvailabilitySelectionPrompt(message) {
			t.Fatalf("router history[%d] retained stale availability prompt: %+v", i, message)
		}
		for label, payload := range map[string]map[string]interface{}{
			"payload": message.Payload, "normalized_payload": message.NormalizedPayload,
		} {
			assertAvailabilityInvalidatedRouterPayloadForTest(t, payload, fmt.Sprintf("history[%d].%s", i, label))
		}
	}
}

func assertAvailabilityInvalidatedRouterPayloadForTest(t *testing.T, payload map[string]interface{}, label string) {
	t.Helper()
	if len(payload) == 0 {
		return
	}
	assertNoRouteIDKeysAtAnyDepthForTest(t, payload, label)
	if index := asInt(payload["selected_option_index"]); index != 0 {
		t.Fatalf("%s retained stale selected_option_index=%d: %+v", label, index, payload)
	}
	if snapshot := asMap(payload[selectedAvailabilityResultPayloadKey]); len(snapshot) != 0 {
		t.Fatalf("%s retained stale selected_availability_result: %+v", label, snapshot)
	}
	if availability := asMap(asMap(payload["tool_context"])[toolNameAvailabilitySearch]); len(availability) != 0 {
		t.Fatalf("%s retained stale availability_search: %+v", label, availability)
	}
	for _, key := range []string{"request_payload", "response_payload"} {
		assertAvailabilityInvalidatedRouterPayloadForTest(t, asMap(payload[key]), label+"."+key)
	}
}

func assertNoRouteIDKeysAtAnyDepthForTest(t *testing.T, value interface{}, label string) {
	t.Helper()
	forbidden := map[string]struct{}{
		"trip_id": {}, "board_stop_id": {}, "alight_stop_id": {},
	}
	var visit func(reflect.Value, string)
	visit = func(node reflect.Value, path string) {
		for node.IsValid() && (node.Kind() == reflect.Interface || node.Kind() == reflect.Ptr) {
			if node.IsNil() {
				return
			}
			node = node.Elem()
		}
		if !node.IsValid() {
			return
		}
		switch node.Kind() {
		case reflect.Map:
			iterator := node.MapRange()
			for iterator.Next() {
				key := fmt.Sprint(iterator.Key().Interface())
				childPath := path + "." + key
				if _, blocked := forbidden[key]; blocked {
					t.Fatalf("%s retained forbidden route key %q", childPath, key)
				}
				visit(iterator.Value(), childPath)
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < node.Len(); i++ {
				visit(node.Index(i), fmt.Sprintf("%s[%d]", path, i))
			}
		}
	}
	visit(reflect.ValueOf(value), label)
}

func assertStringValuesAbsentAtAnyDepthForTest(t *testing.T, value interface{}, label string, forbiddenValues ...string) {
	t.Helper()
	forbidden := make(map[string]struct{}, len(forbiddenValues))
	for _, value := range forbiddenValues {
		forbidden[value] = struct{}{}
	}
	var visit func(reflect.Value, string)
	visit = func(node reflect.Value, path string) {
		for node.IsValid() && (node.Kind() == reflect.Interface || node.Kind() == reflect.Ptr) {
			if node.IsNil() {
				return
			}
			node = node.Elem()
		}
		if !node.IsValid() {
			return
		}
		switch node.Kind() {
		case reflect.Map:
			iterator := node.MapRange()
			for iterator.Next() {
				key := fmt.Sprint(iterator.Key().Interface())
				visit(iterator.Value(), path+"."+key)
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < node.Len(); i++ {
				visit(node.Index(i), fmt.Sprintf("%s[%d]", path, i))
			}
		case reflect.String:
			if _, blocked := forbidden[node.String()]; blocked {
				t.Fatalf("%s retained stale route value %q", path, node.String())
			}
		}
	}
	visit(reflect.ValueOf(value), label)
}

func canonicalConversationStateFromTestValue(t *testing.T, value interface{}, label string) CanonicalConversationState {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal %s: %v", label, err)
	}
	var state CanonicalConversationState
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatalf("decode %s: %v", label, err)
	}
	return state
}

func seedAvailabilitySelectionPromptForFailClosedTest(
	t *testing.T,
	store *fakeStore,
	sessionID string,
	key string,
	availability AvailabilitySearchResult,
	processingStatus string,
	withFacts bool,
	recordedAt time.Time,
) {
	t.Helper()
	payload := map[string]interface{}{}
	if withFacts {
		payload["tool_context"] = map[string]interface{}{
			toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availability),
		}
	}
	if _, err := store.SaveAgentDraft(context.Background(), SaveAgentDraftInput{
		SessionID: sessionID, IdempotencyKey: "draft-fail-closed-" + key,
		Body: buildAvailabilityListReply(availability), SenderName: "SHABAS",
		ProcessingStatus: processingStatus, Payload: payload, RecordedAt: recordedAt,
	}); err != nil {
		t.Fatalf("seed %s availability prompt: %v", key, err)
	}
}

func TestAvailabilityDateSelectionWithHiddenRawPrefixKeepsVisibleTripFacts(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	paymentSearcher := &fakePaymentStatusSearcher{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher, paymentSearcher)

	now := availabilityTestObservedAt()
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
	availability := availabilityOptionPromptHiddenPastPrefixResultAt(now)
	hiddenPast := availability.Results[0]
	visibleFuture := availability.Results[1]
	visibleDate := availabilityTestDateInput(t, visibleFuture.TripDate)
	if _, err := store.SaveAgentDraft(context.Background(), SaveAgentDraftInput{
		SessionID:        session.ID,
		IdempotencyKey:   "draft-availability-hidden-raw-prefix-date-selection",
		Body:             buildAvailabilityListReply(availability),
		SenderName:       "SHABAS",
		ProcessingStatus: messageStatusAutomationSent,
		Payload: map[string]interface{}{
			"tool_context": map[string]interface{}{
				toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availability),
			},
		},
		RecordedAt: now.Add(-1 * time.Minute),
	}); err != nil {
		t.Fatalf("seed availability draft: %v", err)
	}

	out := ingestAndReprocessActivePromptFlowTurn(t, svc, session.ContactKey, "hidden-raw-prefix-date-selection", visibleDate+", paga agora?")
	if out.Draft == nil {
		t.Fatal("expected selection draft")
	}
	if got := strings.TrimSpace(out.Draft.Body); !strings.Contains(got, "documento de 1 passageiro") {
		t.Fatalf("expected document request, got %q", got)
	}
	intentDecision := asMap(out.Memory["intent_decision"])
	if got := asInt(intentDecision["selected_option_index"]); got != 1 {
		t.Fatalf("expected visible selected option index 1, got %d memory=%+v", got, intentDecision)
	}
	canonicalState, ok := out.Memory["canonical_state"].(CanonicalConversationState)
	if !ok {
		t.Fatalf("expected canonical_state in memory, got %#v", out.Memory["canonical_state"])
	}
	if canonicalState.Route.TripID != visibleFuture.TripID ||
		canonicalState.Route.TripID == hiddenPast.TripID {
		t.Fatalf("expected canonical route to use visible trip, got %+v", canonicalState.Route)
	}
	toolContext := asMap(out.Draft.Payload["tool_context"])
	availabilityPayload := asMap(toolContext[toolNameAvailabilitySearch])
	results := asInterfaceSliceMaps(availabilityPayload["results"])
	if len(results) != 1 {
		t.Fatalf("expected filtered availability facts with 1 visible result, got %d payload=%+v", len(results), availabilityPayload)
	}
	if got := strings.TrimSpace(asString(results[0]["trip_id"])); got != visibleFuture.TripID {
		t.Fatalf("expected filtered facts to start with visible trip, got trip_id=%q payload=%+v", got, availabilityPayload)
	}
	if runner.calls != 0 || searcher.calls != 0 || paymentSearcher.calls != 0 || len(out.ToolCalls) != 0 {
		t.Fatalf("expected no LLM or tool calls, runner=%d availability=%d payment_status=%d tool_calls=%+v", runner.calls, searcher.calls, paymentSearcher.calls, out.ToolCalls)
	}
}

func TestAvailabilityDateSelectionPersistsOptionForKnownPassengerContinuation(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	paymentSearcher := &fakePaymentStatusSearcher{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher, paymentSearcher)

	now := availabilityTestObservedAt()
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
	availability := availabilityOptionPromptTwoOptionsFutureResultAt(now)
	selectedOption := availability.Results[1]
	selectedDate := availabilityTestDateInput(t, selectedOption.TripDate)
	if _, err := store.SaveAgentDraft(context.Background(), SaveAgentDraftInput{
		SessionID:        session.ID,
		IdempotencyKey:   "draft-availability-two-options-date-selection",
		Body:             buildAvailabilityListReply(availability),
		SenderName:       "SHABAS",
		ProcessingStatus: messageStatusAutomationSent,
		Payload: map[string]interface{}{
			"tool_context": map[string]interface{}{
				toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availability),
			},
		},
		RecordedAt: now.Add(-1 * time.Minute),
	}); err != nil {
		t.Fatalf("seed availability draft: %v", err)
	}

	first := ingestAndReprocessActivePromptFlowTurn(t, svc, session.ContactKey, "two-options-date-selection", selectedDate+", paga agora?")
	if first.Draft == nil {
		t.Fatal("expected selection draft")
	}
	if got := strings.TrimSpace(first.Draft.Body); !strings.Contains(got, "documento de 1 passageiro") {
		t.Fatalf("expected document request for known passenger state, got %q", got)
	}
	if got := asInt(first.Draft.Payload["selected_option_index"]); got != 2 {
		t.Fatalf("expected draft payload selected_option_index=2, got %d payload=%+v", got, first.Draft.Payload)
	}
	if got := asInt(first.Draft.NormalizedPayload["selected_option_index"]); got != 2 {
		t.Fatalf("expected normalized selected_option_index=2, got %d payload=%+v", got, first.Draft.NormalizedPayload)
	}
	firstSnapshot := asMap(first.Draft.Payload[selectedAvailabilityResultPayloadKey])
	if got := asInt(firstSnapshot["selected_option_index"]); got != 2 {
		t.Fatalf("expected selected availability snapshot index 2, got %d snapshot=%+v", got, firstSnapshot)
	}
	if got := strings.TrimSpace(asString(firstSnapshot["trip_id"])); got != selectedOption.TripID {
		t.Fatalf("expected selected availability snapshot %s, got %q snapshot=%+v", selectedOption.TripID, got, firstSnapshot)
	}
	normalizedSnapshot := asMap(first.Draft.NormalizedPayload[selectedAvailabilityResultPayloadKey])
	if got := strings.TrimSpace(asString(normalizedSnapshot["trip_id"])); got != selectedOption.TripID {
		t.Fatalf("expected normalized selected availability snapshot %s, got %q snapshot=%+v", selectedOption.TripID, got, normalizedSnapshot)
	}
	firstAvailability := asMap(asMap(first.Draft.Payload["tool_context"])[toolNameAvailabilitySearch])
	firstResults := asInterfaceSliceMaps(firstAvailability["results"])
	if len(firstResults) != 2 || strings.TrimSpace(asString(firstResults[1]["trip_id"])) != selectedOption.TripID {
		t.Fatalf("expected selection draft to carry visible options with %s at index 2, got %+v", selectedDate, firstAvailability)
	}
	if runner.calls != 0 || searcher.calls != 0 || paymentSearcher.calls != 0 || len(first.ToolCalls) != 0 {
		t.Fatalf("expected no LLM or tool calls, runner=%d availability=%d payment_status=%d tool_calls=%+v", runner.calls, searcher.calls, paymentSearcher.calls, first.ToolCalls)
	}
}

func TestSelectedAvailabilitySelectionPassengerFailClosedPersistsBeyondHistoryWindow(t *testing.T) {
	base := newFakeStore()
	store := newFakeTravelQueryV2ShadowClaimStore()
	store.fakeStore = base
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "must not run", Model: "gpt-test"}}
	jsonRunner := &fakeJSONDecisionRunner{enabled: true}
	openAI := &fakeOpenAIInterpreter{enabled: true}
	travel := &fakeOpenAITravelQueryV2Interpreter{enabled: true}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	booking := &fakeBookingCreator{enabled: true}
	payment := &fakePaymentCreator{enabled: true}
	svc := NewService(store, config.Config{
		ChatDebounceWindowMS:               1500,
		ChatOpenAIInterpreterShadowEnabled: true,
		ChatOpenAITravelV2ShadowEnabled:    true,
		ChatAgentMode:                      chatAgentModeHybridJSON,
	}, runner, jsonRunner, openAI, travel, searcher, booking, payment)

	now := availabilityTestObservedAt()
	session, err := base.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:        "WHATSAPP",
		ContactKey:     "5511999999998",
		CustomerPhone:  "5511999999998",
		LastMessageAt:  &now,
		LastOutboundAt: &now,
	})
	if err != nil {
		t.Fatalf("seed fail-closed selection session: %v", err)
	}
	availability := availabilityOptionPromptTwoOptionsFutureResultAt(now)
	selected := availability.Results[1]
	if _, err := base.SaveAgentDraft(context.Background(), SaveAgentDraftInput{
		SessionID:        session.ID,
		IdempotencyKey:   "draft-passenger-fail-closed-two-options",
		Body:             buildAvailabilityListReply(availability),
		SenderName:       "SHABAS",
		ProcessingStatus: messageStatusAutomationSent,
		Payload: map[string]interface{}{
			"tool_context": map[string]interface{}{
				toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availability),
			},
		},
		RecordedAt: now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("seed current two-option availability: %v", err)
	}

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "passenger-fail-closed-option-2",
			IdempotencyKey:    "passenger-fail-closed-option-2",
			Body:              "opcao 2",
		},
	})
	if err != nil {
		t.Fatalf("ingest option 2 without passenger authority: %v", err)
	}
	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess option 2 without passenger authority: %v", err)
	}
	if out.Draft == nil {
		t.Fatal("expected fail-closed passenger draft")
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskPassengerCount) {
		t.Fatalf("expected %s, got %q payload=%+v", TemplateAskPassengerCount, got, out.Draft.NormalizedPayload)
	}
	expectedSnapshot := selectedAvailabilityResultPayloadFromAvailability(&availability, 2)
	for label, payload := range map[string]map[string]interface{}{
		"payload":            out.Draft.Payload,
		"normalized_payload": out.Draft.NormalizedPayload,
	} {
		if got := asInt(payload["selected_option_index"]); got != 2 {
			t.Fatalf("%s selected_option_index=%d, want 2: %+v", label, got, payload)
		}
		snapshot := asMap(payload[selectedAvailabilityResultPayloadKey])
		if !reflect.DeepEqual(snapshot, expectedSnapshot) {
			t.Fatalf("%s must preserve the complete selected option snapshot: got=%+v want=%+v", label, snapshot, expectedSnapshot)
		}
	}
	availabilityFacts := asMap(asMap(out.Draft.Payload["tool_context"])[toolNameAvailabilitySearch])
	if want := buildAvailabilityToolResponsePayload(availability); !reflect.DeepEqual(availabilityFacts, want) {
		t.Fatalf("draft must preserve current visible availability facts: got=%+v want=%+v", availabilityFacts, want)
	}
	persistedSelectionSession, err := base.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("reload complete fail-closed selection: %v", err)
	}
	persistedSelectionState := canonicalConversationStateFromTestValue(
		t,
		asMap(persistedSelectionSession.Metadata["agent"])["canonical_state"],
		"complete metadata.agent.canonical_state",
	)
	if persistedSelectionState.Route.SelectedOptionIndex != 2 ||
		persistedSelectionState.Route.TripID != selected.TripID ||
		persistedSelectionState.Route.BoardStopID != selected.BoardStopID ||
		persistedSelectionState.Route.AlightStopID != selected.AlightStopID {
		t.Fatalf("complete selection route was not persisted canonically: %+v", persistedSelectionState.Route)
	}
	if facts := asMap(persistedSelectionState.LastToolFacts[toolNameAvailabilitySearch]); len(facts) == 0 {
		t.Fatalf("complete selection availability facts were not persisted canonically: %+v", persistedSelectionState.LastToolFacts)
	}
	if got := strings.TrimSpace(asString(expectedSnapshot["trip_id"])); got != selected.TripID ||
		strings.TrimSpace(asString(expectedSnapshot["board_stop_id"])) != selected.BoardStopID ||
		strings.TrimSpace(asString(expectedSnapshot["alight_stop_id"])) != selected.AlightStopID {
		t.Fatalf("selected snapshot lost route IDs: %+v", expectedSnapshot)
	}
	if runner.calls != 0 || jsonRunner.calls != 0 || openAI.calls != 0 || travel.calls != 0 ||
		searcher.calls != 0 || booking.calls != 0 || payment.calls != 0 || len(out.ToolCalls) != 0 {
		t.Fatalf("fail-closed selection dispatched external work: runner=%d json=%d shadow=%d travel=%d availability=%d booking=%d payment=%d tools=%+v",
			runner.calls, jsonRunner.calls, openAI.calls, travel.calls, searcher.calls, booking.calls, payment.calls, out.ToolCalls)
	}
	select {
	case <-store.claimAttempts:
		t.Fatal("fail-closed selection scheduled Travel V2 shadow")
	case <-time.After(25 * time.Millisecond):
	}

	markSessionMessagesAutomationSent(t, base, session.ID)
	for i := 0; i < 49; i++ {
		if _, err := base.CreateMessage(context.Background(), CreateMessageInput{
			SessionID: session.ID, Direction: "OUTBOUND", Kind: "TEXT", Body: "window filler",
			ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(time.Duration(i+1) * time.Second),
		}); err != nil {
			t.Fatalf("create history filler %d: %v", i, err)
		}
	}
	history, err := base.ListMessages(context.Background(), session.ID, ListMessagesFilter{})
	if err != nil {
		t.Fatalf("list fail-closed selection history: %v", err)
	}
	if len(history) < 50 {
		t.Fatalf("expected at least 50 messages, got %d", len(history))
	}
	history = history[len(history)-50:]
	for _, message := range history {
		if message.ID == ingested.Message.ID {
			t.Fatalf("selection inbound %s must be outside the bounded history", ingested.Message.ID)
		}
	}
	latestSession, err := base.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("reload fail-closed selection session: %v", err)
	}
	projection := collectBookingDraftContextFromState(latestSession, history, "")
	if projection.SelectedOptionIndex != 2 || projection.TripID != selected.TripID ||
		projection.BoardStopID != selected.BoardStopID || projection.AlightStopID != selected.AlightStopID ||
		projection.TripDate != selected.TripDate || projection.DepartureTime != selected.OriginDepartTime ||
		projection.Price != selected.Price || projection.Currency != selected.Currency {
		t.Fatalf("selected option must survive after the inbound leaves LIMIT 50: %+v", projection)
	}
}

func TestAvailabilityDateSelectionAfterOutOfTurnReminderPersistsOptionForKnownPassengerContinuation(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	paymentSearcher := &fakePaymentStatusSearcher{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher, paymentSearcher)

	now := availabilityTestObservedAt()
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
	availability := availabilityOptionPromptTwoOptionsFutureResultAt(now)
	selectedOption := availability.Results[1]
	selectedDate := availabilityTestDateInput(t, selectedOption.TripDate)
	if _, err := store.SaveAgentDraft(context.Background(), SaveAgentDraftInput{
		SessionID:        session.ID,
		IdempotencyKey:   "draft-availability-two-options-before-reminder",
		Body:             buildAvailabilityListReply(availability),
		SenderName:       "SHABAS",
		ProcessingStatus: messageStatusAutomationSent,
		Payload: map[string]interface{}{
			"tool_context": map[string]interface{}{
				toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availability),
			},
		},
		RecordedAt: now.Add(-2 * time.Minute),
	}); err != nil {
		t.Fatalf("seed availability draft: %v", err)
	}

	first := ingestAndReprocessActivePromptFlowTurn(t, svc, session.ContactKey, "two-options-payment-reminder", "paga agora?")
	if first.Draft == nil {
		t.Fatal("expected payment reminder draft")
	}
	if got := strings.TrimSpace(asString(first.Draft.NormalizedPayload["template_name"])); got != string(TemplatePaymentOptionsInfo) {
		t.Fatalf("expected payment info template, got %q payload=%+v", got, first.Draft.NormalizedPayload)
	}
	firstAvailability := asMap(asMap(first.Draft.Payload["tool_context"])[toolNameAvailabilitySearch])
	firstResults := asInterfaceSliceMaps(firstAvailability["results"])
	if len(firstResults) != 2 || strings.TrimSpace(asString(firstResults[1]["trip_id"])) != selectedOption.TripID {
		t.Fatalf("expected reminder to carry two visible options with %s at index 2, got %+v", selectedDate, firstAvailability)
	}
	markSessionMessagesAutomationSent(t, store, session.ID)

	second := ingestAndReprocessActivePromptFlowTurn(t, svc, session.ContactKey, "two-options-selection-after-reminder", selectedDate)
	if second.Draft == nil {
		t.Fatal("expected selection draft")
	}
	if got := strings.TrimSpace(second.Draft.Body); !strings.Contains(got, "documento de 1 passageiro") {
		t.Fatalf("expected document request after reminder selection, got %q", got)
	}
	if got := asInt(second.Draft.NormalizedPayload["selected_option_index"]); got != 2 {
		t.Fatalf("expected selected option index 2 after reminder, got %d payload=%+v", got, second.Draft.NormalizedPayload)
	}
	secondSnapshot := asMap(second.Draft.NormalizedPayload[selectedAvailabilityResultPayloadKey])
	if got := strings.TrimSpace(asString(secondSnapshot["trip_id"])); got != selectedOption.TripID {
		t.Fatalf("expected selected trip snapshot after reminder, got %q snapshot=%+v", got, secondSnapshot)
	}
	if runner.calls != 0 || searcher.calls != 0 || paymentSearcher.calls != 0 || len(second.ToolCalls) != 0 {
		t.Fatalf("expected no LLM or tool calls through selection, runner=%d availability=%d payment_status=%d tool_calls=%+v", runner.calls, searcher.calls, paymentSearcher.calls, second.ToolCalls)
	}
}

func TestAvailabilitySelectionReplacesExistingCanonicalRoute(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	paymentSearcher := &fakePaymentStatusSearcher{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher, paymentSearcher)

	now := availabilityTestObservedAt()
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
	seedPassengerClarificationStateV1ForTest(store, session.ID, completePassengerStateForTest(1, 0))
	oldAvailability := availabilityOptionPromptFutureResultAt(now)
	oldSelection := selectedAvailabilityResultPayloadFromAvailability(&oldAvailability, 1)
	if _, err := store.SaveAgentDraft(context.Background(), SaveAgentDraftInput{
		SessionID:        session.ID,
		IdempotencyKey:   "draft-old-selected-route",
		Body:             askPassengerCountReply,
		SenderName:       "SHABAS",
		ProcessingStatus: messageStatusAutomationSent,
		Payload: map[string]interface{}{
			"intent":                             string(IntentSelectAvailabilityOption),
			"template_name":                      string(TemplateAskPassengerCount),
			"selected_option_index":              1,
			selectedAvailabilityResultPayloadKey: oldSelection,
		},
		RecordedAt: now.Add(-4 * time.Minute),
	}); err != nil {
		t.Fatalf("seed old selected route draft: %v", err)
	}

	availability := availabilityOptionPromptTwoOptionsFutureResultAt(now)
	selectedOption := availability.Results[1]
	selectedDate := availabilityTestDateInput(t, selectedOption.TripDate)
	if _, err := store.SaveAgentDraft(context.Background(), SaveAgentDraftInput{
		SessionID:        session.ID,
		IdempotencyKey:   "draft-new-availability-two-options",
		Body:             buildAvailabilityListReply(availability),
		SenderName:       "SHABAS",
		ProcessingStatus: messageStatusAutomationSent,
		Payload: map[string]interface{}{
			"tool_context": map[string]interface{}{
				toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availability),
			},
		},
		RecordedAt: now.Add(-2 * time.Minute),
	}); err != nil {
		t.Fatalf("seed new availability draft: %v", err)
	}

	out := ingestAndReprocessActivePromptFlowTurn(t, svc, session.ContactKey, "replace-old-route-with-new-selection", selectedDate+", paga agora?")
	if out.Draft == nil {
		t.Fatal("expected selection draft")
	}
	canonicalState, ok := out.Memory["canonical_state"].(CanonicalConversationState)
	if !ok {
		t.Fatalf("expected canonical_state in memory, got %#v", out.Memory["canonical_state"])
	}
	if canonicalState.Route.SelectedOptionIndex != 2 ||
		canonicalState.Route.TripID != selectedOption.TripID ||
		canonicalState.Route.BoardStopID != selectedOption.BoardStopID ||
		canonicalState.Route.AlightStopID != selectedOption.AlightStopID ||
		canonicalState.Route.TripDate != selectedOption.TripDate ||
		canonicalState.Route.DepartureTime != selectedOption.OriginDepartTime {
		t.Fatalf("expected new selection to replace old canonical route atomically, got %+v", canonicalState.Route)
	}
	snapshot := asMap(out.Draft.NormalizedPayload[selectedAvailabilityResultPayloadKey])
	if got := strings.TrimSpace(asString(snapshot["trip_id"])); got != selectedOption.TripID {
		t.Fatalf("expected draft selected snapshot to use new trip, got %q snapshot=%+v", got, snapshot)
	}
	if runner.calls != 0 || searcher.calls != 0 || paymentSearcher.calls != 0 || len(out.ToolCalls) != 0 {
		t.Fatalf("expected no LLM or tool calls, runner=%d availability=%d payment_status=%d tool_calls=%+v", runner.calls, searcher.calls, paymentSearcher.calls, out.ToolCalls)
	}
}

func TestAvailabilitySelectionContinuesAfterOutOfTurnPaymentReminder(t *testing.T) {
	testCases := []struct {
		name    string
		deictic bool
	}{
		{name: "essa_msm", deictic: true},
		{name: "data_visivel"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			store := newFakeStoreWithPassengerAuthority()
			runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
			searcher := &fakeAvailabilitySearcher{enabled: true}
			paymentSearcher := &fakePaymentStatusSearcher{enabled: true}
			svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher, paymentSearcher)

			now := availabilityTestObservedAt()
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
			availability := availabilityOptionPromptHiddenPastPrefixResultAt(now)
			hiddenPast := availability.Results[0]
			visibleFuture := availability.Results[1]
			reply := availabilityTestDateInput(t, visibleFuture.TripDate)
			if testCase.deictic {
				reply = "essa msm"
			}
			if _, err := store.SaveAgentDraft(context.Background(), SaveAgentDraftInput{
				SessionID:        session.ID,
				IdempotencyKey:   "draft-availability-before-out-of-turn-" + testCase.name,
				Body:             buildAvailabilityListReply(availability),
				SenderName:       "SHABAS",
				ProcessingStatus: messageStatusAutomationSent,
				Payload: map[string]interface{}{
					"tool_context": map[string]interface{}{
						toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availability),
					},
				},
				RecordedAt: now.Add(-2 * time.Minute),
			}); err != nil {
				t.Fatalf("seed availability draft: %v", err)
			}

			first := ingestAndReprocessActivePromptFlowTurn(t, svc, session.ContactKey, "out-of-turn-payment-"+testCase.name, "paga agora?")
			if first.Draft == nil {
				t.Fatal("expected out-of-turn payment draft")
			}
			if got := strings.TrimSpace(asString(first.Draft.NormalizedPayload["template_name"])); got != string(TemplatePaymentOptionsInfo) {
				t.Fatalf("expected payment info template, got %q payload=%+v", got, first.Draft.NormalizedPayload)
			}
			if len(asMap(asMap(first.Draft.Payload["tool_context"])[toolNameAvailabilitySearch])) == 0 {
				t.Fatalf("expected payment reminder draft to carry availability context, got %+v", first.Draft.Payload)
			}
			firstAvailability := asMap(asMap(first.Draft.Payload["tool_context"])[toolNameAvailabilitySearch])
			firstResults := asInterfaceSliceMaps(firstAvailability["results"])
			if len(firstResults) != 1 || strings.TrimSpace(asString(firstResults[0]["trip_id"])) != visibleFuture.TripID {
				t.Fatalf("expected payment reminder to carry filtered visible availability context, got %+v", firstAvailability)
			}
			markSessionMessagesAutomationSent(t, store, session.ID)

			second := ingestAndReprocessActivePromptFlowTurn(t, svc, session.ContactKey, "selection-after-reminder-"+reply, reply)
			if second.Draft == nil {
				t.Fatal("expected selection draft")
			}
			if got := strings.TrimSpace(second.Draft.Body); !strings.Contains(got, "documento de 1 passageiro") {
				t.Fatalf("expected document request after %q, got %q", reply, got)
			}
			if got := strings.TrimSpace(asString(second.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskDocuments) {
				t.Fatalf("expected template %s, got %q payload=%+v", TemplateAskDocuments, got, second.Draft.NormalizedPayload)
			}
			intentDecision := asMap(second.Memory["intent_decision"])
			if got := asInt(intentDecision["selected_option_index"]); got != 1 {
				t.Fatalf("expected selected option index 1 after reminder, got %d memory=%+v", got, intentDecision)
			}
			canonicalState, ok := second.Memory["canonical_state"].(CanonicalConversationState)
			if !ok {
				t.Fatalf("expected canonical_state in memory, got %#v", second.Memory["canonical_state"])
			}
			if canonicalState.Route.TripID != visibleFuture.TripID ||
				canonicalState.Route.TripID == hiddenPast.TripID {
				t.Fatalf("expected selected route to use visible trip after %q, got %+v", reply, canonicalState.Route)
			}
			secondAvailability := asMap(asMap(second.Draft.Payload["tool_context"])[toolNameAvailabilitySearch])
			secondResults := asInterfaceSliceMaps(secondAvailability["results"])
			if len(secondResults) != 1 || strings.TrimSpace(asString(secondResults[0]["trip_id"])) != visibleFuture.TripID {
				t.Fatalf("expected selection draft to carry filtered visible availability context, got %+v", secondAvailability)
			}
			if runner.calls != 0 || searcher.calls != 0 || paymentSearcher.calls != 0 || len(second.ToolCalls) != 0 {
				t.Fatalf("expected no LLM or tool calls, runner=%d availability=%d payment_status=%d tool_calls=%+v", runner.calls, searcher.calls, paymentSearcher.calls, second.ToolCalls)
			}
		})
	}
}

func TestBookingCannotCreateWithoutPassengers(t *testing.T) {
	availability := AvailabilitySearchResult{
		Results: []AvailabilitySearchItem{{
			TripID: "trip-1", BoardStopID: "board-1", AlightStopID: "alight-1",
			OriginDisplayName: "Fraiburgo/SC", DestinationDisplayName: "Moncao/MA",
		}},
	}
	session := sessionWithPassengerClarificationStateForTest(Session{}, unknownPassengerStateForTest())
	input, ok := parseBookingCreateInput(session, nil, "quero reservar opcao 1", &availability)
	if ok {
		t.Fatalf("expected booking create to be blocked without passengers, got %+v", input)
	}
}

func assertAvailabilityTemplateDraft(t *testing.T, draft Message) {
	t.Helper()
	if !strings.Contains(draft.Body, "Chapeco/SC para Moncao/MA") {
		t.Fatalf("expected availability route in template draft, got %q", draft.Body)
	}
	if !strings.Contains(draft.Body, "2026-05-10") || !strings.Contains(draft.Body, "18:30") {
		t.Fatalf("expected availability date/time in template draft, got %q", draft.Body)
	}
	if got := strings.TrimSpace(asString(draft.NormalizedPayload["model"])); got != "template_realizer" {
		t.Fatalf("expected template model, got %q", got)
	}
	if got := strings.TrimSpace(asString(draft.NormalizedPayload["run_mode"])); got != "TEMPLATE_FIRST_REPLY" {
		t.Fatalf("expected template run mode, got %q", got)
	}
	if got := strings.TrimSpace(asString(draft.NormalizedPayload["intent"])); got != string(IntentAvailabilitySearch) {
		t.Fatalf("expected availability intent, got %q", got)
	}
	if got := strings.TrimSpace(asString(draft.NormalizedPayload["template_name"])); got != string(TemplateAvailabilityList) {
		t.Fatalf("expected availability template, got %q", got)
	}
	if got := readInt(draft.NormalizedPayload["tool_call_count"]); got != 1 {
		t.Fatalf("expected tool_call_count=1, got %d", got)
	}
}

func reprocessAvailabilitySelection(t *testing.T, customerText string) (ReprocessResult, *fakeAgentRunner, *fakeAvailabilitySearcher) {
	t.Helper()
	out, runner, searcher, _ := reprocessAvailabilitySelectionWithBookingCreator(t, customerText)
	return out, runner, searcher
}

func reprocessAvailabilitySelectionWithBookingCreator(t *testing.T, customerText string) (ReprocessResult, *fakeAgentRunner, *fakeAvailabilitySearcher, *fakeBookingCreator) {
	t.Helper()
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"},
	}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	creator := &fakeBookingCreator{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher, creator)

	now := time.Now().UTC()
	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:       "WHATSAPP",
		ContactKey:    "5511999999999",
		CustomerPhone: "5511999999999",
		LastMessageAt: &now,
		LastInboundAt: &now,
	})
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}
	_, err = store.SaveAgentDraft(context.Background(), SaveAgentDraftInput{
		SessionID:        session.ID,
		IdempotencyKey:   "draft-prev-availability-selection",
		Body:             "Tenho duas opcoes para essa data.",
		SenderName:       "SHABAS",
		ProcessingStatus: messageStatusAutomationSent,
		Payload: map[string]interface{}{
			"tool_context": map[string]interface{}{
				toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(AvailabilitySearchResult{
					Filter: AvailabilitySearchInput{Origin: "Fraiburgo/SC", Destination: "Moncao/MA", Qty: 1, Limit: 5},
					Results: []AvailabilitySearchItem{{
						TripID: "trip-1", BoardStopID: "board-1", AlightStopID: "alight-1",
						OriginDisplayName: "Fraiburgo/SC", DestinationDisplayName: "Moncao/MA",
						OriginDepartTime: "09:00", TripDate: "2026-05-18", SeatsAvailable: 4,
						Price: 950, Currency: "BRL", Status: "ACTIVE", TripStatus: "SCHEDULED",
					}},
				}),
			},
		},
		RecordedAt: now.Add(-2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("seed availability draft: %v", err)
	}
	_, err = svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-select-" + customerText,
			IdempotencyKey:    "idem-select-" + customerText,
			Body:              customerText,
		},
	})
	if err != nil {
		t.Fatalf("ingest selection: %v", err)
	}
	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess: %v", err)
	}
	return out, runner, searcher, creator
}

func ingestAndReprocessActivePromptFlowTurn(t *testing.T, svc *Service, contactKey string, key string, body string) ReprocessResult {
	t.Helper()
	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: contactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-" + key,
			IdempotencyKey:    "idem-" + key,
			Body:              body,
		},
	})
	if err != nil {
		t.Fatalf("ingest %q: %v", body, err)
	}
	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess %q: %v", body, err)
	}
	return out
}

func seedOutboundDraftSent(t *testing.T, store *fakeStore, sessionID string, draft Message, at time.Time) {
	t.Helper()
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:         sessionID,
		Direction:         "OUTBOUND",
		Kind:              "TEXT",
		Body:              draft.Body,
		Payload:           draft.Payload,
		NormalizedPayload: draft.NormalizedPayload,
		ProcessingStatus:  messageStatusAutomationSent,
		ReceivedAt:        at,
	}); err != nil {
		t.Fatalf("seed outbound draft %q: %v", draft.Body, err)
	}
}

func availabilitySelectionHistory(t *testing.T) []Message {
	t.Helper()
	now := time.Now().UTC()
	return []Message{
		{
			Direction:        "OUTBOUND",
			Body:             "Tenho duas opcoes para essa data.",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-2 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(AvailabilitySearchResult{
						Filter: AvailabilitySearchInput{Origin: "Fraiburgo/SC", Destination: "Moncao/MA", Qty: 1, Limit: 5},
						Results: []AvailabilitySearchItem{
							{
								TripID: "trip-1", BoardStopID: "board-1", AlightStopID: "alight-1",
								OriginDisplayName: "Fraiburgo/SC", DestinationDisplayName: "Moncao/MA",
								OriginDepartTime: "09:00", TripDate: "2026-05-18", SeatsAvailable: 4,
								Price: 950, Currency: "BRL", Status: "ACTIVE", TripStatus: "SCHEDULED",
							},
							{
								TripID: "trip-2", BoardStopID: "board-2", AlightStopID: "alight-2",
								OriginDisplayName: "Fraiburgo/SC", DestinationDisplayName: "Moncao/MA",
								OriginDepartTime: "14:00", TripDate: "2026-05-18", SeatsAvailable: 6,
								Price: 950, Currency: "BRL", Status: "ACTIVE", TripStatus: "SCHEDULED",
							},
						},
					}),
				},
			},
		},
	}
}

func activePromptFlowAvailabilityResult() AvailabilitySearchResult {
	return AvailabilitySearchResult{
		Filter: AvailabilitySearchInput{
			Origin:      "Moncao/MA",
			Destination: "Videira/SC",
			PackageName: packageToSantaCatarina,
			Qty:         1,
			Limit:       5,
		},
		Results: []AvailabilitySearchItem{{
			TripID:                 "trip-active-prompt-2026-07-06",
			BoardStopID:            "board-active-prompt-2026-07-06",
			AlightStopID:           "alight-active-prompt-2026-07-06",
			OriginDisplayName:      "Moncao/MA",
			DestinationDisplayName: "Videira/SC",
			OriginDepartTime:       "09:00",
			TripDate:               "2026-07-06",
			SeatsAvailable:         5,
			Price:                  950,
			Currency:               "BRL",
			Status:                 "ACTIVE",
			TripStatus:             "SCHEDULED",
			PackageName:            packageToSantaCatarina,
		}},
	}
}

type availabilitySearchOrderGuard struct {
	t         *testing.T
	runner    *fakeAgentRunner
	enabled   bool
	calls     int
	result    AvailabilitySearchResult
	lastInput AvailabilitySearchInput
}

func (s *availabilitySearchOrderGuard) Enabled() bool {
	return s != nil && s.enabled
}

func (s *availabilitySearchOrderGuard) Search(_ context.Context, input AvailabilitySearchInput) (AvailabilitySearchResult, error) {
	if s.runner != nil && s.runner.calls != 0 {
		s.t.Fatalf("expected availability search to run before LLM, runner already called %d times", s.runner.calls)
	}
	s.calls++
	s.lastInput = input
	result := s.result
	if result.Filter == (AvailabilitySearchInput{}) {
		result.Filter = input
	}
	return result, nil
}

func prepareMaranhaoDestinationFollowUp(t *testing.T, svc *Service, store *fakeStore) Session {
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
		Body:             "Perfeito. De qual cidade de Santa Catarina voce vai sair para o Maranhao?",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       now.Add(-4 * time.Minute),
	}); err != nil {
		t.Fatalf("seed SC origin question: %v", err)
	}
	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-ma-origin-1",
			IdempotencyKey:    "idem-ma-origin-1",
			Body:              "Chapeco",
		},
	}); err != nil {
		t.Fatalf("ingest SC origin answer: %v", err)
	}
	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess SC origin answer: %v", err)
	}
	if out.Draft == nil || strings.TrimSpace(out.Draft.Body) != "Perfeito — Chapeco/SC. Para qual cidade do Maranhao voce quer ir?" {
		t.Fatalf("expected Maranhão destination question, got %+v", out.Draft)
	}
	return session
}

func seedMAOriginQuestionAfterPublicSCTable(t *testing.T, store *fakeStore) Session {
	t.Helper()
	session := seedPublicSCTableContext(t, store)
	now := time.Now().UTC()
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        session.ID,
		Direction:        "OUTBOUND",
		Kind:             "TEXT",
		Body:             "Perfeito — Videira/SC. De qual cidade do Maranhao voce vai sair?",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       now.Add(-1 * time.Minute),
	}); err != nil {
		t.Fatalf("seed MA origin question: %v", err)
	}
	return session
}

func assertUnsupportedPackageDraftWithoutTools(t *testing.T, out ReprocessResult, runner *fakeAgentRunner, searcher *fakeAvailabilitySearcher) {
	t.Helper()
	if runner.calls != 0 {
		t.Fatalf("expected unsupported template to avoid LLM, got %d calls", runner.calls)
	}
	if searcher.calls != 0 || len(out.ToolCalls) != 0 {
		t.Fatalf("expected no availability search for unsupported follow-up, searcher=%d tool_calls=%d", searcher.calls, len(out.ToolCalls))
	}
	if out.Draft == nil {
		t.Fatal("expected unsupported package draft")
	}
	expected := buildUnsupportedPackageReply()
	if got := strings.TrimSpace(out.Draft.Body); got != expected {
		t.Fatalf("expected unsupported package reply %q, got %q", expected, got)
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got != string(TemplateUnsupportedPackage) {
		t.Fatalf("expected template %s, got %q", TemplateUnsupportedPackage, got)
	}
}

func seedMaranhaoOriginQuestionFromBroadQuery(t *testing.T, svc *Service) Session {
	t.Helper()
	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-broad-ma-seed-1",
			IdempotencyKey:    "idem-broad-ma-seed-1",
			Body:              "Passagem para o Maranhao",
		},
	})
	if err != nil {
		t.Fatalf("ingest broad MA seed: %v", err)
	}
	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess broad MA seed: %v", err)
	}
	if out.Draft == nil || !strings.Contains(out.Draft.Body, "cidade de Santa Catarina") {
		t.Fatalf("expected SC origin question seed, got %+v", out.Draft)
	}
	return ingested.Session
}
