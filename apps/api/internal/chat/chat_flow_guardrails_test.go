package chat

import (
	"context"
	"strings"
	"testing"
	"time"

	"schumacher-tur/api/internal/shared/config"
)

func TestGuardrailHowToReserveAfterAvailabilityDoesNotUseUnsupportedOrLLM(t *testing.T) {
	out, runner, searcher := reprocessAvailabilitySelection(t, "Como posso fazer para reservar?")
	if out.Draft == nil {
		t.Fatal("expected draft")
	}
	body := strings.TrimSpace(out.Draft.Body)
	if body == "" {
		t.Fatal("expected non-empty reply")
	}
	assertNoForbiddenSchedulingVocabulary(t, body)
	if strings.TrimSpace(body) == buildUnsupportedPackageReply() {
		t.Fatalf("expected reservation next step, got unsupported package reply: %q", body)
	}
	if strings.Contains(strings.ToLower(body), "fora") || strings.Contains(strings.ToLower(body), "outras rotas") {
		t.Fatalf("expected no out-of-scope reply, got %q", body)
	}
	if !strings.Contains(foldChatText(body), "opcao") && !strings.Contains(foldChatText(body), "opcoes") && !strings.Contains(foldChatText(body), "passagem e so para voce") {
		t.Fatalf("expected option selection or passenger-count next step, got %q", body)
	}
	if runner.calls != 0 {
		t.Fatalf("expected deterministic reservation reply to avoid LLM, got %d calls", runner.calls)
	}
	if searcher.calls != 0 {
		t.Fatalf("expected no new availability search, got %d calls", searcher.calls)
	}
}

func TestGuardrailAgendarCadeiraDoesNotAdvanceToPassengers(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
	session := seedRouteQuestionAfterCustomerAgendar(t, store)

	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-cadeira-1",
			IdempotencyKey:    "idem-cadeira-1",
			Body:              "Cadeira",
		},
	}); err != nil {
		t.Fatalf("ingest cadeira: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess cadeira: %v", err)
	}
	if out.Draft == nil {
		t.Fatal("expected draft")
	}
	body := strings.TrimSpace(out.Draft.Body)
	assertNoForbiddenSchedulingVocabulary(t, body)
	if strings.Contains(foldChatText(body), "passagem e so para voce") || strings.Contains(foldChatText(body), "vai mais alguem") {
		t.Fatalf("expected no passenger question before route/trip, got %q", body)
	}
	folded := foldChatText(body)
	if !strings.Contains(folded, "origem") && !strings.Contains(folded, "destino") && !strings.Contains(folded, "data") &&
		!strings.Contains(folded, "cidade") {
		t.Fatalf("expected safe route/date clarification, got %q", body)
	}
	if runner.calls != 0 {
		t.Fatalf("expected safe fallback to avoid LLM, got %d calls", runner.calls)
	}
}

func TestGuardrailVerifyAllOptionsWithCompleteContextCallsAvailability(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "vou verificar", Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{
		enabled: true,
		result: AvailabilitySearchResult{
			Results: []AvailabilitySearchItem{{
				TripID: "trip-jul-1", BoardStopID: "board-jul-1", AlightStopID: "alight-jul-1",
				OriginDisplayName: "Santa Ines/MA", DestinationDisplayName: "Videira/SC",
				OriginDepartTime: "08:00", TripDate: "2026-07-10", SeatsAvailable: 5,
				Price: 950, Currency: "BRL", Status: "ACTIVE", TripStatus: "SCHEDULED",
			}},
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
	session := seedVerifyAllOptionsContext(t, store)

	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-all-options-1",
			IdempotencyKey:    "idem-all-options-1",
			Body:              "Verificar todas as opções",
		},
	}); err != nil {
		t.Fatalf("ingest verify all options: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess verify all options: %v", err)
	}
	if searcher.calls != 1 {
		t.Fatalf("expected one availability search, got %d", searcher.calls)
	}
	if runner.calls != 0 {
		t.Fatalf("expected availability template to avoid LLM, got %d calls", runner.calls)
	}
	if len(out.ToolCalls) != 1 || out.ToolCalls[0].ToolName != toolNameAvailabilitySearch {
		t.Fatalf("expected availability_search tool call, got %+v", out.ToolCalls)
	}
	if out.Draft == nil {
		t.Fatal("expected draft")
	}
	body := strings.TrimSpace(out.Draft.Body)
	if strings.Contains(foldChatText(body), "vou verificar") {
		t.Fatalf("expected no promise without result, got %q", body)
	}
	if !strings.Contains(body, "Encontrei estas opcoes") {
		t.Fatalf("expected availability result template, got %q", body)
	}
}

