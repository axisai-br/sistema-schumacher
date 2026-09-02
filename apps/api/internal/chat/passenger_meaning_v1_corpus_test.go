package chat

import (
	"context"
	"reflect"
	"sync"
	"testing"
)

type fixedPassengerMeaningV1Interpreter struct {
	mu     sync.Mutex
	result OpenAIPassengerMeaningV1RunResult
	err    error
	inputs []OpenAIPassengerMeaningV1RunInput
}

func (f *fixedPassengerMeaningV1Interpreter) Enabled() bool { return true }

func (f *fixedPassengerMeaningV1Interpreter) InterpretPassengerMeaningV1(_ context.Context, input OpenAIPassengerMeaningV1RunInput) (OpenAIPassengerMeaningV1RunResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inputs = append(f.inputs, input)
	return f.result, f.err
}

func (f *fixedPassengerMeaningV1Interpreter) snapshotInputs() []OpenAIPassengerMeaningV1RunInput {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]OpenAIPassengerMeaningV1RunInput(nil), f.inputs...)
}

type passengerMeaningV1CorpusCase struct {
	ID       string
	Input    passengerMeaningV1CorpusInput
	Expected PassengerClarificationMeaningV1
}

type passengerMeaningV1CorpusInput struct {
	Turn                string
	State               PassengerClarificationStateV1
	Prompt              PassengerClarificationEventV1
	SourceMessageID     string
	SourcePromptEventID string
}

type passengerMeaningV1CorpusSequence struct {
	ID    string
	Turns []passengerMeaningV1CorpusCase
}

func TestPassengerMeaningV1Corpus(t *testing.T) {
	for _, corpusCase := range passengerMeaningV1Corpus() {
		corpusCase := corpusCase
		t.Run(corpusCase.ID, func(t *testing.T) {
			runPassengerMeaningV1CorpusCase(t, corpusCase)
		})
	}
	for _, sequence := range passengerMeaningV1SequentialCorpus() {
		sequence := sequence
		t.Run(sequence.ID, func(t *testing.T) {
			runPassengerMeaningV1CorpusSequence(t, sequence)
		})
	}
}

func runPassengerMeaningV1CorpusCase(t *testing.T, corpusCase passengerMeaningV1CorpusCase) (PassengerClarificationMeaningV1, OpenAIPassengerMeaningV1RunInput) {
	t.Helper()
	actual := passengerMeaningV1CorpusProviderOutput(corpusCase.ID, corpusCase.Input)
	provider := &fixedPassengerMeaningV1Interpreter{result: OpenAIPassengerMeaningV1RunResult{
		Proposal:          actual,
		ProposalParseable: true,
		SchemaValid:       true,
	}}
	stateBefore := clonePassengerMeaningV1State(corpusCase.Input.State)
	summary := RunPassengerMeaningV1Shadow(context.Background(), PassengerMeaningV1ShadowInput{
		Enabled:             true,
		OpenAIInterpreter:   provider,
		CurrentTurn:         corpusCase.Input.Turn,
		State:               corpusCase.Input.State,
		PromptEvent:         corpusCase.Input.Prompt,
		SourceMessageID:     corpusCase.Input.SourceMessageID,
		SourcePromptEventID: corpusCase.Input.SourcePromptEventID,
		IdempotencyKey:      "corpus-" + corpusCase.ID,
	})
	if !summary.Validation.Accepted {
		t.Fatalf("shadow rejected corpus provider output: %+v", summary.Validation)
	}
	if !reflect.DeepEqual(corpusCase.Input.State, stateBefore) {
		t.Fatalf("corpus shadow must not promote meaning into B1 state: before=%+v after=%+v", stateBefore, corpusCase.Input.State)
	}
	evaluation := EvaluatePassengerClarificationMeaningV1(actual, corpusCase.Expected)
	if !evaluation.Matched {
		t.Fatalf("structured meaning mismatch: %v\nactual=%+v\nexpected=%+v", evaluation.MismatchFields, actual, corpusCase.Expected)
	}
	inputs := provider.snapshotInputs()
	if len(inputs) != 1 || inputs[0].CurrentTurn != corpusCase.Input.Turn {
		t.Fatalf("provider must receive corpus input exactly once, never expected: %+v", inputs)
	}
	return actual, inputs[0]
}

func TestPassengerMeaningV1CorpusExpectedDoesNotChangeProviderInput(t *testing.T) {
	corpusCase := passengerMeaningV1Corpus()[0]
	runInput := OpenAIPassengerMeaningV1RunInput{
		CurrentTurn:         corpusCase.Input.Turn,
		State:               corpusCase.Input.State,
		PromptEvent:         corpusCase.Input.Prompt,
		SourceMessageID:     corpusCase.Input.SourceMessageID,
		SourcePromptEventID: corpusCase.Input.SourcePromptEventID,
		IdempotencyKey:      "corpus-input-proof",
	}
	before := buildOpenAIPassengerMeaningV1CompactInput(runInput)
	mutatedExpected := corpusCase.Expected
	mutatedExpected.PassengerCount = PassengerCountMeaningV1{Status: PassengerMeaningStatusConflicting, Provenance: PassengerCountProvenanceUnknown}
	mutatedExpected.ReasonCodes = []PassengerMeaningReasonCodeV1{PassengerMeaningReasonConflictingValues}
	after := buildOpenAIPassengerMeaningV1CompactInput(runInput)
	if before != after {
		t.Fatalf("expected fixture must not influence provider input: before=%s after=%s mutated=%+v", before, after, mutatedExpected)
	}
}

