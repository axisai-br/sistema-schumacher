package chat

import (
	"strings"
	"testing"
	"time"
)

func TestResponseRealizerTemplates(t *testing.T) {
	reply, ok := realizeResponseTemplate(TemplateAskPassengerCount)
	if !ok || reply != askPassengerCountReply {
		t.Fatalf("unexpected passenger count template: ok=%t reply=%q", ok, reply)
	}
	reply, ok = realizeResponseTemplate(TemplateUnsupportedCargo)
	if !ok || reply == "" {
		t.Fatalf("expected unsupported cargo template")
	}
}

func TestResponseRealizerEveryStaticTemplateRendersWithoutEmptyPlaceholders(t *testing.T) {
	templates := []ResponseTemplateName{
		TemplateAskPassengerCount,
		TemplateAskChildUnder5,
		TemplateAskDocuments,
		TemplateAskPaymentChoice,
		TemplateAskMAOrigin,
		TemplateAskMADestination,
		TemplateAskSCOrigin,
		TemplateAskSCOriginForMA,
		TemplatePublicSCTable,
		TemplateUnsupportedCargo,
		TemplateHumanHandoff,
	}
	for _, template := range templates {
		reply, ok := realizeResponseTemplate(template)
		if !ok {
			t.Fatalf("expected template %s to render", template)
		}
		assertNoEmptyTemplateArtifacts(t, reply)
	}
}

func TestResponseRealizerContextualMATemplates(t *testing.T) {
	reply, ok := realizeIntentResponseTemplate(IntentDecision{
		TemplateName: TemplateAskMADestination,
		AvailabilityInput: &AvailabilitySearchInput{
			Origin: "Chapeco",
		},
	})
	if !ok || reply != "Perfeito — Chapeco/SC. Para qual cidade do Maranhao voce quer ir?" {
		t.Fatalf("unexpected MA destination reply: ok=%t reply=%q", ok, reply)
	}
	reply, ok = realizeIntentResponseTemplate(IntentDecision{
		TemplateName: TemplateAskMAOrigin,
		AvailabilityInput: &AvailabilitySearchInput{
			Destination: "Videira",
		},
	})
	if !ok || reply != "Perfeito — Videira/SC. De qual cidade do Maranhao voce vai sair?" {
		t.Fatalf("unexpected MA origin reply: ok=%t reply=%q", ok, reply)
	}
}

func TestResponseRealizerNoDuplicateMADestinationCase(t *testing.T) {
	directReply, ok := realizeResponseTemplate(TemplateAskMADestination)
	if !ok || directReply == "" {
		t.Fatalf("expected direct MADestination template reply")
	}

	intentReply, ok := realizeIntentResponseTemplate(IntentDecision{TemplateName: TemplateAskMADestination})
	if !ok || intentReply != directReply {
		t.Fatalf("expected intent realizer to fall back to direct template reply, got ok=%t reply=%q direct=%q", ok, intentReply, directReply)
	}
}

func TestResponseRealizerRequiresToolFactForSelection(t *testing.T) {
	decision := IntentDecision{Intent: IntentSelectAvailabilityOption, SelectedOptionIndex: 1, TemplateName: TemplateAskPassengerCount}
	if canRealizeWithoutLLM(decision, CanonicalConversationState{}) {
		t.Fatal("expected selection template to require previous availability fact")
	}
	state := deriveCanonicalConversationState(Session{ID: "session-1", HandoffStatus: "BOT"}, availabilitySelectionHistory(t), "")
	if !canRealizeWithoutLLM(decision, state) {
		t.Fatal("expected selection template with previous availability fact")
	}
}

func TestResponseRealizerOperationalTemplatesRequireToolFacts(t *testing.T) {
	if canRealizeWithoutLLM(IntentDecision{Intent: IntentAvailabilitySearch, TemplateName: TemplateNoAvailability}, discoveryState()) {
		t.Fatal("expected no availability template to require tool facts")
	}
	if canRealizeWithoutLLM(IntentDecision{Intent: IntentAvailabilitySearch, TemplateName: TemplateAvailabilityList}, discoveryState()) {
		t.Fatal("expected availability list template to require tool facts")
	}
	if canRealizeWithoutLLM(IntentDecision{Intent: IntentPaymentPreference, TemplateName: TemplateAskPaymentChoice}, discoveryState()) {
		t.Fatal("expected payment choice template to require booking context")
	}
	state := discoveryState()
	state.Booking = CanonicalBookingState{BookingID: "booking-1"}
	if !canRealizeWithoutLLM(IntentDecision{Intent: IntentPaymentPreference, TemplateName: TemplateAskPaymentChoice}, state) {
		t.Fatal("expected payment choice template with booking context")
	}
}

