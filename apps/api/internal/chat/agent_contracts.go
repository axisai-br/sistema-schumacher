package chat

import (
	"encoding/json"
	"strings"
)

const (
	jsonDecisionDomainGeneral    = "general"
	jsonDecisionDomainScheduling = "scheduling"
	jsonDecisionDomainPayments   = "payments"
	jsonDecisionDomainHandoff    = "handoff"

	jsonDecisionActionTemplate   = "template"
	jsonDecisionActionTool       = "tool"
	jsonDecisionActionSpecialist = "specialist"
	jsonDecisionActionClarify    = "clarify"
	jsonDecisionActionHandoff    = "handoff"
	jsonDecisionActionNoop       = "noop"
)

type IntentDecisionJSON struct {
	Domain              string                `json:"domain"`
	Intent              string                `json:"intent"`
	Action              string                `json:"action"`
	Confidence          float64               `json:"confidence"`
	MissingFields       []string              `json:"missing_fields"`
	SafeNextStep        string                `json:"safe_next_step"`
	SelectedOptionIndex *int                  `json:"selected_option_index"`
	AvailabilityInput   *SchedulingActionPlan `json:"availability_input"`
	PaymentInput        *PaymentActionPlan    `json:"payment_input"`
	BookingInput        *GeneralActionPlan    `json:"booking_input"`
}

type GeneralActionPlan struct {
	TemplateName string   `json:"template_name"`
	ReasonCode   string   `json:"reason_code"`
	Fields       []string `json:"fields"`
}

type SchedulingActionPlan struct {
	ToolName    string `json:"tool_name"`
	Origin      string `json:"origin"`
	Destination string `json:"destination"`
	PackageName string `json:"package_name"`
	TripDate    string `json:"trip_date"`
	Qty         int    `json:"qty"`
	Limit       int    `json:"limit"`
}

type PaymentActionPlan struct {
	ToolName         string  `json:"tool_name"`
	BookingID        string  `json:"booking_id"`
	ReservationCode  string  `json:"reservation_code"`
	PaymentMethod    string  `json:"payment_method"`
	CustomerDocument string  `json:"customer_document"`
	Amount           float64 `json:"amount"`
}

type CustomerReplyPlan struct {
	TemplateName string   `json:"template_name"`
	Slots        []string `json:"slots"`
}

type CanonicalConversationStateSnapshot struct {
	Phase                 string   `json:"phase"`
	AllowedNextActions    []string `json:"allowed_next_actions"`
	Origin                string   `json:"origin,omitempty"`
	Destination           string   `json:"destination,omitempty"`
	PackageName           string   `json:"package_name,omitempty"`
	TripDate              string   `json:"trip_date,omitempty"`
	SelectedOptionIndex   int      `json:"selected_option_index,omitempty"`
	TripID                string   `json:"trip_id,omitempty"`
	PassengerCount        int      `json:"passenger_count,omitempty"`
	ChildUnder5Count      int      `json:"child_under_5_count,omitempty"`
	DocumentsCollected    bool     `json:"documents_collected,omitempty"`
	BookingID             string   `json:"booking_id,omitempty"`
	ReservationCode       string   `json:"reservation_code,omitempty"`
	BookingStatus         string   `json:"booking_status,omitempty"`
	PaymentStatus         string   `json:"payment_status,omitempty"`
	HasAvailabilityFacts  bool     `json:"has_availability_facts"`
	HasBookingFacts       bool     `json:"has_booking_facts"`
	HasPaymentStatusFacts bool     `json:"has_payment_status_facts"`
	HasPaymentCreateFacts bool     `json:"has_payment_create_facts"`
}

type JSONDecisionCompactInput struct {
	CurrentTurn string                             `json:"current_turn"`
	State       CanonicalConversationStateSnapshot `json:"state"`
	RecentTurns []string                           `json:"recent_turns,omitempty"`
}

func buildJSONDecisionSystemPrompt() string {
	return strings.Join([]string{
		"You are a backend routing classifier for Schumacher Tur.",
		"Return only JSON matching the provided schema.",
		"Do not write customer-facing prose.",
		"Do not claim dates, prices, seats, bookings, cancellations, or payment status.",
		"Select tool actions only when the required backend input is present.",
	}, "\n")
}

