package chat

import (
	"context"
	"errors"
	"schumacher-tur/api/internal/shared/config"
	"strings"
	"testing"
	"time"
)

func TestAvailabilityDraftMonthlyFlowUsesStructuredPassengerAuthorityAfterSelection(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{
		enabled: true,
		result:  availabilityDraftSearchResult("Moncao/MA", "Videira/SC", expectedAvailabilityDraftMonthTripDate(7, 10)),
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
	session := seedAvailabilityDraftSession(t, store)

	first := ingestAvailabilityDraftTurn(t, svc, session, "passagens para julho", "jul-1")
	if first.Draft == nil {
		t.Fatal("expected first draft")
	}
	firstBody := strings.TrimSpace(first.Draft.Body)
	if !strings.Contains(foldChatText(firstBody), "para julho") ||
		!strings.Contains(foldChatText(firstBody), "de qual cidade") ||
		!strings.Contains(foldChatText(firstBody), "quantas pessoas") {
		t.Fatalf("expected route and quantity question for July, got %q", firstBody)
	}
	if runner.calls != 0 {
		t.Fatalf("expected deterministic draft to avoid LLM, got %d calls", runner.calls)
	}
	if searcher.calls != 0 {
		t.Fatalf("expected no availability search before route and qty, got %d", searcher.calls)
	}
	if len(first.Messages) != 1 {
		t.Fatalf("expected one processed current-turn message, got %d", len(first.Messages))
	}
	interpretation := asMap(first.Messages[0].NormalizedPayload["interpretation"])
	if len(asMap(interpretation["availability_draft"])) == 0 {
		t.Fatalf("expected availability draft interpretation in normalized payload, got %+v", first.Messages[0].NormalizedPayload)
	}
	if draft, ok := loadAvailabilityDraft(first.Session); !ok || draft.Month != 7 || draft.Qty != 0 {
		t.Fatalf("expected active July draft after first turn, got ok=%t draft=%+v metadata=%+v", ok, draft, first.Session.Metadata)
	}
	pending := assertAvailabilityPendingQuestion(t, first.Session, pendingQuestionAskAvailabilityRouteAndQty, []string{"origin", "destination", "qty"})
	if got := strings.TrimSpace(asString(pending["question"])); got != firstBody {
		t.Fatalf("expected pending question to record bot question %q, got %q", firstBody, got)
	}
	pendingContext := asMap(pending["context"])
	pendingDraft := asMap(pendingContext["availability_draft"])
	if readInt(pendingDraft["month"]) != 7 || readInt(pendingDraft["qty"]) != 0 {
		t.Fatalf("expected pending question context to carry July draft, got %+v", pending)
	}

	markSessionMessagesAutomationSent(t, store, session.ID)
	second := ingestAvailabilityDraftTurn(t, svc, session, "monção para videira, 1 pessoa", "jul-2")
	if second.Draft == nil {
		t.Fatal("expected second draft")
	}
	if searcher.calls != 1 {
		t.Fatalf("expected one availability search, got %d", searcher.calls)
	}
	assertAvailabilityDraftSearchInput(t, searcher.lastInput, "Moncao/MA", "Videira/SC", 1, 7)
	if searcher.lastInput.TripDate != nil {
		t.Fatalf("expected range search, got trip_date=%v", searcher.lastInput.TripDate)
	}
	if len(second.ToolCalls) != 1 || second.ToolCalls[0].ToolName != toolNameAvailabilitySearch {
		t.Fatalf("expected availability_search tool call, got %+v", second.ToolCalls)
	}
	request := second.ToolCalls[0].RequestPayload
	if asString(request["date_from"]) == "" || asString(request["date_to"]) == "" {
		t.Fatalf("expected date_from/date_to in tool request, got %+v", request)
	}
	secondBody := strings.TrimSpace(second.Draft.Body)
	if !strings.Contains(secondBody, "Encontrei estas opcoes") {
		t.Fatalf("expected availability list, got %q", secondBody)
	}
	foldedSecond := foldChatText(secondBody)
	if strings.Contains(foldedSecond, "crianca") || strings.Contains(foldedSecond, "antes de criar") || strings.Contains(foldedSecond, "reserva") {
		t.Fatalf("expected no passenger/reservation prompt before listing options, got %q", secondBody)
	}
	if runner.calls != 0 {
		t.Fatalf("expected deterministic availability template to avoid LLM, got %d calls", runner.calls)
	}
	if _, ok := loadAvailabilityDraft(second.Session); ok {
		t.Fatalf("expected availability draft cleared after search, got %+v", second.Session.Metadata["availability_draft"])
	}
	assertNoActiveAvailabilityPendingQuestion(t, second.Session)

	markSessionMessagesAutomationSent(t, store, session.ID)
	third := ingestAvailabilityDraftTurn(t, svc, session, "primeira", "jul-3")
	if third.Draft == nil {
		t.Fatal("expected third draft")
	}
	if searcher.calls != 1 {
		t.Fatalf("expected no new availability search on option selection, got %d", searcher.calls)
	}
	if !strings.Contains(foldChatText(third.Draft.Body), "documento de 1 passageiro") {
		t.Fatalf("expected option selection to continue from the structured passenger state into documents, got %q", third.Draft.Body)
	}
}

func TestAvailabilityDraftMonthlyVariationsAskOnlyMissingSlots(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		want       []string
		notWant    []string
		wantQty    int
		wantSearch bool
	}{
		{
			name:    "datas de julho asks route and qty",
			body:    "datas de julho",
			want:    []string{"para julho", "de qual cidade", "quantas pessoas"},
			wantQty: 0,
		},
		{
			name:    "month with qty asks only route",
			body:    "passagens para julho, 1 pessoa",
			want:    []string{"para julho", "de qual cidade", "para qual cidade"},
			notWant: []string{"quantas pessoas"},
			wantQty: 1,
		},
		{
			name:       "full monthly query searches immediately",
			body:       "passagens de monção para videira em julho para 1 pessoa",
			wantSearch: true,
			wantQty:    1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeStoreWithPassengerAuthority()
			runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
			searcher := &fakeAvailabilitySearcher{
				enabled: true,
				result:  availabilityDraftSearchResult("Moncao/MA", "Videira/SC", expectedAvailabilityDraftMonthTripDate(7, 10)),
			}
			svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
			session := seedAvailabilityDraftSession(t, store)

			out := ingestAvailabilityDraftTurn(t, svc, session, tc.body, "variation-"+tc.name)
			if out.Draft == nil {
				t.Fatal("expected draft")
			}
			if tc.wantSearch {
				if searcher.calls != 1 {
					t.Fatalf("expected availability search, got %d", searcher.calls)
				}
				assertAvailabilityDraftSearchInput(t, searcher.lastInput, "Moncao/MA", "Videira/SC", tc.wantQty, 7)
				if !strings.Contains(out.Draft.Body, "Encontrei estas opcoes") {
					t.Fatalf("expected availability list, got %q", out.Draft.Body)
				}
			} else {
				if searcher.calls != 0 {
					t.Fatalf("expected no availability search, got %d", searcher.calls)
				}
				folded := foldChatText(out.Draft.Body)
				for _, want := range tc.want {
					if !strings.Contains(folded, strings.TrimSpace(foldChatText(want))) {
						t.Fatalf("expected reply %q to contain %q", out.Draft.Body, want)
					}
				}
				for _, notWant := range tc.notWant {
					if strings.Contains(folded, strings.TrimSpace(foldChatText(notWant))) {
						t.Fatalf("expected reply %q not to contain %q", out.Draft.Body, notWant)
					}
				}
				draft, ok := loadAvailabilityDraft(out.Session)
				if !ok {
					t.Fatalf("expected active draft metadata, got %+v", out.Session.Metadata)
				}
				if draft.Qty != tc.wantQty {
					t.Fatalf("expected qty %d in draft, got %+v", tc.wantQty, draft)
				}
				expectedSlots := availabilityDraftMissingSlots(draft)
				expectedType := availabilityDraftPendingQuestionType(draft)
				assertAvailabilityPendingQuestion(t, out.Session, expectedType, expectedSlots)
			}
			if runner.calls != 0 {
				t.Fatalf("expected deterministic handling to avoid LLM, got %d calls", runner.calls)
			}
		})
	}
}