func TestGuardrailVerifyAllOptionsWithMissingOriginAsksOnlyMissingField(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "vou verificar", Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
	session := seedVerifyAllOptionsMissingOriginContext(t, store)

	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-all-options-missing-origin-1",
			IdempotencyKey:    "idem-all-options-missing-origin-1",
			Body:              "pode verificar todas",
		},
	}); err != nil {
		t.Fatalf("ingest verify all options missing origin: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess verify all options missing origin: %v", err)
	}
	if searcher.calls != 0 || len(out.ToolCalls) != 0 {
		t.Fatalf("expected no availability search before origin, searcher=%d tool_calls=%d", searcher.calls, len(out.ToolCalls))
	}
	if runner.calls != 0 {
		t.Fatalf("expected safe missing-field template to avoid LLM, got %d calls", runner.calls)
	}
	if out.Draft == nil {
		t.Fatal("expected draft")
	}
	body := strings.TrimSpace(out.Draft.Body)
	if !strings.Contains(foldChatText(body), "cidade") && !strings.Contains(foldChatText(body), "origem") {
		t.Fatalf("expected origin question, got %q", body)
	}
	if strings.Contains(foldChatText(body), "vou verificar") {
		t.Fatalf("expected no promise without tool, got %q", body)
	}
}

func TestGuardrailAntiLoopBlocksRepeatedFreeFormQuestion(t *testing.T) {
	store := newFakeStore()
	sender := &fakeReplySender{enabled: true, result: SendReplyResult{ProviderMessageID: "provider-loop-1", ProviderStatus: "SENT"}}
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: "Voce prefere que eu mostre as opcoes por assento, por classe ou por horario?", Model: "gpt-test"},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, sender, runner)
	session := seedRepeatedOutOfDomainQuestion(t, store)

	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-loop-1",
			IdempotencyKey:    "idem-loop-1",
			Body:              "??",
		},
	}); err != nil {
		t.Fatalf("ingest loop reply: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess loop reply: %v", err)
	}
	if out.Draft == nil {
		t.Fatal("expected draft")
	}
	body := strings.TrimSpace(out.Draft.Body)
	assertNoForbiddenSchedulingVocabulary(t, body)
	if strings.Contains(foldChatText(body), "assento") || strings.Contains(foldChatText(body), "classe") {
		t.Fatalf("expected fallback to replace loop vocabulary, got %q", body)
	}
}

func TestGuardrailAdvancedPhasesUseSafeFallbackInsteadOfFreeFormLLM(t *testing.T) {
	tests := []struct {
		name        string
		seed        func(t *testing.T, store *fakeStore) Session
		wantFolded  string
		notContains string
	}{
		{name: "route_selection", seed: seedRouteSelectionPhase, wantFolded: "data"},
		{name: "trip_selection", seed: seedTripSelectionPhase, wantFolded: "opcao"},
		{name: "passenger_collection", seed: seedPassengerCollectionPhase, wantFolded: "passagem"},
		{name: "booking_pending", seed: seedBookingPendingPhase, wantFolded: "conferem"},
		{name: "booked", seed: seedBookedPhase, wantFolded: "valor integral"},
		{name: "payment_pending", seed: seedPaymentPendingPhase, wantFolded: "pagamento"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeStore()
			runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
			svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
			session := tc.seed(t, store)
			if _, err := svc.Ingest(context.Background(), IngestMessageInput{
				ContactKey: session.ContactKey,
				Message: IngestMessagePayload{
					Direction:         "INBOUND",
					ProviderMessageID: "msg-advanced-" + tc.name,
					IdempotencyKey:    "idem-advanced-" + tc.name,
					Body:              "??",
				},
			}); err != nil {
				t.Fatalf("ingest advanced fallback turn: %v", err)
			}

			out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
			if err != nil {
				t.Fatalf("reprocess advanced fallback: %v", err)
			}
			if runner.calls != 0 {
				t.Fatalf("expected safe fallback to avoid free-form LLM, got %d calls", runner.calls)
			}
			if out.Draft == nil {
				t.Fatal("expected draft")
			}
			body := strings.TrimSpace(out.Draft.Body)
			assertNoForbiddenSchedulingVocabulary(t, body)
			if !strings.Contains(foldChatText(body), tc.wantFolded) {
				t.Fatalf("expected safe fallback containing %q, got %q", tc.wantFolded, body)
			}
		})
	}
}

