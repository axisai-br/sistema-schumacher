package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"schumacher-tur/api/internal/bookings"
	"schumacher-tur/api/internal/payments"
	"schumacher-tur/api/internal/shared/config"
)

const testBirthCertificateNumber = "12345678901234567890123456789012"

func TestParseBookingCreateInputBlocksLapChildWithoutAssignment(t *testing.T) {
	now := time.Now().UTC()
	session := Session{
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Messias",
	}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(2, 1))
	history := markAvailabilityPromptHistoryDeliveredForTest([]Message{
		{
			Direction:        "OUTBOUND",
			Body:             "Achei estas opcoes para Petrolandia/SC.",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-6 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(AvailabilitySearchResult{
						Filter: AvailabilitySearchInput{
							Destination: "Petrolandia/SC",
							Qty:         1,
							Limit:       5,
						},
						Results: []AvailabilitySearchItem{
							{
								TripID:                 "trip-1",
								BoardStopID:            "board-1",
								AlightStopID:           "alight-1",
								OriginDisplayName:      "Moncao/MA",
								DestinationDisplayName: "Petrolandia/SC",
								OriginDepartTime:       "09:00",
								TripDate:               "2026-04-26",
							},
							{
								TripID:                 "trip-2",
								BoardStopID:            "board-2",
								AlightStopID:           "alight-2",
								OriginDisplayName:      "Igarape do Meio/MA",
								DestinationDisplayName: "Petrolandia/SC",
								OriginDepartTime:       "11:00",
								TripDate:               "2026-05-11",
							},
							{
								TripID:                 "trip-3",
								BoardStopID:            "board-3",
								AlightStopID:           "alight-3",
								OriginDisplayName:      "Santa Ines/MA",
								DestinationDisplayName: "Petrolandia/SC",
								OriginDepartTime:       "12:00",
								TripDate:               "2026-05-25",
							},
						},
					}),
				},
			},
		},
		{
			Direction:        "INBOUND",
			Body:             "a primeira",
			ProcessingStatus: "PROCESSED",
			ReceivedAt:       now.Add(-5 * time.Minute),
		},
		{
			Direction:        "OUTBOUND",
			Body:             "Perfeito - confirmei a primeira opcao: 26/04/2026, saida de Moncao as 09:00 para Petrolandia/SC. A passagem e so para voce ou tem mais pessoas, e ha crianca de ate 5 anos viajando?",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-4 * time.Minute),
		},
		{
			Direction:        "INBOUND",
			Body:             "sim eu e minha filha",
			ProcessingStatus: "PROCESSED",
			ReceivedAt:       now.Add(-3 * time.Minute),
		},
		{
			Direction:        "OUTBOUND",
			Body:             "Perfeito - voce e sua filha. A sua filha tem ate 5 anos?",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-150 * time.Second),
		},
		{
			Direction:        "INBOUND",
			Body:             "sim",
			ProcessingStatus: "PROCESSED",
			ReceivedAt:       now.Add(-120 * time.Second),
		},
		{
			Direction:        "OUTBOUND",
			Body:             "Pode enviar os nomes completos e os documentos dos dois.",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-90 * time.Second),
		},
		{
			Direction:        "INBOUND",
			Body:             "Joao Vitor Messias  84960815086\nIvoneide Messias 04822340082",
			ProcessingStatus: "PROCESSED",
			ReceivedAt:       now.Add(-60 * time.Second),
		},
		{
			Direction:        "OUTBOUND",
			Body:             "Perfeito - os numeros enviados sao os CPFs do Joao Vitor e da Ivoneide?",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-30 * time.Second),
		},
	})

	if input, ok := parseBookingCreateInput(session, history, "isso", nil); ok {
		t.Fatalf("expected booking create to be blocked until lap child assignment, got %+v", input)
	}
}

func TestParseBookingCreateInputBlocksSoloChildUntilChildDocumentCollected(t *testing.T) {
	now := time.Now().UTC()
	session := Session{ContactKey: "5549988709047", CustomerPhone: "5549988709047", CustomerName: "Messias"}
	session = sessionWithPassengerClarificationStateForTest(session, soloPlusChildPassengerStateForTest())
	history := soloChildBookingHistory(now)
	history = append(history,
		Message{Direction: "OUTBOUND", Body: "Perfeito. Agora pode enviar os nomes completos e os documentos dos 2 passageiros faltantes (CPF, RG ou CNH completos).", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-90 * time.Second)},
	)

	if input, ok := parseBookingCreateInput(session, history, "quero reservar\nJoao Vitor Messias 84960815086", nil); ok {
		t.Fatalf("expected booking_create to wait for child document, got %+v", input)
	}
}

func TestParseBookingCreateBlocksVerboseSoloChildWithMissingChildDocument(t *testing.T) {
	now := time.Now().UTC()
	session := Session{ContactKey: "5549988709047", CustomerPhone: "5549988709047", CustomerName: "Messias"}
	session = sessionWithPassengerClarificationStateForTest(session, soloPlusChildPassengerStateForTest())
	history := soloChildBookingHistoryWithReply(now, "sim, meu filho de 4 anos")
	history = append(history,
		Message{Direction: "OUTBOUND", Body: "Perfeito. Agora pode enviar os nomes completos e os documentos dos 2 passageiros faltantes (CPF, RG ou CNH completos).", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-90 * time.Second)},
	)

	if input, ok := parseBookingCreateInput(session, history, "quero reservar\nJoao Vitor Messias 84960815086", nil); ok {
		t.Fatalf("expected booking_create to wait for verbose child document, got %+v", input)
	}
}

func TestParseBookingCreateFromDocumentConfirmationBlocksSoloChildWithOneDocument(t *testing.T) {
	now := time.Now().UTC()
	session := Session{ContactKey: "5549988709047", CustomerPhone: "5549988709047", CustomerName: "Messias"}
	session = sessionWithPassengerClarificationStateForTest(session, soloPlusChildPassengerStateForTest())
	history := soloChildBookingHistory(now)
	history = append(history,
		Message{Direction: "OUTBOUND", Body: "Perfeito. Agora pode enviar os nomes completos e os documentos dos 2 passageiros faltantes (CPF, RG ou CNH completos).", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-90 * time.Second)},
		Message{Direction: "INBOUND", Body: "Joao Vitor Messias 84960815086", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-60 * time.Second)},
		Message{Direction: "OUTBOUND", Body: "Consegui identificar estes dados. Eles conferem? Posso prosseguir e criar a reserva?\n1. Joao Vitor Messias | CPF | 849.***.***-86", ProcessingStatus: messageStatusAutomationDraft, ReceivedAt: now.Add(-30 * time.Second)},
	)

	if input, ok := parseBookingCreateFromDocumentConfirmation(session, history, "sim"); ok {
		t.Fatalf("expected booking_create confirmation to wait for child document, got %+v", input)
	}
}

func TestParseBookingCreateFromDocumentConfirmationSoloChildAfterAssignment(t *testing.T) {
	now := time.Now().UTC()
	session := Session{ContactKey: "5549988709047", CustomerPhone: "5549988709047", CustomerName: "Messias"}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(2, 1))
	history := soloChildBookingHistory(now)
	history = append(history,
		Message{Direction: "OUTBOUND", Body: "Perfeito. Agora pode enviar os nomes completos e os documentos dos 2 passageiros faltantes (CPF, RG ou CNH completos).", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-3 * time.Minute)},
		Message{Direction: "INBOUND", Body: "Joao Vitor Messias 84960815086\nIvoneide Messias 04822340082", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-2 * time.Minute)},
		Message{Direction: "OUTBOUND", Body: "Recebi os dados dos 2 passageiros. Qual deles e a crianca de ate 5 anos?\n1. Joao Vitor Messias\n2. Ivoneide Messias", ProcessingStatus: messageStatusAutomationDraft, ReceivedAt: now.Add(-90 * time.Second)},
		Message{Direction: "INBOUND", Body: "2", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-60 * time.Second)},
		Message{Direction: "OUTBOUND", Body: "Consegui identificar estes dados. Eles conferem? Posso prosseguir e criar a reserva?\n1. Joao Vitor Messias | CPF | 849.***.***-86\n2. Ivoneide Messias | CPF | 048.***.***-82 | crianca de ate 5 anos", ProcessingStatus: messageStatusAutomationDraft, ReceivedAt: now.Add(-30 * time.Second)},
	)

	input, ok := parseBookingCreateFromDocumentConfirmation(session, history, "sim")
	if !ok {
		t.Fatalf("expected booking_create after child assignment")
	}
	if input.Qty != 2 || len(input.Passengers) != 2 {
		t.Fatalf("expected two travelers in booking input, got %+v", input)
	}
	if input.Passengers[0].IsLapChild || !input.Passengers[1].IsLapChild {
		t.Fatalf("expected second passenger marked as lap child, got %+v", input.Passengers)
	}
}

func TestParseBookingCreateInputUsesExplicitLapChildLabel(t *testing.T) {
	now := time.Now().UTC()
	session := Session{ContactKey: "5549988709047", CustomerPhone: "5549988709047", CustomerName: "Messias"}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(2, 1))
	history := lapChildBookingHistory(now, "Criança: Joao Vitor Messias 84960815086\nAdulto: Ivoneide Messias 04822340082")

	input, ok := parseBookingCreateFromDocumentConfirmation(session, history, "sim")
	if !ok {
		t.Fatalf("expected booking create with explicit lap child label")
	}
	if len(input.Passengers) != 2 || !input.Passengers[0].IsLapChild || input.Passengers[1].IsLapChild {
		t.Fatalf("expected first passenger as lap child only, got %+v", input.Passengers)
	}
	payload := buildBookingCreateRequestPayload(input)
	rawPassengers := asInterfaceSliceMaps(payload["passengers"])
	if len(rawPassengers) != 2 || rawPassengers[0]["is_lap_child"] != true || rawPassengers[1]["is_lap_child"] == true {
		t.Fatalf("expected request payload to preserve lap child assignment, got %+v", rawPassengers)
	}
}

func TestBookingPassengerFromDocumentExtractPreservesCNHAndVisibleCPF(t *testing.T) {
	session := Session{CustomerPhone: "5549999999999"}
	passenger := bookingPassengerFromDocumentExtract(DocumentExtractPassenger{
		Name:                   "Claudecir Schumacher",
		DocumentType:           "CNH",
		Document:               "99999999999",
		CPF:                    "849.608.150-86",
		CNH:                    "99999999999",
		BirthDate:              "1970-01-02",
		BirthCertificateNumber: testBirthCertificateNumber,
		BirthCity:              "Santa Ines",
		Confidence:             0.92,
	}, session)

	if passenger.DocumentType != "CNH" || passenger.Document != "99999999999" || passenger.CNH != "99999999999" {
		t.Fatalf("expected CNH as primary passenger document, got %+v", passenger)
	}
	if passenger.Phone != "5549999999999" {
		t.Fatalf("expected passenger phone from session, got %+v", passenger)
	}
	if passenger.CPF != "84960815086" || passenger.BirthDate != "1970-01-02" || passenger.BirthCertificateNumber != testBirthCertificateNumber || passenger.BirthCity != "Santa Ines" {
		t.Fatalf("expected additional identity fields preserved, got %+v", passenger)
	}
	if passenger.Notes != "Dados adicionais extraidos: CPF: 84960815086 | CERTIDAO_NASCIMENTO: "+testBirthCertificateNumber+" | DATA_NASCIMENTO: 1970-01-02 | NATURALIDADE: Santa Ines" {
		t.Fatalf("expected additional identity data in notes, got %+v", passenger)
	}
}

func TestBookingPassengerFromDocumentExtractRejectsInvalidCPFOnCNH(t *testing.T) {
	session := Session{CustomerPhone: "5549999999999"}
	passenger := bookingPassengerFromDocumentExtract(DocumentExtractPassenger{
		Name:         "Claudecir Schumacher",
		DocumentType: "CNH",
		Document:     "99999999999",
		CPF:          "123.456.789-01",
		CNH:          "99999999999",
		Confidence:   0.92,
	}, session)

	if passenger.DocumentType != "CNH" || passenger.Document != "99999999999" {
		t.Fatalf("expected invalid CPF to keep CNH as primary document, got %+v", passenger)
	}
	if passenger.Notes != "" {
		t.Fatalf("did not expect CNH duplicated in notes, got %+v", passenger)
	}
}

func TestExtractBookingCreatePassengersParsesTypedAdditionalIdentityFields(t *testing.T) {
	session := Session{CustomerPhone: "5549999999999"}
	passengers := extractBookingCreatePassengers("Nome: Maria Silva CPF 84960815086 nascimento 21/05/2022 matricula "+testBirthCertificateNumber+" naturalidade Santa Ines", session)

	if len(passengers) != 1 {
		t.Fatalf("expected one passenger, got %+v", passengers)
	}
	passenger := passengers[0]
	if passenger.Name != "Maria Silva" || passenger.DocumentType != "CPF" || passenger.Document != "84960815086" || passenger.CPF != "84960815086" {
		t.Fatalf("unexpected primary passenger document: %+v", passenger)
	}
	if passenger.BirthDate != "2022-05-21" || passenger.BirthCertificateNumber != testBirthCertificateNumber || passenger.BirthCity != "Santa Ines" {
		t.Fatalf("expected additional typed identity fields, got %+v", passenger)
	}
}

func TestExtractBookingCreatePassengersParsesStructuredLineWithAdditionalIdentityFields(t *testing.T) {
	session := Session{CustomerPhone: "5549999999999"}
	passengers := extractBookingCreatePassengers("Maria Silva | CPF | 84960815086 | nascimento 21/05/2022 | matricula "+testBirthCertificateNumber+" | naturalidade Santa Ines", session)

	if len(passengers) != 1 {
		t.Fatalf("expected one passenger, got %+v", passengers)
	}
	passenger := passengers[0]
	if passenger.Name != "Maria Silva" || passenger.DocumentType != "CPF" || passenger.Document != "84960815086" || passenger.CPF != "84960815086" {
		t.Fatalf("unexpected primary passenger document: %+v", passenger)
	}
	if passenger.BirthDate != "2022-05-21" || passenger.BirthCertificateNumber != testBirthCertificateNumber || passenger.BirthCity != "Santa Ines" {
		t.Fatalf("expected additional identity fields from structured line, got %+v", passenger)
	}
}

func TestExtractBookingCreatePassengersParsesMultipleTypedAdditionalIdentityFields(t *testing.T) {
	session := Session{CustomerPhone: "5549999999999"}
	text := strings.Join([]string{
		"João Silva CPF 52998224725 nascimento 12/03/1990 RG 1234567",
		"Maria Silva certidão " + testBirthCertificateNumber + " nascimento 10/05/2021 naturalidade Santa Inês",
		"Pedro Silva CNH 12345678900 CPF 52998224725 nascimento 02/01/1988",
	}, "\n")

	passengers := extractBookingCreatePassengers(text, session)
	if len(passengers) != 3 {
		t.Fatalf("expected three passengers, got %+v", passengers)
	}
	if passengers[0].Name != "João Silva" || passengers[0].DocumentType != "CPF" || passengers[0].Document != "52998224725" || passengers[0].CPF != "52998224725" || passengers[0].RG != "1234567" || passengers[0].BirthDate != "1990-03-12" {
		t.Fatalf("unexpected first passenger: %+v", passengers[0])
	}
	if passengers[1].Name != "Maria Silva" || passengers[1].DocumentType != "CERTIDAO_NASCIMENTO" || passengers[1].Document != testBirthCertificateNumber || passengers[1].BirthCertificateNumber != testBirthCertificateNumber || passengers[1].BirthDate != "2021-05-10" || passengers[1].BirthCity != "Santa Inês" {
		t.Fatalf("unexpected second passenger: %+v", passengers[1])
	}
	if passengers[2].Name != "Pedro Silva" || passengers[2].DocumentType != "CNH" || passengers[2].Document != "12345678900" || passengers[2].CNH != "12345678900" || passengers[2].CPF != "52998224725" || passengers[2].BirthDate != "1988-01-02" {
		t.Fatalf("unexpected third passenger: %+v", passengers[2])
	}
}

func TestExtractBookingCreatePassengersParsesLooseNameCPF(t *testing.T) {
	session := Session{CustomerPhone: "5549999999999"}
	passengers := extractBookingCreatePassengers("Nome Sobrenome 84960815086", session)

	if len(passengers) != 1 {
		t.Fatalf("expected one passenger, got %+v", passengers)
	}
	passenger := passengers[0]
	if passenger.Name != "Nome Sobrenome" || passenger.DocumentType != "CPF" || passenger.Document != "84960815086" {
		t.Fatalf("unexpected passenger parsed from loose name CPF: %+v", passenger)
	}
	if passenger.Phone != "5549999999999" {
		t.Fatalf("expected passenger phone from session, got %+v", passenger)
	}
}

func TestExtractBookingCreatePassengersParsesObservedNameCPF(t *testing.T) {
	session := Session{CustomerPhone: "5549999999999"}
	passengers := extractBookingCreatePassengers("Joao Vitor Messias 06645648103", session)

	if len(passengers) != 1 {
		t.Fatalf("expected one passenger, got %+v", passengers)
	}
	passenger := passengers[0]
	if passenger.Name != "Joao Vitor Messias" || passenger.DocumentType != "CPF" || passenger.Document != "06645648103" {
		t.Fatalf("unexpected passenger parsed from observed name CPF: %+v", passenger)
	}
}

func TestExtractBookingCreatePassengersParsesMultipleInlineNameCPF(t *testing.T) {
	session := Session{CustomerPhone: "5549999999999"}
	passengers := extractBookingCreatePassengers("João Vitor Messias 06645648103 Ivoneide Pereira 46643591104", session)

	if len(passengers) != 2 {
		t.Fatalf("expected two passengers, got %+v", passengers)
	}
	if passengers[0].Name != "João Vitor Messias" || passengers[0].DocumentType != "CPF" || passengers[0].Document != "06645648103" || passengers[0].CPF != "06645648103" {
		t.Fatalf("unexpected first passenger: %+v", passengers[0])
	}
	if passengers[1].Name != "Ivoneide Pereira" || passengers[1].DocumentType != "CPF" || passengers[1].Document != "46643591104" || passengers[1].CPF != "46643591104" {
		t.Fatalf("unexpected second passenger: %+v", passengers[1])
	}
}

func TestExtractBookingCreatePassengersStripsInitialInlineConjunction(t *testing.T) {
	session := Session{CustomerPhone: "5549999999999"}
	passengers := extractBookingCreatePassengers("João Vitor Messias 06645648103 e Ivoneide Pereira 46643591104", session)

	if len(passengers) != 2 {
		t.Fatalf("expected two passengers, got %+v", passengers)
	}
	if passengers[0].Name != "João Vitor Messias" {
		t.Fatalf("unexpected first passenger name: %+v", passengers[0])
	}
	if passengers[1].Name != "Ivoneide Pereira" {
		t.Fatalf("expected second passenger without conjunction, got %+v", passengers[1])
	}
}

func TestExtractBookingCreatePassengersStripsInlineSeparatorsBeforeLabels(t *testing.T) {
	session := Session{CustomerPhone: "5549999999999"}
	tests := []struct {
		name           string
		text           string
		expectedFirst  string
		expectedSecond string
	}{
		{
			name:           "comma before ordinal",
			text:           "João Vitor Messias 06645648103, 2. Ivoneide Pereira 46643591104",
			expectedFirst:  "João Vitor Messias",
			expectedSecond: "Ivoneide Pereira",
		},
		{
			name:           "conjunction before ordinal",
			text:           "João Vitor Messias 06645648103 e 2. Ivoneide Pereira 46643591104",
			expectedFirst:  "João Vitor Messias",
			expectedSecond: "Ivoneide Pereira",
		},
		{
			name:           "conjunction before passenger label",
			text:           "João Vitor Messias 06645648103 e passageiro 2: Ivoneide Pereira 46643591104",
			expectedFirst:  "João Vitor Messias",
			expectedSecond: "Ivoneide Pereira",
		},
		{
			name:           "names starting with e",
			text:           "Edivaldo Pereira 06645648103 e Eliane Souza 46643591104",
			expectedFirst:  "Edivaldo Pereira",
			expectedSecond: "Eliane Souza",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			passengers := extractBookingCreatePassengers(tc.text, session)

			if len(passengers) != 2 {
				t.Fatalf("expected two passengers, got %+v", passengers)
			}
			if passengers[0].Name != tc.expectedFirst {
				t.Fatalf("expected first passenger %q, got %+v", tc.expectedFirst, passengers[0])
			}
			if passengers[1].Name != tc.expectedSecond {
				t.Fatalf("expected second passenger %q without inline label, got %+v", tc.expectedSecond, passengers[1])
			}
		})
	}
}

