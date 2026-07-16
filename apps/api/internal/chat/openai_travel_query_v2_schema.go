package chat

type OpenAITravelQueryMeaningV2JSON struct {
	Intent             TravelQueryIntent                      `json:"intent"`
	TurnMeaning        TurnMeaning                            `json:"turn_meaning"`
	Origin             *OpenAITravelQueryLocationV2JSON       `json:"origin"`
	Destination        *OpenAITravelQueryLocationV2JSON       `json:"destination"`
	MentionedLocations []OpenAITravelQueryLocationV2JSON      `json:"mentioned_locations"`
	DatePreference     OpenAITravelQueryDatePreferenceV2JSON  `json:"date_preference"`
	OptionReference    OpenAITravelQueryOptionReferenceV2JSON `json:"option_reference"`
	RouteCoverage      OpenAITravelQueryRouteCoverageV2JSON   `json:"route_coverage"`
	SeatRequest        SeatRequestMode                        `json:"seat_request"`
	InstitutionalTopic InstitutionalTopic                     `json:"institutional_topic"`
	NeedsClarification bool                                   `json:"needs_clarification"`
	MissingFields      []string                               `json:"missing_fields"`
	Confidence         float64                                `json:"confidence"`
	Reasons            []string                               `json:"reasons"`
}

type OpenAITravelQueryLocationV2JSON struct {
	Name string       `json:"name"`
	Role LocationRole `json:"role"`
}

type OpenAITravelQueryDatePreferenceV2JSON struct {
	Mode      DateMode `json:"mode"`
	ExactDate string   `json:"exact_date"`
}

type OpenAITravelQueryOptionReferenceV2JSON struct {
	Kind  OptionReferenceKind `json:"kind"`
	Index int                 `json:"index"`
	Date  string              `json:"date"`
}

type OpenAITravelQueryRouteCoverageV2JSON struct {
	QueryLocation string            `json:"query_location"`
	Mode          RouteCoverageMode `json:"mode"`
}

func openAITravelQueryMeaningV2JSONSchema() map[string]interface{} {
	stringEnum := func(values ...string) map[string]interface{} {
		return map[string]interface{}{"type": "string", "enum": values}
	}
	strictObject := func(properties map[string]interface{}, required ...string) map[string]interface{} {
		return map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
			"properties":           properties,
			"required":             required,
		}
	}
	location := strictObject(map[string]interface{}{
		"name": map[string]interface{}{"type": "string"},
		"role": stringEnum(openAITravelQueryLocationRoleV2Enum()...),
	}, "name", "role")
	nullableLocation := map[string]interface{}{
		"anyOf": []interface{}{location, map[string]interface{}{"type": "null"}},
	}
	datePreference := strictObject(map[string]interface{}{
		"mode":       stringEnum(openAITravelQueryDateModeV2Enum()...),
		"exact_date": map[string]interface{}{"type": "string"},
	}, "mode", "exact_date")
	optionReference := strictObject(map[string]interface{}{
		"kind":  stringEnum(openAITravelQueryOptionReferenceKindV2Enum()...),
		"index": map[string]interface{}{"type": "integer", "minimum": 0},
		"date":  map[string]interface{}{"type": "string"},
	}, "kind", "index", "date")
	routeCoverage := strictObject(map[string]interface{}{
		"query_location": map[string]interface{}{"type": "string"},
		"mode":           stringEnum(openAITravelQueryRouteCoverageModeV2Enum()...),
	}, "query_location", "mode")

	responseSchema := strictObject(map[string]interface{}{
		"intent":              stringEnum(openAITravelQueryIntentV2Enum()...),
		"turn_meaning":        stringEnum(openAITravelQueryTurnMeaningV2Enum()...),
		"origin":              nullableLocation,
		"destination":         nullableLocation,
		"mentioned_locations": map[string]interface{}{"type": "array", "items": location},
		"date_preference":     datePreference,
		"option_reference":    optionReference,
		"route_coverage":      routeCoverage,
		"seat_request":        stringEnum(openAITravelQuerySeatRequestModeV2Enum()...),
		"institutional_topic": stringEnum(openAITravelQueryInstitutionalTopicV2Enum()...),
		"needs_clarification": map[string]interface{}{"type": "boolean"},
		"missing_fields":      map[string]interface{}{"type": "array", "items": stringEnum(openAITravelQueryMissingFieldV2Enum()...)},
		"confidence":          map[string]interface{}{"type": "number", "minimum": 0, "maximum": 1},
		"reasons":             map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
	},
		"intent",
		"turn_meaning",
		"origin",
		"destination",
		"mentioned_locations",
		"date_preference",
		"option_reference",
		"route_coverage",
		"seat_request",
		"institutional_topic",
		"needs_clarification",
		"missing_fields",
		"confidence",
		"reasons",
	)

	return map[string]interface{}{
		"type":   "json_schema",
		"name":   "travel_query_meaning_v2",
		"strict": true,
		"schema": responseSchema,
	}
}

func openAITravelQueryIntentV2Enum() []string {
	return []string{
		string(TravelQueryIntentUnknown),
		string(TravelQueryIntentAvailabilitySearch),
		string(TravelQueryIntentSelectAvailabilityOption),
		string(TravelQueryIntentRouteCoverage),
		string(TravelQueryIntentSeatRequest),
		string(TravelQueryIntentInstitutionalQuestion),
	}
}

func openAITravelQueryTurnMeaningV2Enum() []string {
	return []string{
		string(TurnMeaningUnknown),
		string(TurnMeaningGreeting),
		string(TurnMeaningNewRequest),
		string(TurnMeaningAnswerToQuestion),
		string(TurnMeaningConfirmation),
		string(TurnMeaningAcknowledgement),
	}
}

func openAITravelQueryLocationRoleV2Enum() []string {
	return []string{
		string(LocationRoleUnknown),
		string(LocationRoleOrigin),
		string(LocationRoleDestination),
		string(LocationRoleVia),
		string(LocationRoleNearbyReference),
	}
}

func openAITravelQueryDateModeV2Enum() []string {
	return []string{
		string(DateModeUnspecified),
		string(DateModeExact),
		string(DateModeEarliestAvailable),
		string(DateModeAnyAvailable),
	}
}

func openAITravelQueryOptionReferenceKindV2Enum() []string {
	return []string{
		string(OptionReferenceNone),
		string(OptionReferenceIndex),
		string(OptionReferenceDate),
		string(OptionReferenceDeictic),
	}
}

func openAITravelQueryRouteCoverageModeV2Enum() []string {
	return []string{
		string(RouteCoverageModeUnspecified),
		string(RouteCoverageModeExactStop),
		string(RouteCoverageModeNearbyReference),
	}
}

func openAITravelQuerySeatRequestModeV2Enum() []string {
	return []string{
		string(SeatRequestNone),
		string(SeatRequestBookTravel),
		string(SeatRequestChooseSpecificSeat),
	}
}

func openAITravelQueryInstitutionalTopicV2Enum() []string {
	return []string{
		string(InstitutionalTopicNone),
		string(InstitutionalTopicCompanyInfo),
		string(InstitutionalTopicContact),
		string(InstitutionalTopicAddress),
		string(InstitutionalTopicBusinessHours),
	}
}

func openAITravelQueryMissingFieldV2Enum() []string {
	return []string{
		"origin",
		"destination",
		"date",
		"option_reference",
		"route_coverage",
		"seat_request",
		"institutional_topic",
	}
}