func TestAvailabilityListRespectsLimitAndDoesNotExposeInternalIDs(t *testing.T) {
	result := AvailabilitySearchResult{}
	for i := 0; i < 6; i++ {
		result.Results = append(result.Results, AvailabilitySearchItem{
			SegmentID:              "segment-secret",
			TripID:                 "trip-secret",
			RouteID:                "route-secret",
			BoardStopID:            "board-secret",
			AlightStopID:           "alight-secret",
			OriginDisplayName:      "Chapeco",
			DestinationDisplayName: "Santa Ines",
			TripDate:               "2099-05-20",
			OriginDepartTime:       "08:30",
			Price:                  1100,
		})
	}

	reply := buildAvailabilityListReply(result)

	if count := strings.Count(reply, "Chapeco para Santa Ines"); count != 5 {
		t.Fatalf("expected 5 rendered options, got %d in %q", count, reply)
	}
	assertNoInternalIDs(t, reply)
	assertNoEmptyTemplateArtifacts(t, reply)
	if !strings.Contains(reply, "Qual opcao voce prefere?") {
		t.Fatalf("expected option question, got %q", reply)
	}
}

func TestNoAvailabilityReplyIncludesKnownFiltersAndDoesNotInventAlternatives(t *testing.T) {
	tripDate := time.Date(2099, 5, 20, 0, 0, 0, 0, time.UTC)
	reply := buildNoAvailabilityReply(AvailabilitySearchResult{
		Filter: AvailabilitySearchInput{
			Origin:      "Chapeco",
			Destination: "Santa Ines",
			PackageName: "Pacote p/ Maranhao",
			TripDate:    &tripDate,
		},
	})

	if !containsAll(reply, "Chapeco para Santa Ines", "Pacote p/ Maranhao", "20/05/2099") {
		t.Fatalf("expected known filters in reply, got %q", reply)
	}
	if strings.Contains(strings.ToLower(reply), "outra data") || strings.Contains(strings.ToLower(reply), "alternativa") {
		t.Fatalf("expected no invented alternatives, got %q", reply)
	}
	assertNoInternalIDs(t, reply)
	assertNoEmptyTemplateArtifacts(t, reply)
}

func TestBookingCreatedReplyRequiresBookingCreateResult(t *testing.T) {
	if reply := buildBookingCreatedReply(BookingCreateResult{}); reply != "" {
		t.Fatalf("expected empty reply without booking_create facts, got %q", reply)
	}
	reply := buildBookingCreatedReply(BookingCreateResult{BookingID: "booking-1", ReservationCode: "SCH-1", Status: "reserved"})
	if reply != askPaymentChoiceReply {
		t.Fatalf("unexpected booking-created reply: %q", reply)
	}
	assertNoInternalIDs(t, reply)
}

func TestAskDocumentsOnlyMissingPassengers(t *testing.T) {
	reply := buildAskDocumentsReply(3, 1)
	if !strings.Contains(reply, "dos 2 passageiros faltantes") {
		t.Fatalf("expected only missing passenger count, got %q", reply)
	}
	if !strings.Contains(reply, "CPF ou RG") {
		t.Fatalf("expected accepted documents, got %q", reply)
	}
}

func TestConfirmExtractedDocumentTemplate(t *testing.T) {
	reply := buildConfirmExtractedDocumentReply(DocumentExtractResult{
		ExpectedPassengerCount: 1,
		Passengers: []DocumentExtractPassenger{
			{Name: "Joao Vitor Messias", DocumentType: "CPF", Document: "06645648103"},
		},
	})
	if !containsAll(reply, "Joao Vitor Messias", "CPF", "066.***.***-03", "Eles conferem? Posso prosseguir e criar a reserva?") {
		t.Fatalf("unexpected document confirmation reply: %q", reply)
	}
	assertNoInternalIDs(t, reply)
	assertNoEmptyTemplateArtifacts(t, reply)
}

func TestPaymentCreateReplyKeepsPixCodeEasyToCopy(t *testing.T) {
	reply := buildPaymentCreateReply(PaymentCreateResult{
		Mode:      "pix_sent",
		AmountDue: 250,
		PixCode:   "000201PIXCODE",
	})

	if reply != "000201PIXCODE" {
		t.Fatalf("expected PIX-only reply, got %q", reply)
	}
	assertNoInternalIDs(t, reply)
	assertNoEmptyTemplateArtifacts(t, reply)
}

func assertNoInternalIDs(t *testing.T, reply string) {
	t.Helper()
	blocked := []string{"trip-secret", "route-secret", "segment-secret", "board-secret", "alight-secret", "booking-1"}
	for _, item := range blocked {
		if strings.Contains(reply, item) {
			t.Fatalf("reply exposed internal id %q: %q", item, reply)
		}
	}
}

func assertNoEmptyTemplateArtifacts(t *testing.T, reply string) {
	t.Helper()
	if strings.TrimSpace(reply) == "" {
		t.Fatal("expected non-empty reply")
	}
	for _, artifact := range []string{"{{", "}}", "<nil>", "%!", "  .", " ,"} {
		if strings.Contains(reply, artifact) {
			t.Fatalf("reply contains template artifact %q: %q", artifact, reply)
		}
	}
}