func TestPassengerMeaningV1CorpusExpectedSourceIDsAreIndependent(t *testing.T) {
	corpusCase := passengerMeaningV1Corpus()[0]
	providerInputBefore := OpenAIPassengerMeaningV1RunInput{
		CurrentTurn:         corpusCase.Input.Turn,
		State:               corpusCase.Input.State,
		PromptEvent:         corpusCase.Input.Prompt,
		SourceMessageID:     corpusCase.Input.SourceMessageID,
		SourcePromptEventID: corpusCase.Input.SourcePromptEventID,
	}
	providerOutputBefore := passengerMeaningV1CorpusProviderOutput(corpusCase.ID, corpusCase.Input)
	for _, mutation := range []struct {
		name   string
		mutate func(*PassengerClarificationMeaningV1)
	}{
		{name: "source_message_id", mutate: func(expected *PassengerClarificationMeaningV1) {
			expected.SourceMessageID = "wrong-expected-source-message"
		}},
		{name: "source_prompt_event_id", mutate: func(expected *PassengerClarificationMeaningV1) {
			expected.SourcePromptEventID = "wrong-expected-prompt-event"
		}},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			mutated := corpusCase
			mutation.mutate(&mutated.Expected)
			providerInputAfter := OpenAIPassengerMeaningV1RunInput{
				CurrentTurn:         mutated.Input.Turn,
				State:               mutated.Input.State,
				PromptEvent:         mutated.Input.Prompt,
				SourceMessageID:     mutated.Input.SourceMessageID,
				SourcePromptEventID: mutated.Input.SourcePromptEventID,
			}
			providerOutputAfter := passengerMeaningV1CorpusProviderOutput(mutated.ID, mutated.Input)
			if !reflect.DeepEqual(providerInputBefore, providerInputAfter) || !reflect.DeepEqual(providerOutputBefore, providerOutputAfter) {
				t.Fatalf("changing only Expected.%s must not change provider fixture/input", mutation.name)
			}
			if evaluation := EvaluatePassengerClarificationMeaningV1(providerOutputAfter, mutated.Expected); evaluation.Matched {
				t.Fatalf("changing only Expected.%s must produce an evaluator mismatch", mutation.name)
			}
		})
	}
}

func TestPassengerMeaningV1CorpusDistinctChildrenAcrossTurnsIsSequential(t *testing.T) {
	sequences := passengerMeaningV1SequentialCorpus()
	if len(sequences) != 1 {
		t.Fatalf("expected one explicit cross-turn sequence, got %d", len(sequences))
	}
	runPassengerMeaningV1CorpusSequence(t, sequences[0])
}

func TestPassengerMeaningV1CorpusTurnTwoEvidenceDerivesFromTurnOneActual(t *testing.T) {
	sequence := passengerMeaningV1SequentialCorpus()[0]
	turn1 := sequence.Turns[0]
	turn1Actual := passengerMeaningV1CorpusProviderOutput(turn1.ID, turn1.Input)
	turn1Actual.ChildUnder5.References[0].ReferenceID = "child-derived-from-turn-1-actual"

	turn2 := passengerMeaningV1CorpusTurn2FromFirstResult(sequence, turn1Actual)
	if len(turn2.Input.State.ChildReferences) != 1 ||
		turn2.Input.State.ChildReferences[0].ID != "child-derived-from-turn-1-actual" {
		t.Fatalf("turn 2 evidence must derive from turn 1 actual, got %+v", turn2.Input.State.ChildReferences)
	}
	turn2Actual := passengerMeaningV1CorpusProviderOutput(turn2.ID, turn2.Input)
	seen := map[string]int{}
	for _, reference := range turn2Actual.ChildUnder5.References {
		seen[reference.ReferenceID]++
	}
	if len(seen) != 2 || seen["child-derived-from-turn-1-actual"] != 1 || seen["child_turn_2"] != 1 {
		t.Fatalf("turn 2 provider fixture must preserve derived identity and add one distinct child: %+v", turn2Actual.ChildUnder5.References)
	}
}

