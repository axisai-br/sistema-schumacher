package chat

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

const testAvailabilityPromptEventV1MessageKey = availabilityPromptEventV1MessageKey

func TestAvailabilityPromptEventV1SeparatesRawAndPresentedResults(t *testing.T) {
	raw := availabilityPromptAuthorityRawResult()
	presentation := availabilityPromptPresentationV1(raw, []int{3})
	if presentation == nil {
		t.Fatal("expected one presented option from eight raw results")
	}
	event, ok := availabilityPromptEventForRunV1(
		RunAgentResult{AvailabilityPresentation: presentation},
		"draft-eight-to-one",
	)
	if !ok {
		t.Fatal("expected structural event")
	}
	if len(raw.Results) != 8 || event.PresentedOptionCount != 1 || len(event.PresentedOptions) != 1 {
		t.Fatalf("raw/presented boundary collapsed: raw=%d event=%+v", len(raw.Results), event)
	}
	option := event.PresentedOptions[0]
	if option.DisplayIndex != 1 || option.ResultIndex != 3 ||
		option.TripID != raw.Results[3].TripID ||
		option.BoardStopID != raw.Results[3].BoardStopID ||
		option.AlightStopID != raw.Results[3].AlightStopID ||
		option.TripDate != raw.Results[3].TripDate {
		t.Fatalf("display index did not resolve raw result index 3: %+v", option)
	}

	five := availabilityPromptPresentationV1(raw, []int{0, 1, 2, 3, 4})
	fiveEvent, ok := availabilityPromptEventForRunV1(
		RunAgentResult{AvailabilityPresentation: five},
		"draft-eight-to-five",
	)
	if !ok || fiveEvent.PresentedOptionCount != 5 {
		t.Fatalf("expected five explicitly presented options: %+v", fiveEvent)
	}
	if availabilityPromptPresentationV1(raw, []int{0, 1, 2, 3, 4, 5}) != nil {
		t.Fatal("presentation accepted more than five visible options")
	}
}

func TestAvailabilityPromptEventV1ValidatesContractCopiesAndFacts(t *testing.T) {
	raw := availabilityPromptAuthorityRawResult()
	valid := availabilityPromptAuthorityMessage("valid-structural", "SENT", raw, []int{0})
	authority := classifyAvailabilityPromptCandidateV1(valid)
	if authority.Class != availabilityPromptAuthorityValidStructuralV1 ||
		authority.Presented == nil ||
		len(authority.Presented.Results) != 1 {
		t.Fatalf("valid event was not authoritative: %+v", authority)
	}

	mutations := map[string]func(*Message){
		"unilateral": func(message *Message) {
			delete(message.NormalizedPayload, testAvailabilityPromptEventV1MessageKey)
		},
		"malformed": func(message *Message) {
			message.Payload[testAvailabilityPromptEventV1MessageKey] = "malformed"
			message.NormalizedPayload[testAvailabilityPromptEventV1MessageKey] = "malformed"
		},
		"divergent copies": func(message *Message) {
			event := availabilityPromptAuthorityEventMap(t, message.NormalizedPayload[testAvailabilityPromptEventV1MessageKey])
			asInterfaceSliceMaps(event["presented_options"])[0]["trip_id"] = "different-trip"
			message.NormalizedPayload[testAvailabilityPromptEventV1MessageKey] = event
		},
		"wrong version": func(message *Message) {
			for _, payload := range []map[string]interface{}{message.Payload, message.NormalizedPayload} {
				event := availabilityPromptAuthorityEventMap(t, payload[testAvailabilityPromptEventV1MessageKey])
				event["version"] = 2
				payload[testAvailabilityPromptEventV1MessageKey] = event
			}
		},
		"wrong kind": func(message *Message) {
			for _, payload := range []map[string]interface{}{message.Payload, message.NormalizedPayload} {
				event := availabilityPromptAuthorityEventMap(t, payload[testAvailabilityPromptEventV1MessageKey])
				event["kind"] = string(ActivePromptPassengerCount)
				payload[testAvailabilityPromptEventV1MessageKey] = event
			}
		},
		"wrong source": func(message *Message) {
			for _, payload := range []map[string]interface{}{message.Payload, message.NormalizedPayload} {
				event := availabilityPromptAuthorityEventMap(t, payload[testAvailabilityPromptEventV1MessageKey])
				event["source_message_id"] = "other-message"
				payload[testAvailabilityPromptEventV1MessageKey] = event
			}
		},
		"count mismatch": func(message *Message) {
			for _, payload := range []map[string]interface{}{message.Payload, message.NormalizedPayload} {
				event := availabilityPromptAuthorityEventMap(t, payload[testAvailabilityPromptEventV1MessageKey])
				event["presented_option_count"] = 2
				payload[testAvailabilityPromptEventV1MessageKey] = event
			}
		},
		"display index not one based": func(message *Message) {
			for _, payload := range []map[string]interface{}{message.Payload, message.NormalizedPayload} {
				event := availabilityPromptAuthorityEventMap(t, payload[testAvailabilityPromptEventV1MessageKey])
				asInterfaceSliceMaps(event["presented_options"])[0]["display_index"] = 0
				payload[testAvailabilityPromptEventV1MessageKey] = event
			}
		},
		"raw result mismatch": func(message *Message) {
			for _, payload := range []map[string]interface{}{message.Payload, message.NormalizedPayload} {
				tool := asMap(payload["tool_context"])
				availability := asMap(tool[toolNameAvailabilitySearch])
				results := asInterfaceSliceMaps(availability["results"])
				results[0]["trip_id"] = "forged-raw-trip"
			}
		},
	}

	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			message := availabilityPromptAuthorityMessage("invalid-"+strings.ReplaceAll(name, " ", "-"), "SENT", raw, []int{0})
			mutate(&message)
			if got := classifyAvailabilityPromptCandidateV1(message).Class; got != availabilityPromptAuthorityInvalidV1 {
				t.Fatalf("class=%s, want INVALID", got)
			}
		})
	}
}

func TestAvailabilityPromptEventV1RequiresBilateralIdenticalFacts(t *testing.T) {
	raw := availabilityPromptAuthorityRawResult()
	dateFrom := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	dateTo := time.Date(2026, time.August, 31, 0, 0, 0, 0, time.UTC)
	raw.Filter.DateFrom = &dateFrom
	raw.Filter.DateTo = &dateTo
	resultMutation := func(index int, key string, value interface{}) func(*Message) {
		return func(message *Message) {
			tool := asMap(message.Payload["tool_context"])
			availability := asMap(tool[toolNameAvailabilitySearch])
			results := asInterfaceSliceMaps(availability["results"])
			results[index][key] = value
		}
	}
	filterMutation := func(key string, value interface{}) func(*Message) {
		return func(message *Message) {
			tool := asMap(message.Payload["tool_context"])
			availability := asMap(tool[toolNameAvailabilitySearch])
			availability[key] = value
		}
	}

	mutations := []struct {
		name   string
		mutate func(*Message)
	}{
		{name: "payload facts only", mutate: func(message *Message) { delete(message.NormalizedPayload, "tool_context") }},
		{name: "normalized facts only", mutate: func(message *Message) { delete(message.Payload, "tool_context") }},
		{name: "price divergence", mutate: resultMutation(2, "price", 1.0)},
		{name: "package divergence", mutate: resultMutation(2, "package_name", "Pacote divergente")},
		{name: "route divergence", mutate: resultMutation(2, "route_id", "route-divergent")},
		{name: "departure divergence", mutate: resultMutation(2, "origin_depart_time", "23:59")},
		{name: "origin stop divergence", mutate: resultMutation(2, "origin_stop_id", "origin-divergent")},
		{name: "destination stop divergence", mutate: resultMutation(2, "destination_stop_id", "destination-divergent")},
		{name: "filter origin divergence", mutate: filterMutation("origin", "Origem divergente/XX")},
		{name: "filter destination divergence", mutate: filterMutation("destination", "Destino divergente/YY")},
		{name: "filter date from divergence", mutate: filterMutation("date_from", "2026-08-02")},
		{name: "filter date to divergence", mutate: filterMutation("date_to", "2026-08-30")},
		{name: "invalid bilateral date from", mutate: func(message *Message) {
			for _, payload := range []map[string]interface{}{message.Payload, message.NormalizedPayload} {
				tool := asMap(payload["tool_context"])
				availability := asMap(tool[toolNameAvailabilitySearch])
				availability["date_from"] = "invalid-date"
			}
		}},
		{name: "unpresented result divergence", mutate: resultMutation(7, "price", 2.0)},
	}

	old := availabilityPromptAuthorityMessage("bilateral-old-prompt", "SENT", raw, []int{0})
	selection := legacyAvailabilitySelectionMessageForStateTest(
		"bilateral-old-selection",
		raw,
		IntentSelectAvailabilityOption,
		old.ReceivedAt.Add(2*time.Minute),
		old.ID,
	)
	selectionInbound := legacyAvailabilitySelectionInboundForStateTest(
		selection,
		old.ReceivedAt.Add(time.Minute),
	)
	bookableHistory := []Message{old, selectionInbound, selection}
	if state := bootstrapAvailabilitySelectionStateFromHistoryForTest(bookableHistory); state.Status != AvailabilitySelectionStatusBookable {
		t.Fatalf("test precondition did not materialize historical BOOKABLE authority: %+v", state)
	}

	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			message := availabilityPromptAuthorityMessage(
				"bilateral-invalid-"+strings.ReplaceAll(test.name, " ", "-"),
				"DELIVERED",
				raw,
				[]int{2},
			)
			test.mutate(&message)
			authority := classifyAvailabilityPromptCandidateV1(message)
			if authority.Class != availabilityPromptAuthorityInvalidV1 || authority.Presented != nil {
				t.Fatalf("authority=%+v, want INVALID without Presented", authority)
			}
			if !deliveredInvalidAvailabilityPromptBarrierV1(message) {
				t.Fatal("delivered divergent facts did not create a barrier")
			}
			if shouldMergeAvailabilityFactsFromMessage(message) {
				t.Fatal("divergent facts were allowed into canonical state")
			}

			history := append(append([]Message(nil), bookableHistory...), message)
			if availability, _, ok := findLatestAvailabilityContextWithSource(history); ok || availability != nil {
				t.Fatalf("historical prompt crossed divergent-facts barrier: %+v", availability)
			}
			state := bootstrapAvailabilitySelectionStateFromHistoryForTest(history)
			if state.Status == AvailabilitySelectionStatusBookable || availabilitySelectionStateHasAggregateV1(state) {
				t.Fatalf("divergent facts preserved BOOKABLE authority: %+v", state)
			}
			if _, _, ok := resolveBookingCreateSelectionFromState("quero reservar a opção 1", state); ok {
				t.Fatal("divergent facts granted booking_create authority")
			}
			assertNoBookingCreateInputFromAvailabilityHistoryV1(t, history)
		})
	}

	valid := availabilityPromptAuthorityMessage("bilateral-valid-control", "DELIVERED", raw, []int{2})
	authority := classifyAvailabilityPromptCandidateV1(valid)
	if authority.Class != availabilityPromptAuthorityValidStructuralV1 || authority.Presented == nil {
		t.Fatalf("identical bilateral facts lost authority: %+v", authority)
	}
	if got, want := authority.Presented.Results, []AvailabilitySearchItem{raw.Results[2]}; !reflect.DeepEqual(got, want) {
		t.Fatalf("presented facts=%+v, want %+v", got, want)
	}
	if authority.Presented.Filter.DateFrom == nil ||
		authority.Presented.Filter.DateTo == nil ||
		!authority.Presented.Filter.DateFrom.Equal(dateFrom) ||
		!authority.Presented.Filter.DateTo.Equal(dateTo) {
		t.Fatalf("presented interval was not preserved: %+v", authority.Presented.Filter)
	}

	for _, projectionKind := range []struct {
		name         string
		mode         string
		reviewAction string
	}{
		{name: "bot auto reply", mode: string(deliveredPromptProjectionBotAutoReplyV1)},
		{name: "approved review", mode: string(deliveredPromptProjectionDraftReviewV1), reviewAction: "APPROVED_AS_IS"},
	} {
		t.Run("divergent draft "+projectionKind.name, func(t *testing.T) {
			draft, projection := availabilityPromptAuthorityProjectionFixtureV1(
				t,
				raw,
				projectionKind.mode,
				projectionKind.reviewAction,
				"bilateral-divergent-"+strings.ReplaceAll(projectionKind.name, " ", "-"),
			)
			resultMutation(2, "price", 3.0)(&draft)
			history := append(append([]Message(nil), bookableHistory...), draft, projection)
			projectionIndex := len(history) - 1
			if _, _, ok := resolveDeliveredPromptSourceMessageWithIndex(history, projectionIndex, projection); ok {
				t.Fatal("projection resolved a draft with divergent fact copies")
			}
			if !deliveredInvalidAvailabilityPromptBarrierAtV1(history, projectionIndex) {
				t.Fatal("irreconcilable delivered projection did not create a barrier")
			}
			if availability, _, ok := findLatestAvailabilityContextWithSource(history); ok || availability != nil {
				t.Fatalf("historical prompt crossed irreconcilable projection: %+v", availability)
			}
			state := bootstrapAvailabilitySelectionStateFromHistoryForTest(history)
			if state.Status == AvailabilitySelectionStatusBookable || availabilitySelectionStateHasAggregateV1(state) {
				t.Fatalf("irreconcilable projection preserved BOOKABLE authority: %+v", state)
			}
			assertNoBookingCreateInputFromAvailabilityHistoryV1(t, history)
		})
	}
}

