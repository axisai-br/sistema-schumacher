package chat

import (
	"encoding/json"
	"strings"
)

type OpenAIPassengerMeaningV1CompactInput struct {
	CurrentTurn         string                               `json:"current_turn"`
	SourceMessageID     string                               `json:"source_message_id"`
	SourcePromptEventID string                               `json:"source_prompt_event_id"`
	State               OpenAIPassengerMeaningV1State        `json:"state"`
	ActivePrompt        OpenAIPassengerMeaningV1ActivePrompt `json:"active_prompt"`
}

type OpenAIPassengerMeaningV1State struct {
	Version                  int                                      `json:"version"`
	Authority                string                                   `json:"authority"`
	PassengerCountKnown      bool                                     `json:"passenger_count_known"`
	PassengerCount           int                                      `json:"passenger_count"`
	PassengerCountProvenance string                                   `json:"passenger_count_provenance"`
	PassengerSlotStatus      string                                   `json:"passenger_slot_status"`
	ChildUnder5CountKnown    bool                                     `json:"child_under_5_count_known"`
	ChildUnder5Count         int                                      `json:"child_under_5_count"`
	ChildSlotStatus          string                                   `json:"child_slot_status"`
	ChildReferences          []OpenAIPassengerMeaningV1StateReference `json:"child_references"`
}

type OpenAIPassengerMeaningV1StateReference struct {
	ReferenceID string `json:"reference_id"`
	Under5      bool   `json:"under_5"`
	AgeKnown    bool   `json:"age_known"`
}

type OpenAIPassengerMeaningV1ActivePrompt struct {
	Type      string `json:"type"`
	Slot      string `json:"slot"`
	MessageID string `json:"message_id"`
	EventID   string `json:"event_id"`
}

func buildOpenAIPassengerMeaningV1SystemPrompt() string {
	return strings.TrimSpace(`
You are a strict semantic interpreter for a passenger clarification turn in Brazilian Portuguese.
Return only the PassengerClarificationMeaningV1 JSON object required by the supplied schema.

Safety and scope:
- Interpret meaning only. Never answer the customer, call a tool, create an action/event, mutate state, book, pay, quote a price, or handle documents.
- Copy version=1, source_message_id, and source_prompt_event_id exactly from the structured input.
- Use only the current turn plus the supplied structured passenger state and active prompt. Do not invent prior messages.
- Passenger count means the total travelers including the speaker. "eu e mais 2 crianças" and "eu e mais duas crianças" mean KNOWN=3 with INCLUDES_SPEAKER_COMPOSITION.
- A subgroup alone (for example, only a count of children) is not an absolute passenger total.
- A child/family relationship alone does not establish age or under-five count. Set child_under_5 UNKNOWN unless an explicit age or explicit under-five fact is sufficient.
- Ages under five are 0-4 years or 0-59 months. Keep distinct referenced children distinct; use short opaque reference IDs, never names.
- Respect explicit corrections. Mark conflicts and clarification needs rather than guessing.
- Ignore unrelated numbers such as dates, option indexes, prices, phone numbers, and documents.
- reason_codes, missing_fields, enum values, and all nullable values must remain within the schema.
`)
}

func buildOpenAIPassengerMeaningV1CompactInput(input OpenAIPassengerMeaningV1RunInput) string {
	compact := OpenAIPassengerMeaningV1CompactInput{
		CurrentTurn:         redactOpenAITravelQueryV2SensitiveText(input.CurrentTurn),
		SourceMessageID:     strings.TrimSpace(input.SourceMessageID),
		SourcePromptEventID: strings.TrimSpace(input.SourcePromptEventID),
		State: OpenAIPassengerMeaningV1State{
			Version:                  input.State.Version,
			Authority:                string(input.State.Authority),
			PassengerCountKnown:      input.State.PassengerCountKnown,
			PassengerCount:           input.State.PassengerCount,
			PassengerCountProvenance: string(input.State.PassengerCountProvenance),
			PassengerSlotStatus:      string(input.State.PassengerSlotStatus),
			ChildUnder5CountKnown:    input.State.ChildUnder5CountKnown,
			ChildUnder5Count:         input.State.ChildUnder5Count,
			ChildSlotStatus:          string(input.State.ChildSlotStatus),
			ChildReferences:          summarizeOpenAIPassengerMeaningV1StateReferences(input.State.ChildReferences),
		},
		ActivePrompt: OpenAIPassengerMeaningV1ActivePrompt{
			Type:      string(input.PromptEvent.Type),
			Slot:      string(input.PromptEvent.Slot),
			MessageID: strings.TrimSpace(input.PromptEvent.MessageID),
			EventID:   strings.TrimSpace(input.PromptEvent.EventID),
		},
	}
	data, err := json.Marshal(compact)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func summarizeOpenAIPassengerMeaningV1StateReferences(references []PassengerClarificationChildReferenceV1) []OpenAIPassengerMeaningV1StateReference {
	out := make([]OpenAIPassengerMeaningV1StateReference, 0, passengerMeaningV1MinInt(len(references), 16))
	for _, reference := range references {
		id := strings.TrimSpace(reference.ID)
		if !passengerMeaningOpaqueReferenceIDValidV1(id) {
			continue
		}
		out = append(out, OpenAIPassengerMeaningV1StateReference{
			ReferenceID: id,
			Under5:      reference.Under5,
			AgeKnown:    reference.AgeKnown,
		})
		if len(out) == 16 {
			break
		}
	}
	return out
}
