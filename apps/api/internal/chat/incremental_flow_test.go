package chat

import (
	"context"
	"strings"
	"testing"
	"time"

	"schumacher-tur/api/internal/shared/config"
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
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-broad-sc-1",
			IdempotencyKey:    "idem-broad-sc-1",
			Body:              "Tem passagem para Santa Catarina?",
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
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"},
	}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)

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
	return out, runner, searcher
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
