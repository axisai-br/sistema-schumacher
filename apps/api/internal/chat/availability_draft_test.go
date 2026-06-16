package chat

import (
	"context"
	"strings"
	"testing"
	"time"

	"schumacher-tur/api/internal/shared/config"
)

func TestAvailabilityDraftMonthlyFlowCompletesAndSearchesBeforePassengerCollection(t *testing.T) {
	store := newFakeStore()
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

	markSessionMessagesAutomationSent(t, store, session.ID)
	third := ingestAvailabilityDraftTurn(t, svc, session, "primeira", "jul-3")
	if third.Draft == nil {
		t.Fatal("expected third draft")
	}
	if searcher.calls != 1 {
		t.Fatalf("expected no new availability search on option selection, got %d", searcher.calls)
	}
	if !strings.Contains(foldChatText(third.Draft.Body), "passagem e so para voce") {
		t.Fatalf("expected option selection to continue into passenger question, got %q", third.Draft.Body)
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
			store := newFakeStore()
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
			}
			if runner.calls != 0 {
				t.Fatalf("expected deterministic handling to avoid LLM, got %d calls", runner.calls)
			}
		})
	}
}

func TestAvailabilityDraftRouteThenQuantityFollowUps(t *testing.T) {
	t.Run("route follow-up asks quantity", func(t *testing.T) {
		store := newFakeStore()
		runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
		searcher := &fakeAvailabilitySearcher{enabled: true}
		svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)
		session := seedAvailabilityDraftSession(t, store)
		seedAvailabilityDraftMetadata(t, store, session, monthlyAvailabilityDraft(7, "", "", 0))

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
	})

	t.Run("quantity follow-up searches", func(t *testing.T) {
		store := newFakeStore()
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

func TestAvailabilityDraftDoesNotStealPassengerCountWithoutActiveDraft(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner)
	session := seedPassengerCollectionPhase(t, store)

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