func TestPassengerMeaningV1CorpusRejectsCrossTurnChildIdentityMutation(t *testing.T) {
	sequence := passengerMeaningV1SequentialCorpus()[0]
	turn1 := sequence.Turns[0]
	turn1Actual := passengerMeaningV1CorpusProviderOutput(turn1.ID, turn1.Input)
	turn2 := passengerMeaningV1CorpusTurn2FromFirstResult(sequence, turn1Actual)
	valid := passengerMeaningV1CorpusProviderOutput(turn2.ID, turn2.Input)

	tests := []struct {
		name   string
		mutate func(*PassengerClarificationMeaningV1)
	}{
		{name: "omit prior identity", mutate: func(proposal *PassengerClarificationMeaningV1) {
			proposal.ChildUnder5.References = []PassengerChildReferenceMeaningV1{
				passengerMeaningV1AgedChildReference("child_replacement", 3, PassengerChildAgeUnitYears),
				passengerMeaningV1AgedChildReference("child_turn_2", 6, PassengerChildAgeUnitYears),
			}
		}},
		{name: "reclassify prior identity", mutate: func(proposal *PassengerClarificationMeaningV1) {
			proposal.ChildUnder5.References = []PassengerChildReferenceMeaningV1{
				passengerMeaningV1AgedChildReference("child_turn_1", 6, PassengerChildAgeUnitYears),
				passengerMeaningV1AgedChildReference("child_turn_2", 3, PassengerChildAgeUnitYears),
			}
		}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			actual := clonePassengerMeaningV1TestProposal(valid)
			testCase.mutate(&actual)
			validation := validatePassengerMeaningV1TestProposal(actual, turn2.Input.State, turn2.Input.Prompt)
			if validation.Accepted() || !passengerMeaningV1StringContains(validation.ReasonCodes, "child_references_conflict_with_snapshot") {
				t.Fatalf("cross-turn identity mutation must fail validation: %+v", validation)
			}
			if evaluation := EvaluatePassengerClarificationMeaningV1(actual, turn2.Expected); evaluation.Matched {
				t.Fatal("cross-turn identity mutation must remain independent from expected and fail evaluation")
			}
		})
	}
}

func passengerMeaningV1Corpus() []passengerMeaningV1CorpusCase {
	passengerState, passengerPrompt := passengerMeaningV1PassengerPromptFixture()
	childState, childPrompt := passengerMeaningV1ChildPromptFixture(3)
	return []passengerMeaningV1CorpusCase{
		passengerMeaningV1CorpusFixture("real_numeric_children", "eu e mais 2 crianças", passengerState, passengerPrompt),
		passengerMeaningV1CorpusFixture("real_words_children", "eu e mais duas crianças", passengerState, passengerPrompt),
		passengerMeaningV1CorpusFixture("speaker_and_two_children", "eu e meus 2 filhos", passengerState, passengerPrompt),
		passengerMeaningV1CorpusFixture("child_subgroup_only", "meus 2 filhos vão viajar", passengerState, passengerPrompt),
		passengerMeaningV1CorpusFixture("absolute_total", "somos 3", passengerState, passengerPrompt),
		passengerMeaningV1CorpusFixture("solo_speaker", "só pra mim", passengerState, passengerPrompt),
		passengerMeaningV1CorpusFixture("youngest_four", "sim, o mais novo tem 4 anos", childState, childPrompt),
		passengerMeaningV1CorpusFixture("two_distinct_under_five", "meu filho tem 3 / meu outro filho tem 4", childState, childPrompt),
		passengerMeaningV1CorpusFixture("one_under_five_one_over", "uma tem 4 e outra 6", childState, childPrompt),
		passengerMeaningV1CorpusFixture("ten_months", "meu filho de 10 meses", childState, childPrompt),
		passengerMeaningV1CorpusFixture("no_under_five", "não tem criança menor de 5", childState, childPrompt),
		passengerMeaningV1CorpusFixture("passenger_correction", "na verdade somos 2", passengerState, passengerPrompt),
		passengerMeaningV1CorpusFixture("child_correction", "na verdade não tem criança menor de 5", childState, childPrompt),
		passengerMeaningV1CorpusFixture("same_child_reference_repeated", "o mesmo filho tem 4, ele fez aniversário em maio", childState, childPrompt),
		passengerMeaningV1CorpusFixture("total_subgroup_uncertainty_correction", "na verdade somos 4; dois são filhos, mas não sei as idades", passengerState, passengerPrompt),
		passengerMeaningV1CorpusFixture("adversarial_date_option_price", "dia 12, opção 2, custa 350", passengerState, passengerPrompt),
		passengerMeaningV1CorpusFixture("adversarial_document_phone", "CPF 52998224725 e telefone 48999998888", passengerState, passengerPrompt),
	}
}

func passengerMeaningV1CorpusFixture(id string, turn string, state PassengerClarificationStateV1, prompt PassengerClarificationEventV1) passengerMeaningV1CorpusCase {
	messageID := "message-" + id
	input := passengerMeaningV1CorpusInput{
		Turn:                turn,
		State:               clonePassengerMeaningV1State(state),
		Prompt:              clonePassengerMeaningV1Prompt(prompt),
		SourceMessageID:     messageID,
		SourcePromptEventID: prompt.EventID,
	}
	return passengerMeaningV1CorpusCase{
		ID:       id,
		Input:    input,
		Expected: passengerMeaningV1CorpusExpected(id, input.SourceMessageID, input.SourcePromptEventID),
	}
}