func TestGuardrailHumanSupportIntentBypassesSafeFallbackInAdvancedPhase(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	session := seedPassengerCollectionPhase(t, store)

	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-human-support-1",
			IdempotencyKey:    "idem-human-support-1",
			Body:              "quero falar com atendente",
		},
	}); err != nil {
		t.Fatalf("ingest human support request: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess human support request: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected human support template to avoid LLM, got %d calls", runner.calls)
	}
	if out.Draft == nil {
		t.Fatal("expected draft")
	}
	body := strings.TrimSpace(out.Draft.Body)
	folded := foldChatText(body)
	if !strings.Contains(folded, "atendente") {
		t.Fatalf("expected human handoff reply, got %q", body)
	}
	if strings.Contains(folded, "passagem e so para voce") || strings.Contains(folded, "opcao de viagem") {
		t.Fatalf("expected no booking prompt after human support request, got %q", body)
	}
}

func TestGuardrailHumanSupportAlguemBypassesSafeFallbackInRouteSelection(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	session := seedRouteSelectionPhase(t, store)

	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-human-alguem-route-1",
			IdempotencyKey:    "idem-human-alguem-route-1",
			Body:              "falar com alguém",
		},
	}); err != nil {
		t.Fatalf("ingest human support alguem request: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess human support alguem request: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected human support template to avoid LLM, got %d calls", runner.calls)
	}
	if out.Draft == nil {
		t.Fatal("expected draft")
	}
	body := strings.TrimSpace(out.Draft.Body)
	folded := foldChatText(body)
	if !strings.Contains(folded, "atendente") {
		t.Fatalf("expected human handoff reply, got %q", body)
	}
	if strings.Contains(folded, "data") ||
		strings.Contains(folded, "origem") ||
		strings.Contains(folded, "destino") ||
		strings.Contains(folded, "passagem e so para voce") {
		t.Fatalf("expected no safe reservation fallback after human support request, got %q", body)
	}
}

func TestGuardrailHumanSupportAlguemIntentInPassengerCollection(t *testing.T) {
	got := routeDeterministicIntent(
		nil,
		"quero falar com alguem",
		CanonicalConversationState{Phase: ConversationPhasePassengerCollection},
		time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC),
	)
	if got.Intent != IntentHumanSupport {
		t.Fatalf("expected human support intent, got %+v", got)
	}
	if got.TemplateName != TemplateHumanHandoff || got.Action != "template" {
		t.Fatalf("expected human handoff template decision, got %+v", got)
	}
}

func TestGuardrailHumanSupportPessoaInBookedDoesNotAskPayment(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	session := seedBookedPhase(t, store)

	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-human-pessoa-booked-1",
			IdempotencyKey:    "idem-human-pessoa-booked-1",
			Body:              "preciso falar com uma pessoa",
		},
	}); err != nil {
		t.Fatalf("ingest human support pessoa request: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess human support pessoa request: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected human support template to avoid LLM, got %d calls", runner.calls)
	}
	if out.Draft == nil {
		t.Fatal("expected draft")
	}
	body := strings.TrimSpace(out.Draft.Body)
	folded := foldChatText(body)
	if !strings.Contains(folded, "atendente") {
		t.Fatalf("expected human handoff reply, got %q", body)
	}
	if strings.Contains(folded, "valor integral") ||
		strings.Contains(folded, "sinal") ||
		strings.Contains(folded, "pagamento") ||
		strings.Contains(folded, "passagem e so para voce") {
		t.Fatalf("expected no payment or booking prompt after human support request, got %q", body)
	}
}