func TestExtractBookingCreatePassengersPreservesNamesStartingWithE(t *testing.T) {
	session := Session{CustomerPhone: "5549999999999"}
	passengers := extractBookingCreatePassengers("Edivaldo Pereira 06645648103 e Eliane Souza 46643591104", session)

	if len(passengers) != 2 {
		t.Fatalf("expected two passengers, got %+v", passengers)
	}
	if passengers[0].Name != "Edivaldo Pereira" {
		t.Fatalf("expected first name starting with E to be preserved, got %+v", passengers[0])
	}
	if passengers[1].Name != "Eliane Souza" {
		t.Fatalf("expected second name starting with E to be preserved, got %+v", passengers[1])
	}
}

func TestExtractBookingCreatePassengersParsesMultipleInlinePunctuatedCPF(t *testing.T) {
	session := Session{CustomerPhone: "5549999999999"}
	passengers := extractBookingCreatePassengers("João Vitor Messias - 066.456.481-03 / Ivoneide Pereira - 466.435.911-04", session)

	if len(passengers) != 2 {
		t.Fatalf("expected two passengers, got %+v", passengers)
	}
	if passengers[0].Name != "João Vitor Messias" || passengers[0].DocumentType != "CPF" || passengers[0].Document != "06645648103" {
		t.Fatalf("unexpected first passenger: %+v", passengers[0])
	}
	if passengers[1].Name != "Ivoneide Pereira" || passengers[1].DocumentType != "CPF" || passengers[1].Document != "46643591104" {
		t.Fatalf("unexpected second passenger: %+v", passengers[1])
	}
}

func TestExtractBookingCreatePassengersParsesMultipleInlineWithCPFLabels(t *testing.T) {
	session := Session{CustomerPhone: "5549999999999"}
	passengers := extractBookingCreatePassengers("João Vitor Messias CPF 06645648103 Ivoneide Pereira CPF 46643591104", session)

	if len(passengers) != 2 {
		t.Fatalf("expected two passengers, got %+v", passengers)
	}
	if passengers[0].Name != "João Vitor Messias" || passengers[0].Document != "06645648103" {
		t.Fatalf("unexpected first passenger: %+v", passengers[0])
	}
	if passengers[1].Name != "Ivoneide Pereira" || passengers[1].Document != "46643591104" {
		t.Fatalf("unexpected second passenger: %+v", passengers[1])
	}
}

func TestExtractBookingCreatePassengersParsesNameAndRG(t *testing.T) {
	session := Session{CustomerPhone: "5549999999999"}
	passengers := extractBookingCreatePassengers("Nome: Maria Silva RG 2817314 SSP SC", session)

	if len(passengers) != 1 {
		t.Fatalf("expected one passenger, got %+v", passengers)
	}
	passenger := passengers[0]
	if passenger.Name != "Maria Silva" || passenger.DocumentType != "RG" || passenger.Document != "2817314" {
		t.Fatalf("unexpected passenger parsed from name and RG: %+v", passenger)
	}
	if passenger.Phone != "5549999999999" {
		t.Fatalf("expected passenger phone from session, got %+v", passenger)
	}
}

func TestExtractBookingCreatePassengersParsesLooseNameRG(t *testing.T) {
	session := Session{CustomerPhone: "5549999999999"}
	passengers := extractBookingCreatePassengers("Nome Sobrenome RG 2873144", session)

	if len(passengers) != 1 {
		t.Fatalf("expected one passenger, got %+v", passengers)
	}
	passenger := passengers[0]
	if passenger.Name != "Nome Sobrenome" || passenger.DocumentType != "RG" || passenger.Document != "2873144" {
		t.Fatalf("unexpected passenger parsed from loose name RG: %+v", passenger)
	}
	if passenger.Phone != "5549999999999" {
		t.Fatalf("expected passenger phone from session, got %+v", passenger)
	}
}

func TestExtractBookingCreatePassengersRejectsInvalidLooseCPF(t *testing.T) {
	session := Session{CustomerPhone: "5549999999999"}
	passengers := extractBookingCreatePassengers("Nome Sobrenome 12345678901", session)

	if len(passengers) != 0 {
		t.Fatalf("expected invalid CPF to be rejected, got %+v", passengers)
	}
	if !looksLikeInvalidPassengerCPF("Nome Sobrenome 12345678901") {
		t.Fatalf("expected invalid loose CPF to be detected for correction prompt")
	}
}

func TestExtractBookingPassengerDocumentRejectsInvalidCPF(t *testing.T) {
	document, documentType := extractBookingPassengerDocument("CPF 123.456.789-01")

	if document != "" || documentType != "" {
		t.Fatalf("expected invalid CPF to be ignored, got document=%q type=%q", document, documentType)
	}
}

func TestNormalizePassengerDocumentValueCleansRGIssuer(t *testing.T) {
	cases := []struct {
		value    string
		expected string
	}{
		{value: "2817314 SSP SC", expected: "2817314"},
		{value: "2817314 SSP/SC", expected: "2817314"},
		{value: "2817314SSPSC", expected: "2817314"},
		{value: "12.345.678-9 SSP/SC", expected: "123456789"},
		{value: "SSP/SC", expected: ""},
	}

	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			if document := normalizePassengerDocumentValue(tc.value, "RG"); document != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, document)
			}
		})
	}
}

func TestNormalizePassengerDocumentValueRequires32DigitBirthCertificate(t *testing.T) {
	if document := normalizePassengerDocumentValue(testBirthCertificateNumber, "CERTIDAO_NASCIMENTO"); document != testBirthCertificateNumber {
		t.Fatalf("expected 32 digit birth certificate number, got %q", document)
	}
	for _, value := range []string{
		"1234567890123456789012345678901",
		"123456789012345678901234567890123",
		"M1234567890123456789012345678901",
	} {
		if document := normalizePassengerDocumentValue(value, "CERTIDAO_NASCIMENTO"); document != "" {
			t.Fatalf("expected invalid birth certificate number %q to be rejected, got %q", value, document)
		}
	}
}

func TestParseBookingCreateFromDocumentConfirmationUsesLapChildAssignmentReplyByIndex(t *testing.T) {
	now := time.Now().UTC()
	session := Session{ContactKey: "5549988709047", CustomerPhone: "5549988709047", CustomerName: "Messias"}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(2, 1))
	history := lapChildBookingHistory(now, "Joao Vitor Messias 84960815086\nIvoneide Messias 04822340082")
	history = history[:len(history)-1]
	history = append(history, Message{
		Direction:        "OUTBOUND",
		Body:             "Recebi os dados dos 2 passageiros. Qual deles e a crianca de ate 5 anos?\n1. Joao Vitor Messias\n2. Ivoneide Messias",
		ProcessingStatus: messageStatusAutomationDraft,
		ReceivedAt:       now.Add(-30 * time.Second),
	}, Message{
		Direction:        "INBOUND",
		Body:             "1",
		ProcessingStatus: "PROCESSED",
		ReceivedAt:       now.Add(-20 * time.Second),
	}, Message{
		Direction:        "OUTBOUND",
		Body:             "Consegui identificar estes dados. Eles conferem? Posso prosseguir e criar a reserva?\n1. Joao Vitor Messias | CPF | 849.***.***-86 | crianca de ate 5 anos\n2. Ivoneide Messias | CPF | 048.***.***-82",
		ProcessingStatus: messageStatusAutomationDraft,
		ReceivedAt:       now.Add(-10 * time.Second),
	})

	input, ok := parseBookingCreateFromDocumentConfirmation(session, history, "sim")
	if !ok {
		t.Fatalf("expected booking create after document confirmation")
	}
	if len(input.Passengers) != 2 || !input.Passengers[0].IsLapChild || input.Passengers[1].IsLapChild {
		t.Fatalf("expected first passenger as lap child only, got %+v", input.Passengers)
	}
}

func TestFindLatestSelectedOptionIndexRejectsRawHistoricalReplies(t *testing.T) {
	now := time.Now().UTC()
	history := []Message{
		{Direction: "OUTBOUND", Body: "Achei duas opcoes para Santa Ines/MA -> Fraiburgo/SC.", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-6 * time.Minute)},
		{Direction: "INBOUND", Body: "primeira opcao", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-5 * time.Minute)},
		{
			Direction:        "OUTBOUND",
			Body:             "Recebi os dados dos 2 passageiros. Qual deles e a crianca de ate 5 anos?\n1. Joao Vitor Messias\n2. Ivoneide Pereira",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-2 * time.Minute),
			Payload: map[string]interface{}{
				"intent":                string(IntentLapChildAssignmentAnswer),
				"selected_option_index": 2,
			},
			NormalizedPayload: map[string]interface{}{
				"intent":                string(IntentLapChildAssignmentAnswer),
				"selected_option_index": 2,
			},
		},
		{Direction: "INBOUND", Body: "2", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-1 * time.Minute)},
	}

	if got := findLatestSelectedOptionIndex(history); got != 0 {
		t.Fatalf("raw historical trip/passenger indexes must not become durable authority, got %d", got)
	}
}

func TestFindLatestSelectedOptionIndexUsesPersistedAvailabilitySelection(t *testing.T) {
	now := time.Now().UTC()
	availability := AvailabilitySearchResult{
		Results: []AvailabilitySearchItem{
			{
				TripID:       "trip-2026-07-13",
				BoardStopID:  "board-2026-07-13",
				AlightStopID: "alight-2026-07-13",
				TripDate:     "2026-07-13",
			},
			{
				TripID:       "trip-2026-07-14",
				BoardStopID:  "board-2026-07-14",
				AlightStopID: "alight-2026-07-14",
				TripDate:     "2026-07-14",
			},
		},
	}
	selectionPayload := persistedAvailabilitySelectionPayloadForTest(
		availability,
		2,
		availabilitySelectionProjectionAuthorityForTest{
			SelectionMessageID:    "persisted-selection-event-2",
			PromptSourceMessageID: "persisted-availability-prompt-2",
			MaterializesAuthority: true,
		},
	)
	history := markAvailabilityPromptHistoryDeliveredForTest([]Message{
		availabilityAuthorityPromptMessageForTest(
			"persisted-availability-prompt-2",
			availability,
			now.Add(-4*time.Minute),
		),
		availabilityAuthoritySelectionInboundForTest(
			"persisted-selection-event-2",
			now.Add(-3*time.Minute),
		),
		{
			ID:                "persisted-selection-projection-2",
			Direction:         "OUTBOUND",
			Body:              askPassengerCountReply,
			ProcessingStatus:  messageStatusAutomationSent,
			ReceivedAt:        now.Add(-2 * time.Minute),
			CreatedAt:         now.Add(-2 * time.Minute),
			Payload:           cloneMap(selectionPayload),
			NormalizedPayload: cloneMap(selectionPayload),
		},
		{Direction: "INBOUND", Body: "1", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-1 * time.Minute)},
	})

	if got := findLatestSelectedOptionIndex(history); got != 2 {
		t.Fatalf("expected persisted trip selection 2 to beat passenger count reply, got %d", got)
	}
}

func TestFindLatestSelectedOptionIndexIgnoresMetadataOnlyAvailabilitySelection(t *testing.T) {
	now := time.Now().UTC()
	history := []Message{
		{
			Direction:        "OUTBOUND",
			Body:             askPassengerCountReply,
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-2 * time.Minute),
			Payload: map[string]interface{}{
				"intent":                string(IntentSelectAvailabilityOption),
				"template_name":         string(TemplateAskPassengerCount),
				"selected_option_index": 2,
			},
			NormalizedPayload: map[string]interface{}{
				"intent":                string(IntentSelectAvailabilityOption),
				"template_name":         string(TemplateAskPassengerCount),
				"selected_option_index": 2,
			},
		},
	}

	if got := findLatestSelectedOptionIndex(history); got != 0 {
		t.Fatalf("expected metadata-only selected_option_index to be ignored, got %d", got)
	}
}

func TestFindLatestSelectedOptionIndexRejectsSameMessageAvailabilityFactsWithoutAuthority(t *testing.T) {
	now := time.Now().UTC()
	history := []Message{
		{
			Direction:        "OUTBOUND",
			Body:             askPassengerCountReply,
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-2 * time.Minute),
			Payload: map[string]interface{}{
				"intent":                string(IntentSelectAvailabilityOption),
				"template_name":         string(TemplateAskPassengerCount),
				"selected_option_index": 2,
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(AvailabilitySearchResult{
						Results: []AvailabilitySearchItem{
							{TripID: "trip-2026-07-13", BoardStopID: "board-2026-07-13", AlightStopID: "alight-2026-07-13"},
							{TripID: "trip-2026-07-14", BoardStopID: "board-2026-07-14", AlightStopID: "alight-2026-07-14"},
						},
					}),
				},
			},
		},
	}

	if got := findLatestSelectedOptionIndex(history); got != 0 {
		t.Fatalf("same-message availability facts without explicit authority materialized index=%d", got)
	}
}

func TestFindLatestSelectedOptionIndexRejectsIncompleteSameMessageAvailabilityFacts(t *testing.T) {
	now := time.Now().UTC()
	history := []Message{
		{
			Direction:        "OUTBOUND",
			Body:             askPassengerCountReply,
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-2 * time.Minute),
			Payload: map[string]interface{}{
				"intent":                string(IntentSelectAvailabilityOption),
				"template_name":         string(TemplateAskPassengerCount),
				"selected_option_index": 2,
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(AvailabilitySearchResult{
						Results: []AvailabilitySearchItem{
							{TripID: "trip-2026-07-13", BoardStopID: "board-2026-07-13", AlightStopID: "alight-2026-07-13"},
							{TripID: "trip-2026-07-14"},
						},
					}),
				},
			},
		},
	}

	if got := findLatestSelectedOptionIndex(history); got != 0 {
		t.Fatalf("expected incomplete same-message availability facts not to materialize selection, got %d", got)
	}
	context := collectBookingDraftContext(sessionWithPassengerClarificationStateForTest(Session{}, unknownPassengerStateForTest()), history, "")
	if context.SelectedOptionIndex != 0 ||
		context.TripID != "" ||
		context.BoardStopID != "" ||
		context.AlightStopID != "" {
		t.Fatalf("expected incomplete same-message availability facts not to populate booking draft, got %+v", context)
	}
}

func TestFindLatestSelectedOptionIndexIgnoresPassengerCountReplyWithoutPersistedSelection(t *testing.T) {
	now := time.Now().UTC()
	history := []Message{
		{
			Direction:        "OUTBOUND",
			Body:             askPassengerCountReply,
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-2 * time.Minute),
		},
		{
			Direction:        "INBOUND",
			Body:             "1",
			ProcessingStatus: "PROCESSED",
			ReceivedAt:       now.Add(-1 * time.Minute),
		},
	}

	if got := findLatestSelectedOptionIndex(history); got != 0 {
		t.Fatalf("expected passenger count reply not to become selected option index, got %d", got)
	}
}

func TestCollectBookingDraftContextPrefersSelectedAvailabilitySnapshot(t *testing.T) {
	now := time.Now().UTC()
	selectedAvailability := AvailabilitySearchResult{
		Filter: AvailabilitySearchInput{
			Origin:      "Videira/SC",
			Destination: "Santa Ines/MA",
			Qty:         1,
			Limit:       2,
		},
		Results: []AvailabilitySearchItem{
			{
				TripID:                 "trip-visible-1",
				BoardStopID:            "board-visible-1",
				AlightStopID:           "alight-visible-1",
				OriginDisplayName:      "Videira/SC",
				DestinationDisplayName: "Santa Ines/MA",
				OriginDepartTime:       "13:00",
				TripDate:               "2026-07-13",
				Price:                  900,
				Currency:               "BRL",
			},
			{
				TripID:                 "trip-selected-2",
				BoardStopID:            "board-selected-2",
				AlightStopID:           "alight-selected-2",
				OriginDisplayName:      "Videira/SC",
				DestinationDisplayName: "Santa Ines/MA",
				OriginDepartTime:       "14:00",
				TripDate:               "2026-07-14",
				Price:                  950,
				Currency:               "BRL",
			},
		},
	}
	history := []Message{
		{
			Direction:        "OUTBOUND",
			Body:             buildAvailabilityListReply(selectedAvailability),
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-5 * time.Minute),
		},
		{Direction: "INBOUND", Body: "opcao 2", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-4 * time.Minute)},
		{Direction: "OUTBOUND", Body: askPassengerCountReply, ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-3 * time.Minute)},
		{
			Direction:        "OUTBOUND",
			Body:             "Encontrei uma nova lista ainda sem seleção.",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-2 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(AvailabilitySearchResult{
						Filter:  selectedAvailability.Filter,
						Results: selectedAvailability.Results[:1],
					}),
				},
			},
		},
		{Direction: "INBOUND", Body: "só pra mim", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-1 * time.Minute)},
	}
	history = canonicalAvailabilitySelectionHistoryForTest(history, selectedAvailability, 2, 0, 1, 2)

	context := collectBookingDraftContext(sessionWithPassengerClarificationStateForTest(Session{}, passengerOnlyStateForTest(1, PassengerCountProvenanceSoloSpeaker)), history, "")
	if context.SelectedOptionIndex != 2 {
		t.Fatalf("expected selected option index 2, got %+v", context)
	}
	if context.TripID != "trip-selected-2" ||
		context.BoardStopID != "board-selected-2" ||
		context.AlightStopID != "alight-selected-2" ||
		context.TripDate != "2026-07-14" {
		t.Fatalf("expected booking draft to prefer selected availability snapshot, got %+v", context)
	}
	if context.PassengerCount != 1 || !context.PassengerCountKnown {
		t.Fatalf("expected passenger reply to still be collected, got %+v", context)
	}
}

func TestCollectBookingDraftContextKeepsLatestSelectedAvailabilitySnapshotIndex(t *testing.T) {
	now := time.Now().UTC()
	availabilityFor := func(suffix string, price float64) AvailabilitySearchResult {
		return AvailabilitySearchResult{
			Filter: AvailabilitySearchInput{
				Origin: "Videira/SC", Destination: "Santa Ines/MA", Qty: 1, Limit: 1,
			},
			Results: []AvailabilitySearchItem{{
				TripID:                 "trip-2026-07-" + suffix,
				BoardStopID:            "board-2026-07-" + suffix,
				AlightStopID:           "alight-2026-07-" + suffix,
				OriginDisplayName:      "Videira/SC",
				DestinationDisplayName: "Santa Ines/MA",
				OriginDepartTime:       suffix + ":00",
				TripDate:               "2026-07-" + suffix,
				Price:                  price,
				Currency:               "BRL",
			}},
		}
	}
	firstAvailability := availabilityFor("13", 950)
	secondAvailability := availabilityFor("14", 980)
	secondTarget := secondAvailability.Results[0]
	secondAvailability.Filter.Limit = 2
	secondAvailability.Results = []AvailabilitySearchItem{
		{
			TripID:                 "trip-2026-07-13-replacement-list",
			BoardStopID:            "board-2026-07-13-replacement-list",
			AlightStopID:           "alight-2026-07-13-replacement-list",
			OriginDisplayName:      "Videira/SC",
			DestinationDisplayName: "Santa Ines/MA",
			OriginDepartTime:       "13:30",
			TripDate:               "2026-07-13",
			Price:                  960,
			Currency:               "BRL",
		},
		secondTarget,
	}
	first := canonicalAvailabilitySelectionHistoryForTest(
		[]Message{
			{Direction: "OUTBOUND", ReceivedAt: now.Add(-7 * time.Minute)},
			{Direction: "INBOUND", ReceivedAt: now.Add(-6 * time.Minute)},
			{Direction: "OUTBOUND", Body: askPassengerCountReply, ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-5 * time.Minute)},
		},
		firstAvailability,
		1,
		0,
		1,
		2,
	)
	second := canonicalAvailabilitySelectionHistoryForTest(
		[]Message{
			{Direction: "OUTBOUND", ReceivedAt: now.Add(-4 * time.Minute)},
			{Direction: "INBOUND", ReceivedAt: now.Add(-3 * time.Minute)},
			{Direction: "OUTBOUND", Body: askPassengerCountReply, ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-2 * time.Minute)},
		},
		secondAvailability,
		2,
		0,
		1,
		2,
	)
	history := append(first, second...)
	history = append(history, Message{
		Direction: "INBOUND", Body: "só pra mim", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-time.Minute),
	})

	context := collectBookingDraftContext(sessionWithPassengerClarificationStateForTest(Session{}, passengerOnlyStateForTest(1, PassengerCountProvenanceSoloSpeaker)), history, "")
	if context.SelectedOptionIndex != 2 {
		t.Fatalf("expected latest selected option index 2, got %+v", context)
	}
	if context.TripID != "trip-2026-07-14" ||
		context.BoardStopID != "board-2026-07-14" ||
		context.AlightStopID != "alight-2026-07-14" ||
		context.TripDate != "2026-07-14" ||
		context.DepartureTime != "14:00" ||
		context.Price != 980 {
		t.Fatalf("expected latest selected trip snapshot to stay intact, got %+v", context)
	}
	if context.PassengerCount != 1 || !context.PassengerCountKnown {
		t.Fatalf("expected passenger reply to still be collected, got %+v", context)
	}
}

