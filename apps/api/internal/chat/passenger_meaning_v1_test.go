package chat

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestPassengerMeaningV1SchemaIsStrict(t *testing.T) {
	schema := openAIPassengerClarificationMeaningV1JSONSchema()
	if schema["strict"] != true {
		t.Fatalf("passenger meaning schema must enable strict mode: %#v", schema)
	}
	assertPassengerMeaningV1StrictObject(t, schema["schema"], "root")
	raw, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	for _, forbidden := range []string{"action", "tool_call", "booking", "payment", "document", "price", "mutates_state", "sends_message"} {
		if strings.Contains(string(raw), `"`+forbidden+`"`) {
			t.Fatalf("schema must not expose executable field %q: %s", forbidden, raw)
		}
	}
}

func TestPassengerMeaningV1ValidatorDoesNotParseCurrentTurn(t *testing.T) {
	prompt, ok := passengerClarificationPromptEventV1(ActivePromptPassengerCount, "prompt-passenger-1")
	if !ok {
		t.Fatal("expected passenger prompt event")
	}
	state := newPassengerClarificationStateV1()
	state.HasEvidence = true
	state.PassengerSlotStatus = PassengerClarificationSlotOpen
	state.PassengerPromptMessageID = prompt.MessageID

	proposal := passengerMeaningV1KnownPassengerProposal("message-1", prompt.EventID, 3)
	result := ValidatePassengerClarificationMeaningV1(PassengerClarificationMeaningV1ValidationInput{
		Proposal:                    proposal,
		State:                       state,
		PromptEvent:                 prompt,
		ExpectedSourceMessageID:     "message-1",
		ExpectedSourcePromptEventID: prompt.EventID,
	})
	if !result.Accepted() {
		t.Fatalf("structured proposal should validate without receiving current-turn text: %+v", result)
	}
	validationType := reflect.TypeOf(PassengerClarificationMeaningV1ValidationInput{})
	if _, ok := validationType.FieldByName("CurrentTurn"); ok {
		t.Fatal("validator input must not contain current-turn text")
	}
}

func TestPassengerMeaningV1ValidatorRejectsEpochAndInvariantViolations(t *testing.T) {
	state, prompt := passengerMeaningV1PassengerPromptFixture()
	valid := passengerMeaningV1KnownPassengerProposal("message-1", prompt.EventID, 3)
	cases := []struct {
		name   string
		mutate func(*PassengerClarificationMeaningV1, *PassengerClarificationStateV1, *PassengerClarificationEventV1)
		reason string
	}{
		{
			name: "source message mismatch",
			mutate: func(proposal *PassengerClarificationMeaningV1, _ *PassengerClarificationStateV1, _ *PassengerClarificationEventV1) {
				proposal.SourceMessageID = "other-message"
			},
			reason: "source_message_mismatch",
		},
		{
			name: "stale prompt",
			mutate: func(_ *PassengerClarificationMeaningV1, state *PassengerClarificationStateV1, _ *PassengerClarificationEventV1) {
				state.PassengerPromptMessageID = "new-prompt"
			},
			reason: "stale_prompt_epoch",
		},
		{
			name: "child count exceeds total",
			mutate: func(proposal *PassengerClarificationMeaningV1, _ *PassengerClarificationStateV1, _ *PassengerClarificationEventV1) {
				proposal.ChildUnder5 = PassengerChildUnder5MeaningV1{Status: PassengerMeaningStatusKnown, Count: passengerMeaningV1Int(4)}
				proposal.NeedsClarification = false
				proposal.MissingFields = nil
			},
			reason: "children_exceed_passenger_total",
		},
		{
			name: "incomplete correction",
			mutate: func(proposal *PassengerClarificationMeaningV1, _ *PassengerClarificationStateV1, _ *PassengerClarificationEventV1) {
				proposal.Correction = PassengerMeaningCorrectionV1{Present: true, Replaces: PassengerMeaningCorrectionChildAggregate}
				proposal.ReasonCodes = append(proposal.ReasonCodes, PassengerMeaningReasonCorrectionStated)
			},
			reason: "incomplete_child_correction",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			proposal := valid
			proposal.MissingFields = append([]PassengerMeaningMissingFieldV1(nil), valid.MissingFields...)
			proposal.ReasonCodes = append([]PassengerMeaningReasonCodeV1(nil), valid.ReasonCodes...)
			caseState := clonePassengerMeaningV1State(state)
			casePrompt := clonePassengerMeaningV1Prompt(prompt)
			testCase.mutate(&proposal, &caseState, &casePrompt)
			result := ValidatePassengerClarificationMeaningV1(PassengerClarificationMeaningV1ValidationInput{
				Proposal:                    proposal,
				State:                       caseState,
				PromptEvent:                 casePrompt,
				ExpectedSourceMessageID:     "message-1",
				ExpectedSourcePromptEventID: prompt.EventID,
			})
			if result.Accepted() || !passengerMeaningV1StringContains(result.ReasonCodes, testCase.reason) {
				t.Fatalf("expected deterministic rejection %q, got %+v", testCase.reason, result)
			}
		})
	}
}