func passengerMeaningV1SequentialCorpus() []passengerMeaningV1CorpusSequence {
	turn1State, turn1Prompt := passengerMeaningV1ChildPromptFixture(3)
	turn1 := passengerMeaningV1CorpusFixture(
		"distinct_children_turn_1",
		"meu filho tem 3 anos",
		turn1State,
		turn1Prompt,
	)

	turn2Prompt, _ := passengerClarificationPromptEventV1(ActivePromptPassengerCount, "prompt-passenger-cross-turn-2")
	turn2State := newPassengerClarificationStateV1()
	turn2State.HasEvidence = true
	turn2State.PassengerSlotStatus = PassengerClarificationSlotOpen
	turn2State.PassengerPromptMessageID = turn2Prompt.MessageID
	turn2 := passengerMeaningV1CorpusFixture(
		"distinct_children_turn_2",
		"somos 3; a outra criança tem 6 anos",
		turn2State,
		turn2Prompt,
	)

	return []passengerMeaningV1CorpusSequence{{
		ID:    "distinct_children_across_turns",
		Turns: []passengerMeaningV1CorpusCase{turn1, turn2},
	}}
}

func passengerMeaningV1CorpusTurn2FromFirstResult(sequence passengerMeaningV1CorpusSequence, turn1Actual PassengerClarificationMeaningV1) passengerMeaningV1CorpusCase {
	if len(sequence.Turns) != 2 {
		panic("cross-turn fixture must contain exactly two turns")
	}
	turn1 := sequence.Turns[0]
	turn2 := sequence.Turns[1]
	state := clonePassengerMeaningV1State(turn2.Input.State)
	if turn1Actual.ChildUnder5.Status == PassengerMeaningStatusKnown && turn1Actual.ChildUnder5.Count != nil {
		state.ChildUnder5CountKnown = true
		state.ChildUnder5Count = *turn1Actual.ChildUnder5.Count
		state.ChildSlotStatus = PassengerClarificationSlotAnswered
		state.ChildPromptMessageID = turn1.Input.Prompt.MessageID
		state.ChildLastMessageID = turn1.Input.SourceMessageID
	}
	state.ChildReferences = make([]PassengerClarificationChildReferenceV1, 0, len(turn1Actual.ChildUnder5.References))
	for _, reference := range turn1Actual.ChildUnder5.References {
		state.ChildReferences = append(state.ChildReferences, PassengerClarificationChildReferenceV1{
			ID:       reference.ReferenceID,
			Under5:   reference.Under5 != nil && *reference.Under5,
			AgeKnown: reference.AgeValue != nil,
		})
	}
	turn2.Input.State = state
	return turn2
}

func runPassengerMeaningV1CorpusSequence(t *testing.T, sequence passengerMeaningV1CorpusSequence) {
	t.Helper()
	if len(sequence.Turns) != 2 {
		t.Fatalf("cross-turn fixture must have exactly two turns, got %d", len(sequence.Turns))
	}
	turn1 := sequence.Turns[0]
	turn2 := sequence.Turns[1]
	if turn1.Input.SourceMessageID == turn2.Input.SourceMessageID ||
		turn1.Input.SourcePromptEventID == turn2.Input.SourcePromptEventID {
		t.Fatal("cross-turn fixture must use distinct messages and prompt epochs")
	}
	if turn2.Input.State.ChildUnder5CountKnown || len(turn2.Input.State.ChildReferences) != 0 {
		t.Fatalf("turn 2 template must not contain hardcoded prior child evidence: %+v", turn2.Input.State)
	}
	turn1Actual, _ := runPassengerMeaningV1CorpusCase(t, turn1)
	if len(turn1Actual.ChildUnder5.References) != 1 || turn1Actual.ChildUnder5.References[0].ReferenceID != "child_turn_1" {
		t.Fatalf("turn 1 must establish child_turn_1 exactly once: %+v", turn1Actual.ChildUnder5.References)
	}
	turn2 = passengerMeaningV1CorpusTurn2FromFirstResult(sequence, turn1Actual)
	turn2Actual, turn2ProviderInput := runPassengerMeaningV1CorpusCase(t, turn2)
	if len(turn2ProviderInput.State.ChildReferences) != 1 || turn2ProviderInput.State.ChildReferences[0].ID != "child_turn_1" {
		t.Fatalf("provider turn 2 must receive prior structured facts: %+v", turn2ProviderInput.State.ChildReferences)
	}
	seen := map[string]int{}
	for _, reference := range turn2Actual.ChildUnder5.References {
		seen[reference.ReferenceID]++
	}
	if len(seen) != 2 || seen["child_turn_1"] != 1 || seen["child_turn_2"] != 1 {
		t.Fatalf("turn 2 must preserve prior identity and add one distinct identity without collapse/duplication: %+v", turn2Actual.ChildUnder5.References)
	}
}

func passengerMeaningV1CorpusExpected(id string, messageID string, promptEventID string) PassengerClarificationMeaningV1 {
	return passengerMeaningV1CorpusMeaning(id, messageID, promptEventID)
}