func TestGuardrailHumanSupportDoesNotCapturePassengerAlguem(t *testing.T) {
	text := "eu e mais alguém junto"
	folded := strings.Join(strings.Fields(foldChatText(text)), " ")
	if looksLikeHumanSupportIntent(folded) {
		t.Fatalf("did not expect passenger context to route to human support: %q", text)
	}
	got := routeDeterministicIntent(
		nil,
		text,
		CanonicalConversationState{Phase: ConversationPhasePassengerCollection},
		time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC),
	)
	if got.Intent == IntentHumanSupport {
		t.Fatalf("did not expect human support intent for passenger context, got %+v", got)
	}
}

func TestGuardrailForbiddenSchedulingVocabularyIsNormalized(t *testing.T) {
	for _, text := range []string{
		"assento, classe ou horário",
		"assento classe ou horario",
		"por assento, por classe ou por horário",
		"por assento por classe ou por horario",
		"qual passeio/serviço",
		"qual passeio ou serviço",
		"agendar horário",
	} {
		t.Run(text, func(t *testing.T) {
			if !containsOutOfDomainSchedulingVocabulary(text) {
				t.Fatalf("expected forbidden vocabulary to be detected: %q", text)
			}
			policy := evaluateDraftAutoSendPolicy(nil, nil, text)
			if policy.Status != draftAutoSendStatusReviewNeeded {
				t.Fatalf("expected auto-send review for forbidden vocabulary, got %+v", policy)
			}
		})
	}
}

func TestGuardrailOutOfDomainVocabularyDraftIsReplacedWithoutLoop(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: "Voce prefere que eu mostre as opcoes por assento, por classe ou por horário?", Model: "gpt-test"},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	now := time.Now().UTC()
	session := seedSessionOnly(t, store, now)

	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-forbidden-vocab-1",
			IdempotencyKey:    "idem-forbidden-vocab-1",
			Body:              "??",
		},
	}); err != nil {
		t.Fatalf("ingest forbidden vocabulary turn: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess forbidden vocabulary turn: %v", err)
	}
	if runner.calls != 1 {
		t.Fatalf("expected LLM draft to be generated then replaced, got %d calls", runner.calls)
	}
	if out.Draft == nil {
		t.Fatal("expected draft")
	}
	body := strings.TrimSpace(out.Draft.Body)
	assertNoForbiddenSchedulingVocabulary(t, body)
	if strings.Contains(foldChatText(body), "assento") || strings.Contains(foldChatText(body), "classe") {
		t.Fatalf("expected unsafe draft to be replaced, got %q", body)
	}
}

func TestGuardrailVagueAgendarStillUsesSafeFallback(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
	session := seedSessionOnly(t, store, time.Now().UTC())

	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-vague-agendar-1",
			IdempotencyKey:    "idem-vague-agendar-1",
			Body:              "Agendar",
		},
	}); err != nil {
		t.Fatalf("ingest vague agendar: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess vague agendar: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected vague agendar safe fallback to avoid LLM, got %d calls", runner.calls)
	}
	if searcher.calls != 0 {
		t.Fatalf("expected vague agendar not to call availability, got %d calls", searcher.calls)
	}
	if out.Draft == nil {
		t.Fatal("expected draft")
	}
	body := strings.TrimSpace(out.Draft.Body)
	folded := foldChatText(body)
	if !strings.Contains(folded, "origem") && !strings.Contains(folded, "destino") && !strings.Contains(folded, "data") {
		t.Fatalf("expected safe route fallback, got %q", body)
	}
	if strings.Contains(folded, "passagem e so para voce") {
		t.Fatalf("expected no passenger question for vague agendar, got %q", body)
	}
}

