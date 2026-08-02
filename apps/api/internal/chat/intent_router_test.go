package chat

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestIntentRouterDeterministicCases(t *testing.T) {
	history := availabilitySelectionHistory(t)
	state := deriveCanonicalConversationState(Session{ID: "session-1", HandoffStatus: "BOT"}, history, "")
	cases := []struct {
		text   string
		intent Intent
		index  int
	}{
		{text: "primeira", intent: IntentSelectAvailabilityOption, index: 1},
		{text: "segunda", intent: IntentSelectAvailabilityOption, index: 2},
		{text: "opção 1", intent: IntentSelectAvailabilityOption, index: 1},
		{text: "paguei", intent: IntentPaymentStatusQuery},
		{text: "quero cancelar", intent: IntentBookingCancel},
		{text: "quero levar uma moto", intent: IntentUnsupportedCargo},
	}
	for _, tc := range cases {
		got := routeDeterministicIntent(history, tc.text, state, time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC))
		if got.Intent != tc.intent {
			t.Fatalf("routeDeterministicIntent(%q) = %s, want %s", tc.text, got.Intent, tc.intent)
		}
		if tc.index > 0 && got.SelectedOptionIndex != tc.index {
			t.Fatalf("routeDeterministicIntent(%q) selected index = %d, want %d", tc.text, got.SelectedOptionIndex, tc.index)
		}
	}
}

func TestRouteDeterministicIntentAdministrativeNotesSupport(t *testing.T) {
	state := discoveryState()
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	for _, text := range []string{
		"queria verificar com você com relação à baixa das notas",
		"nota fiscal",
		"faturamento",
		"financeiro",
		"emissão de nota",
		"comprovante fiscal",
	} {
		t.Run(text, func(t *testing.T) {
			got := routeDeterministicIntent(nil, text, state, now)
			if got.Intent != IntentHumanSupportInfoQuestion {
				t.Fatalf("expected human support info intent, got %+v", got)
			}
			if got.Source != administrativeNotesSupportDecisionSource ||
				got.TemplateName != TemplateHumanSupportInfo ||
				got.Action != "template" {
				t.Fatalf("expected administrative support template decision, got %+v", got)
			}
			reply, ok := realizeIntentResponseTemplate(got)
			if !ok {
				t.Fatal("expected administrative support decision to be realizable")
			}
			folded := strings.Join(strings.Fields(foldChatText(reply)), " ")
			if !strings.Contains(folded, "notas ou financeiro") ||
				strings.Contains(folded, "numero da nota") ||
				strings.Contains(folded, "periodo") ||
				strings.Contains(folded, "comprovante") {
				t.Fatalf("unexpected administrative support reply: %q", reply)
			}
		})
	}

	got := routeDeterministicIntent(nil, "João Silva CPF 00000000000", state, now)
	if got.Source == administrativeNotesSupportDecisionSource {
		t.Fatalf("passenger document text must not route as administrative support: %+v", got)
	}
}

func assertContextualFallbackDecision(t *testing.T, got IntentDecision, templateName ResponseTemplateName) {
	t.Helper()
	if got.Intent != IntentUnknown {
		t.Fatalf("expected unknown contextual fallback intent, got %+v", got)
	}
	if got.Action != "template" {
		t.Fatalf("expected template action, got %+v", got)
	}
	if got.TemplateName != templateName {
		t.Fatalf("expected template %s, got %+v", templateName, got)
	}
	if got.SelectedOptionIndex != 0 {
		t.Fatalf("contextual fallback must not set selected option index, got %+v", got)
	}
}

func TestIntentRouterCancelIntentWinsOverContextualFallback(t *testing.T) {
	history := []Message{{
		Direction:        "OUTBOUND",
		Body:             "A passagem e so para voce ou vai mais alguem junto? Tem crianca de 5 anos ou menos?",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       time.Now().UTC().Add(-1 * time.Minute),
	}}
	state := CanonicalConversationState{Phase: ConversationPhasePassengerCollection}

	got := routeDeterministicIntent(history, "quero cancelar", state, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))

	if got.Intent != IntentBookingCancel {
		t.Fatalf("expected cancellation to win over contextual fallback, got %+v", got)
	}
	if got.TemplateName != "" {
		t.Fatalf("did not expect contextual template on cancellation, got %+v", got)
	}
}

func TestIntentRouterPrioritizesContextualAvailabilitySelectionOverUnsupportedFollowUp(t *testing.T) {
	history := availabilityDateSelectionAfterRouteQuestionHistory(t)
	state := CanonicalConversationState{Phase: ConversationPhaseRouteSelection}

	got := routeDeterministicIntent(history, "essa mesmo", state, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))

	if got.Intent != IntentSelectAvailabilityOption {
		t.Fatalf("expected availability option selection, got %+v", got)
	}
	if got.Intent == IntentUnsupportedPackage || got.TemplateName == TemplateUnsupportedPackage {
		t.Fatalf("contextual selection must not become unsupported package: %+v", got)
	}
	if got.SelectedOptionIndex != 1 {
		t.Fatalf("expected selected option index 1, got %+v", got)
	}
	if got.TemplateName != TemplateAskPassengerCount || got.Action != "template" {
		t.Fatalf("expected passenger count template decision, got %+v", got)
	}
}

func TestIntentRouterUsesActivePromptForAvailabilityOptionEssaMesmo(t *testing.T) {
	history := availabilityDateSelectionAfterRouteQuestionHistory(t)
	state := CanonicalConversationState{Phase: ConversationPhaseRouteSelection}

	for _, text := range []string{"essa mesmo", "essa mesma", "essa msm", "esse msm", "esta msm", "isso msm", "isso mesmo", "sim", "certo", "pode ser", "não tem problema, pode ser essa", "não precisa pagar agora, pode ser essa", "não dá pra pagar agora, pode ser essa"} {
		t.Run(text, func(t *testing.T) {
			got := routeDeterministicIntent(history, text, state, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))

			if got.Intent != IntentSelectAvailabilityOption {
				t.Fatalf("expected availability option selection, got %+v", got)
			}
			if got.SelectedOptionIndex != 1 {
				t.Fatalf("expected selected option index 1, got %+v", got)
			}
			if got.TemplateName != TemplateAskPassengerCount {
				t.Fatalf("expected passenger count template, got %+v", got)
			}
		})
	}
}

func TestIntentRouterPaymentTimingNegationWithAffirmativeOptionStillSelects(t *testing.T) {
	history := availabilityDateSelectionAfterRouteQuestionHistory(t)
	state := CanonicalConversationState{Phase: ConversationPhaseRouteSelection}

	for _, text := range []string{
		"não dá pra pagar agora, pode ser essa",
		"não precisa pagar agora, pode ser essa",
		"não tem problema, pode ser essa",
	} {
		t.Run(text, func(t *testing.T) {
			got := routeDeterministicIntent(history, text, state, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))

			if got.Intent != IntentSelectAvailabilityOption {
				t.Fatalf("expected payment timing negation with affirmative option to select availability, got %+v", got)
			}
			if got.SelectedOptionIndex != 1 {
				t.Fatalf("expected selected option index 1, got %+v", got)
			}
			if got.TemplateName != TemplateAskPassengerCount {
				t.Fatalf("expected passenger count template, got %+v", got)
			}
		})
	}
}

func TestIntentRouterDoesNotSelectEssaMsmFromRenderedSingleAvailabilityOptionWithStaleFacts(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	history := markAvailabilityPromptHistoryDeliveredForTest([]Message{
		{
			Direction:        "OUTBOUND",
			Body:             "Encontrei estas opcoes:\n1. Videira/SC para Santa Ines/MA, 2026-07-13, saida 13:00, R$ 950\n\nQual opcao voce prefere?",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-1 * time.Minute),
		},
	})
	state := CanonicalConversationState{
		Phase: ConversationPhaseTripSelection,
		LastToolFacts: map[string]interface{}{
			toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availabilityDateSelectionFiveOptionsTestResult()),
		},
	}

	got := routeDeterministicIntent(history, "essa msm", state, now)

	if got.Intent == IntentSelectAvailabilityOption {
		t.Fatalf("rendered-only option with stale facts must not select option 1: %+v", got)
	}
	if got.SelectedOptionIndex != 0 {
		t.Fatalf("stale rendered selection must not set selected option index: %+v", got)
	}
	assertContextualFallbackDecision(t, got, TemplateContextFallbackAvailabilityOption)
}

func TestIntentRouterSelectsEssaMsmFromRenderedSingleAvailabilityOptionWithCurrentFacts(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	history := markAvailabilityPromptHistoryDeliveredForTest([]Message{
		{
			Direction:        "OUTBOUND",
			Body:             "Encontrei estas opcoes:\n1. Videira/SC para Santa Ines/MA, 2026-07-13, saida 13:00, R$ 950\n\nQual opcao voce prefere?",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-1 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availabilityDateSelectionTestResult()),
				},
			},
		},
	})
	state := deriveCanonicalConversationState(Session{ID: "session-1", HandoffStatus: "BOT"}, history, "")

	got := routeDeterministicIntent(history, "essa msm", state, now)

	if got.Intent != IntentSelectAvailabilityOption {
		t.Fatalf("expected rendered single option selection with current facts, got %+v", got)
	}
	if got.SelectedOptionIndex != 1 {
		t.Fatalf("expected selected option index 1, got %+v", got)
	}
	if got.TemplateName != TemplateAskPassengerCount || got.Action != "template" {
		t.Fatalf("expected passenger count template decision, got %+v", got)
	}
}

func TestIntentRouterSelectsEssaMsmFromBotAutoReplyDeliveryMirrorSourceDraft(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	history := availabilityDeliveryMirrorHistory(now, availabilityDateSelectionTestResult(), true)
	state := deriveCanonicalConversationState(Session{ID: "session-1", HandoffStatus: "BOT"}, history, "")

	got := routeDeterministicIntent(history, "essa msm", state, now)

	if got.Intent != IntentSelectAvailabilityOption {
		t.Fatalf("expected delivery mirror to select source draft availability option, got %+v", got)
	}
	if got.SelectedOptionIndex != 1 {
		t.Fatalf("expected selected option index 1, got %+v", got)
	}
	if got.TemplateName != TemplateAskPassengerCount || got.Action != "template" {
		t.Fatalf("expected passenger count template decision, got %+v", got)
	}
}

func TestIntentRouterDeliveryMirrorDoesNotAuthorizeInvisibleOption(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	history := availabilityDeliveryMirrorHistory(now, availabilityDateSelectionTestResult(), true)
	state := deriveCanonicalConversationState(Session{ID: "session-1", HandoffStatus: "BOT"}, history, "")

	got := routeDeterministicIntent(history, "5", state, now)

	if got.Intent == IntentSelectAvailabilityOption {
		t.Fatalf("delivery mirror must not select invisible option: %+v", got)
	}
	if got.SelectedOptionIndex != 0 {
		t.Fatalf("delivery mirror invisible option must not set selected index: %+v", got)
	}
	assertContextualFallbackDecision(t, got, TemplateContextFallbackAvailabilityOption)
}

func TestIntentRouterBotAutoReplyWithoutDraftSourceDoesNotAuthorizeAvailabilitySelection(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	history := availabilityDeliveryMirrorHistory(now, availabilityDateSelectionTestResult(), false)
	state := CanonicalConversationState{
		Phase: ConversationPhaseTripSelection,
		LastToolFacts: map[string]interface{}{
			toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availabilityDateSelectionTestResult()),
		},
	}

	got := routeDeterministicIntent(history, "essa msm", state, now)

	if got.Intent == IntentSelectAvailabilityOption {
		t.Fatalf("BOT_AUTO_REPLY without source draft must not select availability option: %+v", got)
	}
	if got.SelectedOptionIndex != 0 {
		t.Fatalf("BOT_AUTO_REPLY without source draft must not set selected index: %+v", got)
	}
	if got.Intent != IntentUnknown || got.Action != "" || got.TemplateName != "" {
		t.Fatalf("isolated PENDING BOT_AUTO_REPLY must not open availability routing: %+v", got)
	}
}

func TestIntentRouterSelectsExplicitOptionFromCappedCurrentAvailabilityFacts(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	availability := availabilityDateSelectionEightOptionsTestResult()
	history := markAvailabilityPromptHistoryDeliveredForTest([]Message{
		{
			Direction:        "OUTBOUND",
			Body:             buildAvailabilityListReply(availability),
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-1 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availability),
				},
			},
		},
	})
	state := deriveCanonicalConversationState(Session{ID: "session-1", HandoffStatus: "BOT"}, history, "")

	for _, text := range []string{"1", "5"} {
		t.Run(text, func(t *testing.T) {
			got := routeDeterministicIntent(history, text, state, now)

			if got.Intent != IntentSelectAvailabilityOption {
				t.Fatalf("expected capped current availability selection, got %+v", got)
			}
			if got.SelectedOptionIndex != extractSelectedOptionIndex(text) {
				t.Fatalf("expected selected option %s, got %+v", text, got)
			}
			if got.TemplateName != TemplateAskPassengerCount || got.Action != "template" {
				t.Fatalf("expected passenger count template decision, got %+v", got)
			}
		})
	}
}

