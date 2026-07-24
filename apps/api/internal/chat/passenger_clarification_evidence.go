package chat

import (
	"encoding/json"
	"strings"
)

const (
	passengerClarificationEventsV1MessageKey = "passenger_clarification_events_v1"
	passengerPendingPromptEventV1MessageKey  = "pending_prompt_event"
	passengerPromptEventV1MessageKey         = "passenger_prompt_event"
)

// passengerClarificationStructuredEventsV1 reads only canonical, structured
// artifacts. Message bodies, transcript position, tool_context and prompt text
// are deliberately not inputs to the passenger state foundation.
func passengerClarificationStructuredEventsV1(messages []Message) []PassengerClarificationEventV1 {
	events := make([]PassengerClarificationEventV1, 0, len(messages))
	for _, message := range messages {
		if strings.EqualFold(strings.TrimSpace(message.Direction), "INBOUND") {
			events = append(events, passengerClarificationEventsV1FromMessage(message)...)
			continue
		}
		if event, ok := passengerClarificationPromptEventFromMessageV1(message); ok {
			events = append(events, event)
		}
	}
	return events
}

func passengerClarificationEventsV1FromMessage(message Message) []PassengerClarificationEventV1 {
	value := message.NormalizedPayload[passengerClarificationEventsV1MessageKey]
	if value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return invalidPassengerClarificationEventsForMessageV1(message.ID)
	}
	var decoded []PassengerClarificationEventV1
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return invalidPassengerClarificationEventsForMessageV1(message.ID)
	}
	events := make([]PassengerClarificationEventV1, 0, len(decoded))
	invalidArtifact := false
	for _, event := range decoded {
		canonical, ok := passengerClarificationEventForMessageV1(event, message.ID)
		if !ok || passengerClarificationEventIsPromptV1(canonical) {
			invalidArtifact = true
			continue
		}
		events = append(events, canonical)
	}
	if invalidArtifact {
		events = append(events, invalidPassengerClarificationEventsForMessageV1(message.ID)...)
	}
	return events
}

func invalidPassengerClarificationEventsForMessageV1(messageID string) []PassengerClarificationEventV1 {
	events := make([]PassengerClarificationEventV1, 0, 2)
	for _, slot := range []PassengerClarificationSlotV1{
		PassengerClarificationSlotPassenger,
		PassengerClarificationSlotChild,
	} {
		event, ok := passengerClarificationEventForMessageV1(PassengerClarificationEventV1{
			Type:       PassengerClarificationEventSlotInvalidated,
			Slot:       slot,
			MessageID:  strings.TrimSpace(messageID),
			ReasonCode: passengerClarificationReasonInvalidState,
		}, messageID)
		if ok {
			events = append(events, event)
		}
	}
	return events
}

func passengerClarificationPromptEventFromMessageV1(message Message) (PassengerClarificationEventV1, bool) {
	if !strings.EqualFold(strings.TrimSpace(message.Direction), "OUTBOUND") ||
		!passengerPromptDeliveryConfirmedV1(message) {
		return PassengerClarificationEventV1{}, false
	}
	event, ok := decodePassengerClarificationEventV1(message.NormalizedPayload[passengerPromptEventV1MessageKey])
	if !ok {
		return PassengerClarificationEventV1{}, false
	}
	canonical, ok := passengerClarificationEventForMessageV1(event, message.ID)
	if !ok || !passengerClarificationEventIsPromptV1(canonical) {
		return PassengerClarificationEventV1{}, false
	}
	return canonical, true
}

func passengerPromptDeliveryConfirmedV1(message Message) bool {
	switch strings.ToUpper(strings.TrimSpace(message.ProcessingStatus)) {
	case "SENT", "DELIVERED", "READ", messageStatusAutomationSent:
	default:
		return false
	}
	return strings.TrimSpace(asString(message.NormalizedPayload["delivery_recorded_at"])) != ""
}

func passengerPendingPromptEventFromDraftV1(draft Message) (PassengerClarificationEventV1, bool) {
	event, ok := decodePassengerClarificationEventV1(draft.NormalizedPayload[passengerPendingPromptEventV1MessageKey])
	if !ok {
		event, ok = decodePassengerClarificationEventV1(draft.Payload[passengerPendingPromptEventV1MessageKey])
	}
	if !ok || !passengerClarificationEventIsPromptV1(event) {
		return PassengerClarificationEventV1{}, false
	}
	return event, true
}

func passengerPromptEventForOutboundV1(draft Message, outboundMessageID string, bodyCompatible bool) (PassengerClarificationEventV1, bool) {
	if !bodyCompatible {
		return PassengerClarificationEventV1{}, false
	}
	event, ok := passengerPendingPromptEventFromDraftV1(draft)
	if !ok {
		return PassengerClarificationEventV1{}, false
	}
	event.MessageID = strings.TrimSpace(outboundMessageID)
	event.PromptMessageID = ""
	event.EventID = ""
	return passengerClarificationEventForMessageV1(event, outboundMessageID)
}