func TestCollectBookingDraftContextDoesNotCombineMetadataOnlySelectionWithOldSnapshot(t *testing.T) {
	now := time.Now().UTC()
	history := []Message{
		{
			Direction:        "OUTBOUND",
			Body:             askPassengerCountReply,
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-4 * time.Minute),
			Payload: map[string]interface{}{
				"intent":                string(IntentSelectAvailabilityOption),
				"template_name":         string(TemplateAskPassengerCount),
				"selected_option_index": 1,
				selectedAvailabilityResultPayloadKey: map[string]interface{}{
					"selected_option_index":    1,
					"trip_id":                  "trip-2026-07-13",
					"board_stop_id":            "board-2026-07-13",
					"alight_stop_id":           "alight-2026-07-13",
					"origin":                   "Videira/SC",
					"destination":              "Santa Ines/MA",
					"origin_display_name":      "Videira/SC",
					"destination_display_name": "Santa Ines/MA",
					"origin_depart_time":       "13:00",
					"trip_date":                "2026-07-13",
					"price":                    950,
					"currency":                 "BRL",
				},
			},
		},
		{
			Direction:        "OUTBOUND",
			Body:             askPassengerCountReply,
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-2 * time.Minute),
			Payload: map[string]interface{}{
				"intent":                string(IntentSelectAvailabilityOption),
				"template_name":         string(TemplateAskPassengerCount),
				"selected_option_index": 2,
			},
		},
		{Direction: "INBOUND", Body: "só pra mim", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-1 * time.Minute)},
	}

	context := collectBookingDraftContext(sessionWithPassengerClarificationStateForTest(Session{}, passengerOnlyStateForTest(1, PassengerCountProvenanceSoloSpeaker)), history, "")
	if context.SelectedOptionIndex != 0 {
		t.Fatalf("expected metadata-only selected_option_index not to remain a valid selection, got %+v", context)
	}
	if context.TripID != "" ||
		context.BoardStopID != "" ||
		context.AlightStopID != "" ||
		context.TripDate != "" {
		t.Fatalf("expected old snapshot not to materialize route for metadata-only selection, got %+v", context)
	}
	if context.PassengerCount != 1 || !context.PassengerCountKnown {
		t.Fatalf("expected passenger reply to still be collected, got %+v", context)
	}
}

func TestAvailabilityDraftHasSelectedTripIgnoresMetadataOnlySelection(t *testing.T) {
	now := time.Now().UTC()
	history := []Message{
		{
			Direction:        "OUTBOUND",
			Body:             askPassengerCountReply,
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-2 * time.Minute),
			Payload: map[string]interface{}{
				"intent":                string(IntentSelectAvailabilityOption),
				"template_name":         string(TemplateAskPassengerCount),
				"selected_option_index": 2,
			},
			NormalizedPayload: map[string]interface{}{
				"intent":                string(IntentSelectAvailabilityOption),
				"template_name":         string(TemplateAskPassengerCount),
				"selected_option_index": 2,
			},
		},
	}

	if availabilityDraftHasSelectedTrip(Session{}, history, "") {
		t.Fatalf("expected metadata-only selected_option_index not to count as selected trip")
	}
}

func TestAvailabilityDraftHasSelectedTripRequiresCompleteSelectedTripFacts(t *testing.T) {
	now := time.Now().UTC()
	incompleteHistory := []Message{
		{
			Direction:        "OUTBOUND",
			Body:             askPassengerCountReply,
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-2 * time.Minute),
			Payload: map[string]interface{}{
				"intent":                string(IntentSelectAvailabilityOption),
				"template_name":         string(TemplateAskPassengerCount),
				"selected_option_index": 2,
				selectedAvailabilityResultPayloadKey: map[string]interface{}{
					"selected_option_index": 2,
					"trip_id":               "trip-2026-07-14",
				},
			},
		},
	}
	if availabilityDraftHasSelectedTrip(Session{}, incompleteHistory, "") {
		t.Fatalf("expected trip_id without board/alight stops not to count as selected trip")
	}

	completeAvailability := AvailabilitySearchResult{
		Filter: AvailabilitySearchInput{
			Origin: "Videira/SC", Destination: "Santa Ines/MA", Qty: 1, Limit: 1,
		},
		Results: []AvailabilitySearchItem{{
			TripID:                 "trip-2026-07-14",
			BoardStopID:            "board-2026-07-14",
			AlightStopID:           "alight-2026-07-14",
			OriginDisplayName:      "Videira/SC",
			DestinationDisplayName: "Santa Ines/MA",
			OriginDepartTime:       "14:00",
			TripDate:               "2026-07-14",
			Price:                  950,
			Currency:               "BRL",
		}},
	}
	completeHistory := canonicalAvailabilitySelectionHistoryForTest(
		[]Message{
			{Direction: "OUTBOUND", ReceivedAt: now.Add(-4 * time.Minute)},
			{Direction: "INBOUND", ReceivedAt: now.Add(-3 * time.Minute)},
			{Direction: "OUTBOUND", Body: askPassengerCountReply, ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-2 * time.Minute)},
		},
		completeAvailability,
		1,
		0,
		1,
		2,
	)
	session := materializePersistedAvailabilitySelectionForTest(Session{}, completeHistory)
	if !availabilityDraftHasSelectedTrip(session, completeHistory, "") {
		t.Fatalf("expected complete selected availability snapshot to count as selected trip")
	}
}

func TestAvailabilityDraftHasSelectedTripRejectsBareOptionWithIncompleteAvailabilityFacts(t *testing.T) {
	now := time.Now().UTC()
	history := singleAvailabilitySearchHistory(now, AvailabilitySearchItem{
		TripID:                 "trip-2026-07-13",
		OriginDisplayName:      "Videira/SC",
		DestinationDisplayName: "Santa Ines/MA",
		OriginDepartTime:       "13:00",
		TripDate:               "2026-07-13",
		Price:                  950,
		Currency:               "BRL",
	})

	if availabilityDraftHasSelectedTrip(Session{}, history, "1") {
		t.Fatalf("expected bare option index with incomplete facts not to count as selected trip")
	}
}

func TestAvailabilityDraftHasSelectedTripAcceptsBareOptionWithCompleteAvailabilityFacts(t *testing.T) {
	now := time.Now().UTC()
	history := singleAvailabilitySearchHistory(now, AvailabilitySearchItem{
		TripID:                 "trip-2026-07-13",
		BoardStopID:            "board-2026-07-13",
		AlightStopID:           "alight-2026-07-13",
		OriginDisplayName:      "Videira/SC",
		DestinationDisplayName: "Santa Ines/MA",
		OriginDepartTime:       "13:00",
		TripDate:               "2026-07-13",
		Price:                  950,
		Currency:               "BRL",
	})

	if !availabilityDraftHasSelectedTrip(Session{}, history, "1") {
		t.Fatalf("expected bare option index with complete facts to count as selected trip")
	}
}

func TestCollectBookingDraftContextKeepsFreshAvailabilityEnvelopeAfterOldSelectionBlocker(t *testing.T) {
	now := time.Now().UTC()
	oldAvailability := singleAvailabilitySearchHistory(now, AvailabilitySearchItem{
		TripID:                 "trip-2026-07-13",
		BoardStopID:            "board-2026-07-13",
		AlightStopID:           "alight-2026-07-13",
		OriginDisplayName:      "Videira/SC",
		DestinationDisplayName: "Santa Ines/MA",
		OriginDepartTime:       "13:00",
		TripDate:               "2026-07-13",
	})[0]
	blocker := metadataOnlyAvailabilitySelectionMessage(now.Add(-2*time.Minute), 2)
	freshAvailability := singleAvailabilitySearchHistory(now, AvailabilitySearchItem{
		TripID:                 "trip-2026-07-14",
		BoardStopID:            "board-2026-07-14",
		AlightStopID:           "alight-2026-07-14",
		OriginDisplayName:      "Videira/SC",
		DestinationDisplayName: "Santa Ines/MA",
		OriginDepartTime:       "14:00",
		TripDate:               "2026-07-14",
	})[0]
	history := []Message{oldAvailability, blocker, freshAvailability}

	context := collectBookingDraftContext(sessionWithPassengerClarificationStateForTest(Session{}, unknownPassengerStateForTest()), history, "")
	if !context.HasAvailabilityShown || context.Origin != "Videira/SC" || context.Destination != "Santa Ines/MA" {
		t.Fatalf("expected fresh availability after old blocker to preserve its envelope, got %+v", context)
	}
	if context.HasBookableSelection || context.SelectedOptionIndex != 0 ||
		context.TripID != "" || context.BoardStopID != "" || context.AlightStopID != "" ||
		context.TripDate != "" || context.DepartureTime != "" {
		t.Fatalf("fresh unselected availability must not promote its single item, got %+v", context)
	}
}

func TestCollectBookingDraftContextBlocksOldAvailabilityWhenSelectionBlockerIsNewer(t *testing.T) {
	now := time.Now().UTC()
	oldAvailability := singleAvailabilitySearchHistory(now, AvailabilitySearchItem{
		TripID:                 "trip-2026-07-13",
		BoardStopID:            "board-2026-07-13",
		AlightStopID:           "alight-2026-07-13",
		OriginDisplayName:      "Videira/SC",
		DestinationDisplayName: "Santa Ines/MA",
		OriginDepartTime:       "13:00",
		TripDate:               "2026-07-13",
	})[0]
	blocker := metadataOnlyAvailabilitySelectionMessage(now.Add(-1*time.Minute), 2)
	history := []Message{oldAvailability, blocker}

	context := collectBookingDraftContext(sessionWithPassengerClarificationStateForTest(Session{}, unknownPassengerStateForTest()), history, "")
	if context.TripID != "" ||
		context.BoardStopID != "" ||
		context.AlightStopID != "" {
		t.Fatalf("expected newer blocker to prevent stale availability facts, got %+v", context)
	}
}

func TestAvailabilityDraftHasSelectedTripAllowsFreshAvailabilityAfterOldSelectionBlocker(t *testing.T) {
	now := time.Now().UTC()
	oldAvailability := singleAvailabilitySearchHistory(now, AvailabilitySearchItem{
		TripID:                 "trip-2026-07-13",
		BoardStopID:            "board-2026-07-13",
		AlightStopID:           "alight-2026-07-13",
		OriginDisplayName:      "Videira/SC",
		DestinationDisplayName: "Santa Ines/MA",
		OriginDepartTime:       "13:00",
		TripDate:               "2026-07-13",
	})[0]
	blocker := metadataOnlyAvailabilitySelectionMessage(now.Add(-2*time.Minute), 2)
	freshAvailability := singleAvailabilitySearchHistory(now, AvailabilitySearchItem{
		TripID:                 "trip-2026-07-14",
		BoardStopID:            "board-2026-07-14",
		AlightStopID:           "alight-2026-07-14",
		OriginDisplayName:      "Videira/SC",
		DestinationDisplayName: "Santa Ines/MA",
		OriginDepartTime:       "14:00",
		TripDate:               "2026-07-14",
	})[0]
	history := []Message{oldAvailability, blocker, freshAvailability}

	if !availabilityDraftHasSelectedTrip(Session{}, history, "1") {
		t.Fatalf("expected current index to resolve against fresh availability after old blocker")
	}
}

func TestAvailabilityDraftHasSelectedTripBlocksOldAvailabilityWhenSelectionBlockerIsNewer(t *testing.T) {
	now := time.Now().UTC()
	oldAvailability := singleAvailabilitySearchHistory(now, AvailabilitySearchItem{
		TripID:                 "trip-2026-07-13",
		BoardStopID:            "board-2026-07-13",
		AlightStopID:           "alight-2026-07-13",
		OriginDisplayName:      "Videira/SC",
		DestinationDisplayName: "Santa Ines/MA",
		OriginDepartTime:       "13:00",
		TripDate:               "2026-07-13",
	})[0]
	blocker := metadataOnlyAvailabilitySelectionMessage(now.Add(-1*time.Minute), 2)
	history := []Message{oldAvailability, blocker}

	if availabilityDraftHasSelectedTrip(Session{}, history, "1") {
		t.Fatalf("expected newer blocker to prevent current index from resolving against stale availability")
	}
}

func TestCollectBookingDraftContextRejectsSameMessageAvailabilityWithoutAuthority(t *testing.T) {
	now := time.Now().UTC()
	history := []Message{
		{
			Direction:        "OUTBOUND",
			Body:             askPassengerCountReply,
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-4 * time.Minute),
			Payload: map[string]interface{}{
				"intent":                string(IntentSelectAvailabilityOption),
				"template_name":         string(TemplateAskPassengerCount),
				"selected_option_index": 1,
				selectedAvailabilityResultPayloadKey: map[string]interface{}{
					"selected_option_index":    1,
					"trip_id":                  "trip-2026-07-13",
					"board_stop_id":            "board-2026-07-13",
					"alight_stop_id":           "alight-2026-07-13",
					"origin":                   "Videira/SC",
					"destination":              "Santa Ines/MA",
					"origin_display_name":      "Videira/SC",
					"destination_display_name": "Santa Ines/MA",
					"origin_depart_time":       "13:00",
					"trip_date":                "2026-07-13",
					"price":                    950,
					"currency":                 "BRL",
				},
			},
		},
		{
			Direction:        "OUTBOUND",
			Body:             askPassengerCountReply,
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-2 * time.Minute),
			Payload: map[string]interface{}{
				"intent":                string(IntentSelectAvailabilityOption),
				"template_name":         string(TemplateAskPassengerCount),
				"selected_option_index": 2,
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(AvailabilitySearchResult{
						Filter: AvailabilitySearchInput{Origin: "Videira/SC", Destination: "Santa Ines/MA", Qty: 1, Limit: 2},
						Results: []AvailabilitySearchItem{
							{
								TripID:                 "trip-2026-07-13",
								BoardStopID:            "board-2026-07-13",
								AlightStopID:           "alight-2026-07-13",
								OriginDisplayName:      "Videira/SC",
								DestinationDisplayName: "Santa Ines/MA",
								OriginDepartTime:       "13:00",
								TripDate:               "2026-07-13",
								Price:                  950,
								Currency:               "BRL",
							},
							{
								TripID:                 "trip-2026-07-14",
								BoardStopID:            "board-2026-07-14",
								AlightStopID:           "alight-2026-07-14",
								OriginDisplayName:      "Videira/SC",
								DestinationDisplayName: "Santa Ines/MA",
								OriginDepartTime:       "14:00",
								TripDate:               "2026-07-14",
								Price:                  980,
								Currency:               "BRL",
							},
						},
					}),
				},
			},
		},
		{Direction: "INBOUND", Body: "só pra mim", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-1 * time.Minute)},
	}

	context := collectBookingDraftContext(sessionWithPassengerClarificationStateForTest(Session{}, unknownPassengerStateForTest()), history, "")
	if context.HasBookableSelection || context.SelectedOptionIndex != 0 ||
		context.TripID != "" || context.BoardStopID != "" ||
		context.AlightStopID != "" || context.TripDate != "" {
		t.Fatalf("same-message availability without authority leaked selected facts: %+v", context)
	}
}

func TestParseBookingCreateInputRejectsSingleOptionWithoutBookableSelection(t *testing.T) {
	now := time.Now().UTC()
	session := Session{
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Joao Vitor Messias",
	}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(1, 0))
	history := singleOptionBookingCreateAvailabilityHistory(now)

	if input, ok := parseBookingCreateInput(session, history, "quero reservar Joao Vitor Messias 84960815086", nil); ok {
		t.Fatalf("single visible option without a bookable selection must not authorize booking_create: %+v", input)
	}
}

func TestParseBookingCreateInputUsesPersistedBookableSelection(t *testing.T) {
	now := time.Now().UTC()
	session := Session{
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Joao Vitor Messias",
	}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(1, 0))
	availability := bookingCreateSelectionAvailabilityResult("trip-bookable", "board-bookable", "alight-bookable")
	history := []Message{
		{
			Direction:        "OUTBOUND",
			Body:             buildAvailabilityListReply(availability),
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-3 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availability),
				},
			},
		},
		{
			Direction:        "INBOUND",
			Body:             "so pra mim, sem crianca",
			ProcessingStatus: "PROCESSED",
			ReceivedAt:       now.Add(-2 * time.Minute),
		},
	}
	history = appendCanonicalAvailabilitySelectionForPromptForTest(
		history,
		availability,
		1,
		0,
		now.Add(-150*time.Second),
	)

	input, ok := parseBookingCreateInput(session, history, "quero reservar\nJoao Vitor Messias | CPF | 84960815086", nil)
	if !ok {
		t.Fatal("persisted bookable selection must continue authorizing the normal booking flow")
	}
	if input.SelectedOptionIndex != 1 || input.TripID != "trip-bookable" ||
		input.BoardStopID != "board-bookable" || input.AlightStopID != "alight-bookable" {
		t.Fatalf("booking_create did not use the persisted bookable snapshot: %+v", input)
	}
}

func TestParseBookingCreateInputRejectsFailedHistoricalIndexAfterFreshAvailability(t *testing.T) {
	now := time.Now().UTC()
	failedReply := Message{
		ID:               "failed-selection-boundary",
		Direction:        "INBOUND",
		Body:             "1",
		ProcessingStatus: "PROCESSED",
		ReceivedAt:       now.Add(-3 * time.Minute),
	}
	fresh := bookingCreateSelectionAvailabilityResult("trip-fresh", "board-fresh", "alight-fresh")
	history := []Message{
		{
			ID:               "old-rendered-without-facts",
			Direction:        "OUTBOUND",
			Body:             buildAvailabilityListReply(fresh),
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-4 * time.Minute),
		},
		failedReply,
		{
			ID:               "fresh-complete-availability",
			Direction:        "OUTBOUND",
			Body:             buildAvailabilityListReply(fresh),
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-2 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(fresh),
				},
			},
		},
		{
			Direction:        "INBOUND",
			Body:             "so pra mim, sem crianca",
			ProcessingStatus: "PROCESSED",
			ReceivedAt:       now.Add(-time.Minute),
		},
	}
	session := Session{
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Joao Vitor Messias",
		Metadata:      map[string]interface{}{"agent": map[string]interface{}{}},
	}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(1, 0))
	writeCanonicalAvailabilityFactsInvalidationMetadata(
		asMap(session.Metadata["agent"]),
		true,
		canonicalAvailabilityFactsInvalidationBoundary{
			AfterMessageID: failedReply.ID,
			AfterCreatedAt: canonicalAvailabilityHistoryMessageTime(failedReply),
		},
	)

	if input, ok := parseBookingCreateInput(
		session,
		history,
		"quero reservar\nJoao Vitor Messias | CPF | 84960815086",
		nil,
	); ok {
		t.Fatalf("failed historical index must not combine with fresh availability or authorize booking_create: %+v", input)
	}
}

func TestParseBookingCreateInputExplicitIndexWorksWithoutSelectionBlocker(t *testing.T) {
	now := time.Now().UTC()
	session := Session{
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Joao Vitor Messias",
	}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(1, 0))
	history := singleOptionBookingCreateAvailabilityHistory(now)
	availability, _, ok := latestVisibleAvailabilitySelectionContextWithSource(history)
	if !ok {
		t.Fatal("expected structural availability fixture")
	}
	history = appendCanonicalAvailabilitySelectionForPromptForTest(
		history,
		*availability,
		1,
		0,
		now.Add(-150*time.Second),
	)

	input, ok := parseBookingCreateInput(session, history, "quero reservar opcao 1\nJoao Vitor Messias | CPF | 84960815086", nil)
	if !ok {
		t.Fatalf("expected explicit index without blocker to build booking input")
	}
	if input.SelectedOptionIndex != 1 ||
		input.TripID != "trip-2026-07-13" ||
		input.BoardStopID != "board-2026-07-13" ||
		input.AlightStopID != "alight-2026-07-13" {
		t.Fatalf("expected explicit index to use the visible trip, got %+v", input)
	}
}