func TestIntentRouterUsesReliableAvailabilityPromptWhenInvisibleOutboundFollows(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	for _, invisibleStatus := range []string{messageStatusAutomationDraft, messageStatusAutomationReviewed, messageStatusAutomationPending} {
		t.Run(invisibleStatus, func(t *testing.T) {
			history := availabilitySingleOptionHistoryWithInvisibleFollowUp(now, invisibleStatus, availabilityDateSelectionFiveOptionsTestResult(), true)
			state := deriveCanonicalConversationState(Session{ID: "session-1", HandoffStatus: "BOT"}, history, "")

			got := routeDeterministicIntent(history, "essa msm", state, now)

			if got.Intent != IntentSelectAvailabilityOption {
				t.Fatalf("expected selection from sent availability prompt after invisible %s, got %+v", invisibleStatus, got)
			}
			if got.SelectedOptionIndex != 1 {
				t.Fatalf("expected sent option index 1 after invisible %s, got %+v", invisibleStatus, got)
			}
			if got.TemplateName != TemplateAskPassengerCount || got.Action != "template" {
				t.Fatalf("expected passenger count template after invisible %s, got %+v", invisibleStatus, got)
			}
		})
	}
}

func TestIntentRouterInvisibleAvailabilityDraftDoesNotAuthorizeUnseenOption(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	history := availabilitySingleOptionHistoryWithInvisibleFollowUp(now, messageStatusAutomationDraft, availabilityDateSelectionFiveOptionsTestResult(), true)
	state := deriveCanonicalConversationState(Session{ID: "session-1", HandoffStatus: "BOT"}, history, "")

	got := routeDeterministicIntent(history, "5", state, now)

	if got.Intent == IntentSelectAvailabilityOption {
		t.Fatalf("invisible draft option must not be selectable: %+v", got)
	}
	if got.SelectedOptionIndex != 0 {
		t.Fatalf("invisible draft option must not set selected index: %+v", got)
	}
	assertContextualFallbackDecision(t, got, TemplateContextFallbackAvailabilityOption)
}

func TestIntentRouterInvisibleDraftWithoutFactsDoesNotInvalidateSentAvailabilityPrompt(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	history := availabilitySingleOptionHistoryWithInvisibleFollowUp(now, messageStatusAutomationDraft, availabilityDateSelectionTestResult(), false)
	state := deriveCanonicalConversationState(Session{ID: "session-1", HandoffStatus: "BOT"}, history, "")

	got := routeDeterministicIntent(history, "1", state, now)

	if got.Intent != IntentSelectAvailabilityOption {
		t.Fatalf("expected selection from sent availability prompt despite draft without facts, got %+v", got)
	}
	if got.SelectedOptionIndex != 1 {
		t.Fatalf("expected sent option index 1 despite draft without facts, got %+v", got)
	}
}

func TestIntentRouterUsesActivePromptForAvailabilityOptionPodeSerEssa(t *testing.T) {
	history := availabilityDateSelectionAfterRouteQuestionHistory(t)
	state := CanonicalConversationState{Phase: ConversationPhaseRouteSelection}

	got := routeDeterministicIntent(history, "pode ser essa", state, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))

	if got.Intent != IntentSelectAvailabilityOption {
		t.Fatalf("expected availability option selection, got %+v", got)
	}
	if got.SelectedOptionIndex != 1 {
		t.Fatalf("expected selected option index 1, got %+v", got)
	}
}

func TestIntentRouterUsesActivePromptForAvailabilityNumericSelection(t *testing.T) {
	history := availabilityDateSelectionAfterRouteQuestionHistory(t)
	state := CanonicalConversationState{Phase: ConversationPhaseRouteSelection}

	got := routeDeterministicIntent(history, "1", state, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))

	if got.Intent != IntentSelectAvailabilityOption {
		t.Fatalf("expected availability option selection, got %+v", got)
	}
	if got.SelectedOptionIndex != 1 {
		t.Fatalf("expected selected option index 1, got %+v", got)
	}
}

func TestIntentRouterRejectsOutOfRangeOptionWithActivePrompt(t *testing.T) {
	history := availabilityDateSelectionAfterRouteQuestionHistory(t)
	state := CanonicalConversationState{Phase: ConversationPhaseRouteSelection}

	got := routeDeterministicIntent(history, "5", state, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))

	if got.Intent == IntentSelectAvailabilityOption {
		t.Fatalf("did not expect out-of-range option to be selected: %+v", got)
	}
	if got.SelectedOptionIndex != 0 {
		t.Fatalf("out-of-range option must not set selected index: %+v", got)
	}
	assertContextualFallbackDecision(t, got, TemplateContextFallbackAvailabilityOption)
}

func TestIntentRouterDoesNotSelectEssaMsmFromRenderedMultipleAvailabilityOptions(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	history := markAvailabilityPromptHistoryDeliveredForTest([]Message{
		{
			Direction: "OUTBOUND",
			Body: "Encontrei estas opcoes:\n" +
				"1. Videira/SC para Santa Ines/MA, 2026-07-13, saida 13:00, R$ 950\n" +
				"2. Videira/SC para Santa Ines/MA, 2026-07-13, saida 20:00, R$ 950\n\n" +
				"Qual opcao voce prefere?",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-1 * time.Minute),
		},
	})
	state := CanonicalConversationState{Phase: ConversationPhaseTripSelection}

	got := routeDeterministicIntent(history, "essa msm", state, now)

	if got.Intent == IntentSelectAvailabilityOption {
		t.Fatalf("rendered multiple options must not select option 1: %+v", got)
	}
	if got.SelectedOptionIndex != 0 {
		t.Fatalf("ambiguous rendered selection must not set selected option index: %+v", got)
	}
	assertContextualFallbackDecision(t, got, TemplateContextFallbackAvailabilityOption)
}

func TestIntentRouterDoesNotSelectAmbiguousAvailabilityReplyFromMultipleOptions(t *testing.T) {
	history := availabilityDateSelectionWithFiveOptionsHistory(t)
	state := CanonicalConversationState{Phase: ConversationPhaseRouteSelection}

	for _, text := range []string{"sim", "ok", "certo", "isso", "isso mesmo", "isso msm", "pode ser", "essa mesmo", "essa mesma", "essa msm", "esse msm", "esta msm", "pode ser essa", "cartao"} {
		t.Run(text, func(t *testing.T) {
			got := routeDeterministicIntent(history, text, state, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))

			if got.Intent == IntentSelectAvailabilityOption {
				t.Fatalf("ambiguous reply must not select option 1 from multiple options: %+v", got)
			}
			if got.SelectedOptionIndex != 0 {
				t.Fatalf("ambiguous reply must not set selected option index: %+v", got)
			}
			if got.Intent == IntentBookingCreateConfirmation {
				t.Fatalf("ambiguous availability reply must not become booking confirmation: %+v", got)
			}
			if got.Intent == IntentUnsupportedPackage || got.TemplateName == TemplateUnsupportedPackage {
				t.Fatalf("ambiguous availability reply must not become unsupported package: %+v", got)
			}
			assertContextualFallbackDecision(t, got, TemplateContextFallbackAvailabilityOption)
		})
	}
}

func TestIntentRouterPaymentInfoQuestionsUseClosedTemplate(t *testing.T) {
	for _, text := range []string{
		"o pagamento faz logo ou só no dia mesmo?",
		"paga agora ou no embarque?",
		"posso pagar só o sinal?",
		"como funciona o pagamento?",
		"quais formas de pagamento?",
	} {
		t.Run(text, func(t *testing.T) {
			got := routeDeterministicIntent(nil, text, CanonicalConversationState{Phase: ConversationPhaseDiscovery}, time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC))

			if got.Intent != IntentPaymentInfoQuestion {
				t.Fatalf("expected payment info intent, got %+v", got)
			}
			if got.TemplateName != TemplatePaymentOptionsInfo || got.Action != "template" {
				t.Fatalf("expected payment options info template, got %+v", got)
			}
		})
	}
}

func TestIntentRouterOutOfTurnInfoQuestionsPreservePassengerCountPrompt(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	history := []Message{{
		Direction:        "OUTBOUND",
		Body:             askPassengerCountReply,
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       now.Add(-1 * time.Minute),
	}}
	state := CanonicalConversationState{Phase: ConversationPhasePassengerCollection}

	cases := []struct {
		text     string
		intent   Intent
		template ResponseTemplateName
		wantInfo string
	}{
		{
			text:     "ai o pagamento faz logo ou só no dia mesmo?",
			intent:   IntentPaymentInfoQuestion,
			template: TemplatePaymentOptionsInfo,
			wantInfo: "pagamento pode ser realizado",
		},
		{
			text:     "quais documentos precisa?",
			intent:   IntentDocumentRequirementsInfoQuestion,
			template: TemplateDocumentRequirementsInfo,
			wantInfo: "preciso do nome completo",
		},
		{
			text:     "o que é passageiro pagante?",
			intent:   IntentPayingPassengerInfoQuestion,
			template: TemplatePayingPassengerInfo,
			wantInfo: "passageiro pagante",
		},
		{
			text:     "criança de colo tem que informar?",
			intent:   IntentChildPolicyInfoQuestion,
			template: TemplateChildPolicyInfo,
			wantInfo: "crianca de 5 anos ou menos",
		},
		{
			text:     "quantas malas posso levar?",
			intent:   IntentBaggageInfoQuestion,
			template: TemplateBaggageInfo,
			wantInfo: "bagagens comuns",
		},
		{
			text:     "onde é o embarque?",
			intent:   IntentBoardingInfoQuestion,
			template: TemplateBoardingInfo,
			wantInfo: "local e o horario de embarque",
		},
		{
			text:     "qual telefone do suporte?",
			intent:   IntentHumanSupportInfoQuestion,
			template: TemplateHumanSupportInfo,
			wantInfo: "suporte",
		},
	}

	for _, tc := range cases {
		t.Run(tc.text, func(t *testing.T) {
			got := routeDeterministicIntent(history, tc.text, state, now)
			if got.Intent != tc.intent {
				t.Fatalf("expected intent %s, got %+v", tc.intent, got)
			}
			if got.TemplateName != tc.template || got.Action != "template" {
				t.Fatalf("expected template %s, got %+v", tc.template, got)
			}
			if got.Source != "deterministic_out_of_turn_info" {
				t.Fatalf("expected out-of-turn source, got %+v", got)
			}
			if pending := ResponseTemplateName(asString(got.TemplateData[outOfTurnPendingPromptTemplateDataKey])); pending != TemplateContextFallbackPassengerCount {
				t.Fatalf("expected pending passenger prompt, got %+v", got.TemplateData)
			}

			reply, ok := realizeIntentResponseTemplate(got)
			if !ok {
				t.Fatalf("expected reply to render for %+v", got)
			}
			folded := foldChatText(reply)
			if !strings.Contains(folded, tc.wantInfo) {
				t.Fatalf("expected info text %q in reply %q", tc.wantInfo, reply)
			}
			if !strings.Contains(folded, "para continuar") ||
				!strings.Contains(folded, "passagem e so para voce") {
				t.Fatalf("expected pending passenger prompt reminder, got %q", reply)
			}
		})
	}
}

func TestIntentRouterOneSidedPaymentQuestionsPreservePassengerCountPrompt(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	history := []Message{{
		Direction:        "OUTBOUND",
		Body:             askPassengerCountReply,
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       now.Add(-1 * time.Minute),
	}}
	state := CanonicalConversationState{Phase: ConversationPhasePassengerCollection}

	for _, text := range []string{
		"paga agora?",
		"paga no dia?",
		"precisa pagar agora?",
		"tem que pagar agora?",
		"pode pagar no embarque?",
	} {
		t.Run(text, func(t *testing.T) {
			got := routeDeterministicIntent(history, text, state, now)
			if got.Intent != IntentPaymentInfoQuestion ||
				got.TemplateName != TemplatePaymentOptionsInfo ||
				got.Action != "template" ||
				got.Source != "deterministic_out_of_turn_info" {
				t.Fatalf("expected out-of-turn payment info template, got %+v", got)
			}
			if got.TemplateName == TemplateContextFallbackPassengerCount {
				t.Fatalf("payment question must not become passenger-count fallback: %+v", got)
			}
			if pending := ResponseTemplateName(asString(got.TemplateData[outOfTurnPendingPromptTemplateDataKey])); pending != TemplateContextFallbackPassengerCount {
				t.Fatalf("expected pending passenger prompt, got %+v", got.TemplateData)
			}
			reply, ok := realizeIntentResponseTemplate(got)
			if !ok {
				t.Fatalf("expected reply to render for %+v", got)
			}
			folded := foldChatText(reply)
			if !strings.Contains(folded, "pagamento pode ser realizado") ||
				!strings.Contains(folded, "para continuar") ||
				!strings.Contains(folded, "passagem e so para voce") {
				t.Fatalf("expected payment answer with passenger reminder, got %q", reply)
			}
		})
	}
}