func TestPassengerMeaningV1ValidatorRejectsChildReferencesExceedingAbsolutePassengerTotal(t *testing.T) {
	state, prompt := passengerMeaningV1PassengerPromptFixture()
	buildProposal := func(total *int, referenceIDs ...string) PassengerClarificationMeaningV1 {
		proposal := passengerMeaningV1KnownPassengerProposal("message-child-reference-total", prompt.EventID, 2)
		proposal.PassengerCount = PassengerCountMeaningV1{
			Status:     PassengerMeaningStatusKnown,
			Value:      total,
			Provenance: PassengerCountProvenanceAbsoluteTotal,
		}
		proposal.ChildUnder5 = PassengerChildUnder5MeaningV1{
			Status: PassengerMeaningStatusKnown,
			Count:  passengerMeaningV1Int(0),
		}
		for _, referenceID := range referenceIDs {
			proposal.ChildUnder5.References = append(
				proposal.ChildUnder5.References,
				passengerMeaningV1AgedChildReference(referenceID, 6, PassengerChildAgeUnitYears),
			)
		}
		proposal.NeedsClarification = false
		proposal.MissingFields = nil
		proposal.ReasonCodes = []PassengerMeaningReasonCodeV1{
			PassengerMeaningReasonPassengerCountStated,
			PassengerMeaningReasonChildAgeStated,
		}
		return proposal
	}

	t.Run("three distinct children exceed absolute total two", func(t *testing.T) {
		proposal := buildProposal(passengerMeaningV1Int(2), "child_1", "child_2", "child_3")
		result := validatePassengerMeaningV1TestProposal(proposal, state, prompt)
		if result.Accepted() || !passengerMeaningV1StringContains(result.ReasonCodes, "children_exceed_passenger_total") {
			t.Fatalf("three distinct child identities must not fit absolute passenger total two: %+v", result)
		}
	})

	for _, referenceIDs := range [][]string{
		nil,
		{"child_1"},
		{"child_1", "child_2"},
	} {
		name := "references_" + string(rune('0'+len(referenceIDs)))
		t.Run(name, func(t *testing.T) {
			proposal := buildProposal(passengerMeaningV1Int(2), referenceIDs...)
			if result := validatePassengerMeaningV1TestProposal(proposal, state, prompt); !result.Accepted() {
				t.Fatalf("absolute passenger total two must accept %d coherent child identities: %+v", len(referenceIDs), result)
			}
		})
	}

	t.Run("duplicate identity remains owned by duplicate invariant", func(t *testing.T) {
		proposal := buildProposal(passengerMeaningV1Int(2), "child_1", "child_1", "child_2")
		result := validatePassengerMeaningV1TestProposal(proposal, state, prompt)
		if result.Accepted() || !passengerMeaningV1StringContains(result.ReasonCodes, "duplicate_child_reference_id") {
			t.Fatalf("duplicate child identity must remain rejected by the existing invariant: %+v", result)
		}
		if passengerMeaningV1StringContains(result.ReasonCodes, "children_exceed_passenger_total") {
			t.Fatalf("duplicate identity must not be counted as another traveler: %+v", result)
		}
	})

	t.Run("unknown passenger total does not gain absolute-total invariant", func(t *testing.T) {
		proposal := buildProposal(nil, "child_1", "child_2", "child_3")
		proposal.PassengerCount = PassengerCountMeaningV1{
			Status:     PassengerMeaningStatusUnknown,
			Provenance: PassengerCountProvenanceUnknown,
		}
		proposal.NeedsClarification = true
		proposal.MissingFields = []PassengerMeaningMissingFieldV1{PassengerMeaningMissingPassengerCount}
		proposal.ReasonCodes = []PassengerMeaningReasonCodeV1{PassengerMeaningReasonChildAgeStated}
		if result := validatePassengerMeaningV1TestProposal(proposal, state, prompt); !result.Accepted() {
			t.Fatalf("without an absolute passenger total, references must remain eligible for clarification: %+v", result)
		}
	})
}