func passengerClarificationEventForMessageV1(event PassengerClarificationEventV1, messageID string) (PassengerClarificationEventV1, bool) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return PassengerClarificationEventV1{}, false
	}
	if event.MessageID != "" && strings.TrimSpace(event.MessageID) != messageID {
		return PassengerClarificationEventV1{}, false
	}
	event.MessageID = messageID
	event.EventID = ""
	event.EventID = passengerClarificationEventIDV1(event)
	if event.EventID == "" || !passengerClarificationEventValidV1(event) {
		return PassengerClarificationEventV1{}, false
	}
	return event, true
}

func decodePassengerClarificationEventV1(value interface{}) (PassengerClarificationEventV1, bool) {
	if value == nil {
		return PassengerClarificationEventV1{}, false
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return PassengerClarificationEventV1{}, false
	}
	var event PassengerClarificationEventV1
	if err := json.Unmarshal(raw, &event); err != nil {
		return PassengerClarificationEventV1{}, false
	}
	return event, true
}

func passengerClarificationPromptEventV1(kind ActivePromptKind, messageID string) (PassengerClarificationEventV1, bool) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return PassengerClarificationEventV1{}, false
	}
	var event PassengerClarificationEventV1
	switch kind {
	case ActivePromptPassengerCount:
		event = PassengerClarificationEventV1{
			Type:       PassengerClarificationEventPassengerPromptOpened,
			Slot:       PassengerClarificationSlotPassenger,
			MessageID:  messageID,
			ReasonCode: passengerClarificationReasonPassengerPromptOpened,
		}
	case ActivePromptLapChildQuestion:
		event = PassengerClarificationEventV1{
			Type:       PassengerClarificationEventChildPromptOpened,
			Slot:       PassengerClarificationSlotChild,
			MessageID:  messageID,
			ReasonCode: passengerClarificationReasonChildPromptOpened,
		}
	default:
		return PassengerClarificationEventV1{}, false
	}
	event.EventID = passengerClarificationEventIDV1(event)
	return event, true
}

func passengerClarificationPromptEventForRunV1(
	run RunAgentResult,
	messageID string,
	_ PassengerClarificationStateV1,
) (PassengerClarificationEventV1, bool) {
	kind, ok := passengerClarificationPromptKindForRunV1(run)
	if !ok {
		return PassengerClarificationEventV1{}, false
	}
	return passengerClarificationPromptEventV1(kind, messageID)
}

func passengerClarificationPromptKindForRunV1(run RunAgentResult) (ActivePromptKind, bool) {
	templateName := ResponseTemplateName(firstNonEmpty(
		asString(run.ResponsePayload["template_name"]),
		asString(run.RequestPayload["template_name"]),
	))
	action := strings.TrimSpace(firstNonEmpty(
		asString(run.ResponsePayload["action"]),
		asString(run.RequestPayload["action"]),
	))
	switch templateName {
	case TemplateAskPassengerCount:
		if action != "template" && action != string(BookingNextAskPassengerClarification) {
			return ActivePromptUnknown, false
		}
		return ActivePromptPassengerCount, true
	case TemplateContextFallbackPassengerCount:
		if action != "template" {
			return ActivePromptUnknown, false
		}
		return ActivePromptPassengerCount, true
	case TemplateAskChildUnder5:
		if action != string(BookingNextAskPassengerClarification) {
			return ActivePromptUnknown, false
		}
		return ActivePromptLapChildQuestion, true
	case TemplateContextFallbackChildUnder5:
		if action != "template" {
			return ActivePromptUnknown, false
		}
		return ActivePromptLapChildQuestion, true
	default:
		return ActivePromptUnknown, false
	}
}

func passengerClarificationEventIsPromptV1(event PassengerClarificationEventV1) bool {
	return (event.Type == PassengerClarificationEventPassengerPromptOpened && event.Slot == PassengerClarificationSlotPassenger) ||
		(event.Type == PassengerClarificationEventChildPromptOpened && event.Slot == PassengerClarificationSlotChild)
}

func passengerClarificationEventValidV1(event PassengerClarificationEventV1) bool {
	switch event.Type {
	case PassengerClarificationEventPassengerPromptOpened:
		return event.Slot == PassengerClarificationSlotPassenger
	case PassengerClarificationEventChildPromptOpened:
		return event.Slot == PassengerClarificationSlotChild
	case PassengerClarificationEventPassengerCountSet:
		return event.Slot == PassengerClarificationSlotPassenger && event.ValueKnown && event.Value > 0 &&
			passengerCountProvenanceActionableV1(event.PassengerProvenance)
	case PassengerClarificationEventChildCountSet:
		return event.Slot == PassengerClarificationSlotChild && event.ValueKnown && event.Value >= 0
	case PassengerClarificationEventSlotCorrected:
		if event.Slot == PassengerClarificationSlotPassenger {
			return event.ValueKnown && event.Value > 0 &&
				passengerCountProvenanceActionableV1(event.PassengerProvenance)
		}
		return event.Slot == PassengerClarificationSlotChild && event.ValueKnown && event.Value >= 0
	case PassengerClarificationEventSlotInvalidated:
		return event.Slot == PassengerClarificationSlotPassenger || event.Slot == PassengerClarificationSlotChild
	default:
		return false
	}
}
