package chat

import (
	"reflect"
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
		TemplateAskReservationRouteSC,
		TemplatePublicSCTable,
		TemplateUnsupportedCargo,
		TemplateHumanHandoff,
		TemplatePaymentOptionsInfo,
		TemplatePayingPassengerInfo,
		TemplateDocumentRequirementsInfo,
		TemplateChildPolicyInfo,
		TemplateBaggageInfo,
		TemplateBoardingInfo,
		TemplateHumanSupportInfo,
		TemplateContextFallbackAvailabilityOption,
		TemplateContextFallbackAvailabilityDate,
		TemplateContextFallbackPassengerCount,
		TemplateContextFallbackChildUnder5,
		TemplateContextFallbackLapChildAssignment,
		TemplateContextFallbackPassengerDocuments,
		TemplateContextFallbackDocumentConfirmation,
		TemplateContextFallbackPaymentPreference,
		TemplateContextFallbackPayerCPF,
	}
	for _, template := range templates {
		reply, ok := realizeResponseTemplate(template)
		if !ok {
			t.Fatalf("expected template %s to render", template)
		}
		assertNoEmptyTemplateArtifacts(t, reply)
	}
}

func TestResponseRealizerPaymentInfoTemplates(t *testing.T) {
	paymentReply, ok := realizeResponseTemplate(TemplatePaymentOptionsInfo)
	if !ok || paymentReply != paymentOptionsInfoReply {
		t.Fatalf("unexpected payment options info template: ok=%t reply=%q", ok, paymentReply)
	}
	if paymentReply != "O pagamento pode ser realizado de 2 formas: você pode pagar agora o valor integral, ou pagar agora apenas o sinal de R$ 250 por passageiro pagante e pagar o restante no embarque." {
		t.Fatalf("payment options reply does not match expected production text: %q", paymentReply)
	}

	passengerReply, ok := realizeResponseTemplate(TemplatePayingPassengerInfo)
	if !ok || passengerReply != payingPassengerInfoReply {
		t.Fatalf("unexpected paying passenger info template: ok=%t reply=%q", ok, passengerReply)
	}
	if passengerReply != "Passageiro pagante é o passageiro maior de 5 anos." {
		t.Fatalf("paying passenger reply does not match expected production text: %q", passengerReply)
	}
}

func TestResponseRealizerContextualFallbackTemplates(t *testing.T) {
	templates := []ResponseTemplateName{
		TemplateContextFallbackAvailabilityOption,
		TemplateContextFallbackAvailabilityDate,
		TemplateContextFallbackPassengerCount,
		TemplateContextFallbackChildUnder5,
		TemplateContextFallbackLapChildAssignment,
		TemplateContextFallbackPassengerDocuments,
		TemplateContextFallbackDocumentConfirmation,
		TemplateContextFallbackPaymentPreference,
		TemplateContextFallbackPayerCPF,
	}

	for _, template := range templates {
		t.Run(string(template), func(t *testing.T) {
			reply, ok := realizeIntentResponseTemplate(IntentDecision{TemplateName: template})
			if !ok || strings.TrimSpace(reply) == "" {
				t.Fatalf("expected contextual fallback template %s to render, ok=%t reply=%q", template, ok, reply)
			}
			assertNoEmptyTemplateArtifacts(t, reply)
		})
	}
}

func TestApplyIntentDecisionContextualFallbackDoesNotMutateCanonicalState(t *testing.T) {
	before := CanonicalConversationState{
		SessionID: "session-1",
		Phase:     ConversationPhaseTripSelection,
		Route: CanonicalRouteState{
			Origin:              "Santa Ines/MA",
			Destination:         "Videira/SC",
			PackageName:         packageToSantaCatarina,
			SelectedOptionIndex: 2,
			TripID:              "trip-2",
		},
		LastToolFacts: map[string]interface{}{
			toolNameAvailabilitySearch: map[string]interface{}{
				"package_name": packageToSantaCatarina,
			},
		},
		AllowedNextActions: []string{string(IntentSelectAvailabilityOption)},
	}
	templates := []ResponseTemplateName{
		TemplateContextFallbackAvailabilityOption,
		TemplateContextFallbackAvailabilityDate,
		TemplateContextFallbackPassengerCount,
		TemplateContextFallbackChildUnder5,
		TemplateContextFallbackLapChildAssignment,
		TemplateContextFallbackPassengerDocuments,
		TemplateContextFallbackDocumentConfirmation,
		TemplateContextFallbackPaymentPreference,
		TemplateContextFallbackPayerCPF,
	}

	for _, template := range templates {
		t.Run(string(template), func(t *testing.T) {
			after := applyIntentDecisionToCanonicalState(before, IntentDecision{
				Intent:       IntentUnknown,
				Action:       "template",
				TemplateName: template,
			})
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("contextual fallback must not mutate canonical_state\nbefore=%+v\nafter=%+v", before, after)
			}
		})
	}
}