func buildJSONDecisionCompactInput(currentTurn string, state CanonicalConversationState, history []Message) string {
	input := JSONDecisionCompactInput{
		CurrentTurn: strings.TrimSpace(currentTurn),
		State:       snapshotCanonicalConversationState(state),
		RecentTurns: recentMessageBodies(history, 6),
	}
	data, err := json.Marshal(input)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func snapshotCanonicalConversationState(state CanonicalConversationState) CanonicalConversationStateSnapshot {
	facts := state.LastToolFacts
	return CanonicalConversationStateSnapshot{
		Phase:                 string(state.Phase),
		AllowedNextActions:    append([]string(nil), state.AllowedNextActions...),
		Origin:                state.Route.Origin,
		Destination:           state.Route.Destination,
		PackageName:           state.Route.PackageName,
		TripDate:              state.Route.TripDate,
		SelectedOptionIndex:   state.Route.SelectedOptionIndex,
		TripID:                state.Route.TripID,
		PassengerCount:        state.Passengers.ExpectedCount,
		ChildUnder5Count:      state.Passengers.ChildUnder5Count,
		DocumentsCollected:    state.Passengers.DocumentsCollected,
		BookingID:             state.Booking.BookingID,
		ReservationCode:       state.Booking.ReservationCode,
		BookingStatus:         state.Booking.Status,
		PaymentStatus:         state.Payment.Status,
		HasAvailabilityFacts:  len(asMap(facts[toolNameAvailabilitySearch])) > 0,
		HasBookingFacts:       len(asMap(facts[toolNameBookingCreate])) > 0 || strings.TrimSpace(state.Booking.BookingID) != "",
		HasPaymentStatusFacts: len(asMap(facts[toolNamePaymentStatus])) > 0,
		HasPaymentCreateFacts: len(asMap(facts[toolNamePaymentCreate])) > 0,
	}
}

func recentMessageBodies(history []Message, limit int) []string {
	if limit <= 0 || len(history) == 0 {
		return nil
	}
	start := len(history) - limit
	if start < 0 {
		start = 0
	}
	out := make([]string, 0, len(history)-start)
	for _, message := range history[start:] {
		body := strings.TrimSpace(message.Body)
		if body == "" {
			continue
		}
		out = append(out, strings.ToUpper(strings.TrimSpace(message.Direction))+": "+body)
	}
	return out
}

func intentDecisionJSONSchema() map[string]interface{} {
	stringEnum := func(values ...string) map[string]interface{} {
		return map[string]interface{}{"type": "string", "enum": values}
	}
	nullableInteger := map[string]interface{}{
		"anyOf": []interface{}{
			map[string]interface{}{"type": "integer"},
			map[string]interface{}{"type": "null"},
		},
	}
	nullableObject := func(properties map[string]interface{}, required []string) map[string]interface{} {
		return map[string]interface{}{
			"anyOf": []interface{}{
				map[string]interface{}{
					"type":                 "object",
					"additionalProperties": false,
					"properties":           properties,
					"required":             required,
				},
				map[string]interface{}{"type": "null"},
			},
		}
	}
	generalProperties := map[string]interface{}{
		"template_name": map[string]interface{}{"type": "string"},
		"reason_code":   map[string]interface{}{"type": "string"},
		"fields": map[string]interface{}{
			"type":  "array",
			"items": map[string]interface{}{"type": "string"},
		},
	}
	schedulingProperties := map[string]interface{}{
		"tool_name":    map[string]interface{}{"type": "string"},
		"origin":       map[string]interface{}{"type": "string"},
		"destination":  map[string]interface{}{"type": "string"},
		"package_name": map[string]interface{}{"type": "string"},
		"trip_date":    map[string]interface{}{"type": "string"},
		"qty":          map[string]interface{}{"type": "integer"},
		"limit":        map[string]interface{}{"type": "integer"},
	}
	paymentProperties := map[string]interface{}{
		"tool_name":         map[string]interface{}{"type": "string"},
		"booking_id":        map[string]interface{}{"type": "string"},
		"reservation_code":  map[string]interface{}{"type": "string"},
		"payment_method":    map[string]interface{}{"type": "string"},
		"customer_document": map[string]interface{}{"type": "string"},
		"amount":            map[string]interface{}{"type": "number"},
	}
	requiredGeneral := []string{"template_name", "reason_code", "fields"}
	requiredScheduling := []string{"tool_name", "origin", "destination", "package_name", "trip_date", "qty", "limit"}
	requiredPayment := []string{"tool_name", "booking_id", "reservation_code", "payment_method", "customer_document", "amount"}
	return map[string]interface{}{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]interface{}{
			"domain":                stringEnum(jsonDecisionDomainGeneral, jsonDecisionDomainScheduling, jsonDecisionDomainPayments, jsonDecisionDomainHandoff),
			"intent":                map[string]interface{}{"type": "string"},
			"action":                stringEnum(jsonDecisionActionTemplate, jsonDecisionActionTool, jsonDecisionActionSpecialist, jsonDecisionActionClarify, jsonDecisionActionHandoff, jsonDecisionActionNoop),
			"confidence":            map[string]interface{}{"type": "number", "minimum": 0, "maximum": 1},
			"missing_fields":        map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
			"safe_next_step":        map[string]interface{}{"type": "string"},
			"selected_option_index": nullableInteger,
			"availability_input":    nullableObject(schedulingProperties, requiredScheduling),
			"payment_input":         nullableObject(paymentProperties, requiredPayment),
			"booking_input":         nullableObject(generalProperties, requiredGeneral),
		},
		"required": []string{
			"domain",
			"intent",
			"action",
			"confidence",
			"missing_fields",
			"safe_next_step",
			"selected_option_index",
			"availability_input",
			"payment_input",
			"booking_input",
		},
	}
}