func TestAvailabilityPromptEventV1LegacyFactsRequireBilateralCopies(t *testing.T) {
	raw := availabilityPromptAuthorityRawResult()
	message := availabilityPromptAuthorityMessage("legacy-unilateral-facts", "DELIVERED", raw, []int{0})
	delete(message.Payload, testAvailabilityPromptEventV1MessageKey)
	delete(message.NormalizedPayload, testAvailabilityPromptEventV1MessageKey)
	delete(message.NormalizedPayload, "tool_context")

	authority := classifyAvailabilityPromptCandidateV1(message)
	if authority.Class != availabilityPromptAuthorityInvalidV1 || authority.Presented != nil {
		t.Fatalf("unilateral legacy facts authority=%+v, want INVALID without Presented", authority)
	}
	if contexts, present, valid := availabilityPromptRawContextsV1(message); !present || valid || len(contexts) != 0 {
		t.Fatalf("unilateral facts reconciliation contexts=%d present=%t valid=%t, want 0/true/false", len(contexts), present, valid)
	}
	if context, ok := availabilityPromptClassifiedContextV1(message); ok || context != nil {
		t.Fatalf("unilateral legacy facts were published: ok=%t context=%+v", ok, context)
	}
	if count := availabilityOptionCountFromMessageToolContext(message); count != 0 {
		t.Fatalf("unilateral legacy facts produced option count %d", count)
	}
	if context := trustedAvailabilityContextFromPromptMessage(message); context != nil {
		t.Fatalf("unilateral legacy facts became trusted: %+v", context)
	}
	if context := visibleAvailabilityContextFromPromptMessage(message); context != nil {
		t.Fatalf("unilateral legacy facts became visible: %+v", context)
	}
	if context, _, ok := findLatestAvailabilityContextWithSource([]Message{message}); ok || context != nil {
		t.Fatalf("finder published unilateral legacy facts: %+v", context)
	}
	draft := collectBookingDraftContextForRoutingBaseline(Session{}, []Message{message})
	if draft.HasAvailabilityShown || draft.TripID != "" || draft.Origin != "" || draft.Destination != "" {
		t.Fatalf("booking draft published unilateral legacy facts: %+v", draft)
	}
	state := bootstrapAvailabilitySelectionStateFromHistoryForTest([]Message{message})
	if state.Status == AvailabilitySelectionStatusBookable || availabilitySelectionStateHasAggregateV1(state) {
		t.Fatalf("bootstrap published unilateral legacy facts: %+v", state)
	}
	if _, _, ok := resolveBookingCreateSelectionFromState("quero reservar a opção 1", state); ok {
		t.Fatal("booking_create received unilateral legacy authority")
	}
}

func TestAvailabilityContextTemporalRangeValidationV1(t *testing.T) {
	utcDate := func(year int, month time.Month, day int) *time.Time {
		value := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
		return &value
	}
	assertValid := func(t *testing.T, name string, dateFrom *time.Time, dateTo *time.Time) {
		t.Helper()
		raw := availabilityPromptAuthorityRawResult()
		raw.Filter.DateFrom = dateFrom
		raw.Filter.DateTo = dateTo
		message := availabilityPromptAuthorityMessage("temporal-valid-"+name, "DELIVERED", raw, []int{0})
		authority := classifyAvailabilityPromptCandidateV1(message)
		if authority.Class != availabilityPromptAuthorityValidStructuralV1 || authority.Presented == nil {
			t.Fatalf("valid temporal filter lost authority: %+v", authority)
		}
		if !sameDeliveredPromptOptionalTimeV1(authority.Presented.Filter.DateFrom, dateFrom) ||
			!sameDeliveredPromptOptionalTimeV1(authority.Presented.Filter.DateTo, dateTo) {
			t.Fatalf("presented temporal filter changed: got=%+v want_from=%v want_to=%v", authority.Presented.Filter, dateFrom, dateTo)
		}
	}

	t.Run("valid controls", func(t *testing.T) {
		assertValid(t, "absent", nil, nil)
		assertValid(t, "from-only", utcDate(2026, time.August, 3), nil)
		assertValid(t, "to-only", nil, utcDate(2026, time.August, 10))
		assertValid(t, "ordered", utcDate(2026, time.August, 3), utcDate(2026, time.August, 10))
		assertValid(t, "same-day", utcDate(2026, time.August, 3), utcDate(2026, time.August, 3))
	})

	t.Run("inverted interval is invalid even when copies match", func(t *testing.T) {
		raw := availabilityPromptAuthorityRawResult()
		raw.Filter.DateFrom = utcDate(2026, time.August, 10)
		raw.Filter.DateTo = utcDate(2026, time.August, 3)
		message := availabilityPromptAuthorityMessage("temporal-inverted", "DELIVERED", raw, []int{0})
		authority := classifyAvailabilityPromptCandidateV1(message)
		if authority.Class != availabilityPromptAuthorityInvalidV1 || authority.Presented != nil {
			t.Fatalf("inverted interval authority=%+v, want INVALID without Presented", authority)
		}
		if !deliveredInvalidAvailabilityPromptBarrierV1(message) {
			t.Fatal("delivered inverted interval did not create a barrier")
		}
	})

	for _, test := range []struct {
		name   string
		mutate func(payload map[string]interface{})
	}{
		{name: "empty", mutate: func(payload map[string]interface{}) { payload["date_from"] = "" }},
		{name: "null", mutate: func(payload map[string]interface{}) { payload["date_from"] = nil }},
		{name: "non string", mutate: func(payload map[string]interface{}) { payload["date_from"] = 20260803 }},
		{name: "impossible", mutate: func(payload map[string]interface{}) { payload["date_from"] = "2026-02-30" }},
		{name: "malformed", mutate: func(payload map[string]interface{}) { payload["date_from"] = "03/08/2026" }},
	} {
		t.Run("bilateral "+test.name, func(t *testing.T) {
			message := availabilityPromptAuthorityMessage("temporal-"+strings.ReplaceAll(test.name, " ", "-"), "DELIVERED", availabilityPromptAuthorityRawResult(), []int{0})
			for _, root := range []map[string]interface{}{message.Payload, message.NormalizedPayload} {
				test.mutate(asMap(asMap(root["tool_context"])[toolNameAvailabilitySearch]))
			}
			authority := classifyAvailabilityPromptCandidateV1(message)
			if authority.Class != availabilityPromptAuthorityInvalidV1 || authority.Presented != nil {
				t.Fatalf("%s temporal value authority=%+v, want INVALID", test.name, authority)
			}
		})
	}

	t.Run("bilateral divergence", func(t *testing.T) {
		message := availabilityPromptAuthorityMessage("temporal-divergent", "DELIVERED", availabilityPromptAuthorityRawResult(), []int{0})
		payloadAvailability := asMap(asMap(message.Payload["tool_context"])[toolNameAvailabilitySearch])
		normalizedAvailability := asMap(asMap(message.NormalizedPayload["tool_context"])[toolNameAvailabilitySearch])
		payloadAvailability["date_from"] = "2026-08-03"
		normalizedAvailability["date_from"] = "2026-08-04"
		authority := classifyAvailabilityPromptCandidateV1(message)
		if authority.Class != availabilityPromptAuthorityInvalidV1 || authority.Presented != nil {
			t.Fatalf("divergent temporal copies authority=%+v, want INVALID", authority)
		}
	})

	for _, test := range []struct {
		name   string
		mutate func(payload map[string]interface{})
	}{
		{name: "invalid", mutate: func(payload map[string]interface{}) { payload["date_from"] = "malformed-date" }},
		{name: "absent", mutate: func(payload map[string]interface{}) { delete(payload, "date_from") }},
		{name: "divergent", mutate: func(payload map[string]interface{}) { payload["date_from"] = "2026-08-04" }},
	} {
		t.Run("one copy "+test.name, func(t *testing.T) {
			raw := availabilityPromptAuthorityRawResult()
			raw.Filter.DateFrom = utcDate(2026, time.August, 3)
			message := availabilityPromptAuthorityMessage("temporal-one-copy-"+test.name, "DELIVERED", raw, []int{0})
			test.mutate(asMap(asMap(message.NormalizedPayload["tool_context"])[toolNameAvailabilitySearch]))
			authority := classifyAvailabilityPromptCandidateV1(message)
			if authority.Class != availabilityPromptAuthorityInvalidV1 || authority.Presented != nil {
				t.Fatalf("one-copy %s authority=%+v, want INVALID", test.name, authority)
			}
			if !deliveredInvalidAvailabilityPromptBarrierV1(message) {
				t.Fatalf("one-copy %s did not create a barrier", test.name)
			}
		})
	}

	t.Run("canonical exact temporal values", func(t *testing.T) {
		canonical := time.Date(2026, time.August, 3, 0, 0, 0, 0, time.UTC)
		raw := availabilityPromptAuthorityRawResult()
		raw.Filter.TripDate = &canonical
		raw.Filter.DateFrom = &canonical
		raw.Filter.DateTo = &canonical
		message := availabilityPromptAuthorityMessage("temporal-canonical-exact", "DELIVERED", raw, []int{0})
		authority := classifyAvailabilityPromptCandidateV1(message)
		if authority.Class != availabilityPromptAuthorityValidStructuralV1 || authority.Presented == nil {
			t.Fatalf("canonical exact temporal values lost authority: %+v", authority)
		}
		for key, value := range map[string]*time.Time{
			"trip_date": authority.Presented.Filter.TripDate,
			"date_from": authority.Presented.Filter.DateFrom,
			"date_to":   authority.Presented.Filter.DateTo,
		} {
			if value == nil || value.Format("2006-01-02") != "2026-08-03" {
				t.Fatalf("canonical %s was not preserved: %v", key, value)
			}
		}
	})

	for _, key := range []string{"trip_date", "date_from", "date_to"} {
		for _, value := range []string{
			" 2026-08-03 ",
			"2026-08-03 ",
			" 2026-08-03",
			"\t2026-08-03",
			"2026-08-03\n",
		} {
			key := key
			value := value
			t.Run("noncanonical "+key+" "+fmt.Sprintf("%q", value), func(t *testing.T) {
				message := availabilityPromptAuthorityMessage(
					"temporal-noncanonical-"+key,
					"DELIVERED",
					availabilityPromptAuthorityRawResult(),
					[]int{0},
				)
				for _, root := range []map[string]interface{}{message.Payload, message.NormalizedPayload} {
					availability := asMap(asMap(root["tool_context"])[toolNameAvailabilitySearch])
					availability[key] = value
				}
				authority := classifyAvailabilityPromptCandidateV1(message)
				if authority.Class != availabilityPromptAuthorityInvalidV1 || authority.Presented != nil {
					t.Fatalf("noncanonical %s=%q authority=%+v, want INVALID", key, value, authority)
				}
				if !deliveredInvalidAvailabilityPromptBarrierV1(message) {
					t.Fatalf("noncanonical %s=%q did not create a barrier", key, value)
				}
			})
		}
	}

	t.Run("canonical results trip_date", func(t *testing.T) {
		message := availabilityPromptAuthorityMessage(
			"result-trip-date-canonical",
			"DELIVERED",
			availabilityPromptAuthorityRawResult(),
			[]int{0},
		)
		authority := classifyAvailabilityPromptCandidateV1(message)
		if authority.Class != availabilityPromptAuthorityValidStructuralV1 || authority.Presented == nil {
			t.Fatalf("canonical results trip_date lost authority: %+v", authority)
		}
		if got := authority.Presented.Results[0].TripDate; got != "2026-08-03" {
			t.Fatalf("canonical results trip_date=%q, want preserved value", got)
		}
	})

	for _, test := range []struct {
		name  string
		value interface{}
	}{
		{name: "surrounding whitespace", value: " 2026-08-03 "},
		{name: "trailing whitespace", value: "2026-08-03 "},
		{name: "leading whitespace", value: " 2026-08-03"},
		{name: "leading tab", value: "\t2026-08-03"},
		{name: "trailing newline", value: "2026-08-03\n"},
		{name: "empty", value: ""},
		{name: "null", value: nil},
		{name: "non string", value: 20260803},
		{name: "alternate format", value: "03/08/2026"},
		{name: "impossible date", value: "2026-02-30"},
	} {
		t.Run("bilateral results trip_date "+test.name, func(t *testing.T) {
			message := availabilityPromptAuthorityMessage(
				"result-trip-date-invalid-"+strings.ReplaceAll(test.name, " ", "-"),
				"DELIVERED",
				availabilityPromptAuthorityRawResult(),
				[]int{0},
			)
			for _, root := range []map[string]interface{}{message.Payload, message.NormalizedPayload} {
				availability := asMap(asMap(root["tool_context"])[toolNameAvailabilitySearch])
				results := asInterfaceSliceMaps(availability["results"])
				results[0]["trip_date"] = test.value
				if _, valid := parseAvailabilityContextPayloadWithValidityV1(availability); valid {
					t.Fatalf("persisted results trip_date=%#v decoded as valid", test.value)
				}
			}
			authority := classifyAvailabilityPromptCandidateV1(message)
			if authority.Class != availabilityPromptAuthorityInvalidV1 || authority.Presented != nil {
				t.Fatalf("results trip_date=%#v authority=%+v, want INVALID without Presented", test.value, authority)
			}
			if !deliveredInvalidAvailabilityPromptBarrierV1(message) {
				t.Fatalf("results trip_date=%#v did not create a barrier", test.value)
			}
		})
	}
}