func TestAvailabilityDraftCurrentMonthQuantityWithoutRouteAsksRouteNotChild(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
	session := seedAvailabilityDraftSession(t, store)

	out := ingestAvailabilityDraftTurn(t, svc, session, "quero pra esse mês, 1 pessoa", "current-month-route-missing")
	if out.Draft == nil {
		t.Fatal("expected draft")
	}
	if runner.calls != 0 {
		t.Fatalf("expected deterministic draft to avoid LLM, got %d calls", runner.calls)
	}
	if searcher.calls != 0 {
		t.Fatalf("expected no availability search before route, got %d", searcher.calls)
	}
	folded := foldChatText(out.Draft.Body)
	for _, want := range []string{"de qual cidade", "para qual cidade"} {
		if !strings.Contains(folded, strings.TrimSpace(foldChatText(want))) {
			t.Fatalf("expected route question to contain %q, got %q", want, out.Draft.Body)
		}
	}
	for _, notWant := range []string{"crianca", "documento", "pagamento", "opcao de viagem"} {
		if strings.Contains(folded, strings.TrimSpace(foldChatText(notWant))) {
			t.Fatalf("expected route question not to contain %q, got %q", notWant, out.Draft.Body)
		}
	}
	if got := asString(out.Memory["passenger_count_reply_context"]); got == "true" {
		t.Fatalf("expected not to enter passenger count context, got memory %+v", out.Memory)
	}
	draft, ok := loadAvailabilityDraft(out.Session)
	if !ok {
		t.Fatalf("expected active availability draft, got %+v", out.Session.Metadata)
	}
	if draft.Qty != 1 {
		t.Fatalf("expected qty 1 to stay in availability draft, got %+v", draft)
	}
}

func TestAvailabilityDraftCurrentMonthMatcherWordBoundaries(t *testing.T) {
	now := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	for _, text := range []string{
		"esse mês",
		"nesse mês",
		"este mês",
		"neste mês",
	} {
		t.Run(text, func(t *testing.T) {
			draft, ok := parseAvailabilityDraftFromText(text, now)
			if !ok {
				t.Fatalf("expected current-month draft for %q", text)
			}
			if draft.Month != int(now.Month()) || draft.Year != now.Year() {
				t.Fatalf("expected current-month period for %q, got %+v", text, draft)
			}
		})
	}

	for _, text := range []string{
		"esse mesmo",
		"nesse mesmo",
		"isso mesmo",
		"pode ser esse mesmo",
	} {
		t.Run(text, func(t *testing.T) {
			if draft, ok := parseAvailabilityDraftFromText(text, now); ok {
				t.Fatalf("did not expect current-month draft for %q, got %+v", text, draft)
			}
		})
	}
}

func TestAvailabilityDraftRoutePassengerPromptReplyDoesNotAskChild(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
	session := seedAvailabilityDraftSession(t, store)
	now := time.Now().UTC()
	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        session.ID,
		Direction:        "OUTBOUND",
		Kind:             "TEXT",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       now.Add(-2 * time.Minute),
		Body:             "Qual a cidade de origem, a data da viagem e quantos passageiros vao viajar?",
	}); err != nil {
		t.Fatalf("seed broad route prompt: %v", err)
	}

	out := ingestAvailabilityDraftTurn(t, svc, session, "quero pra esse mês, 1 pessoa", "route-passenger-prompt-reply")
	if out.Draft == nil {
		t.Fatal("expected draft")
	}
	if runner.calls != 0 {
		t.Fatalf("expected deterministic draft to avoid LLM, got %d calls", runner.calls)
	}
	if searcher.calls != 0 {
		t.Fatalf("expected no availability search before route, got %d", searcher.calls)
	}
	folded := foldChatText(out.Draft.Body)
	if strings.Contains(folded, "crianca") || strings.Contains(folded, "opcao de viagem") {
		t.Fatalf("expected route prompt instead of child/trip-selection prompt, got %q", out.Draft.Body)
	}
	if !strings.Contains(folded, "de qual cidade") || !strings.Contains(folded, "para qual cidade") {
		t.Fatalf("expected missing route question, got %q", out.Draft.Body)
	}
	if got := asString(out.Memory["passenger_count_reply_context"]); got == "true" {
		t.Fatalf("expected route reply not passenger context, got memory %+v", out.Memory)
	}
}