func TestIntentRouterOutOfTurnInfoPreservesAvailabilityOptionPrompt(t *testing.T) {
	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	history := availabilityOptionPromptHistory(now, availabilityOptionPromptFutureResult())
	state := deriveCanonicalConversationState(Session{ID: "session-1", HandoffStatus: "BOT"}, history, "")

	cases := []struct {
		text     string
		intent   Intent
		template ResponseTemplateName
		wantInfo string
	}{
		{
			text:     "ai o pagamento eu faço logo ou só no dia mesmo?",
			intent:   IntentPaymentInfoQuestion,
			template: TemplatePaymentOptionsInfo,
			wantInfo: "pagamento pode ser realizado",
		},
		{
			text:     "quais documentos precisa?",
			intent:   IntentDocumentRequirementsInfoQuestion,
			template: TemplateDocumentRequirementsInfo,
			wantInfo: "preciso do nome completo",
		},
		{
			text:     "criança de colo tem que informar?",
			intent:   IntentChildPolicyInfoQuestion,
			template: TemplateChildPolicyInfo,
			wantInfo: "crianca de 5 anos ou menos",
		},
		{
			text:     "quantas malas posso levar?",
			intent:   IntentBaggageInfoQuestion,
			template: TemplateBaggageInfo,
			wantInfo: "bagagens comuns",
		},
		{
			text:     "onde é o embarque?",
			intent:   IntentBoardingInfoQuestion,
			template: TemplateBoardingInfo,
			wantInfo: "local e o horario de embarque",
		},
		{
			text:     "qual telefone do suporte?",
			intent:   IntentHumanSupportInfoQuestion,
			template: TemplateHumanSupportInfo,
			wantInfo: "suporte",
		},
	}

	for _, tc := range cases {
		t.Run(tc.text, func(t *testing.T) {
			got := routeDeterministicIntent(history, tc.text, state, now)
			if got.Intent != tc.intent ||
				got.TemplateName != tc.template ||
				got.Action != "template" ||
				got.Source != "deterministic_out_of_turn_info" {
				t.Fatalf("expected out-of-turn template %s/%s, got %+v", tc.intent, tc.template, got)
			}
			if got.SelectedOptionIndex != 0 {
				t.Fatalf("out-of-turn info must not select availability option, got %+v", got)
			}
			if pending := ResponseTemplateName(asString(got.TemplateData[outOfTurnPendingPromptTemplateDataKey])); pending != TemplateContextFallbackAvailabilityOption {
				t.Fatalf("expected pending availability option prompt, got %+v", got.TemplateData)
			}

			reply, ok := realizeIntentResponseTemplate(got)
			if !ok {
				t.Fatalf("expected reply to render for %+v", got)
			}
			folded := foldChatText(reply)
			if !strings.Contains(folded, tc.wantInfo) ||
				!strings.Contains(folded, "para continuar") ||
				!strings.Contains(folded, "qual opcao voce prefere") {
				t.Fatalf("expected info reply with availability prompt reminder, got %q", reply)
			}
		})
	}
}

func TestIntentRouterAvailabilityOptionAnswerWinsOverOutOfTurnPaymentQuestion(t *testing.T) {
	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	history := availabilityOptionPromptHistory(now, availabilityOptionPromptFutureResult())
	state := deriveCanonicalConversationState(Session{ID: "session-1", HandoffStatus: "BOT"}, history, "")

	for _, text := range []string{
		"essa msm, paga agora?",
		"13/07, paga agora?",
	} {
		t.Run(text, func(t *testing.T) {
			got := routeDeterministicIntent(history, text, state, now)
			if got.Intent != IntentSelectAvailabilityOption {
				t.Fatalf("expected active prompt answer to select availability option, got %+v", got)
			}
			if got.SelectedOptionIndex != 1 {
				t.Fatalf("expected selected option index 1, got %+v", got)
			}
			if got.TemplateName != TemplateAskPassengerCount || got.Action != "template" {
				t.Fatalf("expected passenger count template, got %+v", got)
			}
			if got.TemplateName == TemplatePaymentOptionsInfo || got.Source == "deterministic_out_of_turn_info" {
				t.Fatalf("active prompt answer must win over out-of-turn payment info, got %+v", got)
			}
		})
	}
}

func TestIntentRouterAvailabilityDateSelectionMatchesVisibleOptions(t *testing.T) {
	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	history := availabilityOptionPromptHistory(now, availabilityOptionPromptHiddenPastPrefixResult())
	state := deriveCanonicalConversationState(Session{ID: "session-1", HandoffStatus: "BOT"}, history, "")

	got := routeDeterministicIntent(history, "13/07, paga agora?", state, now)

	if got.Intent != IntentSelectAvailabilityOption {
		t.Fatalf("expected date answer to select visible availability option, got %+v", got)
	}
	if got.SelectedOptionIndex != 1 {
		t.Fatalf("expected visible selected option index 1, got %+v", got)
	}
	if got.TemplateName != TemplateAskPassengerCount || got.Action != "template" {
		t.Fatalf("expected passenger count template, got %+v", got)
	}
}

func TestAvailabilityDateSelectionAcrossYearBoundaryPreservesFutureYear(t *testing.T) {
	observedAt := time.Date(2026, 12, 27, 12, 0, 0, 0, time.UTC)
	availability := availabilityOptionPromptFutureResultAt(observedAt)
	futureTrip := availability.Results[0]
	dateInput := availabilityTestDateInput(t, futureTrip.TripDate)
	if dateInput != "03/01/2027" {
		t.Fatalf("expected full next-year date input 03/01/2027, got %q", dateInput)
	}

	parsedInput := extractTripDate(dateInput, observedAt)
	if parsedInput == nil || parsedInput.UTC().Format("2006-01-02") != futureTrip.TripDate {
		t.Fatalf("expected input to preserve future trip year %s, got %v", futureTrip.TripDate, parsedInput)
	}
	previousYearDate := parsedInput.AddDate(-1, 0, 0).Format("2006-01-02")
	if parsedInput.Format("2006-01-02") == previousYearDate || parsedInput.Year() == observedAt.Year() {
		t.Fatalf("next-year input must not resolve to previous year %s, got %s", previousYearDate, parsedInput.Format("2006-01-02"))
	}

	history := availabilityOptionPromptHistory(observedAt, availability)
	state := deriveCanonicalConversationState(Session{ID: "session-1", HandoffStatus: "BOT"}, history, "")
	decision := routeDeterministicIntent(history, dateInput+", paga agora?", state, observedAt)
	if decision.Intent != IntentSelectAvailabilityOption ||
		decision.SelectedOptionIndex != 1 ||
		decision.TemplateName != TemplateAskPassengerCount {
		t.Fatalf("expected next-year date to select the future trip, got %+v", decision)
	}

	visible := currentAvailabilitySelectionPromptAvailabilityContextAt(history, observedAt)
	snapshot := selectedAvailabilityResultPayloadFromAvailability(visible, decision.SelectedOptionIndex)
	if got := strings.TrimSpace(asString(snapshot["trip_date"])); got != futureTrip.TripDate || got == previousYearDate {
		t.Fatalf("expected selected snapshot date %s and not %s, got %q snapshot=%+v", futureTrip.TripDate, previousYearDate, got, snapshot)
	}
	if got := strings.TrimSpace(asString(snapshot["trip_id"])); got != futureTrip.TripID {
		t.Fatalf("expected selected future trip %s, got %q snapshot=%+v", futureTrip.TripID, got, snapshot)
	}
}

func TestIntentRouterAvailabilityDateSelectionIgnoresHiddenOrAmbiguousVisibleDates(t *testing.T) {
	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name   string
		result AvailabilitySearchResult
		text   string
	}{
		{
			name:   "hidden sixth date",
			result: availabilityOptionPromptHiddenSixthDateResult(),
			text:   "13/07, paga agora?",
		},
		{
			name:   "duplicate visible date",
			result: availabilityOptionPromptDuplicateVisibleDateResult(),
			text:   "13/07, paga agora?",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			history := availabilityOptionPromptHistory(now, tc.result)
			state := deriveCanonicalConversationState(Session{ID: "session-1", HandoffStatus: "BOT"}, history, "")

			got := routeDeterministicIntent(history, tc.text, state, now)
			if got.Intent == IntentSelectAvailabilityOption ||
				got.SelectedOptionIndex != 0 ||
				got.TemplateName == TemplateAskPassengerCount {
				t.Fatalf("hidden or ambiguous date must not select availability option, got %+v", got)
			}
			if got.Intent != IntentPaymentInfoQuestion ||
				got.TemplateName != TemplatePaymentOptionsInfo ||
				got.Source != "deterministic_out_of_turn_info" {
				t.Fatalf("expected mixed payment question to stay informational, got %+v", got)
			}
		})
	}
}

func TestIntentRouterNegatedAvailabilityOptionRepliesDoNotSelect(t *testing.T) {
	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	history := availabilityOptionPromptHistory(now, availabilityOptionPromptFutureResult())
	state := deriveCanonicalConversationState(Session{ID: "session-1", HandoffStatus: "BOT"}, history, "")

	cases := []struct {
		text                 string
		wantOutOfTurnPayment bool
	}{
		{text: "não pode ser essa"},
		{text: "não pode ser esta"},
		{text: "não pode ser essa, paga agora?", wantOutOfTurnPayment: true},
		{text: "não quero essa, paga agora?", wantOutOfTurnPayment: true},
		{text: "não quero opção 1, paga agora?", wantOutOfTurnPayment: true},
		{text: "1 não serve, paga agora?", wantOutOfTurnPayment: true},
		{text: "13/07 não serve, paga agora?", wantOutOfTurnPayment: true},
		{text: "essa não dá, paga agora?", wantOutOfTurnPayment: true},
		{text: "não é essa msm"},
		{text: "não fico com essa"},
		{text: "não vou com esta"},
		{text: "essa não"},
		{text: "13/07 não"},
		{text: "opção 1 não serve"},
		{text: "1 não serve"},
		{text: "1 não dá"},
		{text: "não serve 1"},
		{text: "13/07 não serve"},
		{text: "essa opção não serve"},
		{text: "esse dia não dá"},
		{text: "não quero 13/07, quero 14/07"},
		{text: "não quero 13/07, paga agora?", wantOutOfTurnPayment: true},
	}

	for _, tc := range cases {
		t.Run(tc.text, func(t *testing.T) {
			got := routeDeterministicIntent(history, tc.text, state, now)
			if got.Intent == IntentSelectAvailabilityOption ||
				got.SelectedOptionIndex != 0 ||
				got.TemplateName == TemplateAskPassengerCount {
				t.Fatalf("negated availability option reply must not select or advance, got %+v", got)
			}
			if tc.wantOutOfTurnPayment {
				if got.Intent != IntentPaymentInfoQuestion ||
					got.TemplateName != TemplatePaymentOptionsInfo ||
					got.Source != "deterministic_out_of_turn_info" {
					t.Fatalf("expected negated mixed payment question to answer info without selecting, got %+v", got)
				}
				if !templateDataBool(got.TemplateData, outOfTurnRejectedAvailabilityDataKey) {
					t.Fatalf("expected rejected availability flag in template data, got %+v", got.TemplateData)
				}
				return
			}
			assertContextualFallbackDecision(t, got, TemplateContextFallbackAvailabilityOption)
		})
	}
}

func TestLooksLikeNegatedAvailabilitySelectionTargetsAndRejections(t *testing.T) {
	rejects := []string{
		"essa não",
		"esta não funciona",
		"esse não rola",
		"isso não serve",
		"opção 1 não serve",
		"opcao 1 nao funciona",
		"não fico com essa",
		"não vou com esta",
		"1 não serve",
		"1 não dá",
		"1 não rola",
		"1 não funciona",
		"1 fica ruim",
		"1 não",
		"não serve 1",
		"não quero 1",
		"primeira não rola",
		"segunda fica ruim",
		"terceira não consigo",
		"13/07 não dá",
		"13-07 não fica bom",
		"dia 13 não pode ser",
		"não é essa data",
		"não quero esse horário",
		"não quero 13/07, quero 14/07",
	}
	for _, text := range rejects {
		t.Run(text, func(t *testing.T) {
			folded := strings.Join(strings.Fields(foldChatText(text)), " ")
			if !looksLikeNegatedAvailabilitySelection(folded) {
				t.Fatalf("expected %q to be treated as negated availability selection", text)
			}
		})
	}

	allows := []string{
		"não tem problema, pode ser essa",
		"não precisa pagar agora, pode ser essa",
		"não dá pra pagar agora, pode ser essa",
		"84960815086 não serve",
		"quero reservar 1 passageiro",
		"quero reservar Joao Vitor Messias | CPF | 84960815086",
	}
	for _, text := range allows {
		t.Run(text, func(t *testing.T) {
			folded := strings.Join(strings.Fields(foldChatText(text)), " ")
			if looksLikeNegatedAvailabilitySelection(folded) {
				t.Fatalf("expected %q not to be treated as negated availability selection", text)
			}
		})
	}
}

func TestParseAvailabilityRejectionEvidenceSpecificTargets(t *testing.T) {
	cases := []struct {
		text        string
		wantIndexes []int
		wantDates   []string
		wantWhole   bool
	}{
		{text: "não quero opção 1", wantIndexes: []int{1}},
		{text: "não quero essa 1", wantIndexes: []int{1}},
		{text: "opção 1 não serve", wantIndexes: []int{1}},
		{text: "1 não serve", wantIndexes: []int{1}},
		{text: "não serve 1", wantIndexes: []int{1}},
		{text: "13/07 não serve", wantDates: []string{"13/07"}},
		{text: "13/05 não serve", wantDates: []string{"13/05"}},
		{text: "não quero essa", wantWhole: true},
		{text: "essa não", wantWhole: true},
	}
	for _, tc := range cases {
		t.Run(tc.text, func(t *testing.T) {
			folded := strings.Join(strings.Fields(foldChatText(tc.text)), " ")
			got := parseAvailabilityRejectionEvidence(folded)
			if !got.Found {
				t.Fatalf("expected rejection evidence for %q", tc.text)
			}
			if !sameIntSlice(got.OptionIndexes, tc.wantIndexes) {
				t.Fatalf("expected indexes %+v, got %+v for %q", tc.wantIndexes, got.OptionIndexes, tc.text)
			}
			if !sameStringSlice(got.TripDates, tc.wantDates) {
				t.Fatalf("expected dates %+v, got %+v for %q", tc.wantDates, got.TripDates, tc.text)
			}
			if got.WholeContext != tc.wantWhole {
				t.Fatalf("expected whole_context=%v, got %+v for %q", tc.wantWhole, got, tc.text)
			}
		})
	}
}

