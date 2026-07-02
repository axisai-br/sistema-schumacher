package chat

import (
	"context"
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

func TestSelectAvailabilityOptionPrimeiraAsksPassengerCount(t *testing.T) {
	out, runner, searcher := reprocessAvailabilitySelection(t, "primeira")
	if out.Draft == nil {
		t.Fatal("expected draft")
	}
	if got := strings.TrimSpace(out.Draft.Body); got != askPassengerCountReply {
		t.Fatalf("expected passenger question %q, got %q", askPassengerCountReply, got)
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

func TestSelectAvailabilityOptionWithIntentPhraseAsksPassengerCount(t *testing.T) {
	out, runner, searcher, creator := reprocessAvailabilitySelectionWithBookingCreator(t, "quero a primeira opcao")
	if out.Draft == nil {
		t.Fatal("expected draft")
	}
	if got := strings.TrimSpace(out.Draft.Body); got != askPassengerCountReply {
		t.Fatalf("expected passenger question %q, got %q", askPassengerCountReply, got)
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskPassengerCount) {
		t.Fatalf("expected template %s, got %q", TemplateAskPassengerCount, got)
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

func TestSelectAvailabilityOptionContextualConfirmationsAskPassengerCount(t *testing.T) {
	for _, text := range []string{"isso msm", "essa msm"} {
		t.Run(text, func(t *testing.T) {
			assertContextualAvailabilitySelectionAsksPassengerCount(t, text)
		})
	}
}

func TestAvailabilityOptionEssaMsmRenderedSingleOptionWithStaleFactsUsesFallback(t *testing.T) {
	store := newFakeStore()
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

func assertContextualAvailabilitySelectionAsksPassengerCount(t *testing.T, text string) {
	t.Helper()
	store := newFakeStore()
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
	if got := strings.TrimSpace(out.Draft.Body); got != askPassengerCountReply {
		t.Fatalf("expected passenger question %q, got %q", askPassengerCountReply, got)
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["intent"])); got != string(IntentSelectAvailabilityOption) {
		t.Fatalf("expected selected availability intent, got %q payload=%+v", got, out.Draft.NormalizedPayload)
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskPassengerCount) {
		t.Fatalf("expected template %s, got %q", TemplateAskPassengerCount, got)
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
	store := newFakeStore()
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
			store := newFakeStore()
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
	store := newFakeStore()
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

	second := ingestAndReprocessActivePromptFlowTurn(t, svc, contactKey, "active-prompt-flow-route", "de monção pra videira")
	if second.Draft == nil {
		t.Fatal("expected availability draft after route")
	}
	seedOutboundDraftSent(t, store, sessionID, *second.Draft, now.Add(2*time.Minute))

	third := ingestAndReprocessActivePromptFlowTurn(t, svc, contactKey, "active-prompt-flow-date", "06/07")
	if third.Draft == nil {
		t.Fatal("expected availability draft after date")
	}
	seedOutboundDraftSent(t, store, sessionID, *third.Draft, now.Add(3*time.Minute))

	out := ingestAndReprocessActivePromptFlowTurn(t, svc, contactKey, "active-prompt-flow-option", "essa mesmo")
	if out.Draft == nil {
		t.Fatal("expected passenger count draft")
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["intent"])); got != string(IntentSelectAvailabilityOption) {
		t.Fatalf("expected selected availability intent, got %q payload=%+v", got, out.Draft.NormalizedPayload)
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskPassengerCount) {
		t.Fatalf("expected template %s, got %q", TemplateAskPassengerCount, got)
	}
	if got := strings.TrimSpace(out.Draft.Body); got != askPassengerCountReply {
		t.Fatalf("expected passenger count reply %q, got %q", askPassengerCountReply, got)
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
	store := newFakeStore()
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
			store := newFakeStore()
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
	store := newFakeStore()
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
	store := newFakeStore()
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
	store := newFakeStore()
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
	store := newFakeStore()
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
	store := newFakeStore()
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
	store := newFakeStore()
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
	store := newFakeStore()
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
	store := newFakeStore()
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

	store := newFakeStore()
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

	store := newFakeStore()
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

	store := newFakeStore()
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

	store := newFakeStore()
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

	store := newFakeStore()
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
	store := newFakeStore()
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
	store := newFakeStore()
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
	store := newFakeStore()
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
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	session := seedPassengerCountContext(t, store, "Tem crianca de 5 anos ou menos viajando?")

	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-sem-crianca-1",
			IdempotencyKey:    "idem-sem-crianca-1",
			Body:              "não, sem criança",
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
	store := newFakeStore()
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

func TestBookingCannotCreateWithoutPassengers(t *testing.T) {
	availability := AvailabilitySearchResult{
		Results: []AvailabilitySearchItem{{
			TripID: "trip-1", BoardStopID: "board-1", AlightStopID: "alight-1",
			OriginDisplayName: "Fraiburgo/SC", DestinationDisplayName: "Moncao/MA",
		}},
	}
	input, ok := parseBookingCreateInput(Session{}, nil, "quero reservar opcao 1", &availability)
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
	store := newFakeStore()
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