func TestPassengerMeaningV1ValidatorRejectsPassengerTotalContradictingSnapshotWithoutCorrection(t *testing.T) {
	state, prompt := passengerMeaningV1ChildPromptFixture(3)
	proposal := passengerMeaningV1KnownPassengerProposal("message-snapshot-conflict", prompt.EventID, 99)
	result := ValidatePassengerClarificationMeaningV1(PassengerClarificationMeaningV1ValidationInput{
		Proposal:                    proposal,
		State:                       state,
		PromptEvent:                 prompt,
		ExpectedSourceMessageID:     proposal.SourceMessageID,
		ExpectedSourcePromptEventID: prompt.EventID,
	})
	if result.Accepted() || !passengerMeaningV1StringContains(result.ReasonCodes, "passenger_count_conflicts_with_snapshot") {
		t.Fatalf("proposal total 99 must not overwrite structured snapshot total 3 without compatible correction: %+v", result)
	}

	proposal.Correction = PassengerMeaningCorrectionV1{Present: true, Replaces: PassengerMeaningCorrectionPassengerAggregate}
	proposal.ReasonCodes = append(proposal.ReasonCodes, PassengerMeaningReasonCorrectionStated)
	corrected := ValidatePassengerClarificationMeaningV1(PassengerClarificationMeaningV1ValidationInput{
		Proposal:                    proposal,
		State:                       state,
		PromptEvent:                 prompt,
		ExpectedSourceMessageID:     proposal.SourceMessageID,
		ExpectedSourcePromptEventID: prompt.EventID,
	})
	if !corrected.Accepted() {
		t.Fatalf("explicit passenger aggregate correction must be compatible with changing snapshot total 3: %+v", corrected)
	}
}

func TestPassengerMeaningV1ValidatorPassengerCountFactualMatrix(t *testing.T) {
	tests := []struct {
		name       string
		status     PassengerMeaningStatusV1
		provenance PassengerCountProvenance
		value      *int
		accepted   bool
	}{
		{name: "solo speaker one", status: PassengerMeaningStatusKnown, provenance: PassengerCountProvenanceSoloSpeaker, value: passengerMeaningV1Int(1), accepted: true},
		{name: "solo speaker three", status: PassengerMeaningStatusKnown, provenance: PassengerCountProvenanceSoloSpeaker, value: passengerMeaningV1Int(3)},
		{name: "includes speaker two", status: PassengerMeaningStatusKnown, provenance: PassengerCountProvenanceIncludesSpeakerComposition, value: passengerMeaningV1Int(2), accepted: true},
		{name: "includes speaker one", status: PassengerMeaningStatusKnown, provenance: PassengerCountProvenanceIncludesSpeakerComposition, value: passengerMeaningV1Int(1)},
		{name: "absolute total one", status: PassengerMeaningStatusKnown, provenance: PassengerCountProvenanceAbsoluteTotal, value: passengerMeaningV1Int(1), accepted: true},
		{name: "unknown", status: PassengerMeaningStatusUnknown, provenance: PassengerCountProvenanceUnknown, accepted: true},
		{name: "subgroup only", status: PassengerMeaningStatusUnknown, provenance: PassengerCountProvenanceSubgroupOnly, accepted: true},
		{name: "conflicting", status: PassengerMeaningStatusConflicting, provenance: PassengerCountProvenanceUnknown, accepted: true},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			reasons := validatePassengerCountMeaningV1(PassengerCountMeaningV1{
				Status:     testCase.status,
				Value:      testCase.value,
				Provenance: testCase.provenance,
			})
			if accepted := len(reasons) == 0; accepted != testCase.accepted {
				t.Fatalf("accepted=%t, want %t: reasons=%v", accepted, testCase.accepted, reasons)
			}
		})
	}
}