func TestAvailabilityContextInvalidPayloadFailsClosedAcrossReaders(t *testing.T) {
	buildHistory := func() []Message {
		message := availabilityPromptAuthorityMessage(
			"legacy-malformed-temporal-prompt",
			"DELIVERED",
			availabilityPromptAuthorityRawResult(),
			[]int{0},
		)
		delete(message.Payload, testAvailabilityPromptEventV1MessageKey)
		delete(message.NormalizedPayload, testAvailabilityPromptEventV1MessageKey)
		for _, root := range []map[string]interface{}{message.Payload, message.NormalizedPayload} {
			availability := asMap(asMap(root["tool_context"])[toolNameAvailabilitySearch])
			availability["date_from"] = "malformed-date"
		}
		return []Message{message}
	}

	t.Run("classification and readers", func(t *testing.T) {
		history := buildHistory()
		message := history[0]
		authority := classifyAvailabilityPromptCandidateV1(message)
		if authority.Class != availabilityPromptAuthorityInvalidV1 || authority.Presented != nil {
			t.Fatalf("malformed legacy payload authority=%+v, want INVALID without Presented", authority)
		}
		if !deliveredInvalidAvailabilityPromptBarrierAtV1(history, 0) {
			t.Fatal("delivered malformed legacy payload did not create a barrier")
		}
		if trusted := trustedAvailabilityContextFromPromptMessage(message); trusted != nil {
			t.Fatalf("trusted reader accepted malformed payload: %+v", trusted)
		}
		if visible := visibleAvailabilitySelectionContextFromHistoryMessage(message); visible != nil {
			t.Fatalf("visible reader accepted malformed payload: %+v", visible)
		}
		if count := availabilityOptionCountFromMessage(message); count != 0 {
			t.Fatalf("malformed payload exposed option count=%d", count)
		}
		if availability, _, ok := findLatestAvailabilityContextWithSource(history); ok || availability != nil {
			t.Fatalf("finder accepted malformed payload: %+v", availability)
		}
		draft := collectBookingDraftContextWithPassengerState(
			Session{},
			history,
			"1",
			completePassengerStateForTest(1, 0),
		)
		if draft.HasBookableSelection || draft.SelectedOptionIndex > 0 ||
			strings.TrimSpace(draft.TripID) != "" {
			t.Fatalf("booking draft accepted malformed payload: %+v", draft)
		}
	})

	t.Run("numeric reply cannot materialize or book", func(t *testing.T) {
		history := buildHistory()
		now := history[0].ReceivedAt.Add(time.Minute)
		activePrompt := InferActivePromptContext(history, CanonicalConversationState{Phase: ConversationPhaseTripSelection})
		routed := routeDeterministicIntent(history, "1", discoveryState(), now)
		gate := classifyAvailabilitySelectionTurn(
			activePrompt,
			history,
			"1",
			routed,
			false,
			newAvailabilitySelectionStateV1(),
		)
		if gate.Class == availabilitySelectionGateMaterialize {
			t.Fatalf("malformed payload produced MATERIALIZE: %+v", gate)
		}
		inbound := Message{
			ID: "legacy-malformed-selection-reply", Direction: "INBOUND", Body: "1",
			ReceivedAt: now, CreatedAt: now,
		}
		events := availabilitySelectionEventsForTurnV1(
			[]Message{inbound},
			history,
			"1",
			activePrompt,
			routed,
			gate,
			newAvailabilitySelectionStateV1(),
			"legacy-malformed-selection-projection",
		)
		for _, event := range events {
			if event.Type == AvailabilitySelectionEventMaterialized || event.MaterializesAuthority {
				t.Fatalf("malformed payload emitted SELECTION_MATERIALIZED: %+v", event)
			}
		}
		hydrated, err := hydrateAvailabilitySelectionEventsV1(events, []Message{inbound}, true)
		if err != nil {
			t.Fatalf("hydrate invalid-payload control events: %v", err)
		}
		state := ReduceAvailabilitySelectionEventsV1(newAvailabilitySelectionStateV1(), hydrated)
		if state.Status == AvailabilitySelectionStatusBookable || availabilitySelectionStateHasAggregateV1(state) {
			t.Fatalf("malformed payload produced BOOKABLE: %+v", state)
		}
		session := sessionWithAvailabilitySelectionStateForTest(Session{
			ContactKey: "invalid-temporal", CustomerName: "Pessoa Teste", CustomerPhone: "5500000000000",
		}, state)
		if input, ok := parseBookingCreateInputWithPassengerState(
			session,
			append(history, inbound),
			"quero reservar opção 1\nPessoa Teste | CPF | 84960815086",
			nil,
			completePassengerStateForTest(1, 0),
		); ok {
			t.Fatalf("malformed payload produced BookingCreateInput: %+v", input)
		}
	})
}

func TestAvailabilityResultTripDateInvalidFailsClosedAcrossReaders(t *testing.T) {
	raw := availabilityPromptAuthorityRawResult()
	oldPrompt := availabilityPromptAuthorityMessage("result-trip-date-old-prompt", "DELIVERED", raw, []int{0})
	oldSelection := legacyAvailabilitySelectionMessageForStateTest(
		"result-trip-date-old-selection",
		raw,
		IntentSelectAvailabilityOption,
		oldPrompt.ReceivedAt.Add(2*time.Minute),
		oldPrompt.ID,
	)
	oldSelectionInbound := legacyAvailabilitySelectionInboundForStateTest(
		oldSelection,
		oldPrompt.ReceivedAt.Add(time.Minute),
	)
	bookablePrefix := []Message{oldPrompt, oldSelectionInbound, oldSelection}
	bookable := bootstrapAvailabilitySelectionStateFromHistoryForTest(bookablePrefix)
	if bookable.Status != AvailabilitySelectionStatusBookable {
		t.Fatalf("test precondition did not materialize historical BOOKABLE: %+v", bookable)
	}

	malformed := availabilityPromptAuthorityMessage("result-trip-date-invalid", "DELIVERED", raw, []int{0})
	delete(malformed.Payload, testAvailabilityPromptEventV1MessageKey)
	delete(malformed.NormalizedPayload, testAvailabilityPromptEventV1MessageKey)
	malformedAt := oldSelection.ReceivedAt.Add(time.Minute)
	malformed.ReceivedAt = malformedAt
	malformed.CreatedAt = malformedAt
	malformed.NormalizedPayload["delivery_recorded_at"] = malformedAt.Format(time.RFC3339Nano)
	for _, root := range []map[string]interface{}{malformed.Payload, malformed.NormalizedPayload} {
		availability := asMap(asMap(root["tool_context"])[toolNameAvailabilitySearch])
		asInterfaceSliceMaps(availability["results"])[0]["trip_date"] = " 2026-08-03 "
		if _, valid := parseAvailabilityContextPayloadWithValidityV1(availability); valid {
			t.Fatal("noncanonical persisted results trip_date decoded as valid")
		}
	}
	history := append(append([]Message(nil), bookablePrefix...), malformed)

	authority := classifyAvailabilityPromptCandidateV1(malformed)
	if authority.Class != availabilityPromptAuthorityInvalidV1 || authority.Presented != nil {
		t.Fatalf("noncanonical results trip_date authority=%+v, want INVALID without Presented", authority)
	}
	if !deliveredInvalidAvailabilityPromptBarrierAtV1(history, len(history)-1) {
		t.Fatal("noncanonical results trip_date did not create a temporal barrier")
	}
	if active := InferActivePromptContext(history, CanonicalConversationState{Phase: ConversationPhaseTripSelection}); active.Kind != ActivePromptUnknown || active.SourceMessageID != "" || active.HasAvailabilityList {
		t.Fatalf("noncanonical results trip_date opened active prompt: %+v", active)
	}
	if trusted := trustedAvailabilityContextFromPromptMessage(malformed); trusted != nil {
		t.Fatalf("trusted reader accepted noncanonical results trip_date: %+v", trusted)
	}
	if visible := visibleAvailabilitySelectionContextFromHistoryMessage(malformed); visible != nil {
		t.Fatalf("visible reader accepted noncanonical results trip_date: %+v", visible)
	}
	if count := availabilityOptionCountFromMessage(malformed); count != 0 {
		t.Fatalf("noncanonical results trip_date exposed option count=%d", count)
	}
	if availability, sourceIndex, ok := findLatestAvailabilityContextWithSource(history); ok || availability != nil || sourceIndex != -1 {
		t.Fatalf("finder crossed noncanonical results trip_date barrier: ok=%t index=%d availability=%+v", ok, sourceIndex, availability)
	}
	if state := bootstrapAvailabilitySelectionStateFromHistoryForTest(history); state.Status == AvailabilitySelectionStatusBookable || availabilitySelectionStateHasAggregateV1(state) {
		t.Fatalf("bootstrap preserved BOOKABLE across noncanonical results trip_date: %+v", state)
	}
	session := sessionWithAvailabilitySelectionStateForTest(Session{}, bookable)
	if state := availabilitySelectionStateV1ForRead(session, history); state.Status == AvailabilitySelectionStatusBookable || availabilitySelectionStateHasAggregateV1(state) {
		t.Fatalf("AvailabilitySelectionStateV1 preserved BOOKABLE across noncanonical results trip_date: %+v", state)
	}
	draft := collectBookingDraftContextWithPassengerState(
		session,
		history,
		"1",
		completePassengerStateForTest(1, 0),
	)
	if draft.HasBookableSelection || draft.HasAvailabilityShown || draft.SelectedOptionIndex > 0 ||
		strings.TrimSpace(draft.TripID) != "" || strings.TrimSpace(draft.BoardStopID) != "" ||
		strings.TrimSpace(draft.AlightStopID) != "" {
		t.Fatalf("booking draft crossed noncanonical results trip_date barrier: %+v", draft)
	}
	assertNoBookingCreateInputFromAvailabilityHistoryV1(t, history)
}