func passengerMeaningV1CorpusProviderOutput(id string, input passengerMeaningV1CorpusInput) PassengerClarificationMeaningV1 {
	// This provider fixture is deliberately declared independently from
	// passengerMeaningV1CorpusExpected. The deterministic evaluator receives
	// expected only after this output and the provider input already exist.
	base := PassengerClarificationMeaningV1{
		Version:             passengerClarificationMeaningV1Version,
		SourceMessageID:     input.SourceMessageID,
		SourcePromptEventID: input.SourcePromptEventID,
		Correction:          PassengerMeaningCorrectionV1{Replaces: PassengerMeaningCorrectionNone},
		Confidence:          0.96,
	}
	providerKnownPassenger := func(count int, provenance PassengerCountProvenance, reasons ...PassengerMeaningReasonCodeV1) PassengerClarificationMeaningV1 {
		proposal := base
		proposal.PassengerCount = PassengerCountMeaningV1{Status: PassengerMeaningStatusKnown, Value: passengerMeaningV1Int(count), Provenance: provenance}
		proposal.ChildUnder5 = PassengerChildUnder5MeaningV1{Status: PassengerMeaningStatusUnknown}
		proposal.NeedsClarification = true
		proposal.MissingFields = []PassengerMeaningMissingFieldV1{PassengerMeaningMissingChildUnder5}
		proposal.ReasonCodes = append([]PassengerMeaningReasonCodeV1(nil), reasons...)
		return proposal
	}
	providerKnownChild := func(count int, references []PassengerChildReferenceMeaningV1, reasons ...PassengerMeaningReasonCodeV1) PassengerClarificationMeaningV1 {
		proposal := base
		proposal.PassengerCount = PassengerCountMeaningV1{Status: PassengerMeaningStatusKnown, Value: passengerMeaningV1Int(3), Provenance: PassengerCountProvenanceAbsoluteTotal}
		proposal.ChildUnder5 = PassengerChildUnder5MeaningV1{Status: PassengerMeaningStatusKnown, Count: passengerMeaningV1Int(count), References: references}
		proposal.ReasonCodes = append([]PassengerMeaningReasonCodeV1(nil), reasons...)
		return proposal
	}

	switch id {
	case "real_numeric_children", "real_words_children", "speaker_and_two_children":
		return providerKnownPassenger(3, PassengerCountProvenanceIncludesSpeakerComposition, PassengerMeaningReasonIncludesSpeakerComposition)
	case "child_subgroup_only":
		base.PassengerCount = PassengerCountMeaningV1{Status: PassengerMeaningStatusUnknown, Provenance: PassengerCountProvenanceSubgroupOnly}
		base.ChildUnder5 = PassengerChildUnder5MeaningV1{Status: PassengerMeaningStatusUnknown, References: []PassengerChildReferenceMeaningV1{
			passengerMeaningV1UnknownChildReference("child_1"), passengerMeaningV1UnknownChildReference("child_2"),
		}}
		base.NeedsClarification = true
		base.MissingFields = []PassengerMeaningMissingFieldV1{PassengerMeaningMissingPassengerCount, PassengerMeaningMissingChildUnder5, PassengerMeaningMissingChildAges}
		base.ReasonCodes = []PassengerMeaningReasonCodeV1{PassengerMeaningReasonPassengerSubgroupOnly, PassengerMeaningReasonMissingAge}
		return base
	case "absolute_total":
		return providerKnownPassenger(3, PassengerCountProvenanceAbsoluteTotal, PassengerMeaningReasonPassengerCountStated)
	case "solo_speaker":
		return providerKnownPassenger(1, PassengerCountProvenanceSoloSpeaker, PassengerMeaningReasonPassengerCountStated)
	case "youngest_four":
		return providerKnownChild(1, []PassengerChildReferenceMeaningV1{passengerMeaningV1AgedChildReference("child_1", 4, PassengerChildAgeUnitYears)}, PassengerMeaningReasonChildAgeStated)
	case "two_distinct_under_five":
		return providerKnownChild(2, []PassengerChildReferenceMeaningV1{
			passengerMeaningV1AgedChildReference("child_1", 3, PassengerChildAgeUnitYears),
			passengerMeaningV1AgedChildReference("child_2", 4, PassengerChildAgeUnitYears),
		}, PassengerMeaningReasonChildAgeStated)
	case "one_under_five_one_over":
		return providerKnownChild(1, []PassengerChildReferenceMeaningV1{
			passengerMeaningV1AgedChildReference("child_1", 4, PassengerChildAgeUnitYears),
			passengerMeaningV1AgedChildReference("child_2", 6, PassengerChildAgeUnitYears),
		}, PassengerMeaningReasonChildAgeStated)
	case "distinct_children_turn_1":
		return providerKnownChild(1, []PassengerChildReferenceMeaningV1{
			passengerMeaningV1AgedChildReference("child_turn_1", 3, PassengerChildAgeUnitYears),
		}, PassengerMeaningReasonChildAgeStated)
	case "distinct_children_turn_2":
		references := make([]PassengerChildReferenceMeaningV1, 0, len(input.State.ChildReferences)+1)
		for _, prior := range input.State.ChildReferences {
			under5 := prior.Under5
			references = append(references, PassengerChildReferenceMeaningV1{
				ReferenceID: prior.ID,
				Relation:    PassengerChildRelationChild,
				AgeUnit:     PassengerChildAgeUnitUnknown,
				Under5:      &under5,
			})
		}
		references = append(references, passengerMeaningV1AgedChildReference("child_turn_2", 6, PassengerChildAgeUnitYears))
		return providerKnownChild(input.State.ChildUnder5Count, references, PassengerMeaningReasonChildAgeStated)
	case "ten_months":
		return providerKnownChild(1, []PassengerChildReferenceMeaningV1{passengerMeaningV1AgedChildReference("child_1", 10, PassengerChildAgeUnitMonths)}, PassengerMeaningReasonChildAgeStated)
	case "no_under_five":
		return providerKnownChild(0, nil, PassengerMeaningReasonChildUnder5NotStated)
	case "passenger_correction":
		proposal := providerKnownPassenger(2, PassengerCountProvenanceAbsoluteTotal, PassengerMeaningReasonPassengerCountStated, PassengerMeaningReasonCorrectionStated)
		proposal.Correction = PassengerMeaningCorrectionV1{Present: true, Replaces: PassengerMeaningCorrectionPassengerAggregate}
		return proposal
	case "child_correction":
		proposal := providerKnownChild(0, nil, PassengerMeaningReasonChildUnder5NotStated, PassengerMeaningReasonCorrectionStated)
		proposal.Correction = PassengerMeaningCorrectionV1{Present: true, Replaces: PassengerMeaningCorrectionChildAggregate}
		return proposal
	case "same_child_reference_repeated":
		return providerKnownChild(1, []PassengerChildReferenceMeaningV1{passengerMeaningV1AgedChildReference("child_1", 4, PassengerChildAgeUnitYears)}, PassengerMeaningReasonChildAgeStated)
	case "total_subgroup_uncertainty_correction":
		base.PassengerCount = PassengerCountMeaningV1{Status: PassengerMeaningStatusKnown, Value: passengerMeaningV1Int(4), Provenance: PassengerCountProvenanceAbsoluteTotal}
		base.ChildUnder5 = PassengerChildUnder5MeaningV1{Status: PassengerMeaningStatusUnknown, References: []PassengerChildReferenceMeaningV1{
			passengerMeaningV1UnknownChildReference("child_1"), passengerMeaningV1UnknownChildReference("child_2"),
		}}
		base.Correction = PassengerMeaningCorrectionV1{Present: true, Replaces: PassengerMeaningCorrectionPassengerAggregate}
		base.NeedsClarification = true
		base.MissingFields = []PassengerMeaningMissingFieldV1{PassengerMeaningMissingChildUnder5, PassengerMeaningMissingChildAges}
		base.ReasonCodes = []PassengerMeaningReasonCodeV1{PassengerMeaningReasonPassengerCountStated, PassengerMeaningReasonPassengerSubgroupOnly, PassengerMeaningReasonMissingAge, PassengerMeaningReasonCorrectionStated}
		return base
	case "adversarial_date_option_price", "adversarial_document_phone":
		base.PassengerCount = PassengerCountMeaningV1{Status: PassengerMeaningStatusUnknown, Provenance: PassengerCountProvenanceUnknown}
		base.ChildUnder5 = PassengerChildUnder5MeaningV1{Status: PassengerMeaningStatusUnknown}
		base.NeedsClarification = true
		base.MissingFields = []PassengerMeaningMissingFieldV1{PassengerMeaningMissingPassengerCount, PassengerMeaningMissingChildUnder5}
		base.Confidence = 0.99
		base.ReasonCodes = []PassengerMeaningReasonCodeV1{PassengerMeaningReasonUnrelatedNumberIgnored}
		return base
	default:
		panic("unknown passenger meaning provider fixture: " + id)
	}
}