func TestAvailabilitySelectionAfterSpecificRejectedOptionWithoutPayment(t *testing.T) {
	now := availabilityTestObservedAt()
	availability := availabilityOptionPromptFiveOptionsFutureResultAt(now)

	history := append(availabilityOptionPromptHistory(now, availability), Message{
		Direction:  "INBOUND",
		Body:       "opção 1 não serve",
		ReceivedAt: now.Add(-30 * time.Second),
	})
	state := deriveCanonicalConversationState(
		Session{ID: "session-1", HandoffStatus: "BOT"},
		history,
		"",
	)

	selected := routeDeterministicIntent(history, "2", state, now)
	if selected.Intent != IntentSelectAvailabilityOption ||
		selected.SelectedOptionIndex != 2 ||
		selected.TemplateName != TemplateAskPassengerCount {
		t.Fatalf(
			"expected option 2 to remain selectable after rejecting option 1, got %+v",
			selected,
		)
	}

	rejected := routeDeterministicIntent(history, "1", state, now)
	if rejected.Intent == IntentSelectAvailabilityOption ||
		rejected.SelectedOptionIndex != 0 ||
		rejected.TemplateName == TemplateAskPassengerCount {
		t.Fatalf("expected rejected option 1 not to select, got %+v", rejected)
	}
	assertContextualFallbackDecision(
		t,
		rejected,
		TemplateContextFallbackAvailabilityOption,
	)
}

func TestAvailabilitySelectionAfterSpecificRejectedDateOutOfTurnPaymentBlocksMatchingNumericOption(t *testing.T) {
	now := availabilityTestObservedAt()
	history, rejectedDateInput := availabilityRejectedDateOutOfTurnPaymentHistory(t, now)
	state := deriveCanonicalConversationState(Session{ID: "session-1", HandoffStatus: "BOT"}, history, "")

	rejected := routeDeterministicIntent(history, "1", state, now)
	if rejected.Intent == IntentSelectAvailabilityOption ||
		rejected.SelectedOptionIndex != 0 ||
		rejected.TemplateName == TemplateAskPassengerCount {
		t.Fatalf("expected rejected date option 1 not to select, got %+v", rejected)
	}
	assertContextualFallbackDecision(t, rejected, TemplateContextFallbackAvailabilityOption)

	selected := routeDeterministicIntent(history, "2", state, now)
	if selected.Intent != IntentSelectAvailabilityOption ||
		selected.SelectedOptionIndex != 2 ||
		selected.TemplateName != TemplateAskPassengerCount {
		t.Fatalf("expected option 2 to remain selectable after rejecting %s, got %+v", rejectedDateInput, selected)
	}
}

func availabilityRejectedDateOutOfTurnPaymentHistory(t *testing.T, now time.Time) ([]Message, string) {
	t.Helper()

	availability := availabilityOptionPromptFiveOptionsFutureResultAt(now)
	rejectedDateInput := availabilityTestDateInput(t, availability.Results[0].TripDate)
	rejectedDateMetadata := availabilityTestDayMonthMetadata(t, availability.Results[0].TripDate)
	rejectionBody := "não quero " + rejectedDateInput + ", paga agora?"
	history := availabilityOptionPromptHistory(now, availability)
	state := deriveCanonicalConversationState(Session{ID: "session-1", HandoffStatus: "BOT"}, history, "")
	activePrompt := InferActivePromptContext(history, state)
	decision, ok := buildOutOfTurnInfoDecision(rejectionBody, activePrompt)
	if !ok {
		t.Fatal("expected rejected date payment question to build out-of-turn decision")
	}
	reply, ok := realizeIntentResponseTemplate(decision)
	if !ok {
		t.Fatalf("expected rejected date payment question to render, got %+v", decision)
	}
	templateData := cloneMap(decision.TemplateData)
	if got := availabilityRejectedTripDatesFromMetadata(templateData[outOfTurnRejectedTripDatesDataKey]); !sameStringSlice(got, []string{rejectedDateMetadata}) {
		t.Fatalf("expected rejected trip date metadata [%s], got %+v data=%+v", rejectedDateMetadata, got, templateData)
	}

	return append(history,
		Message{
			Direction:  "INBOUND",
			Body:       rejectionBody,
			ReceivedAt: now.Add(-30 * time.Second),
		},
		Message{
			Direction:        "OUTBOUND",
			Body:             reply,
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-20 * time.Second),
			Payload: map[string]interface{}{
				"template_name": string(TemplatePaymentOptionsInfo),
				"intent":        string(IntentPaymentInfoQuestion),
				"template_data": templateData,
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availability),
				},
			},
		},
	), rejectedDateInput
}

func sameIntSlice(left []int, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func sameStringSlice(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func TestIntentRouterOutOfTurnPaymentDuringMultipleAvailabilityOptionsDoesNotSelect(t *testing.T) {
	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	history := availabilityOptionPromptHistory(now, availabilityOptionPromptTwoOptionsFutureResult())
	state := deriveCanonicalConversationState(Session{ID: "session-1", HandoffStatus: "BOT"}, history, "")

	got := routeDeterministicIntent(history, "paga agora?", state, now)

	if got.Intent != IntentPaymentInfoQuestion ||
		got.TemplateName != TemplatePaymentOptionsInfo ||
		got.Source != "deterministic_out_of_turn_info" {
		t.Fatalf("expected out-of-turn payment info, got %+v", got)
	}
	if got.SelectedOptionIndex != 0 {
		t.Fatalf("payment question with multiple options must not select option 1, got %+v", got)
	}
	reply, ok := realizeIntentResponseTemplate(got)
	if !ok {
		t.Fatalf("expected reply to render for %+v", got)
	}
	folded := foldChatText(reply)
	if !strings.Contains(folded, "para continuar") || !strings.Contains(folded, "qual opcao voce prefere") {
		t.Fatalf("expected availability option reminder, got %q", reply)
	}
}

func TestIntentRouterGuardrailsWinOverOutOfTurnInfoDuringAvailabilitySelection(t *testing.T) {
	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	history := availabilityOptionPromptHistory(now, availabilityOptionPromptFutureResult())
	state := deriveCanonicalConversationState(Session{ID: "session-1", HandoffStatus: "BOT"}, history, "")

	cases := []struct {
		text       string
		wantIntent Intent
	}{
		{text: "quero cancelar, paga agora?", wantIntent: IntentBookingCancel},
		{text: "quero falar com atendente, paga agora?", wantIntent: IntentHumanSupport},
		{text: "paga agora? posso levar uma moto?", wantIntent: IntentUnsupportedCargo},
	}
	for _, tc := range cases {
		t.Run(tc.text, func(t *testing.T) {
			got := routeDeterministicIntent(history, tc.text, state, now)
			if got.Intent != tc.wantIntent {
				t.Fatalf("expected guardrail %s to win, got %+v", tc.wantIntent, got)
			}
			if got.TemplateName == TemplatePaymentOptionsInfo || got.Source == "deterministic_out_of_turn_info" {
				t.Fatalf("guardrail must not become out-of-turn payment info, got %+v", got)
			}
		})
	}
}

func TestIntentRouterPassengerCountAnswerWinsOverOutOfTurnPaymentQuestion(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	history := []Message{{
		Direction:        "OUTBOUND",
		Body:             askPassengerCountReply,
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       now.Add(-1 * time.Minute),
	}}
	state := CanonicalConversationState{Phase: ConversationPhasePassengerCollection}

	for _, text := range []string{
		"só eu, paga agora?",
		"só pra mim, paga no dia?",
		"apenas eu, pagamento faz logo?",
		"é só pra mim, pode pagar no embarque?",
	} {
		t.Run(text, func(t *testing.T) {
			got := routeDeterministicIntent(history, text, state, now)
			if got.Intent != IntentPassengerCountReply ||
				got.Source != "deterministic_active_prompt_passenger_count" {
				t.Fatalf("expected passenger count active prompt answer to win, got %+v", got)
			}
			if got.TemplateName == TemplatePaymentOptionsInfo ||
				got.Source == "deterministic_out_of_turn_info" {
				t.Fatalf("mixed passenger answer must not become out-of-turn payment info: %+v", got)
			}
		})
	}
}

func TestIntentRouterOutOfTurnInfoAppliesToReservationActivePrompts(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name            string
		prompt          string
		phase           ConversationPhase
		pendingTemplate ResponseTemplateName
	}{
		{
			name:            "passenger_count",
			prompt:          askPassengerCountReply,
			phase:           ConversationPhasePassengerCollection,
			pendingTemplate: TemplateContextFallbackPassengerCount,
		},
		{
			name:            "child_under_5",
			prompt:          askChildUnder5Reply,
			phase:           ConversationPhasePassengerCollection,
			pendingTemplate: TemplateContextFallbackChildUnder5,
		},
		{
			name:            "passenger_documents",
			prompt:          buildAskDocumentsReply(1, 0),
			phase:           ConversationPhasePassengerCollection,
			pendingTemplate: TemplateContextFallbackPassengerDocuments,
		},
		{
			name:            "document_confirmation",
			prompt:          "Consegui identificar estes dados. Eles conferem? Posso prosseguir e criar a reserva?",
			phase:           ConversationPhaseBookingPending,
			pendingTemplate: TemplateContextFallbackDocumentConfirmation,
		},
		{
			name:            "payment_preference",
			prompt:          askPaymentChoiceReply,
			phase:           ConversationPhaseBooked,
			pendingTemplate: TemplateContextFallbackPaymentPreference,
		},
		{
			name:            "payer_cpf",
			prompt:          "Para gerar o PIX, preciso do CPF do pagador.",
			phase:           ConversationPhaseBooked,
			pendingTemplate: TemplateContextFallbackPayerCPF,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			history := []Message{{
				Direction:        "OUTBOUND",
				Body:             tc.prompt,
				ProcessingStatus: messageStatusAutomationSent,
				ReceivedAt:       now.Add(-1 * time.Minute),
			}}
			got := routeDeterministicIntent(history, "como funciona o pagamento?", CanonicalConversationState{Phase: tc.phase}, now)
			if got.Intent != IntentPaymentInfoQuestion ||
				got.TemplateName != TemplatePaymentOptionsInfo ||
				got.Source != "deterministic_out_of_turn_info" {
				t.Fatalf("expected out-of-turn payment info for %s, got %+v", tc.name, got)
			}
			if pending := ResponseTemplateName(asString(got.TemplateData[outOfTurnPendingPromptTemplateDataKey])); pending != tc.pendingTemplate {
				t.Fatalf("expected pending template %s for %s, got %+v", tc.pendingTemplate, tc.name, got.TemplateData)
			}
		})
	}
}

func TestIntentRouterOutOfTurnInfoReplyKeepsPassengerCountActive(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	history := []Message{{
		Direction:        "OUTBOUND",
		Body:             askPassengerCountReply,
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       now.Add(-2 * time.Minute),
	}}
	state := CanonicalConversationState{Phase: ConversationPhasePassengerCollection}

	infoDecision := routeDeterministicIntent(history, "quais documentos precisa?", state, now)
	infoReply, ok := realizeIntentResponseTemplate(infoDecision)
	if !ok || strings.TrimSpace(infoReply) == "" {
		t.Fatalf("expected out-of-turn info reply, got ok=%t decision=%+v", ok, infoDecision)
	}
	history = append(history, Message{
		Direction:        "OUTBOUND",
		Body:             infoReply,
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       now.Add(-1 * time.Minute),
	})

	got := routeDeterministicIntent(history, "só eu", state, now)
	if got.Intent != IntentPassengerCountReply {
		t.Fatalf("expected passenger count reply after out-of-turn info, got %+v", got)
	}
}

func TestIntentRouterOutOfTurnPaymentPreferenceKeepsAnswerPriority(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	history := []Message{{
		Direction:        "OUTBOUND",
		Body:             askPaymentChoiceReply,
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       now.Add(-1 * time.Minute),
	}}
	state := CanonicalConversationState{Phase: ConversationPhaseBooked}

	answer := routeDeterministicIntent(history, "vou pagar o sinal", state, now)
	if answer.Intent != IntentPaymentPreference {
		t.Fatalf("expected payment preference answer to win, got %+v", answer)
	}
	if answer.TemplateName == TemplatePaymentOptionsInfo {
		t.Fatalf("payment preference answer must not become info template: %+v", answer)
	}

	question := routeDeterministicIntent(history, "pode pagar só o sinal?", state, now)
	if question.Intent != IntentPaymentInfoQuestion ||
		question.TemplateName != TemplatePaymentOptionsInfo ||
		question.Source != "deterministic_out_of_turn_info" {
		t.Fatalf("expected out-of-turn payment info question, got %+v", question)
	}
	if pending := ResponseTemplateName(asString(question.TemplateData[outOfTurnPendingPromptTemplateDataKey])); pending != TemplateContextFallbackPaymentPreference {
		t.Fatalf("expected pending payment preference prompt, got %+v", question.TemplateData)
	}
}