func TestAvailabilityContextNonMapToolContextFailsClosedAcrossReaders(t *testing.T) {
	raw := availabilityPromptAuthorityRawResult()
	oldPrompt := availabilityPromptAuthorityMessage("nonmap-old-prompt", "DELIVERED", raw, []int{0})
	oldSelection := legacyAvailabilitySelectionMessageForStateTest(
		"nonmap-old-selection",
		raw,
		IntentSelectAvailabilityOption,
		oldPrompt.ReceivedAt.Add(2*time.Minute),
		oldPrompt.ID,
	)
	oldSelectionInbound := legacyAvailabilitySelectionInboundForStateTest(
		oldSelection,
		oldPrompt.ReceivedAt.Add(time.Minute),
	)
	bookablePrefix := []Message{oldPrompt, oldSelectionInbound, oldSelection}
	if state := bootstrapAvailabilitySelectionStateFromHistoryForTest(bookablePrefix); state.Status != AvailabilitySelectionStatusBookable {
		t.Fatalf("test precondition did not materialize historical BOOKABLE: %+v", state)
	}

	malformed := availabilityPromptAuthorityMessage("nonmap-tool-context", "DELIVERED", raw, []int{0})
	delete(malformed.Payload, testAvailabilityPromptEventV1MessageKey)
	delete(malformed.NormalizedPayload, testAvailabilityPromptEventV1MessageKey)
	malformed.NormalizedPayload["tool_context"] = "malformed-tool-context"
	history := append(append([]Message(nil), bookablePrefix...), malformed)

	contexts, factsPresent, factsValid := availabilityPromptRawContextsV1(malformed)
	if !factsPresent || factsValid {
		t.Fatalf("non-map tool_context presence/validity=(%t,%t), contexts=%+v; want present and invalid", factsPresent, factsValid, contexts)
	}
	authority := classifyAvailabilityPromptCandidateV1(malformed)
	if authority.Class != availabilityPromptAuthorityInvalidV1 || authority.Presented != nil {
		t.Fatalf("non-map factual copy authority=%+v, want INVALID without Presented", authority)
	}
	if !deliveredInvalidAvailabilityPromptBarrierAtV1(history, len(history)-1) {
		t.Fatal("delivered non-map factual copy did not create a temporal barrier")
	}
	if trusted := trustedAvailabilityContextFromPromptMessage(malformed); trusted != nil {
		t.Fatalf("trusted reader accepted non-map factual copy: %+v", trusted)
	}
	if visible := visibleAvailabilitySelectionContextFromHistoryMessage(malformed); visible != nil {
		t.Fatalf("visible reader accepted non-map factual copy: %+v", visible)
	}
	if count := availabilityOptionCountFromMessage(malformed); count != 0 {
		t.Fatalf("non-map factual copy exposed option count=%d", count)
	}
	if active := InferActivePromptContext(history, CanonicalConversationState{Phase: ConversationPhaseTripSelection}); active.Kind != ActivePromptUnknown || active.SourceMessageID != "" || active.HasAvailabilityList {
		t.Fatalf("non-map factual copy opened active prompt: %+v", active)
	}
	if availability, sourceIndex, ok := findLatestAvailabilityContextWithSource(history); ok || availability != nil || sourceIndex != -1 {
		t.Fatalf("finder crossed non-map factual barrier: ok=%t index=%d availability=%+v", ok, sourceIndex, availability)
	}
	state := bootstrapAvailabilitySelectionStateFromHistoryForTest(history)
	if state.Status == AvailabilitySelectionStatusBookable || availabilitySelectionStateHasAggregateV1(state) {
		t.Fatalf("non-map factual copy resurrected BOOKABLE: %+v", state)
	}
	draft := collectBookingDraftContextWithPassengerState(
		Session{},
		history,
		"1",
		completePassengerStateForTest(1, 0),
	)
	if draft.HasBookableSelection || draft.HasAvailabilityShown || draft.SelectedOptionIndex > 0 ||
		strings.TrimSpace(draft.TripID) != "" || strings.TrimSpace(draft.BoardStopID) != "" ||
		strings.TrimSpace(draft.AlightStopID) != "" {
		t.Fatalf("non-map factual copy enriched booking draft: %+v", draft)
	}
	assertNoBookingCreateInputFromAvailabilityHistoryV1(t, history)
}

func TestAvailabilityPromptEventV1LegacyProjectionRequiresResolvableDraft(t *testing.T) {
	raw := availabilityPromptAuthorityRawResult()
	old := availabilityPromptAuthorityMessage("legacy-barrier-old-prompt", "SENT", raw, []int{0})
	selection := legacyAvailabilitySelectionMessageForStateTest(
		"legacy-barrier-old-selection",
		raw,
		IntentSelectAvailabilityOption,
		old.ReceivedAt.Add(2*time.Minute),
		old.ID,
	)
	selectionInbound := legacyAvailabilitySelectionInboundForStateTest(
		selection,
		old.ReceivedAt.Add(time.Minute),
	)
	bookablePrefix := []Message{old, selectionInbound, selection}
	if state := bootstrapAvailabilitySelectionStateFromHistoryForTest(bookablePrefix); state.Status != AvailabilitySelectionStatusBookable {
		t.Fatalf("test precondition did not materialize historical BOOKABLE authority: %+v", state)
	}

	for _, projectionKind := range []struct {
		name         string
		mode         string
		reviewAction string
	}{
		{name: "bot auto reply", mode: string(deliveredPromptProjectionBotAutoReplyV1)},
		{name: "approved review", mode: string(deliveredPromptProjectionDraftReviewV1), reviewAction: "APPROVED_AS_IS"},
	} {
		t.Run(projectionKind.name, func(t *testing.T) {
			newFixture := func(suffix string) (Message, Message) {
				draft, projection := availabilityPromptAuthorityProjectionFixtureV1(
					t,
					raw,
					projectionKind.mode,
					projectionKind.reviewAction,
					"legacy-"+strings.ReplaceAll(projectionKind.name+"-"+suffix, " ", "-"),
				)
				for _, message := range []*Message{&draft, &projection} {
					delete(message.Payload, testAvailabilityPromptEventV1MessageKey)
					delete(message.NormalizedPayload, testAvailabilityPromptEventV1MessageKey)
				}
				projection.Body = draft.Body
				return draft, projection
			}
			cloneDraft := func(message Message) Message {
				cloned := message
				cloned.Payload = cloneMap(message.Payload)
				cloned.NormalizedPayload = cloneMap(message.NormalizedPayload)
				return cloned
			}

			for _, test := range []struct {
				name  string
				build func() ([]Message, int)
			}{
				{
					name: "missing draft",
					build: func() ([]Message, int) {
						_, projection := newFixture("missing")
						history := append(append([]Message(nil), bookablePrefix...), projection)
						return history, len(history) - 1
					},
				},
				{
					name: "missing draft message id",
					build: func() ([]Message, int) {
						_, projection := newFixture("missing-reference")
						delete(projection.Payload, "draft_message_id")
						delete(projection.NormalizedPayload, "draft_message_id")
						history := append(append([]Message(nil), bookablePrefix...), projection)
						return history, len(history) - 1
					},
				},
				{
					name: "incorrect draft message id",
					build: func() ([]Message, int) {
						draft, projection := newFixture("incorrect-reference")
						projection.Payload["draft_message_id"] = "missing-draft"
						projection.NormalizedPayload["draft_message_id"] = "missing-draft"
						history := append(append([]Message(nil), bookablePrefix...), draft, projection)
						return history, len(history) - 1
					},
				},
				{
					name: "duplicate draft message id",
					build: func() ([]Message, int) {
						draft, projection := newFixture("duplicate-reference")
						history := append(append([]Message(nil), bookablePrefix...), draft, cloneDraft(draft), projection)
						return history, len(history) - 1
					},
				},
				{
					name: "causally invalid draft timestamp",
					build: func() ([]Message, int) {
						draft, projection := newFixture("causally-invalid")
						draft.ReceivedAt = projection.ReceivedAt.Add(time.Minute)
						draft.CreatedAt = projection.CreatedAt.Add(time.Minute)
						history := append(append([]Message(nil), bookablePrefix...), draft, projection)
						return history, len(history) - 1
					},
				},
			} {
				t.Run(test.name, func(t *testing.T) {
					history, projectionIndex := test.build()
					projection := history[projectionIndex]
					if _, _, ok := resolveDeliveredPromptSourceMessageWithIndex(history, projectionIndex, projection); ok {
						t.Fatal("legacy projection resolved without exactly one causal draft")
					}
					if !deliveredInvalidAvailabilityPromptBarrierAtV1(history, projectionIndex) {
						t.Fatal("delivered unresolved legacy projection did not create a barrier")
					}
					if message, _, ok := classifiedAvailabilityPromptMessageAtV1(history, projectionIndex); ok {
						t.Fatalf("unresolved legacy projection produced classified facts: %+v", message)
					}
					if availability, _, ok := findLatestAvailabilityContextWithSource(history); ok || availability != nil {
						t.Fatalf("historical prompt crossed unresolved legacy projection: %+v", availability)
					}
					state := bootstrapAvailabilitySelectionStateFromHistoryForTest(history)
					if state.Status == AvailabilitySelectionStatusBookable || availabilitySelectionStateHasAggregateV1(state) {
						t.Fatalf("unresolved legacy projection preserved BOOKABLE authority: %+v", state)
					}
					assertNoBookingCreateInputFromAvailabilityHistoryV1(t, history)
				})
			}

			t.Run("exactly one causal legacy draft", func(t *testing.T) {
				draft, projection := newFixture("positive")
				history := []Message{draft, projection}
				resolved, sourceIndex, ok := resolveDeliveredPromptSourceMessageWithIndex(history, 1, projection)
				if !ok || sourceIndex != 1 {
					t.Fatalf("unique causal legacy draft did not resolve: ok=%t index=%d", ok, sourceIndex)
				}
				authority := classifyAvailabilityPromptCandidateV1(resolved)
				if authority.Class != availabilityPromptAuthorityAbsentLegacyV1 || authority.Presented != nil {
					t.Fatalf("resolved legacy authority=%+v, want ABSENT_LEGACY without structural Presented", authority)
				}
				if deliveredInvalidAvailabilityPromptBarrierAtV1(history, 1) {
					t.Fatal("unique causal legacy projection created a barrier")
				}
				availability, sourceIndex, ok := findLatestAvailabilityContextWithSource(history)
				if !ok || availability == nil || sourceIndex != 1 || len(availability.Results) == 0 {
					t.Fatalf("unique causal legacy projection lost fallback facts: ok=%t index=%d availability=%+v", ok, sourceIndex, availability)
				}
			})

			t.Run("undelivered unresolved projection has no barrier", func(t *testing.T) {
				_, projection := newFixture("undelivered")
				projection.ProcessingStatus = "SEND_FAILED"
				delete(projection.NormalizedPayload, "delivery_recorded_at")
				history := append(append([]Message(nil), bookablePrefix...), projection)
				projectionIndex := len(history) - 1
				if deliveredInvalidAvailabilityPromptBarrierAtV1(history, projectionIndex) {
					t.Fatal("UNDELIVERED unresolved legacy projection created a barrier")
				}
				if availability, _, ok := findLatestAvailabilityContextWithSource(history); !ok || availability == nil {
					t.Fatal("UNDELIVERED unresolved projection hid prior valid prompt")
				}
			})
		})
	}
}