func TestGuardrailExplicitAgendarRouteUsesAvailabilitySearch(t *testing.T) {
	tests := []string{
		"quero agendar de Santa Ines para Videira em 10/07",
		"quero reservar de Fraiburgo para Moncao dia 15/06",
	}
	for _, body := range tests {
		t.Run(body, func(t *testing.T) {
			store := newFakeStore()
			runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "resultado da disponibilidade", Model: "gpt-test"}}
			searcher := &fakeAvailabilitySearcher{
				enabled: true,
				result: AvailabilitySearchResult{
					Results: []AvailabilitySearchItem{{
						TripID: "trip-explicit-1", BoardStopID: "board-explicit-1", AlightStopID: "alight-explicit-1",
						OriginDisplayName: "Santa Ines/MA", DestinationDisplayName: "Videira/SC",
						OriginDepartTime: "08:00", TripDate: "2026-07-10", SeatsAvailable: 5,
						Price: 950, Currency: "BRL", Status: "ACTIVE", TripStatus: "SCHEDULED",
					}},
				},
			}
			svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
			session := seedSessionOnly(t, store, time.Now().UTC())

			if _, err := svc.Ingest(context.Background(), IngestMessageInput{
				ContactKey: session.ContactKey,
				Message: IngestMessagePayload{
					Direction:         "INBOUND",
					ProviderMessageID: "msg-explicit-route-" + strings.ReplaceAll(foldChatText(body), " ", "-"),
					IdempotencyKey:    "idem-explicit-route-" + strings.ReplaceAll(foldChatText(body), " ", "-"),
					Body:              body,
				},
			}); err != nil {
				t.Fatalf("ingest explicit route: %v", err)
			}

			out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
			if err != nil {
				t.Fatalf("reprocess explicit route: %v", err)
			}
			if searcher.calls != 1 {
				t.Fatalf("expected explicit route to call availability, got %d calls", searcher.calls)
			}
			if len(out.ToolCalls) == 0 || out.ToolCalls[0].ToolName != toolNameAvailabilitySearch {
				t.Fatalf("expected availability_search tool call, got %+v", out.ToolCalls)
			}
			if out.Draft == nil {
				t.Fatal("expected draft")
			}
			if strings.Contains(foldChatText(out.Draft.Body), "preciso da cidade de origem destino e data") {
				t.Fatalf("expected no generic booking fallback for explicit route, got %q", out.Draft.Body)
			}
		})
	}
}

func TestGuardrailPaymentMethodQuestionsUsePixSupportTemplate(t *testing.T) {
	for _, body := range []string{
		"aceita cartão?",
		"posso parcelar?",
	} {
		t.Run(body, func(t *testing.T) {
			store := newFakeStore()
			runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
			paymentCreator := &fakePaymentCreator{enabled: true}
			svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, paymentCreator)
			session := seedBookedPhase(t, store)

			if _, err := svc.Ingest(context.Background(), IngestMessageInput{
				ContactKey: session.ContactKey,
				Message: IngestMessagePayload{
					Direction:         "INBOUND",
					ProviderMessageID: "msg-payment-method-" + strings.ReplaceAll(foldChatText(body), " ", "-"),
					IdempotencyKey:    "idem-payment-method-" + strings.ReplaceAll(foldChatText(body), " ", "-"),
					Body:              body,
				},
			}); err != nil {
				t.Fatalf("ingest payment method question: %v", err)
			}

			out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
			if err != nil {
				t.Fatalf("reprocess payment method question: %v", err)
			}
			if paymentCreator.calls != 0 {
				t.Fatalf("expected no payment_create for unsupported payment method, got %d calls", paymentCreator.calls)
			}
			if runner.calls != 0 {
				t.Fatalf("expected payment method template to avoid LLM, got %d calls", runner.calls)
			}
			if len(out.ToolCalls) != 0 {
				t.Fatalf("expected no tool call for payment method question, got %+v", out.ToolCalls)
			}
			if out.Draft == nil {
				t.Fatal("expected draft")
			}
			if strings.TrimSpace(out.Draft.Body) != paymentMethodsSupportReply {
				t.Fatalf("expected PIX support template, got %q", out.Draft.Body)
			}
		})
	}
}