func TestIntentRouterPaymentInfoReplyDoesNotBecomePaymentPreferencePrompt(t *testing.T) {
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	history := []Message{{
		ID:               "payment-info",
		Direction:        "OUTBOUND",
		Body:             paymentOptionsInfoReply,
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       now.Add(-1 * time.Minute),
		Payload: map[string]interface{}{
			"template_name": string(TemplatePaymentOptionsInfo),
			"intent":        string(IntentPaymentInfoQuestion),
		},
	}}
	state := CanonicalConversationState{Phase: ConversationPhaseDiscovery}

	activePrompt := InferActivePromptContext(history, state)
	if activePrompt.Kind != ActivePromptUnknown {
		t.Fatalf("payment info reply must not become active prompt, got %+v", activePrompt)
	}

	for _, text := range []string{
		"quero reservar",
		"quero ir de Videira para Santa Inês",
		"13/07",
		"como faço pra reservar",
	} {
		t.Run(text, func(t *testing.T) {
			got := routeDeterministicIntent(history, text, state, now)

			if got.TemplateName == TemplateContextFallbackPaymentPreference {
				t.Fatalf("payment info reply must not force payment preference fallback for %q: %+v", text, got)
			}
			if text == "quero reservar" || text == "como faço pra reservar" {
				if got.TemplateName != TemplateAskReservationRouteSC {
					t.Fatalf("expected reservation start to remain available for %q, got %+v", text, got)
				}
			}
		})
	}
}

func TestIntentRouterPaymentStatusQueriesStayPaymentStatus(t *testing.T) {
	for _, text := range []string{
		"paguei",
		"já paguei",
		"já paguei o sinal",
		"já paguei integral",
		"meu pagamento caiu?",
		"o pagamento do sinal caiu?",
		"pagamento aprovado?",
		"confirma se o pagamento entrou",
		"confirma se meu pagamento entrou?",
		"qual o status do pagamento?",
		"já caiu?",
	} {
		t.Run(text, func(t *testing.T) {
			got := routeDeterministicIntent(nil, text, CanonicalConversationState{Phase: ConversationPhaseBooked}, time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC))

			if got.Intent != IntentPaymentStatusQuery {
				t.Fatalf("expected payment status query, got %+v", got)
			}
			if got.TemplateName == TemplatePaymentOptionsInfo {
				t.Fatalf("payment status query must not become payment info template: %+v", got)
			}
		})
	}
}

func TestIntentRouterPaymentStatusWinsOverPaymentPreferencePrompt(t *testing.T) {
	history := []Message{{
		Direction:        "OUTBOUND",
		Body:             "Prefere pagar o valor integral ou apenas o sinal?",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       time.Now().UTC().Add(-1 * time.Minute),
	}}
	state := CanonicalConversationState{Phase: ConversationPhaseBooked}

	for _, text := range []string{
		"já paguei o sinal",
		"o pagamento do sinal caiu?",
		"já paguei integral",
		"confirma se meu pagamento entrou?",
	} {
		t.Run(text, func(t *testing.T) {
			got := routeDeterministicIntent(history, text, state, time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC))

			if got.Intent != IntentPaymentStatusQuery {
				t.Fatalf("expected payment status to win over active prompt preference, got %+v", got)
			}
			if got.Intent == IntentPaymentPreference {
				t.Fatalf("status phrase must not become payment preference: %+v", got)
			}
		})
	}
}

func TestIntentRouterPaymentStatusWinsOverPassengerCountOutOfTurnInfo(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	history := []Message{{
		Direction:        "OUTBOUND",
		Body:             askPassengerCountReply,
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       now.Add(-1 * time.Minute),
	}}
	state := CanonicalConversationState{Phase: ConversationPhasePassengerCollection}

	for _, text := range []string{
		"já paguei",
		"pagamento caiu?",
	} {
		t.Run(text, func(t *testing.T) {
			got := routeDeterministicIntent(history, text, state, now)
			if got.Intent != IntentPaymentStatusQuery {
				t.Fatalf("expected payment status query to win, got %+v", got)
			}
			if got.TemplateName == TemplatePaymentOptionsInfo ||
				got.TemplateName == TemplateContextFallbackPassengerCount {
				t.Fatalf("status query must not become info/fallback template: %+v", got)
			}
		})
	}
}

func TestIntentRouterPayingPassengerInfoQuestionsUseClosedTemplate(t *testing.T) {
	for _, text := range []string{
		"o que é passageiro pagante?",
		"quem é passageiro pagante?",
		"criança paga?",
		"maior de 5 anos paga?",
	} {
		t.Run(text, func(t *testing.T) {
			got := routeDeterministicIntent(nil, text, CanonicalConversationState{Phase: ConversationPhaseDiscovery}, time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC))

			if got.Intent != IntentPayingPassengerInfoQuestion {
				t.Fatalf("expected paying passenger info intent, got %+v", got)
			}
			if got.TemplateName != TemplatePayingPassengerInfo || got.Action != "template" {
				t.Fatalf("expected paying passenger info template, got %+v", got)
			}
		})
	}
}

func TestIntentRouterHumanSupportInfoDoesNotBreakExplicitHandoff(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)

	info := routeDeterministicIntent(nil, "qual telefone do suporte?", CanonicalConversationState{Phase: ConversationPhaseDiscovery}, now)
	if info.Intent != IntentHumanSupportInfoQuestion ||
		info.TemplateName != TemplateHumanSupportInfo ||
		info.Action != "template" {
		t.Fatalf("expected human support info template, got %+v", info)
	}

	handoff := routeDeterministicIntent(nil, "quero falar com atendente", CanonicalConversationState{Phase: ConversationPhasePassengerCollection}, now)
	if handoff.Intent != IntentHumanSupport ||
		handoff.TemplateName != TemplateHumanHandoff ||
		handoff.Action != "template" {
		t.Fatalf("expected explicit handoff to stay protected, got %+v", handoff)
	}
}

func TestIntentRouterSelectsExplicitAvailabilityOptionFromMultipleOptions(t *testing.T) {
	history := availabilityDateSelectionWithFiveOptionsHistory(t)
	state := CanonicalConversationState{Phase: ConversationPhaseRouteSelection}

	for _, text := range []string{"1", "primeira", "opção 1"} {
		t.Run(text, func(t *testing.T) {
			got := routeDeterministicIntent(history, text, state, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))

			if got.Intent != IntentSelectAvailabilityOption {
				t.Fatalf("expected explicit availability option selection, got %+v", got)
			}
			if got.SelectedOptionIndex != 1 {
				t.Fatalf("expected selected option index 1, got %+v", got)
			}
			if got.TemplateName != TemplateAskPassengerCount {
				t.Fatalf("expected passenger count template, got %+v", got)
			}
		})
	}
}

func TestIntentRouterContextualAvailabilitySelectionRequiresLatestAvailabilityPrompt(t *testing.T) {
	history := append(availabilitySelectionHistory(t), Message{
		Direction:        "OUTBOUND",
		Body:             "Consegui identificar estes dados. Eles conferem?",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       time.Now().UTC().Add(-1 * time.Minute),
	})
	state := CanonicalConversationState{Phase: ConversationPhaseBookingPending}

	got := routeDeterministicIntent(history, "essa mesmo", state, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))

	if got.Intent == IntentSelectAvailabilityOption {
		t.Fatalf("document confirmation context must not reuse stale availability list: %+v", got)
	}
}

func TestIntentRouterAvailabilitySearch(t *testing.T) {
	got := routeDeterministicIntent(nil, "Fraiburgo para monção 18/05", CanonicalConversationState{}, time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC))
	if got.Intent != IntentAvailabilitySearch || got.AvailabilityInput == nil {
		t.Fatalf("expected availability search decision, got %+v", got)
	}
}

func TestIntentRouterPrioritizesAvailabilityDateSelectionOverUnsupportedFollowUp(t *testing.T) {
	history := availabilityDateSelectionAfterRouteQuestionHistory(t)
	state := CanonicalConversationState{Phase: ConversationPhaseRouteSelection}

	for _, text := range []string{"06/7", "6/7", "6/07", "06/07"} {
		t.Run(text, func(t *testing.T) {
			got := routeDeterministicIntent(history, text, state, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))

			if got.Intent != IntentAvailabilitySearch {
				t.Fatalf("expected availability search intent, got %+v", got)
			}
			if got.Intent == IntentUnsupportedPackage || got.TemplateName == TemplateUnsupportedPackage {
				t.Fatalf("date selection must not become unsupported package: %+v", got)
			}
			if got.Source != "deterministic_availability_date_selection" {
				t.Fatalf("expected deterministic date selection source, got %+v", got)
			}
			if got.Action != "tool" || got.AvailabilityInput == nil {
				t.Fatalf("expected deterministic availability tool decision, got %+v", got)
			}
			if got.AvailabilityInput.TripDate == nil || got.AvailabilityInput.TripDate.UTC().Format("2006-01-02") != "2026-07-06" {
				t.Fatalf("expected selected trip date 2026-07-06, got %+v", got.AvailabilityInput)
			}
			if got.AvailabilityInput.Origin != "Santa Ines/MA" || got.AvailabilityInput.Destination != "Videira/SC" {
				t.Fatalf("expected route from availability context, got %+v", got.AvailabilityInput)
			}
		})
	}
}

func TestIntentRouterUsesActivePromptForAvailabilityDate(t *testing.T) {
	history := availabilityDateChoiceAfterRouteQuestionHistory(t)
	state := CanonicalConversationState{Phase: ConversationPhaseRouteSelection}

	got := routeDeterministicIntent(history, "06/07", state, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))

	if got.Intent != IntentAvailabilitySearch {
		t.Fatalf("expected availability search, got %+v", got)
	}
	if got.Action != "tool" || got.AvailabilityInput == nil {
		t.Fatalf("expected availability tool decision, got %+v", got)
	}
	if got.AvailabilityInput.TripDate == nil || got.AvailabilityInput.TripDate.UTC().Format("2006-01-02") != "2026-07-06" {
		t.Fatalf("expected selected trip date 2026-07-06, got %+v", got.AvailabilityInput)
	}
}

func TestIntentRouterActivePromptAvailabilityDateFallback(t *testing.T) {
	history := availabilityDateChoiceAfterRouteQuestionHistory(t)
	state := CanonicalConversationState{Phase: ConversationPhaseRouteSelection}

	got := routeDeterministicIntent(history, "ok", state, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))

	assertContextualFallbackDecision(t, got, TemplateContextFallbackAvailabilityDate)
}

func TestIntentRouterDoesNotLetUnsupportedStealAvailabilityOptionContext(t *testing.T) {
	history := availabilityDateSelectionAfterRouteQuestionHistory(t)
	state := CanonicalConversationState{Phase: ConversationPhaseRouteSelection}

	got := routeDeterministicIntent(history, "essa mesmo", state, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))

	if got.Intent == IntentUnsupportedPackage || got.TemplateName == TemplateUnsupportedPackage {
		t.Fatalf("availability option answer must not become unsupported package: %+v", got)
	}
	if got.Intent != IntentSelectAvailabilityOption {
		t.Fatalf("expected availability option selection, got %+v", got)
	}
}

func TestIntentRouterBlocksUnsupportedFollowUpInAvailabilityOptionContext(t *testing.T) {
	history := availabilityDateSelectionWithFiveOptionsHistory(t)
	state := CanonicalConversationState{Phase: ConversationPhaseRouteSelection}

	got := routeDeterministicIntent(history, "quero passagem para Bahia", state, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))

	if got.Intent == IntentUnsupportedPackage || got.TemplateName == TemplateUnsupportedPackage {
		t.Fatalf("unsupported follow-up must stay blocked during availability option choice: %+v", got)
	}
}

func TestIntentRouterDocumentConfirmationWinsOverStaleAvailability(t *testing.T) {
	history := append(availabilitySelectionHistory(t), Message{
		Direction:        "OUTBOUND",
		Body:             "Consegui identificar estes dados. Eles conferem?",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       time.Now().UTC().Add(-1 * time.Minute),
	})
	state := CanonicalConversationState{Phase: ConversationPhaseBookingPending}

	got := routeDeterministicIntent(history, "isso mesmo", state, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))

	if got.Intent != IntentDocumentConfirmation {
		t.Fatalf("expected document confirmation, got %+v", got)
	}
	if got.Intent == IntentSelectAvailabilityOption {
		t.Fatalf("document confirmation must not reuse stale availability list: %+v", got)
	}
}

func TestIntentRouterPassengerDocumentsContextualFallback(t *testing.T) {
	history := []Message{{
		Direction:        "OUTBOUND",
		Body:             "Pode enviar o nome completo e CPF, RG ou CNH completo do passageiro. Se preferir, envie foto legivel do documento.",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       time.Now().UTC().Add(-1 * time.Minute),
	}}
	state := CanonicalConversationState{Phase: ConversationPhasePassengerCollection}

	got := routeDeterministicIntent(history, "ok", state, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))

	assertContextualFallbackDecision(t, got, TemplateContextFallbackPassengerDocuments)

	got = routeDeterministicIntent(history, "Joao da Silva CPF 52998224725", state, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))
	if got.Intent != IntentPassengerDocumentsProvided {
		t.Fatalf("expected passenger documents intent, got %+v", got)
	}
}