func TestParseBookingCreateInputRejectsNegatedAvailabilitySelection(t *testing.T) {
	now := time.Now().UTC()
	session := Session{
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Joao Vitor Messias",
	}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(1, 0))
	history := singleOptionBookingCreateAvailabilityHistory(now)

	for _, text := range []string{
		"não quero opção 1\nquero reservar\nJoao Vitor Messias | CPF | 84960815086",
		"não quero 13/07\nquero reservar\nJoao Vitor Messias | CPF | 84960815086",
		"opção 1 não\nquero reservar\nJoao Vitor Messias | CPF | 84960815086",
		"opção 1 não serve\nquero reservar\nJoao Vitor Messias | CPF | 84960815086",
		"1 não serve\nquero reservar\nJoao Vitor Messias | CPF | 84960815086",
		"1 não dá\nquero reservar\nJoao Vitor Messias | CPF | 84960815086",
		"não serve 1\nquero reservar\nJoao Vitor Messias | CPF | 84960815086",
		"13/07 não serve\nquero reservar\nJoao Vitor Messias | CPF | 84960815086",
		"essa opção não serve\nquero reservar\nJoao Vitor Messias | CPF | 84960815086",
		"esse dia não dá\nquero reservar\nJoao Vitor Messias | CPF | 84960815086",
	} {
		t.Run(text, func(t *testing.T) {
			if input, ok := parseBookingCreateInput(session, history, text, nil); ok {
				t.Fatalf("expected negated availability selection not to build booking input, got %+v", input)
			}
		})
	}
}

func TestParseBookingCreateInputBlocksAvailabilityAfterPriorNegatedOptionReply(t *testing.T) {
	now := time.Now().UTC()
	session := Session{
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Joao Vitor Messias",
	}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(1, 0))

	for _, text := range []string{"opção 1 não serve", "1 não serve"} {
		t.Run(text, func(t *testing.T) {
			history := append(singleOptionBookingCreateAvailabilityHistory(now), Message{
				Direction:  "INBOUND",
				Body:       text,
				ReceivedAt: now.Add(-1 * time.Minute),
			})

			input, ok := parseBookingCreateInput(session, history, "quero reservar\nJoao Vitor Messias | CPF | 84960815086", nil)
			if ok {
				t.Fatalf("expected prior negated availability reply to block stale single-option fallback, got %+v", input)
			}
		})
	}
}

func TestParseBookingCreateInputSpecificRejectedOptionAllowsOtherOption(t *testing.T) {
	now := availabilityTestObservedAt()
	availability := availabilityOptionPromptFiveOptionsFutureResultAt(now)
	optionTwo := availability.Results[1]

	session := Session{
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Joao Vitor Messias",
	}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(1, 0))

	history := append(availabilityOptionPromptHistory(now, availability), Message{
		Direction:  "INBOUND",
		Body:       "não quero opção 1",
		ReceivedAt: now.Add(-30 * time.Second),
	})
	history = appendExplicitSoloPassengerDeclaration(history, now)
	history = appendCanonicalAvailabilitySelectionForPromptForTest(
		history,
		availability,
		2,
		0,
		now.Add(time.Second),
	)

	input, ok := parseBookingCreateInput(
		session,
		history,
		"quero reservar opção 2\nJoao Vitor Messias | CPF | 84960815086",
		nil,
	)
	if !ok {
		t.Fatal("expected option 2 booking input after rejecting option 1")
	}

	if input.SelectedOptionIndex != 2 ||
		input.TripID != optionTwo.TripID ||
		input.BoardStopID != optionTwo.BoardStopID ||
		input.AlightStopID != optionTwo.AlightStopID {
		t.Fatalf("expected option 2 trip after rejecting option 1, got %+v", input)
	}

	if input, ok := parseBookingCreateInput(
		session,
		history,
		"quero reservar opção 1\nJoao Vitor Messias | CPF | 84960815086",
		nil,
	); ok {
		t.Fatalf("expected rejected option 1 to be blocked, got %+v", input)
	}
}

func TestParseBookingCreateInputRejectedHiddenVisibleSelectionUsesVisibleContext(t *testing.T) {
	now := time.Date(2099, 7, 12, 12, 0, 0, 0, time.UTC)
	session := Session{
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Joao Vitor Messias",
	}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(1, 0))
	visibleAvailability := bookingCreateHiddenRawPrefixAvailabilityResult()
	visibleAvailability.Results = append(
		[]AvailabilitySearchItem(nil),
		visibleAvailability.Results[1:]...,
	)

	cases := []struct {
		name                           string
		rejection                      string
		appendInformationalToolContext bool
	}{
		{name: "option index", rejection: "não quero opção 1"},
		{name: "trip date", rejection: "não quero 13/07"},
		{name: "informational reattached raw context", rejection: "não quero opção 1", appendInformationalToolContext: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			history := append(bookingCreateHiddenRawPrefixAvailabilityHistory(now), Message{
				Direction:  "INBOUND",
				Body:       tc.rejection,
				ReceivedAt: now.Add(30 * time.Second),
			})
			if tc.appendInformationalToolContext {
				history = append(history, Message{
					Direction:        "OUTBOUND",
					Body:             "Pode pagar no Pix ou no cartão. Qual opção você prefere para seguir?",
					ProcessingStatus: messageStatusAutomationSent,
					ReceivedAt:       now.Add(time.Minute),
					Payload: map[string]interface{}{
						"tool_context": map[string]interface{}{
							toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(bookingCreateHiddenRawPrefixAvailabilityResult()),
						},
					},
				})
			}
			history = appendExplicitSoloPassengerDeclaration(history, now.Add(2*time.Minute))
			history = appendCanonicalAvailabilitySelectionForPromptForTest(
				history,
				visibleAvailability,
				2,
				0,
				now.Add(3*time.Minute),
			)

			if input, ok := parseBookingCreateInput(session, history, "quero reservar opção 1\nJoao Vitor Messias | CPF | 84960815086", nil); ok {
				t.Fatalf("expected rejected visible option 1 to be blocked, got %+v", input)
			}

			input, ok := parseBookingCreateInput(session, history, "quero reservar opção 2\nJoao Vitor Messias | CPF | 84960815086", nil)
			if !ok {
				t.Fatalf("expected visible option 2 booking input after rejecting visible option 1")
			}
			if input.SelectedOptionIndex != 2 ||
				input.TripID != "trip-visible-2026-07-14" ||
				input.BoardStopID != "board-visible-2026-07-14" ||
				input.AlightStopID != "alight-visible-2026-07-14" {
				t.Fatalf("expected visible option 2 trip, not raw result 2, got %+v", input)
			}
		})
	}
}

func TestParseBookingCreateInputAllowsFreshAvailabilityAfterPriorNegatedOptionReply(t *testing.T) {
	now := time.Now().UTC()
	session := Session{
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Joao Vitor Messias",
	}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(1, 0))
	oldAvailability := singleAvailabilitySearchHistory(now, AvailabilitySearchItem{
		TripID:                 "trip-2026-07-13",
		BoardStopID:            "board-2026-07-13",
		AlightStopID:           "alight-2026-07-13",
		OriginDisplayName:      "Videira/SC",
		DestinationDisplayName: "Santa Ines/MA",
		OriginDepartTime:       "13:00",
		TripDate:               "2026-07-13",
	})[0]
	rejection := Message{
		Direction:  "INBOUND",
		Body:       "opção 1 não serve",
		ReceivedAt: now.Add(-2 * time.Minute),
	}
	freshResult := AvailabilitySearchResult{
		Filter: AvailabilitySearchInput{
			Origin:      "Videira/SC",
			Destination: "Santa Ines/MA",
			Qty:         1,
			Limit:       1,
		},
		Results: []AvailabilitySearchItem{{
			TripID:                 "trip-2026-07-14",
			BoardStopID:            "board-2026-07-14",
			AlightStopID:           "alight-2026-07-14",
			OriginDisplayName:      "Videira/SC",
			DestinationDisplayName: "Santa Ines/MA",
			OriginDepartTime:       "14:00",
			TripDate:               "2026-07-14",
		}},
	}
	freshAvailability := singleAvailabilitySearchHistory(now, freshResult.Results[0])[0]
	freshAvailability.ReceivedAt = now.Add(-time.Minute)
	freshAvailability.CreatedAt = now.Add(-time.Minute)
	history := appendExplicitSoloPassengerDeclaration([]Message{oldAvailability, rejection, freshAvailability}, now)
	history = appendCanonicalAvailabilitySelectionForPromptForTest(
		history,
		freshResult,
		1,
		2,
		now.Add(time.Second),
	)

	input, ok := parseBookingCreateInput(session, history, "quero reservar opção 1\nJoao Vitor Messias | CPF | 84960815086", nil)
	if !ok {
		t.Fatalf("expected fresh availability after prior rejection to build booking input")
	}
	if input.SelectedOptionIndex != 1 ||
		input.TripID != "trip-2026-07-14" ||
		input.BoardStopID != "board-2026-07-14" ||
		input.AlightStopID != "alight-2026-07-14" {
		t.Fatalf("expected fresh availability trip after rejection, got %+v", input)
	}
}

func TestParseBookingCreateInputMetadataOnlySelectionBlocksStaleSingleOptionFallback(t *testing.T) {
	now := time.Now().UTC()
	session := Session{
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Joao Vitor Messias",
	}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(1, 0))
	history := append(singleOptionBookingCreateAvailabilityHistory(now), Message{
		Direction:        "OUTBOUND",
		Body:             askPassengerCountReply,
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       now.Add(-1 * time.Minute),
		Payload: map[string]interface{}{
			"intent":                string(IntentSelectAvailabilityOption),
			"template_name":         string(TemplateAskPassengerCount),
			"selected_option_index": 2,
		},
	})

	if input, ok := parseBookingCreateInput(session, history, "quero reservar Joao Vitor Messias 84960815086", nil); ok {
		t.Fatalf("expected metadata-only selection blocker to prevent stale single-option fallback, got %+v", input)
	}
}

func TestParseBookingCreateInputMetadataOnlySelectionBlocksExplicitStaleAvailabilityIndex(t *testing.T) {
	now := time.Now().UTC()
	session := Session{
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Joao Vitor Messias",
	}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(1, 0))
	history := append(singleOptionBookingCreateAvailabilityHistory(now), Message{
		Direction:        "OUTBOUND",
		Body:             askPassengerCountReply,
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       now.Add(-1 * time.Minute),
		Payload: map[string]interface{}{
			"intent":                string(IntentSelectAvailabilityOption),
			"template_name":         string(TemplateAskPassengerCount),
			"selected_option_index": 2,
		},
	})

	if input, ok := parseBookingCreateInput(session, history, "quero reservar opcao 1\nJoao Vitor Messias | CPF | 84960815086", nil); ok {
		t.Fatalf("expected metadata-only selection blocker to prevent explicit stale availability index, got %+v", input)
	}
}

func TestParseBookingCreateInputIncompleteSelectionBlocksStaleSingleOptionFallback(t *testing.T) {
	now := time.Now().UTC()
	session := Session{
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Joao Vitor Messias",
	}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(1, 0))
	history := append(singleOptionBookingCreateAvailabilityHistory(now), Message{
		Direction:        "OUTBOUND",
		Body:             askPassengerCountReply,
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       now.Add(-1 * time.Minute),
		Payload: map[string]interface{}{
			"intent":                string(IntentSelectAvailabilityOption),
			"template_name":         string(TemplateAskPassengerCount),
			"selected_option_index": 2,
			selectedAvailabilityResultPayloadKey: map[string]interface{}{
				"selected_option_index": 2,
				"trip_id":               "trip-2026-07-14",
			},
		},
	})

	if input, ok := parseBookingCreateInput(session, history, "quero reservar Joao Vitor Messias 84960815086", nil); ok {
		t.Fatalf("expected incomplete selection blocker to prevent stale single-option fallback, got %+v", input)
	}
}

func TestParseBookingCreateInputIncompleteSelectionBlocksExplicitStaleAvailabilityIndex(t *testing.T) {
	now := time.Now().UTC()
	session := Session{
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Joao Vitor Messias",
	}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(1, 0))
	history := append(singleOptionBookingCreateAvailabilityHistory(now), Message{
		Direction:        "OUTBOUND",
		Body:             askPassengerCountReply,
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       now.Add(-1 * time.Minute),
		Payload: map[string]interface{}{
			"intent":                string(IntentSelectAvailabilityOption),
			"template_name":         string(TemplateAskPassengerCount),
			"selected_option_index": 2,
			selectedAvailabilityResultPayloadKey: map[string]interface{}{
				"selected_option_index": 2,
				"trip_id":               "trip-2026-07-14",
			},
		},
	})

	if input, ok := parseBookingCreateInput(session, history, "quero reservar opcao 1\nJoao Vitor Messias | CPF | 84960815086", nil); ok {
		t.Fatalf("expected incomplete selection blocker to prevent explicit stale availability index, got %+v", input)
	}
}

func TestParseBookingCreateInputOldSelectionBlockerAllowsFreshAvailability(t *testing.T) {
	now := time.Now().UTC()
	session := Session{
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Joao Vitor Messias",
	}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(1, 0))
	oldItem := bookingCreateSelectionAvailabilityResult("trip-2026-07-13", "board-2026-07-13", "alight-2026-07-13").Results[0]
	freshResult := bookingCreateSelectionAvailabilityResult("trip-2026-07-14", "board-2026-07-14", "alight-2026-07-14")
	freshItem := freshResult.Results[0]

	for _, tc := range []struct {
		name    string
		payload map[string]interface{}
	}{
		{
			name: "metadata_only",
			payload: map[string]interface{}{
				"intent":                string(IntentSelectAvailabilityOption),
				"template_name":         string(TemplateAskPassengerCount),
				"selected_option_index": 2,
			},
		},
		{
			name: "incomplete",
			payload: map[string]interface{}{
				"intent":                string(IntentSelectAvailabilityOption),
				"template_name":         string(TemplateAskPassengerCount),
				"selected_option_index": 2,
				selectedAvailabilityResultPayloadKey: map[string]interface{}{
					"selected_option_index": 2,
					"trip_id":               "trip-2026-07-14",
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oldAvailability := singleAvailabilitySearchHistory(now, oldItem)[0]
			oldAvailability.ReceivedAt = now.Add(-3 * time.Minute)
			blocker := Message{
				Direction:        "OUTBOUND",
				Body:             askPassengerCountReply,
				ProcessingStatus: messageStatusAutomationSent,
				ReceivedAt:       now.Add(-2 * time.Minute),
				Payload:          tc.payload,
			}
			freshAvailability := singleAvailabilitySearchHistory(now, freshItem)[0]
			freshAvailability.ReceivedAt = now.Add(-1 * time.Minute)
			freshAvailability.CreatedAt = now.Add(-1 * time.Minute)
			history := appendExplicitSoloPassengerDeclaration([]Message{oldAvailability, blocker, freshAvailability}, now)
			history = appendCanonicalAvailabilitySelectionForPromptForTest(
				history,
				freshResult,
				1,
				2,
				now.Add(time.Second),
			)

			input, ok := parseBookingCreateInput(session, history, "quero reservar opcao 1\nJoao Vitor Messias | CPF | 84960815086", nil)
			if !ok {
				t.Fatalf("expected fresh availability after old blocker to build booking input")
			}
			if input.SelectedOptionIndex != 1 ||
				input.TripID != "trip-2026-07-14" ||
				input.BoardStopID != "board-2026-07-14" ||
				input.AlightStopID != "alight-2026-07-14" {
				t.Fatalf("expected fresh availability trip to win over old blocker, got %+v", input)
			}
		})
	}
}

func TestParseBookingCreateInputCurrentAvailabilityIgnoresOldSelectionBlocker(t *testing.T) {
	now := time.Now().UTC()
	session := Session{
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Joao Vitor Messias",
	}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(1, 0))
	history := []Message{{
		Direction:        "OUTBOUND",
		Body:             askPassengerCountReply,
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       now.Add(-2 * time.Minute),
		Payload: map[string]interface{}{
			"intent":                string(IntentSelectAvailabilityOption),
			"template_name":         string(TemplateAskPassengerCount),
			"selected_option_index": 2,
		},
	}}
	history = appendExplicitSoloPassengerDeclaration(history, now.Add(-time.Minute))
	currentAvailability := bookingCreateSelectionAvailabilityResult("trip-current", "board-current", "alight-current")
	history = appendCanonicalAvailabilitySelectionForTest(
		history,
		currentAvailability,
		1,
		now,
	)

	input, ok := parseBookingCreateInput(session, history, "quero reservar opcao 1\nJoao Vitor Messias | CPF | 84960815086", &currentAvailability)
	if !ok {
		t.Fatalf("expected current availability to build booking input despite old blocker")
	}
	if input.SelectedOptionIndex != 1 ||
		input.TripID != "trip-current" ||
		input.BoardStopID != "board-current" ||
		input.AlightStopID != "alight-current" {
		t.Fatalf("expected current availability to win over old blocker, got %+v", input)
	}
}

func bookingCreateHiddenRawPrefixAvailabilityHistory(now time.Time) []Message {
	raw := bookingCreateHiddenRawPrefixAvailabilityResult()
	rendered := raw
	rendered.Results = append([]AvailabilitySearchItem(nil), raw.Results[1:]...)
	return []Message{{
		Direction:        "OUTBOUND",
		Body:             buildAvailabilityListReply(rendered),
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       now,
		Payload: map[string]interface{}{
			"tool_context": map[string]interface{}{
				toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(raw),
			},
		},
	}}
}

func bookingCreateHiddenRawPrefixAvailabilityResult() AvailabilitySearchResult {
	base := AvailabilitySearchItem{
		OriginDisplayName:      "Videira/SC",
		DestinationDisplayName: "Santa Ines/MA",
		SeatsAvailable:         5,
		Price:                  950,
		Currency:               "BRL",
		Status:                 "ACTIVE",
		TripStatus:             "SCHEDULED",
		PackageName:            packageToMaranhao,
	}
	hidden := base
	hidden.TripID = "trip-hidden-2026-07-11"
	hidden.BoardStopID = "board-hidden-2026-07-11"
	hidden.AlightStopID = "alight-hidden-2026-07-11"
	hidden.OriginDepartTime = "11:00"
	hidden.TripDate = "2099-07-11"
	visibleRejected := base
	visibleRejected.TripID = "trip-visible-2026-07-13"
	visibleRejected.BoardStopID = "board-visible-2026-07-13"
	visibleRejected.AlightStopID = "alight-visible-2026-07-13"
	visibleRejected.OriginDepartTime = "13:00"
	visibleRejected.TripDate = "2099-07-13"
	visibleAllowed := base
	visibleAllowed.TripID = "trip-visible-2026-07-14"
	visibleAllowed.BoardStopID = "board-visible-2026-07-14"
	visibleAllowed.AlightStopID = "alight-visible-2026-07-14"
	visibleAllowed.OriginDepartTime = "14:00"
	visibleAllowed.TripDate = "2099-07-14"
	return AvailabilitySearchResult{
		Filter: AvailabilitySearchInput{
			Origin:      "Videira/SC",
			Destination: "Santa Ines/MA",
			PackageName: packageToMaranhao,
			Qty:         1,
			Limit:       5,
		},
		Results: []AvailabilitySearchItem{hidden, visibleRejected, visibleAllowed},
	}
}

func singleOptionBookingCreateAvailabilityHistory(now time.Time) []Message {
	history := singleAvailabilitySearchHistory(now, AvailabilitySearchItem{
		TripID:                 "trip-2026-07-13",
		BoardStopID:            "board-2026-07-13",
		AlightStopID:           "alight-2026-07-13",
		OriginDisplayName:      "Videira/SC",
		DestinationDisplayName: "Santa Ines/MA",
		OriginDepartTime:       "13:00",
		TripDate:               "2026-07-13",
		Price:                  950,
		Currency:               "BRL",
	})
	return append(history, Message{Direction: "INBOUND", Body: "so pra mim, sem crianca", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-2 * time.Minute)})
}

func appendExplicitSoloPassengerDeclaration(history []Message, receivedAt time.Time) []Message {
	return append(history, Message{
		Direction:        "INBOUND",
		Body:             "so pra mim, sem crianca",
		ProcessingStatus: "PROCESSED",
		ReceivedAt:       receivedAt,
	})
}

func singleAvailabilitySearchHistory(now time.Time, item AvailabilitySearchItem) []Message {
	return markAvailabilityPromptHistoryDeliveredForTest([]Message{
		{
			Direction:        "OUTBOUND",
			Body:             "Achei uma opcao para Santa Ines/MA. Qual opcao voce prefere?",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-3 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(AvailabilitySearchResult{
						Filter:  AvailabilitySearchInput{Origin: "Videira/SC", Destination: "Santa Ines/MA", Qty: 1, Limit: 1},
						Results: []AvailabilitySearchItem{item},
					}),
				},
			},
		},
	})
}

func metadataOnlyAvailabilitySelectionMessage(receivedAt time.Time, index int) Message {
	return markAvailabilityPromptDeliveredForTest(Message{
		Direction:        "OUTBOUND",
		Body:             askPassengerCountReply,
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       receivedAt,
		Payload: map[string]interface{}{
			"intent":                string(IntentSelectAvailabilityOption),
			"template_name":         string(TemplateAskPassengerCount),
			"selected_option_index": index,
		},
	})
}

