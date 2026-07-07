package chat

import "testing"

func TestBuildOpenAIInterpreterAssistReportFromMessagesLoadsValidFallbackAndMalformed(t *testing.T) {
	report := BuildOpenAIInterpreterAssistReportFromMessages([]Message{
		{
			ID: "no-assist",
			NormalizedPayload: map[string]interface{}{
				"raw_prompt": "must be ignored",
			},
		},
		{
			ID: "accepted-normalized",
			NormalizedPayload: map[string]interface{}{
				openAIInterpreterAssistMetadataKey: map[string]interface{}{
					"openai_assist_status": string(OpenAIInterpreterAssistAccepted),
					"validation_status":    string(OpenAIInterpreterAssistValidationAccepted),
					"considered":           true,
					"accepted":             true,
					"local_intent":         string(StructuredIntentUnknown),
					"local_confidence":     0.41,
					"openai_intent":        string(StructuredIntentPassengerCountReply),
					"openai_confidence":    0.76,
					"decision_source":      "openai_interpreter_assist",
				},
			},
		},
		{
			ID: "rejected-payload-fallback",
			Payload: map[string]interface{}{
				openAIInterpreterAssistMetadataKey: map[string]interface{}{
					"status":            string(OpenAIInterpreterAssistRejected),
					"considered":        "true",
					"openai_intent":     string(StructuredIntentPaymentPreference),
					"openai_confidence": "0.81",
					"reject_reason":     "openai_confidence_below_threshold",
					"reasons":           []interface{}{"openai_confidence_below_threshold"},
				},
			},
		},
		{
			ID: "skipped-normalized",
			NormalizedPayload: map[string]interface{}{
				openAIInterpreterAssistMetadataKey: map[string]interface{}{
					"openai_assist_status": string(OpenAIInterpreterAssistSkipped),
					"reject_reason":        "local_result_not_eligible",
					"skip_reason":          "local_result_not_eligible",
					"controlled_reason_codes": []string{
						"local_result_not_eligible",
					},
				},
			},
		},
		{
			ID: "malformed",
			NormalizedPayload: map[string]interface{}{
				openAIInterpreterAssistMetadataKey: "not-a-map",
			},
		},
	})

	if report.Total != 4 ||
		report.Considered != 2 ||
		report.NotConsidered != 0 ||
		report.Accepted != 1 ||
		report.Rejected != 1 ||
		report.Skipped != 1 ||
		report.MalformedCount != 1 {
		t.Fatalf("unexpected assist report from messages: %+v", report)
	}
	if report.LocalUnknownCount != 1 || report.LocalLowConfidenceCount != 1 {
		t.Fatalf("expected local UNKNOWN and low confidence counters, got %+v", report)
	}
	if report.ByReason["openai_confidence_below_threshold"] != 1 ||
		report.ByReason["local_result_not_eligible"] != 1 ||
		report.BySkipReason["local_result_not_eligible"] != 1 {
		t.Fatalf("expected known reasons from loader, got %+v", report)
	}
}

func TestBuildOpenAIInterpreterAssistReportFromMessagesDoesNotCountMissingConsideredAsFalse(t *testing.T) {
	report := BuildOpenAIInterpreterAssistReportFromMessages([]Message{
		{
			ID: "older-assist-payload",
			NormalizedPayload: map[string]interface{}{
				openAIInterpreterAssistMetadataKey: map[string]interface{}{
					"openai_assist_status": string(OpenAIInterpreterAssistRejected),
					"openai_intent":        string(StructuredIntentPaymentPreference),
					"reject_reason":        "openai_confidence_below_threshold",
				},
			},
		},
	})

	if report.Total != 1 || report.Rejected != 1 || report.Considered != 0 || report.NotConsidered != 0 {
		t.Fatalf("expected missing considered field to be excluded from considered counters, got %+v", report)
	}
	if report.ByRejectReason["openai_confidence_below_threshold"] != 1 {
		t.Fatalf("expected older payload reject reason to remain counted, got %+v", report.ByRejectReason)
	}
}

