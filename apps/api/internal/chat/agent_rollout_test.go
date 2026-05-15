package chat

import (
	"context"
	"errors"
	"strings"
	"testing"

	"schumacher-tur/api/internal/shared/config"
)

func TestChatAgentModeLegacyPreservesCurrentFallback(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "Resposta legado", Model: "gpt-test"}}
	jsonRunner := &fakeJSONDecisionRunner{enabled: true, decision: validAvailabilityJSONDecision()}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, jsonRunner)

	out := ingestAndReprocessRollout(t, svc, "5511900000001", "Quero viajar")

	if jsonRunner.calls != 0 {
		t.Fatalf("expected legacy mode to skip JSON runner, got %d calls", jsonRunner.calls)
	}
	if runner.calls != 1 {
		t.Fatalf("expected legacy LLM fallback once, got %d", runner.calls)
	}
	if out.Draft == nil || strings.TrimSpace(out.Draft.Body) != "Resposta legado" {
		t.Fatalf("expected legacy draft, got %+v", out.Draft)
	}
	if got := asString(out.Draft.NormalizedPayload["chat_agent_mode"]); got != chatAgentModeLegacy {
		t.Fatalf("expected legacy mode metadata, got %q", got)
	}
	if got := asString(out.Draft.NormalizedPayload["decision_source"]); got != "legacy_llm" {
		t.Fatalf("expected legacy_llm decision source, got %q", got)
	}
}

func TestChatAgentModeHybridUsesJSONAfterDeterministicMiss(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	jsonRunner := &fakeJSONDecisionRunner{enabled: true, decision: validAvailabilityJSONDecision()}
	searcher := &fakeAvailabilitySearcher{
		enabled: true,
		result:  rolloutAvailabilityResult(),
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500, ChatAgentMode: chatAgentModeHybridJSON}, runner, jsonRunner, searcher)

	out := ingestAndReprocessRollout(t, svc, "5511900000002", "Quero viajar")

	if jsonRunner.calls != 1 {
		t.Fatalf("expected JSON runner once after deterministic miss, got %d", jsonRunner.calls)
	}
	if runner.calls != 0 {
		t.Fatalf("expected valid JSON decision to avoid legacy LLM, got %d calls", runner.calls)
	}
	if searcher.calls != 1 {
		t.Fatalf("expected availability search once, got %d", searcher.calls)
	}
	if out.Draft == nil || !strings.Contains(out.Draft.Body, "Encontrei") {
		t.Fatalf("expected availability template draft, got %+v", out.Draft)
	}
	if got := asString(out.Draft.NormalizedPayload["decision_source"]); got != "json_agent" {
		t.Fatalf("expected json_agent decision source, got %q", got)
	}
}

func TestChatAgentModeHybridSkipsJSONWhenDeterministicHandles(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	jsonRunner := &fakeJSONDecisionRunner{enabled: true, decision: validAvailabilityJSONDecision()}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500, ChatAgentMode: chatAgentModeHybridJSON}, runner, jsonRunner, searcher)

	out := ingestAndReprocessRollout(t, svc, "5511900000003", "Passagem para o Maranhão")

	if jsonRunner.calls != 0 {
		t.Fatalf("expected deterministic template to skip JSON runner, got %d calls", jsonRunner.calls)
	}
	if runner.calls != 0 {
		t.Fatalf("expected deterministic template to skip LLM, got %d calls", runner.calls)
	}
	if out.Draft == nil || !strings.Contains(out.Draft.Body, "cidade de Santa Catarina") {
		t.Fatalf("expected deterministic SC origin question, got %+v", out.Draft)
	}
}

func TestChatAgentModeJSONOnlyNeverCallsFreeFormLLMForOperationalDecision(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	jsonRunner := &fakeJSONDecisionRunner{enabled: true, decision: validAvailabilityJSONDecision()}
	searcher := &fakeAvailabilitySearcher{
		enabled: true,
		result:  rolloutAvailabilityResult(),
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500, ChatAgentMode: chatAgentModeJSONOnly}, runner, jsonRunner, searcher)

	out := ingestAndReprocessRollout(t, svc, "5511900000004", "Quero viajar")

	if runner.calls != 0 {
		t.Fatalf("expected json_only to skip free-form LLM, got %d calls", runner.calls)
	}
	if jsonRunner.calls != 1 || searcher.calls != 1 {
		t.Fatalf("expected JSON decision and availability tool once, json=%d search=%d", jsonRunner.calls, searcher.calls)
	}
	if out.Draft == nil {
		t.Fatal("expected draft")
	}
	if got := asString(out.Draft.NormalizedPayload["chat_agent_mode"]); got != chatAgentModeJSONOnly {
		t.Fatalf("expected json_only metadata, got %q", got)
	}
}