func passengerMeaningV1CorpusMeaning(id string, messageID string, promptEventID string) PassengerClarificationMeaningV1 {
	unknownChild := PassengerChildUnder5MeaningV1{Status: PassengerMeaningStatusUnknown}
	base := PassengerClarificationMeaningV1{
		Version:             passengerClarificationMeaningV1Version,
		SourceMessageID:     messageID,
		SourcePromptEventID: promptEventID,
		Correction:          PassengerMeaningCorrectionV1{Replaces: PassengerMeaningCorrectionNone},
		Confidence:          0.96,
	}
	knownPassenger := func(count int, provenance PassengerCountProvenance, reasons ...PassengerMeaningReasonCodeV1) PassengerClarificationMeaningV1 {
		proposal := base
		proposal.PassengerCount = PassengerCountMeaningV1{Status: PassengerMeaningStatusKnown, Value: passengerMeaningV1Int(count), Provenance: provenance}
		proposal.ChildUnder5 = unknownChild
		proposal.NeedsClarification = true
		proposal.MissingFields = []PassengerMeaningMissingFieldV1{PassengerMeaningMissingChildUnder5}
		proposal.ReasonCodes = append([]PassengerMeaningReasonCodeV1(nil), reasons...)
		return proposal
	}
	knownChild := func(count int, references []PassengerChildReferenceMeaningV1, reasons ...PassengerMeaningReasonCodeV1) PassengerClarificationMeaningV1 {
		proposal := base
		proposal.PassengerCount = PassengerCountMeaningV1{Status: PassengerMeaningStatusKnown, Value: passengerMeaningV1Int(3), Provenance: PassengerCountProvenanceAbsoluteTotal}
		proposal.ChildUnder5 = PassengerChildUnder5MeaningV1{Status: PassengerMeaningStatusKnown, Count: passengerMeaningV1Int(count), References: references}
		proposal.ReasonCodes = append([]PassengerMeaningReasonCodeV1(nil), reasons...)
		return proposal
	}

	switch id {
	case "real_numeric_children", "real_words_children", "speaker_and_two_children":
		return knownPassenger(3, PassengerCountProvenanceIncludesSpeakerComposition, PassengerMeaningReasonIncludesSpeakerComposition)
	case "child_subgroup_only":
		base.PassengerCount = PassengerCountMeaningV1{Status: PassengerMeaningStatusUnknown, Provenance: PassengerCountProvenanceSubgroupOnly}
		base.ChildUnder5 = PassengerChildUnder5MeaningV1{
			Status: PassengerMeaningStatusUnknown,
			References: []PassengerChildReferenceMeaningV1{
				passengerMeaningV1UnknownChildReference("child_1"),
				passengerMeaningV1UnknownChildReference("child_2"),
			},
		}
		base.NeedsClarification = true
		base.MissingFields = []PassengerMeaningMissingFieldV1{PassengerMeaningMissingPassengerCount, PassengerMeaningMissingChildUnder5, PassengerMeaningMissingChildAges}
		base.ReasonCodes = []PassengerMeaningReasonCodeV1{PassengerMeaningReasonPassengerSubgroupOnly, PassengerMeaningReasonMissingAge}
		return base
	case "absolute_total":
		return knownPassenger(3, PassengerCountProvenanceAbsoluteTotal, PassengerMeaningReasonPassengerCountStated)
	case "solo_speaker":
		return knownPassenger(1, PassengerCountProvenanceSoloSpeaker, PassengerMeaningReasonPassengerCountStated)
	case "youngest_four":
		return knownChild(1, []PassengerChildReferenceMeaningV1{passengerMeaningV1AgedChildReference("child_1", 4, PassengerChildAgeUnitYears)}, PassengerMeaningReasonChildAgeStated)
	case "two_distinct_under_five":
		return knownChild(2, []PassengerChildReferenceMeaningV1{
			passengerMeaningV1AgedChildReference("child_1", 3, PassengerChildAgeUnitYears),
			passengerMeaningV1AgedChildReference("child_2", 4, PassengerChildAgeUnitYears),
		}, PassengerMeaningReasonChildAgeStated)
	case "one_under_five_one_over":
		return knownChild(1, []PassengerChildReferenceMeaningV1{
			passengerMeaningV1AgedChildReference("child_1", 4, PassengerChildAgeUnitYears),
			passengerMeaningV1AgedChildReference("child_2", 6, PassengerChildAgeUnitYears),
		}, PassengerMeaningReasonChildAgeStated)
	case "distinct_children_turn_1":
		return knownChild(1, []PassengerChildReferenceMeaningV1{
			passengerMeaningV1AgedChildReference("child_turn_1", 3, PassengerChildAgeUnitYears),
		}, PassengerMeaningReasonChildAgeStated)
	case "distinct_children_turn_2":
		priorUnder5 := true
		return knownChild(1, []PassengerChildReferenceMeaningV1{
			{
				ReferenceID: "child_turn_1",
				Relation:    PassengerChildRelationChild,
				AgeUnit:     PassengerChildAgeUnitUnknown,
				Under5:      &priorUnder5,
			},
			passengerMeaningV1AgedChildReference("child_turn_2", 6, PassengerChildAgeUnitYears),
		}, PassengerMeaningReasonChildAgeStated)
	case "ten_months":
		return knownChild(1, []PassengerChildReferenceMeaningV1{passengerMeaningV1AgedChildReference("child_1", 10, PassengerChildAgeUnitMonths)}, PassengerMeaningReasonChildAgeStated)
	case "no_under_five":
		return knownChild(0, nil, PassengerMeaningReasonChildUnder5NotStated)
	case "passenger_correction":
		proposal := knownPassenger(2, PassengerCountProvenanceAbsoluteTotal, PassengerMeaningReasonPassengerCountStated, PassengerMeaningReasonCorrectionStated)
		proposal.Correction = PassengerMeaningCorrectionV1{Present: true, Replaces: PassengerMeaningCorrectionPassengerAggregate}
		return proposal
	case "child_correction":
		proposal := knownChild(0, nil, PassengerMeaningReasonChildUnder5NotStated, PassengerMeaningReasonCorrectionStated)
		proposal.Correction = PassengerMeaningCorrectionV1{Present: true, Replaces: PassengerMeaningCorrectionChildAggregate}
		return proposal
	case "same_child_reference_repeated":
		return knownChild(1, []PassengerChildReferenceMeaningV1{passengerMeaningV1AgedChildReference("child_1", 4, PassengerChildAgeUnitYears)}, PassengerMeaningReasonChildAgeStated)
	case "total_subgroup_uncertainty_correction":
		base.PassengerCount = PassengerCountMeaningV1{Status: PassengerMeaningStatusKnown, Value: passengerMeaningV1Int(4), Provenance: PassengerCountProvenanceAbsoluteTotal}
		base.ChildUnder5 = PassengerChildUnder5MeaningV1{
			Status: PassengerMeaningStatusUnknown,
			References: []PassengerChildReferenceMeaningV1{
				passengerMeaningV1UnknownChildReference("child_1"),
				passengerMeaningV1UnknownChildReference("child_2"),
			},
		}
		base.Correction = PassengerMeaningCorrectionV1{Present: true, Replaces: PassengerMeaningCorrectionPassengerAggregate}
		base.NeedsClarification = true
		base.MissingFields = []PassengerMeaningMissingFieldV1{PassengerMeaningMissingChildUnder5, PassengerMeaningMissingChildAges}
		base.ReasonCodes = []PassengerMeaningReasonCodeV1{PassengerMeaningReasonPassengerCountStated, PassengerMeaningReasonPassengerSubgroupOnly, PassengerMeaningReasonMissingAge, PassengerMeaningReasonCorrectionStated}
		return base
	case "adversarial_date_option_price", "adversarial_document_phone":
		base.PassengerCount = PassengerCountMeaningV1{Status: PassengerMeaningStatusUnknown, Provenance: PassengerCountProvenanceUnknown}
		base.ChildUnder5 = unknownChild
		base.NeedsClarification = true
		base.MissingFields = []PassengerMeaningMissingFieldV1{PassengerMeaningMissingPassengerCount, PassengerMeaningMissingChildUnder5}
		base.Confidence = 0.99
		base.ReasonCodes = []PassengerMeaningReasonCodeV1{PassengerMeaningReasonUnrelatedNumberIgnored}
		return base
	default:
		panic("unknown passenger meaning corpus fixture: " + id)
	}
}