func TestAvailabilityDraftMoncaoCurrentMonthAsksSCDestination(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	currentMonth := int(time.Now().UTC().Month())
	searcher := &fakeAvailabilitySearcher{
		enabled: true,
		result:  availabilityDraftSearchResult("Moncao/MA", "Fraiburgo/SC", expectedAvailabilityDraftMonthTripDate(currentMonth, 10)),
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
	session := seedAvailabilityDraftSession(t, store)

	out := ingestAvailabilityDraftTurn(t, svc, session, "quero ir pra monção esse mês", "moncao-current-month")
	if out.Draft == nil {
		t.Fatal("expected draft")
	}
	if runner.calls != 0 {
		t.Fatalf("expected deterministic draft to avoid LLM, got %d calls", runner.calls)
	}
	if searcher.calls != 0 {
		t.Fatalf("expected no availability search before destination, got %d", searcher.calls)
	}
	folded := foldChatText(out.Draft.Body)
	for _, want := range []string{"moncao", "santa catarina", "para qual cidade"} {
		if !strings.Contains(folded, strings.TrimSpace(foldChatText(want))) {
			t.Fatalf("expected SC destination question to contain %q, got %q", want, out.Draft.Body)
		}
	}
	for _, notWant := range []string{"crianca", "documento", "pagamento", "quantas pessoas"} {
		if strings.Contains(folded, strings.TrimSpace(foldChatText(notWant))) {
			t.Fatalf("expected SC destination question not to contain %q, got %q", notWant, out.Draft.Body)
		}
	}
	draft, ok := loadAvailabilityDraft(out.Session)
	if !ok {
		t.Fatalf("expected active availability draft, got %+v", out.Session.Metadata)
	}
	if draft.Origin != "Moncao/MA" || draft.Destination != "" || draft.Qty != 1 {
		t.Fatalf("unexpected draft for Moncao current month: %+v", draft)
	}

	markSessionMessagesAutomationSent(t, store, session.ID)
	second := ingestAvailabilityDraftTurn(t, svc, session, "Fraiburgo, 2 pessoas", "moncao-current-month-destination")
	if second.Draft == nil {
		t.Fatal("expected second draft")
	}
	if searcher.calls != 1 {
		t.Fatalf("expected availability search after SC destination reply, got %d", searcher.calls)
	}
	assertAvailabilityDraftSearchInput(t, searcher.lastInput, "Moncao/MA", "Fraiburgo/SC", 2, currentMonth)
	if strings.Contains(foldChatText(second.Draft.Body), "para qual cidade de santa catarina") {
		t.Fatalf("expected destination reply to advance instead of repeating destination question, got %q", second.Draft.Body)
	}
	if !strings.Contains(second.Draft.Body, "Encontrei estas opcoes") {
		t.Fatalf("expected availability list after destination reply, got %q", second.Draft.Body)
	}
	assertNoActiveAvailabilityPendingQuestion(t, second.Session)
}

func TestAvailabilityDraftRouteThenQuantityFollowUps(t *testing.T) {
	t.Run("route follow-up asks quantity", func(t *testing.T) {
		store := newFakeStoreWithPassengerAuthority()
		runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
		searcher := &fakeAvailabilitySearcher{enabled: true}
		svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
		session := seedAvailabilityDraftSession(t, store)
		seedAvailabilityDraftWithPendingQuestionMetadata(t, store, session, monthlyAvailabilityDraft(7, "", "", 0))

		out := ingestAvailabilityDraftTurn(t, svc, session, "monção para videira", "route-follow")
		if out.Draft == nil {
			t.Fatal("expected draft")
		}
		if searcher.calls != 0 {
			t.Fatalf("expected no search before qty, got %d", searcher.calls)
		}
		folded := foldChatText(out.Draft.Body)
		if !strings.Contains(folded, "moncao") || !strings.Contains(folded, "videira") || !strings.Contains(folded, "quantas pessoas") {
			t.Fatalf("expected route confirmation and qty question, got %q", out.Draft.Body)
		}
		pending := assertAvailabilityPendingQuestion(t, out.Session, pendingQuestionAskAvailabilityQty, []string{"qty"})
		pendingContext := asMap(pending["context"])
		pendingDraft := asMap(pendingContext["availability_draft"])
		if asString(pendingDraft["origin"]) != "Moncao/MA" || asString(pendingDraft["destination"]) != "Videira/SC" {
			t.Fatalf("expected pending question context to be replaced with route, got %+v", pending)
		}
	})

	t.Run("quantity follow-up searches", func(t *testing.T) {
		store := newFakeStoreWithPassengerAuthority()
		runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
		searcher := &fakeAvailabilitySearcher{
			enabled: true,
			result:  availabilityDraftSearchResult("Moncao/MA", "Videira/SC", expectedAvailabilityDraftMonthTripDate(7, 10)),
		}
		svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
		session := seedAvailabilityDraftSession(t, store)
		seedAvailabilityDraftMetadata(t, store, session, monthlyAvailabilityDraft(7, "Moncao/MA", "Videira/SC", 0))

		out := ingestAvailabilityDraftTurn(t, svc, session, "1 pessoa", "qty-follow")
		if out.Draft == nil {
			t.Fatal("expected draft")
		}
		if searcher.calls != 1 {
			t.Fatalf("expected availability search, got %d", searcher.calls)
		}
		assertAvailabilityDraftSearchInput(t, searcher.lastInput, "Moncao/MA", "Videira/SC", 1, 7)
		if strings.Contains(foldChatText(out.Draft.Body), "crianca") {
			t.Fatalf("expected no child question before availability list, got %q", out.Draft.Body)
		}
	})
}

func TestAvailabilityDraftRouteOnlyPendingQuestionCompletesAndSearches(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{
		enabled: true,
		result:  availabilityDraftSearchResult("Moncao/MA", "Videira/SC", expectedAvailabilityDraftMonthTripDate(7, 10)),
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
	session := seedAvailabilityDraftSession(t, store)

	first := ingestAvailabilityDraftTurn(t, svc, session, "passagens para julho, 1 pessoa", "route-only-1")
	if first.Draft == nil {
		t.Fatal("expected first draft")
	}
	assertAvailabilityPendingQuestion(t, first.Session, pendingQuestionAskAvailabilityRoute, []string{"origin", "destination"})
	if searcher.calls != 0 {
		t.Fatalf("expected no availability search before route, got %d", searcher.calls)
	}

	markSessionMessagesAutomationSent(t, store, session.ID)
	second := ingestAvailabilityDraftTurn(t, svc, session, "monção para videira", "route-only-2")
	if second.Draft == nil {
		t.Fatal("expected second draft")
	}
	if searcher.calls != 1 {
		t.Fatalf("expected availability search after route reply, got %d", searcher.calls)
	}
	assertAvailabilityDraftSearchInput(t, searcher.lastInput, "Moncao/MA", "Videira/SC", 1, 7)
	if strings.Contains(foldChatText(second.Draft.Body), "crianca") {
		t.Fatalf("expected no passenger collection prompt before availability list, got %q", second.Draft.Body)
	}
	assertNoActiveAvailabilityPendingQuestion(t, second.Session)
}

func TestAvailabilityDraftQtyOnlyPendingQuestionCompletesAndSearches(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{
		enabled: true,
		result:  availabilityDraftSearchResult("Moncao/MA", "Videira/SC", expectedAvailabilityDraftMonthTripDate(7, 10)),
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
	session := seedAvailabilityDraftSession(t, store)

	first := ingestAvailabilityDraftTurn(t, svc, session, "passagens de monção para videira em julho", "qty-only-1")
	if first.Draft == nil {
		t.Fatal("expected first draft")
	}
	assertAvailabilityPendingQuestion(t, first.Session, pendingQuestionAskAvailabilityQty, []string{"qty"})
	if searcher.calls != 0 {
		t.Fatalf("expected no availability search before qty, got %d", searcher.calls)
	}

	markSessionMessagesAutomationSent(t, store, session.ID)
	second := ingestAvailabilityDraftTurn(t, svc, session, "1 pessoa", "qty-only-2")
	if second.Draft == nil {
		t.Fatal("expected second draft")
	}
	if searcher.calls != 1 {
		t.Fatalf("expected availability search after qty reply, got %d", searcher.calls)
	}
	assertAvailabilityDraftSearchInput(t, searcher.lastInput, "Moncao/MA", "Videira/SC", 1, 7)
	if strings.Contains(foldChatText(second.Draft.Body), "crianca") {
		t.Fatalf("expected no passenger collection prompt before availability list, got %q", second.Draft.Body)
	}
	assertNoActiveAvailabilityPendingQuestion(t, second.Session)
}

func TestAvailabilityDraftDoesNotCreateEmptyPendingQuestionForReadyDraft(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{enabled: false}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
	session := seedAvailabilityDraftSession(t, store)

	seedAvailabilityDraftWithPendingQuestionMetadata(t, store, session, monthlyAvailabilityDraft(7, "Moncao/MA", "Videira/SC", 0))

	out := ingestAvailabilityDraftTurn(t, svc, session, "1 pessoa", "ready-draft-search-disabled")
	if out.Draft == nil {
		t.Fatal("expected draft")
	}
	if searcher.calls != 0 {
		t.Fatalf("expected no availability search when searcher is disabled, got %d", searcher.calls)
	}

	if strings.Contains(foldChatText(out.Draft.Body), "quantas pessoas") {
		t.Fatalf("expected ready draft without searcher not to ask quantity again, got %q", out.Draft.Body)
	}
	if strings.Contains(foldChatText(out.Draft.Body), "de qual cidade") {
		t.Fatalf("expected ready draft without searcher not to ask route again, got %q", out.Draft.Body)
	}

	payload := asMap(out.Draft.Payload)
	requestPayload := asMap(payload["request_payload"])
	if got := strings.TrimSpace(asString(requestPayload["template_name"])); got != availabilityDraftTemplateSearchUnavailable {
		t.Fatalf("expected search unavailable template, got %+v", requestPayload)
	}
	if got := strings.TrimSpace(asString(requestPayload["action"])); got != "search_unavailable" {
		t.Fatalf("expected search unavailable action, got %+v", requestPayload)
	}
	payloadPending := asMap(requestPayload["pending_question"])
	if strings.TrimSpace(asString(payloadPending["status"])) == pendingQuestionStatusActive {
		t.Fatalf("expected no active pending question in draft payload, got %+v", payloadPending)
	}

	assertNoActiveAvailabilityPendingQuestion(t, out.Session)

	sessionPending := asMap(out.Session.Metadata["pending_question"])
	if got := strings.TrimSpace(asString(sessionPending["type"])); got != "" {
		t.Fatalf("expected no pending question type for ready draft, got %+v", sessionPending)
	}
}

func TestAvailabilityDraftUsesPendingQuestionContextWithoutTopLevelDraft(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{
		enabled: true,
		result:  availabilityDraftSearchResult("Moncao/MA", "Videira/SC", expectedAvailabilityDraftMonthTripDate(7, 10)),
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
	session := seedAvailabilityDraftSession(t, store)
	draft := monthlyAvailabilityDraft(7, "", "", 0)
	if _, err := store.UpdateSessionMetadata(context.Background(), UpdateSessionMetadataInput{
		SessionID: session.ID,
		Metadata: map[string]interface{}{
			"pending_question": availabilityDraftPendingQuestion(draft, "Para julho, de qual cidade voce pretende sair e para qual cidade quer ir? Para quantas pessoas?"),
		},
	}); err != nil {
		t.Fatalf("seed pending question: %v", err)
	}

	out := ingestAvailabilityDraftTurn(t, svc, session, "monção para videira, 1 pessoa", "pending-only")
	if out.Draft == nil {
		t.Fatal("expected draft")
	}
	if searcher.calls != 1 {
		t.Fatalf("expected availability search from pending question context, got %d", searcher.calls)
	}
	assertAvailabilityDraftSearchInput(t, searcher.lastInput, "Moncao/MA", "Videira/SC", 1, 7)
	if !strings.Contains(out.Draft.Body, "Encontrei estas opcoes") {
		t.Fatalf("expected availability list, got %q", out.Draft.Body)
	}
	assertNoActiveAvailabilityPendingQuestion(t, out.Session)
	if runner.calls != 0 {
		t.Fatalf("expected deterministic handling to avoid LLM, got %d calls", runner.calls)
	}
}

func TestAvailabilityDraftDoesNotStealPassengerCountWithoutActiveDraft(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
	session := seedPassengerCollectionPhase(t, store)
	state := ReducePassengerClarificationEventsV1(newPassengerClarificationStateV1(), []PassengerClarificationEventV1{{
		Type: PassengerClarificationEventPassengerCountSet, Slot: PassengerClarificationSlotPassenger,
		MessageID: "explicit-passenger-count", Value: 1, ValueKnown: true,
		PassengerProvenance: PassengerCountProvenanceSoloSpeaker,
	}})
	state.BootstrapCompleted = true
	seedPassengerClarificationStateV1ForTest(store, session.ID, state)

	out := ingestAvailabilityDraftTurn(t, svc, session, "1 pessoa", "passenger-count")
	if out.Draft == nil {
		t.Fatal("expected draft")
	}
	if !strings.Contains(foldChatText(out.Draft.Body), "crianca") {
		t.Fatalf("expected passenger flow to keep asking child-under-5 slot, got %q", out.Draft.Body)
	}
	if _, ok := loadAvailabilityDraft(out.Session); ok {
		t.Fatalf("did not expect availability draft without active monthly context, got %+v", out.Session.Metadata)
	}
	if searcher.calls != 0 {
		t.Fatalf("expected no availability search for passenger count flow, got %d", searcher.calls)
	}
}

func TestOutOfTurnInfoDuringPassengerCountDoesNotCallTools(t *testing.T) {
	cases := []struct {
		name         string
		text         string
		template     ResponseTemplateName
		wantInfoFold string
	}{
		{
			name:         "payment_two_sided",
			text:         "ai o pagamento faz logo ou só no dia mesmo?",
			template:     TemplatePaymentOptionsInfo,
			wantInfoFold: "pagamento pode ser realizado",
		},
		{
			name:         "payment_paga_agora",
			text:         "paga agora?",
			template:     TemplatePaymentOptionsInfo,
			wantInfoFold: "pagamento pode ser realizado",
		},
		{
			name:         "payment_paga_no_dia",
			text:         "paga no dia?",
			template:     TemplatePaymentOptionsInfo,
			wantInfoFold: "pagamento pode ser realizado",
		},
		{
			name:         "payment_precisa_pagar_agora",
			text:         "precisa pagar agora?",
			template:     TemplatePaymentOptionsInfo,
			wantInfoFold: "pagamento pode ser realizado",
		},
		{
			name:         "payment_tem_que_pagar_agora",
			text:         "tem que pagar agora?",
			template:     TemplatePaymentOptionsInfo,
			wantInfoFold: "pagamento pode ser realizado",
		},
		{
			name:         "payment_pode_pagar_no_embarque",
			text:         "pode pagar no embarque?",
			template:     TemplatePaymentOptionsInfo,
			wantInfoFold: "pagamento pode ser realizado",
		},
		{
			name:         "documents",
			text:         "quais documentos precisa?",
			template:     TemplateDocumentRequirementsInfo,
			wantInfoFold: "preciso do nome completo",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeStoreWithPassengerAuthority()
			runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
			searcher := &fakeAvailabilitySearcher{enabled: true}
			bookingCreator := &fakeBookingCreator{enabled: true}
			paymentCreator := &fakePaymentCreator{enabled: true}
			openAI := &fakeOpenAIInterpreter{enabled: true}
			svc := NewService(store, config.Config{
				ChatDebounceWindowMS:               1500,
				ChatOpenAIInterpreterShadowEnabled: true,
				ChatOpenAIInterpreterAssistEnabled: true,
			}, runner, searcher, bookingCreator, paymentCreator, openAI)
			session := seedPassengerCollectionPhase(t, store)

			if _, err := svc.Ingest(context.Background(), IngestMessageInput{
				ContactKey: session.ContactKey,
				Message: IngestMessagePayload{
					Direction:         "INBOUND",
					ProviderMessageID: "msg-out-of-turn-" + tc.name,
					IdempotencyKey:    "idem-out-of-turn-" + tc.name,
					Body:              tc.text,
				},
			}); err != nil {
				t.Fatalf("ingest out-of-turn info: %v", err)
			}

			out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
			if err != nil {
				t.Fatalf("reprocess out-of-turn info: %v", err)
			}
			if runner.calls != 0 {
				t.Fatalf("expected deterministic template without LLM/document_extract, got runner calls=%d", runner.calls)
			}
			if openAI.calls != 0 {
				t.Fatalf("unsafe passenger state must gate OpenAI shadow, got calls=%d", openAI.calls)
			}
			if searcher.calls != 0 {
				t.Fatalf("expected no availability search, got %d", searcher.calls)
			}
			if bookingCreator.calls != 0 {
				t.Fatalf("expected no booking_create, got %d", bookingCreator.calls)
			}
			if paymentCreator.calls != 0 {
				t.Fatalf("expected no payment_create, got %d", paymentCreator.calls)
			}
			if len(out.ToolCalls) != 0 {
				t.Fatalf("expected no tool calls, got %+v", out.ToolCalls)
			}
			if out.Draft == nil {
				t.Fatal("expected out-of-turn info draft")
			}
			if got := readDraftAutoSendStatus(*out.Draft); got != draftAutoSendStatusEligible {
				t.Fatalf("expected out-of-turn info draft to be auto-send eligible, got %s reasons=%v", got, readDraftAutoSendReasons(*out.Draft))
			}
			if reasons := readDraftAutoSendReasons(*out.Draft); len(reasons) != 0 {
				t.Fatalf("expected no auto-send reasons for out-of-turn info draft, got %+v", reasons)
			}
			if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got != string(tc.template) {
				t.Fatalf("safe parallel question must use local info template, got %q payload=%+v", got, out.Draft.NormalizedPayload)
			}
			body := strings.TrimSpace(out.Draft.Body)
			folded := foldChatText(body)
			if !strings.Contains(folded, "passagem e so para voce") || !strings.Contains(folded, tc.wantInfoFold) {
				t.Fatalf("safe local answer must preserve passenger prompt, got %q", body)
			}
		})
	}
}

func TestPassengerCountAnswerWithPaymentQuestionDoesNotUseOutOfTurnShortcut(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{name: "so_eu_paga_agora", text: "só eu, paga agora?"},
		{name: "so_pra_mim_paga_no_dia", text: "só pra mim, paga no dia?"},
		{name: "apenas_eu_pagamento_faz_logo", text: "apenas eu, pagamento faz logo?"},
		{name: "so_pra_mim_pagar_no_embarque", text: "é só pra mim, pode pagar no embarque?"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeStoreWithPassengerAuthority()
			runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
			searcher := &fakeAvailabilitySearcher{enabled: true}
			bookingCreator := &fakeBookingCreator{enabled: true}
			paymentCreator := &fakePaymentCreator{enabled: true}
			openAI := &fakeOpenAIInterpreter{
				enabled: true,
				result: OpenAIStructuredInterpreterRunResult{
					Interpretation: StructuredInterpretation{
						Intent:      StructuredIntentPassengerCountReply,
						TurnMeaning: TurnMeaningAnswerToQuestion,
						Confidence:  0.94,
						Source:      "openai_test",
					},
				},
			}
			svc := NewService(store, config.Config{
				ChatDebounceWindowMS:               1500,
				ChatOpenAIInterpreterShadowEnabled: true,
				ChatOpenAIInterpreterAssistEnabled: true,
			}, runner, searcher, bookingCreator, paymentCreator, openAI)
			session := seedPassengerCollectionPhase(t, store)
			passengerState, ok := passengerClarificationStateV1FromSession(store.sessions[session.ID])
			if !ok || strings.TrimSpace(passengerState.PassengerPromptMessageID) == "" {
				t.Fatalf("expected explicit delivered passenger prompt fixture, got %+v", passengerState)
			}

			if _, err := svc.Ingest(context.Background(), IngestMessageInput{
				ContactKey: session.ContactKey,
				Message: IngestMessagePayload{
					Direction:         "INBOUND",
					ProviderMessageID: "msg-mixed-passenger-payment-" + tc.name,
					IdempotencyKey:    "idem-mixed-passenger-payment-" + tc.name,
					Body:              tc.text,
					NormalizedPayload: map[string]interface{}{
						passengerClarificationEventsV1MessageKey: []PassengerClarificationEventV1{{
							Type: PassengerClarificationEventPassengerCountSet, Slot: PassengerClarificationSlotPassenger,
							PromptMessageID: passengerState.PassengerPromptMessageID,
							Value:           1, ValueKnown: true, PassengerProvenance: PassengerCountProvenanceSoloSpeaker,
						}},
					},
				},
			}); err != nil {
				t.Fatalf("ingest mixed passenger/payment reply: %v", err)
			}

			out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
			if err != nil {
				t.Fatalf("reprocess mixed passenger/payment reply: %v", err)
			}
			if runner.calls != 0 {
				t.Fatalf("expected passenger count handling without LLM/document_extract, got runner calls=%d", runner.calls)
			}
			if openAI.calls != 0 {
				t.Fatalf("unsafe passenger state must gate OpenAI shadow, got calls=%d", openAI.calls)
			}
			if searcher.calls != 0 {
				t.Fatalf("expected no availability search, got %d", searcher.calls)
			}
			if bookingCreator.calls != 0 {
				t.Fatalf("expected no booking_create, got %d", bookingCreator.calls)
			}
			if paymentCreator.calls != 0 {
				t.Fatalf("expected no payment_create, got %d", paymentCreator.calls)
			}
			if len(out.ToolCalls) != 0 {
				t.Fatalf("expected no tool calls, got %+v", out.ToolCalls)
			}
			if got := asInt(out.Memory["passenger_count"]); got != 1 || out.Memory["passenger_count_known"] != true {
				t.Fatalf("legacy solo fast path must still provide its structured fixture event, memory=%+v", out.Memory)
			}
			if out.Draft == nil {
				t.Fatal("expected passenger-count continuation draft")
			}
			if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got != string(TemplateAskChildUnder5) {
				t.Fatalf("expected the unresolved child slot clarification, got template %q payload=%+v", got, out.Draft.NormalizedPayload)
			}
			body := strings.TrimSpace(out.Draft.Body)
			folded := foldChatText(body)
			if !strings.Contains(folded, "tem crianca de 5 anos ou menos") {
				t.Fatalf("expected unresolved child clarification, got %q", body)
			}
			if strings.Contains(folded, "pagamento pode ser realizado") ||
				strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])) == string(TemplatePaymentOptionsInfo) {
				t.Fatalf("mixed passenger answer must not become payment info draft: %q payload=%+v", body, out.Draft.NormalizedPayload)
			}
		})
	}
}