func TestGuardrailPixPaymentStillAllowsPaymentCreate(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	paymentCreator := &fakePaymentCreator{
		enabled: true,
		result: PaymentCreateResult{
			Mode:            "pix_sent",
			BookingID:       "booking-1",
			ReservationCode: "ABC12345",
			PaymentType:     "sinal",
			PixCode:         "000201PIX",
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, paymentCreator)
	session := seedBookedPhase(t, store)

	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-payment-pix-1",
			IdempotencyKey:    "idem-payment-pix-1",
			Body:              "quero pagar no pix",
		},
	}); err != nil {
		t.Fatalf("ingest pix payment: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess pix payment: %v", err)
	}
	if paymentCreator.calls != 1 {
		t.Fatalf("expected payment_create for PIX, got %d calls", paymentCreator.calls)
	}
	if len(out.ToolCalls) != 1 || out.ToolCalls[0].ToolName != toolNamePaymentCreate {
		t.Fatalf("expected payment_create tool call, got %+v", out.ToolCalls)
	}
	if out.Draft == nil || strings.TrimSpace(out.Draft.Body) != "000201PIX" {
		t.Fatalf("expected PIX draft, got %+v", out.Draft)
	}
}

func TestGuardrailPaymentAmountChoiceStillAllowsPaymentCreate(t *testing.T) {
	tests := []struct {
		name            string
		body            string
		wantPaymentType string
	}{
		{name: "integral", body: "integral", wantPaymentType: "integral"},
		{name: "sinal", body: "sinal", wantPaymentType: "sinal"},
		{name: "so_o_sinal", body: "só o sinal", wantPaymentType: "sinal"},
		{name: "apenas_o_sinal", body: "apenas o sinal", wantPaymentType: "sinal"},
		{name: "quero_pagar_o_sinal", body: "quero pagar o sinal", wantPaymentType: "sinal"},
		{name: "vou_pagar_so_o_sinal", body: "vou pagar só o sinal", wantPaymentType: "sinal"},
		{name: "sinal_por_passageiro_pagante", body: "sinal por passageiro pagante", wantPaymentType: "sinal"},
		{name: "o_valor_integral", body: "o valor integral", wantPaymentType: "integral"},
		{name: "quero_pagar_integral", body: "quero pagar integral", wantPaymentType: "integral"},
		{name: "vou_pagar_tudo_agora", body: "vou pagar tudo agora", wantPaymentType: "integral"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeStore()
			runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
			paymentCreator := &fakePaymentCreator{
				enabled: true,
				result: PaymentCreateResult{
					Mode:            "pix_sent",
					BookingID:       "booking-1",
					ReservationCode: "ABC12345",
					PaymentType:     tc.wantPaymentType,
					PixCode:         "000201PIX" + strings.ToUpper(tc.wantPaymentType),
				},
			}
			svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, paymentCreator)
			session := seedBookedPhase(t, store)
			seedOutboundSent(t, store, session.ID, askPaymentChoiceReply, time.Now().UTC().Add(-30*time.Second))

			if _, err := svc.Ingest(context.Background(), IngestMessageInput{
				ContactKey: session.ContactKey,
				Message: IngestMessagePayload{
					Direction:         "INBOUND",
					ProviderMessageID: "msg-payment-amount-" + tc.name,
					IdempotencyKey:    "idem-payment-amount-" + tc.name,
					Body:              tc.body,
				},
			}); err != nil {
				t.Fatalf("ingest payment amount choice: %v", err)
			}

			out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
			if err != nil {
				t.Fatalf("reprocess payment amount choice: %v", err)
			}
			if paymentCreator.calls != 1 {
				t.Fatalf("expected payment_create for amount choice, got %d calls", paymentCreator.calls)
			}
			if paymentCreator.lastInput.PaymentType != tc.wantPaymentType {
				t.Fatalf("expected payment type %q, got %+v", tc.wantPaymentType, paymentCreator.lastInput)
			}
			if len(out.ToolCalls) != 1 || out.ToolCalls[0].ToolName != toolNamePaymentCreate {
				t.Fatalf("expected payment_create tool call, got %+v", out.ToolCalls)
			}
		})
	}
}

func seedRouteQuestionAfterCustomerAgendar(t *testing.T, store *fakeStore) Session {
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
		Direction:        "INBOUND",
		Kind:             "TEXT",
		Body:             "Agendar",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       now.Add(-3 * time.Minute),
	}); err != nil {
		t.Fatalf("seed agendar: %v", err)
	}
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        session.ID,
		Direction:        "OUTBOUND",
		Kind:             "TEXT",
		Body:             "Para verificar a viagem, preciso da cidade de origem, destino e data.",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       now.Add(-2 * time.Minute),
	}); err != nil {
		t.Fatalf("seed route question: %v", err)
	}
	return session
}

