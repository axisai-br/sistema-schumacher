package chat

import (
	"context"
	"strings"
	"testing"
	"time"

	"schumacher-tur/api/internal/shared/config"
)

func passengerCountPromptHistory() []Message {
	at := time.Now().UTC()
	return []Message{
		{ID: "bot-1", Direction: "OUTBOUND", Body: "Perfeito. A passagem e so para voce ou vai mais alguem junto?", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: at.Add(-2 * time.Minute)},
		{ID: "cli-1", Direction: "INBOUND", Body: "hmm", ReceivedAt: at.Add(-90 * time.Second)},
		{ID: "bot-2", Direction: "OUTBOUND", Body: "Perfeito. A passagem e so para voce ou vai mais alguem junto?", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: at.Add(-time.Minute)},
	}
}

func TestRouteDeterministicIntentHelpRequestsGoToHumanSupport(t *testing.T) {
	state := CanonicalConversationState{Phase: ConversationPhasePassengerCollection}
	for _, turn := range []string{
		"Preciso de ajuda", "quero ajuda", "me ajuda", "Ajuda por favor", "atendente",
		"quero falar com atendente", "falar com alguem", "falar com uma pessoa",
		"quero falar com humano", "humano", "pessoa de verdade", "suporte", "AJUDA",
	} {
		t.Run(turn, func(t *testing.T) {
			got := routeDeterministicIntent(passengerCountPromptHistory(), turn, state, time.Now().UTC())
			if got.Intent != IntentHumanSupport || got.TemplateName != TemplateHumanHandoff {
				t.Fatalf("expected HUMAN_SUPPORT for %q, got %+v", turn, got)
			}
		})
	}
}

func TestRouteDeterministicIntentHelpRequestNegatives(t *testing.T) {
	state := CanonicalConversationState{Phase: ConversationPhasePassengerCollection}
	got := routeDeterministicIntent(passengerCountPromptHistory(), "só eu", state, time.Now().UTC())
	if got.Intent != IntentPassengerCountReply {
		t.Fatalf("expected PASSENGER_COUNT_REPLY for 'so eu', got %+v", got)
	}
	got = routeDeterministicIntent(passengerCountPromptHistory(), "somos em 3", state, time.Now().UTC())
	if got.Intent == IntentHumanSupport {
		t.Fatalf("'somos em 3' must not be human support, got %+v", got)
	}
	for _, text := range []string{"ajuda de custo", "ajudante", "preciso de ajuda de custo para a viagem toda"} {
		if looksLikeExplicitHelpRequest(normalizeGuardrailPhrase(text)) {
			t.Fatalf("%q must not be an explicit help request", text)
		}
	}
}

func TestRepeatedQuestionHandoffRunOnlyReplacesRepeatedSafeFallback(t *testing.T) {
	history := passengerCountPromptHistory()
	repeat := buildSafeFallbackDraftRun("Perfeito. A passagem e so para voce ou vai mais alguem junto?", CanonicalConversationState{Phase: ConversationPhasePassengerCollection}, "unsafe_or_looping_draft_replaced")
	got, ok := repeatedQuestionHandoffRun(history, repeat, nil)
	if !ok || got.ReplyText != "Vou te encaminhar para um atendente continuar por aqui." {
		t.Fatalf("expected handoff run, got ok=%t %+v", ok, got)
	}
	if asString(got.ResponsePayload["fallback_reason"]) != repeatedQuestionHandoffReason {
		t.Fatalf("missing fallback_reason: %+v", got.ResponsePayload)
	}
	if _, ok := repeatedQuestionHandoffRun(history[:1], repeat, nil); ok {
		t.Fatalf("a single prior send must still allow one re-ask")
	}
	different := buildSafeFallbackDraftRun("Para qual data voce quer viajar?", CanonicalConversationState{}, "x")
	if _, ok := repeatedQuestionHandoffRun(history, different, nil); ok {
		t.Fatalf("different fallback must not become handoff")
	}
	llm := RunAgentResult{ReplyText: "Perfeito. A passagem e so para voce ou vai mais alguem junto?"}
	if _, ok := repeatedQuestionHandoffRun(history, llm, nil); ok {
		t.Fatalf("non-fallback drafts must not be touched")
	}
}

func TestReprocessRepeatedSafeFallbackBecomesHumanHandoff(t *testing.T) {
	const repeated = "Para consultar, preciso da cidade de origem, destino e data da viagem."
	store := newFakeStore()
	sender := &fakeReplySender{
		enabled: true,
		result:  SendReplyResult{ProviderMessageID: "MSG-HANDOFF-1", ProviderStatus: "SENT", SentAt: time.Now().UTC()},
	}
	runner := &fakeAgentRunner{
		enabled: true,
		result: RunAgentResult{
			ReplyText:          "Claro! Qual passeio voce quer agendar?",
			Model:              "gpt-test",
			ProviderResponseID: "resp-repeat-1",
		},
	}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, sender, runner)
	var outbound IngestMessageResult
	for _, n := range []string{"1", "2"} {
		var err error
		outbound, err = svc.Ingest(context.Background(), IngestMessageInput{
			ContactKey: "5549988700001",
			Message: IngestMessagePayload{
				Direction: "OUTBOUND", ProviderMessageID: "msg-rep-out-" + n, IdempotencyKey: "idem-rep-out-" + n, Body: repeated,
			},
		})
		if err != nil {
			t.Fatalf("ingest outbound: %v", err)
		}
	}
	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: "5549988700001",
		Message: IngestMessagePayload{
			Direction: "INBOUND", ProviderMessageID: "msg-rep-in-1", IdempotencyKey: "idem-rep-in-1", Body: "hmm nao sei bem",
		},
	})
	if err != nil {
		t.Fatalf("ingest inbound: %v", err)
	}
	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess: %v", err)
	}
	if out.Draft == nil || !strings.Contains(out.Draft.Body, "encaminhar para um atendente") {
		t.Fatalf("expected handoff draft, got %+v (outbound session %s)", out.Draft, outbound.Session.ID)
	}
	if got := asString(out.Draft.NormalizedPayload["fallback_reason"]); got != repeatedQuestionHandoffReason {
		t.Fatalf("expected fallback_reason %s, got %q", repeatedQuestionHandoffReason, got)
	}
	if sender.calls != 1 {
		t.Fatalf("expected handoff text sent exactly once, got %d", sender.calls)
	}
	if out.Session.HandoffStatus == "BOT" {
		t.Fatalf("expected session out of BOT handoff status, got %q", out.Session.HandoffStatus)
	}
	if _, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID}); err == nil || !strings.Contains(err.Error(), ErrReprocessRequiresBot.Error()) {
		t.Fatalf("bot must stop processing after handoff, got %v", err)
	}
	if sender.calls != 1 {
		t.Fatalf("no further sends expected, got %d", sender.calls)
	}
}