func TestBuildBookingContinuationDraftRunDoesNotPersistMetadataOnlySelectedOptionIndex(t *testing.T) {
	run := buildBookingContinuationDraftRun("Ainda preciso dos dados.", BookingNextAskPassengerClarification, BookingDraftContext{
		SelectedOptionIndex:   2,
		PassengerCount:        1,
		PassengerCountKnown:   true,
		ChildUnder5CountKnown: false,
	})

	if got := asInt(run.RequestPayload["selected_option_index"]); got != 0 {
		t.Fatalf("expected request payload not to persist metadata-only selected_option_index, got %d payload=%+v", got, run.RequestPayload)
	}
	if got := asInt(run.ResponsePayload["selected_option_index"]); got != 0 {
		t.Fatalf("expected response payload not to persist metadata-only selected_option_index, got %d payload=%+v", got, run.ResponsePayload)
	}
	if len(asMap(run.RequestPayload[selectedAvailabilityResultPayloadKey])) != 0 ||
		len(asMap(run.ResponsePayload[selectedAvailabilityResultPayloadKey])) != 0 {
		t.Fatalf("expected no selected availability snapshot without trip facts, request=%+v response=%+v", run.RequestPayload, run.ResponsePayload)
	}
}

func TestParseBookingCreateInputUsesStructuredPassengerConfirmationOnHistory(t *testing.T) {
	now := time.Now().UTC()
	availability := AvailabilitySearchResult{
		Filter: AvailabilitySearchInput{
			Destination: "Ituporanga/SC",
			Qty:         1,
			Limit:       5,
		},
		Results: []AvailabilitySearchItem{
			{
				TripID:                 "trip-1",
				BoardStopID:            "board-1",
				AlightStopID:           "alight-1",
				OriginDisplayName:      "Moncao/MA",
				DestinationDisplayName: "Ituporanga/SC",
				OriginDepartTime:       "09:00",
				TripDate:               "2026-04-26",
			},
		},
	}
	session := Session{
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Messias",
	}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(1, 0))
	history := []Message{
		{
			Direction:        "OUTBOUND",
			Body:             "Achei estas opcoes para Ituporanga/SC.",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-4 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availability),
				},
			},
		},
		{
			Direction:        "INBOUND",
			Body:             "a primeira",
			ProcessingStatus: "PROCESSED",
			ReceivedAt:       now.Add(-3 * time.Minute),
		},
		{
			Direction:        "OUTBOUND",
			Body:             askPassengerCountReply,
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-165 * time.Second),
		},
		{
			Direction:        "INBOUND",
			Body:             "so pra mim, sem crianca",
			ProcessingStatus: "PROCESSED",
			ReceivedAt:       now.Add(-150 * time.Second),
		},
		{
			Direction:        "OUTBOUND",
			Body:             "Consegui identificar estes dados. Eles conferem?\n- Passageiro 1: Joao Vitor Messias | CPF | 84960815086",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-2 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameDocumentExtract: buildDocumentExtractResponsePayload(DocumentExtractResult{
						Mode:                   "EXTRACTED",
						ExpectedPassengerCount: 1,
						Passengers: []DocumentExtractPassenger{{
							Name: "Joao Vitor Messias", DocumentType: "CPF", Document: "84960815086", CPF: "84960815086", Confidence: 1,
						}},
					}),
				},
			},
		},
	}
	history = canonicalAvailabilitySelectionHistoryForTest(history, availability, 1, 0, 1, 2)

	input, ok := parseBookingCreateInput(session, history, "isso", nil)
	if !ok {
		t.Fatalf(
			"expected booking create input from structured extraction history: selected_option=%d availability=%v passenger_text=%q passengers=%+v qty=%d",
			findLatestSelectedOptionIndex(history),
			findLatestAvailabilityContext(history) != nil,
			findLatestPassengerDetailsText(history, session),
			extractBookingCreatePassengers(findLatestPassengerDetailsText(history, session), session),
			extractPassengerQuantity(findLatestPassengerDetailsText(history, session)),
		)
	}
	if len(input.Passengers) != 1 {
		t.Fatalf("expected one passenger, got %+v", input.Passengers)
	}
	if input.Passengers[0].Name != "Joao Vitor Messias" {
		t.Fatalf("unexpected passenger name: %+v", input.Passengers[0])
	}
	if input.Passengers[0].DocumentType != "CPF" || input.Passengers[0].Document != "84960815086" {
		t.Fatalf("unexpected passenger document: %+v", input.Passengers[0])
	}
}

func TestFindLatestAvailabilityContextIgnoresInvisibleAvailabilityFacts(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	visibleResult := bookingCreateSelectionAvailabilityResult("visible-trip", "visible-board", "visible-alight")
	hiddenResult := bookingCreateSelectionAvailabilityResult("hidden-trip", "hidden-board", "hidden-alight")

	for _, status := range []string{
		messageStatusAutomationDraft,
		messageStatusAutomationReviewed,
		messageStatusAutomationPending,
	} {
		t.Run(status, func(t *testing.T) {
			history := bookingCreateAvailabilityHistory(now, visibleResult, hiddenResult, status)

			got := findLatestAvailabilityContext(history)
			if got == nil || len(got.Results) != 1 {
				t.Fatalf("expected visible availability context, got %+v", got)
			}
			if got.Results[0].TripID != "visible-trip" {
				t.Fatalf("expected visible trip facts, got %+v", got.Results[0])
			}
		})
	}
}

func TestFindLatestAvailabilityContextIgnoresBotAutoReplyMirrorFactsWithoutSourceDraft(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	result := bookingCreateSelectionAvailabilityResult("mirror-trip", "mirror-board", "mirror-alight")
	history := []Message{
		{
			ID:               "mirror-list",
			Direction:        "OUTBOUND",
			Body:             buildAvailabilityListReply(result),
			ProcessingStatus: "PENDING",
			ReceivedAt:       now,
			Payload: map[string]interface{}{
				"mode":             "BOT_AUTO_REPLY",
				"draft_message_id": "missing-draft",
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(result),
				},
			},
		},
	}

	if got := findLatestAvailabilityContext(history); got != nil {
		t.Fatalf("BOT_AUTO_REPLY without source draft must not expose availability facts, got %+v", got)
	}
}

func TestParseBookingCreateInputIgnoresInvisibleAvailabilityFactsWhenResolvingSelection(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	visibleResult := bookingCreateSelectionAvailabilityResult("visible-trip", "visible-board", "visible-alight")
	hiddenResult := bookingCreateSelectionAvailabilityResult("hidden-trip", "hidden-board", "hidden-alight")
	session := Session{
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Joao Vitor Messias",
	}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(1, 0))

	for _, status := range []string{
		messageStatusAutomationDraft,
		messageStatusAutomationReviewed,
		messageStatusAutomationPending,
	} {
		t.Run(status, func(t *testing.T) {
			history := append(bookingCreateAvailabilityHistory(now, visibleResult, hiddenResult, status),
				Message{Direction: "INBOUND", Body: "primeira opcao", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-50 * time.Second)},
				Message{Direction: "OUTBOUND", Body: "A passagem e so para voce ou vai mais alguem junto?", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-40 * time.Second)},
				Message{Direction: "INBOUND", Body: "so eu", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-30 * time.Second)},
				Message{Direction: "OUTBOUND", Body: "Perfeito. Agora pode enviar seu nome completo e o documento.", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-20 * time.Second)},
				Message{Direction: "INBOUND", Body: "Joao Vitor Messias 84960815086", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-10 * time.Second)},
				Message{Direction: "OUTBOUND", Body: "Consegui identificar estes dados. Eles conferem? Posso prosseguir e criar a reserva?", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now},
			)
			history = canonicalAvailabilitySelectionHistoryForTest(history, visibleResult, 1, 0, 2, 3)

			input, ok := parseBookingCreateInput(session, history, "isso", nil)
			if !ok {
				t.Fatalf("expected booking create input from visible availability context")
			}
			if input.TripID != "visible-trip" || input.BoardStopID != "visible-board" || input.AlightStopID != "visible-alight" {
				t.Fatalf("expected booking selection to use visible trip facts, got %+v", input)
			}
		})
	}
}

func bookingCreateAvailabilityHistory(now time.Time, visibleResult AvailabilitySearchResult, hiddenResult AvailabilitySearchResult, hiddenStatus string) []Message {
	return markAvailabilityPromptHistoryDeliveredForTest([]Message{
		{
			Direction:        "OUTBOUND",
			Body:             buildAvailabilityListReply(visibleResult),
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-2 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(visibleResult),
				},
			},
		},
		{
			Direction:        "OUTBOUND",
			Body:             buildAvailabilityListReply(hiddenResult),
			ProcessingStatus: hiddenStatus,
			ReceivedAt:       now.Add(-1 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(hiddenResult),
				},
			},
		},
	})
}

func bookingCreateSelectionAvailabilityResult(tripID string, boardStopID string, alightStopID string) AvailabilitySearchResult {
	return AvailabilitySearchResult{
		Filter: AvailabilitySearchInput{
			Origin:      "Santa Ines/MA",
			Destination: "Videira/SC",
			PackageName: packageToSantaCatarina,
			Qty:         1,
			Limit:       5,
		},
		Results: []AvailabilitySearchItem{{
			TripID:                 tripID,
			BoardStopID:            boardStopID,
			AlightStopID:           alightStopID,
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

func TestParseBookingCreateInputRejectsConfirmationWhenPassengerCountStillIncomplete(t *testing.T) {
	now := time.Now().UTC()
	session := Session{
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Messias",
	}
	session = sessionWithPassengerClarificationStateForTest(session, passengerOnlyStateForTest(2, PassengerCountProvenanceIncludesSpeakerComposition))
	history := []Message{
		{
			Direction:        "OUTBOUND",
			Body:             "Achei estas opcoes para Fraiburgo/SC.",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-4 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(AvailabilitySearchResult{
						Filter: AvailabilitySearchInput{
							Destination: "Fraiburgo/SC",
							Qty:         1,
							Limit:       5,
						},
						Results: []AvailabilitySearchItem{
							{
								TripID:                 "trip-1",
								BoardStopID:            "board-1",
								AlightStopID:           "alight-1",
								OriginDisplayName:      "Moncao/MA",
								DestinationDisplayName: "Fraiburgo/SC",
								OriginDepartTime:       "09:00",
								TripDate:               "2026-05-11",
							},
						},
					}),
				},
			},
		},
		{Direction: "INBOUND", Body: "primeira opcao", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-3 * time.Minute)},
		{Direction: "OUTBOUND", Body: "A passagem e so para voce ou ha mais passageiros? Tem crianca de ate 5 anos viajando?", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-2 * time.Minute)},
		{Direction: "INBOUND", Body: "eu e minha filha", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-90 * time.Second)},
		{Direction: "OUTBOUND", Body: "Pode enviar os nomes completos e os documentos dos dois.", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-60 * time.Second)},
		{Direction: "OUTBOUND", Body: "Consegui identificar estes dados. Eles conferem?\n- Passageiro 1: Joao Vitor Messias | CPF | 84960815086", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-30 * time.Second)},
	}

	if input, ok := parseBookingCreateInput(session, history, "isso", nil); ok {
		t.Fatalf("expected incomplete passenger confirmation to block booking create, got %+v", input)
	}
}

func TestParseBookingCreateInputBlocksShortPaymentReplyAfterBookingCreated(t *testing.T) {
	now := time.Now().UTC()
	session := Session{
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Messias",
	}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(1, 0))
	history := []Message{
		{
			Direction:        "OUTBOUND",
			Body:             "Reserva criada com sucesso. Codigo ABC12345.",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-2 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameBookingCreate: buildBookingCreateResponsePayload(BookingCreateResult{
						Mode:            "created",
						BookingID:       "BK-ABC123456",
						ReservationCode: "ABC12345",
						Status:          "PENDING",
						TotalAmount:     950,
						RemainderAmount: 950,
					}),
				},
			},
		},
		{
			Direction:        "OUTBOUND",
			Body:             "Voce prefere pagar o valor integral ou apenas o sinal de R$ 250 por passageiro pagante?",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-1 * time.Minute),
		},
	}

	if input, ok := parseBookingCreateInput(session, history, "pix", nil); ok {
		t.Fatalf("expected payment reply to block booking create, got %+v", input)
	}
}

func TestParseBookingCreateFromDocumentConfirmation(t *testing.T) {
	session := Session{
		ID:            "session-document-confirmation",
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Messias",
	}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(1, 0))
	history := documentConfirmationBookingHistory(time.Now().UTC(), "EXTRACTED", true)
	session = materializePersistedAvailabilitySelectionForTest(session, history)
	context := collectBookingDraftContextFromState(session, history, "conferem")
	if !context.HasBookableSelection {
		t.Fatalf("positive document confirmation fixture must contain durable bookable selection: context=%+v evidence=%+v", context, latestAvailabilitySelectionEvidence(history))
	}

	input, ok := parseBookingCreateFromDocumentConfirmation(session, history, "conferem")
	if !ok {
		t.Fatalf("expected booking create from document confirmation")
	}
	if input.TripID != "trip-doc-1" || input.BoardStopID != "board-doc-1" || input.AlightStopID != "alight-doc-1" {
		t.Fatalf("unexpected trip identifiers: %+v", input)
	}
	if input.OriginDisplayName != "Santa Inês/MA" || input.DestinationDisplayName != "Fraiburgo/SC" {
		t.Fatalf("unexpected route: %+v", input)
	}
	if input.TripDate != "2026-05-11" || input.DepartureTime != "12:00" {
		t.Fatalf("unexpected schedule: %+v", input)
	}
	if input.Qty != 1 || len(input.Passengers) != 1 {
		t.Fatalf("expected one passenger, got %+v", input)
	}
	if input.Passengers[0].Name != "Joao Vitor Messias" {
		t.Fatalf("unexpected passenger name: %+v", input.Passengers[0])
	}
	if input.Passengers[0].DocumentType != "CPF" || input.Passengers[0].Document != "84960815086" {
		t.Fatalf("unexpected passenger document: %+v", input.Passengers[0])
	}
	if input.IdempotencyKey == "" {
		t.Fatalf("expected idempotency key")
	}
}

func TestParseBookingCreateFromDocumentConfirmationRequiresBookableSelection(t *testing.T) {
	session := Session{
		ID:            "session-document-confirmation-without-bookable-selection",
		ContactKey:    "5549988709147",
		CustomerPhone: "5549988709147",
		CustomerName:  "Messias",
	}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(1, 0))
	history := documentConfirmationBookingHistoryWithoutBookableSelection(time.Now().UTC(), "EXTRACTED", true)

	if input, ok := parseBookingCreateFromDocumentConfirmation(session, history, "sim"); ok {
		t.Fatalf("document confirmation without durable bookable selection must not produce BookingCreateInput: %+v", input)
	}
}

func TestDocumentConfirmationWithoutBookableSelectionDoesNotCallBookingCreate(t *testing.T) {
	store := newFakeStore()
	runner := &fakeAgentRunner{enabled: true, result: RunAgentResult{ReplyText: "must not authorize booking", Model: "gpt-test"}}
	creator := &fakeBookingCreator{enabled: true}
	svc := NewService(store, config.Config{ChatDebounceWindowMS: 1500}, runner, creator)

	now := time.Now().UTC()
	contactKey := "document-confirmation-without-bookable-selection"
	seed := sessionWithPassengerClarificationStateForTest(Session{}, completePassengerStateForTest(1, 0))
	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel: "WHATSAPP", ContactKey: contactKey, CustomerPhone: contactKey, CustomerName: "Messias",
		LastMessageAt: &now, LastOutboundAt: &now, Metadata: seed.Metadata,
	})
	if err != nil {
		t.Fatalf("seed document confirmation session: %v", err)
	}
	for index, message := range documentConfirmationBookingHistoryWithoutBookableSelection(now, "EXTRACTED", true) {
		if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
			SessionID:         session.ID,
			Direction:         message.Direction,
			Kind:              "TEXT",
			ProviderMessageID: fmt.Sprintf("document-confirmation-history-%d", index),
			IdempotencyKey:    fmt.Sprintf("document-confirmation-history-%d", index),
			Body:              message.Body,
			Payload:           message.Payload,
			NormalizedPayload: message.NormalizedPayload,
			ProcessingStatus:  message.ProcessingStatus,
			ReceivedAt:        message.ReceivedAt,
		}); err != nil {
			t.Fatalf("seed document confirmation history message %d: %v", index, err)
		}
	}

	ingested, err := svc.Ingest(context.Background(), IngestMessageInput{
		ContactKey: contactKey,
		Message: IngestMessagePayload{
			Direction: "INBOUND", ProviderMessageID: "document-confirmation-without-authority",
			IdempotencyKey: "document-confirmation-without-authority", Body: "sim",
		},
	})
	if err != nil {
		t.Fatalf("ingest document confirmation without authority: %v", err)
	}
	out, err := svc.Reprocess(context.Background(), ReprocessInput{SessionID: ingested.Session.ID})
	if err != nil {
		t.Fatalf("reprocess document confirmation without authority: %v", err)
	}
	if creator.calls != 0 {
		t.Fatalf("document confirmation without durable bookable selection called booking_create %d times: tools=%+v draft=%+v", creator.calls, out.ToolCalls, out.Draft)
	}
	for _, call := range out.ToolCalls {
		if call.ToolName == toolNameBookingCreate {
			t.Fatalf("document confirmation without authority emitted booking_create tool call: %+v", call)
		}
	}
}

func TestParseBookingCreateFromPartialDocumentConfirmationUsesVisibleCPF(t *testing.T) {
	session := Session{
		ID:            "session-partial-document-confirmation",
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Messias",
	}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(1, 0))
	history := partialDocumentConfirmationBookingHistory(time.Now().UTC(), DocumentExtractPassenger{
		Name:         "Joao Vitor Messias",
		DocumentType: "RG",
		Document:     "numero nao identificado",
		CPF:          syntheticValidCPFForTests(),
		Confidence:   0.72,
	})

	input, ok := parseBookingCreateFromDocumentConfirmation(session, history, "ta certo")
	if !ok {
		t.Fatalf("expected booking create from partial document confirmation with visible CPF")
	}
	if len(input.Passengers) != 1 {
		t.Fatalf("expected one passenger, got %+v", input.Passengers)
	}
	passenger := input.Passengers[0]
	if passenger.Name != "Joao Vitor Messias" {
		t.Fatalf("unexpected passenger name: %+v", passenger)
	}
	if passenger.DocumentType != "CPF" || passenger.Document != syntheticValidCPFForTests() || passenger.CPF != syntheticValidCPFForTests() {
		t.Fatalf("expected visible CPF to become primary document, got %+v", passenger)
	}
}

func TestParseBookingCreateFromPartialDocumentConfirmationAcceptsContextPhrases(t *testing.T) {
	session := Session{
		ID:            "session-partial-document-confirmation-phrases",
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Messias",
	}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(1, 0))
	cases := []string{
		"tá certo",
		"esta certo",
		"correto",
		"isso mesmo",
		"confere",
		"pode seguir",
	}

	for _, phrase := range cases {
		t.Run(phrase, func(t *testing.T) {
			history := partialDocumentConfirmationBookingHistory(time.Now().UTC(), DocumentExtractPassenger{
				Name:         "Joao Vitor Messias",
				DocumentType: "RG",
				Document:     "",
				CPF:          syntheticValidCPFForTests(),
				Confidence:   0.72,
			})
			if _, ok := parseBookingCreateFromDocumentConfirmation(session, history, phrase); !ok {
				t.Fatalf("expected %q to confirm partial document with usable CPF", phrase)
			}
		})
	}
}

func TestParseBookingCreateFromDocumentConfirmationRejectsShortConfirmationOutsideDocumentContext(t *testing.T) {
	now := time.Now().UTC()
	session := Session{
		ID:            "session-short-confirmation-outside-document",
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Messias",
	}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(1, 0))
	histories := [][]Message{
		{
			{Direction: "OUTBOUND", Body: "A passagem e so para voce ou vai mais alguem junto?", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-3 * time.Minute)},
		},
		{
			{Direction: "OUTBOUND", Body: "Voce prefere pagar o valor integral ou apenas o sinal de R$ 250?", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-3 * time.Minute)},
		},
		{
			{Direction: "OUTBOUND", Body: "Encontrei estas opcoes. Qual opcao voce prefere?", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-3 * time.Minute)},
		},
	}

	for _, history := range histories {
		if lastAssistantAskedDocumentConfirmation(history) {
			t.Fatalf("test history should not be document confirmation context: %+v", history)
		}
		if input, ok := parseBookingCreateFromDocumentConfirmation(session, history, "ta certo"); ok {
			t.Fatalf("did not expect short confirmation outside document context to create booking, got %+v", input)
		}
	}
}