func TestServiceOutOfTurnInfoShortcutRequiresFinalRouterDecision(t *testing.T) {
	t.Run("payment choice cancellation wins over payment info", func(t *testing.T) {
		store := newFakeStoreWithPassengerAuthority()
		runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
		openAI := &fakeOpenAIInterpreter{enabled: true}
		bookingSearcher := &fakeBookingLookupSearcher{
			enabled: true,
			result: BookingLookupResult{
				Results: []BookingLookupItem{{
					ID:              "BK-ABC123456",
					Status:          "PENDING",
					ReservationCode: "ABC12345",
					PassengerName:   "Maria Silva",
					PassengerPhone:  "48999999999",
					CreatedAt:       time.Now().UTC(),
				}},
			},
		}
		bookingCanceler := &fakeBookingCanceler{
			enabled: true,
			result: BookingCancelResult{
				Mode:            "cancel",
				BookingID:       "BK-ABC123456",
				ReservationCode: "ABC12345",
				PreviousStatus:  "PENDING",
				BookingStatus:   "CANCELLED",
				Reason:          "customer_requested",
				Actor:           "CUSTOMER",
				MessageForAgent: "Cancelamento aplicado com sucesso. Confirme ao cliente que a reserva foi cancelada.",
			},
		}
		paymentCreator := &fakePaymentCreator{enabled: true}
		svc := NewService(store, config.Config{
			ChatDebounceWindowMS:               1500,
			ChatOpenAIInterpreterShadowEnabled: true,
		}, runner, bookingSearcher, bookingCanceler, paymentCreator, openAI)
		session := seedBookedPhase(t, store)
		seedOutboundSent(t, store, session.ID, askPaymentChoiceReply, time.Now().UTC().Add(-30*time.Second))

		if _, err := svc.Ingest(context.Background(), IngestMessageInput{
			ContactKey: session.ContactKey,
			Message: IngestMessagePayload{
				Direction:         "INBOUND",
				ProviderMessageID: "msg-cancel-payment-info",
				IdempotencyKey:    "idem-cancel-payment-info",
				Body:              "quero cancelar, paga agora?",
			},
		}); err != nil {
			t.Fatalf("ingest cancellation/payment mixed turn: %v", err)
		}

		out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
		if err != nil {
			t.Fatalf("reprocess cancellation/payment mixed turn: %v", err)
		}
		assertShadowNotSkippedAsOutOfTurnInfo(t, out)
		if openAI.calls != 1 {
			t.Fatalf("expected OpenAI shadow to run for cancellation guardrail, got calls=%d", openAI.calls)
		}
		if paymentCreator.calls != 0 {
			t.Fatalf("expected no payment_create for cancellation guardrail, got %d calls", paymentCreator.calls)
		}
		if bookingCanceler.calls != 1 {
			t.Fatalf("expected booking_cancel to win over payment info, got %d calls", bookingCanceler.calls)
		}
		if out.Draft == nil {
			t.Fatal("expected cancellation draft")
		}
		if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got == string(TemplatePaymentOptionsInfo) {
			t.Fatalf("cancellation guardrail must not become payment info draft: payload=%+v body=%q", out.Draft.NormalizedPayload, out.Draft.Body)
		}
	})

	t.Run("payment choice handoff wins over payment info", func(t *testing.T) {
		store := newFakeStoreWithPassengerAuthority()
		runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
		openAI := &fakeOpenAIInterpreter{enabled: true}
		paymentCreator := &fakePaymentCreator{enabled: true}
		svc := NewService(store, config.Config{
			ChatDebounceWindowMS:               1500,
			ChatOpenAIInterpreterShadowEnabled: true,
		}, runner, paymentCreator, openAI)
		session := seedBookedPhase(t, store)
		seedOutboundSent(t, store, session.ID, askPaymentChoiceReply, time.Now().UTC().Add(-30*time.Second))

		if _, err := svc.Ingest(context.Background(), IngestMessageInput{
			ContactKey: session.ContactKey,
			Message: IngestMessagePayload{
				Direction:         "INBOUND",
				ProviderMessageID: "msg-handoff-payment-info",
				IdempotencyKey:    "idem-handoff-payment-info",
				Body:              "quero falar com atendente, paga agora?",
			},
		}); err != nil {
			t.Fatalf("ingest handoff/payment mixed turn: %v", err)
		}

		out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
		if err != nil {
			t.Fatalf("reprocess handoff/payment mixed turn: %v", err)
		}
		assertShadowNotSkippedAsOutOfTurnInfo(t, out)
		if openAI.calls != 1 {
			t.Fatalf("expected OpenAI shadow to run for handoff guardrail, got calls=%d", openAI.calls)
		}
		if paymentCreator.calls != 0 {
			t.Fatalf("expected no payment_create for handoff guardrail, got %d calls", paymentCreator.calls)
		}
		if runner.calls != 0 {
			t.Fatalf("expected handoff template without LLM, got runner calls=%d", runner.calls)
		}
		if out.Draft == nil {
			t.Fatal("expected handoff draft")
		}
		if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got != string(TemplateHumanHandoff) {
			t.Fatalf("expected human handoff template, got %q payload=%+v body=%q", got, out.Draft.NormalizedPayload, out.Draft.Body)
		}
	})

	t.Run("unsupported cargo wins over passenger payment info", func(t *testing.T) {
		store := newFakeStoreWithPassengerAuthority()
		runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
		searcher := &fakeAvailabilitySearcher{enabled: true}
		bookingCreator := &fakeBookingCreator{enabled: true}
		paymentCreator := &fakePaymentCreator{enabled: true}
		openAI := &fakeOpenAIInterpreter{enabled: true}
		svc := NewService(store, config.Config{
			ChatDebounceWindowMS:               1500,
			ChatOpenAIInterpreterShadowEnabled: true,
		}, runner, searcher, bookingCreator, paymentCreator, openAI)
		session := seedPassengerCollectionPhase(t, store)

		if _, err := svc.Ingest(context.Background(), IngestMessageInput{
			ContactKey: session.ContactKey,
			Message: IngestMessagePayload{
				Direction:         "INBOUND",
				ProviderMessageID: "msg-cargo-payment-info",
				IdempotencyKey:    "idem-cargo-payment-info",
				Body:              "paga agora? posso levar uma moto?",
			},
		}); err != nil {
			t.Fatalf("ingest cargo/payment mixed turn: %v", err)
		}

		out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
		if err != nil {
			t.Fatalf("reprocess cargo/payment mixed turn: %v", err)
		}
		if openAI.calls != 0 {
			t.Fatalf("unsafe passenger state must gate OpenAI shadow, got calls=%d", openAI.calls)
		}
		if searcher.calls != 0 {
			t.Fatalf("expected no availability search for unsupported cargo, got %d", searcher.calls)
		}
		if bookingCreator.calls != 0 {
			t.Fatalf("expected no booking_create for unsupported cargo, got %d", bookingCreator.calls)
		}
		if paymentCreator.calls != 0 {
			t.Fatalf("expected no payment_create for unsupported cargo, got %d", paymentCreator.calls)
		}
		if out.Draft == nil {
			t.Fatal("expected unsupported cargo draft")
		}
		if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got != safePhaseFallbackTemplateName {
			t.Fatalf("unsafe passenger state must precede cargo routing, got %q payload=%+v body=%q", got, out.Draft.NormalizedPayload, out.Draft.Body)
		}
	})

	t.Run("pure passenger payment question runs shadow and emits info template", func(t *testing.T) {
		store := newFakeStoreWithPassengerAuthority()
		runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
		searcher := &fakeAvailabilitySearcher{enabled: true}
		bookingCreator := &fakeBookingCreator{enabled: true}
		paymentCreator := &fakePaymentCreator{enabled: true}
		openAI := &fakeOpenAIInterpreter{enabled: true}
		svc := NewService(store, config.Config{
			ChatDebounceWindowMS:               1500,
			ChatOpenAIInterpreterShadowEnabled: true,
		}, runner, searcher, bookingCreator, paymentCreator, openAI)
		session := seedPassengerCollectionPhase(t, store)

		if _, err := svc.Ingest(context.Background(), IngestMessageInput{
			ContactKey: session.ContactKey,
			Message: IngestMessagePayload{
				Direction:         "INBOUND",
				ProviderMessageID: "msg-pure-payment-info",
				IdempotencyKey:    "idem-pure-payment-info",
				Body:              "paga agora?",
			},
		}); err != nil {
			t.Fatalf("ingest pure payment info turn: %v", err)
		}

		out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
		if err != nil {
			t.Fatalf("reprocess pure payment info turn: %v", err)
		}
		if openAI.calls != 0 {
			t.Fatalf("unsafe passenger state must gate OpenAI shadow, got calls=%d", openAI.calls)
		}
		if runner.calls != 0 {
			t.Fatalf("expected payment info template without LLM, got runner calls=%d", runner.calls)
		}
		if searcher.calls != 0 {
			t.Fatalf("expected no availability search for payment info, got %d", searcher.calls)
		}
		if bookingCreator.calls != 0 {
			t.Fatalf("expected no booking_create for payment info, got %d", bookingCreator.calls)
		}
		if paymentCreator.calls != 0 {
			t.Fatalf("expected no payment_create for payment info, got %d", paymentCreator.calls)
		}
		if out.Draft == nil {
			t.Fatal("expected payment info draft")
		}
		if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got != string(TemplatePaymentOptionsInfo) {
			t.Fatalf("safe parallel payment question must use local template, got %q payload=%+v body=%q", got, out.Draft.NormalizedPayload, out.Draft.Body)
		}
		folded := foldChatText(out.Draft.Body)
		if !strings.Contains(folded, "pagamento pode ser realizado") || !strings.Contains(folded, "passagem e so para voce") {
			t.Fatalf("safe local payment answer must preserve passenger prompt, got %q", out.Draft.Body)
		}
	})

	for _, tc := range []struct {
		name string
		env  string
	}{
		{name: "intent router flag disables shortcut", env: "CHAT_INTENT_ROUTER_ENABLED"},
		{name: "template realizer flag disables shortcut", env: "CHAT_TEMPLATE_REALIZER_ENABLED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.env, "false")
			store := newFakeStoreWithPassengerAuthority()
			runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
			searcher := &fakeAvailabilitySearcher{enabled: true}
			bookingCreator := &fakeBookingCreator{enabled: true}
			paymentCreator := &fakePaymentCreator{enabled: true}
			openAI := &fakeOpenAIInterpreter{enabled: true}
			svc := NewService(store, config.Config{
				ChatDebounceWindowMS:               1500,
				ChatOpenAIInterpreterShadowEnabled: true,
			}, runner, searcher, bookingCreator, paymentCreator, openAI)
			session := seedPassengerCollectionPhase(t, store)

			if _, err := svc.Ingest(context.Background(), IngestMessageInput{
				ContactKey: session.ContactKey,
				Message: IngestMessagePayload{
					Direction:         "INBOUND",
					ProviderMessageID: "msg-payment-info-flag-disabled-" + tc.env,
					IdempotencyKey:    "idem-payment-info-flag-disabled-" + tc.env,
					Body:              "paga agora?",
				},
			}); err != nil {
				t.Fatalf("ingest pure payment info with %s disabled: %v", tc.env, err)
			}

			out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
			if err != nil {
				t.Fatalf("reprocess pure payment info with %s disabled: %v", tc.env, err)
			}
			if openAI.calls != 0 {
				t.Fatalf("unsafe passenger state must gate OpenAI shadow with %s disabled, got calls=%d", tc.env, openAI.calls)
			}
			if searcher.calls != 0 {
				t.Fatalf("expected no availability search with %s disabled, got %d", tc.env, searcher.calls)
			}
			if bookingCreator.calls != 0 {
				t.Fatalf("expected no booking_create with %s disabled, got %d", tc.env, bookingCreator.calls)
			}
			if paymentCreator.calls != 0 {
				t.Fatalf("expected no payment_create with %s disabled, got %d", tc.env, paymentCreator.calls)
			}
			if out.Draft == nil {
				t.Fatal("expected draft with shortcut disabled")
			}
			if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got == string(TemplatePaymentOptionsInfo) {
				t.Fatalf("expected %s to disable out-of-turn payment template, got payload=%+v body=%q", tc.env, out.Draft.NormalizedPayload, out.Draft.Body)
			}
			if strings.Contains(foldChatText(out.Draft.Body), "pagamento pode ser realizado") {
				t.Fatalf("expected %s to disable payment info shortcut, got %q", tc.env, out.Draft.Body)
			}
		})
	}
}