func TestPassengerMeaningV1ValidatorRejectsPassengerProvenanceContradictingSnapshotWithoutCorrection(t *testing.T) {
	state, prompt := passengerMeaningV1ChildPromptFixture(3)
	proposal := passengerMeaningV1KnownPassengerProposal("message-provenance-conflict", prompt.EventID, 3)
	result := validatePassengerMeaningV1TestProposal(proposal, state, prompt)
	if result.Accepted() || !passengerMeaningV1StringContains(result.ReasonCodes, "passenger_count_conflicts_with_snapshot") {
		t.Fatalf("proposal must preserve known snapshot provenance without compatible correction: %+v", result)
	}

	proposal.Correction = PassengerMeaningCorrectionV1{Present: true, Replaces: PassengerMeaningCorrectionPassengerAggregate}
	proposal.ReasonCodes = append(proposal.ReasonCodes, PassengerMeaningReasonCorrectionStated)
	corrected := validatePassengerMeaningV1TestProposal(proposal, state, prompt)
	if !corrected.Accepted() {
		t.Fatalf("passenger aggregate correction must allow replacing known provenance: %+v", corrected)
	}
}

func TestPassengerMeaningV1ValidatorReconcilesKnownChildReferencesByCorrectionCoverage(t *testing.T) {
	state, prompt, valid := passengerMeaningV1KnownChildSnapshotFixture()
	tests := []struct {
		name       string
		correction PassengerMeaningCorrectionTargetV1
		mutate     func(*PassengerClarificationMeaningV1)
		accepted   bool
	}{
		{name: "none preserves reference", correction: PassengerMeaningCorrectionNone, accepted: true},
		{name: "none rejects omitted reference", correction: PassengerMeaningCorrectionNone, mutate: func(proposal *PassengerClarificationMeaningV1) {
			proposal.ChildUnder5.References = []PassengerChildReferenceMeaningV1{passengerMeaningV1AgedChildReference("child_replacement", 3, PassengerChildAgeUnitYears)}
		}},
		{name: "none rejects reclassified reference", correction: PassengerMeaningCorrectionNone, mutate: func(proposal *PassengerClarificationMeaningV1) {
			proposal.ChildUnder5.References = []PassengerChildReferenceMeaningV1{
				passengerMeaningV1AgedChildReference("child_1", 6, PassengerChildAgeUnitYears),
				passengerMeaningV1AgedChildReference("child_replacement", 3, PassengerChildAgeUnitYears),
			}
		}},
		{name: "none rejects changed relation", correction: PassengerMeaningCorrectionNone, mutate: func(proposal *PassengerClarificationMeaningV1) {
			proposal.ChildUnder5.References[0].Relation = PassengerChildRelationOther
		}},
		{name: "passenger correction does not replace child references", correction: PassengerMeaningCorrectionPassengerAggregate, mutate: func(proposal *PassengerClarificationMeaningV1) {
			proposal.ChildUnder5.References = []PassengerChildReferenceMeaningV1{passengerMeaningV1AgedChildReference("child_replacement", 3, PassengerChildAgeUnitYears)}
		}},
		{name: "child correction replaces child references", correction: PassengerMeaningCorrectionChildAggregate, mutate: func(proposal *PassengerClarificationMeaningV1) {
			proposal.ChildUnder5.References = []PassengerChildReferenceMeaningV1{passengerMeaningV1AgedChildReference("child_replacement", 3, PassengerChildAgeUnitYears)}
		}, accepted: true},
		{name: "full correction replaces child references", correction: PassengerMeaningCorrectionFullAggregate, mutate: func(proposal *PassengerClarificationMeaningV1) {
			proposal.ChildUnder5.References = []PassengerChildReferenceMeaningV1{passengerMeaningV1AgedChildReference("child_replacement", 3, PassengerChildAgeUnitYears)}
		}, accepted: true},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			proposal := clonePassengerMeaningV1TestProposal(valid)
			if testCase.mutate != nil {
				testCase.mutate(&proposal)
			}
			if testCase.correction != PassengerMeaningCorrectionNone {
				proposal.Correction = PassengerMeaningCorrectionV1{Present: true, Replaces: testCase.correction}
				proposal.ReasonCodes = append(proposal.ReasonCodes, PassengerMeaningReasonCorrectionStated)
			}
			result := validatePassengerMeaningV1TestProposal(proposal, state, prompt)
			if result.Accepted() != testCase.accepted {
				t.Fatalf("accepted=%t, want %t: %+v", result.Accepted(), testCase.accepted, result)
			}
			if !testCase.accepted && !passengerMeaningV1StringContains(result.ReasonCodes, "child_references_conflict_with_snapshot") {
				t.Fatalf("expected child reference conflict, got %+v", result)
			}
		})
	}
}