func TestParseBookingCreateFromPartialDocumentConfirmationRejectsIncompleteDocument(t *testing.T) {
	session := Session{
		ID:            "session-partial-document-incomplete",
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Messias",
	}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(1, 0))
	history := partialDocumentConfirmationBookingHistory(time.Now().UTC(), DocumentExtractPassenger{
		Name:         "Joao Vitor Messias",
		DocumentType: "RG",
		Document:     "numero nao identificado",
		Confidence:   0.72,
	})

	if input, ok := parseBookingCreateFromDocumentConfirmation(session, history, "ta certo"); ok {
		t.Fatalf("expected incomplete partial document to block booking create, got %+v", input)
	}
}

func TestParsePassengerDocumentCorrectionPreservesNameAndReplacesRGWithCPF(t *testing.T) {
	now := time.Now().UTC()
	session := Session{
		ID:            "session-document-correction",
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Claudecir",
	}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(1, 0))
	history := documentCorrectionBookingHistory(now)

	correction, ok := findLatestPassengerDocumentCorrection(history, "sim")
	if !ok {
		t.Fatalf("expected document correction to be detected")
	}
	if !correction.NameConfirmed || correction.DocumentType != "CPF" || correction.Document != "52998224725" {
		t.Fatalf("unexpected correction: %+v", correction)
	}
	namedCorrection, ok := parsePassengerDocumentCorrection("Claudecir Schumacher 52998224725")
	if !ok || namedCorrection.Name != "Claudecir Schumacher" || namedCorrection.Document != "52998224725" {
		t.Fatalf("expected name + CPF correction to be parsed, got ok=%v correction=%+v", ok, namedCorrection)
	}

	input, ok := parseBookingCreateFromDocumentConfirmation(session, history, "sim")
	if !ok {
		t.Fatalf("expected booking create from corrected document confirmation")
	}
	if len(input.Passengers) != 1 {
		t.Fatalf("expected one passenger, got %+v", input.Passengers)
	}
	passenger := input.Passengers[0]
	if passenger.Name != "Claudecir Schumacher" {
		t.Fatalf("expected previous extracted name to be preserved, got %+v", passenger)
	}
	if passenger.DocumentType != "CPF" || passenger.Document != "52998224725" {
		t.Fatalf("expected corrected CPF as passenger document, got %+v", passenger)
	}
	if passenger.Document == "2817314" || passenger.DocumentType == "RG" {
		t.Fatalf("old RG must not survive after CPF correction, got %+v", passenger)
	}
}

func TestTranscriptCNHeCorrectionCreatesBookingAndPaymentUsesReservationCPF(t *testing.T) {
	extract := parseDocumentExtractResult(`{
		"mode":"EXTRACTED",
		"passengers":[{
			"name":"CLAUDECIR SCHUMACHER",
			"document_type":"RG",
			"document":"2817314 SSP SC",
			"cpf":"529.982.247-25",
			"cnh":"01235234139",
			"rg":"2817314 SSP SC",
			"confidence":0.92
		}]
	}`)
	if extract.Mode != "EXTRACTED" {
		t.Fatalf("expected extracted CNH-e payload, got %+v", extract)
	}
	if len(extract.Passengers) != 1 || extract.Passengers[0].DocumentType != "RG" || extract.Passengers[0].Document != "2817314" || extract.Passengers[0].CPF != "52998224725" {
		t.Fatalf("expected RG as primary document and CPF as additional field, got %+v", extract.Passengers)
	}
	reply := buildDocumentExtractReply(extract)
	if !containsAll(reply, "CLAUDECIR SCHUMACHER | RG | 2817314") || strings.Contains(reply, "SSPSC") {
		t.Fatalf("expected RG confirmation without issuer suffix, got %q", reply)
	}

	now := time.Now().UTC()
	session := sessionWithPassengerClarificationStateForTest(
		Session{ID: "session-transcript", ContactKey: "5549988709047", CustomerPhone: "5549988709047", CustomerName: "Claudecir"},
		completePassengerStateForTest(1, 0),
	)
	history := documentCorrectionBookingHistory(now)
	input, ok := parseBookingCreateFromDocumentConfirmation(session, history, "sim")
	if !ok {
		t.Fatalf("expected corrected booking create input")
	}
	if len(input.Passengers) != 1 || input.Passengers[0].DocumentType != "CPF" || input.Passengers[0].Document != "52998224725" {
		t.Fatalf("expected booking passenger with corrected CPF, got %+v", input.Passengers)
	}

	paymentSvc := &fakePaymentCreatePaymentsService{
		payment: payments.Payment{ID: "pay-transcript", BookingID: "BK-TRANSCRIPT", Status: "PENDING", CreatedAt: now},
		raw:     json.RawMessage(`{"charges":[{"last_transaction":{"qr_code":"000201TRANSCRIPT","qr_code_url":"https://provider/pix"}}]}`),
	}
	tool := NewPaymentCreateTool(&fakePaymentCreateBookingsService{
		result: bookings.BookingDetails{
			Booking: bookings.Booking{
				ID:              "BK-TRANSCRIPT",
				Status:          "PENDING",
				ReservationCode: "TR123456",
				TotalAmount:     950,
				RemainderAmount: 950,
			},
			Passengers: []bookings.BookingPassenger{
				{
					Name:         input.Passengers[0].Name,
					Document:     input.Passengers[0].Document,
					DocumentType: input.Passengers[0].DocumentType,
					Phone:        "48999999999",
				},
			},
		},
	}, paymentSvc)

	payment, err := tool.Create(context.Background(), PaymentCreateInput{
		BookingID:       "BK-TRANSCRIPT",
		ReservationCode: "TR123456",
		PaymentType:     "integral",
	})
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	if payment.Mode != "pix_sent" || payment.PixCode != "000201TRANSCRIPT" {
		t.Fatalf("expected PIX generated from reservation CPF, got %+v", payment)
	}
	if paymentSvc.lastInput.Customer == nil || paymentSvc.lastInput.Customer.Document != "52998224725" {
		t.Fatalf("expected payment to reuse reservation passenger CPF, got %+v", paymentSvc.lastInput.Customer)
	}
}

func TestDocumentConfirmationPreservesAdditionalIdentityFields(t *testing.T) {
	now := time.Now().UTC()
	session := Session{ID: "session-doc-extra", ContactKey: "5549988709047", CustomerPhone: "5549988709047", CustomerName: "Maria"}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(1, 1))
	history := documentConfirmationBookingHistory(now, "EXTRACTED", true)
	history[2].Body = "uma pessoa, com uma crianca de 4 anos"
	history[len(history)-1].Payload = map[string]interface{}{
		"tool_context": map[string]interface{}{
			toolNameDocumentExtract: buildDocumentExtractResponsePayload(DocumentExtractResult{
				Mode:                   "EXTRACTED",
				ExpectedPassengerCount: 1,
				MediaCount:             1,
				Passengers: []DocumentExtractPassenger{
					{
						Name:                   "Maria Silva",
						DocumentType:           "CERTIDAO_NASCIMENTO",
						Document:               testBirthCertificateNumber,
						CPF:                    "84960815086",
						BirthDate:              "2022-05-21",
						BirthCertificateNumber: testBirthCertificateNumber,
						BirthCity:              "Santa Ines",
						Confidence:             0.95,
					},
				},
			}),
		},
	}

	input, ok := parseBookingCreateFromDocumentConfirmation(session, history, "conferem")
	if !ok {
		t.Fatalf("expected booking create from document confirmation")
	}
	if len(input.Passengers) != 1 {
		t.Fatalf("expected one passenger, got %+v", input.Passengers)
	}
	passenger := input.Passengers[0]
	if passenger.DocumentType != "CERTIDAO_NASCIMENTO" || passenger.Document != testBirthCertificateNumber || passenger.BirthCertificateNumber != testBirthCertificateNumber || passenger.CPF != "84960815086" {
		t.Fatalf("expected birth certificate as primary document and cpf field preserved, got %+v", passenger)
	}
	if passenger.BirthDate != "2022-05-21" || passenger.BirthCertificateNumber != testBirthCertificateNumber || passenger.BirthCity != "Santa Ines" {
		t.Fatalf("expected additional identity fields from document extract, got %+v", passenger)
	}
}

func TestLooksLikeDocumentConfirmationAcceptsNaturalConfirmations(t *testing.T) {
	cases := []string{
		"conferem",
		"confere",
		"sim",
		"sim sim",
		"isso",
		"isso mesmo",
		"correto",
		"certo",
		"sim esta correto",
		"sim está correto",
		"esta correto",
		"está correto",
		"sim correto",
		"sim esta certo",
		"sim está certo",
		"sim ta certo",
		"Sim, tá certo.",
		"esta certo",
		"está certo",
		"ta certo",
		"tá certo",
		"ta tudo certo",
		"tá tudo certo",
		"tudo certo",
		"sim ta tudo certo",
		"sim tá tudo certo",
		"sim sim ta tudo certo",
		"sim sim tá tudo certo",
		"sim sim ta tudo certo tudo certo",
		"sim sim tá tudo certo tudo certo",
		"pode seguir",
		"pode continuar",
		"pode prosseguir",
		"pode criar",
		"pode criar a reserva",
		"pode fazer a reserva",
		"pode reservar",
		"pode sim",
		"confirmado",
		"confirmo",
		"ss",
		"s",
		"positivo",
		"posi",
		"aham",
	}

	for _, tc := range cases {
		t.Run(tc, func(t *testing.T) {
			if !looksLikeDocumentConfirmation(tc) {
				t.Fatalf("expected %q to be accepted as document confirmation", tc)
			}
		})
	}
}

func TestLooksLikeDocumentConfirmationRejectsCorrectionRequests(t *testing.T) {
	cases := []string{
		"pode",
		"pode corrigir",
		"pode alterar",
		"pode mudar",
		"pode trocar",
		"pode arrumar",
		"pode ajustar",
		"pode corrigir o cpf",
		"pode alterar o documento",
		"pode trocar o nome",
		"nao",
		"não",
		"nao esta certo",
		"não está certo",
		"nao confere",
		"está errado",
		"ta errado",
		"sim, mas o cpf esta errado",
		"sim, mas precisa corrigir o documento",
		"isso, mas o nome esta errado",
	}

	for _, tc := range cases {
		if looksLikeDocumentConfirmation(tc) {
			t.Fatalf("expected %q not to be treated as document confirmation", tc)
		}
	}
}

func TestLooksLikeDocumentConfirmationContextReplyAcceptsCPFCorrection(t *testing.T) {
	cases := []string{
		"o cpf correto é 52998224725",
		"cpf correto 52998224725",
		"o nome está certo, mas o cpf é 52998224725",
		"sim, mas o cpf está errado, é 52998224725",
		"corrigir cpf 52998224725",
		"pode corrigir o cpf para 52998224725",
	}

	for _, tc := range cases {
		if !looksLikeDocumentConfirmationContextReply(tc) {
			t.Fatalf("expected %q to be treated as document confirmation context reply", tc)
		}
		if looksLikeDocumentConfirmation(tc) {
			t.Fatalf("expected %q not to be treated as pure document confirmation", tc)
		}
	}
}

func TestLooksLikeDocumentConfirmationContextReplyRejectsBareCorrectionWithoutDocument(t *testing.T) {
	cases := []string{
		"pode corrigir",
		"quero alterar",
		"está errado",
		"tem erro",
		"não confere",
	}

	for _, tc := range cases {
		if looksLikeDocumentConfirmationContextReply(tc) {
			t.Fatalf("expected %q not to be treated as actionable document correction without document value", tc)
		}
	}
}

func TestParseBookingCreateFromDocumentConfirmationRejectsCorrectionSameTurn(t *testing.T) {
	session := Session{
		ID:            "session-document-correction-same-turn",
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Messias",
	}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(1, 0))
	history := documentConfirmationBookingHistory(time.Now().UTC(), "EXTRACTED", true)

	currentTurn := "pode corrigir o cpf para 52998224725"

	if !looksLikeDocumentConfirmationContextReply(currentTurn) {
		t.Fatalf("expected correction to remain in document confirmation context")
	}
	if looksLikeDocumentConfirmation(currentTurn) {
		t.Fatalf("expected correction not to be treated as pure confirmation")
	}
	if input, ok := parseBookingCreateFromDocumentConfirmation(session, history, currentTurn); ok {
		t.Fatalf("expected correction turn not to create booking directly, got %+v", input)
	}
}

func TestParseBookingCreateFromManualPassengerConfirmation(t *testing.T) {
	now := time.Now().UTC()
	session := Session{
		ID:            "session-manual-confirmation",
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Joao",
	}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(1, 0))
	availability := AvailabilitySearchResult{
		Filter: AvailabilitySearchInput{
			Origin:      "Igarape do Meio/MA",
			Destination: "Petrolandia/SC",
			Qty:         1,
			Limit:       5,
		},
		Results: []AvailabilitySearchItem{
			{
				TripID:                 "trip-manual-1",
				BoardStopID:            "board-manual-1",
				AlightStopID:           "alight-manual-1",
				OriginDisplayName:      "Igarape do Meio/MA",
				DestinationDisplayName: "Petrolandia/SC",
				OriginDepartTime:       "11:00",
				TripDate:               "2026-05-11",
				Price:                  950,
				Currency:               "BRL",
				PackageName:            packageToSantaCatarina,
			},
		},
	}
	history := []Message{
		{
			Direction:        "OUTBOUND",
			Body:             "Datas para Igarape do Meio/MA -> Petrolandia/SC.",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-7 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availability),
				},
			},
		},
		{Direction: "INBOUND", Body: "11/05", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-6 * time.Minute)},
		{Direction: "OUTBOUND", Body: "A passagem é só para você ou tem mais alguém, informe também se há criança até 5 anos?", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-5 * time.Minute)},
		{Direction: "INBOUND", Body: "so eu", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-4 * time.Minute)},
		{Direction: "OUTBOUND", Body: "Perfeito. Agora pode enviar seu nome completo e o documento. Se preferir, pode mandar foto legivel do documento.", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-3 * time.Minute)},
		{Direction: "INBOUND", Body: "Joao Vitor Messias 84960815086", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-2 * time.Minute)},
		{Direction: "OUTBOUND", Body: "Nome e CPF confirmados. Posso prosseguir e criar a reserva?", ProcessingStatus: messageStatusAutomationDraft, ReceivedAt: now.Add(-1 * time.Minute)},
	}
	history = canonicalAvailabilitySelectionHistoryForTest(history, availability, 1, 0, 1, 2)

	input, ok := parseBookingCreateFromDocumentConfirmation(session, history, "sim")
	if !ok {
		t.Fatalf("expected booking create from manual passenger confirmation")
	}
	if input.Qty != 1 {
		t.Fatalf("expected qty 1, got %d", input.Qty)
	}
	if input.TripID == "" || input.BoardStopID == "" || input.AlightStopID == "" {
		t.Fatalf("expected trip identifiers, got %+v", input)
	}
	if len(input.Passengers) != 1 {
		t.Fatalf("expected one passenger, got %+v", input.Passengers)
	}
	passenger := input.Passengers[0]
	if passenger.Name != "Joao Vitor Messias" || passenger.DocumentType != "CPF" || passenger.Document != "84960815086" {
		t.Fatalf("unexpected passenger: %+v", passenger)
	}
	if passenger.Phone != "5549988709047" {
		t.Fatalf("expected passenger phone from session, got %+v", passenger)
	}
}

func TestParseBookingCreateFromDocumentConfirmationRejectsDetailsAboveKnownPassengerCount(t *testing.T) {
	session := Session{ID: "session-known-count-mismatch", ContactKey: "5549988709201", CustomerPhone: "5549988709201", CustomerName: "Joao"}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(1, 0))
	history := knownPassengerCountDocumentConfirmationHistory(t, "so pra mim", DocumentExtractResult{
		Mode:                   "EXTRACTED",
		ExpectedPassengerCount: 2,
		Passengers: []DocumentExtractPassenger{
			{Name: "Joao Vitor Messias", DocumentType: "CPF", Document: "52998224725", Confidence: 0.98},
			{Name: "Maria Messias", DocumentType: "RG", Document: "1234567", Confidence: 0.98},
		},
	})

	context := collectBookingDraftContext(session, history, "sim")
	if context.PassengerCount != 1 || context.ExpectedDocumentCount != 1 || context.PassengerDetailsCount != 2 {
		t.Fatalf("expected known count mismatch to remain visible, got %+v", context)
	}
	if input, ok := parseBookingCreateFromDocumentConfirmation(session, history, "sim"); ok {
		t.Fatalf("known count mismatch must not authorize booking_create, got %+v", input)
	}
}

func TestCanonicalBookingCreateContextExcessBlocksBodyPassengerBypass(t *testing.T) {
	session := Session{ID: "session-canonical-body-bypass", ContactKey: "5549988709208", CustomerPhone: "5549988709208", CustomerName: "Joao"}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(1, 0))
	history := knownPassengerCountDocumentConfirmationHistory(t, "so pra mim", DocumentExtractResult{
		Mode:                   "EXTRACTED",
		ExpectedPassengerCount: 2,
		Passengers: []DocumentExtractPassenger{
			{Name: "Joao Vitor Messias", DocumentType: "CPF", Document: "52998224725", Confidence: 0.98},
			{Name: "Maria Messias", DocumentType: "RG", Document: "1234567", Confidence: 0.98},
		},
	})
	body := "quero reservar opcao 1\nJoao Vitor Messias CPF 52998224725"
	availability := canonicalBookingCreateTestAvailability(1)
	selectionHistory := appendCanonicalAvailabilitySelectionForTest(
		nil,
		availability,
		1,
		time.Now().UTC().Add(-10*time.Minute),
	)
	history = append(selectionHistory, history...)
	session = materializePersistedAvailabilitySelectionForTest(session, history)
	selectionState, ok := availabilitySelectionStateV1FromSession(session)
	if !ok {
		t.Fatal("test must persist the current selection before exercising canonical readiness")
	}
	if _, _, ok := resolveBookingCreateSelectionFromState(body, selectionState); !ok {
		t.Fatalf("test must have a valid trip selection before exercising canonical readiness")
	}

	context := collectBookingDraftContext(session, history, body)
	readiness := evaluateCanonicalBookingCreateReadiness(context)
	if context.PassengerCount != 1 || len(readiness.Passengers) != 2 || readiness.QuantityStatus != bookingCreateQuantityExcess || readiness.Ready {
		t.Fatalf("expected canonical excess for known count 1 and merged context 2, context=%+v readiness=%+v", context, readiness)
	}
	if input, ok := parseBookingCreateInput(session, history, body, &availability); ok {
		t.Fatalf("body passenger list must not bypass canonical context excess, got %+v", input)
	}
}

func TestUnknownAvailabilityQtyDoesNotBecomePassengerCountDeclaration(t *testing.T) {
	session := Session{ID: "session-unknown-availability-qty", ContactKey: "5549988709220", CustomerPhone: "5549988709220", CustomerName: "Joao"}
	session = sessionWithPassengerClarificationStateForTest(session, unknownPassengerStateForTest())
	history := passengerSlotAvailabilityHistory(t, askPassengerCountReply)
	now := time.Now().UTC()
	extract := DocumentExtractResult{
		Mode:                   "EXTRACTED",
		ExpectedPassengerCount: 1,
		Passengers: []DocumentExtractPassenger{
			{Name: "Joao Vitor Messias", DocumentType: "CPF", Document: "52998224725", CPF: "52998224725", Confidence: 0.98},
		},
	}
	toolContext := map[string]interface{}{toolNameDocumentExtract: buildDocumentExtractResponsePayload(extract)}
	history = append(history, Message{
		Direction:         "OUTBOUND",
		Body:              buildConfirmExtractedDocumentReply(extract),
		ProcessingStatus:  messageStatusAutomationDraft,
		ReceivedAt:        now,
		Payload:           map[string]interface{}{"tool_context": toolContext},
		NormalizedPayload: map[string]interface{}{"tool_context": toolContext},
	})

	context := collectBookingDraftContext(session, history, "sim")
	readiness := evaluateCanonicalBookingCreateReadiness(context)
	if context.PassengerCountKnown || context.PassengerCount != 0 || readiness.QuantityStatus != bookingCreateQuantityUnknown || readiness.Ready {
		t.Fatalf("availability qty=1 must not become an explicit passenger declaration, context=%+v readiness=%+v", context, readiness)
	}
	if input, ok := parseBookingCreateFromDocumentConfirmation(session, history, "sim"); ok {
		t.Fatalf("document confirmation must stay blocked while passenger quantity is unknown, got %+v", input)
	}
	availability := canonicalBookingCreateTestAvailability(1)
	body := "quero reservar\nJoao Vitor Messias CPF 52998224725"
	if input, ok := parseBookingCreateInput(session, history, body, &availability); ok {
		t.Fatalf("current availability qty=1 must not authorize booking_create, got %+v", input)
	}
}

