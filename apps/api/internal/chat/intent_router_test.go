package chat

import (
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

	for _, text := range []string{"essa mesmo", "essa mesma"} {
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
}

func TestIntentRouterDoesNotSelectAmbiguousAvailabilityReplyFromMultipleOptions(t *testing.T) {
	history := availabilityDateSelectionWithFiveOptionsHistory(t)
	state := CanonicalConversationState{Phase: ConversationPhaseRouteSelection}

	for _, text := range []string{"sim", "ok", "certo", "isso", "isso mesmo", "pode ser", "essa mesmo", "essa mesma", "pode ser essa"} {
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
		})
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

func TestIntentRouterPaymentPreferenceSinalWithActivePrompt(t *testing.T) {
	history := []Message{{
		Direction:        "OUTBOUND",
		Body:             "Prefere pagar o valor integral ou apenas o sinal?",
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       time.Now().UTC().Add(-1 * time.Minute),
	}}
	state := CanonicalConversationState{Phase: ConversationPhaseBooked}

	got := routeDeterministicIntent(history, "sinal", state, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))

	if got.Intent != IntentPaymentPreference {
		t.Fatalf("expected payment preference, got %+v", got)
	}
}

func TestIntentRouterPaymentPreferencePixStillUnknown(t *testing.T) {
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
	if got.Intent != IntentUnknown {
		t.Fatalf("expected pix-only payment preference answer to remain unknown, got %+v", got)
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
		"como faço pra reservar uma passagem",
		"como faço para reservar uma passagem",
		"como reservar uma passagem",
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
		{text: "como faço pra reservar uma passagem"},
		{text: "Oi, tudo certo. Queria saber como é que eu faço pra reservar uma passagem."},
		{text: "como faço pra reservar passagem para Bahia", unsupported: true, wantDestination: "bahia"},
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
	got := routeDeterministicIntent(
		nil,
		"Oi, tudo certo. Queria saber como é que eu faço pra reservar uma passagem.",
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
	return []Message{
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
	}
}

func availabilityDateSelectionWithFiveOptionsHistory(t *testing.T) []Message {
	t.Helper()
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	return []Message{
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
	}
}

func availabilityDateChoiceAfterRouteQuestionHistory(t *testing.T) []Message {
	t.Helper()
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	return []Message{
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
	}
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