func TestApplyIntentDecisionInformationalTemplatesDoNotMutateCanonicalState(t *testing.T) {
	before := CanonicalConversationState{
		SessionID: "session-1",
		Phase:     ConversationPhaseBooked,
		Route: CanonicalRouteState{
			Origin:              "Santa Ines/MA",
			Destination:         "Videira/SC",
			PackageName:         packageToSantaCatarina,
			SelectedOptionIndex: 1,
			TripID:              "trip-1",
		},
		Booking: CanonicalBookingState{BookingID: "booking-1"},
		LastToolFacts: map[string]interface{}{
			toolNameAvailabilitySearch: map[string]interface{}{
				"package_name": packageToSantaCatarina,
			},
		},
		AllowedNextActions: []string{string(IntentPaymentPreference), string(IntentPaymentCreate)},
	}
	templates := []ResponseTemplateName{
		TemplatePaymentOptionsInfo,
		TemplatePayingPassengerInfo,
		TemplateDocumentRequirementsInfo,
		TemplateChildPolicyInfo,
		TemplateBaggageInfo,
		TemplateBoardingInfo,
		TemplateHumanSupportInfo,
	}

	for _, template := range templates {
		t.Run(string(template), func(t *testing.T) {
			after := applyIntentDecisionToCanonicalState(before, IntentDecision{
				Intent:       IntentPaymentInfoQuestion,
				Action:       "template",
				TemplateName: template,
			})
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("informational template must not mutate canonical_state\nbefore=%+v\nafter=%+v", before, after)
			}
		})
	}
}