func TestIntentRouterDocumentConfirmationContextualFallback(t *testing.T) {
	history := []Message{{
		Direction:        "OUTBOUND",
		Body:             "Consegui identificar estes dados. Eles conferem? Posso prosseguir e criar a reserva?",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       time.Now().UTC().Add(-1 * time.Minute),
	}}
	state := CanonicalConversationState{Phase: ConversationPhaseBookingPending}

	got := routeDeterministicIntent(history, "talvez", state, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))

	assertContextualFallbackDecision(t, got, TemplateContextFallbackDocumentConfirmation)

	got = routeDeterministicIntent(history, "certo", state, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))
	if got.Intent != IntentDocumentConfirmation {
		t.Fatalf("expected document confirmation, got %+v", got)
	}
}

func TestIntentRouterPaymentPreferenceRepliesWinBeforePaymentInfoTemplates(t *testing.T) {
	history := []Message{{
		Direction:        "OUTBOUND",
		Body:             "Prefere pagar o valor integral ou apenas o sinal?",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       time.Now().UTC().Add(-1 * time.Minute),
	}}
	state := CanonicalConversationState{Phase: ConversationPhaseBooked}

	for _, text := range []string{
		"sinal",
		"só o sinal",
		"apenas o sinal",
		"quero pagar o sinal",
		"vou pagar só o sinal",
		"sinal por passageiro pagante",
		"integral",
		"o valor integral",
		"quero pagar integral",
		"vou pagar tudo agora",
	} {
		t.Run(text, func(t *testing.T) {
			got := routeDeterministicIntent(history, text, state, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))

			if got.Intent != IntentPaymentPreference {
				t.Fatalf("expected payment preference, got %+v", got)
			}
			if got.TemplateName == TemplatePaymentOptionsInfo || got.TemplateName == TemplatePayingPassengerInfo {
				t.Fatalf("payment preference reply must not become informational template: %+v", got)
			}
		})
	}
}

func TestIntentRouterPaymentPreferencePromptStillAnswersPaymentInfoQuestions(t *testing.T) {
	history := []Message{{
		Direction:        "OUTBOUND",
		Body:             "Prefere pagar o valor integral ou apenas o sinal?",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       time.Now().UTC().Add(-1 * time.Minute),
	}}
	state := CanonicalConversationState{Phase: ConversationPhaseBooked}
	tests := []struct {
		text     string
		intent   Intent
		template ResponseTemplateName
	}{
		{text: "como funciona o pagamento?", intent: IntentPaymentInfoQuestion, template: TemplatePaymentOptionsInfo},
		{text: "paga agora?", intent: IntentPaymentInfoQuestion, template: TemplatePaymentOptionsInfo},
		{text: "paga no dia?", intent: IntentPaymentInfoQuestion, template: TemplatePaymentOptionsInfo},
		{text: "precisa pagar agora?", intent: IntentPaymentInfoQuestion, template: TemplatePaymentOptionsInfo},
		{text: "tem que pagar agora?", intent: IntentPaymentInfoQuestion, template: TemplatePaymentOptionsInfo},
		{text: "pode pagar no embarque?", intent: IntentPaymentInfoQuestion, template: TemplatePaymentOptionsInfo},
		{text: "paga agora ou no embarque?", intent: IntentPaymentInfoQuestion, template: TemplatePaymentOptionsInfo},
		{text: "o pagamento faz logo ou só no dia?", intent: IntentPaymentInfoQuestion, template: TemplatePaymentOptionsInfo},
		{text: "posso pagar só o sinal?", intent: IntentPaymentInfoQuestion, template: TemplatePaymentOptionsInfo},
		{text: "o que é passageiro pagante?", intent: IntentPayingPassengerInfoQuestion, template: TemplatePayingPassengerInfo},
		{text: "quem é passageiro pagante?", intent: IntentPayingPassengerInfoQuestion, template: TemplatePayingPassengerInfo},
	}

	for _, tc := range tests {
		t.Run(tc.text, func(t *testing.T) {
			got := routeDeterministicIntent(history, tc.text, state, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))

			if got.Intent != tc.intent || got.TemplateName != tc.template || got.Action != "template" {
				t.Fatalf("expected %s template %s, got %+v", tc.intent, tc.template, got)
			}
		})
	}
}

func TestIntentRouterAffirmativePayingPassengerPhraseIsNotInfoQuestion(t *testing.T) {
	got := routeDeterministicIntent(nil, "sinal por passageiro pagante", CanonicalConversationState{Phase: ConversationPhaseDiscovery}, time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC))

	if got.Intent == IntentPayingPassengerInfoQuestion || got.TemplateName == TemplatePayingPassengerInfo {
		t.Fatalf("affirmative payment preference phrase must not become paying passenger info: %+v", got)
	}
}

func TestIntentRouterPaymentPreferencePixUsesContextualFallback(t *testing.T) {
	history := []Message{{
		Direction:        "OUTBOUND",
		Body:             "Prefere pagar o valor integral ou apenas o sinal?",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       time.Now().UTC().Add(-1 * time.Minute),
	}}
	state := CanonicalConversationState{Phase: ConversationPhaseBooked}

	got := routeDeterministicIntent(history, "pix", state, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))

	if got.Intent == IntentPaymentPreference {
		t.Fatalf("pix-only answer must not become payment preference: %+v", got)
	}
	assertContextualFallbackDecision(t, got, TemplateContextFallbackPaymentPreference)
}

func TestIntentRouterPaymentPreferenceUnsupportedMethodStillUsesPaymentMethodsTemplate(t *testing.T) {
	history := []Message{{
		Direction:        "OUTBOUND",
		Body:             "Prefere pagar o valor integral ou apenas o sinal?",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       time.Now().UTC().Add(-1 * time.Minute),
	}}
	state := CanonicalConversationState{Phase: ConversationPhaseBooked}

	for _, text := range []string{"boleto", "cartao"} {
		t.Run(text, func(t *testing.T) {
			got := routeDeterministicIntent(history, text, state, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))

			if got.Intent != IntentPaymentMethodQuestion || got.TemplateName != TemplatePaymentMethods || got.Action != "template" {
				t.Fatalf("expected payment methods template, got %+v", got)
			}
		})
	}
}

func TestIntentRouterPassengerCountWithActivePrompt(t *testing.T) {
	history := []Message{{
		Direction:        "OUTBOUND",
		Body:             "A passagem e so para voce ou vai mais alguem junto? Tem crianca de 5 anos ou menos?",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       time.Now().UTC().Add(-1 * time.Minute),
	}}
	state := CanonicalConversationState{Phase: ConversationPhasePassengerCollection}

	for _, text := range []string{"só eu", "somente eu", "eu e mais uma pessoa", "2 pessoas"} {
		t.Run(text, func(t *testing.T) {
			got := routeDeterministicIntent(history, text, state, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))

			if got.Intent != IntentPassengerCountReply {
				t.Fatalf("expected passenger count reply, got %+v", got)
			}
		})
	}
}

func TestIntentRouterPassengerCountContextualFallback(t *testing.T) {
	history := []Message{{
		Direction:        "OUTBOUND",
		Body:             "A passagem e so para voce ou vai mais alguem junto? Tem crianca de 5 anos ou menos?",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       time.Now().UTC().Add(-1 * time.Minute),
	}}
	state := CanonicalConversationState{Phase: ConversationPhasePassengerCollection}

	got := routeDeterministicIntent(history, "ok", state, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))

	assertContextualFallbackDecision(t, got, TemplateContextFallbackPassengerCount)
}

func TestIntentRouterLapChildQuestionWithActivePrompt(t *testing.T) {
	history := []Message{{
		Direction:        "OUTBOUND",
		Body:             "Tem criança de 5 anos ou menos viajando?",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       time.Now().UTC().Add(-1 * time.Minute),
	}}
	state := CanonicalConversationState{Phase: ConversationPhasePassengerCollection}

	got := routeDeterministicIntent(history, "não", state, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))
	if got.Intent != IntentPassengerCountReply {
		t.Fatalf("expected passenger count reply, got %+v", got)
	}

	got = routeDeterministicIntent(history, "talvez", state, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))
	assertContextualFallbackDecision(t, got, TemplateContextFallbackChildUnder5)
}

func TestIntentRouterLapChildAssignmentDoesNotBecomeAvailabilitySelection(t *testing.T) {
	history := append(availabilitySelectionHistory(t), Message{
		Direction:        "OUTBOUND",
		Body:             "Recebi os dados dos 2 passageiros. Qual deles e a crianca de ate 5 anos?\n1. Joao Vitor Messias\n2. Ivoneide Messias",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       time.Now().UTC().Add(-1 * time.Minute),
	})
	state := CanonicalConversationState{Phase: ConversationPhaseBookingPending}

	got := routeDeterministicIntent(history, "1", state, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))

	if got.Intent == IntentSelectAvailabilityOption {
		t.Fatalf("lap child assignment index must not select availability option: %+v", got)
	}
	if got.Intent != IntentLapChildAssignmentAnswer {
		t.Fatalf("expected lap child assignment answer, got %+v", got)
	}
}

func TestIntentRouterActivePromptLapChildAssignmentFallback(t *testing.T) {
	history := []Message{{
		Direction:        "OUTBOUND",
		Body:             "Recebi os dados dos 2 passageiros. Qual deles e a crianca de ate 5 anos?\n1. Joao Vitor Messias\n2. Ivoneide Messias",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       time.Now().UTC().Add(-1 * time.Minute),
	}}
	state := CanonicalConversationState{Phase: ConversationPhaseBookingPending}

	invalid := routeDeterministicIntent(history, "ok", state, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))
	assertContextualFallbackDecision(t, invalid, TemplateContextFallbackLapChildAssignment)

	outOfRange := routeDeterministicIntent(history, "3", state, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))
	assertContextualFallbackDecision(t, outOfRange, TemplateContextFallbackLapChildAssignment)
}

func TestIntentRouterActivePromptLapChildAssignmentAcceptsMultiDigitOption(t *testing.T) {
	state := CanonicalConversationState{Phase: ConversationPhaseBookingPending}
	cases := []struct {
		passengerCount int
		reply          string
		wantIndex      int
	}{
		{passengerCount: 10, reply: "10", wantIndex: 10},
		{passengerCount: 12, reply: "12", wantIndex: 12},
	}

	for _, tc := range cases {
		t.Run(tc.reply, func(t *testing.T) {
			history := lapChildAssignmentHistoryWithPassengers(t, tc.passengerCount)

			got := routeDeterministicIntent(history, tc.reply, state, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))

			if got.Intent != IntentLapChildAssignmentAnswer {
				t.Fatalf("expected lap child assignment answer, got %+v", got)
			}
			if got.SelectedOptionIndex != tc.wantIndex {
				t.Fatalf("expected selected option %d, got %+v", tc.wantIndex, got)
			}
		})
	}
}

func TestIntentRouterActivePromptLapChildAssignmentMultiDigitOutOfRangeFallback(t *testing.T) {
	history := lapChildAssignmentHistoryWithPassengers(t, 10)
	state := CanonicalConversationState{Phase: ConversationPhaseBookingPending}

	got := routeDeterministicIntent(history, "11", state, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))

	assertContextualFallbackDecision(t, got, TemplateContextFallbackLapChildAssignment)
}

func TestActivePromptLapChildAssignmentOptionCountParsesMultiDigitLines(t *testing.T) {
	ctx := ActivePromptContext{SourceMessageBody: strings.Join([]string{
		"Recebi os dados. Qual passageiro e a crianca?",
		"1. Ana",
		"10. Maria",
		"12) Pedro",
		"3 - Joao",
		"CPF 123",
		"Passageiro sem numero",
	}, "\n")}

	if got := activePromptLapChildAssignmentOptionCount(ctx); got != 12 {
		t.Fatalf("expected max option 12, got %d", got)
	}
}

func TestIntentRouterReservationStartTemplateOnlyInDiscovery(t *testing.T) {
	discovery := routeDeterministicIntent(nil, "como faço uma reserva?", CanonicalConversationState{Phase: ConversationPhaseDiscovery}, time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC))
	if discovery.Intent != IntentAvailabilitySearch || discovery.TemplateName != TemplateAskReservationRouteSC {
		t.Fatalf("expected discovery reservation start template, got %+v", discovery)
	}

	activeCases := []CanonicalConversationState{
		{Phase: ConversationPhasePassengerCollection},
		{Phase: ConversationPhaseBookingPending},
		{Phase: ConversationPhaseBooked, Booking: CanonicalBookingState{BookingID: "booking-1"}},
		{Phase: ConversationPhasePaymentPending, Payment: CanonicalPaymentState{Status: "PENDING"}},
		{Phase: ConversationPhaseDiscovery, Route: CanonicalRouteState{Origin: "Moncao/MA"}},
	}
	for _, state := range activeCases {
		got := routeDeterministicIntent(nil, "quero reservar", state, time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC))
		if got.TemplateName == TemplateAskReservationRouteSC {
			t.Fatalf("did not expect reservation start template for active state %+v, got %+v", state, got)
		}
	}

	history := availabilitySelectionHistory(t)
	got := routeDeterministicIntent(history, "quero reservar", CanonicalConversationState{Phase: ConversationPhaseDiscovery}, time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC))
	if got.TemplateName == TemplateAskReservationRouteSC {
		t.Fatalf("did not expect reservation start template when availability list exists, got %+v", got)
	}
}