func TestAvailabilityPromptEventV1ResolvedLegacyProjectionRevalidatesDraftFacts(t *testing.T) {
	raw := availabilityPromptAuthorityRawResult()
	oldPrompt := availabilityPromptAuthorityMessage("resolved-invalid-old-prompt", "DELIVERED", raw, []int{0})
	oldSelection := legacyAvailabilitySelectionMessageForStateTest(
		"resolved-invalid-old-selection",
		raw,
		IntentSelectAvailabilityOption,
		oldPrompt.ReceivedAt.Add(2*time.Minute),
		oldPrompt.ID,
	)
	oldSelectionInbound := legacyAvailabilitySelectionInboundForStateTest(
		oldSelection,
		oldPrompt.ReceivedAt.Add(time.Minute),
	)
	bookablePrefix := []Message{oldPrompt, oldSelectionInbound, oldSelection}
	if state := bootstrapAvailabilitySelectionStateFromHistoryForTest(bookablePrefix); state.Status != AvailabilitySelectionStatusBookable {
		t.Fatalf("test precondition did not materialize historical BOOKABLE: %+v", state)
	}

	draft, projection := availabilityPromptAuthorityProjectionFixtureV1(
		t,
		raw,
		string(deliveredPromptProjectionDraftReviewV1),
		"APPROVED_AS_IS",
		"resolved-invalid-facts",
	)
	for _, message := range []*Message{&draft, &projection} {
		delete(message.Payload, testAvailabilityPromptEventV1MessageKey)
		delete(message.NormalizedPayload, testAvailabilityPromptEventV1MessageKey)
	}
	for _, root := range []map[string]interface{}{draft.Payload, draft.NormalizedPayload} {
		availability := asMap(asMap(root["tool_context"])[toolNameAvailabilitySearch])
		availability["date_from"] = "malformed-date"
	}
	delete(projection.Payload, "tool_context")
	delete(projection.NormalizedPayload, "tool_context")
	projection.Body = draft.Body
	history := append(append([]Message(nil), bookablePrefix...), draft, projection)
	projectionIndex := len(history) - 1

	resolved, sourceIndex, ok := resolveDeliveredPromptSourceMessageWithIndex(history, projectionIndex, projection)
	if !ok || sourceIndex != projectionIndex {
		t.Fatalf("expected projection link to resolve before factual validation: ok=%t index=%d", ok, sourceIndex)
	}
	resolvedAuthority := classifyAvailabilityPromptCandidateV1(resolved)
	if resolvedAuthority.Class != availabilityPromptAuthorityInvalidV1 || resolvedAuthority.Presented != nil {
		t.Fatalf("resolved malformed draft authority=%+v, want INVALID without Presented", resolvedAuthority)
	}
	if !deliveredInvalidAvailabilityPromptBarrierAtV1(history, projectionIndex) {
		t.Fatal("resolved projection with INVALID effective facts removed the temporal barrier")
	}
	if availability, foundIndex, found := findLatestAvailabilityContextWithSource(history); found || availability != nil || foundIndex != -1 {
		t.Fatalf("finder resurrected authority across resolved INVALID projection: ok=%t index=%d availability=%+v", found, foundIndex, availability)
	}
	state := bootstrapAvailabilitySelectionStateFromHistoryForTest(history)
	if state.Status == AvailabilitySelectionStatusBookable || availabilitySelectionStateHasAggregateV1(state) {
		t.Fatalf("resolved INVALID projection resurrected BOOKABLE: %+v", state)
	}
	draftContext := collectBookingDraftContextForRoutingBaseline(Session{}, history)
	if draftContext.HasAvailabilityShown || draftContext.HasBookableSelection ||
		draftContext.SelectedOptionIndex > 0 || strings.TrimSpace(draftContext.TripID) != "" ||
		strings.TrimSpace(draftContext.BoardStopID) != "" || strings.TrimSpace(draftContext.AlightStopID) != "" {
		t.Fatalf("resolved INVALID projection enriched booking draft: %+v", draftContext)
	}
	assertNoBookingCreateInputFromAvailabilityHistoryV1(t, history)
}

func assertNoBookingCreateInputFromAvailabilityHistoryV1(t *testing.T, history []Message) {
	t.Helper()
	session := Session{
		ContactKey:    "structural-reconciliation-test",
		CustomerPhone: "structural-reconciliation-test",
		CustomerName:  "Pessoa Teste",
	}
	session = sessionWithPassengerClarificationStateForTest(
		session,
		completePassengerStateForTest(1, 0),
	)
	if input, ok := parseBookingCreateInput(
		session,
		history,
		"quero reservar opção 1\nPessoa Teste | CPF | 84960815086",
		nil,
	); ok {
		t.Fatalf("irreconcilable history produced BookingCreateInput: %+v", input)
	}
}

func TestAvailabilityPromptEventV1DeliveryClassification(t *testing.T) {
	raw := availabilityPromptAuthorityRawResult()
	for _, test := range []struct {
		name       string
		status     string
		evidence   bool
		want       availabilityPromptAuthorityClassV1
		structural bool
	}{
		{name: "server ack", status: "SERVER_ACK", evidence: true, want: availabilityPromptAuthorityUndeliveredV1, structural: true},
		{name: "sent", status: "SENT", evidence: true, want: availabilityPromptAuthorityValidStructuralV1, structural: true},
		{name: "automation sent", status: "AUTOMATION_SENT", evidence: true, want: availabilityPromptAuthorityValidStructuralV1, structural: true},
		{name: "delivery ack", status: "DELIVERY_ACK", evidence: true, want: availabilityPromptAuthorityValidStructuralV1, structural: true},
		{name: "delivered", status: "DELIVERED", evidence: true, want: availabilityPromptAuthorityValidStructuralV1, structural: true},
		{name: "read", status: "READ", evidence: true, want: availabilityPromptAuthorityValidStructuralV1, structural: true},
		{name: "unknown", status: "PROCESSED", evidence: true, want: availabilityPromptAuthorityUndeliveredV1, structural: true},
		{name: "sent without evidence", status: "SENT", want: availabilityPromptAuthorityUndeliveredV1, structural: true},
		{name: "delivered legacy", status: "SENT", evidence: true, want: availabilityPromptAuthorityAbsentLegacyV1},
	} {
		t.Run(test.name, func(t *testing.T) {
			message := availabilityPromptAuthorityMessage("delivery-"+strings.ReplaceAll(test.name, " ", "-"), test.status, raw, []int{0})
			if !test.structural {
				delete(message.Payload, testAvailabilityPromptEventV1MessageKey)
				delete(message.NormalizedPayload, testAvailabilityPromptEventV1MessageKey)
			}
			if test.evidence {
				message.NormalizedPayload["delivery_recorded_at"] = time.Now().UTC().Format(time.RFC3339Nano)
			} else {
				delete(message.NormalizedPayload, "delivery_recorded_at")
			}
			if got := classifyAvailabilityPromptCandidateV1(message).Class; got != test.want {
				t.Fatalf("class=%s, want %s", got, test.want)
			}
		})
	}
}

func TestAvailabilityPromptEventV1DeliveryStatusIsMonotonic(t *testing.T) {
	sequence := []string{"SERVER_ACK", "SENT", "AUTOMATION_SENT", "DELIVERY_ACK", "SERVER_ACK", "DELIVERED", "UNKNOWN", "READ", "SENT"}
	status := ""
	for _, observed := range sequence {
		status = AdvanceDeliveryStatusV1(status, observed)
	}
	if status != "READ" {
		t.Fatalf("monotonic delivery regressed to %q", status)
	}
	if got := AdvanceDeliveryStatusV1("DELIVERY_ACK", "SERVER_ACK"); got != "DELIVERY_ACK" {
		t.Fatalf("late SERVER_ACK regressed DELIVERY_ACK to %q", got)
	}
	if got := AdvanceDeliveryStatusV1("DELIVERED", "UNRECOGNIZED"); got != "DELIVERED" {
		t.Fatalf("unknown status changed delivered status to %q", got)
	}
	if !DeliveryStatusConfirmsOutboundV1("SENT") || DeliveryStatusConfirmsOutboundV1("SERVER_ACK") {
		t.Fatal("sent/delivery confirmation predicates are inconsistent")
	}
	if !DeliveryStatusRecordsSentAtV1("DELIVERY_ACK") ||
		!DeliveryStatusRecordsDeliveredAtV1("DELIVERY_ACK") ||
		DeliveryStatusRecordsDeliveredAtV1("SENT") {
		t.Fatal("sent/delivered timestamp predicates are inconsistent")
	}
}

func TestAvailabilityPromptEventV1BotAndApprovedReviewProjectDeliveredIdentity(t *testing.T) {
	raw := availabilityPromptAuthorityRawResult()
	now := time.Date(2026, time.August, 2, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name         string
		mode         string
		reviewAction string
	}{
		{name: "bot auto reply", mode: "BOT_AUTO_REPLY"},
		{name: "approved review", mode: "DRAFT_REVIEW", reviewAction: "APPROVED_AS_IS"},
	} {
		t.Run(test.name, func(t *testing.T) {
			draftStatus := messageStatusAutomationSent
			if test.reviewAction != "" {
				draftStatus = messageStatusAutomationReviewed
			}
			draft := availabilityPromptAuthorityMessage("draft-"+strings.ReplaceAll(test.name, " ", "-"), draftStatus, raw, []int{2})
			draft.Payload["mode"] = messageStatusAutomationDraft
			draft.NormalizedPayload["mode"] = messageStatusAutomationDraft
			if test.reviewAction != "" {
				draft.Payload["review_action"] = test.reviewAction
				draft.NormalizedPayload["review_action"] = test.reviewAction
			}
			deliveredID := "delivered-" + strings.ReplaceAll(test.name, " ", "-")
			event, ok := availabilityPromptEventForOutboundV1(draft, deliveredID, true)
			if !ok || event.SourceMessageID != deliveredID {
				t.Fatalf("event did not project outbound identity: %+v", event)
			}
			payload := map[string]interface{}{
				"mode":                                  test.mode,
				"draft_message_id":                      draft.ID,
				testAvailabilityPromptEventV1MessageKey: event,
			}
			if test.reviewAction != "" {
				payload["review_action"] = test.reviewAction
			}
			delivered := Message{
				ID:                deliveredID,
				Direction:         "OUTBOUND",
				Kind:              "TEXT",
				Body:              "Texto efetivamente entregue com apresentação estrutural equivalente.",
				ProcessingStatus:  "DELIVERY_ACK",
				Payload:           availabilityPromptAuthorityCloneMap(t, payload),
				NormalizedPayload: availabilityPromptAuthorityCloneMap(t, payload),
				ReceivedAt:        now,
			}
			delivered.NormalizedPayload["delivery_recorded_at"] = now.Format(time.RFC3339Nano)
			resolved, _, ok := resolveDeliveredPromptSourceMessageWithIndex([]Message{draft, delivered}, 1, delivered)
			if !ok || resolved.ID != deliveredID {
				t.Fatalf("delivery did not reconcile draft facts: ok=%t resolved=%+v", ok, resolved)
			}
			authority := classifyAvailabilityPromptCandidateV1(resolved)
			if authority.Class != availabilityPromptAuthorityValidStructuralV1 ||
				authority.Presented == nil ||
				authority.Presented.Results[0].TripID != raw.Results[2].TripID {
				t.Fatalf("resolved delivery lost validated raw facts: %+v", authority)
			}
		})
	}
}