func TestResponseRealizerOutOfTurnInfoAppendsPendingPromptReminder(t *testing.T) {
	reply, ok := realizeIntentResponseTemplate(IntentDecision{
		Intent:       IntentPaymentInfoQuestion,
		Action:       "template",
		TemplateName: TemplatePaymentOptionsInfo,
		TemplateData: map[string]interface{}{
			outOfTurnTemplateDataKey:              true,
			outOfTurnPendingPromptTemplateDataKey: string(TemplateContextFallbackPassengerCount),
		},
	})
	if !ok {
		t.Fatal("expected out-of-turn payment info template to render")
	}
	folded := foldChatText(reply)
	if !strings.Contains(folded, "pagamento pode ser realizado") {
		t.Fatalf("expected payment answer in reply, got %q", reply)
	}
	if !strings.Contains(folded, "para continuar") ||
		!strings.Contains(folded, "passagem e so para voce") {
		t.Fatalf("expected pending passenger prompt reminder, got %q", reply)
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
	if !strings.Contains(reply, "CPF, RG ou CNH") {
		t.Fatalf("expected accepted documents, got %q", reply)
	}
}

func TestConfirmExtractedDocumentTemplate(t *testing.T) {
	reply := buildConfirmExtractedDocumentReply(DocumentExtractResult{
		ExpectedPassengerCount: 1,
		Passengers: []DocumentExtractPassenger{
			{
				Name:                   "Joao Vitor Messias",
				DocumentType:           "CPF",
				Document:               "06645648103",
				BirthDate:              "2022-05-21",
				BirthCertificateNumber: testBirthCertificateNumber,
				BirthCity:              "Santa Ines",
			},
		},
	})
	if !containsAll(reply, "Joao Vitor Messias", "CPF", "066.***.***-03", "nascimento 21-05-2022", "certidao "+testBirthCertificateNumber, "naturalidade Santa Ines", "Eles conferem? Posso prosseguir e criar a reserva?") {
		t.Fatalf("unexpected document confirmation reply: %q", reply)
	}
	assertNoInternalIDs(t, reply)
	assertNoEmptyTemplateArtifacts(t, reply)
}

func TestConfirmExtractedDocumentShowsAdditionalDocuments(t *testing.T) {
	tests := []struct {
		name      string
		passenger DocumentExtractPassenger
		want      []string
		notWant   []string
	}{
		{
			name: "cnh with additional cpf",
			passenger: DocumentExtractPassenger{
				Name:         "Passageiro Teste",
				DocumentType: "CNH",
				Document:     "99999999999",
				CNH:          "99999999999",
				CPF:          syntheticValidCPFForTests(),
			},
			want:    []string{"Passageiro Teste | CNH | 99999999999", "CPF 849.***.***-86"},
			notWant: []string{"CNH 99999999999 | CNH 99999999999"},
		},
		{
			name: "birth certificate with additional cpf",
			passenger: DocumentExtractPassenger{
				Name:                   "Passageiro Teste",
				DocumentType:           "CERTIDAO_NASCIMENTO",
				Document:               testBirthCertificateNumber,
				BirthCertificateNumber: testBirthCertificateNumber,
				CPF:                    syntheticValidCPFForTests(),
			},
			want:    []string{"Passageiro Teste | CERTIDAO_NASCIMENTO | " + testBirthCertificateNumber, "CPF 849.***.***-86"},
			notWant: []string{"certidao " + testBirthCertificateNumber},
		},
		{
			name: "rg with additional cpf",
			passenger: DocumentExtractPassenger{
				Name:         "Passageiro Teste",
				DocumentType: "RG",
				Document:     "2817314",
				RG:           "2817314",
				CPF:          syntheticValidCPFForTests(),
			},
			want:    []string{"Passageiro Teste | RG | 2817314", "CPF 849.***.***-86"},
			notWant: []string{"RG 2817314 | RG 2817314"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reply := buildConfirmExtractedDocumentReply(DocumentExtractResult{
				ExpectedPassengerCount: 1,
				Passengers:             []DocumentExtractPassenger{tc.passenger},
			})
			if !containsAll(reply, tc.want...) {
				t.Fatalf("expected additional documents in confirmation, got %q", reply)
			}
			for _, part := range tc.notWant {
				if strings.Contains(reply, part) {
					t.Fatalf("did not expect duplicate field %q in %q", part, reply)
				}
			}
		})
	}
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

func TestReservationStartTemplateDoesNotTriggerOperationalClaimWithoutTool(t *testing.T) {
	if containsOperationalAutoSendClaimWithoutTool(askReservationRouteSCReply) {
		t.Fatalf("reservation start template must not look like operational availability claim: %q", askReservationRouteSCReply)
	}

	policy := evaluateDraftAutoSendPolicy(
		[]Message{{Direction: "INBOUND", Body: "como faço uma reserva?"}},
		nil,
		askReservationRouteSCReply,
	)

	if policy.Status != draftAutoSendStatusEligible {
		t.Fatalf("expected reservation start template to be auto-send eligible, got %s reasons=%v", policy.Status, policy.Reasons)
	}
}

func TestPaymentOptionsInfoTemplateDoesNotTriggerOperationalAutoSendBlock(t *testing.T) {
	if containsOperationalAutoSendClaimWithoutTool(paymentOptionsInfoReply) {
		t.Fatalf("payment options info template must not look like dynamic operational claim: %q", paymentOptionsInfoReply)
	}

	policy := evaluateDraftAutoSendPolicy(nil, nil, paymentOptionsInfoReply)
	if policy.Status != draftAutoSendStatusEligible {
		t.Fatalf("expected payment options info template to be auto-send eligible, got %s reasons=%v", policy.Status, policy.Reasons)
	}
	if len(policy.Reasons) != 0 {
		t.Fatalf("expected no auto-send reasons for payment options info template, got %+v", policy.Reasons)
	}
}

func TestAutoSendPolicyStillBlocksDynamicOperationalClaimsWithoutTool(t *testing.T) {
	for _, text := range []string{
		"Encontrei estas opções: 1. Videira/SC para Santa Inês/MA, 2026-07-13, saída 13:00, R$ 950",
		"Tem disponibilidade para 13/07 às 13:00 por R$ 950.",
		"A saída é às 13:00 e custa R$ 950.",
	} {
		t.Run(text, func(t *testing.T) {
			if !containsOperationalAutoSendClaimWithoutTool(text) {
				t.Fatalf("expected dynamic operational text to be blocked: %q", text)
			}
			policy := evaluateDraftAutoSendPolicy(nil, nil, text)
			if policy.Status != draftAutoSendStatusReviewNeeded {
				t.Fatalf("expected review for dynamic operational text, got %+v", policy)
			}
			if !containsString(policy.Reasons, draftAutoSendReasonOperationalClaimWithoutTool) {
				t.Fatalf("expected reason %s, got %+v", draftAutoSendReasonOperationalClaimWithoutTool, policy.Reasons)
			}
		})
	}
}