func TestBuildOpenAIInterpreterAssistReportFromMessagesAggregatesOldStatusesWithoutConsidered(t *testing.T) {
	report := BuildOpenAIInterpreterAssistReportFromMessages([]Message{
		{
			ID: "older-accepted",
			NormalizedPayload: map[string]interface{}{
				openAIInterpreterAssistMetadataKey: map[string]interface{}{
					"openai_assist_status": string(OpenAIInterpreterAssistAccepted),
				},
			},
		},
		{
			ID: "older-rejected",
			NormalizedPayload: map[string]interface{}{
				openAIInterpreterAssistMetadataKey: map[string]interface{}{
					"openai_assist_status": string(OpenAIInterpreterAssistRejected),
					"reject_reason":        "openai_confidence_below_threshold",
				},
			},
		},
		{
			ID: "older-skipped",
			NormalizedPayload: map[string]interface{}{
				openAIInterpreterAssistMetadataKey: map[string]interface{}{
					"openai_assist_status": string(OpenAIInterpreterAssistSkipped),
					"skip_reason":          "local_result_not_eligible",
				},
			},
		},
	})

	if report.Total != 3 ||
		report.Accepted != 1 ||
		report.Rejected != 1 ||
		report.Skipped != 1 ||
		report.Considered != 0 ||
		report.NotConsidered != 0 {
		t.Fatalf("expected old status payloads without considered to aggregate without considered counters, got %+v", report)
	}
	if report.ByRejectReason["openai_confidence_below_threshold"] != 1 ||
		report.BySkipReason["local_result_not_eligible"] != 1 {
		t.Fatalf("expected old payload reasons to stay aggregated, got reject=%+v skip=%+v", report.ByRejectReason, report.BySkipReason)
	}
}

func TestBuildOpenAIInterpreterAssistReportFromMessagesCountsExplicitConsideredValues(t *testing.T) {
	report := BuildOpenAIInterpreterAssistReportFromMessages([]Message{
		{
			ID: "explicit-considered-true",
			NormalizedPayload: map[string]interface{}{
				openAIInterpreterAssistMetadataKey: map[string]interface{}{
					"openai_assist_status": string(OpenAIInterpreterAssistAccepted),
					"considered":           true,
				},
			},
		},
		{
			ID: "explicit-considered-false",
			NormalizedPayload: map[string]interface{}{
				openAIInterpreterAssistMetadataKey: map[string]interface{}{
					"openai_assist_status": string(OpenAIInterpreterAssistSkipped),
					"considered":           false,
					"skip_reason":          "local_result_not_eligible",
				},
			},
		},
	})

	if report.Total != 2 ||
		report.Considered != 1 ||
		report.NotConsidered != 1 ||
		report.Accepted != 1 ||
		report.Skipped != 1 {
		t.Fatalf("expected explicit considered true/false to be counted, got %+v", report)
	}
	if report.BySkipReason["local_result_not_eligible"] != 1 {
		t.Fatalf("expected explicit considered=false skip reason to remain counted, got %+v", report.BySkipReason)
	}
}

func TestOpenAIInterpreterAssistReportItemFromMessageFallsBackAfterMalformedNormalizedPayload(t *testing.T) {
	item, ok := OpenAIInterpreterAssistReportItemFromMessage(Message{
		NormalizedPayload: map[string]interface{}{
			openAIInterpreterAssistMetadataKey: "not-a-map",
		},
		Payload: map[string]interface{}{
			openAIInterpreterAssistMetadataKey: map[string]interface{}{
				"openai_assist_status": string(OpenAIInterpreterAssistAccepted),
				"accepted":             true,
			},
		},
	})
	if !ok {
		t.Fatal("expected assist item")
	}
	if item.Malformed || item.Status != string(OpenAIInterpreterAssistAccepted) || !item.Accepted {
		t.Fatalf("expected valid payload fallback item, got %+v", item)
	}
}