func TestChatAgentModeJSONOnlyInvalidDecisionProducesClarification(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	decision := validAvailabilityJSONDecision()
	decision.Confidence = 0.20
	jsonRunner := &fakeJSONDecisionRunner{enabled: true, decision: decision}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500, ChatAgentMode: chatAgentModeJSONOnly}, runner, jsonRunner, searcher)

	out := ingestAndReprocessRollout(t, svc, "5511900000005", "Quero viajar")

	if runner.calls != 0 {
		t.Fatalf("expected json_only invalid decision to skip LLM, got %d calls", runner.calls)
	}
	if searcher.calls != 0 {
		t.Fatalf("expected invalid decision to skip tools, got %d calls", searcher.calls)
	}
	if out.Draft == nil || !strings.Contains(out.Draft.Body, "Nao consegui confirmar") {
		t.Fatalf("expected deterministic clarification, got %+v", out.Draft)
	}
	if got := asString(out.Draft.NormalizedPayload["fallback_reason"]); !strings.Contains(got, "low_confidence") {
		t.Fatalf("expected low_confidence fallback reason, got %q", got)
	}
}

func TestChatAgentModeMetadataPersistsOnDraftAndToolCalls(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "fallback LLM", Model: "gpt-test"}}
	jsonRunner := &fakeJSONDecisionRunner{enabled: true, decision: validAvailabilityJSONDecision()}
	searcher := &fakeAvailabilitySearcher{
		enabled: true,
		result:  rolloutAvailabilityResult(),
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500, ChatAgentMode: chatAgentModeHybridJSON}, runner, jsonRunner, searcher)

	out := ingestAndReprocessRollout(t, svc, "5511900000006", "Quero viajar")

	if out.Draft == nil {
		t.Fatal("expected draft")
	}
	payload := out.Draft.NormalizedPayload
	for _, key := range []string{
		"chat_agent_mode",
		"decision_source",
		"decision_valid",
		"decision_confidence",
		"canonical_phase_before",
		"canonical_phase_after",
		"tool_call_count",
	} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("expected draft metadata key %s in %#v", key, payload)
		}
	}
	if len(out.ToolCalls) == 0 {
		t.Fatal("expected tool call")
	}
	toolPayload := out.ToolCalls[0].RequestPayload
	if got := asString(toolPayload["chat_agent_mode"]); got != chatAgentModeHybridJSON {
		t.Fatalf("expected tool chat_agent_mode metadata, got %q", got)
	}
	if got := asString(toolPayload["decision_source"]); got != "json_agent" {
		t.Fatalf("expected tool decision_source json_agent, got %q", got)
	}
}

func ingestAndReprocessRollout(t *testing.T, svc *Service, contactKey string, body string) ReprocessResult {
	t.Helper()
	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: contactKey,
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: contactKey + "-provider",
			IdempotencyKey:    contactKey + "-idem",
			Body:              body,
		},
	})
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess: %v", err)
	}
	return out
}

func rolloutAvailabilityResult() AvailabilitySearchResult {
	return AvailabilitySearchResult{
		Filter: AvailabilitySearchInput{Origin: "Chapeco/SC", Destination: "Santa Ines/MA", PackageName: "Pacote p/ Maranhão", Qty: 1, Limit: 5},
		Results: []AvailabilitySearchItem{{
			SegmentID:              "segment-1",
			TripID:                 "trip-1",
			RouteID:                "route-1",
			BoardStopID:            "board-1",
			AlightStopID:           "alight-1",
			OriginDisplayName:      "Chapecó/SC",
			DestinationDisplayName: "Santa Inês/MA",
			OriginDepartTime:       "2026-06-01T08:00:00Z",
			TripDate:               "2026-06-01",
			SeatsAvailable:         12,
			Price:                  350,
			Currency:               "BRL",
			Status:                 "ACTIVE",
			TripStatus:             "ACTIVE",
			PackageName:            "Pacote p/ Maranhão",
		}},
	}
}

type fakeJSONDecisionRunner struct {
	enabled   bool
	calls     int
	decision  IntentDecisionJSON
	result    OpenAIJSONRunResult
	err       error
	lastInput RunJSONDecisionInput
}

func (f *fakeJSONDecisionRunner) Enabled() bool {
	return f != nil && f.enabled
}

func (f *fakeJSONDecisionRunner) RunIntentDecision(_ context.Context, input RunJSONDecisionInput) (IntentDecisionJSON, OpenAIJSONRunResult, error) {
	f.calls++
	f.lastInput = input
	if f.err != nil {
		return IntentDecisionJSON{}, f.result, f.err
	}
	if f.result.ProviderResponseID == "" {
		f.result.ProviderResponseID = "resp-json-test"
	}
	return f.decision, f.result, nil
}

var errFakeJSONDecision = errors.New("fake json decision error")