func TestPassengerMeaningV1ValidatorRejectsPassengerOnlyCorrectionOfSoloChildDependency(t *testing.T) {
	state, _, proposal := passengerMeaningV1KnownChildSnapshotFixture()
	state.PassengerCountKnown = true
	state.PassengerCount = 1
	state.PassengerCountProvenance = PassengerCountProvenanceSoloSpeaker
	state.PassengerSlotStatus = PassengerClarificationSlotAnswered
	state.ChildUnder5AddsTraveler = true
	state.ChildUnder5AddsTravelerOrigin = PassengerClarificationAddsTravelerOriginV1{
		SourceMessageID:          "message-child-snapshot",
		SourcePromptMessageID:    state.ChildPromptMessageID,
		PassengerSourceMessageID: "message-solo-snapshot",
		ReasonCode:               passengerClarificationReasonSoloChildAddsTraveler,
	}
	proposal.PassengerCount = PassengerCountMeaningV1{
		Status:     PassengerMeaningStatusKnown,
		Value:      passengerMeaningV1Int(2),
		Provenance: PassengerCountProvenanceAbsoluteTotal,
	}
	proposal.Correction = PassengerMeaningCorrectionV1{Present: true, Replaces: PassengerMeaningCorrectionPassengerAggregate}
	reasons := validatePassengerMeaningV1AgainstSnapshot(proposal, state)
	if !passengerMeaningV1StringContains(reasons, "passenger_correction_conflicts_with_child_dependency") {
		t.Fatalf("passenger-only correction must not silently retain a solo-child dependency: %v", reasons)
	}

	proposal.Correction.Replaces = PassengerMeaningCorrectionFullAggregate
	fullReasons := validatePassengerMeaningV1AgainstSnapshot(proposal, state)
	if passengerMeaningV1StringContains(fullReasons, "passenger_correction_conflicts_with_child_dependency") {
		t.Fatalf("full aggregate correction must cover the solo-child dependency: %v", fullReasons)
	}
}

func TestPassengerMeaningV1SchemaRunnerDisablesStorageAndTools(t *testing.T) {
	state, prompt := passengerMeaningV1PassengerPromptFixture()
	runner := &OpenAIPassengerMeaningV1Runner{model: "test-model"}
	payload := runner.buildRequestPayload(OpenAIPassengerMeaningV1RunInput{
		CurrentTurn:         "eu e mais duas crianças",
		State:               state,
		PromptEvent:         prompt,
		SourceMessageID:     "message-1",
		SourcePromptEventID: prompt.EventID,
		IdempotencyKey:      "request-1",
	})
	if payload["store"] != false || payload["tool_choice"] != "none" {
		t.Fatalf("runner must disable storage and tool choice: %#v", payload)
	}
	tools, ok := payload["tools"].([]interface{})
	if !ok || len(tools) != 0 {
		t.Fatalf("runner must send an explicit empty tools array: %#v", payload["tools"])
	}
	text, ok := payload["text"].(map[string]interface{})
	if !ok || text["format"] == nil {
		t.Fatalf("runner must use structured text.format: %#v", payload)
	}
	input, ok := payload["input"].(string)
	if !ok || strings.Contains(input, "expected") {
		t.Fatalf("provider input must be compact and contain no expected fixture: %q", input)
	}
}