func TestAvailabilityPromptEventV1ProjectionRequiresReconciledDraftFacts(t *testing.T) {
	raw := availabilityPromptAuthorityRawResult()
	old := availabilityPromptAuthorityMessage("old-structural-prompt", "SENT", raw, []int{0})
	old.ReceivedAt = time.Date(2026, time.August, 2, 10, 0, 0, 0, time.UTC)
	old.CreatedAt = old.ReceivedAt
	selection := legacyAvailabilitySelectionMessageForStateTest(
		"old-bookable-selection",
		raw,
		IntentSelectAvailabilityOption,
		old.ReceivedAt.Add(2*time.Minute),
		old.ID,
	)
	selectionInbound := legacyAvailabilitySelectionInboundForStateTest(
		selection,
		old.ReceivedAt.Add(time.Minute),
	)
	bookableHistory := []Message{old, selectionInbound, selection}
	if state := bootstrapAvailabilitySelectionStateFromHistoryForTest(bookableHistory); state.Status != AvailabilitySelectionStatusBookable {
		t.Fatalf("test precondition did not materialize historical BOOKABLE authority: %+v", state)
	}
	historyWith := func(messages ...Message) []Message {
		history := append([]Message(nil), bookableHistory...)
		return append(history, messages...)
	}

	projectionKinds := []struct {
		name         string
		mode         string
		reviewAction string
	}{
		{name: "bot auto reply", mode: string(deliveredPromptProjectionBotAutoReplyV1)},
		{name: "approved review", mode: string(deliveredPromptProjectionDraftReviewV1), reviewAction: "APPROVED_AS_IS"},
	}
	invalidSources := []struct {
		name          string
		projectionRaw *AvailabilitySearchResult
		mutateHistory func(Message, Message) []Message
	}{
		{
			name:          "compatible projection facts with unresolvable draft id",
			projectionRaw: &raw,
			mutateHistory: func(_ Message, projection Message) []Message {
				for _, payload := range []map[string]interface{}{projection.Payload, projection.NormalizedPayload} {
					payload["draft_message_id"] = "missing-draft"
				}
				return historyWith(projection)
			},
		},
		{
			name: "residual projection facts from another draft with matching structural ids",
			projectionRaw: func() *AvailabilitySearchResult {
				residual := availabilityPromptAuthorityRawResult()
				residual.Filter.Origin = "Residual/XX"
				residual.Filter.Destination = "Alheio/YY"
				residual.Filter.PackageName = "Pacote residual"
				residual.Results[2].RouteID = "route-residual"
				residual.Results[2].OriginStopID = "origin-residual"
				residual.Results[2].DestinationStopID = "destination-residual"
				residual.Results[2].OriginDisplayName = "Residual/XX"
				residual.Results[2].DestinationDisplayName = "Alheio/YY"
				residual.Results[2].OriginDepartTime = "23:59"
				residual.Results[2].Price = 1
				residual.Results[2].PackageName = "Pacote residual"
				return &residual
			}(),
			mutateHistory: func(_ Message, projection Message) []Message {
				for _, payload := range []map[string]interface{}{projection.Payload, projection.NormalizedPayload} {
					payload["draft_message_id"] = "other-draft"
				}
				return historyWith(projection)
			},
		},
		{
			name: "bilateral event without facts and absent draft",
			mutateHistory: func(_ Message, projection Message) []Message {
				return historyWith(projection)
			},
		},
		{
			name: "webhook confirmed before truncated draft has structural evidence",
			mutateHistory: func(draft Message, projection Message) []Message {
				delete(draft.Payload, testAvailabilityPromptEventV1MessageKey)
				delete(draft.NormalizedPayload, testAvailabilityPromptEventV1MessageKey)
				delete(draft.Payload, "tool_context")
				delete(draft.NormalizedPayload, "tool_context")
				return historyWith(draft, projection)
			},
		},
	}

	for _, projectionKind := range projectionKinds {
		projectionKind := projectionKind
		t.Run(projectionKind.name, func(t *testing.T) {
			for _, invalidSource := range invalidSources {
				invalidSource := invalidSource
				t.Run(invalidSource.name, func(t *testing.T) {
					draft, projection := availabilityPromptAuthorityProjectionFixtureV1(
						t,
						raw,
						projectionKind.mode,
						projectionKind.reviewAction,
						"invalid-"+strings.ReplaceAll(projectionKind.name+"-"+invalidSource.name, " ", "-"),
					)
					if invalidSource.projectionRaw != nil {
						toolPayload := buildAvailabilityToolResponsePayload(*invalidSource.projectionRaw)
						for _, payload := range []map[string]interface{}{projection.Payload, projection.NormalizedPayload} {
							payload["tool_context"] = map[string]interface{}{
								toolNameAvailabilitySearch: availabilityPromptAuthorityCloneMap(t, toolPayload),
							}
						}
					}
					history := invalidSource.mutateHistory(draft, projection)

					authority := classifyAvailabilityPromptCandidateV1(projection)
					if authority.Class != availabilityPromptAuthorityInvalidV1 || authority.Presented != nil {
						t.Fatalf("projection authority=%+v, want INVALID without presented facts", authority)
					}
					if !deliveredInvalidAvailabilityPromptBarrierV1(projection) {
						t.Fatal("delivered invalid projection did not create temporal barrier")
					}
					if shouldMergeAvailabilityFactsFromMessage(projection) {
						t.Fatal("invalid projection was allowed to merge availability facts")
					}
					if availability, _, ok := findLatestAvailabilityContextWithSource(history); ok || availability != nil {
						t.Fatalf("older prompt crossed projection barrier: %+v", availability)
					}
					draftContext := collectBookingDraftContextForRoutingBaseline(Session{}, history)
					if draftContext.HasAvailabilityShown ||
						draftContext.TripID != "" ||
						draftContext.BoardStopID != "" ||
						draftContext.AlightStopID != "" ||
						draftContext.PackageName != "" ||
						draftContext.DepartureTime != "" ||
						draftContext.Price != 0 {
						t.Fatalf("invalid projection granted booking draft facts: %+v", draftContext)
					}
					state := bootstrapAvailabilitySelectionStateFromHistoryForTest(history)
					if state.Status == AvailabilitySelectionStatusBookable || availabilitySelectionStateHasAggregateV1(state) {
						t.Fatalf("invalid projection materialized BOOKABLE authority: %+v", state)
					}
					if _, _, ok := resolveBookingCreateSelectionFromState("quero reservar a opção 1", state); ok {
						t.Fatal("booking_create received authority from invalid projection")
					}
				})
			}
		})
	}
}

func TestAvailabilityPromptEventV1ProjectionCanonicalResolutionControls(t *testing.T) {
	raw := availabilityPromptAuthorityRawResult()
	for _, test := range []struct {
		name         string
		mode         string
		reviewAction string
	}{
		{name: "bot auto reply", mode: string(deliveredPromptProjectionBotAutoReplyV1)},
		{name: "approved review", mode: string(deliveredPromptProjectionDraftReviewV1), reviewAction: "APPROVED_AS_IS"},
	} {
		t.Run(test.name, func(t *testing.T) {
			draft, projection := availabilityPromptAuthorityProjectionFixtureV1(
				t,
				raw,
				test.mode,
				test.reviewAction,
				"control-"+strings.ReplaceAll(test.name, " ", "-"),
			)
			forged := availabilityPromptAuthorityRawResult()
			forged.Filter.PackageName = "Pacote da projeção"
			forged.Results[2].Price = 1
			forged.Results[2].OriginDepartTime = "23:59"
			forged.Results[2].RouteID = "route-projection"
			for _, payload := range []map[string]interface{}{projection.Payload, projection.NormalizedPayload} {
				payload["tool_context"] = map[string]interface{}{
					toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(forged),
				}
			}

			resolved, _, ok := resolveDeliveredPromptSourceMessageWithIndex(
				[]Message{draft, projection},
				1,
				projection,
			)
			if !ok {
				t.Fatal("correctly linked projection did not resolve canonical draft")
			}
			authority := classifyAvailabilityPromptCandidateV1(resolved)
			if authority.Class != availabilityPromptAuthorityValidStructuralV1 || authority.Presented == nil {
				t.Fatalf("resolved authority=%+v, want VALID_STRUCTURAL with facts", authority)
			}
			got := authority.Presented.Results[0]
			want := raw.Results[2]
			if !reflect.DeepEqual(got, want) ||
				authority.Presented.Filter.PackageName != raw.Filter.PackageName {
				t.Fatalf("resolved facts did not come exclusively from draft: got=%+v want=%+v", got, want)
			}

			undelivered := projection
			undelivered.ProcessingStatus = "SEND_FAILED"
			delete(undelivered.NormalizedPayload, "delivery_recorded_at")
			if got := classifyAvailabilityPromptCandidateV1(undelivered).Class; got != availabilityPromptAuthorityUndeliveredV1 {
				t.Fatalf("undelivered class=%s, want UNDELIVERED", got)
			}
			if deliveredInvalidAvailabilityPromptBarrierV1(undelivered) {
				t.Fatal("UNDELIVERED projection created a barrier")
			}
			old := availabilityPromptAuthorityMessage("old-before-undelivered-"+test.name, "SENT", raw, []int{0})
			if availability, _, ok := findLatestAvailabilityContextWithSource([]Message{old, undelivered}); !ok || availability == nil {
				t.Fatal("UNDELIVERED projection hid prior valid prompt")
			}
		})
	}
}

func availabilityPromptAuthorityProjectionFixtureV1(
	t *testing.T,
	raw AvailabilitySearchResult,
	mode string,
	reviewAction string,
	suffix string,
) (Message, Message) {
	t.Helper()
	draftStatus := messageStatusAutomationSent
	if reviewAction != "" {
		draftStatus = messageStatusAutomationReviewed
	}
	draft := availabilityPromptAuthorityMessage("draft-"+suffix, draftStatus, raw, []int{2})
	draft.Payload["mode"] = messageStatusAutomationDraft
	draft.NormalizedPayload["mode"] = messageStatusAutomationDraft
	if reviewAction != "" {
		draft.Payload["review_action"] = reviewAction
		draft.NormalizedPayload["review_action"] = reviewAction
	}
	at := time.Date(2026, time.August, 2, 12, 0, 0, 0, time.UTC)
	projection := deliveredAvailabilityPromptProjectionForStateTest(
		t,
		draft,
		"projection-"+suffix,
		mode,
		reviewAction,
		"Texto entregue",
		at,
	)
	return draft, projection
}

func TestAvailabilityPromptEventV1RejectsUnapprovedReviewAndProjectionWithoutFacts(t *testing.T) {
	raw := availabilityPromptAuthorityRawResult()
	now := time.Date(2026, time.August, 2, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name              string
		payloadAction     string
		normalizedAction  string
		includePayload    bool
		includeNormalized bool
	}{
		{name: "edited", payloadAction: "EDITED", normalizedAction: "EDITED", includePayload: true, includeNormalized: true},
		{name: "rejected", payloadAction: "REJECTED", normalizedAction: "REJECTED", includePayload: true, includeNormalized: true},
		{name: "missing action"},
		{name: "unknown action", payloadAction: "UNKNOWN", normalizedAction: "UNKNOWN", includePayload: true, includeNormalized: true},
		{name: "divergent action", payloadAction: "APPROVED_AS_IS", normalizedAction: "EDITED", includePayload: true, includeNormalized: true},
		{name: "unilateral action", payloadAction: "APPROVED_AS_IS", includePayload: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			message := availabilityPromptAuthorityMessage("review-"+strings.ReplaceAll(test.name, " ", "-"), "SENT", raw, []int{0})
			message.Payload["mode"] = "DRAFT_REVIEW"
			message.NormalizedPayload["mode"] = "DRAFT_REVIEW"
			if test.includePayload {
				message.Payload["review_action"] = test.payloadAction
			}
			if test.includeNormalized {
				message.NormalizedPayload["review_action"] = test.normalizedAction
			}
			message.NormalizedPayload["delivery_recorded_at"] = now.Format(time.RFC3339Nano)
			if got := classifyAvailabilityPromptCandidateV1(message).Class; got != availabilityPromptAuthorityInvalidV1 {
				t.Fatalf("class=%s, want INVALID", got)
			}
		})
	}

	draft := availabilityPromptAuthorityMessage("draft-without-facts", messageStatusAutomationDraft, raw, []int{0})
	delete(draft.Payload, "tool_context")
	delete(draft.NormalizedPayload, "tool_context")
	if _, ok := availabilityPromptEventForOutboundV1(draft, "outbound-without-validated-facts", true); ok {
		t.Fatal("projected a structural event from a draft without raw availability facts")
	}
}