func passengerMeaningV1PassengerPromptFixture() (PassengerClarificationStateV1, PassengerClarificationEventV1) {
	prompt, _ := passengerClarificationPromptEventV1(ActivePromptPassengerCount, "prompt-passenger-v1")
	state := newPassengerClarificationStateV1()
	state.HasEvidence = true
	state.PassengerSlotStatus = PassengerClarificationSlotOpen
	state.PassengerPromptMessageID = prompt.MessageID
	return state, prompt
}

func passengerMeaningV1ChildPromptFixture(passengerCount int) (PassengerClarificationStateV1, PassengerClarificationEventV1) {
	prompt, _ := passengerClarificationPromptEventV1(ActivePromptLapChildQuestion, "prompt-child-v1")
	state := newPassengerClarificationStateV1()
	state.HasEvidence = true
	state.PassengerCountKnown = true
	state.PassengerCount = passengerCount
	state.PassengerCountProvenance = PassengerCountProvenanceAbsoluteTotal
	state.PassengerSlotStatus = PassengerClarificationSlotAnswered
	state.ChildSlotStatus = PassengerClarificationSlotOpen
	state.ChildPromptMessageID = prompt.MessageID
	return state, prompt
}

func passengerMeaningV1UnknownChildReference(id string) PassengerChildReferenceMeaningV1 {
	return PassengerChildReferenceMeaningV1{
		ReferenceID: id,
		Relation:    PassengerChildRelationChild,
		AgeUnit:     PassengerChildAgeUnitUnknown,
	}
}

func passengerMeaningV1AgedChildReference(id string, age int, unit PassengerChildAgeUnitV1) PassengerChildReferenceMeaningV1 {
	under5 := passengerChildAgeUnder5V1(unit, age)
	return PassengerChildReferenceMeaningV1{
		ReferenceID: id,
		Relation:    PassengerChildRelationChild,
		AgeValue:    passengerMeaningV1Int(age),
		AgeUnit:     unit,
		Under5:      &under5,
	}
}

func passengerMeaningV1Int(value int) *int {
	return &value
}

func TestPassengerMeaningV1CorpusMapperProducesNoRuntimeEvent(t *testing.T) {
	meaning := passengerMeaningV1Corpus()[0].Expected
	proposal := MapPassengerClarificationMeaningV1(meaning)
	if reflect.TypeOf(proposal) == reflect.TypeOf(PassengerClarificationEventV1{}) {
		t.Fatal("local proposal must not be a B1 runtime event")
	}
	proposal.PassengerCount.Value = passengerMeaningV1Int(99)
	if *meaning.PassengerCount.Value == 99 {
		t.Fatal("mapper must return an isolated local proposal")
	}
}
