package chat

import (
	"context"
	"strings"
	"testing"

	"schumacher-tur/api/internal/shared/config"
)

func TestInferUnsupportedCargoQuery(t *testing.T) {
	cases := []struct {
		text        string
		unsupported bool
	}{
		{text: "Posso enviar uma moto?", unsupported: true},
		{text: "Voces levam motocicleta?", unsupported: true},
		{text: "Da para mandar encomenda?", unsupported: true},
		{text: "Posso levar uma mala?", unsupported: false},
		{text: "Quero passagem para Santa Catarina", unsupported: false},
	}

	for _, tc := range cases {
		t.Run(tc.text, func(t *testing.T) {
			_, got := inferUnsupportedCargoQuery(tc.text)
			if got != tc.unsupported {
				t.Fatalf("expected unsupported=%v for %q, got %v", tc.unsupported, tc.text, got)
			}
		})
	}
}

func TestReprocessGeneratesSupportDraftForUnsupportedCargo(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Vou consultar a disponibilidade.",
			Model:              "gpt-test",
			ProviderResponseID: "resp-should-not-run",
		},
	}
	searcher := &fakeAvailabilitySearcher{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, searcher)

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5511999999999",
		Message: IngestMessagePayload{
			Direction:         "INBOUND",
			ProviderMessageID: "msg-unsupported-cargo",
			IdempotencyKey:    "idem-unsupported-cargo",
			Body:              "Posso enviar uma moto?",
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
		t.Fatalf("expected runner not to be called, got %d", runner.calls)
	}
	if searcher.calls != 0 {
		t.Fatalf("expected availability_search not to be called, got %d", searcher.calls)
	}
	if len(out.ToolCalls) != 0 {
		t.Fatalf("expected no tool calls, got %d", len(out.ToolCalls))
	}
	if out.Draft == nil {
		t.Fatalf("expected deterministic support draft")
	}
	if got := readInt(out.Draft.NormalizedPayload["tool_call_count"]); got != 0 {
		t.Fatalf("expected tool_call_count=0, got %d", got)
	}
	if got := strings.TrimSpace(out.Draft.Body); got != unsupportedCargoReply {
		t.Fatalf("expected unsupported cargo reply %q, got %q", unsupportedCargoReply, got)
	}
}
