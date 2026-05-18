package chat

func specialistActionPlanSchema(_ string, allowedTools []string) map[string]interface{} {
	toolEnum := make([]interface{}, 0, len(allowedTools))
	for _, tool := range allowedTools {
		toolEnum = append(toolEnum, tool)
	}
	toolRequest := map[string]interface{}{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]interface{}{
			"tool_name":         map[string]interface{}{"type": "string", "enum": toolEnum},
			"reason_code":       map[string]interface{}{"type": "string"},
			"origin":            map[string]interface{}{"type": "string"},
			"destination":       map[string]interface{}{"type": "string"},
			"package_name":      map[string]interface{}{"type": "string"},
			"trip_date":         map[string]interface{}{"type": "string"},
			"qty":               map[string]interface{}{"type": "integer"},
			"limit":             map[string]interface{}{"type": "integer"},
			"booking_id":        map[string]interface{}{"type": "string"},
			"reservation_code":  map[string]interface{}{"type": "string"},
			"payment_method":    map[string]interface{}{"type": "string"},
			"customer_document": map[string]interface{}{"type": "string"},
			"amount":            map[string]interface{}{"type": "number"},
		},
		"required": []string{
			"tool_name",
			"reason_code",
			"origin",
			"destination",
			"package_name",
			"trip_date",
			"qty",
			"limit",
			"booking_id",
			"reservation_code",
			"payment_method",
			"customer_document",
			"amount",
		},
	}
	replyPlan := map[string]interface{}{
		"anyOf": []interface{}{
			map[string]interface{}{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]interface{}{
					"template_name": map[string]interface{}{"type": "string"},
					"slots":         map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
					"message":       map[string]interface{}{"type": "string"},
					"question":      map[string]interface{}{"type": "string"},
				},
				"required": []string{"template_name", "slots", "message", "question"},
			},
			map[string]interface{}{"type": "null"},
		},
	}
	return map[string]interface{}{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type": "string",
				"enum": []string{specialistPlanActionTool, specialistPlanActionTemplate, specialistPlanActionClarify, specialistPlanActionNoop},
			},
			"template_name":        map[string]interface{}{"type": "string"},
			"reason_code":          map[string]interface{}{"type": "string"},
			"fields":               map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
			"missing_fields":       map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
			"tool_requests":        map[string]interface{}{"type": "array", "items": toolRequest},
			"reply_plan":           replyPlan,
			"customer_facing_text": map[string]interface{}{"type": "string"},
			"tool_name":            map[string]interface{}{"type": "string"},
			"origin":               map[string]interface{}{"type": "string"},
			"destination":          map[string]interface{}{"type": "string"},
			"package_name":         map[string]interface{}{"type": "string"},
			"trip_date":            map[string]interface{}{"type": "string"},
			"qty":                  map[string]interface{}{"type": "integer"},
			"limit":                map[string]interface{}{"type": "integer"},
			"booking_id":           map[string]interface{}{"type": "string"},
			"reservation_code":     map[string]interface{}{"type": "string"},
			"payment_method":       map[string]interface{}{"type": "string"},
			"customer_document":    map[string]interface{}{"type": "string"},
			"amount":               map[string]interface{}{"type": "number"},
		},
		"required": []string{
			"action",
			"template_name",
			"reason_code",
			"fields",
			"missing_fields",
			"tool_requests",
			"reply_plan",
			"customer_facing_text",
			"tool_name",
			"origin",
			"destination",
			"package_name",
			"trip_date",
			"qty",
			"limit",
			"booking_id",
			"reservation_code",
			"payment_method",
			"customer_document",
			"amount",
		},
	}
}