func seedVerifyAllOptionsContext(t *testing.T, store *fakeStore) Session {
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
	seedInboundSent(t, store, session.ID, "E as datas disponíveis pro mês de julho", now.Add(-4*time.Minute))
	seedOutboundSent(t, store, session.ID, "Para qual cidade de Santa Catarina voce quer ir?", now.Add(-3*time.Minute))
	seedInboundSent(t, store, session.ID, "Destino-videira", now.Add(-2*time.Minute))
	seedOutboundSent(t, store, session.ID, "Perfeito. De qual cidade do Maranhao voce vai sair?", now.Add(-1*time.Minute))
	seedInboundSent(t, store, session.ID, "Santa Ines", now.Add(-30*time.Second))
	return session
}

func seedVerifyAllOptionsMissingOriginContext(t *testing.T, store *fakeStore) Session {
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
	seedInboundSent(t, store, session.ID, "E as datas disponíveis pro mês de julho", now.Add(-3*time.Minute))
	seedOutboundSent(t, store, session.ID, "Para qual cidade de Santa Catarina voce quer ir?", now.Add(-2*time.Minute))
	seedInboundSent(t, store, session.ID, "Destino-videira", now.Add(-1*time.Minute))
	return session
}

func seedRepeatedOutOfDomainQuestion(t *testing.T, store *fakeStore) Session {
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
	seedOutboundSent(t, store, session.ID, "Voce prefere que eu mostre as opcoes por assento, por classe ou por horario?", now.Add(-1*time.Minute))
	return session
}

func seedRouteSelectionPhase(t *testing.T, store *fakeStore) Session {
	t.Helper()
	now := time.Now().UTC()
	session := seedSessionOnly(t, store, now)
	seedInboundSent(t, store, session.ID, "Quero passagem para Videira", now.Add(-2*time.Minute))
	seedOutboundSent(t, store, session.ID, "De qual cidade do Maranhao voce vai sair?", now.Add(-1*time.Minute))
	return session
}

func seedTripSelectionPhase(t *testing.T, store *fakeStore) Session {
	t.Helper()
	now := time.Now().UTC()
	session := seedSessionOnly(t, store, now)
	seedAvailabilityOutbound(t, store, session.ID, now.Add(-1*time.Minute))
	return session
}

func seedPassengerCollectionPhase(t *testing.T, store *fakeStore) Session {
	t.Helper()
	now := time.Now().UTC()
	session := seedSessionOnly(t, store, now)
	seedAvailabilityOutbound(t, store, session.ID, now.Add(-3*time.Minute))
	seedInboundSent(t, store, session.ID, "opcao 1", now.Add(-2*time.Minute))
	seedOutboundSent(t, store, session.ID, askPassengerCountReply, now.Add(-1*time.Minute))
	return session
}

func seedBookingPendingPhase(t *testing.T, store *fakeStore) Session {
	t.Helper()
	now := time.Now().UTC()
	session := seedSessionOnly(t, store, now)
	seedAvailabilityOutbound(t, store, session.ID, now.Add(-5*time.Minute))
	seedOutboundSent(t, store, session.ID, askPassengerCountReply, now.Add(-4*time.Minute))
	seedInboundSent(t, store, session.ID, "sou eu mesmo, sem crianca", now.Add(-3*time.Minute))
	seedOutboundSent(t, store, session.ID, "Perfeito. Agora pode enviar seu nome completo e o documento.", now.Add(-2*time.Minute))
	seedInboundSent(t, store, session.ID, "Joao Vitor Messias CPF 52998224725", now.Add(-1*time.Minute))
	return session
}

func seedBookedPhase(t *testing.T, store *fakeStore) Session {
	t.Helper()
	now := time.Now().UTC()
	session := seedSessionOnly(t, store, now)
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        session.ID,
		Direction:        "OUTBOUND",
		Kind:             "TEXT",
		Body:             "Reserva criada com sucesso.",
		ProcessingStatus: messageStatusAutomationSent,
		Payload: map[string]interface{}{
			"tool_context": map[string]interface{}{
				toolNameBookingCreate: buildBookingCreateResponsePayload(BookingCreateResult{
					BookingID: "booking-1", ReservationCode: "ABC12345", Status: "PENDING_PAYMENT",
					Filter: BookingCreateInput{Qty: 1},
				}),
			},
		},
		ReceivedAt: now.Add(-1 * time.Minute),
	}); err != nil {
		t.Fatalf("seed booking create: %v", err)
	}
	return session
}

