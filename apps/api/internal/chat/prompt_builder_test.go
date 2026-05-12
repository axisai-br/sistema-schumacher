package chat

import (
	"strings"
	"testing"
	"time"
)

func TestBuildAgentUserPromptGuidesPassengerCheckpointAfterTravelOptionChoice(t *testing.T) {
	session := Session{
		Channel:       "WHATSAPP",
		CustomerPhone: "5549988709047",
		CustomerName:  "Messias",
	}
	memory := map[string]interface{}{
		"current_turn_body": "quero a primeira opcao saindo de igarape do meio",
		"recent_messages": []map[string]interface{}{
			{"direction": "INBOUND", "body": "quero passagem para sc"},
			{"direction": "INBOUND", "body": "monte carlo"},
			{"direction": "INBOUND", "body": "26/04"},
			{"direction": "OUTBOUND", "body": "26/04/2026 - Moncao 09:00; Igarape do Meio 11:00; Santa Ines 12:00 - R$ 950. Deseja alguma dessas opcoes?"},
		},
	}

	prompt := buildAgentUserPrompt(session, memory, agentToolContext{})

	if !strings.Contains(prompt, "Caso atual: o cliente acabou de escolher uma opcao de viagem") {
		t.Fatalf("expected travel option guidance in prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "se a passagem e so para ele ou se ha mais alguem incluso") {
		t.Fatalf("expected passenger checkpoint guidance in prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "nao pedir documento nem falar de pagamento ainda") {
		t.Fatalf("expected prompt to block document/payment jump, got %q", prompt)
	}
}

func TestBuildAgentUserPromptGuidesIntegralOrDepositAfterBookingCreate(t *testing.T) {
	session := Session{
		Channel:       "WHATSAPP",
		CustomerPhone: "5549988709047",
		CustomerName:  "Messias",
	}
	memory := map[string]interface{}{
		"current_turn_body": "isso",
	}
	tools := agentToolContext{
		BookingCreate: &BookingCreateResult{
			Filter: BookingCreateInput{
				OriginDisplayName:      "Igarape do Meio/MA",
				DestinationDisplayName: "Monte Carlo/SC",
				TripDate:               "2026-04-26",
				DepartureTime:          "11:00",
				Qty:                    1,
			},
			Mode:            "created",
			BookingID:       "BK-ABC123456",
			ReservationCode: "ABC12345",
			Status:          "PENDING",
			TotalAmount:     950,
			DepositAmount:   250,
			RemainderAmount: 700,
		},
	}

	prompt := buildAgentUserPrompt(session, memory, tools)

	if !strings.Contains(prompt, "valor integral ou apenas o sinal de R$ 250 por passageiro pagante") {
		t.Fatalf("expected integral-or-deposit guidance in prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "nao perguntar PIX, cartao ou pagar no embarque antes de o cliente escolher entre integral e sinal") {
		t.Fatalf("expected prompt to block generic payment-method question, got %q", prompt)
	}
}

func TestAvailabilityReplyUsesOnlyReturnedDepartureTimes(t *testing.T) {
	session := Session{
		Channel:       "WHATSAPP",
		CustomerPhone: "5549988709047",
		CustomerName:  "Messias",
	}
	tripDate := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	tools := agentToolContext{
		Availability: &AvailabilitySearchResult{
			Filter: AvailabilitySearchInput{
				Origin:      "Fraiburgo/SC",
				Destination: "Moncao/MA",
				PackageName: packageToMaranhao,
				TripDate:    &tripDate,
				Qty:         1,
				Limit:       8,
			},
			Results: []AvailabilitySearchItem{
				{
					OriginDisplayName:      "Fraiburgo/SC",
					DestinationDisplayName: "Moncao/MA",
					OriginDepartTime:       "15:00",
					TripDate:               "2026-05-18",
					Price:                  950,
					Currency:               "BRL",
					PackageName:            packageToMaranhao,
				},
			},
		},
	}

	prompt := buildAgentUserPrompt(session, map[string]interface{}{"current_turn_body": "Fraiburgo para monção 18/05"}, tools)

	if !strings.Contains(prompt, "15:00") {
		t.Fatalf("expected prompt to contain returned departure time, got %q", prompt)
	}
	if !strings.Contains(prompt, "Use somente o horario 15:00") {
		t.Fatalf("expected prompt to force the single returned departure time, got %q", prompt)
	}
	if !strings.Contains(prompt, "nao ofereca manha, tarde, noite") {
		t.Fatalf("expected prompt to block abstract time periods, got %q", prompt)
	}
}

func TestBuildAgentUserPromptMentionsCurrentTurnMediaWithoutText(t *testing.T) {
	session := Session{
		Channel:       "WHATSAPP",
		CustomerPhone: "5549988709047",
		CustomerName:  "Messias",
	}
	memory := map[string]interface{}{
		"current_turn_body":  "",
		"current_turn_kinds": []string{"IMAGE"},
		"current_turn_media": []map[string]interface{}{
			{"kind": "IMAGE", "url": "https://files.example.test/rg.jpg", "mime_type": "image/jpeg"},
		},
		"recent_messages": []map[string]interface{}{
			{"direction": "OUTBOUND", "kind": "TEXT", "body": "Pode enviar a foto legivel do documento."},
			{"direction": "INBOUND", "kind": "IMAGE", "body": ""},
		},
	}

	prompt := buildAgentUserPrompt(session, memory, agentToolContext{})

	if !strings.Contains(prompt, "Tipos da mensagem atual: IMAGE") {
		t.Fatalf("expected prompt to mention current turn kind, got %q", prompt)
	}
	if !strings.Contains(prompt, "Midia recebida no turno atual: 1 arquivo(s)") {
		t.Fatalf("expected prompt to mention media count, got %q", prompt)
	}
	if !strings.Contains(prompt, "INBOUND [IMAGE]: [sem texto]") {
		t.Fatalf("expected prompt to preserve non-text history, got %q", prompt)
	}
}

func TestBuildAgentUserPromptGuidesImageExtractionAndMissingPassengerDocuments(t *testing.T) {
	session := Session{
		Channel:       "WHATSAPP",
		CustomerPhone: "5549988709047",
		CustomerName:  "Messias",
	}
	memory := map[string]interface{}{
		"current_turn_body":  "",
		"current_turn_kinds": []string{"IMAGE"},
		"current_turn_media": []map[string]interface{}{
			{"kind": "IMAGE", "url": "https://files.example.test/rg.jpg", "mime_type": "image/jpeg"},
		},
		"recent_messages": []map[string]interface{}{
			{"direction": "INBOUND", "body": "primeira opcao"},
			{"direction": "OUTBOUND", "body": "A passagem e so para voce ou ha mais passageiros? Tem crianca de ate 5 anos ou menos viajando?"},
			{"direction": "INBOUND", "body": "eu e minha esposa"},
			{"direction": "OUTBOUND", "body": "Pode enviar os nomes completos e os documentos dos dois. Se preferir, pode mandar foto legivel do documento."},
		},
	}

	prompt := buildAgentUserPrompt(session, memory, agentToolContext{})

	if !strings.Contains(prompt, "Primeiro tente extrair nome completo + tipo + numero") {
		t.Fatalf("expected prompt to force extraction first, got %q", prompt)
	}
	if !strings.Contains(prompt, "Quantidad") || !strings.Contains(prompt, "2") {
		t.Fatalf("expected prompt to mention expected passenger count, got %q", prompt)
	}
	if !strings.Contains(prompt, "peca apenas o documento do(s) passageiro(s) restante(s)") {
		t.Fatalf("expected prompt to ask only for missing passenger documents, got %q", prompt)
	}
}
