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
		{text: "essa opção", intent: IntentSelectAvailabilityOption, index: 1},
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
	query, ok := inferUnsupportedPackageQuery("quero passagem para Bahia")
	if !ok {
		t.Fatal("expected unsupported package query in discovery")
	}
	if query.Destination != "bahia" {
		t.Fatalf("expected destination bahia, got %+v", query)
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