func TestPassengerMeaningV1SchemaPayloadRejectsUnknownExecutableFields(t *testing.T) {
	proposal := passengerMeaningV1KnownPassengerProposal("message-1", "prompt-event-1", 3)
	raw, err := json.Marshal(proposal)
	if err != nil {
		t.Fatalf("marshal proposal: %v", err)
	}
	var object map[string]interface{}
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatalf("decode proposal: %v", err)
	}
	object["tool_call"] = map[string]interface{}{"name": "forbidden"}
	withTool, err := json.Marshal(object)
	if err != nil {
		t.Fatalf("marshal hostile proposal: %v", err)
	}
	validation := validateOpenAIPassengerMeaningV1Payload(withTool)
	if validation.Valid || !passengerMeaningV1StringContains(validation.ReasonCodes, "invalid_json_shape") {
		t.Fatalf("unknown executable field must fail strict decode: %+v", validation)
	}
}

func TestPassengerMeaningV1CorpusRealChildCompositionCases(t *testing.T) {
	for _, currentTurn := range []string{
		"eu e mais 2 crianças",
		"eu e mais duas crianças",
	} {
		t.Run(currentTurn, func(t *testing.T) {
			actual := passengerMeaningV1KnownPassengerProposal("message-real", "prompt-event-real", 3)
			if actual.PassengerCount.Status != PassengerMeaningStatusKnown ||
				actual.PassengerCount.Value == nil || *actual.PassengerCount.Value != 3 ||
				actual.PassengerCount.Provenance != PassengerCountProvenanceIncludesSpeakerComposition {
				t.Fatalf("%q must propose passenger_count KNOWN=3 with includes-speaker provenance: %+v", currentTurn, actual.PassengerCount)
			}
			if actual.ChildUnder5.Status != PassengerMeaningStatusUnknown || actual.ChildUnder5.Count != nil {
				t.Fatalf("%q must not infer under-5 count without sufficient age: %+v", currentTurn, actual.ChildUnder5)
			}
		})
	}
}

func assertPassengerMeaningV1StrictObject(t *testing.T, raw interface{}, path string) {
	t.Helper()
	object, ok := raw.(map[string]interface{})
	if !ok {
		t.Fatalf("%s must be an object schema: %#v", path, raw)
	}
	if object["type"] == "object" && object["additionalProperties"] != false {
		t.Fatalf("%s must reject additional properties: %#v", path, object)
	}
	properties, _ := object["properties"].(map[string]interface{})
	if object["type"] == "object" {
		required, _ := object["required"].([]string)
		if required == nil {
			if rawRequired, ok := object["required"].([]interface{}); ok {
				for _, value := range rawRequired {
					required = append(required, value.(string))
				}
			}
		}
		sort.Strings(required)
		propertyNames := make([]string, 0, len(properties))
		for name := range properties {
			propertyNames = append(propertyNames, name)
		}
		sort.Strings(propertyNames)
		if !reflect.DeepEqual(required, propertyNames) {
			t.Fatalf("%s must require every declared property: required=%v properties=%v", path, required, propertyNames)
		}
	}
	for name, property := range properties {
		child, _ := property.(map[string]interface{})
		if child["type"] == "object" {
			assertPassengerMeaningV1StrictObject(t, child, path+"."+name)
		}
		if items, ok := child["items"].(map[string]interface{}); ok && items["type"] == "object" {
			assertPassengerMeaningV1StrictObject(t, items, path+"."+name+"[]")
		}
		if alternatives, ok := child["anyOf"].([]interface{}); ok {
			for index, alternative := range alternatives {
				candidate, _ := alternative.(map[string]interface{})
				if candidate["type"] == "object" {
					assertPassengerMeaningV1StrictObject(t, candidate, path+"."+name+".anyOf["+string(rune('0'+index))+"]")
				}
			}
		}
	}
}