func TestCanonicalBookingCreateReadinessClassifiesQuantity(t *testing.T) {
	passenger1 := BookingCreatePassengerInput{Name: "Joao Vitor Messias", DocumentType: "CPF", Document: "52998224725", CPF: "52998224725"}
	passenger2 := BookingCreatePassengerInput{Name: "Maria Messias", DocumentType: "RG", Document: "1234567", RG: "1234567"}
	tests := []struct {
		name       string
		context    BookingDraftContext
		wantStatus bookingCreateQuantityStatus
		wantReady  bool
	}{
		{name: "unknown", context: BookingDraftContext{PassengerDetails: []BookingCreatePassengerInput{passenger1}}, wantStatus: bookingCreateQuantityUnknown},
		{name: "missing", context: BookingDraftContext{PassengerCount: 2, PassengerCountKnown: true, PassengerDetails: []BookingCreatePassengerInput{passenger1}}, wantStatus: bookingCreateQuantityMissing},
		{name: "exact", context: BookingDraftContext{PassengerCount: 1, PassengerCountKnown: true, PassengerDetails: []BookingCreatePassengerInput{passenger1}}, wantStatus: bookingCreateQuantityExact, wantReady: true},
		{name: "excess", context: BookingDraftContext{PassengerCount: 1, PassengerCountKnown: true, PassengerDetails: []BookingCreatePassengerInput{passenger1, passenger2}}, wantStatus: bookingCreateQuantityExcess},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.context.HasBookableSelection = true
			readiness := evaluateCanonicalBookingCreateReadiness(tc.context)
			if readiness.QuantityStatus != tc.wantStatus || readiness.Ready != tc.wantReady {
				t.Fatalf("unexpected canonical readiness: got %+v want status=%s ready=%t", readiness, tc.wantStatus, tc.wantReady)
			}
		})
	}
}

func TestCanonicalBookingCreateResolvedPartialByLaterDocumentExtract(t *testing.T) {
	session := Session{ID: "session-resolved-partial", ContactKey: "5549988709209", CustomerPhone: "5549988709209", CustomerName: "Ivoneide"}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(1, 0))
	history := passengerSlotAvailabilityHistory(t, askPassengerCountReply)
	now := time.Now().UTC()
	extract := DocumentExtractResult{
		Mode:                   "EXTRACTED",
		ExpectedPassengerCount: 1,
		Passengers: []DocumentExtractPassenger{
			{Name: "Ivoneide Pereira", DocumentType: "CPF", Document: "46643591104", CPF: "46643591104", Confidence: 0.98},
		},
	}
	toolContext := map[string]interface{}{toolNameDocumentExtract: buildDocumentExtractResponsePayload(extract)}
	history = append(history,
		Message{Direction: "INBOUND", Body: "so pra mim", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-5 * time.Minute)},
		Message{Direction: "OUTBOUND", Body: askChildUnder5Reply, ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-4 * time.Minute)},
		Message{Direction: "INBOUND", Body: "nao", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-3 * time.Minute)},
		Message{Direction: "OUTBOUND", Body: "Pode enviar seu nome completo e CPF.", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-2 * time.Minute)},
		Message{Direction: "INBOUND", Body: "ivoneide 46643591104", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-90 * time.Second)},
		Message{
			Direction:         "OUTBOUND",
			Body:              buildConfirmExtractedDocumentReply(extract),
			ProcessingStatus:  messageStatusAutomationDraft,
			ReceivedAt:        now.Add(-time.Minute),
			Payload:           map[string]interface{}{"tool_context": toolContext},
			NormalizedPayload: map[string]interface{}{"tool_context": toolContext},
		},
	)

	context := collectBookingDraftContext(session, history, "sim")
	readiness := evaluateCanonicalBookingCreateReadiness(context)
	if context.PartialPassengerDetailsCount != 0 || len(readiness.UnresolvedPartials) != 0 {
		t.Fatalf("later complete extract must resolve the partial with the same CPF, context=%+v readiness=%+v", context, readiness)
	}
	if !readiness.Ready || len(readiness.Passengers) != 1 || readiness.Passengers[0].Name != "Ivoneide Pereira" {
		t.Fatalf("expected canonical passenger to be ready after partial resolution, got %+v", readiness)
	}
	if input, ok := parseBookingCreateFromDocumentConfirmation(session, history, "sim"); !ok || len(input.Passengers) != 1 {
		t.Fatalf("resolved partial must allow confirmed booking_create, ok=%v input=%+v", ok, input)
	}
}

func TestCanonicalBookingCreateUnresolvedPartialsKeepOnlyRealIdentities(t *testing.T) {
	passenger := BookingCreatePassengerInput{
		Name:         "Joao Vitor Messias",
		DocumentType: "CPF",
		Document:     "52998224725",
		CPF:          "52998224725",
	}
	context := BookingDraftContext{
		HasBookableSelection: true,
		PassengerCount:       1,
		PassengerCountKnown:  true,
		PassengerDetails:     []BookingCreatePassengerInput{passenger},
		PartialPassengerDetails: []BookingPassengerDocumentPartial{
			{NameFragment: "ivoneide", DocumentType: "CPF", Document: "46643591104"},
			{NameFragment: "ivoneide pereira", DocumentType: "CPF", Document: "466.435.911-04"},
			{NameFragment: "maria", DocumentType: "RG", Document: "1234567"},
		},
	}

	readiness := evaluateCanonicalBookingCreateReadiness(context)
	if readiness.Ready || len(readiness.UnresolvedPartials) != 2 {
		t.Fatalf("expected two real unresolved identities to remain blocked, got %+v", readiness)
	}
	if readiness.UnresolvedPartials[0].NameFragment != "ivoneide pereira" {
		t.Fatalf("expected latest duplicate partial to win, got %+v", readiness.UnresolvedPartials)
	}
}

func TestParseBookingCreateFromDocumentConfirmationRejectsMergedIncompletePassenger(t *testing.T) {
	session := Session{ID: "session-merged-incomplete", ContactKey: "5549988709205", CustomerPhone: "5549988709205", CustomerName: "Joao"}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(2, 0))
	history := knownPassengerCountDocumentConfirmationHistory(t, "eu e mais uma pessoa", DocumentExtractResult{
		Mode:                   "EXTRACTED",
		ExpectedPassengerCount: 2,
		Passengers: []DocumentExtractPassenger{
			{Name: "Joao Vitor Messias", DocumentType: "CPF", Document: "52998224725", Confidence: 0.98},
			{Name: "Maria Messias", DocumentType: "RG", Confidence: 0.72},
		},
	})

	context := collectBookingDraftContext(session, history, "sim")
	if context.PassengerDetailsCount != 2 || len(context.PassengerDetails) != 2 {
		t.Fatalf("test must reconstruct the two-passenger merged list, got %+v", context)
	}
	if context.PassengerDetails[1].Document != "" {
		t.Fatalf("test must keep the second passenger incomplete, got %+v", context.PassengerDetails[1])
	}
	if input, ok := parseBookingCreateFromDocumentConfirmation(session, history, "sim"); ok {
		t.Fatalf("incomplete merged passenger must not authorize booking_create, got %+v", input)
	}
}

func TestParseBookingCreateFromDocumentConfirmationRejectsUnresolvedInboundPartial(t *testing.T) {
	session := Session{ID: "session-unresolved-partial", ContactKey: "5549988709206", CustomerPhone: "5549988709206", CustomerName: "Joao"}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(1, 0))
	history := knownPassengerCountDocumentConfirmationHistory(t, "so pra mim", DocumentExtractResult{
		Mode:                   "EXTRACTED",
		ExpectedPassengerCount: 1,
		Passengers: []DocumentExtractPassenger{
			{Name: "Joao Vitor Messias", DocumentType: "CPF", Document: "52998224725", Confidence: 0.98},
		},
	})
	now := time.Now().UTC()
	history = append(history,
		Message{Direction: "INBOUND", Body: "Joao Vitor Messias 52998224725 ivoneide 46643591104", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-time.Minute)},
		Message{Direction: "OUTBOUND", Body: "Consegui identificar estes dados. Eles conferem?\n1. Joao Vitor Messias | CPF | 529.***.***-25", ProcessingStatus: messageStatusAutomationDraft, ReceivedAt: now},
	)

	context := collectBookingDraftContext(session, history, "sim")
	readiness := evaluateCanonicalBookingCreateReadiness(context)
	if context.PassengerDetailsCount != 1 || context.PartialPassengerDetailsCount == 0 || len(readiness.UnresolvedPartials) == 0 || readiness.Ready {
		t.Fatalf("test must retain the complete passenger and unresolved partial, got %+v", context)
	}
	if input, ok := parseBookingCreateFromDocumentConfirmation(session, history, "sim"); ok {
		t.Fatalf("unresolved inbound partial must not authorize booking_create, got %+v", input)
	}
}

func TestParseBookingCreateFromDocumentConfirmationUsesMergedPassengerCorrection(t *testing.T) {
	session := Session{ID: "session-merged-passenger-correction", ContactKey: "5549988709202", CustomerPhone: "5549988709202", CustomerName: "Joao"}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(2, 0))
	history := knownPassengerCountDocumentConfirmationHistory(t, "eu e mais uma pessoa", DocumentExtractResult{
		Mode:                   "EXTRACTED",
		ExpectedPassengerCount: 2,
		Passengers: []DocumentExtractPassenger{
			{Name: "Joao Vitor Messias", DocumentType: "CPF", Document: "52998224725", Confidence: 0.98},
			{Name: "Maria Messias", DocumentType: "RG", Document: "1234567", Confidence: 0.98},
		},
	})
	now := time.Now().UTC()
	history = append(history,
		Message{Direction: "INBOUND", Body: "nome: Maria Nova RG 1234567", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-time.Minute)},
		Message{Direction: "OUTBOUND", Body: "Consegui identificar estes dados. Eles conferem?\n1. Joao Vitor Messias | CPF | 529.***.***-25\n2. Maria Nova | RG | 1234567", ProcessingStatus: messageStatusAutomationDraft, ReceivedAt: now},
	)

	context := collectBookingDraftContext(session, history, "sim")
	if len(context.PassengerDetails) != 2 || context.PassengerDetails[1].Name != "Maria Nova" {
		t.Fatalf("expected correction in merged booking context, got %+v", context.PassengerDetails)
	}
	input, ok := parseBookingCreateFromDocumentConfirmation(session, history, "sim")
	if !ok {
		t.Fatalf("expected corrected document confirmation to authorize booking_create")
	}
	if len(input.Passengers) != 2 || input.Passengers[1].Name != "Maria Nova" || input.Passengers[1].RG != "1234567" {
		t.Fatalf("booking_create must use merged corrected passenger, got %+v", input.Passengers)
	}
}

func TestParseBookingCreateFromDocumentConfirmationUsesMergedImageThenTextPassengers(t *testing.T) {
	session := Session{ID: "session-image-then-text", ContactKey: "5549988709203", CustomerPhone: "5549988709203", CustomerName: "Joao"}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(2, 0))
	history := knownPassengerCountDocumentConfirmationHistory(t, "eu e mais uma pessoa", DocumentExtractResult{
		Mode:                   "EXTRACTED",
		ExpectedPassengerCount: 2,
		Passengers: []DocumentExtractPassenger{
			{Name: "Joao Vitor Messias", DocumentType: "CPF", Document: "52998224725", Confidence: 0.98},
		},
	})
	now := time.Now().UTC()
	history = append(history,
		Message{Direction: "INBOUND", Body: "Maria Messias RG 1234567", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-time.Minute)},
		Message{Direction: "OUTBOUND", Body: "Consegui identificar estes dados. Eles conferem?\n1. Joao Vitor Messias | CPF | 529.***.***-25\n2. Maria Messias | RG | 1234567", ProcessingStatus: messageStatusAutomationDraft, ReceivedAt: now},
	)

	extract := findLatestDocumentExtractContext(history)
	if extract == nil || len(extract.Passengers) != 1 {
		t.Fatalf("test must keep the latest isolated extract partial, got %+v", extract)
	}
	context := collectBookingDraftContext(session, history, "sim")
	if context.PassengerDetailsCount != 2 || len(context.PassengerDetails) != 2 {
		t.Fatalf("expected complete merged image and text passengers, got %+v", context)
	}
	input, ok := parseBookingCreateFromDocumentConfirmation(session, history, "sim")
	if !ok {
		t.Fatalf("merged confirmation must not be gated by the shorter isolated extract")
	}
	if input.Qty != 2 || len(input.Passengers) != 2 || input.Passengers[0].Name != "Joao Vitor Messias" || input.Passengers[1].Name != "Maria Messias" {
		t.Fatalf("expected booking_create with the confirmed merged list, got %+v", input)
	}
}

func TestBookingCreateEntryPointsUseCanonicalReadinessAndPassengers(t *testing.T) {
	session := Session{ID: "session-entrypoint-readiness", ContactKey: "5549988709210", CustomerPhone: "5549988709210", CustomerName: "Joao"}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(2, 0))
	history := knownPassengerCountDocumentConfirmationHistory(t, "eu e mais uma pessoa", DocumentExtractResult{
		Mode:                   "EXTRACTED",
		ExpectedPassengerCount: 2,
		Passengers: []DocumentExtractPassenger{
			{Name: "Joao Vitor Messias", DocumentType: "CPF", Document: "52998224725", Confidence: 0.98},
			{Name: "Maria Messias", DocumentType: "RG", Document: "1234567", Confidence: 0.98},
		},
	})
	context := collectBookingDraftContext(session, history, "sim")
	readiness := evaluateCanonicalBookingCreateReadiness(context)
	if !readiness.Ready || readiness.QuantityStatus != bookingCreateQuantityExact {
		t.Fatalf("test must start from exact canonical readiness, got %+v", readiness)
	}

	availability := canonicalBookingCreateTestAvailability(2)
	fromIntent, intentOK := parseBookingCreateInput(session, history, "quero reservar opcao 1", &availability)
	fromConfirmation, confirmationOK := parseBookingCreateFromDocumentConfirmation(session, history, "sim")
	if !intentOK || !confirmationOK {
		t.Fatalf("both entry points must accept the same ready context, intent=%t confirmation=%t", intentOK, confirmationOK)
	}
	if !reflect.DeepEqual(fromIntent.Passengers, readiness.Passengers) || !reflect.DeepEqual(fromConfirmation.Passengers, readiness.Passengers) {
		t.Fatalf("entry points must use the exact canonical passenger list, readiness=%+v intent=%+v confirmation=%+v", readiness.Passengers, fromIntent.Passengers, fromConfirmation.Passengers)
	}
	if fromIntent.Qty != readiness.Expected || fromConfirmation.Qty != readiness.Expected {
		t.Fatalf("entry points must use canonical expected count %d, intent=%d confirmation=%d", readiness.Expected, fromIntent.Qty, fromConfirmation.Qty)
	}
}

func TestParseBookingCreateFromDocumentConfirmationStillRequiresLapChildAssignment(t *testing.T) {
	now := time.Now().UTC()
	session := Session{ID: "session-lap-child-unassigned", ContactKey: "5549988709207", CustomerPhone: "5549988709207", CustomerName: "Joao"}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(2, 1))
	history := lapChildBookingHistory(now, "Joao Vitor Messias 52998224725\nMaria Messias RG 1234567")

	context := collectBookingDraftContext(session, history, "sim")
	readiness := evaluateCanonicalBookingCreateReadiness(context)
	if context.PassengerDetailsCount != 2 || context.ChildUnder5Count != 1 || context.LapChildAssignmentKnown || readiness.LapChildAssignmentComplete || readiness.Ready {
		t.Fatalf("test must have two complete passengers with child assignment pending, got %+v", context)
	}
	if input, ok := parseBookingCreateFromDocumentConfirmation(session, history, "sim"); ok {
		t.Fatalf("unassigned lap child must not authorize booking_create, got %+v", input)
	}
}

func canonicalBookingCreateTestAvailability(qty int) AvailabilitySearchResult {
	return AvailabilitySearchResult{
		Filter: AvailabilitySearchInput{Origin: "Santa Ines/MA", Destination: "Fraiburgo/SC", Qty: qty, Limit: 1},
		Results: []AvailabilitySearchItem{{
			TripID: "trip-canonical-1", BoardStopID: "board-canonical-1", AlightStopID: "alight-canonical-1",
			OriginDisplayName: "Santa Ines/MA", DestinationDisplayName: "Fraiburgo/SC", OriginDepartTime: "12:00", TripDate: "2026-08-25",
		}},
	}
}

func TestBookingCreateDoesNotInferLapChildFromGenericSim(t *testing.T) {
	now := time.Now().UTC()
	session := Session{
		ID:            "session-no-lap-child",
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
	}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(1, 0))
	availability := AvailabilitySearchResult{
		Filter: AvailabilitySearchInput{Origin: "Igarape do Meio/MA", Destination: "Petrolandia/SC", Qty: 1, Limit: 5},
		Results: []AvailabilitySearchItem{{
			TripID: "trip-no-child-1", BoardStopID: "board-no-child-1", AlightStopID: "alight-no-child-1",
			OriginDisplayName: "Igarape do Meio/MA", DestinationDisplayName: "Petrolandia/SC", OriginDepartTime: "11:00", TripDate: "2026-05-11",
		}},
	}
	history := []Message{
		{
			Direction:        "OUTBOUND",
			Body:             "Datas para Igarape do Meio/MA -> Petrolandia/SC.",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-7 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availability),
				},
			},
		},
		{Direction: "INBOUND", Body: "primeira opcao", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-6 * time.Minute)},
		{Direction: "OUTBOUND", Body: "A passagem é só para você ou tem mais alguém, informe também se há criança até 5 anos?", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-5 * time.Minute)},
		{Direction: "INBOUND", Body: "so eu", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-4 * time.Minute)},
		{Direction: "OUTBOUND", Body: "Perfeito. Agora pode enviar seu nome completo e o documento.", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-3 * time.Minute)},
		{Direction: "INBOUND", Body: "Joao Vitor Messias 84960815086", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-2 * time.Minute)},
		{Direction: "OUTBOUND", Body: "Posso prosseguir e criar a reserva?", ProcessingStatus: messageStatusAutomationDraft, ReceivedAt: now.Add(-1 * time.Minute)},
	}
	history = canonicalAvailabilitySelectionHistoryForTest(history, availability, 1, 0, 1, 2)

	context := collectBookingDraftContext(session, history, "sim")
	if context.ChildUnder5Count != 0 {
		t.Fatalf("expected child_under_5_count 0, got %+v", context)
	}
	input, ok := parseBookingCreateFromDocumentConfirmation(session, history, "sim")
	if !ok {
		t.Fatalf("expected booking create from confirmation")
	}
	for _, passenger := range input.Passengers {
		if passenger.IsLapChild {
			t.Fatalf("did not expect lap child from generic sim, got %+v", input.Passengers)
		}
	}
}

func TestDocumentConfirmationDoesNotCreateBookingWithoutPreviousDocumentExtract(t *testing.T) {
	session := Session{
		ID:            "session-document-missing",
		ContactKey:    "5549988709047",
		CustomerPhone: "5549988709047",
		CustomerName:  "Messias",
	}
	session = sessionWithPassengerClarificationStateForTest(session, completePassengerStateForTest(1, 0))
	history := documentConfirmationBookingHistory(time.Now().UTC(), "EXTRACTED", false)

	if input, ok := parseBookingCreateFromDocumentConfirmation(session, history, "conferem"); ok {
		t.Fatalf("outbound confirmation without structured or inbound passenger data must not create booking, got %+v", input)
	}

	history = documentConfirmationBookingHistory(time.Now().UTC(), "PARTIAL", true)
	if input, ok := parseBookingCreateFromDocumentConfirmation(session, history, "conferem"); !ok || input.Qty != 1 {
		t.Fatalf("expected partial document_extract with complete CPF to create booking, got ok=%v input=%+v", ok, input)
	}
}

func TestLastBotAskedPassengerCount(t *testing.T) {
	history := []Message{
		{Direction: "OUTBOUND", Body: "Perfeito, a passagem e so para voce ou tem mais alguem? Ha crianca de ate 5 anos viajando?"},
		{Direction: "INBOUND", Body: "é só para mim"},
	}
	if !lastBotAskedPassengerCount(history) {
		t.Fatal("expected passenger count question to be detected")
	}
}

