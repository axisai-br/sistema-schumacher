package chat

import (
	"testing"
	"time"
)

func TestAdministrativeNotesSupportReplyIncludesPhone(t *testing.T) {
	decision := routeDeterministicIntent(nil, "baixa das notas", CanonicalConversationState{}, time.Now())
	if decision.TemplateName != TemplateHumanSupportInfo ||
		decision.Intent != IntentHumanSupportInfoQuestion {
		t.Fatalf("expected administrative notes to route to human support info, got %+v", decision)
	}
	reply, ok := realizeIntentResponseTemplate(decision)
	if !ok {
		t.Fatalf("expected administrative support reply for %+v", decision)
	}
	if !containsAll(reply, "notas ou financeiro", "+55 49 9886-2222") {
		t.Fatalf("expected administrative support reply with phone, got %q", reply)
	}
	for _, forbidden := range []string{
		"vou te encaminhar",
		"me informar os números",
		"enviar comprovantes",
	} {
		if containsAll(reply, forbidden) {
			t.Fatalf("administrative support reply must not contain %q: %q", forbidden, reply)
		}
	}
}

func TestLooksLikeAdministrativeNotesSupportQuestion(t *testing.T) {
	for _, text := range []string{
		"queria verificar com você com relação à baixa das notas",
		"nota fiscal",
		"notas",
		"baixa das notas",
		"faturamento",
		"financeiro",
		"emissão de nota",
		"comprovante fiscal",
	} {
		if !looksLikeAdministrativeNotesSupportQuestion(text) {
			t.Fatalf("expected administrative notes support question for %q", text)
		}
	}

	for _, text := range []string{
		"João Silva CPF 00000000000",
		"já mandei acima",
		"quais documentos precisa?",
		"paga agora?",
	} {
		if looksLikeAdministrativeNotesSupportQuestion(text) {
			t.Fatalf("did not expect administrative notes support question for %q", text)
		}
	}
}

func TestShouldRouteAdministrativeNotesSupportTurnProtectedPhases(t *testing.T) {
	for _, phase := range []ConversationPhase{
		ConversationPhaseBookingPending,
		ConversationPhasePassengerCollection,
	} {
		if !shouldRouteAdministrativeNotesSupportTurn(phase, nil, "baixa das notas", Session{}, false) {
			t.Fatalf("expected administrative notes support route for phase %s", phase)
		}
	}

	if shouldRouteAdministrativeNotesSupportTurn(ConversationPhasePassengerCollection, nil, "nota fiscal", Session{}, true) {
		t.Fatal("media/document turns must not be routed as administrative notes support")
	}
	if shouldRouteAdministrativeNotesSupportTurn(ConversationPhasePassengerCollection, nil, "João Silva CPF 00000000000", Session{}, false) {
		t.Fatal("passenger document text must not be routed as administrative notes support")
	}
}