func TestServiceOutOfTurnInfoDocumentMediaDoesNotRecordShadowSkip(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          `{"mode":"EXTRACTED","passengers":[{"name":"Claudecir Schumacher","document_type":"CPF","document":"529.982.247-25","confidence":0.93}]}`,
			Model:              "gpt-vision-test",
			ProviderResponseID: "resp-document-out-of-turn-payment-caption",
		},
	}
	openAI := &fakeOpenAIInterpreter{enabled: true}
	svc := NewService(store, config.Config{
		ChatDebounceWindowMS:               1500,
		ChatOpenAIInterpreterShadowEnabled: true,
	}, runner, openAI)
	session := seedDocumentCollectionBookingHistory(t, store, "5549988709091")

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			Kind:              "IMAGE",
			ProviderMessageID: "msg-doc-image-payment-caption",
			IdempotencyKey:    "idem-doc-image-payment-caption",
			Body:              "paga agora?",
			NormalizedPayload: map[string]interface{}{
				"image_data_url":  "data:image/jpeg;base64,/9j/2Q==",
				"image_mime_type": "image/jpeg",
			},
		},
	})
	if err != nil {
		t.Fatalf("ingest document image with payment caption: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess document image with payment caption: %v", err)
	}
	assertShadowNotSkippedAsOutOfTurnInfo(t, out)
	if openAI.calls != 1 {
		t.Fatalf("expected OpenAI shadow to run for document media turn, got calls=%d", openAI.calls)
	}
	if runner.calls != 1 {
		t.Fatalf("expected document_extract runner to win over out-of-turn info, got calls=%d", runner.calls)
	}
	if len(out.ToolCalls) != 1 || out.ToolCalls[0].ToolName != toolNameDocumentExtract {
		t.Fatalf("expected document_extract tool call, got %+v", out.ToolCalls)
	}
	if out.Draft == nil {
		t.Fatal("expected document extraction draft")
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got == string(TemplatePaymentOptionsInfo) {
		t.Fatalf("document media turn must not become payment info draft: payload=%+v body=%q", out.Draft.NormalizedPayload, out.Draft.Body)
	}
	if strings.Contains(foldChatText(out.Draft.Body), "pagamento pode ser realizado") {
		t.Fatalf("document media turn must not render payment info shortcut, got %q", out.Draft.Body)
	}
	if !strings.Contains(out.Draft.Body, "Claudecir Schumacher | CPF | 529.***.***-25") {
		t.Fatalf("expected extracted document confirmation, got %q", out.Draft.Body)
	}
}