func documentConfirmationBookingHistory(now time.Time, documentMode string, includeDocumentExtract bool) []Message {
	history := documentConfirmationBookingHistoryWithoutBookableSelection(now, documentMode, includeDocumentExtract)
	selection := Message{
		Direction:        "OUTBOUND",
		Body:             askPassengerCountReply,
		ProcessingStatus: messageStatusAutomationSent,
		ReceivedAt:       now.Add(-225 * time.Second),
	}
	history = append(history, Message{})
	copy(history[3:], history[2:])
	history[2] = selection
	return canonicalAvailabilitySelectionHistoryForTest(
		history,
		documentConfirmationAvailabilityResult(),
		1,
		0,
		1,
		2,
	)
}

func documentConfirmationBookingHistoryWithoutBookableSelection(now time.Time, documentMode string, includeDocumentExtract bool) []Message {
	history := []Message{
		{
			Direction:        "OUTBOUND",
			Body:             "Opções para Santa Inês/MA -> Fraiburgo/SC.",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-5 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(documentConfirmationAvailabilityResult()),
				},
			},
		},
		{
			Direction:        "INBOUND",
			Body:             "a primeira",
			ProcessingStatus: "PROCESSED",
			ReceivedAt:       now.Add(-4 * time.Minute),
		},
		{
			Direction:        "INBOUND",
			Body:             "so pra mim, sem crianca",
			ProcessingStatus: "PROCESSED",
			ReceivedAt:       now.Add(-210 * time.Second),
		},
		{
			Direction:        "OUTBOUND",
			Body:             "Perfeito. Agora pode enviar seu nome completo e o documento. Se preferir, pode mandar foto legivel do documento.",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-3 * time.Minute),
		},
	}
	if includeDocumentExtract {
		history = append(history, Message{
			Direction:        "OUTBOUND",
			Body:             "Messias, consegui identificar estes dados. Eles conferem?\n- Passageiro 1: Joao Vitor Messias | CPF | 84960815086",
			ProcessingStatus: messageStatusAutomationDraft,
			ReceivedAt:       now.Add(-2 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameDocumentExtract: buildDocumentExtractResponsePayload(DocumentExtractResult{
						Mode:                   documentMode,
						ExpectedPassengerCount: 1,
						MediaCount:             1,
						Passengers: []DocumentExtractPassenger{
							{
								Name:         "Joao Vitor Messias",
								DocumentType: "CPF",
								Document:     "84960815086",
								Confidence:   0.98,
							},
						},
					}),
				},
			},
		})
	} else {
		history = append(history, Message{
			Direction:        "OUTBOUND",
			Body:             "Messias, consegui identificar estes dados. Eles conferem?\n- Passageiro 1: Joao Vitor Messias | CPF | 84960815086",
			ProcessingStatus: messageStatusAutomationDraft,
			ReceivedAt:       now.Add(-2 * time.Minute),
		})
	}
	return history
}

func documentConfirmationAvailabilityResult() AvailabilitySearchResult {
	return AvailabilitySearchResult{
		Filter: AvailabilitySearchInput{
			Origin:      "Santa Inês/MA",
			Destination: "Fraiburgo/SC",
			Qty:         1,
			Limit:       8,
		},
		Results: []AvailabilitySearchItem{
			{
				TripID:                 "trip-doc-1",
				BoardStopID:            "board-doc-1",
				AlightStopID:           "alight-doc-1",
				OriginDisplayName:      "Santa Inês/MA",
				DestinationDisplayName: "Fraiburgo/SC",
				OriginDepartTime:       "12:00",
				TripDate:               "2026-05-11",
				Price:                  950,
				Currency:               "BRL",
				PackageName:            packageToSantaCatarina,
			},
		},
	}
}

func knownPassengerCountDocumentConfirmationHistory(t *testing.T, passengerReply string, extract DocumentExtractResult) []Message {
	t.Helper()
	history := passengerSlotAvailabilityHistory(t, askPassengerCountReply)
	now := time.Now().UTC()
	toolContext := map[string]interface{}{
		toolNameDocumentExtract: buildDocumentExtractResponsePayload(extract),
	}
	history = append(history,
		Message{Direction: "INBOUND", Body: passengerReply, ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-5 * time.Minute)},
		Message{Direction: "OUTBOUND", Body: askChildUnder5Reply, ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-4 * time.Minute)},
		Message{Direction: "INBOUND", Body: "nao", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-3 * time.Minute)},
		Message{Direction: "OUTBOUND", Body: "Pode enviar os nomes completos e os documentos dos passageiros.", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-2 * time.Minute)},
		Message{
			Direction:        "OUTBOUND",
			Body:             buildConfirmExtractedDocumentReply(extract),
			ProcessingStatus: messageStatusAutomationDraft,
			ReceivedAt:       now.Add(-90 * time.Second),
			Payload:          map[string]interface{}{"tool_context": toolContext},
			NormalizedPayload: map[string]interface{}{
				"tool_context": toolContext,
			},
		},
	)
	return history
}

func partialDocumentConfirmationBookingHistory(now time.Time, passenger DocumentExtractPassenger) []Message {
	history := documentConfirmationBookingHistory(now, "EXTRACTED", false)
	result := DocumentExtractResult{
		Mode:                   "PARTIAL",
		ExpectedPassengerCount: 1,
		MediaCount:             1,
		Passengers:             []DocumentExtractPassenger{passenger},
	}
	toolContext := map[string]interface{}{
		toolNameDocumentExtract: buildDocumentExtractResponsePayload(result),
	}
	last := len(history) - 1
	history[last].Body = buildConfirmExtractedDocumentReply(result)
	history[last].Payload = map[string]interface{}{"tool_context": toolContext}
	history[last].NormalizedPayload = map[string]interface{}{"tool_context": toolContext}
	return history
}

func documentCorrectionBookingHistory(now time.Time) []Message {
	availability := AvailabilitySearchResult{
		Filter: AvailabilitySearchInput{
			Origin:      "Monção/MA",
			Destination: "Fraiburgo/SC",
			Qty:         1,
			Limit:       5,
		},
		Results: []AvailabilitySearchItem{
			{
				TripID:                 "trip-correction-1",
				BoardStopID:            "board-correction-1",
				AlightStopID:           "alight-correction-1",
				OriginDisplayName:      "Monção/MA",
				DestinationDisplayName: "Fraiburgo/SC",
				OriginDepartTime:       "09:00",
				TripDate:               "2026-05-11",
				Price:                  950,
				Currency:               "BRL",
			},
		},
	}
	history := []Message{
		{
			Direction:        "OUTBOUND",
			Body:             "Opções para Monção/MA -> Fraiburgo/SC.",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-8 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availability),
				},
			},
		},
		{Direction: "INBOUND", Body: "primeira", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-7 * time.Minute)},
		{Direction: "OUTBOUND", Body: "A passagem e so para voce ou vai mais alguem junto? Tem crianca de ate 5 anos?", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-6 * time.Minute)},
		{Direction: "INBOUND", Body: "so eu", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-5 * time.Minute)},
		{Direction: "OUTBOUND", Body: "Tem crianca de 5 anos ou menos viajando?", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-4 * time.Minute)},
		{Direction: "INBOUND", Body: "nao", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-3 * time.Minute)},
		{Direction: "OUTBOUND", Body: "Perfeito. Agora pode enviar seu nome completo e o documento. Se preferir, pode mandar foto legivel do documento.", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-150 * time.Second)},
		{
			Direction:        "OUTBOUND",
			Body:             "Consegui identificar estes dados. Eles conferem? Posso prosseguir e criar a reserva?\n1. Claudecir Schumacher | RG | 2817314",
			ProcessingStatus: messageStatusAutomationDraft,
			ReceivedAt:       now.Add(-120 * time.Second),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameDocumentExtract: buildDocumentExtractResponsePayload(DocumentExtractResult{
						Mode:                   "EXTRACTED",
						ExpectedPassengerCount: 1,
						MediaCount:             1,
						Passengers: []DocumentExtractPassenger{
							{Name: "Claudecir Schumacher", DocumentType: "RG", Document: "2817314", RG: "2817314", Confidence: 0.9},
						},
					}),
				},
			},
		},
		{Direction: "INBOUND", Body: "o nome está certo, mas quero que use o cpf 52998224725", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-90 * time.Second)},
		{Direction: "OUTBOUND", Body: "Vou usar o CPF 529*******25. Posso prosseguir e criar a reserva?", ProcessingStatus: messageStatusAutomationDraft, ReceivedAt: now.Add(-60 * time.Second)},
	}
	return canonicalAvailabilitySelectionHistoryForTest(history, availability, 1, 0, 1, 2)
}

func lapChildBookingHistory(now time.Time, passengerDetails string) []Message {
	availability := AvailabilitySearchResult{
		Filter: AvailabilitySearchInput{Origin: "Santa Ines/MA", Destination: "Monte Carlo/SC", Qty: 2, Limit: 5},
		Results: []AvailabilitySearchItem{{
			TripID: "trip-lap-1", BoardStopID: "board-lap-1", AlightStopID: "alight-lap-1",
			OriginDisplayName: "Santa Ines/MA", DestinationDisplayName: "Monte Carlo/SC",
			OriginDepartTime: "12:00", TripDate: "2026-05-25", Price: 950, Currency: "BRL",
		}},
	}
	history := []Message{
		{
			Direction:        "OUTBOUND",
			Body:             "Opcoes para Santa Ines/MA -> Monte Carlo/SC.",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-7 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availability),
				},
			},
		},
		{Direction: "INBOUND", Body: "primeira", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-6 * time.Minute)},
		{Direction: "OUTBOUND", Body: "A passagem e so para voce ou vai mais alguem junto?", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-5 * time.Minute)},
		{Direction: "INBOUND", Body: "eu e meu filho de 4 anos", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-4 * time.Minute)},
		{Direction: "OUTBOUND", Body: "Perfeito. Agora pode enviar os nomes completos e os documentos dos 2 passageiros faltantes (CPF ou RG).", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-3 * time.Minute)},
		{Direction: "INBOUND", Body: passengerDetails, ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-2 * time.Minute)},
		{Direction: "OUTBOUND", Body: "Consegui identificar estes dados. Eles conferem? Posso prosseguir e criar a reserva?", ProcessingStatus: messageStatusAutomationDraft, ReceivedAt: now.Add(-1 * time.Minute)},
	}
	return canonicalAvailabilitySelectionHistoryForTest(history, availability, 1, 0, 1, 2)
}

func soloChildBookingHistory(now time.Time) []Message {
	return soloChildBookingHistoryWithReply(now, "sim")
}

func soloChildBookingHistoryWithReply(now time.Time, childReply string) []Message {
	availability := AvailabilitySearchResult{
		Filter: AvailabilitySearchInput{Origin: "Santa Ines/MA", Destination: "Fraiburgo/SC", Qty: 1, Limit: 5},
		Results: []AvailabilitySearchItem{{
			TripID: "trip-solo-child-1", BoardStopID: "board-solo-child-1", AlightStopID: "alight-solo-child-1",
			OriginDisplayName: "Santa Ines/MA", DestinationDisplayName: "Fraiburgo/SC", OriginDepartTime: "12:00", TripDate: "2026-05-25", Price: 950, Currency: "BRL",
		}},
	}
	history := []Message{
		{
			Direction:        "OUTBOUND",
			Body:             "Datas para Santa Ines/MA -> Fraiburgo/SC.",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(-7 * time.Minute),
			Payload: map[string]interface{}{
				"tool_context": map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availability),
				},
			},
		},
		{Direction: "INBOUND", Body: "opcao 1", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-6 * time.Minute)},
		{Direction: "OUTBOUND", Body: "A passagem e so para voce ou vai mais alguem junto?", ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-5 * time.Minute)},
		{Direction: "INBOUND", Body: "so pra mim", ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-4 * time.Minute)},
		{Direction: "OUTBOUND", Body: askChildUnder5Reply, ProcessingStatus: messageStatusAutomationSent, ReceivedAt: now.Add(-3 * time.Minute)},
		{Direction: "INBOUND", Body: childReply, ProcessingStatus: "PROCESSED", ReceivedAt: now.Add(-2 * time.Minute)},
	}
	return canonicalAvailabilitySelectionHistoryForTest(history, availability, 1, 0, 1, 2)
}

func TestLastBotAskedPassengerCountIgnoresOlderPassengerQuestionAfterDocumentRequest(t *testing.T) {
	history := []Message{
		{Direction: "OUTBOUND", Body: "Perfeito, a passagem e so para voce ou tem mais alguem? Ha crianca de ate 5 anos viajando?"},
		{Direction: "INBOUND", Body: "é só para mim"},
		{Direction: "OUTBOUND", Body: "Pode enviar os nomes completos e os documentos dos dois. Se preferir, pode mandar foto legivel do documento."},
	}
	if lastBotAskedPassengerCount(history) {
		t.Fatal("expected latest outbound document request to clear passenger count context")
	}
}

func TestRouteAndPassengerCollectionPromptDoesNotActLikePassengerOnlyContext(t *testing.T) {
	history := []Message{
		{Direction: "OUTBOUND", Body: "Qual a cidade de origem, a data da viagem e quantos passageiros vao viajar?"},
	}
	if !lastBotAskedPassengerCount(history) {
		t.Fatal("expected broad prompt to contain passenger count question")
	}
	if !lastBotAskedRouteAndPassengerCollection(history) {
		t.Fatal("expected broad route/date/passenger prompt to be classified separately")
	}
}

func TestParsePassengerCountReply(t *testing.T) {
	cases := []struct {
		name       string
		text       string
		passengers int
		children   int
		ok         bool
	}{
		{name: "solo", text: "é só para mim", passengers: 1, children: 0, ok: true},
		{name: "alone", text: "sou só eu", passengers: 1, children: 0, ok: true},
		{name: "couple", text: "eu e minha esposa", passengers: 2, children: 0, ok: true},
		{name: "two_people", text: "eu e mais uma pessoa", passengers: 2, children: 0, ok: true},
		{name: "child_denial", text: "não tem criança", passengers: 0, children: 0, ok: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			passengers, children, ok := parsePassengerCountReply(tc.text)
			if ok != tc.ok || passengers != tc.passengers || children != tc.children {
				t.Fatalf("unexpected parse result for %q: passengers=%d children=%d ok=%v", tc.text, passengers, children, ok)
			}
		})
	}
}

func TestParsePassengerClarificationSlotsPraMim(t *testing.T) {
	cases := []string{
		"pra mim",
		"para mim",
		"A passagem é só pra mim.",
		"A passagem é só para mim.",
		"É só pra mim.",
		"É só para mim.",
		"so pra mim",
		"só pra mim",
		"eh so pra mim",
		"so eu",
		"só eu",
		"sou eu",
		"Sou só eu.",
		"Só eu mesmo.",
		"Só pra mim mesmo.",
		"Pra mim mesmo.",
		"sozinho",
		"Eu vou sozinho.",
		"Vou sozinho.",
		"Eu sozinho.",
		"Passagem só pra mim.",
		"a passagem é só para mim mesmo",
		"a passagem é só para ele",
		"só pra min",
		"somente para mim",
	}

	for _, text := range cases {
		t.Run(text, func(t *testing.T) {
			slots := parsePassengerClarificationSlots(text)
			if !slots.PassengerCountKnown || slots.PassengerCount != 1 {
				t.Fatalf("expected passenger_count=1 known for %q, got %+v", text, slots)
			}
			if slots.ChildUnder5CountKnown {
				t.Fatalf("expected child slot unknown for %q, got %+v", text, slots)
			}
		})
	}
}

func TestParsePassengerClarificationSlotsSoloAndNoChild(t *testing.T) {
	cases := []struct {
		text          string
		wantPassenger bool
		wantChild     bool
	}{
		{text: "A passagem é só pra mim, não tem criança.", wantPassenger: true, wantChild: true},
		{text: "Só eu, sem criança.", wantPassenger: true, wantChild: true},
		{text: "Sou eu mesmo, sem criança.", wantPassenger: true, wantChild: true},
		{text: "Não tem criança.", wantChild: true},
	}

	for _, tc := range cases {
		t.Run(tc.text, func(t *testing.T) {
			slots := parsePassengerClarificationSlots(tc.text)
			if tc.wantPassenger {
				if !slots.PassengerCountKnown || slots.PassengerCount != 1 {
					t.Fatalf("expected passenger_count=1 known for %q, got %+v", tc.text, slots)
				}
			} else if slots.PassengerCountKnown {
				t.Fatalf("expected passenger slot unknown for %q, got %+v", tc.text, slots)
			}
			if tc.wantChild {
				if !slots.ChildUnder5CountKnown || slots.ChildUnder5Count != 0 {
					t.Fatalf("expected child_under_5_count=0 known for %q, got %+v", tc.text, slots)
				}
			} else if slots.ChildUnder5CountKnown {
				t.Fatalf("expected child slot unknown for %q, got %+v", tc.text, slots)
			}
		})
	}
}

func TestParsePassengerClarificationSlotsSoloDoesNotOverrideCompanion(t *testing.T) {
	cases := []string{
		"só eu e mais uma pessoa",
		"somente eu e minha filha",
		"a passagem é só para mim e meu filho",
	}

	for _, text := range cases {
		t.Run(text, func(t *testing.T) {
			slots := parsePassengerClarificationSlots(text)
			if slots.PassengerCountKnown && slots.PassengerCount == 1 {
				t.Fatalf("expected companion cue not to become passenger_count=1 for %q, got %+v", text, slots)
			}
		})
	}
}

func TestParsePassengerClarificationSlotsMePlusAndExplicitTotals(t *testing.T) {
	cases := []struct {
		name       string
		text       string
		passengers int
	}{
		{name: "me_plus_1_digit", text: "eu e mais 1 pessoa", passengers: 2},
		{name: "me_plus_1_word", text: "eu e mais uma pessoa", passengers: 2},
		{name: "me_plus_2_digit", text: "eu e mais 2 pessoas", passengers: 3},
		{name: "me_plus_2_word", text: "eu e mais duas pessoas", passengers: 3},
		{name: "me_plus_3_digit", text: "eu e mais 3 pessoas", passengers: 4},
		{name: "me_plus_3_word", text: "eu e mais tres pessoas", passengers: 4},
		{name: "pra_mim_plus_2_digit", text: "pra mim e mais 2 pessoas", passengers: 3},
		{name: "para_mim_plus_2_word", text: "para mim e mais duas pessoas", passengers: 3},
		{name: "me_plus_3_passengers", text: "eu e mais 3 passageiros", passengers: 4},
		{name: "three_people", text: "3 pessoas", passengers: 3},
		{name: "three_people_total", text: "3 pessoas no total", passengers: 3},
		{name: "somos_three_people", text: "somos 3 pessoas", passengers: 3},
		{name: "somos_em_three", text: "somos em 3", passengers: 3},
		{name: "vai_three_people", text: "vai 3 pessoas", passengers: 3},
		{name: "four_passengers", text: "4 passageiros", passengers: 4},
		{name: "two_people_word", text: "duas pessoas", passengers: 2},
		{name: "three_people_word", text: "tres pessoas", passengers: 3},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			slots := parsePassengerClarificationSlots(tc.text)
			if !slots.PassengerCountKnown || slots.PassengerCount != tc.passengers {
				t.Fatalf("expected passenger_count=%d known for %q, got %+v", tc.passengers, tc.text, slots)
			}
		})
	}
}

func TestParsePassengerClarificationSlotsMePlusDoesNotBecomeSolo(t *testing.T) {
	slots := parsePassengerClarificationSlots("eu e mais 3 pessoas")
	if slots.PassengerCount == 1 {
		t.Fatalf("expected me-plus reply not to become solo, got %+v", slots)
	}
	if !slots.PassengerCountKnown || slots.PassengerCount != 4 {
		t.Fatalf("expected passenger_count=4 known, got %+v", slots)
	}
}

func TestParsePassengerClarificationSlotsExplicitTotalWithChild(t *testing.T) {
	slots := parsePassengerClarificationSlots("3 pessoas no total, uma crianca de 4 anos")
	if !slots.PassengerCountKnown || slots.PassengerCount != 3 {
		t.Fatalf("expected passenger_count=3 known, got %+v", slots)
	}
	if !slots.ChildUnder5CountKnown || slots.ChildUnder5Count != 1 {
		t.Fatalf("expected child_under_5_count=1 known, got %+v", slots)
	}
}

func TestParsePassengerClarificationSlotsNaoChild(t *testing.T) {
	cases := []string{
		"nao",
		"não",
		"sem criança",
		"nao tem criança",
		"não tem criança",
		"não vai criança",
		"não tem criança de 5 anos",
		"não tem criança não",
	}

	for _, text := range cases {
		t.Run(text, func(t *testing.T) {
			slots := parsePassengerClarificationSlots(text)
			if !slots.ChildUnder5CountKnown || slots.ChildUnder5Count != 0 {
				t.Fatalf("expected child_under_5_count=0 known for %q, got %+v", text, slots)
			}
			if slots.PassengerCountKnown {
				t.Fatalf("expected passenger slot unknown for %q, got %+v", text, slots)
			}
		})
	}
}