func seedPaymentPendingPhase(t *testing.T, store *fakeStore) Session {
	t.Helper()
	now := time.Now().UTC()
	session := seedBookedPhase(t, store)
	seedOutboundSent(t, store, session.ID, askPaymentChoiceReply, now.Add(-45*time.Second))
	seedInboundSent(t, store, session.ID, "sinal", now.Add(-30*time.Second))
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        session.ID,
		Direction:        "OUTBOUND",
		Kind:             "TEXT",
		Body:             "Pagamento gerado.",
		ProcessingStatus: messageStatusAutomationSent,
		Payload: map[string]interface{}{
			"tool_context": map[string]interface{}{
				toolNamePaymentCreate: buildPaymentCreateResponsePayload(PaymentCreateResult{
					BookingID: "booking-1", ReservationCode: "ABC12345", PaymentStatus: "PENDING", PaymentType: "DEPOSIT",
				}),
			},
		},
		ReceivedAt: now.Add(-15 * time.Second),
	}); err != nil {
		t.Fatalf("seed payment create: %v", err)
	}
	return session
}

func seedSessionOnly(t *testing.T, store *fakeStore, now time.Time) Session {
	t.Helper()
	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:       "WHATSAPP",
		ContactKey:    "5511999999999",
		CustomerPhone: "5511999999999",
		LastMessageAt: &now,
	})
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}
	return session
}

func seedAvailabilityOutbound(t *testing.T, store *fakeStore, sessionID string, at time.Time) {
	t.Helper()
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        sessionID,
		Direction:        "OUTBOUND",
		Kind:             "TEXT",
		Body:             "Encontrei estas opcoes:\n1. Santa Ines/MA para Videira/SC, 2026-07-10, saida 08:00, R$ 950\n\nQual opcao voce prefere?",
		ProcessingStatus: messageStatusAutomationSent,
		Payload: map[string]interface{}{
			"tool_context": map[string]interface{}{
				toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(AvailabilitySearchResult{
					Filter: AvailabilitySearchInput{Origin: "Santa Ines/MA", Destination: "Videira/SC", PackageName: packageToSantaCatarina, Qty: 1, Limit: 5},
					Results: []AvailabilitySearchItem{{
						TripID: "trip-jul-1", BoardStopID: "board-jul-1", AlightStopID: "alight-jul-1",
						OriginDisplayName: "Santa Ines/MA", DestinationDisplayName: "Videira/SC",
						OriginDepartTime: "08:00", TripDate: "2026-07-10", SeatsAvailable: 5,
						Price: 950, Currency: "BRL", Status: "ACTIVE", TripStatus: "SCHEDULED", PackageName: packageToSantaCatarina,
					}},
				}),
			},
		},
		ReceivedAt: at,
	}); err != nil {
		t.Fatalf("seed availability outbound: %v", err)
	}
}

func seedInboundSent(t *testing.T, store *fakeStore, sessionID string, body string, at time.Time) {
	t.Helper()
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        sessionID,
		Direction:        "INBOUND",
		Kind:             "TEXT",
		Body:             body,
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       at,
	}); err != nil {
		t.Fatalf("seed inbound %q: %v", body, err)
	}
}

func seedOutboundSent(t *testing.T, store *fakeStore, sessionID string, body string, at time.Time) {
	t.Helper()
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        sessionID,
		Direction:        "OUTBOUND",
		Kind:             "TEXT",
		Body:             body,
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       at,
	}); err != nil {
		t.Fatalf("seed outbound %q: %v", body, err)
	}
}

func assertNoForbiddenSchedulingVocabulary(t *testing.T, body string) {
	t.Helper()
	if containsOutOfDomainSchedulingVocabulary(body) {
		t.Fatalf("reply contains forbidden scheduling vocabulary: %q", body)
	}
}