func passengerMeaningV1KnownPassengerProposal(messageID string, promptEventID string, count int) PassengerClarificationMeaningV1 {
	return PassengerClarificationMeaningV1{
		Version:             passengerClarificationMeaningV1Version,
		SourceMessageID:     messageID,
		SourcePromptEventID: promptEventID,
		PassengerCount: PassengerCountMeaningV1{
			Status:     PassengerMeaningStatusKnown,
			Value:      &count,
			Provenance: PassengerCountProvenanceIncludesSpeakerComposition,
		},
		ChildUnder5: PassengerChildUnder5MeaningV1{
			Status: PassengerMeaningStatusUnknown,
		},
		Correction: PassengerMeaningCorrectionV1{
			Replaces: PassengerMeaningCorrectionNone,
		},
		NeedsClarification: true,
		MissingFields:      []PassengerMeaningMissingFieldV1{PassengerMeaningMissingChildUnder5},
		Confidence:         0.99,
		ReasonCodes:        []PassengerMeaningReasonCodeV1{PassengerMeaningReasonIncludesSpeakerComposition},
	}
}

func passengerMeaningV1KnownChildSnapshotFixture() (PassengerClarificationStateV1, PassengerClarificationEventV1, PassengerClarificationMeaningV1) {
	state, prompt := passengerMeaningV1PassengerPromptFixture()
	state.ChildUnder5CountKnown = true
	state.ChildUnder5Count = 1
	state.ChildSlotStatus = PassengerClarificationSlotAnswered
	state.ChildPromptMessageID = "prompt-child-snapshot"
	state.ChildLastMessageID = "message-child-snapshot"
	state.ChildReferences = []PassengerClarificationChildReferenceV1{{ID: "child_1", Under5: true, AgeKnown: true}}

	proposal := passengerMeaningV1KnownPassengerProposal("message-child-reference", prompt.EventID, 3)
	under5 := true
	proposal.ChildUnder5 = PassengerChildUnder5MeaningV1{
		Status: PassengerMeaningStatusKnown,
		Count:  passengerMeaningV1Int(1),
		References: []PassengerChildReferenceMeaningV1{{
			ReferenceID: "child_1",
			Relation:    PassengerChildRelationChild,
			AgeUnit:     PassengerChildAgeUnitUnknown,
			Under5:      &under5,
		}},
	}
	proposal.NeedsClarification = false
	proposal.MissingFields = nil
	return state, prompt, proposal
}

func validatePassengerMeaningV1TestProposal(proposal PassengerClarificationMeaningV1, state PassengerClarificationStateV1, prompt PassengerClarificationEventV1) PassengerClarificationMeaningV1ValidationResult {
	return ValidatePassengerClarificationMeaningV1(PassengerClarificationMeaningV1ValidationInput{
		Proposal:                    proposal,
		State:                       state,
		PromptEvent:                 prompt,
		ExpectedSourceMessageID:     proposal.SourceMessageID,
		ExpectedSourcePromptEventID: prompt.EventID,
	})
}

func clonePassengerMeaningV1TestProposal(proposal PassengerClarificationMeaningV1) PassengerClarificationMeaningV1 {
	cloned := proposal
	cloned.PassengerCount = clonePassengerCountMeaningV1(proposal.PassengerCount)
	cloned.ChildUnder5 = clonePassengerChildUnder5MeaningV1(proposal.ChildUnder5)
	cloned.MissingFields = append([]PassengerMeaningMissingFieldV1(nil), proposal.MissingFields...)
	cloned.ReasonCodes = append([]PassengerMeaningReasonCodeV1(nil), proposal.ReasonCodes...)
	return cloned
}

func passengerMeaningV1StringContains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