func TestServiceOutOfTurnInfoDocumentMediaFailureDoesNotUseShortcut(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{
		enabled: true,
		err:     errors.New("vision failed"),
	}
	openAI := &fakeOpenAIInterpreter{enabled: true}
	svc := NewService(store, config.Config{
		ChatDebounceWindowMS:               1500,
		ChatOpenAIInterpreterShadowEnabled: true,
	}, runner, openAI)
	session := seedDocumentCollectionBookingHistory(t, store, "5549988709092")

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			Kind:              "IMAGE",
			ProviderMessageID: "msg-doc-image-payment-caption-failed",
			IdempotencyKey:    "idem-doc-image-payment-caption-failed",
			Body:              "paga agora?",
			NormalizedPayload: map[string]interface{}{
				"image_data_url":  "data:image/jpeg;base64,/9j/2Q==",
				"image_mime_type": "image/jpeg",
			},
		},
	})
	if err != nil {
		t.Fatalf("ingest document image with payment caption: %v", err)
	}

	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess document image with failed extraction and payment caption: %v", err)
	}
	assertShadowNotSkippedAsOutOfTurnInfo(t, out)
	if openAI.calls != 1 {
		t.Fatalf("expected OpenAI shadow to run for failed document media turn, got calls=%d", openAI.calls)
	}
	if runner.calls != 1 {
		t.Fatalf("expected one document_extract attempt without out-of-turn shortcut, got calls=%d", runner.calls)
	}
	if len(runner.inputs) == 0 || !strings.Contains(runner.inputs[0].SystemPrompt, "extrai dados de documentos brasileiros") {
		t.Fatalf("expected first runner call to be document_extract, got %+v", runner.inputs)
	}
	if len(out.ToolCalls) != 1 || out.ToolCalls[0].ToolName != toolNameDocumentExtract {
		t.Fatalf("expected failed document_extract tool call, got %+v", out.ToolCalls)
	}
	if got := strings.TrimSpace(out.ToolCalls[0].Status); got != "FAILED" {
		t.Fatalf("expected failed document_extract status, got %q in %+v", got, out.ToolCalls[0])
	}
	if got := strings.TrimSpace(out.ToolCalls[0].ErrorCode); got != "DOCUMENT_EXTRACT_ERROR" {
		t.Fatalf("expected document_extract error code, got %q in %+v", got, out.ToolCalls[0])
	}
	if out.Draft == nil {
		t.Fatal("expected fallback draft after document_extract failure")
	}
	if got := strings.TrimSpace(asString(out.Draft.NormalizedPayload["template_name"])); got == string(TemplatePaymentOptionsInfo) {
		t.Fatalf("failed document media turn must not become payment info draft: payload=%+v body=%q", out.Draft.NormalizedPayload, out.Draft.Body)
	}
	if strings.Contains(foldChatText(out.Draft.Body), "pagamento pode ser realizado") {
		t.Fatalf("failed document media turn must not render payment info shortcut, got %q", out.Draft.Body)
	}
}

