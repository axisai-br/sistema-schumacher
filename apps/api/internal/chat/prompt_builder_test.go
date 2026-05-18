package chat

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBuildAgentSystemPromptIsMinimalSafetyPrompt(t *testing.T) {
	prompt := buildAgentSystemPrompt()
	if len(prompt) > 700 {
		t.Fatalf("expected compact system prompt, got %d chars", len(prompt))
	}
	for _, removed := range []string{
		"Fraiburgo R$ 950",
		"valor integral ou apenas o sinal",
		"Primeiro tente extrair",
		"Guardrail de direcao",
		"RESULTADO DE FERRAMENTA",
	} {
		if strings.Contains(prompt, removed) {
			t.Fatalf("expected removed business rule %q not to be in system prompt: %q", removed, prompt)
		}
	}
}

func TestBuildAgentUserPromptUsesCompactValidatedSnapshot(t *testing.T) {
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
					TripID:                 "internal-trip-id",
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

	prompt := buildAgentUserPrompt(session, map[string]interface{}{
		"current_turn_body": "Fraiburgo para moncao 18/05",
		"canonical_state": CanonicalConversationState{
			Phase:              ConversationPhaseRouteSelection,
			AllowedNextActions: []string{string(IntentAvailabilitySearch)},
			Route: CanonicalRouteState{
				Origin:      "Fraiburgo/SC",
				Destination: "Moncao/MA",
				TripDate:    "2026-05-18",
			},
		},
	}, tools)

	snapshot := decodePromptSnapshot(t, prompt)
	if snapshot["current_user_turn"] != "Fraiburgo para moncao 18/05" {
		t.Fatalf("unexpected current turn: %#v", snapshot["current_user_turn"])
	}
	if !strings.Contains(prompt, "last_validated_tool_facts") || !strings.Contains(prompt, "15:00") || !strings.Contains(prompt, "R$ 950.00") {
		t.Fatalf("expected compact tool facts in prompt, got %q", prompt)
	}
	if strings.Contains(prompt, "RESULTADO DE FERRAMENTA") || strings.Contains(prompt, "Use somente o horario") {
		t.Fatalf("expected facts without legacy prose instructions, got %q", prompt)
	}
	if strings.Contains(prompt, "internal-option-id") || strings.Contains(prompt, "internal-trip-id") {
		t.Fatalf("expected compact prompt not to expose internal availability ids, got %q", prompt)
	}
}

func TestBuildAgentUserPromptIncludesRouteContextWithoutOperationalGuardrailProse(t *testing.T) {
	prompt := buildAgentUserPrompt(Session{Channel: "WHATSAPP"}, map[string]interface{}{
		"current_turn_body": "quero passagem para sc\npara Seara",
		"recent_messages": []map[string]interface{}{
			{"direction": "INBOUND", "body": "quero passagem para sc"},
			{"direction": "INBOUND", "body": "para Seara"},
		},
	}, agentToolContext{})

	if !strings.Contains(prompt, `"destination":"Seara/SC"`) {
		t.Fatalf("expected route context destination in compact prompt, got %q", prompt)
	}
	if strings.Contains(prompt, "pergunta correta") || strings.Contains(prompt, "cidade de saida no Maranhao") {
		t.Fatalf("expected route facts without legacy next-step prose, got %q", prompt)
	}
}

func TestBuildAgentUserPromptMentionsCurrentTurnMediaWithoutLegacyDocumentRules(t *testing.T) {
	prompt := buildAgentUserPrompt(Session{Channel: "WHATSAPP"}, map[string]interface{}{
		"current_turn_body":  "",
		"current_turn_kinds": []string{"IMAGE"},
		"current_turn_media": []map[string]interface{}{
			{"kind": "IMAGE", "url": "https://files.example.test/rg.jpg", "mime_type": "image/jpeg"},
		},
		"recent_messages": []map[string]interface{}{
			{"direction": "OUTBOUND", "kind": "TEXT", "body": "Pode enviar a foto legivel do documento."},
			{"direction": "INBOUND", "kind": "IMAGE", "body": ""},
		},
	}, agentToolContext{})

	if !strings.Contains(prompt, `"current_turn_kinds":["IMAGE"]`) || !strings.Contains(prompt, `"current_turn_media_count":1`) {
		t.Fatalf("expected compact media snapshot, got %q", prompt)
	}
	if strings.Contains(prompt, "Primeiro tente extrair") || strings.Contains(prompt, "peca apenas o documento") {
		t.Fatalf("expected document behavior to stay outside prompt prose, got %q", prompt)
	}
}

func TestBuildAgentUserPromptIncludesBookingCreateFactsWithoutPaymentRuleProse(t *testing.T) {
	prompt := buildAgentUserPrompt(Session{Channel: "WHATSAPP"}, map[string]interface{}{
		"current_turn_body": "isso",
	}, agentToolContext{
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
	})

	if !strings.Contains(prompt, `"reservation_code":"ABC12345"`) || !strings.Contains(prompt, `"deposit":"R$ 250.00"`) {
		t.Fatalf("expected booking_create facts, got %q", prompt)
	}
	if strings.Contains(prompt, "BK-ABC123456") {
		t.Fatalf("expected compact prompt not to expose booking id, got %q", prompt)
	}
	if strings.Contains(prompt, "valor integral ou apenas o sinal") || strings.Contains(prompt, "nao perguntar PIX") {
		t.Fatalf("expected payment choice rule to be owned by response templates, got %q", prompt)
	}
}

func decodePromptSnapshot(t *testing.T, prompt string) map[string]interface{} {
	t.Helper()
	const prefix = "SNAPSHOT_VALIDADO_JSON\n"
	if !strings.HasPrefix(prompt, prefix) {
		t.Fatalf("expected prompt prefix %q, got %q", prefix, prompt)
	}
	var snapshot map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(prompt, prefix)), &snapshot); err != nil {
		t.Fatalf("decode prompt snapshot: %v\n%s", err, prompt)
	}
	return snapshot
}
