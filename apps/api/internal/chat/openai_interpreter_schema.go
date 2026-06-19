package chat

type OpenAIStructuredInterpretationJSON struct {
	Intent            StructuredIntent                           `json:"intent"`
	TurnMeaning       TurnMeaning                                `json:"turn_meaning"`
	Confidence        float64                                    `json:"confidence"`
	Booking           OpenAIStructuredBookingSlotsJSON           `json:"booking"`
	PassengerDocument OpenAIStructuredPassengerDocumentSlotsJSON `json:"passenger_document"`
	Payment           OpenAIStructuredPaymentSlotsJSON           `json:"payment"`
	Safety            OpenAIStructuredInterpretationSafetyJSON   `json:"safety"`
	Reasons           []string                                   `json:"reasons"`
}

type OpenAIStructuredBookingSlotsJSON struct {
	SelectedOptionIndex      int   `json:"selected_option_index"`
	SelectedOptionIndexKnown bool  `json:"selected_option_index_known"`
	PassengerCount           int   `json:"passenger_count"`
	PassengerCountKnown      bool  `json:"passenger_count_known"`
	ChildUnder5Count         int   `json:"child_under_5_count"`
	ChildUnder5CountKnown    bool  `json:"child_under_5_count_known"`
	LapChildPassengerIndexes []int `json:"lap_child_passenger_indexes"`
}

type OpenAIStructuredPassengerDocumentSlotsJSON struct {
	DocumentLikeText bool `json:"document_like_text"`
}

type OpenAIStructuredPaymentSlotsJSON struct {
	PaymentPreference string `json:"payment_preference"`
}

type OpenAIStructuredInterpretationSafetyJSON struct {
	ExecutesTool          bool `json:"executes_tool"`
	MutatesState          bool `json:"mutates_state"`
	SendsMessage          bool `json:"sends_message"`
	CallsExternalProvider bool `json:"calls_external_provider"`
}

func openAIStructuredInterpretationJSONSchema() map[string]interface{} {
	stringEnum := func(values ...string) map[string]interface{} {
		return map[string]interface{}{"type": "string", "enum": values}
	}
	integer := map[string]interface{}{"type": "integer", "minimum": 0}
	boolean := map[string]interface{}{"type": "boolean"}

	bookingProperties := map[string]interface{}{
		"selected_option_index":       integer,
		"selected_option_index_known": boolean,
		"passenger_count":             integer,
		"passenger_count_known":       boolean,
		"child_under_5_count":         integer,
		"child_under_5_count_known":   boolean,
		"lap_child_passenger_indexes": map[string]interface{}{
			"type":  "array",
			"items": map[string]interface{}{"type": "integer", "minimum": 1},
		},
	}
	passengerDocumentProperties := map[string]interface{}{
		"document_like_text": boolean,
	}
	paymentProperties := map[string]interface{}{
		"payment_preference": stringEnum("", "sinal", "integral"),
	}
	safetyProperties := map[string]interface{}{
		"executes_tool":           boolean,
		"mutates_state":           boolean,
		"sends_message":           boolean,
		"calls_external_provider": boolean,
	}

	responseSchema := map[string]interface{}{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]interface{}{
			"intent":       stringEnum(openAIStructuredIntentEnum()...),
			"turn_meaning": stringEnum(openAITurnMeaningEnum()...),
			"confidence": map[string]interface{}{
				"type":    "number",
				"minimum": 0,
				"maximum": 1,
			},
			"booking": map[string]interface{}{
				"type":                 "object",
				"additionalProperties": false,
				"properties":           bookingProperties,
				"required": []string{
					"selected_option_index",
					"selected_option_index_known",
					"passenger_count",
					"passenger_count_known",
					"child_under_5_count",
					"child_under_5_count_known",
					"lap_child_passenger_indexes",
				},
			},
			"passenger_document": map[string]interface{}{
				"type":                 "object",
				"additionalProperties": false,
				"properties":           passengerDocumentProperties,
				"required":             []string{"document_like_text"},
			},
			"payment": map[string]interface{}{
				"type":                 "object",
				"additionalProperties": false,
				"properties":           paymentProperties,
				"required":             []string{"payment_preference"},
			},
			"safety": map[string]interface{}{
				"type":                 "object",
				"additionalProperties": false,
				"properties":           safetyProperties,
				"required": []string{
					"executes_tool",
					"mutates_state",
					"sends_message",
					"calls_external_provider",
				},
			},
			"reasons": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
		},
		"required": []string{
			"intent",
			"turn_meaning",
			"confidence",
			"booking",
			"passenger_document",
			"payment",
			"safety",
			"reasons",
		},
	}

	return map[string]interface{}{
		"type":   "json_schema",
		"name":   "openai_structured_interpretation",
		"strict": true,
		"schema": responseSchema,
	}
}

func openAIStructuredIntentEnum() []string {
	return []string{
		string(StructuredIntentUnknown),
		string(StructuredIntentGreeting),
		string(StructuredIntentAvailabilitySearch),
		string(StructuredIntentSelectAvailabilityOption),
		string(StructuredIntentPassengerCountReply),
		string(StructuredIntentLapChildAssignmentAnswer),
		string(StructuredIntentPassengerDocumentsProvided),
		string(StructuredIntentDocumentConfirmation),
		string(StructuredIntentPaymentPreference),
		string(StructuredIntentBookingCancelRequest),
	}
}

func openAITurnMeaningEnum() []string {
	return []string{
		string(TurnMeaningUnknown),
		string(TurnMeaningGreeting),
		string(TurnMeaningNewRequest),
		string(TurnMeaningAnswerToQuestion),
		string(TurnMeaningConfirmation),
	}
}
