package chat

func openAIPassengerClarificationMeaningV1JSONSchema() map[string]interface{} {
	stringEnum := func(values ...string) map[string]interface{} {
		return map[string]interface{}{"type": "string", "enum": values}
	}
	nullableInteger := func(minimum int, maximum int) map[string]interface{} {
		return map[string]interface{}{
			"anyOf": []interface{}{
				map[string]interface{}{"type": "integer", "minimum": minimum, "maximum": maximum},
				map[string]interface{}{"type": "null"},
			},
		}
	}
	nullableBoolean := map[string]interface{}{
		"anyOf": []interface{}{
			map[string]interface{}{"type": "boolean"},
			map[string]interface{}{"type": "null"},
		},
	}
	strictObject := func(properties map[string]interface{}, required ...string) map[string]interface{} {
		return map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
			"properties":           properties,
			"required":             required,
		}
	}

	passengerCount := strictObject(map[string]interface{}{
		"status": stringEnum(passengerMeaningV1StatusEnum()...),
		"value":  nullableInteger(1, 99),
		"provenance": stringEnum(
			string(PassengerCountProvenanceUnknown),
			string(PassengerCountProvenanceSoloSpeaker),
			string(PassengerCountProvenanceAbsoluteTotal),
			string(PassengerCountProvenanceIncludesSpeakerComposition),
			string(PassengerCountProvenanceSubgroupOnly),
		),
	}, "status", "value", "provenance")

	childReference := strictObject(map[string]interface{}{
		"reference_id": map[string]interface{}{"type": "string", "minLength": 1, "maxLength": 64},
		"relation": stringEnum(
			string(PassengerChildRelationChild),
			string(PassengerChildRelationOther),
			string(PassengerChildRelationUnknown),
		),
		"age_value": nullableInteger(0, 1440),
		"age_unit": stringEnum(
			string(PassengerChildAgeUnitYears),
			string(PassengerChildAgeUnitMonths),
			string(PassengerChildAgeUnitUnknown),
		),
		"under_5": nullableBoolean,
	}, "reference_id", "relation", "age_value", "age_unit", "under_5")

	childUnder5 := strictObject(map[string]interface{}{
		"status":     stringEnum(passengerMeaningV1StatusEnum()...),
		"count":      nullableInteger(0, 99),
		"references": map[string]interface{}{"type": "array", "maxItems": 16, "items": childReference},
	}, "status", "count", "references")

	correction := strictObject(map[string]interface{}{
		"present": map[string]interface{}{"type": "boolean"},
		"replaces": stringEnum(
			string(PassengerMeaningCorrectionNone),
			string(PassengerMeaningCorrectionPassengerAggregate),
			string(PassengerMeaningCorrectionChildAggregate),
			string(PassengerMeaningCorrectionFullAggregate),
		),
	}, "present", "replaces")

	responseSchema := strictObject(map[string]interface{}{
		"version":                map[string]interface{}{"type": "integer", "enum": []int{passengerClarificationMeaningV1Version}},
		"source_message_id":      map[string]interface{}{"type": "string", "minLength": 1, "maxLength": 128},
		"source_prompt_event_id": map[string]interface{}{"type": "string", "minLength": 1, "maxLength": 256},
		"passenger_count":        passengerCount,
		"child_under_5":          childUnder5,
		"correction":             correction,
		"needs_clarification":    map[string]interface{}{"type": "boolean"},
		"missing_fields": map[string]interface{}{
			"type":     "array",
			"maxItems": 4,
			"items": stringEnum(
				string(PassengerMeaningMissingPassengerCount),
				string(PassengerMeaningMissingChildUnder5),
				string(PassengerMeaningMissingChildAges),
				string(PassengerMeaningMissingCorrection),
			),
		},
		"confidence": map[string]interface{}{"type": "number", "minimum": 0, "maximum": 1},
		"reason_codes": map[string]interface{}{
			"type":     "array",
			"minItems": 1,
			"maxItems": 16,
			"items":    stringEnum(passengerMeaningV1ReasonCodeEnum()...),
		},
	},
		"version",
		"source_message_id",
		"source_prompt_event_id",
		"passenger_count",
		"child_under_5",
		"correction",
		"needs_clarification",
		"missing_fields",
		"confidence",
		"reason_codes",
	)

	return map[string]interface{}{
		"type":   "json_schema",
		"name":   "passenger_clarification_meaning_v1",
		"strict": true,
		"schema": responseSchema,
	}
}

func passengerMeaningV1StatusEnum() []string {
	return []string{
		string(PassengerMeaningStatusKnown),
		string(PassengerMeaningStatusUnknown),
		string(PassengerMeaningStatusConflicting),
	}
}

func passengerMeaningV1ReasonCodeEnum() []string {
	return []string{
		string(PassengerMeaningReasonPassengerCountStated),
		string(PassengerMeaningReasonIncludesSpeakerComposition),
		string(PassengerMeaningReasonPassengerSubgroupOnly),
		string(PassengerMeaningReasonChildAgeStated),
		string(PassengerMeaningReasonChildUnder5Stated),
		string(PassengerMeaningReasonChildUnder5NotStated),
		string(PassengerMeaningReasonCorrectionStated),
		string(PassengerMeaningReasonAmbiguousReference),
		string(PassengerMeaningReasonConflictingValues),
		string(PassengerMeaningReasonMissingAge),
		string(PassengerMeaningReasonUnrelatedNumberIgnored),
	}
}