func TestAvailabilityPromptEventV1InvalidDeliveredIsTemporalBarrier(t *testing.T) {
	raw := availabilityPromptAuthorityRawResult()
	old := availabilityPromptAuthorityMessage("old-valid", "SENT", raw, []int{0})
	invalid := availabilityPromptAuthorityMessage("new-invalid", "DELIVERED", raw, []int{1})
	invalid.Payload[testAvailabilityPromptEventV1MessageKey] = "malformed"
	invalid.NormalizedPayload[testAvailabilityPromptEventV1MessageKey] = "malformed"
	if !deliveredInvalidAvailabilityPromptBarrierV1(invalid) {
		t.Fatal("delivered INVALID did not create a barrier")
	}
	if got := latestDeliveredInvalidAvailabilityPromptIndexV1([]Message{old, invalid}, 2); got != 1 {
		t.Fatalf("barrier index=%d, want 1", got)
	}
	if availability, _, ok := findLatestAvailabilityContextWithSource([]Message{old, invalid}); ok || availability != nil {
		t.Fatalf("older prompt was resurrected through INVALID barrier: %+v", availability)
	}

	undelivered := availabilityPromptAuthorityMessage("new-undelivered-invalid", "SEND_FAILED", raw, []int{1})
	undelivered.Payload[testAvailabilityPromptEventV1MessageKey] = "malformed"
	undelivered.NormalizedPayload[testAvailabilityPromptEventV1MessageKey] = "malformed"
	delete(undelivered.NormalizedPayload, "delivery_recorded_at")
	if deliveredInvalidAvailabilityPromptBarrierV1(undelivered) {
		t.Fatal("UNDELIVERED candidate created a temporal barrier")
	}
	if availability, _, ok := findLatestAvailabilityContextWithSource([]Message{old, undelivered}); !ok || availability == nil {
		t.Fatal("UNDELIVERED candidate hid the previous valid prompt")
	}
}

func TestAvailabilityPromptInvalidDomainEvidencePreservesCandidateAndBarrierV1(t *testing.T) {
	raw := availabilityPromptAuthorityRawResult()
	oldPrompt := availabilityPromptAuthorityMessage("candidate-old-prompt", "DELIVERED", raw, []int{0})
	oldSelection := legacyAvailabilitySelectionMessageForStateTest(
		"candidate-old-selection",
		raw,
		IntentSelectAvailabilityOption,
		oldPrompt.ReceivedAt.Add(2*time.Minute),
		oldPrompt.ID,
	)
	oldSelectionInbound := legacyAvailabilitySelectionInboundForStateTest(
		oldSelection,
		oldPrompt.ReceivedAt.Add(time.Minute),
	)
	bookableHistory := []Message{oldPrompt, oldSelectionInbound, oldSelection}
	if state := bootstrapAvailabilitySelectionStateFromHistoryForTest(bookableHistory); state.Status != AvailabilitySelectionStatusBookable {
		t.Fatalf("test precondition did not materialize historical BOOKABLE authority: %+v", state)
	}

	passengerEvent := func(messageID string) PassengerClarificationEventV1 {
		event, ok := passengerClarificationPromptEventV1(ActivePromptPassengerCount, messageID)
		if !ok {
			t.Fatal("build passenger prompt event")
		}
		return event
	}
	invalidCases := []struct {
		name  string
		build func() Message
	}{
		{
			name: "canonical list body with bilateral passenger metadata",
			build: func() Message {
				message := availabilityPromptAuthorityMessage("candidate-list-body-passenger-metadata", "DELIVERED", raw, []int{0})
				message.Payload = map[string]interface{}{
					"intent":        string(IntentPassengerCountReply),
					"template_name": string(TemplateAskPassengerCount),
				}
				message.NormalizedPayload = cloneMap(message.Payload)
				message.NormalizedPayload["delivery_recorded_at"] = message.ReceivedAt.Format(time.RFC3339Nano)
				return message
			},
		},
		{
			name: "canonical earliest body with bilateral passenger event",
			build: func() Message {
				message := availabilityPromptAuthorityMessage("candidate-earliest-body-passenger-event", "DELIVERED", raw, []int{0})
				message.Body = buildEarliestAvailabilityReply(raw.Results[0])
				event := passengerEvent(message.ID)
				message.Payload = map[string]interface{}{passengerPromptEventV1MessageKey: event}
				message.NormalizedPayload = cloneMap(message.Payload)
				message.NormalizedPayload["delivery_recorded_at"] = message.ReceivedAt.Format(time.RFC3339Nano)
				return message
			},
		},
		{
			name: "canonical availability body with bilateral payment metadata",
			build: func() Message {
				message := availabilityPromptAuthorityMessage("candidate-body-payment-metadata", "DELIVERED", raw, []int{0})
				message.Payload = map[string]interface{}{
					"intent":        string(IntentPaymentPreference),
					"template_name": string(TemplateAskPaymentChoice),
				}
				message.NormalizedPayload = cloneMap(message.Payload)
				message.NormalizedPayload["delivery_recorded_at"] = message.ReceivedAt.Format(time.RFC3339Nano)
				return message
			},
		},
		{
			name: "unilateral availability metadata with passenger artifact",
			build: func() Message {
				message := availabilityPromptAuthorityMessage("candidate-unilateral-metadata", "DELIVERED", raw, []int{0})
				delete(message.Payload, testAvailabilityPromptEventV1MessageKey)
				delete(message.NormalizedPayload, testAvailabilityPromptEventV1MessageKey)
				delete(message.Payload, "tool_context")
				delete(message.NormalizedPayload, "tool_context")
				delete(message.NormalizedPayload, "intent")
				delete(message.Payload, "template_name")
				delete(message.NormalizedPayload, "template_name")
				event := passengerEvent(message.ID)
				message.Payload[passengerPromptEventV1MessageKey] = event
				message.NormalizedPayload[passengerPromptEventV1MessageKey] = event
				return message
			},
		},
		{
			name: "malformed tool context with availability metadata",
			build: func() Message {
				message := availabilityPromptAuthorityMessage("candidate-malformed-tool-metadata", "DELIVERED", raw, []int{0})
				delete(message.Payload, testAvailabilityPromptEventV1MessageKey)
				delete(message.NormalizedPayload, testAvailabilityPromptEventV1MessageKey)
				message.Payload["tool_context"] = "not-a-map"
				message.NormalizedPayload["tool_context"] = "not-a-map"
				return message
			},
		},
		{
			name: "malformed tool context with canonical availability body",
			build: func() Message {
				message := availabilityPromptAuthorityMessage("candidate-malformed-tool-body", "DELIVERED", raw, []int{0})
				message.Payload = map[string]interface{}{"tool_context": "not-a-map"}
				message.NormalizedPayload = map[string]interface{}{
					"tool_context":         "not-a-map",
					"delivery_recorded_at": message.ReceivedAt.Format(time.RFC3339Nano),
				}
				return message
			},
		},
	}

	for _, test := range invalidCases {
		t.Run(test.name, func(t *testing.T) {
			message := test.build()
			message.ReceivedAt = oldSelection.ReceivedAt.Add(time.Minute)
			message.CreatedAt = message.ReceivedAt
			authority := classifyAvailabilityPromptCandidateV1(message)
			if authority.Class != availabilityPromptAuthorityInvalidV1 || !authority.Candidate || authority.Facts != nil || authority.Presented != nil {
				t.Fatalf("authority=%+v, want delivered INVALID candidate with zero facts", authority)
			}
			history := append(append([]Message(nil), bookableHistory...), message)
			if !deliveredInvalidAvailabilityPromptBarrierAtV1(history, len(history)-1) {
				t.Fatal("delivered INVALID availability candidate did not form a barrier")
			}
			if availability, _, ok := findLatestAvailabilityContextWithSource(history); ok || availability != nil {
				t.Fatalf("historical availability crossed INVALID candidate barrier: %+v", availability)
			}
			state := bootstrapAvailabilitySelectionStateFromHistoryForTest(history)
			if state.Status == AvailabilitySelectionStatusBookable || availabilitySelectionStateHasAggregateV1(state) {
				t.Fatalf("INVALID candidate preserved BOOKABLE authority: %+v", state)
			}
			assertNoBookingCreateInputFromAvailabilityHistoryV1(t, history)

			undelivered := test.build()
			undelivered.ProcessingStatus = "SEND_FAILED"
			delete(undelivered.NormalizedPayload, "delivery_recorded_at")
			undeliveredAuthority := classifyAvailabilityPromptCandidateV1(undelivered)
			if undeliveredAuthority.Class != availabilityPromptAuthorityUndeliveredV1 || !undeliveredAuthority.Candidate || undeliveredAuthority.Facts != nil || undeliveredAuthority.Presented != nil {
				t.Fatalf("undelivered authority=%+v, want UNDELIVERED candidate with zero authority", undeliveredAuthority)
			}
			undeliveredHistory := append(append([]Message(nil), bookableHistory...), undelivered)
			if deliveredInvalidAvailabilityPromptBarrierAtV1(undeliveredHistory, len(undeliveredHistory)-1) {
				t.Fatal("UNDELIVERED candidate created a barrier")
			}
			if availability, _, ok := findLatestAvailabilityContextWithSource(undeliveredHistory); !ok || availability == nil {
				t.Fatal("UNDELIVERED candidate hid the previously delivered prompt")
			}
		})
	}

	passengerOnly := availabilityPromptAuthorityMessage("candidate-passenger-only", "DELIVERED", raw, []int{0})
	passengerOnly.Payload = map[string]interface{}{
		"intent":        string(IntentPassengerCountReply),
		"template_name": string(TemplateAskPassengerCount),
	}
	passengerOnly.NormalizedPayload = cloneMap(passengerOnly.Payload)
	passengerOnly.NormalizedPayload["delivery_recorded_at"] = passengerOnly.ReceivedAt.Format(time.RFC3339Nano)
	passengerOnly.Body = "A passagem é só para você ou vai mais alguém?"
	if authority := classifyAvailabilityPromptCandidateV1(passengerOnly); authority.Candidate || authority.Class != availabilityPromptAuthorityAbsentLegacyV1 {
		t.Fatalf("passenger-only authority=%+v, want non-candidate ABSENT_LEGACY", authority)
	}
	if deliveredInvalidAvailabilityPromptBarrierAtV1([]Message{passengerOnly}, 0) {
		t.Fatal("passenger-only message created an availability barrier")
	}

	paymentOnly := passengerOnly
	paymentOnly.ID = "candidate-payment-only"
	paymentOnly.Payload = map[string]interface{}{
		"intent":        string(IntentPaymentPreference),
		"template_name": string(TemplateAskPaymentChoice),
	}
	paymentOnly.NormalizedPayload = cloneMap(paymentOnly.Payload)
	paymentOnly.NormalizedPayload["delivery_recorded_at"] = paymentOnly.ReceivedAt.Format(time.RFC3339Nano)
	paymentOnly.Body = "Você prefere pagar à vista ou parcelado?"
	if authority := classifyAvailabilityPromptCandidateV1(paymentOnly); authority.Candidate || authority.Class != availabilityPromptAuthorityAbsentLegacyV1 {
		t.Fatalf("payment-only authority=%+v, want non-candidate ABSENT_LEGACY", authority)
	}
	if deliveredInvalidAvailabilityPromptBarrierAtV1([]Message{paymentOnly}, 0) {
		t.Fatal("payment-only message created an availability barrier")
	}

	generic := passengerOnly
	generic.ID = "candidate-generic-option-language"
	generic.Body = "Qual opção você prefere?"
	if authority := classifyAvailabilityPromptCandidateV1(generic); authority.Candidate {
		t.Fatalf("generic option language became availability evidence: %+v", authority)
	}

	valid := availabilityPromptAuthorityMessage("candidate-valid-canonical", "DELIVERED", raw, []int{0})
	if authority := classifyAvailabilityPromptCandidateV1(valid); authority.Class != availabilityPromptAuthorityValidStructuralV1 || !authority.Candidate || authority.Presented == nil {
		t.Fatalf("valid canonical availability prompt lost authority: %+v", authority)
	}
}