func TestReservationHowToProceedHelpersRecognizeNaturalReservationHelp(t *testing.T) {
	cases := []string{
		"queria saber como é que eu faço pra reservar uma passagem",
		"como é que eu faço pra reservar uma passagem",
		"como faço pra fazer uma reserva?",
		"como faço para fazer uma reserva",
		"como faço pra reservar uma passagem",
		"como faço para reservar uma passagem",
		"como reservar uma passagem",
		"quero fazer uma reserva",
	}

	for _, text := range cases {
		t.Run(text, func(t *testing.T) {
			if !looksLikeReservationHowToProceedIntent(text) {
				t.Fatalf("expected looksLikeReservationHowToProceedIntent to recognize %q", text)
			}
			if !looksLikeReservationStartTemplateIntent(text) {
				t.Fatalf("expected looksLikeReservationStartTemplateIntent to recognize %q", text)
			}
			if query, ok := inferUnsupportedPackageQuery(text); ok {
				t.Fatalf("reservation help must not be unsupported package, got %+v", query)
			}
		})
	}
}

func TestInferUnsupportedPackageQueryKeepsRealDestinationInReservationHelp(t *testing.T) {
	cases := []struct {
		text            string
		unsupported     bool
		wantDestination string
	}{
		{text: "como faço pra fazer uma reserva?"},
		{text: "quero fazer uma reserva"},
		{text: "como faço pra reservar uma passagem"},
		{text: "Oi, tudo certo. Queria saber como é que eu faço pra reservar uma passagem."},
		{text: "como faço pra reservar passagem para Bahia", unsupported: true, wantDestination: "bahia"},
		{text: "como faço pra fazer uma reserva para Bahia", unsupported: true, wantDestination: "bahia"},
		{text: "como é que eu faço pra reservar passagem para Bahia", unsupported: true, wantDestination: "bahia"},
		{text: "quero passagem para Bahia", unsupported: true, wantDestination: "bahia"},
		{text: "como faço pra reservar passagem para Santa Catarina"},
		{text: "como faço pra reservar passagem para Monção"},
	}

	for _, tc := range cases {
		t.Run(tc.text, func(t *testing.T) {
			query, unsupported := inferUnsupportedPackageQuery(tc.text)
			if unsupported != tc.unsupported {
				t.Fatalf("expected unsupported=%v for %q, got %v query=%+v", tc.unsupported, tc.text, unsupported, query)
			}
			if tc.wantDestination != "" && query.Destination != tc.wantDestination {
				t.Fatalf("expected destination %q, got %+v", tc.wantDestination, query)
			}
		})
	}
}

func TestIntentRouterNaturalReservationHelpStartsReservationInDiscovery(t *testing.T) {
	for _, text := range []string{
		"Oi, tudo certo. Queria saber como é que eu faço pra reservar uma passagem.",
		"como faço pra fazer uma reserva?",
		"quero fazer uma reserva",
	} {
		t.Run(text, func(t *testing.T) {
			got := routeDeterministicIntent(
				nil,
				text,
				CanonicalConversationState{Phase: ConversationPhaseDiscovery},
				time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC),
			)

			if got.Intent != IntentAvailabilitySearch {
				t.Fatalf("expected availability search intent, got %+v", got)
			}
			if got.Source != "deterministic_reservation_start" {
				t.Fatalf("expected deterministic_reservation_start source, got %+v", got)
			}
			if got.TemplateName != TemplateAskReservationRouteSC {
				t.Fatalf("expected template %s, got %+v", TemplateAskReservationRouteSC, got)
			}
			if got.Action != "template" {
				t.Fatalf("expected template action, got %+v", got)
			}
		})
	}
}

func TestIntentRouterExplicitSupportedRouteStillSearchesAvailability(t *testing.T) {
	for _, text := range []string{
		"de Monção para Videira",
		"como faço para reservar de Monção para Videira",
	} {
		t.Run(text, func(t *testing.T) {
			got := routeDeterministicIntent(
				nil,
				text,
				CanonicalConversationState{Phase: ConversationPhaseDiscovery},
				time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC),
			)

			if got.Intent != IntentAvailabilitySearch {
				t.Fatalf("expected availability search for supported route, got %+v", got)
			}
			if got.TemplateName == TemplateAskReservationRouteSC {
				t.Fatalf("explicit route must not become generic reservation start template: %+v", got)
			}
			if got.AvailabilityInput == nil {
				t.Fatalf("expected availability input for supported route, got %+v", got)
			}
		})
	}
}

func TestIntentRouterDoesNotClassifyUnsupportedRouteDuringPassengerCollection(t *testing.T) {
	history := []Message{
		{Direction: "OUTBOUND", Body: "Para qual cidade no Maranhao voce vai?"},
		{Direction: "OUTBOUND", Body: "Pode enviar seu nome completo e o documento. Se for foto, envie frente e verso."},
	}
	state := CanonicalConversationState{Phase: ConversationPhasePassengerCollection}

	got := routeDeterministicIntent(history, "Salvador", state, time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC))

	if got.Intent == IntentUnsupportedPackage {
		t.Fatalf("did not expect unsupported package during passenger collection, got %+v", got)
	}
	if got.Intent != IntentUnknown {
		t.Fatalf("expected unknown intent during passenger collection, got %+v", got)
	}
}

func TestIntentRouterDoesNotClassifyUnsupportedRouteDuringBookingPending(t *testing.T) {
	history := []Message{
		{Direction: "OUTBOUND", Body: "De qual cidade do Maranhao voce vai sair?"},
		{Direction: "OUTBOUND", Body: "Consegui identificar estes dados. Eles conferem? Posso prosseguir e criar a reserva?"},
	}
	state := CanonicalConversationState{Phase: ConversationPhaseBookingPending}

	got := routeDeterministicIntent(history, "Mas eu já enviei.", state, time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC))

	if got.Intent == IntentUnsupportedPackage {
		t.Fatalf("did not expect unsupported package during booking pending, got %+v", got)
	}
	if got.Intent != IntentUnknown {
		t.Fatalf("expected unknown intent during booking pending, got %+v", got)
	}
}

func TestDiscoveryUnsupportedPackageQueryStillMatches(t *testing.T) {
	cases := map[string]string{
		"quero passagem para Bahia": "bahia",
		"quero ir para Bahia":       "bahia",
	}

	for text, wantDestination := range cases {
		t.Run(text, func(t *testing.T) {
			query, ok := inferUnsupportedPackageQuery(text)
			if !ok {
				t.Fatal("expected unsupported package query in discovery")
			}
			if query.Destination != wantDestination {
				t.Fatalf("expected destination %q, got %+v", wantDestination, query)
			}
		})
	}
}

func TestIntentRouterRoutesBareCPFAsPaymentAfterPayerCPFRequest(t *testing.T) {
	history := []Message{
		{Direction: "OUTBOUND", Body: "Para qual cidade no Maranhao voce vai?"},
		{Direction: "OUTBOUND", Body: "Para gerar o PIX, preciso do CPF do pagador."},
	}
	state := CanonicalConversationState{Phase: ConversationPhaseBooked}

	got := routeDeterministicIntent(history, "52998224725", state, time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC))

	if got.Intent != IntentPaymentCreate {
		t.Fatalf("expected payment create intent, got %+v", got)
	}
	if got.Source != "deterministic_payer_document_reply" || got.Action != "tool" {
		t.Fatalf("unexpected payer document route decision: %+v", got)
	}
	if got.Intent == IntentUnsupportedPackage {
		t.Fatalf("CPF reply must not become unsupported package: %+v", got)
	}
}

func TestIntentRouterDoesNotRouteBareCPFWithoutPaymentContext(t *testing.T) {
	history := []Message{
		{Direction: "OUTBOUND", Body: "Para qual cidade no Maranhao voce vai?"},
	}
	state := CanonicalConversationState{Phase: ConversationPhaseBooked}

	got := routeDeterministicIntent(history, "52998224725", state, time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC))

	if got.Intent == IntentPaymentCreate {
		t.Fatalf("did not expect payment create without payer CPF prompt, got %+v", got)
	}
}

func TestIntentRouterDoesNotRouteInvalidBareCPF(t *testing.T) {
	history := []Message{
		{Direction: "OUTBOUND", Body: "Para gerar o PIX, preciso do CPF do pagador."},
	}
	state := CanonicalConversationState{Phase: ConversationPhaseBooked}

	got := routeDeterministicIntent(history, "12345678901", state, time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC))

	if got.Intent == IntentPaymentCreate {
		t.Fatalf("did not expect invalid CPF to route to payment create, got %+v", got)
	}
	assertContextualFallbackDecision(t, got, TemplateContextFallbackPayerCPF)
}

func TestIntentRouterPayerCPFContextualFallback(t *testing.T) {
	history := []Message{
		{Direction: "OUTBOUND", Body: "Para gerar o PIX, preciso do CPF do pagador."},
	}
	state := CanonicalConversationState{Phase: ConversationPhaseBooked}

	got := routeDeterministicIntent(history, "ok", state, time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC))

	assertContextualFallbackDecision(t, got, TemplateContextFallbackPayerCPF)
}

func TestSCDestinationFollowUpAfterPublicSCTableItuporanga(t *testing.T) {
	history := []Message{
		{Direction: "INBOUND", Body: "Passagem para Santa Catarina"},
		{Direction: "OUTBOUND", Body: "Oi Messias, temos sim. Valores por cidade em Santa Catarina:\nFraiburgo R$ 950\nVideira R$ 950\nItuporanga R$ 1100"},
	}

	got := routeDeterministicIntent(history, "Quero pra Ituporanga.", CanonicalConversationState{}, time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC))
	if got.Intent != IntentAvailabilitySearch {
		t.Fatalf("expected availability search intent, got %+v", got)
	}
	if got.Source != "deterministic" || got.Action != "template" {
		t.Fatalf("expected deterministic template action, got %+v", got)
	}
	if got.TemplateName != TemplateAskMAOrigin {
		t.Fatalf("expected template %s, got %+v", TemplateAskMAOrigin, got)
	}
	if got.AvailabilityInput == nil {
		t.Fatalf("expected availability input")
	}
	if got.AvailabilityInput.Destination != "Ituporanga/SC" {
		t.Fatalf("expected destination Ituporanga/SC, got %+v", got.AvailabilityInput)
	}
	if got.AvailabilityInput.PackageName != packageToSantaCatarina {
		t.Fatalf("expected package %q, got %+v", packageToSantaCatarina, got.AvailabilityInput)
	}
	if got.AvailabilityInput.Qty != 1 || got.AvailabilityInput.Limit != 8 {
		t.Fatalf("expected qty=1 limit=8, got %+v", got.AvailabilityInput)
	}
	if got.AvailabilityInput.Origin != "" || got.AvailabilityInput.TripDate != nil {
		t.Fatalf("expected follow-up to avoid origin/date resolution, got %+v", got.AvailabilityInput)
	}
}

func availabilityDateSelectionAfterRouteQuestionHistory(t *testing.T) []Message {
	t.Helper()
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	return markAvailabilityPromptHistoryDeliveredForTest([]Message{
		{
			Direction:        "OUTBOUND",
			Body:             "De qual cidade do Maranhao voce vai sair?",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-3 * time.Minute),
		},
		{
			Direction:        "OUTBOUND",
			Body:             "Encontrei estas opcoes:\n1. Santa Ines/MA para Videira/SC, 2026-07-06, saida 08:00, R$ 950\n\nQual opcao voce prefere?",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-2 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availabilityDateSelectionTestResult()),
				},
			},
		},
	})
}

func availabilitySingleOptionHistoryWithInvisibleFollowUp(now time.Time, invisibleStatus string, invisibleResult AvailabilitySearchResult, withInvisibleFacts bool) []Message {
	sentResult := availabilityDateSelectionTestResult()
	invisible := Message{
		ID:               "invisible-availability",
		Direction:        "OUTBOUND",
		Body:             buildAvailabilityListReply(invisibleResult),
		ProcessingStatus: invisibleStatus,
		ReceivedAt:       now.Add(-1 * time.Minute),
	}
	if withInvisibleFacts {
		invisible.Payload = map[string]interface{}{
			"tool_context": map[string]interface{}{
				toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(invisibleResult),
			},
		}
	}
	return markAvailabilityPromptHistoryDeliveredForTest([]Message{
		{
			ID:               "sent-availability",
			Direction:        "OUTBOUND",
			Body:             buildAvailabilityListReply(sentResult),
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-2 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(sentResult),
				},
			},
		},
		invisible,
	})
}

func availabilityDeliveryMirrorHistory(now time.Time, result AvailabilitySearchResult, withSourceDraft bool) []Message {
	body := buildAvailabilityListReply(result)
	resultIndexes := make([]int, visibleAvailabilityOptionCount(result))
	for index := range resultIndexes {
		resultIndexes[index] = index
	}
	presentation := availabilityPromptPresentationV1(result, resultIndexes)
	sourceEvent, ok := availabilityPromptEventForRunV1(
		RunAgentResult{AvailabilityPresentation: presentation},
		"draft-list",
	)
	if !ok {
		panic("invalid availability delivery mirror source fixture")
	}
	mirrorEvent := sourceEvent
	mirrorEvent.SourceMessageID = "mirror-list"
	mirrorPayload := map[string]interface{}{
		"mode":                              "BOT_AUTO_REPLY",
		"draft_message_id":                  "draft-list",
		availabilityPromptEventV1MessageKey: mirrorEvent,
	}
	mirror := markAvailabilityPromptDeliveredForTest(Message{
		ID:                "mirror-list",
		Direction:         "OUTBOUND",
		Body:              body,
		ProcessingStatus:  "SENT",
		ReceivedAt:        now.Add(-1 * time.Minute),
		Payload:           cloneMap(mirrorPayload),
		NormalizedPayload: cloneMap(mirrorPayload),
	})
	if !withSourceDraft {
		return []Message{mirror}
	}
	sourcePayload := map[string]interface{}{
		"mode":                              messageStatusAutomationDraft,
		availabilityPromptEventV1MessageKey: sourceEvent,
		"tool_context": map[string]interface{}{
			toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(result),
		},
	}
	return []Message{
		{
			ID:                "draft-list",
			Direction:         "OUTBOUND",
			Body:              body,
			ProcessingStatus:  messageStatusAutomationSent,
			ReceivedAt:        now.Add(-2 * time.Minute),
			Payload:           cloneMap(sourcePayload),
			NormalizedPayload: cloneMap(sourcePayload),
		},
		mirror,
	}
}