func assertShadowNotSkippedAsOutOfTurnInfo(t *testing.T, out ReprocessResult) {
	t.Helper()
	shadow := structuredShadowSummaryFromMemory(t, out)
	if shadow.OpenAIValidation.RejectReason == "deterministic_out_of_turn_info" {
		t.Fatalf("shadow must not be skipped as deterministic_out_of_turn_info, got %+v", shadow)
	}
}

func structuredShadowSummaryFromMemory(t *testing.T, out ReprocessResult) StructuredInterpreterShadowSummary {
	t.Helper()
	shadow, ok := out.Memory[structuredInterpreterShadowKey].(StructuredInterpreterShadowSummary)
	if !ok {
		t.Fatalf("expected structured shadow summary in memory, got %T: %+v", out.Memory[structuredInterpreterShadowKey], out.Memory[structuredInterpreterShadowKey])
	}
	return shadow
}

func TestAvailabilityDraftPendingQuestionDoesNotBlockHumanSupportOrCancellation(t *testing.T) {
	t.Run("human support", func(t *testing.T) {
		store := newFakeStoreWithPassengerAuthority()
		runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
		searcher := &fakeAvailabilitySearcher{enabled: true}
		svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
		session := seedAvailabilityDraftSession(t, store)
		seedAvailabilityDraftWithPendingQuestionMetadata(t, store, session, monthlyAvailabilityDraft(7, "", "", 0))

		out := ingestAvailabilityDraftTurn(t, svc, session, "quero falar com um atendente", "human-support")
		if out.Draft == nil {
			t.Fatal("expected draft")
		}
		if searcher.calls != 0 {
			t.Fatalf("expected no availability search for human support, got %d", searcher.calls)
		}
		if runner.calls != 0 {
			t.Fatalf("expected human support template to avoid LLM, got %d calls", runner.calls)
		}
		if !strings.Contains(foldChatText(out.Draft.Body), "encaminhar") {
			t.Fatalf("expected human support template, got %q", out.Draft.Body)
		}

		assertNoActiveAvailabilityPendingQuestion(t, out.Session)
		if _, ok := loadAvailabilityDraft(out.Session); ok {
			t.Fatalf("expected availability draft cleared after human support interrupt, got %+v", out.Session.Metadata["availability_draft"])
		}
	})

	t.Run("cancellation", func(t *testing.T) {
		store := newFakeStoreWithPassengerAuthority()
		runner := &fakeAgentRunner{
			enabled: true,
			result: RunAgentResult{
				ReplyText: "Sua reserva foi cancelada.",
				Model:     "gpt-test",
			},
		}
		searcher := &fakeAvailabilitySearcher{enabled: true}
		bookingSearcher := &fakeBookingLookupSearcher{
			enabled: true,
			result: BookingLookupResult{
				Results: []BookingLookupItem{{
					ID:              "BK-ABC123456",
					Status:          "PENDING",
					ReservationCode: "ABC12345",
					PassengerName:   "Maria Silva",
					PassengerPhone:  "48999999999",
					CreatedAt:       time.Now().UTC(),
				}},
			},
		}
		bookingCanceler := &fakeBookingCanceler{
			enabled: true,
			result: BookingCancelResult{
				Mode:            "cancel",
				BookingID:       "BK-ABC123456",
				ReservationCode: "ABC12345",
				PreviousStatus:  "PENDING",
				BookingStatus:   "CANCELLED",
				Reason:          "customer_requested",
				Actor:           "CUSTOMER",
				MessageForAgent: "Cancelamento aplicado com sucesso. Confirme ao cliente que a reserva foi cancelada.",
			},
		}
		svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher, bookingSearcher, bookingCanceler)
		session := seedAvailabilityDraftSession(t, store)
		seedAvailabilityDraftWithPendingQuestionMetadata(t, store, session, monthlyAvailabilityDraft(7, "", "", 0))

		out := ingestAvailabilityDraftTurn(t, svc, session, "quero cancelar a reserva ABC12345", "cancel")
		if out.Draft == nil {
			t.Fatal("expected draft")
		}
		if searcher.calls != 0 {
			t.Fatalf("expected no availability search for cancellation, got %d", searcher.calls)
		}
		if bookingCanceler.calls != 1 {
			t.Fatalf("expected booking cancel to run, got %d calls", bookingCanceler.calls)
		}
		if strings.Contains(foldChatText(out.Draft.Body), "de qual cidade") {
			t.Fatalf("expected cancellation not to be blocked by availability question, got %q", out.Draft.Body)
		}

		assertNoActiveAvailabilityPendingQuestion(t, out.Session)
		if _, ok := loadAvailabilityDraft(out.Session); ok {
			t.Fatalf("expected availability draft cleared after cancellation interrupt, got %+v", out.Session.Metadata["availability_draft"])
		}
	})
}