func TestCanonicalConversationStateRejectsUntrustedAvailabilityAuthorityClassesV1(t *testing.T) {
	raw := availabilityPromptAuthorityRawResult()
	selection := selectedAvailabilityResultPayloadFromAvailability(&raw, 1)
	selection[selectedAvailabilitySelectionMessageIDPayloadKey] = "merge-selection-inbound"
	selection[availabilityPromptSourceMessageIDPayloadKey] = "merge-prompt-source"

	build := func(status string) Message {
		message := availabilityPromptAuthorityMessage("merge-untrusted-"+strings.ToLower(status), status, raw, []int{0})
		for _, payload := range []map[string]interface{}{message.Payload, message.NormalizedPayload} {
			payload["selected_option_index"] = 1
			payload[selectedAvailabilityResultPayloadKey] = cloneMap(selection)
		}
		if status == "DELIVERED" {
			event, ok := passengerClarificationPromptEventV1(ActivePromptPassengerCount, message.ID)
			if !ok {
				t.Fatal("build passenger event")
			}
			message.Payload[passengerPromptEventV1MessageKey] = event
			message.NormalizedPayload[passengerPromptEventV1MessageKey] = event
		} else {
			delete(message.NormalizedPayload, "delivery_recorded_at")
		}
		return message
	}

	for _, test := range []struct {
		name  string
		class availabilityPromptAuthorityClassV1
		msg   Message
	}{
		{name: "invalid", class: availabilityPromptAuthorityInvalidV1, msg: build("DELIVERED")},
		{name: "undelivered", class: availabilityPromptAuthorityUndeliveredV1, msg: build("SEND_FAILED")},
	} {
		t.Run(test.name, func(t *testing.T) {
			authority := classifyAvailabilityPromptCandidateV1(test.msg)
			if authority.Class != test.class {
				t.Fatalf("class=%s, want %s", authority.Class, test.class)
			}
			if !legacyAvailabilitySelectionProjectionFactsV1(test.msg) {
				t.Fatal("test precondition did not retain decodable reconciled facts/selection")
			}
			if shouldMergeAvailabilityFactsFromMessage(test.msg) {
				t.Fatal("untrusted authority class was allowed to merge availability facts")
			}
			toolContext := cloneMap(asMap(test.msg.Payload["tool_context"]))
			toolContext[toolNamePaymentStatus] = map[string]interface{}{"status": "PENDING"}
			state := CanonicalConversationState{LastToolFacts: map[string]interface{}{}}
			mergeMessageToolFactsIntoCanonicalState(&state, test.msg, toolContext)
			if _, ok := state.LastToolFacts[toolNameAvailabilitySearch]; ok || state.Route.PackageName != "" {
				t.Fatalf("untrusted authority merged availability into canonical state: %+v", state)
			}
			if _, ok := state.LastToolFacts[toolNamePaymentStatus]; !ok {
				t.Fatalf("independent tool facts were removed with availability: %+v", state.LastToolFacts)
			}
		})
	}

	valid := availabilityPromptAuthorityMessage("merge-valid-structural", "DELIVERED", raw, []int{0})
	if !shouldMergeAvailabilityFactsFromMessage(valid) {
		t.Fatal("VALID_STRUCTURAL control lost merge authority")
	}
	legacy := legacyAvailabilityPromptMessageForStateTest("merge-valid-legacy", raw, valid.ReceivedAt)
	if authority := classifyAvailabilityPromptCandidateV1(legacy); authority.Class != availabilityPromptAuthorityAbsentLegacyV1 || !shouldMergeAvailabilityFactsFromMessage(legacy) {
		t.Fatalf("trusted ABSENT_LEGACY control lost merge authority: %+v", authority)
	}
}

func TestAvailabilityPromptEventV1RoutesOnlyPresentedOptions(t *testing.T) {
	raw := availabilityPromptAuthorityRawResult()
	now := time.Date(2026, time.August, 2, 12, 0, 0, 0, time.UTC)
	one := availabilityPromptAuthorityMessage("route-one", "SENT", raw, []int{3})
	one.ReceivedAt = now.Add(-time.Minute)
	state := discoveryState()
	for _, text := range []string{"sim", "1", "essa msm", "pode ser"} {
		decision := routeDeterministicIntent([]Message{one}, text, state, now)
		if decision.Intent != IntentSelectAvailabilityOption || decision.SelectedOptionIndex != 1 {
			t.Fatalf("%q did not select the sole presented option: %+v", text, decision)
		}
	}
	presented := visibleAvailabilityContextFromPromptMessage(one)
	if presented == nil || len(presented.Results) != 1 || presented.Results[0].TripID != raw.Results[3].TripID {
		t.Fatalf("display index 1 did not map to raw result 3: %+v", presented)
	}

	five := availabilityPromptAuthorityMessage("route-five", "SENT", raw, []int{0, 1, 2, 3, 4})
	five.ReceivedAt = now.Add(-time.Minute)
	decision := routeDeterministicIntent([]Message{five}, "sim", state, now)
	if decision.Intent == IntentSelectAvailabilityOption || decision.SelectedOptionIndex != 0 {
		t.Fatalf("ambiguous confirmation selected from five options: %+v", decision)
	}
}

func TestAvailabilityPromptEventV1AloneNeverGrantsBookableAuthorityAndSurvivesRoundTrip(t *testing.T) {
	raw := availabilityPromptAuthorityRawResult()
	prompt := availabilityPromptAuthorityMessage("roundtrip-prompt", "READ", raw, []int{0})
	encoded, err := json.Marshal(prompt)
	if err != nil {
		t.Fatalf("marshal prompt: %v", err)
	}
	var reloaded Message
	if err := json.Unmarshal(encoded, &reloaded); err != nil {
		t.Fatalf("reload prompt: %v", err)
	}
	authority := classifyAvailabilityPromptCandidateV1(reloaded)
	if authority.Class != availabilityPromptAuthorityValidStructuralV1 || authority.Presented == nil {
		t.Fatalf("structural prompt did not survive reload: %+v", authority)
	}
	state := bootstrapAvailabilitySelectionStateFromHistoryForTest([]Message{reloaded})
	if state.Status == AvailabilitySelectionStatusBookable || availabilitySelectionStateHasAggregateV1(state) {
		t.Fatalf("prompt event alone granted BOOKABLE: %+v", state)
	}
	if _, _, ok := resolveBookingCreateSelectionFromState("quero reservar", state); ok {
		t.Fatal("booking selection resolved without SELECTION_MATERIALIZED")
	}
}

func availabilityPromptAuthorityRawResult() AvailabilitySearchResult {
	result := AvailabilitySearchResult{
		Filter: AvailabilitySearchInput{
			Origin:      "Monção/MA",
			Destination: "Videira/SC",
			PackageName: packageToSantaCatarina,
			Limit:       8,
		},
		Results: make([]AvailabilitySearchItem, 0, 8),
	}
	for index := 0; index < 8; index++ {
		result.Results = append(result.Results, AvailabilitySearchItem{
			SegmentID:              fmt.Sprintf("segment-%d", index+1),
			TripID:                 fmt.Sprintf("trip-%d", index+1),
			RouteID:                "route-ma-sc",
			BoardStopID:            fmt.Sprintf("board-%d", index+1),
			AlightStopID:           fmt.Sprintf("alight-%d", index+1),
			OriginStopID:           "origin-moncao",
			DestinationStopID:      "destination-videira",
			OriginDisplayName:      "Monção/MA",
			DestinationDisplayName: "Videira/SC",
			OriginDepartTime:       fmt.Sprintf("%02d:00", 8+index),
			TripDate:               time.Date(2026, time.August, 3+index, 0, 0, 0, 0, time.UTC).Format("2006-01-02"),
			SeatsAvailable:         20 - index,
			Price:                  900 + float64(index*10),
			Currency:               "BRL",
			Status:                 "OPEN",
			TripStatus:             "SCHEDULED",
			PackageName:            packageToSantaCatarina,
		})
	}
	return result
}

func availabilityPromptAuthorityMessage(
	messageID string,
	status string,
	raw AvailabilitySearchResult,
	resultIndexes []int,
) Message {
	presentation := availabilityPromptPresentationV1(raw, resultIndexes)
	if presentation == nil {
		panic("invalid availability prompt authority fixture presentation")
	}
	event, ok := availabilityPromptEventForRunV1(
		RunAgentResult{AvailabilityPresentation: presentation},
		messageID,
	)
	if !ok {
		panic("invalid availability prompt authority fixture event")
	}
	toolPayload := buildAvailabilityToolResponsePayload(raw)
	payload := map[string]interface{}{
		"intent":                                string(IntentAvailabilitySearch),
		"template_name":                         string(TemplateAvailabilityList),
		testAvailabilityPromptEventV1MessageKey: event,
		"tool_context": map[string]interface{}{
			toolNameAvailabilitySearch: toolPayload,
		},
	}
	normalized := availabilityPromptAuthorityCloneMapWithoutTest(payload)
	recordedAt := time.Date(2026, time.August, 2, 12, 0, 0, 0, time.UTC)
	if DeliveryStatusConfirmsOutboundV1(status) {
		normalized["delivery_recorded_at"] = recordedAt.Format(time.RFC3339Nano)
	}
	return Message{
		ID:                messageID,
		Direction:         "OUTBOUND",
		Kind:              "TEXT",
		Body:              buildAvailabilityListReplyForResultIndexes(raw, resultIndexes),
		Payload:           payload,
		NormalizedPayload: normalized,
		ProcessingStatus:  status,
		ReceivedAt:        recordedAt,
		CreatedAt:         recordedAt,
	}
}

func availabilityPromptAuthorityEventMap(t *testing.T, raw interface{}) map[string]interface{} {
	t.Helper()
	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal availability prompt event: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatalf("decode availability prompt event map: %v", err)
	}
	return result
}

func availabilityPromptAuthorityCloneMap(t *testing.T, raw map[string]interface{}) map[string]interface{} {
	t.Helper()
	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal availability prompt fixture: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatalf("decode availability prompt fixture: %v", err)
	}
	return result
}

func availabilityPromptAuthorityCloneMapWithoutTest(raw map[string]interface{}) map[string]interface{} {
	encoded, err := json.Marshal(raw)
	if err != nil {
		panic(err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(encoded, &result); err != nil {
		panic(err)
	}
	return result
}

func rawAvailabilityContextFromPromptMessage(message Message) *AvailabilitySearchResult {
	contexts, present, valid := availabilityPromptRawContextsV1(message)
	if !present || !valid || len(contexts) == 0 {
		return nil
	}
	result := contexts[0]
	return &result
}

func TestAvailabilityPromptEventV1HelperCopiesStayIndependent(t *testing.T) {
	message := availabilityPromptAuthorityMessage("copy-proof", "SENT", availabilityPromptAuthorityRawResult(), []int{0})
	if reflect.ValueOf(message.Payload).Pointer() == reflect.ValueOf(message.NormalizedPayload).Pointer() {
		t.Fatal("payload copies unexpectedly share their root map")
	}
	event := availabilityPromptAuthorityEventMap(t, message.Payload[testAvailabilityPromptEventV1MessageKey])
	event["source_message_id"] = "mutated"
	if decoded, ok := decodeAvailabilityPromptEventV1(message.NormalizedPayload[testAvailabilityPromptEventV1MessageKey]); !ok || decoded.SourceMessageID != message.ID {
		t.Fatal("fixture mutation leaked across copies")
	}
}