func lapChildAssignmentHistoryWithPassengers(t *testing.T, passengerCount int) []Message {
	t.Helper()
	var builder strings.Builder
	builder.WriteString("Recebi os dados dos passageiros. Qual deles e a crianca de ate 5 anos?")
	for i := 1; i <= passengerCount; i++ {
		builder.WriteString("\n")
		builder.WriteString(strconv.Itoa(i))
		builder.WriteString(". Passageiro ")
		builder.WriteString(strconv.Itoa(i))
	}
	return []Message{{
		Direction:        "OUTBOUND",
		Body:             builder.String(),
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       time.Now().UTC().Add(-1 * time.Minute),
	}}
}

func availabilityDateSelectionWithFiveOptionsHistory(t *testing.T) []Message {
	t.Helper()
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	return markAvailabilityPromptHistoryDeliveredForTest([]Message{
		{
			Direction:        "OUTBOUND",
			Body:             "De qual cidade do Maranhao voce vai sair?",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-3 * time.Minute),
		},
		{
			Direction: "OUTBOUND",
			Body: "Encontrei estas opcoes:\n" +
				"1. Santa Ines/MA para Videira/SC, 2026-07-06, saida 08:00, R$ 950\n" +
				"2. Santa Ines/MA para Videira/SC, 2026-07-06, saida 10:00, R$ 950\n" +
				"3. Santa Ines/MA para Videira/SC, 2026-07-06, saida 12:00, R$ 950\n" +
				"4. Santa Ines/MA para Videira/SC, 2026-07-06, saida 14:00, R$ 950\n" +
				"5. Santa Ines/MA para Videira/SC, 2026-07-06, saida 16:00, R$ 950\n\n" +
				"Qual opcao voce prefere?",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-2 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availabilityDateSelectionFiveOptionsTestResult()),
				},
			},
		},
	})
}

func availabilityDateChoiceAfterRouteQuestionHistory(t *testing.T) []Message {
	t.Helper()
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	return markAvailabilityPromptHistoryDeliveredForTest([]Message{
		{
			Direction:        "OUTBOUND",
			Body:             "De qual cidade do Maranhao voce vai sair?",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-3 * time.Minute),
		},
		{
			Direction:        "OUTBOUND",
			Body:             "Tenho datas disponiveis para essa viagem. Qual data prefere?",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-2 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availabilityDateSelectionTestResult()),
				},
			},
		},
	})
}

func availabilityDateSelectionTestResult() AvailabilitySearchResult {
	return AvailabilitySearchResult{
		Filter: AvailabilitySearchInput{
			Origin:      "Santa Ines/MA",
			Destination: "Videira/SC",
			PackageName: packageToSantaCatarina,
			Qty:         1,
			Limit:       5,
		},
		Results: []AvailabilitySearchItem{{
			TripID:                 "trip-2026-07-06",
			BoardStopID:            "board-2026-07-06",
			AlightStopID:           "alight-2026-07-06",
			OriginDisplayName:      "Santa Ines/MA",
			DestinationDisplayName: "Videira/SC",
			OriginDepartTime:       "08:00",
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

func availabilityOptionPromptHistory(now time.Time, result AvailabilitySearchResult) []Message {
	return markAvailabilityPromptHistoryDeliveredForTest([]Message{{
		Direction:        "OUTBOUND",
		Body:             buildAvailabilityListReply(result),
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       now.Add(-1 * time.Minute),
		Payload: map[string]interface{}{
			"tool_context": map[string]interface{}{
				toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(result),
			},
		},
	}})
}

func availabilityOptionPromptFutureResult() AvailabilitySearchResult {
	return AvailabilitySearchResult{
		Filter: AvailabilitySearchInput{
			Origin:      "Videira/SC",
			Destination: "Santa Ines/MA",
			PackageName: packageToMaranhao,
			Qty:         1,
			Limit:       5,
		},
		Results: []AvailabilitySearchItem{{
			TripID:                 "trip-2026-07-13",
			BoardStopID:            "board-2026-07-13",
			AlightStopID:           "alight-2026-07-13",
			OriginDisplayName:      "Videira/SC",
			DestinationDisplayName: "Santa Ines/MA",
			OriginDepartTime:       "13:00",
			TripDate:               "2026-07-13",
			SeatsAvailable:         5,
			Price:                  950,
			Currency:               "BRL",
			Status:                 "ACTIVE",
			TripStatus:             "SCHEDULED",
			PackageName:            packageToMaranhao,
		}},
	}
}

func availabilityOptionPromptTwoOptionsFutureResult() AvailabilitySearchResult {
	result := availabilityOptionPromptFutureResult()
	second := result.Results[0]
	second.TripID = "trip-2026-07-14"
	second.BoardStopID = "board-2026-07-14"
	second.AlightStopID = "alight-2026-07-14"
	second.OriginDepartTime = "14:00"
	second.TripDate = "2026-07-14"
	result.Results = []AvailabilitySearchItem{result.Results[0], second}
	return result
}

func availabilityOptionPromptFiveOptionsFutureResult() AvailabilitySearchResult {
	result := availabilityOptionPromptFutureResult()
	options := make([]AvailabilitySearchItem, 0, 5)
	for day := 13; day <= 17; day++ {
		item := result.Results[0]
		suffix := "2026-07-" + twoDigit(day)
		item.TripID = "trip-" + suffix
		item.BoardStopID = "board-" + suffix
		item.AlightStopID = "alight-" + suffix
		item.OriginDepartTime = twoDigit(day) + ":00"
		item.TripDate = suffix
		options = append(options, item)
	}
	result.Results = options
	return result
}

func availabilityOptionPromptHiddenPastPrefixResult() AvailabilitySearchResult {
	result := availabilityOptionPromptFutureResult()
	hiddenPast := result.Results[0]
	hiddenPast.TripID = "trip-2026-07-06"
	hiddenPast.BoardStopID = "board-2026-07-06"
	hiddenPast.AlightStopID = "alight-2026-07-06"
	hiddenPast.OriginDepartTime = "08:00"
	hiddenPast.TripDate = "2026-07-06"
	result.Results = []AvailabilitySearchItem{hiddenPast, result.Results[0]}
	return result
}

func availabilityTestObservedAt() time.Time {
	return time.Now().UTC()
}

func availabilityOptionPromptFutureResultAt(observedAt time.Time) AvailabilitySearchResult {
	result := availabilityOptionPromptFutureResult()
	result.Results[0] = availabilityOptionPromptItemAt(result.Results[0], availabilityTestDate(observedAt, 7), "13:00")
	return result
}

func availabilityOptionPromptTwoOptionsFutureResultAt(observedAt time.Time) AvailabilitySearchResult {
	result := availabilityOptionPromptFutureResultAt(observedAt)
	second := availabilityOptionPromptItemAt(result.Results[0], availabilityTestDate(observedAt, 8), "14:00")
	result.Results = []AvailabilitySearchItem{result.Results[0], second}
	return result
}

func availabilityOptionPromptTodayAndFutureResultAt(observedAt time.Time) AvailabilitySearchResult {
	result := availabilityOptionPromptFutureResult()
	today := availabilityOptionPromptItemAt(result.Results[0], availabilityTestDate(observedAt, 0), "13:00")
	future := availabilityOptionPromptItemAt(result.Results[0], availabilityTestDate(observedAt, 1), "14:00")
	result.Results = []AvailabilitySearchItem{today, future}
	return result
}

func availabilityOptionPromptFiveOptionsFutureResultAt(observedAt time.Time) AvailabilitySearchResult {
	result := availabilityOptionPromptFutureResultAt(observedAt)
	options := make([]AvailabilitySearchItem, 0, 5)
	for index := 0; index < 5; index++ {
		item := availabilityOptionPromptItemAt(
			result.Results[0],
			availabilityTestDate(observedAt, 7+index),
			twoDigit(13+index)+":00",
		)
		options = append(options, item)
	}
	result.Results = options
	return result
}

func availabilityOptionPromptHiddenPastPrefixResultAt(observedAt time.Time) AvailabilitySearchResult {
	result := availabilityOptionPromptFutureResultAt(observedAt)
	hiddenPast := availabilityOptionPromptItemAt(result.Results[0], availabilityTestDate(observedAt, -7), "08:00")
	result.Results = []AvailabilitySearchItem{hiddenPast, result.Results[0]}
	return result
}

func availabilityOptionPromptItemAt(item AvailabilitySearchItem, tripDate time.Time, departureTime string) AvailabilitySearchItem {
	suffix := tripDate.UTC().Format("2006-01-02")
	item.TripID = "trip-" + suffix
	item.BoardStopID = "board-" + suffix
	item.AlightStopID = "alight-" + suffix
	item.OriginDepartTime = departureTime
	item.TripDate = suffix
	return item
}

func availabilityTestDate(observedAt time.Time, dayOffset int) time.Time {
	observedAt = observedAt.UTC()
	today := time.Date(observedAt.Year(), observedAt.Month(), observedAt.Day(), 0, 0, 0, 0, time.UTC)
	return today.AddDate(0, 0, dayOffset)
}

func availabilityTestDateInput(t *testing.T, tripDate string) string {
	t.Helper()
	parsed, err := time.Parse("2006-01-02", tripDate)
	if err != nil {
		t.Fatalf("parse availability fixture trip date %q: %v", tripDate, err)
	}
	return parsed.Format("02/01/2006")
}

func availabilityTestDayMonthMetadata(t *testing.T, tripDate string) string {
	t.Helper()
	parsed, err := time.Parse("2006-01-02", tripDate)
	if err != nil {
		t.Fatalf("parse availability fixture trip date %q: %v", tripDate, err)
	}
	return parsed.Format("02/01")
}

func availabilityOptionPromptDuplicateVisibleDateResult() AvailabilitySearchResult {
	result := availabilityOptionPromptTwoOptionsFutureResult()
	result.Results[1].TripDate = result.Results[0].TripDate
	return result
}

func availabilityOptionPromptHiddenSixthDateResult() AvailabilitySearchResult {
	result := availabilityOptionPromptFutureResult()
	options := make([]AvailabilitySearchItem, 0, 6)
	for day := 8; day <= 13; day++ {
		item := result.Results[0]
		item.TripID = "trip-2026-07-" + twoDigit(day)
		item.BoardStopID = "board-2026-07-" + twoDigit(day)
		item.AlightStopID = "alight-2026-07-" + twoDigit(day)
		item.OriginDepartTime = twoDigit(day) + ":00"
		item.TripDate = "2026-07-" + twoDigit(day)
		options = append(options, item)
	}
	result.Results = options
	return result
}

func twoDigit(value int) string {
	if value < 10 {
		return "0" + strconv.Itoa(value)
	}
	return strconv.Itoa(value)
}

func availabilityDateSelectionFiveOptionsTestResult() AvailabilitySearchResult {
	result := availabilityDateSelectionTestResult()
	first := result.Results[0]
	second := first
	second.TripID = "trip-2026-07-06-2"
	second.BoardStopID = "board-2026-07-06-2"
	second.AlightStopID = "alight-2026-07-06-2"
	second.OriginDepartTime = "10:00"
	third := first
	third.TripID = "trip-2026-07-06-3"
	third.BoardStopID = "board-2026-07-06-3"
	third.AlightStopID = "alight-2026-07-06-3"
	third.OriginDepartTime = "12:00"
	fourth := first
	fourth.TripID = "trip-2026-07-06-4"
	fourth.BoardStopID = "board-2026-07-06-4"
	fourth.AlightStopID = "alight-2026-07-06-4"
	fourth.OriginDepartTime = "14:00"
	fifth := first
	fifth.TripID = "trip-2026-07-06-5"
	fifth.BoardStopID = "board-2026-07-06-5"
	fifth.AlightStopID = "alight-2026-07-06-5"
	fifth.OriginDepartTime = "16:00"
	result.Results = []AvailabilitySearchItem{first, second, third, fourth, fifth}
	return result
}

func availabilityDateSelectionEightOptionsTestResult() AvailabilitySearchResult {
	result := availabilityDateSelectionFiveOptionsTestResult()
	first := result.Results[0]
	for i := 6; i <= 8; i++ {
		item := first
		item.TripID = "trip-2026-07-06-" + strconv.Itoa(i)
		item.BoardStopID = "board-2026-07-06-" + strconv.Itoa(i)
		item.AlightStopID = "alight-2026-07-06-" + strconv.Itoa(i)
		item.OriginDepartTime = strconv.Itoa(6+i*2) + ":00"
		result.Results = append(result.Results, item)
	}
	return result
}