func seedAvailabilityDraftSession(t *testing.T, store *fakeStore) Session {
	t.Helper()
	now := time.Now().UTC()
	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:       "WHATSAPP",
		ContactKey:    "5511999998888",
		CustomerPhone: "5511999998888",
		LastMessageAt: &now,
	})
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}
	return session
}

func ingestAvailabilityDraftTurn(t *testing.T, svc *Service, session Session, body string, suffix string) ReprocessResult {
	t.Helper()
	if _, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: session.ContactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			Kind:              "TEXT",
			ProviderMessageID: "msg-" + suffix,
			IdempotencyKey:    "idem-" + suffix,
			Body:              body,
		},
	}); err != nil {
		t.Fatalf("ingest %q: %v", body, err)
	}
	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: session.ID})
	if err != nil {
		t.Fatalf("reprocess %q: %v", body, err)
	}
	return out
}

func seedAvailabilityDraftMetadata(t *testing.T, store *fakeStore, session Session, draft AvailabilityDraft) {
	t.Helper()
	if _, err := store.UpdateSessionMetadata(context.Background(), UpdateSessionMetadataInput{
		SessionID: session.ID,
		Metadata:  availabilityDraftMetadata(draft),
	}); err != nil {
		t.Fatalf("seed availability draft metadata: %v", err)
	}
}

func seedAvailabilityDraftWithPendingQuestionMetadata(t *testing.T, store *fakeStore, session Session, draft AvailabilityDraft) {
	t.Helper()
	if _, err := store.UpdateSessionMetadata(context.Background(), UpdateSessionMetadataInput{
		SessionID: session.ID,
		Metadata:  availabilityDraftWithPendingQuestionMetadata(draft, buildAvailabilityDraftMissingSlotsReply(draft)),
	}); err != nil {
		t.Fatalf("seed availability draft pending question metadata: %v", err)
	}
}

func monthlyAvailabilityDraft(month int, origin string, destination string, qty int) AvailabilityDraft {
	dateFrom, dateTo := expectedAvailabilityDraftMonthRange(month)
	return AvailabilityDraft{
		Status:       availabilityDraftStatusActive,
		PeriodType:   availabilityDraftPeriodMonth,
		Month:        month,
		Year:         dateFrom.Year(),
		DateFrom:     &dateFrom,
		DateTo:       &dateTo,
		Origin:       origin,
		Destination:  destination,
		Qty:          qty,
		UpdatedAt:    time.Now().UTC(),
		MissingSlots: nil,
	}
}

func availabilityDraftSearchResult(origin string, destination string, tripDate time.Time) AvailabilitySearchResult {
	return AvailabilitySearchResult{
		Results: []AvailabilitySearchItem{{
			TripID:                 "trip-jul-1",
			RouteID:                "MA_SC",
			BoardStopID:            "board-jul-1",
			AlightStopID:           "alight-jul-1",
			OriginStopID:           "MA_MONCAO",
			DestinationStopID:      "SC_VIDEIRA",
			OriginDisplayName:      origin,
			DestinationDisplayName: destination,
			OriginDepartTime:       "08:00",
			TripDate:               tripDate.Format("2006-01-02"),
			SeatsAvailable:         5,
			Price:                  950,
			Currency:               "BRL",
			Status:                 "ACTIVE",
			TripStatus:             "SCHEDULED",
			PackageName:            packageToSantaCatarina,
		}},
	}
}

func expectedAvailabilityDraftMonthTripDate(month int, day int) time.Time {
	dateFrom, _ := expectedAvailabilityDraftMonthRange(month)
	return time.Date(dateFrom.Year(), time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

func expectedAvailabilityDraftMonthRange(month int) (time.Time, time.Time) {
	now := time.Now().UTC()
	year := now.Year()
	if time.Month(month) < now.Month() {
		year++
	}
	dateFrom := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	dateTo := time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC)
	return dateFrom, dateTo
}

func assertAvailabilityDraftSearchInput(t *testing.T, input AvailabilitySearchInput, origin string, destination string, qty int, month int) {
	t.Helper()
	if input.Origin != origin || input.Destination != destination {
		t.Fatalf("unexpected route: %+v", input)
	}
	if input.Qty != qty {
		t.Fatalf("expected qty %d, got %+v", qty, input)
	}
	dateFrom, dateTo := expectedAvailabilityDraftMonthRange(month)
	if input.DateFrom == nil || input.DateFrom.UTC().Format("2006-01-02") != dateFrom.Format("2006-01-02") {
		t.Fatalf("unexpected date_from: got %v want %s", input.DateFrom, dateFrom.Format("2006-01-02"))
	}
	if input.DateTo == nil || input.DateTo.UTC().Format("2006-01-02") != dateTo.Format("2006-01-02") {
		t.Fatalf("unexpected date_to: got %v want %s", input.DateTo, dateTo.Format("2006-01-02"))
	}
}

func assertAvailabilityPendingQuestion(t *testing.T, session Session, expectedType string, expectedSlots []string) map[string]interface{} {
	t.Helper()
	pending := asMap(session.Metadata["pending_question"])
	if len(pending) == 0 {
		t.Fatalf("expected pending_question metadata, got %+v", session.Metadata)
	}
	if got := strings.TrimSpace(asString(pending["status"])); got != pendingQuestionStatusActive {
		t.Fatalf("expected active pending_question, got %+v", pending)
	}
	if got := strings.TrimSpace(asString(pending["type"])); got != expectedType {
		t.Fatalf("expected pending question type %s, got %+v", expectedType, pending)
	}
	if got := strings.TrimSpace(asString(pending["intent"])); got != string(IntentAvailabilitySearch) {
		t.Fatalf("expected availability intent in pending question, got %+v", pending)
	}
	gotSlots := asStringSlice(pending["expected_slots"])
	if strings.Join(gotSlots, ",") != strings.Join(expectedSlots, ",") {
		t.Fatalf("expected pending slots %v, got %v in %+v", expectedSlots, gotSlots, pending)
	}
	if strings.TrimSpace(asString(pending["question"])) == "" {
		t.Fatalf("expected pending question text, got %+v", pending)
	}
	return pending
}

func assertNoActiveAvailabilityPendingQuestion(t *testing.T, session Session) {
	t.Helper()
	pending := asMap(session.Metadata["pending_question"])
	if len(pending) == 0 {
		return
	}
	if got := strings.TrimSpace(asString(pending["status"])); got == pendingQuestionStatusActive {
		t.Fatalf("expected pending_question cleared, got %+v", pending)
	} else if got != pendingQuestionStatusCompleted {
		t.Fatalf("expected completed pending_question status, got %+v", pending)
	}
}

func markSessionMessagesAutomationSent(t *testing.T, store *fakeStore, sessionID string) {
	t.Helper()
	for id, message := range store.messages {
		if message.SessionID != sessionID {
			continue
		}
		message.ProcessingStatus = messageStatusAutomationSent
		store.messages[id] = message
	}
}
