package chat

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"schumacher-tur/api/internal/shared/config"
)

func TestAvailabilitySelectionStateV1SupersededAuthorityNeverResurrects(t *testing.T) {
	now := availabilityTestObservedAt()
	availabilityA := availabilityOptionPromptFutureResultAt(now)
	availabilityB := availabilityOptionPromptFutureResultAt(now.Add(24 * time.Hour))
	availabilityB.Results[0].TripID = "trip-b"
	availabilityB.Results[0].BoardStopID = "board-b"
	availabilityB.Results[0].AlightStopID = "alight-b"
	snapshotA := mustAvailabilitySelectionSnapshotV1ForTest(t, &availabilityA, 1)
	snapshotB := mustAvailabilitySelectionSnapshotV1ForTest(t, &availabilityB, 1)

	state := ReduceAvailabilitySelectionEventsV1(
		newAvailabilitySelectionStateV1(),
		orderedAvailabilitySelectionEventsForTest(
			materializedAvailabilitySelectionEventForTest("selection-event-a", "projection-a", "prompt-a", snapshotA),
			materializedAvailabilitySelectionEventForTest("selection-event-b", "projection-b", "prompt-b", snapshotB),
			AvailabilitySelectionEventV1{
				Type:                              AvailabilitySelectionEventRejected,
				MessageID:                         "rejection-b",
				AvailabilityPromptSourceMessageID: "prompt-b",
				RejectedOptionIndexes:             []int{1},
			},
		),
	)
	if state.Status != AvailabilitySelectionStatusRejected ||
		state.SelectedOptionIndex != 0 ||
		state.SelectionEventMessageID != "" ||
		state.SelectionProjectionMessageID != "" ||
		state.AvailabilityPromptSourceMessageID != "" ||
		state.Snapshot != (AvailabilitySelectionSnapshotV1{}) {
		t.Fatalf("S1 -> S2 -> reject S2 must end with no authority: %+v", state)
	}

	state = ReduceAvailabilitySelectionEventsV1(
		state,
		orderedAvailabilitySelectionEventsForTestFrom(
			3,
			AvailabilitySelectionEventV1{
				Type:                              AvailabilitySelectionEventMaterialized,
				MessageID:                         "copied-projection-a",
				ProjectionMessageID:               "copied-projection-a",
				AvailabilityPromptSourceMessageID: "prompt-a",
				SelectedOptionIndex:               1,
				Snapshot:                          snapshotA,
				MaterializesAuthority:             false,
			},
		),
	)
	if state.Status != AvailabilitySelectionStatusRejected ||
		state.SelectedOptionIndex != 0 ||
		state.Snapshot != (AvailabilitySelectionSnapshotV1{}) {
		t.Fatalf("copied S1 projection resurrected superseded authority: %+v", state)
	}
}

func TestAvailabilitySelectionStateV1ScopesRejectionToPromptSource(t *testing.T) {
	availability := availabilityOptionPromptFutureResultAt(availabilityTestObservedAt())
	snapshot := mustAvailabilitySelectionSnapshotV1ForTest(t, &availability, 1)
	state := ReduceAvailabilitySelectionEventsV1(
		newAvailabilitySelectionStateV1(),
		orderedAvailabilitySelectionEventsForTest(
			materializedAvailabilitySelectionEventForTest(
				"selection-event-a",
				"projection-a",
				"prompt-a",
				snapshot,
			),
			AvailabilitySelectionEventV1{
				Type:                              AvailabilitySelectionEventRejected,
				MessageID:                         "rejection-b",
				AvailabilityPromptSourceMessageID: "prompt-b",
				RejectedOptionIndexes:             []int{1},
			},
		),
	)
	if state.Status != AvailabilitySelectionStatusBookable ||
		state.SelectionEventMessageID != "selection-event-a" ||
		state.AvailabilityPromptSourceMessageID != "prompt-a" ||
		state.Snapshot.TripID != snapshot.TripID {
		t.Fatalf("rejection from prompt B cleared selection from prompt A: %+v", state)
	}
	if !state.rejectsPromptOption("prompt-b", 1, snapshot.TripDate) {
		t.Fatalf("prompt B tombstone was not retained durably: %+v", state.Rejections)
	}
	if state.rejectsPromptOption("prompt-a", 1, snapshot.TripDate) {
		t.Fatalf("prompt B tombstone leaked into prompt A: %+v", state.Rejections)
	}
}

func TestAvailabilitySelectionStateV1ProjectionOnlyWindowHasNoAuthority(t *testing.T) {
	availability := availabilityOptionPromptFutureResultAt(availabilityTestObservedAt())
	snapshot := mustAvailabilitySelectionSnapshotV1ForTest(t, &availability, 1)
	state := bootstrapAvailabilitySelectionStateV1(
		nil,
		orderedAvailabilitySelectionEventsForTest(AvailabilitySelectionEventV1{
			Type:                              AvailabilitySelectionEventMaterialized,
			MessageID:                         "projection-only",
			ProjectionMessageID:               "projection-only",
			AvailabilityPromptSourceMessageID: "superseded-prompt",
			SelectedOptionIndex:               1,
			Snapshot:                          snapshot,
			MaterializesAuthority:             false,
		}),
	)
	if state.Status != AvailabilitySelectionStatusNone ||
		state.SelectedOptionIndex != 0 ||
		state.Snapshot != (AvailabilitySelectionSnapshotV1{}) {
		t.Fatalf("projection-only bootstrap created authority: %+v", state)
	}
}

func TestAvailabilitySelectionSnapshotEnrichmentRejectsUntrustedAuthorityClassesV1(t *testing.T) {
	raw := availabilityPromptAuthorityRawResult()
	fullSnapshot := mustAvailabilitySelectionSnapshotV1ForTest(t, &raw, 1)
	selectionPayload := selectedAvailabilityResultPayloadFromAvailability(&raw, 1)
	selectionPayload[selectedAvailabilitySelectionMessageIDPayloadKey] = "snapshot-selection-inbound"
	selectionPayload[availabilityPromptSourceMessageIDPayloadKey] = "snapshot-prompt-source"

	build := func(messageID, status string) Message {
		message := availabilityPromptAuthorityMessage(messageID, status, raw, []int{0})
		for _, payload := range []map[string]interface{}{message.Payload, message.NormalizedPayload} {
			payload["selected_option_index"] = 1
			payload[selectedAvailabilityResultPayloadKey] = cloneMap(selectionPayload)
		}
		if status == "DELIVERED" {
			passengerEvent, ok := passengerClarificationPromptEventV1(ActivePromptPassengerCount, message.ID)
			if !ok {
				t.Fatal("build passenger event")
			}
			message.Payload[passengerPromptEventV1MessageKey] = passengerEvent
			message.NormalizedPayload[passengerPromptEventV1MessageKey] = passengerEvent
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
		{name: "invalid", class: availabilityPromptAuthorityInvalidV1, msg: build("snapshot-invalid", "DELIVERED")},
		{name: "undelivered", class: availabilityPromptAuthorityUndeliveredV1, msg: build("snapshot-undelivered", "SEND_FAILED")},
	} {
		t.Run(test.name, func(t *testing.T) {
			if authority := classifyAvailabilityPromptCandidateV1(test.msg); authority.Class != test.class {
				t.Fatalf("class=%s, want %s", authority.Class, test.class)
			}
			if !legacyAvailabilitySelectionProjectionFactsV1(test.msg) {
				t.Fatal("test precondition did not retain decodable reconciled facts/selection")
			}
			if candidates := availabilitySelectionSnapshotCandidatesFromMessageV1(test.msg, 1); len(candidates) != 0 {
				t.Fatalf("untrusted authority produced snapshot candidates: %+v", candidates)
			}

			incomplete := AvailabilitySelectionSnapshotV1{
				SelectedOptionIndex: 1,
				TripID:              fullSnapshot.TripID,
				BoardStopID:         fullSnapshot.BoardStopID,
				AlightStopID:        fullSnapshot.AlightStopID,
			}
			event := AvailabilitySelectionEventV1{
				Type:                              AvailabilitySelectionEventMaterialized,
				MessageID:                         "snapshot-event-" + test.name,
				ProjectionMessageID:               test.msg.ID,
				AvailabilityPromptSourceMessageID: "snapshot-prompt-source",
				SelectedOptionIndex:               1,
				Snapshot:                          incomplete,
				MaterializesAuthority:             true,
			}
			enriched := enrichLegacyAvailabilitySelectionEventSnapshotV1(event, []Message{test.msg})
			if enriched.Snapshot != incomplete {
				t.Fatalf("untrusted authority enriched legacy snapshot: got=%+v want=%+v", enriched.Snapshot, incomplete)
			}
		})
	}

	valid := availabilityPromptAuthorityMessage("snapshot-valid", "DELIVERED", raw, []int{0})
	validCandidates := availabilitySelectionSnapshotCandidatesFromMessageV1(valid, 1)
	if len(validCandidates) != 1 || validCandidates[0].PackageName != fullSnapshot.PackageName || validCandidates[0].TripDate != fullSnapshot.TripDate {
		t.Fatalf("VALID_STRUCTURAL control lost Presented enrichment: %+v", validCandidates)
	}
	legacy := legacyAvailabilityPromptMessageForStateTest("snapshot-legacy", raw, valid.ReceivedAt)
	legacyCandidates := availabilitySelectionSnapshotCandidatesFromMessageV1(legacy, 1)
	if len(legacyCandidates) == 0 || legacyCandidates[0].TripID != fullSnapshot.TripID {
		t.Fatalf("trusted ABSENT_LEGACY control lost enrichment: %+v", legacyCandidates)
	}

	mismatch := AvailabilitySelectionEventV1{
		Type:                              AvailabilitySelectionEventMaterialized,
		MessageID:                         "snapshot-mismatch-event",
		ProjectionMessageID:               valid.ID,
		AvailabilityPromptSourceMessageID: valid.ID,
		SelectedOptionIndex:               1,
		Snapshot: AvailabilitySelectionSnapshotV1{
			SelectedOptionIndex: 1,
			TripID:              "different-trip",
			BoardStopID:         fullSnapshot.BoardStopID,
			AlightStopID:        fullSnapshot.AlightStopID,
		},
		MaterializesAuthority: true,
	}
	mismatchEnriched := enrichLegacyAvailabilitySelectionEventSnapshotV1(mismatch, []Message{valid})
	if mismatchEnriched.Snapshot.PackageName != "" || mismatchEnriched.Snapshot.TripDate != "" {
		t.Fatalf("identity mismatch enriched snapshot: %+v", mismatchEnriched.Snapshot)
	}
}

func TestAvailabilitySelectionSnapshotEnrichmentPreservesTrustedAbsentLegacySelectionV1(t *testing.T) {
	raw := availabilityPromptAuthorityRawResult()
	fullSnapshot := mustAvailabilitySelectionSnapshotV1ForTest(t, &raw, 1)
	selected := selectedAvailabilityResultPayloadFromAvailability(&raw, 1)
	selected[selectedAvailabilitySelectionMessageIDPayloadKey] = "trusted-legacy-selection-inbound"
	selected[availabilityPromptSourceMessageIDPayloadKey] = "trusted-legacy-prompt-source"

	build := func(messageID, status string, trustedIdentity bool) Message {
		snapshot := cloneMap(selected)
		if !trustedIdentity {
			delete(snapshot, availabilityPromptSourceMessageIDPayloadKey)
		}
		payload := map[string]interface{}{
			"intent":                             string(IntentSelectAvailabilityOption),
			"selected_option_index":              1,
			selectedAvailabilityResultPayloadKey: snapshot,
		}
		message := Message{
			ID:                messageID,
			Direction:         "OUTBOUND",
			ProcessingStatus:  status,
			Payload:           cloneMap(payload),
			NormalizedPayload: cloneMap(payload),
			ReceivedAt:        availabilityTestObservedAt(),
			CreatedAt:         availabilityTestObservedAt(),
		}
		if DeliveryStatusConfirmsOutboundV1(status) {
			message.NormalizedPayload["delivery_recorded_at"] = message.ReceivedAt.Format(time.RFC3339Nano)
		}
		return message
	}

	trusted := build("trusted-legacy-selection-projection", "DELIVERED", true)
	trustedAuthority := classifyAvailabilityPromptCandidateV1(trusted)
	if trustedAuthority.Class != availabilityPromptAuthorityAbsentLegacyV1 || trustedAuthority.Facts != nil || trustedAuthority.Selection == nil {
		t.Fatalf("trusted selection precondition failed: %+v", trustedAuthority)
	}
	if !legacyAvailabilitySelectionProjectionFactsV1(trusted) {
		t.Fatal("bilateral complete legacy selection was not recognized as trusted")
	}
	trustedCandidates := availabilitySelectionSnapshotCandidatesFromMessageV1(trusted, 1)
	if len(trustedCandidates) != 1 || trustedCandidates[0].TripDate != fullSnapshot.TripDate ||
		trustedCandidates[0].PackageName != fullSnapshot.PackageName || trustedCandidates[0].Price != fullSnapshot.Price {
		t.Fatalf("trusted ABSENT_LEGACY selection lost snapshot enrichment: %+v", trustedCandidates)
	}

	incomplete := AvailabilitySelectionSnapshotV1{
		SelectedOptionIndex: 1,
		TripID:              fullSnapshot.TripID,
		BoardStopID:         fullSnapshot.BoardStopID,
		AlightStopID:        fullSnapshot.AlightStopID,
	}
	event := AvailabilitySelectionEventV1{
		Type:                              AvailabilitySelectionEventMaterialized,
		MessageID:                         "trusted-legacy-selection-inbound",
		ProjectionMessageID:               trusted.ID,
		AvailabilityPromptSourceMessageID: "trusted-legacy-prompt-source",
		SelectedOptionIndex:               1,
		Snapshot:                          incomplete,
		MaterializesAuthority:             true,
	}
	trustedSource := availabilityPromptAuthorityMessage("trusted-legacy-prompt-source", "DELIVERED", raw, []int{0})
	enriched := enrichLegacyAvailabilitySelectionEventSnapshotV1(event, []Message{trustedSource, trusted})
	if enriched.Snapshot.TripDate != fullSnapshot.TripDate ||
		enriched.Snapshot.PackageName != fullSnapshot.PackageName || enriched.Snapshot.Price != fullSnapshot.Price {
		t.Fatalf("reload did not recover authorized legacy selection fields: %+v", enriched.Snapshot)
	}
	mismatchedSource := event
	mismatchedSource.AvailabilityPromptSourceMessageID = "superseded-prompt-source"
	mismatchedSource.Snapshot = incomplete
	mismatchedEnrichment := enrichLegacyAvailabilitySelectionEventSnapshotV1(mismatchedSource, []Message{trustedSource, trusted})
	if mismatchedEnrichment.Snapshot != incomplete {
		t.Fatalf("legacy selection from a different prompt source enriched snapshot: %+v", mismatchedEnrichment.Snapshot)
	}

	invalid := build("trusted-legacy-selection-invalid", "DELIVERED", true)
	passengerEvent, ok := passengerClarificationPromptEventV1(ActivePromptPassengerCount, invalid.ID)
	if !ok {
		t.Fatal("build passenger event")
	}
	invalid.Payload[passengerPromptEventV1MessageKey] = passengerEvent
	invalid.NormalizedPayload[passengerPromptEventV1MessageKey] = passengerEvent
	if authority := classifyAvailabilityPromptCandidateV1(invalid); authority.Class != availabilityPromptAuthorityInvalidV1 {
		t.Fatalf("invalid control class=%s", authority.Class)
	}
	if candidates := availabilitySelectionSnapshotCandidatesFromMessageV1(invalid, 1); len(candidates) != 0 {
		t.Fatalf("INVALID selection enriched snapshot: %+v", candidates)
	}

	undelivered := build("trusted-legacy-selection-undelivered", "SEND_FAILED", true)
	if authority := classifyAvailabilityPromptCandidateV1(undelivered); authority.Class != availabilityPromptAuthorityUndeliveredV1 {
		t.Fatalf("undelivered control class=%s", authority.Class)
	}
	if candidates := availabilitySelectionSnapshotCandidatesFromMessageV1(undelivered, 1); len(candidates) != 0 {
		t.Fatalf("UNDELIVERED selection enriched snapshot: %+v", candidates)
	}

	untrusted := build("untrusted-legacy-selection-projection", "DELIVERED", false)
	if legacyAvailabilitySelectionProjectionFactsV1(untrusted) {
		t.Fatal("legacy selection without trusted identity passed the trust gate")
	}
	if candidates := availabilitySelectionSnapshotCandidatesFromMessageV1(untrusted, 1); len(candidates) != 0 {
		t.Fatalf("legacy selection without trusted identity enriched snapshot: %+v", candidates)
	}

	valid := availabilityPromptAuthorityMessage("trusted-selection-valid-structural", "DELIVERED", raw, []int{0})
	validCandidates := availabilitySelectionSnapshotCandidatesFromMessageV1(valid, 1)
	if len(validCandidates) != 1 || validCandidates[0].TripDate != fullSnapshot.TripDate {
		t.Fatalf("VALID_STRUCTURAL enrichment regressed: %+v", validCandidates)
	}
}

func TestAvailabilitySelectionSnapshotEnrichmentUsesOnlyExactLegacyProjectionV1(t *testing.T) {
	raw := availabilityPromptAuthorityRawResult()
	fullSnapshot := mustAvailabilitySelectionSnapshotV1ForTest(t, &raw, 1)
	selected := selectedAvailabilityResultPayloadFromAvailability(&raw, 1)
	selected[selectedAvailabilitySelectionMessageIDPayloadKey] = "exclusive-selection-inbound"
	selected[availabilityPromptSourceMessageIDPayloadKey] = "exclusive-structural-source"
	delete(selected, "route_id")
	delete(selected, "package_name")
	delete(selected, "currency")
	payload := map[string]interface{}{
		"intent":                             string(IntentSelectAvailabilityOption),
		"selected_option_index":              1,
		selectedAvailabilityResultPayloadKey: selected,
		"tool_context": map[string]interface{}{
			toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(raw),
		},
	}
	projection := Message{
		ID:                "exclusive-legacy-projection",
		Direction:         "OUTBOUND",
		ProcessingStatus:  "DELIVERED",
		Payload:           cloneMap(payload),
		NormalizedPayload: cloneMap(payload),
		ReceivedAt:        availabilityTestObservedAt(),
		CreatedAt:         availabilityTestObservedAt(),
	}
	projection.NormalizedPayload["delivery_recorded_at"] = projection.ReceivedAt.Format(time.RFC3339Nano)
	if authority := classifyAvailabilityPromptCandidateV1(projection); authority.Class != availabilityPromptAuthorityAbsentLegacyV1 || authority.Selection == nil {
		t.Fatalf("projection precondition failed: %+v", authority)
	}
	source := availabilityPromptAuthorityMessage("exclusive-structural-source", "DELIVERED", raw, []int{0})
	event := materializedAvailabilitySelectionEventForTest(
		"exclusive-selection-inbound",
		projection.ID,
		source.ID,
		AvailabilitySelectionSnapshotV1{
			SelectedOptionIndex: 1,
			TripID:              fullSnapshot.TripID,
			BoardStopID:         fullSnapshot.BoardStopID,
			AlightStopID:        fullSnapshot.AlightStopID,
		},
	)

	enriched := enrichLegacyAvailabilitySelectionEventSnapshotV1(event, []Message{source, projection})
	if !enriched.MaterializesAuthority || enriched.Snapshot.TripDate != fullSnapshot.TripDate ||
		enriched.Snapshot.Price != fullSnapshot.Price {
		t.Fatalf("exact projection did not provide its selected result: %+v", enriched)
	}
	if enriched.Snapshot.RouteID != "" || enriched.Snapshot.PackageName != "" || enriched.Snapshot.Currency != "" {
		t.Fatalf("structural prompt source leaked extra facts into legacy snapshot: %+v", enriched.Snapshot)
	}

	for _, test := range []struct {
		name   string
		mutate func(*AvailabilitySelectionEventV1)
	}{
		{name: "projection", mutate: func(event *AvailabilitySelectionEventV1) { event.ProjectionMessageID = "other-projection" }},
		{name: "selection message", mutate: func(event *AvailabilitySelectionEventV1) { event.MessageID = "other-selection" }},
		{name: "prompt source", mutate: func(event *AvailabilitySelectionEventV1) { event.AvailabilityPromptSourceMessageID = "other-source" }},
		{name: "index", mutate: func(event *AvailabilitySelectionEventV1) {
			event.SelectedOptionIndex = 2
			event.Snapshot.SelectedOptionIndex = 2
		}},
	} {
		t.Run(test.name+" mismatch", func(t *testing.T) {
			mismatched := event
			test.mutate(&mismatched)
			got := enrichLegacyAvailabilitySelectionEventSnapshotV1(mismatched, []Message{source, projection})
			if got.MaterializesAuthority {
				t.Fatalf("identity mismatch retained legacy authority: %+v", got)
			}
			state := ReduceAvailabilitySelectionEventsV1(
				newAvailabilitySelectionStateV1(),
				[]AvailabilitySelectionEventV1{got},
			)
			if state.Status == AvailabilitySelectionStatusBookable {
				t.Fatalf("identity mismatch recovered BOOKABLE: %+v", state)
			}
			if _, _, ok := resolveBookingCreateSelectionFromState("quero reservar opção 1", state); ok {
				t.Fatal("identity mismatch reached booking_create")
			}
		})
	}
}

func TestAvailabilitySelectionSnapshotEnrichmentRequiresExplicitPromptSourceV1(t *testing.T) {
	raw := availabilityPromptAuthorityRawResult()
	fullSnapshot := mustAvailabilitySelectionSnapshotV1ForTest(t, &raw, 1)
	baseSelected := selectedAvailabilityResultPayloadFromAvailability(&raw, 1)
	baseSelected[selectedAvailabilitySelectionMessageIDPayloadKey] = "explicit-source-selection-inbound"
	buildProjection := func(selected map[string]interface{}) Message {
		payload := map[string]interface{}{
			"intent":                             string(IntentSelectAvailabilityOption),
			"selected_option_index":              1,
			selectedAvailabilityResultPayloadKey: cloneMap(selected),
			"tool_context": map[string]interface{}{
				toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(raw),
			},
		}
		message := Message{
			ID:                "explicit-source-legacy-projection",
			Direction:         "OUTBOUND",
			ProcessingStatus:  "DELIVERED",
			Payload:           cloneMap(payload),
			NormalizedPayload: cloneMap(payload),
			ReceivedAt:        availabilityTestObservedAt(),
			CreatedAt:         availabilityTestObservedAt(),
		}
		message.NormalizedPayload["delivery_recorded_at"] = message.ReceivedAt.Format(time.RFC3339Nano)
		return message
	}
	buildEvent := func(projectionID string) AvailabilitySelectionEventV1 {
		return materializedAvailabilitySelectionEventForTest(
			"explicit-source-selection-inbound",
			projectionID,
			"explicit-source-prompt",
			AvailabilitySelectionSnapshotV1{
				SelectedOptionIndex: 1,
				TripID:              fullSnapshot.TripID,
				BoardStopID:         fullSnapshot.BoardStopID,
				AlightStopID:        fullSnapshot.AlightStopID,
			},
		)
	}
	source := availabilityPromptAuthorityMessage("explicit-source-prompt", "DELIVERED", raw, []int{0})

	for _, test := range []struct {
		name          string
		setSource     bool
		sourceID      string
		historySource []Message
		wantAuthority bool
	}{
		{name: "omitted without history", historySource: nil},
		{name: "explicit empty", setSource: true, sourceID: "", historySource: []Message{source}},
		{name: "mismatch", setSource: true, sourceID: "other-source", historySource: []Message{source}},
		{name: "exact", setSource: true, sourceID: source.ID, historySource: []Message{source}, wantAuthority: true},
		{name: "compatible history cannot infer omitted id", historySource: []Message{source}},
		{name: "duplicate exact source", setSource: true, sourceID: source.ID, historySource: []Message{source, source}},
	} {
		t.Run(test.name, func(t *testing.T) {
			selected := cloneMap(baseSelected)
			if test.setSource {
				selected[availabilityPromptSourceMessageIDPayloadKey] = test.sourceID
			}
			projection := buildProjection(selected)
			history := append(append([]Message(nil), test.historySource...), projection)
			got := enrichLegacyAvailabilitySelectionEventSnapshotV1(buildEvent(projection.ID), history)
			if got.MaterializesAuthority != test.wantAuthority {
				t.Fatalf("MaterializesAuthority=%t, want %t: %+v", got.MaterializesAuthority, test.wantAuthority, got)
			}
			state := ReduceAvailabilitySelectionEventsV1(
				newAvailabilitySelectionStateV1(),
				[]AvailabilitySelectionEventV1{got},
			)
			if !test.wantAuthority {
				if state.Status == AvailabilitySelectionStatusBookable {
					t.Fatalf("invalid prompt source recovered BOOKABLE: %+v", state)
				}
				if _, _, ok := resolveBookingCreateSelectionFromState("quero reservar opção 1", state); ok {
					t.Fatal("invalid prompt source reached booking_create")
				}
			}
		})
	}
}

func TestAvailabilitySelectionLegacyProjectionUsesProjectionBarrierBoundaryV1(t *testing.T) {
	raw := availabilityPromptAuthorityRawResult()
	makeInvalid := func(messageID string, at time.Time) Message {
		message := availabilityPromptAuthorityMessage(messageID, "DELIVERED", raw, []int{0})
		message.Payload[testAvailabilityPromptEventV1MessageKey] = "malformed"
		message.NormalizedPayload[testAvailabilityPromptEventV1MessageKey] = "malformed"
		message.ReceivedAt = at
		message.CreatedAt = at
		message.NormalizedPayload["delivery_recorded_at"] = at.Format(time.RFC3339Nano)
		if authority := classifyAvailabilityPromptCandidateV1(message); authority.Class != availabilityPromptAuthorityInvalidV1 {
			t.Fatalf("invalid fixture class=%s, want INVALID", authority.Class)
		}
		return message
	}
	makeNonAvailability := func(messageID string, intent Intent, template ResponseTemplateName, at time.Time) Message {
		payload := map[string]interface{}{
			"intent":        string(intent),
			"template_name": string(template),
		}
		normalized := cloneMap(payload)
		normalized["delivery_recorded_at"] = at.Format(time.RFC3339Nano)
		return Message{
			ID:                messageID,
			Direction:         "OUTBOUND",
			ProcessingStatus:  "DELIVERED",
			Payload:           payload,
			NormalizedPayload: normalized,
			ReceivedAt:        at,
			CreatedAt:         at,
		}
	}

	for _, test := range []struct {
		name                    string
		buildHistory            func(Message, Message, Message, time.Time) []Message
		wantProjectionAuthority bool
		wantFinalBookable       bool
	}{
		{
			name: "INVALID between selection and projection",
			buildHistory: func(source, selection, projection Message, at time.Time) []Message {
				return []Message{source, selection, makeInvalid("barrier-after-selection", at.Add(2*time.Minute)), projection}
			},
		},
		{
			name: "INVALID between source and selection",
			buildHistory: func(source, selection, projection Message, at time.Time) []Message {
				return []Message{source, makeInvalid("barrier-before-selection", at.Add(time.Minute)), selection, projection}
			},
		},
		{
			name: "new source after old INVALID",
			buildHistory: func(source, selection, projection Message, at time.Time) []Message {
				return []Message{makeInvalid("old-barrier", at.Add(-time.Minute)), source, selection, projection}
			},
			wantProjectionAuthority: true,
			wantFinalBookable:       true,
		},
		{
			name: "INVALID after projection is not retroactive",
			buildHistory: func(source, selection, projection Message, at time.Time) []Message {
				return []Message{source, selection, projection, makeInvalid("posterior-barrier", at.Add(4*time.Minute))}
			},
			wantProjectionAuthority: true,
		},
		{
			name: "UNDELIVERED availability is not a barrier",
			buildHistory: func(source, selection, projection Message, at time.Time) []Message {
				undelivered := availabilityPromptAuthorityMessage("undelivered-availability", "SEND_FAILED", raw, []int{0})
				undelivered.ReceivedAt = at.Add(2 * time.Minute)
				undelivered.CreatedAt = undelivered.ReceivedAt
				delete(undelivered.NormalizedPayload, "delivery_recorded_at")
				return []Message{source, selection, undelivered, projection}
			},
			wantProjectionAuthority: true,
			wantFinalBookable:       true,
		},
		{
			name: "passenger payment and document messages are not availability barriers",
			buildHistory: func(source, selection, projection Message, at time.Time) []Message {
				passenger := makeNonAvailability(
					"passenger-between-selection-and-projection",
					IntentPassengerCountReply,
					TemplateAskPassengerCount,
					at.Add(2*time.Minute),
				)
				payment := makeNonAvailability(
					"payment-between-selection-and-projection",
					IntentPaymentPreference,
					TemplateAskPaymentChoice,
					at.Add(3*time.Minute),
				)
				document := makeNonAvailability(
					"document-between-selection-and-projection",
					IntentPassengerDocumentsProvided,
					TemplateAskDocuments,
					at.Add(4*time.Minute),
				)
				return []Message{source, selection, passenger, payment, document, projection}
			},
			wantProjectionAuthority: true,
			wantFinalBookable:       true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			suffix := strings.ReplaceAll(test.name, " ", "-")
			source := availabilityPromptAuthorityMessage("projection-boundary-source-"+suffix, "DELIVERED", raw, []int{0})
			at := source.ReceivedAt
			projection := legacyAvailabilitySelectionMessageForStateTest(
				"projection-boundary-"+suffix,
				raw,
				IntentSelectAvailabilityOption,
				at.Add(5*time.Minute),
				source.ID,
			)
			selection := legacyAvailabilitySelectionInboundForStateTest(projection, at.Add(time.Minute))
			history := test.buildHistory(source, selection, projection, at)
			projectionIndex := -1
			for index := range history {
				if history[index].ID == projection.ID {
					projectionIndex = index
					break
				}
			}
			if projectionIndex < 0 {
				t.Fatal("projection missing from fixture history")
			}

			event, materialized := legacyAvailabilitySelectionEventV1FromMessage(
				projection,
				projectionIndex,
				history,
			)
			if materialized != test.wantProjectionAuthority || materialized && !event.MaterializesAuthority {
				t.Fatalf(
					"projection authority materialized=%t event=%+v, want %t",
					materialized,
					event,
					test.wantProjectionAuthority,
				)
			}

			state := bootstrapAvailabilitySelectionStateFromHistoryForTest(history)
			if got := state.Status == AvailabilitySelectionStatusBookable; got != test.wantFinalBookable {
				t.Fatalf("final BOOKABLE=%t, want %t: %+v", got, test.wantFinalBookable, state)
			}
			session := sessionWithAvailabilitySelectionStateForTest(Session{}, state)
			draft := collectBookingDraftContextWithPassengerState(
				session,
				history,
				"",
				completePassengerStateForTest(1, 0),
			)
			if draft.HasBookableSelection != test.wantFinalBookable {
				t.Fatalf("draft HasBookableSelection=%t, want %t: %+v", draft.HasBookableSelection, test.wantFinalBookable, draft)
			}
			if !test.wantFinalBookable {
				if state.SelectedOptionIndex != 0 || availabilitySelectionStateHasAggregateV1(state) {
					t.Fatalf("fail-closed state retained selection authority: %+v", state)
				}
				if _, _, ok := resolveBookingCreateSelectionFromState("quero reservar opção 1", state); ok {
					t.Fatal("fail-closed state reached booking_create selection")
				}
				assertNoBookingCreateInputFromAvailabilityHistoryV1(t, history)
			}
		})
	}
}

func TestAvailabilitySelectionSnapshotEnrichmentRejectsConflictingLegacySnapshotV1(t *testing.T) {
	raw := availabilityPromptAuthorityRawResult()
	fullSnapshot := mustAvailabilitySelectionSnapshotV1ForTest(t, &raw, 1)
	selected := selectedAvailabilityResultPayloadFromAvailability(&raw, 1)
	selected[selectedAvailabilitySelectionMessageIDPayloadKey] = "conflict-selection-inbound"
	selected[availabilityPromptSourceMessageIDPayloadKey] = "conflict-prompt-source"
	payload := map[string]interface{}{
		"intent":                             string(IntentSelectAvailabilityOption),
		"selected_option_index":              1,
		selectedAvailabilityResultPayloadKey: selected,
	}
	projection := Message{
		ID:                "conflict-legacy-projection",
		Direction:         "OUTBOUND",
		ProcessingStatus:  "DELIVERED",
		Payload:           cloneMap(payload),
		NormalizedPayload: cloneMap(payload),
		ReceivedAt:        availabilityTestObservedAt(),
		CreatedAt:         availabilityTestObservedAt(),
	}
	projection.NormalizedPayload["delivery_recorded_at"] = projection.ReceivedAt.Format(time.RFC3339Nano)
	source := availabilityPromptAuthorityMessage("conflict-prompt-source", "DELIVERED", raw, []int{0})
	base := materializedAvailabilitySelectionEventForTest(
		"conflict-selection-inbound",
		projection.ID,
		"conflict-prompt-source",
		AvailabilitySelectionSnapshotV1{
			SelectedOptionIndex: 1,
			TripID:              fullSnapshot.TripID,
			BoardStopID:         fullSnapshot.BoardStopID,
			AlightStopID:        fullSnapshot.AlightStopID,
		},
	)

	for _, test := range []struct {
		name   string
		mutate func(*AvailabilitySelectionSnapshotV1)
	}{
		{name: "segment", mutate: func(snapshot *AvailabilitySelectionSnapshotV1) { snapshot.SegmentID = "other-segment" }},
		{name: "trip date", mutate: func(snapshot *AvailabilitySelectionSnapshotV1) { snapshot.TripDate = "2099-12-31" }},
		{name: "route", mutate: func(snapshot *AvailabilitySelectionSnapshotV1) { snapshot.RouteID = "other-route" }},
		{name: "origin stop", mutate: func(snapshot *AvailabilitySelectionSnapshotV1) { snapshot.OriginStopID = "other-origin-stop" }},
		{name: "destination stop", mutate: func(snapshot *AvailabilitySelectionSnapshotV1) { snapshot.DestinationStopID = "other-destination-stop" }},
		{name: "origin", mutate: func(snapshot *AvailabilitySelectionSnapshotV1) { snapshot.Origin = "Outra origem" }},
		{name: "destination", mutate: func(snapshot *AvailabilitySelectionSnapshotV1) { snapshot.Destination = "Outro destino" }},
		{name: "package", mutate: func(snapshot *AvailabilitySelectionSnapshotV1) { snapshot.PackageName = "Outro pacote" }},
		{name: "origin display", mutate: func(snapshot *AvailabilitySelectionSnapshotV1) { snapshot.OriginDisplayName = "Outra origem exibida" }},
		{name: "destination display", mutate: func(snapshot *AvailabilitySelectionSnapshotV1) {
			snapshot.DestinationDisplayName = "Outro destino exibido"
		}},
		{name: "origin depart time", mutate: func(snapshot *AvailabilitySelectionSnapshotV1) { snapshot.OriginDepartTime = "23:59" }},
		{name: "seats available", mutate: func(snapshot *AvailabilitySelectionSnapshotV1) {
			snapshot.SeatsAvailable = fullSnapshot.SeatsAvailable + 1
		}},
		{name: "price", mutate: func(snapshot *AvailabilitySelectionSnapshotV1) { snapshot.Price = fullSnapshot.Price + 1 }},
		{name: "currency", mutate: func(snapshot *AvailabilitySelectionSnapshotV1) { snapshot.Currency = "USD" }},
		{name: "status", mutate: func(snapshot *AvailabilitySelectionSnapshotV1) { snapshot.Status = "CLOSED" }},
		{name: "trip status", mutate: func(snapshot *AvailabilitySelectionSnapshotV1) { snapshot.TripStatus = "CANCELLED" }},
		{name: "trip id", mutate: func(snapshot *AvailabilitySelectionSnapshotV1) { snapshot.TripID = "other-trip" }},
		{name: "board stop", mutate: func(snapshot *AvailabilitySelectionSnapshotV1) { snapshot.BoardStopID = "other-board" }},
		{name: "alight stop", mutate: func(snapshot *AvailabilitySelectionSnapshotV1) { snapshot.AlightStopID = "other-alight" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			conflicting := base
			test.mutate(&conflicting.Snapshot)
			original := conflicting.Snapshot
			got := enrichLegacyAvailabilitySelectionEventSnapshotV1(conflicting, []Message{source, projection})
			if got.MaterializesAuthority {
				t.Fatalf("conflicting snapshot retained authority: %+v", got)
			}
			if got.Snapshot != original {
				t.Fatalf("conflicting snapshot was silently rewritten: got=%+v want=%+v", got.Snapshot, original)
			}
			state := ReduceAvailabilitySelectionEventsV1(
				newAvailabilitySelectionStateV1(),
				[]AvailabilitySelectionEventV1{got},
			)
			if state.Status == AvailabilitySelectionStatusBookable {
				t.Fatalf("conflicting snapshot recovered BOOKABLE: %+v", state)
			}
			if _, _, ok := resolveBookingCreateSelectionFromState("quero reservar opção 1", state); ok {
				t.Fatal("conflicting snapshot reached booking_create")
			}
		})
	}

	compatible := base
	compatible.Snapshot.TripDate = fullSnapshot.TripDate
	got := enrichLegacyAvailabilitySelectionEventSnapshotV1(compatible, []Message{source, projection})
	if !got.MaterializesAuthority || got.Snapshot.TripDate != fullSnapshot.TripDate ||
		got.Snapshot.PackageName != fullSnapshot.PackageName || got.Snapshot.Price != fullSnapshot.Price {
		t.Fatalf("compatible partial snapshot did not recover only missing fields: %+v", got)
	}
}

func TestAvailabilitySelectionSnapshotEnrichmentPreservesPersistedFieldPresenceV1(t *testing.T) {
	raw := availabilityPromptAuthorityRawResult()
	selected := selectedAvailabilityResultPayloadFromAvailability(&raw, 1)
	selected[selectedAvailabilitySelectionMessageIDPayloadKey] = "presence-selection-inbound"
	selected[availabilityPromptSourceMessageIDPayloadKey] = "presence-prompt-source"
	projectionPayload := map[string]interface{}{
		"intent":                             string(IntentSelectAvailabilityOption),
		"selected_option_index":              1,
		selectedAvailabilityResultPayloadKey: cloneMap(selected),
	}
	projection := Message{
		ID:                "presence-legacy-projection",
		Direction:         "OUTBOUND",
		ProcessingStatus:  "DELIVERED",
		Payload:           cloneMap(projectionPayload),
		NormalizedPayload: cloneMap(projectionPayload),
		ReceivedAt:        availabilityTestObservedAt().Add(2 * time.Minute),
		CreatedAt:         availabilityTestObservedAt().Add(2 * time.Minute),
	}
	projection.NormalizedPayload["delivery_recorded_at"] = projection.ReceivedAt.Format(time.RFC3339Nano)
	source := availabilityPromptAuthorityMessage("presence-prompt-source", "DELIVERED", raw, []int{0})

	decodePersistedEvent := func(t *testing.T, snapshot map[string]interface{}) AvailabilitySelectionEventV1 {
		t.Helper()
		message := Message{
			ID:         "presence-selection-inbound",
			Direction:  "INBOUND",
			ReceivedAt: availabilityTestObservedAt().Add(time.Minute),
			CreatedAt:  availabilityTestObservedAt().Add(time.Minute),
			NormalizedPayload: map[string]interface{}{
				availabilitySelectionEventsV1MessageKey: []interface{}{map[string]interface{}{
					"type":                                  string(AvailabilitySelectionEventMaterialized),
					"message_id":                            "presence-selection-inbound",
					"projection_message_id":                 projection.ID,
					"availability_prompt_source_message_id": source.ID,
					"selected_option_index":                 1,
					"snapshot":                              snapshot,
					"materializes_authority":                true,
				}},
			},
		}
		events := availabilitySelectionEventsV1FromMessage(message)
		if len(events) != 1 {
			t.Fatalf("decoded events=%d, want 1", len(events))
		}
		return events[0]
	}

	for _, test := range []struct {
		name          string
		field         string
		setPersisted  bool
		persisted     interface{}
		wantAuthority bool
	}{
		{name: "price absent is filled", field: "price", wantAuthority: true},
		{name: "price zero is a conflict", field: "price", setPersisted: true, persisted: float64(0)},
		{name: "price equal remains", field: "price", setPersisted: true, persisted: selected["price"], wantAuthority: true},
		{name: "seats absent is filled", field: "seats_available", wantAuthority: true},
		{name: "seats zero is a conflict", field: "seats_available", setPersisted: true, persisted: 0},
		{name: "seats equal remains", field: "seats_available", setPersisted: true, persisted: selected["seats_available"], wantAuthority: true},
		{name: "string absent is filled", field: "package_name", wantAuthority: true},
		{name: "string empty is a conflict", field: "package_name", setPersisted: true, persisted: ""},
		{name: "string equal remains", field: "package_name", setPersisted: true, persisted: selected["package_name"], wantAuthority: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot := cloneMap(selected)
			delete(snapshot, selectedAvailabilitySelectionMessageIDPayloadKey)
			delete(snapshot, availabilityPromptSourceMessageIDPayloadKey)
			delete(snapshot, test.field)
			if test.setPersisted {
				snapshot[test.field] = test.persisted
			}
			persistedValue, persistedPresent := snapshot[test.field]
			got := enrichLegacyAvailabilitySelectionEventSnapshotV1(
				decodePersistedEvent(t, snapshot),
				[]Message{source, {
					ID:         "presence-selection-inbound",
					Direction:  "INBOUND",
					ReceivedAt: availabilityTestObservedAt().Add(time.Minute),
					CreatedAt:  availabilityTestObservedAt().Add(time.Minute),
				}, projection},
			)
			if got.MaterializesAuthority != test.wantAuthority {
				t.Fatalf("MaterializesAuthority=%t, want %t: %+v", got.MaterializesAuthority, test.wantAuthority, got)
			}
			if !test.wantAuthority {
				gotPayload := got.Snapshot.payload()
				if !persistedPresent || gotPayload[test.field] != persistedValue {
					t.Fatalf("persisted conflict was rewritten: field=%s got=%v want=%v", test.field, gotPayload[test.field], persistedValue)
				}
			}
			state := ReduceAvailabilitySelectionEventsV1(
				newAvailabilitySelectionStateV1(),
				[]AvailabilitySelectionEventV1{got},
			)
			if test.wantAuthority {
				if state.Status != AvailabilitySelectionStatusBookable {
					t.Fatalf("compatible reload lost BOOKABLE: %+v", state)
				}
				return
			}
			if state.Status == AvailabilitySelectionStatusBookable {
				t.Fatalf("conflicting reload recovered BOOKABLE: %+v", state)
			}
			if _, _, ok := resolveBookingCreateSelectionFromState("quero reservar opção 1", state); ok {
				t.Fatal("conflicting reload reached booking_create")
			}
		})
	}
}

func TestAvailabilitySelectionSnapshotPresenceRejectsNullAndInvalidTypesV1(t *testing.T) {
	selected := selectedAvailabilityResultPayloadFromAvailability(
		func() *AvailabilitySearchResult {
			raw := availabilityPromptAuthorityRawResult()
			return &raw
		}(),
		1,
	)
	delete(selected, selectedAvailabilitySelectionMessageIDPayloadKey)
	delete(selected, availabilityPromptSourceMessageIDPayloadKey)

	for _, test := range []struct {
		name  string
		field string
		value interface{}
	}{
		{name: "string null", field: "package_name", value: nil},
		{name: "string invalid type", field: "package_name", value: 7},
		{name: "float null", field: "price", value: nil},
		{name: "float invalid type", field: "price", value: "0"},
		{name: "integer null", field: "seats_available", value: nil},
		{name: "integer non integral", field: "seats_available", value: 1.5},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot := cloneMap(selected)
			snapshot[test.field] = test.value
			if got, presence, ok := availabilitySelectionSnapshotV1FromPayloadWithPresence(snapshot); ok {
				t.Fatalf("invalid typed field decoded: snapshot=%+v presence=%b", got, presence)
			}

			message := Message{
				ID:         "invalid-presence-selection",
				Direction:  "INBOUND",
				ReceivedAt: availabilityTestObservedAt(),
				CreatedAt:  availabilityTestObservedAt(),
				NormalizedPayload: map[string]interface{}{
					availabilitySelectionEventsV1MessageKey: []interface{}{map[string]interface{}{
						"type":                                  string(AvailabilitySelectionEventMaterialized),
						"message_id":                            "invalid-presence-selection",
						"projection_message_id":                 "invalid-presence-projection",
						"availability_prompt_source_message_id": "invalid-presence-source",
						"selected_option_index":                 1,
						"snapshot":                              snapshot,
						"materializes_authority":                true,
					}},
				},
			}
			events := availabilitySelectionEventsV1FromMessage(message)
			if len(events) != 0 {
				t.Fatalf("invalid persisted field decoded events: %+v", events)
			}
			state := ReduceAvailabilitySelectionEventsV1(newAvailabilitySelectionStateV1(), events)
			if state.Status == AvailabilitySelectionStatusBookable {
				t.Fatalf("invalid persisted field recovered BOOKABLE: %+v", state)
			}
			if _, _, ok := resolveBookingCreateSelectionFromState("quero reservar opção 1", state); ok {
				t.Fatal("invalid persisted field reached booking_create")
			}
		})
	}
}

func TestAvailabilitySelectionLiveSnapshotPresenceSurvivesSerializationV1(t *testing.T) {
	raw := availabilityPromptAuthorityRawResult()
	raw.Results[0].Price = 0
	raw.Results[0].SeatsAvailable = 0
	raw.Results[0].PackageName = ""
	raw.Filter.PackageName = ""
	snapshot, presence, ok := availabilitySelectionSnapshotV1FromAvailabilityWithPresence(&raw, 1)
	if !ok {
		t.Fatal("build live zero-value availability snapshot")
	}

	for _, test := range []struct {
		name            string
		presence        availabilitySelectionSnapshotPresenceV1
		wantZeroKeys    bool
		wantEmptyString bool
	}{
		{
			name:            "present zero and empty values",
			presence:        presence,
			wantZeroKeys:    true,
			wantEmptyString: true,
		},
		{
			name: "truly absent zero and empty values",
			presence: presence &^ (availabilitySelectionSnapshotPricePresentV1 |
				availabilitySelectionSnapshotSeatsAvailablePresentV1 |
				availabilitySelectionSnapshotPackageNamePresentV1),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			event := AvailabilitySelectionEventV1{
				Type:                              AvailabilitySelectionEventMaterialized,
				MessageID:                         "live-presence-selection",
				ProjectionMessageID:               "live-presence-projection",
				AvailabilityPromptSourceMessageID: "live-presence-source",
				SelectedOptionIndex:               1,
				Snapshot:                          snapshot,
				MaterializesAuthority:             true,
				snapshotPresence:                  test.presence,
				snapshotPresenceKnown:             true,
			}
			rawEvent, err := json.Marshal(event)
			if err != nil {
				t.Fatalf("marshal live event: %v", err)
			}
			var persisted map[string]interface{}
			if err := json.Unmarshal(rawEvent, &persisted); err != nil {
				t.Fatalf("decode persisted live event: %v", err)
			}
			persistedSnapshot := asMap(persisted["snapshot"])
			_, pricePresent := persistedSnapshot["price"]
			_, seatsPresent := persistedSnapshot["seats_available"]
			_, packagePresent := persistedSnapshot["package_name"]
			if pricePresent != test.wantZeroKeys || seatsPresent != test.wantZeroKeys || packagePresent != test.wantEmptyString {
				t.Fatalf(
					"serialized presence price=%t seats=%t package=%t, want zero=%t empty=%t: %s",
					pricePresent,
					seatsPresent,
					packagePresent,
					test.wantZeroKeys,
					test.wantEmptyString,
					string(rawEvent),
				)
			}

			message := Message{
				ID:                event.MessageID,
				Direction:         "INBOUND",
				ReceivedAt:        availabilityTestObservedAt(),
				CreatedAt:         availabilityTestObservedAt(),
				NormalizedPayload: map[string]interface{}{availabilitySelectionEventsV1MessageKey: []interface{}{persisted}},
			}
			reloaded := availabilitySelectionEventsV1FromMessage(message)
			if len(reloaded) != 1 {
				t.Fatalf("reloaded events=%d, want 1", len(reloaded))
			}
			if got := availabilitySelectionEventSnapshotPresenceV1(reloaded[0]); got != test.presence {
				t.Fatalf("reloaded presence=%b, want %b", got, test.presence)
			}
			state := ReduceAvailabilitySelectionEventsV1(newAvailabilitySelectionStateV1(), reloaded)
			if state.Status != AvailabilitySelectionStatusBookable {
				t.Fatalf("live round-trip lost authority: %+v", state)
			}
		})
	}
}

func TestAvailabilitySelectionStateV1SimpleRejectionPersistsSourceAndTombstone(t *testing.T) {
	store := newFakeStore()
	now := availabilityTestObservedAt()
	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:        "WHATSAPP",
		ContactKey:     "availability-selection-state-restart",
		CustomerPhone:  "availability-selection-state-restart",
		LastMessageAt:  &now,
		LastInboundAt:  &now,
		LastOutboundAt: &now,
	})
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}
	availability := availabilityOptionPromptFutureResultAt(now)
	snapshot := mustAvailabilitySelectionSnapshotV1ForTest(t, &availability, 1)
	selectionInbound := createAvailabilitySelectionInboundForTest(t, store, session.ID, "1", now)
	materialized, err := store.ApplyPassengerClarificationEventsV1(
		context.Background(),
		ApplyPassengerClarificationEventsV1Input{
			SessionID: session.ID,
			AvailabilitySelectionEvents: []AvailabilitySelectionEventV1{
				materializedAvailabilitySelectionEventForTest(
					selectionInbound.ID,
					"selection-projection",
					"availability-prompt",
					snapshot,
				),
			},
		},
	)
	if err != nil {
		t.Fatalf("materialize selection: %v", err)
	}
	if materialized.AvailabilitySelectionState.Status != AvailabilitySelectionStatusBookable {
		t.Fatalf("selection did not become bookable: %+v", materialized.AvailabilitySelectionState)
	}

	rejectionInbound := createAvailabilitySelectionInboundForTest(
		t,
		store,
		session.ID,
		"não quero a opção 1",
		now.Add(time.Minute),
	)
	rejected, err := store.ApplyPassengerClarificationEventsV1(
		context.Background(),
		ApplyPassengerClarificationEventsV1Input{
			SessionID: session.ID,
			AvailabilitySelectionEvents: []AvailabilitySelectionEventV1{{
				Type:                              AvailabilitySelectionEventRejected,
				MessageID:                         rejectionInbound.ID,
				AvailabilityPromptSourceMessageID: "availability-prompt",
				RejectedOptionIndexes:             []int{1},
			}},
		},
	)
	if err != nil {
		t.Fatalf("reject selection: %v", err)
	}
	state := rejected.AvailabilitySelectionState
	if state.Status != AvailabilitySelectionStatusRejected ||
		state.Tombstone == nil ||
		state.Tombstone.AvailabilityPromptSourceMessageID != "availability-prompt" {
		t.Fatalf("simple rejection did not retain its real prompt source: %+v", state)
	}
	persistedRejection := store.messages[rejectionInbound.ID]
	events := availabilitySelectionEventsV1FromMessage(persistedRejection)
	if len(events) != 1 ||
		events[0].AvailabilityPromptSourceMessageID != "availability-prompt" {
		t.Fatalf("inbound rejection event lost prompt source: %+v", persistedRejection.NormalizedPayload)
	}

	reloaded, err := store.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	reloadedState, ok := availabilitySelectionStateV1FromSession(reloaded)
	if !ok || reloadedState.Status != AvailabilitySelectionStatusRejected ||
		reloadedState.Tombstone == nil ||
		reloadedState.Tombstone.AvailabilityPromptSourceMessageID != "availability-prompt" {
		t.Fatalf("restart lost rejection tombstone: %+v", reloadedState)
	}
	if _, _, ok := resolveBookingCreateSelectionFromState("quero reservar opção 1", reloadedState); ok {
		t.Fatal("booking_create reopened without BOOKABLE state")
	}
}

func TestAvailabilitySelectionStateV1BookableStateFeedsDraftAndBookingCreate(t *testing.T) {
	availability := availabilityOptionPromptFutureResultAt(availabilityTestObservedAt())
	snapshot := mustAvailabilitySelectionSnapshotV1ForTest(t, &availability, 1)
	state := ReduceAvailabilitySelectionEventsV1(
		newAvailabilitySelectionStateV1(),
		[]AvailabilitySelectionEventV1{
			materializedAvailabilitySelectionEventForTest(
				"selection-event",
				"selection-projection",
				"availability-prompt",
				snapshot,
			),
		},
	)
	state.BootstrapCompleted = true
	session := Session{Metadata: map[string]interface{}{
		"memory": map[string]interface{}{availabilitySelectionStateV1MemoryKey: state},
	}}
	draft := collectBookingDraftContextWithPassengerState(
		session,
		nil,
		"",
		completePassengerStateForTest(1, 0),
	)
	if !draft.HasBookableSelection ||
		draft.TripID != snapshot.TripID ||
		draft.BoardStopID != snapshot.BoardStopID ||
		draft.AlightStopID != snapshot.AlightStopID ||
		draft.SelectionMessageID != "selection-projection" ||
		draft.AvailabilityPromptSourceMessageID != "availability-prompt" {
		t.Fatalf("durable BOOKABLE state did not feed booking draft: %+v", draft)
	}
	index, item, ok := resolveBookingCreateSelectionFromState("quero reservar opção 1", state)
	if !ok || index != 1 ||
		item.TripID != snapshot.TripID ||
		item.BoardStopID != snapshot.BoardStopID ||
		item.AlightStopID != snapshot.AlightStopID {
		t.Fatalf("durable BOOKABLE state did not feed booking_create: index=%d item=%+v ok=%t", index, item, ok)
	}
}

func TestAvailabilitySelectionStateV1RoundTripKeepsTombstone(t *testing.T) {
	state := invalidAvailabilitySelectionStateV1("RESTART_TOMBSTONE")
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	var value map[string]interface{}
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("unmarshal state map: %v", err)
	}
	decoded, ok := decodeAvailabilitySelectionStateV1(value)
	if !ok || decoded.Status != AvailabilitySelectionStatusInvalidated ||
		decoded.Tombstone == nil ||
		decoded.Tombstone.ReasonCode != "RESTART_TOMBSTONE" {
		t.Fatalf("round trip lost tombstone: %+v ok=%t", decoded, ok)
	}
}

func TestAvailabilitySelectionStateV1BookingCreateRequiresBookableStatus(t *testing.T) {
	for _, state := range []AvailabilitySelectionStateV1{
		func() AvailabilitySelectionStateV1 {
			state := newAvailabilitySelectionStateV1()
			state.BootstrapCompleted = true
			return state
		}(),
		invalidAvailabilitySelectionStateV1("TEST_INVALIDATED"),
		func() AvailabilitySelectionStateV1 {
			state := ReduceAvailabilitySelectionEventsV1(
				newAvailabilitySelectionStateV1(),
				orderedAvailabilitySelectionEventsForTest(AvailabilitySelectionEventV1{
					Type:                              AvailabilitySelectionEventRejected,
					MessageID:                         "test-rejection",
					AvailabilityPromptSourceMessageID: "test-prompt",
					RejectedWholeContext:              true,
				}),
			)
			state.BootstrapCompleted = true
			return state
		}(),
	} {
		if _, _, ok := resolveBookingCreateSelectionFromState(
			"quero reservar opção 1",
			state,
		); ok {
			t.Fatalf("booking_create accepted non-BOOKABLE state: %+v", state)
		}
	}
}

func TestAvailabilitySelectionStateV1ConcurrentFakeApplyKeepsBothEvents(t *testing.T) {
	base := newFakeStore()
	store := &serializedAvailabilitySelectionFakeStore{fakeStore: base}
	now := availabilityTestObservedAt()
	session, err := base.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:        "WHATSAPP",
		ContactKey:     "availability-selection-concurrent-fake",
		CustomerPhone:  "availability-selection-concurrent-fake",
		LastMessageAt:  &now,
		LastInboundAt:  &now,
		LastOutboundAt: &now,
	})
	if err != nil {
		t.Fatalf("seed concurrent fake session: %v", err)
	}
	availabilityA := availabilityOptionPromptFutureResultAt(now)
	availabilityB := availabilityOptionPromptFutureResultAt(now.Add(24 * time.Hour))
	availabilityB.Results[0].TripID = "concurrent-trip-b"
	availabilityB.Results[0].BoardStopID = "concurrent-board-b"
	availabilityB.Results[0].AlightStopID = "concurrent-alight-b"
	snapshots := []AvailabilitySelectionSnapshotV1{
		mustAvailabilitySelectionSnapshotV1ForTest(t, &availabilityA, 1),
		mustAvailabilitySelectionSnapshotV1ForTest(t, &availabilityB, 1),
	}
	messageIDs := []string{"concurrent-selection-a", "concurrent-selection-b"}
	for index, messageID := range messageIDs {
		createAvailabilitySelectionInboundForTest(
			t,
			base,
			session.ID,
			messageID,
			now.Add(time.Duration(index)*time.Second),
		)
	}
	inputs := make([]ApplyPassengerClarificationEventsV1Input, 0, len(messageIDs))
	for index, messageID := range messageIDs {
		inputs = append(inputs, ApplyPassengerClarificationEventsV1Input{
			SessionID: session.ID,
			AvailabilitySelectionEvents: []AvailabilitySelectionEventV1{
				materializedAvailabilitySelectionEventForTest(
					base.byIdempotencyKey["availability-selection-"+messageID],
					"concurrent-projection-"+messageID,
					"concurrent-prompt-"+messageID,
					snapshots[index],
				),
			},
		})
	}

	results := make(chan error, len(inputs))
	for _, input := range inputs {
		input := input
		go func() {
			_, applyErr := store.ApplyPassengerClarificationEventsV1(context.Background(), input)
			results <- applyErr
		}()
	}
	for range inputs {
		if err := <-results; err != nil {
			t.Fatalf("concurrent fake apply: %v", err)
		}
	}
	reloaded, err := store.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("reload concurrent fake state: %v", err)
	}
	state, ok := availabilitySelectionStateV1FromSession(reloaded)
	if !ok || state.Status != AvailabilitySelectionStatusBookable ||
		len(state.AppliedEventIDs) != 2 {
		t.Fatalf("serialized fake apply lost an event: %+v", state)
	}
	if state.Snapshot.TripID != snapshots[0].TripID &&
		state.Snapshot.TripID != snapshots[1].TripID {
		t.Fatalf("serialized fake apply produced an unknown authority: %+v", state)
	}
}

func TestAvailabilitySelectionReplayUsesFullStructuredSessionEveryTime(t *testing.T) {
	store := newFakeStore()
	now := availabilityTestObservedAt()
	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:        "WHATSAPP",
		ContactKey:     "availability-selection-full-bootstrap",
		CustomerPhone:  "availability-selection-full-bootstrap",
		LastMessageAt:  &now,
		LastInboundAt:  &now,
		LastOutboundAt: &now,
	})
	if err != nil {
		t.Fatalf("seed bootstrap session: %v", err)
	}
	availability := availabilityOptionPromptFutureResultAt(now)
	snapshot := mustAvailabilitySelectionSnapshotV1ForTest(t, &availability, 1)
	bootstrapEvent := materializedAvailabilitySelectionEventForTest(
		"bootstrap-selection-inbound",
		"bootstrap-selection-projection",
		"bootstrap-availability-prompt",
		snapshot,
	)
	bootstrapMessage, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        session.ID,
		Direction:        "INBOUND",
		Kind:             "TEXT",
		IdempotencyKey:   "bootstrap-selection-inbound",
		Body:             "body is not a bootstrap input",
		ProcessingStatus: "PROCESSED",
		NormalizedPayload: map[string]interface{}{
			availabilitySelectionEventsV1MessageKey: []AvailabilitySelectionEventV1{bootstrapEvent},
		},
		ReceivedAt: now,
	})
	if err != nil {
		t.Fatalf("seed structured bootstrap event: %v", err)
	}
	for index := 0; index < 60; index++ {
		if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
			SessionID:        session.ID,
			Direction:        "OUTBOUND",
			Kind:             "TEXT",
			IdempotencyKey:   "bootstrap-filler-" + strconv.Itoa(index),
			Body:             "window filler",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       now.Add(time.Duration(index+1) * time.Second),
		}); err != nil {
			t.Fatalf("seed bootstrap filler %d: %v", index, err)
		}
	}
	bootstrapped, err := store.ApplyPassengerClarificationEventsV1(
		context.Background(),
		ApplyPassengerClarificationEventsV1Input{SessionID: session.ID},
	)
	if err != nil {
		t.Fatalf("bootstrap full structured session: %v", err)
	}
	if bootstrapped.AvailabilitySelectionState.Status != AvailabilitySelectionStatusBookable ||
		bootstrapped.AvailabilitySelectionState.SelectionEventMessageID != bootstrapMessage.ID ||
		!bootstrapped.AvailabilitySelectionState.BootstrapCompleted {
		t.Fatalf("full-session bootstrap lost old structured authority: %+v", bootstrapped.AvailabilitySelectionState)
	}

	if _, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        session.ID,
		Direction:        "INBOUND",
		Kind:             "TEXT",
		IdempotencyKey:   "late-direct-rejection",
		ProcessingStatus: "PROCESSED",
		NormalizedPayload: map[string]interface{}{
			availabilitySelectionEventsV1MessageKey: []AvailabilitySelectionEventV1{{
				Type:                              AvailabilitySelectionEventRejected,
				MessageID:                         "late-direct-rejection",
				AvailabilityPromptSourceMessageID: bootstrapEvent.AvailabilityPromptSourceMessageID,
				RejectedWholeContext:              true,
			}},
		},
		ReceivedAt: now.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("seed late direct structured event: %v", err)
	}
	reapplied, err := store.ApplyPassengerClarificationEventsV1(
		context.Background(),
		ApplyPassengerClarificationEventsV1Input{SessionID: session.ID},
	)
	if err != nil {
		t.Fatalf("reapply bootstrapped state: %v", err)
	}
	if reapplied.AvailabilitySelectionState.Status != AvailabilitySelectionStatusRejected ||
		reapplied.AvailabilitySelectionState.Tombstone == nil ||
		len(reapplied.AvailabilitySelectionState.AppliedEventIDs) != 2 {
		t.Fatalf("canonical replay ignored a late durable event: %+v", reapplied.AvailabilitySelectionState)
	}
}

func TestAvailabilitySelectionStateV1UnitListRejectionDoesNotReopenBookingCreate(t *testing.T) {
	store := newFakeStoreWithPassengerAuthority()
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: "fallback", Model: "gpt-test"},
	}
	booking := &fakeBookingCreator{enabled: true}
	svc := NewService(
		store,
		configForAvailabilitySelectionStateTest(),
		runner,
		booking,
	)
	now := availabilityTestObservedAt()
	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:        "WHATSAPP",
		ContactKey:     "availability-unit-list-rejection",
		CustomerPhone:  "availability-unit-list-rejection",
		CustomerName:   "Joao Vitor Messias",
		LastMessageAt:  &now,
		LastOutboundAt: &now,
	})
	if err != nil {
		t.Fatalf("seed unit-list session: %v", err)
	}
	availability := availabilityOptionPromptFutureResultAt(now)
	availabilityToolContext := map[string]interface{}{
		toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availability),
	}
	prompt, err := store.SaveAgentDraft(context.Background(), SaveAgentDraftInput{
		SessionID:        session.ID,
		IdempotencyKey:   "availability-unit-list-prompt",
		Body:             buildAvailabilityListReply(availability),
		SenderName:       "SHABAS",
		ProcessingStatus: messageStatusAutomationSent,
		Payload: map[string]interface{}{
			"tool_context": availabilityToolContext,
		},
		NormalizedPayload: map[string]interface{}{
			"tool_context": cloneMap(availabilityToolContext),
		},
		RecordedAt: now,
	})
	if err != nil {
		t.Fatalf("seed unit-list prompt: %v", err)
	}

	materialized := ingestAndReprocessActivePromptFlowTurn(
		t,
		svc,
		session.ContactKey,
		"availability-unit-list-select",
		"1",
	)
	if materialized.Draft == nil {
		t.Fatal("expected materialized unit-list draft")
	}
	selectedSession, err := store.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("reload selected session: %v", err)
	}
	selectedState, ok := availabilitySelectionStateV1FromSession(selectedSession)
	if !ok || selectedState.Status != AvailabilitySelectionStatusBookable ||
		selectedState.AvailabilityPromptSourceMessageID != prompt.Message.ID ||
		selectedState.SelectionProjectionMessageID != materialized.Draft.ID {
		t.Fatalf("unit-list selection authority was not persisted: %+v", selectedState)
	}
	markSessionMessagesAutomationSent(t, store, session.ID)

	_ = ingestAndReprocessActivePromptFlowTurn(
		t,
		svc,
		session.ContactKey,
		"availability-unit-list-reject",
		"não quero a opção 1",
	)
	rejectedSession, err := store.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("reload rejected session: %v", err)
	}
	rejectedState, ok := availabilitySelectionStateV1FromSession(rejectedSession)
	if !ok || rejectedState.Status != AvailabilitySelectionStatusRejected ||
		rejectedState.Tombstone == nil ||
		rejectedState.Tombstone.AvailabilityPromptSourceMessageID != prompt.Message.ID {
		t.Fatalf("unit-list rejection did not persist its tombstone: %+v", rejectedState)
	}
	var rejectionEvents []AvailabilitySelectionEventV1
	for _, messageID := range store.messageOrder {
		message := store.messages[messageID]
		if message.SessionID == session.ID && message.Direction == "INBOUND" &&
			message.Body == "não quero a opção 1" {
			rejectionEvents = availabilitySelectionEventsV1FromMessage(message)
		}
	}
	if len(rejectionEvents) != 1 ||
		rejectionEvents[0].AvailabilityPromptSourceMessageID != prompt.Message.ID {
		t.Fatalf("simple inbound rejection did not persist the original prompt source: %+v", rejectionEvents)
	}
	projectionOnlyHistory := []Message{*materialized.Draft}
	bookingDraft := collectBookingDraftContextWithPassengerState(
		rejectedSession,
		projectionOnlyHistory,
		"",
		completePassengerStateForTest(1, 0),
	)
	if bookingDraft.HasBookableSelection ||
		bookingDraft.SelectedOptionIndex != 0 ||
		bookingDraft.TripID != "" ||
		bookingDraft.BoardStopID != "" ||
		bookingDraft.AlightStopID != "" {
		t.Fatalf("projection-only window reopened rejected authority: %+v", bookingDraft)
	}
	if _, _, ok := resolveBookingCreateSelectionFromState(
		"quero reservar opção 1",
		rejectedState,
	); ok {
		t.Fatal("booking_create resolved a rejected unit-list selection")
	}
	if booking.calls != 0 {
		t.Fatalf("unit-list selection/rejection dispatched booking_create %d times", booking.calls)
	}
}

func TestAvailabilitySelectionStateV1CurrentTurnRequiresAppliedMaterialization(t *testing.T) {
	base := newFakeStoreWithPassengerAuthority()
	store := &droppingAvailabilitySelectionEventStore{fakeStore: base}
	runner := &fakeAgentRunner{
		enabled: true,
		result:  RunAgentResult{ReplyText: "unsafe runner reply", Model: "gpt-test"},
	}
	booking := &fakeBookingCreator{enabled: true}
	svc := NewService(
		store,
		configForAvailabilitySelectionStateTest(),
		runner,
		booking,
	)
	now := availabilityTestObservedAt()
	session, err := base.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:        "WHATSAPP",
		ContactKey:     "availability-selection-unapplied-event",
		CustomerPhone:  "availability-selection-unapplied-event",
		LastMessageAt:  &now,
		LastOutboundAt: &now,
	})
	if err != nil {
		t.Fatalf("seed unapplied-event session: %v", err)
	}
	availability := availabilityOptionPromptFutureResultAt(now)
	if _, err := base.SaveAgentDraft(context.Background(), SaveAgentDraftInput{
		SessionID:        session.ID,
		IdempotencyKey:   "availability-unapplied-event-prompt",
		Body:             buildAvailabilityListReply(availability),
		SenderName:       "SHABAS",
		ProcessingStatus: messageStatusAutomationSent,
		Payload: map[string]interface{}{
			"tool_context": map[string]interface{}{
				toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availability),
			},
		},
		RecordedAt: now,
	}); err != nil {
		t.Fatalf("seed unapplied-event prompt: %v", err)
	}

	out := ingestAndReprocessActivePromptFlowTurn(
		t,
		svc,
		session.ContactKey,
		"availability-unapplied-event-select",
		"1",
	)
	if out.Draft == nil {
		t.Fatal("expected fail-closed draft when materialization was not applied")
	}
	if snapshot := asMap(out.Draft.NormalizedPayload[selectedAvailabilityResultPayloadKey]); len(snapshot) != 0 {
		t.Fatalf("unapplied materialization leaked a selected snapshot: %+v", snapshot)
	}
	reloaded, err := base.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("reload unapplied-event session: %v", err)
	}
	state, ok := availabilitySelectionStateV1FromSession(reloaded)
	if !ok || state.Status == AvailabilitySelectionStatusBookable {
		t.Fatalf("unapplied event became bookable: %+v ok=%t", state, ok)
	}
	if runner.calls != 0 || booking.calls != 0 {
		t.Fatalf(
			"unapplied materialization reached external work: runner=%d booking_create=%d",
			runner.calls,
			booking.calls,
		)
	}
}

func TestAvailabilitySelectionStateV1LegacyInvalidationBoundaryOrdersBootstrap(t *testing.T) {
	now := availabilityTestObservedAt()
	availabilityA := availabilityOptionPromptFutureResultAt(now)
	availabilityB := availabilityOptionPromptFutureResultAt(now.Add(24 * time.Hour))
	availabilityB.Results[0].TripID = "post-boundary-trip"
	availabilityB.Results[0].BoardStopID = "post-boundary-board"
	availabilityB.Results[0].AlightStopID = "post-boundary-alight"
	snapshotA := mustAvailabilitySelectionSnapshotV1ForTest(t, &availabilityA, 1)
	snapshotB := mustAvailabilitySelectionSnapshotV1ForTest(t, &availabilityB, 1)

	for _, test := range []struct {
		name             string
		addPostBoundary  bool
		wantStatus       AvailabilitySelectionStatusV1
		wantSelectedTrip string
	}{
		{
			name:       "pre-boundary selection stays invalidated",
			wantStatus: AvailabilitySelectionStatusInvalidated,
		},
		{
			name:             "post-boundary materialization reopens bookable",
			addPostBoundary:  true,
			wantStatus:       AvailabilitySelectionStatusBookable,
			wantSelectedTrip: snapshotB.TripID,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := newFakeStore()
			session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
				Channel:       "WHATSAPP",
				ContactKey:    "legacy-boundary-" + test.name,
				CustomerPhone: "legacy-boundary-" + test.name,
			})
			if err != nil {
				t.Fatalf("seed invalidated session: %v", err)
			}
			oldSelection := createAvailabilitySelectionInboundForTest(
				t,
				store,
				session.ID,
				"legacy-pre-boundary-selection-"+test.name,
				now,
			)
			persistAvailabilitySelectionEventOnFakeMessageForTest(
				t,
				store,
				oldSelection.ID,
				materializedAvailabilitySelectionEventForTest(
					oldSelection.ID,
					"legacy-pre-boundary-projection",
					"legacy-pre-boundary-prompt",
					snapshotA,
				),
			)
			boundary := createAvailabilitySelectionInboundForTest(
				t,
				store,
				session.ID,
				"legacy-invalidation-boundary-"+test.name,
				now.Add(time.Minute),
			)
			if test.addPostBoundary {
				postBoundary := createAvailabilitySelectionInboundForTest(
					t,
					store,
					session.ID,
					"legacy-post-boundary-selection-"+test.name,
					now.Add(2*time.Minute),
				)
				persistAvailabilitySelectionEventOnFakeMessageForTest(
					t,
					store,
					postBoundary.ID,
					materializedAvailabilitySelectionEventForTest(
						postBoundary.ID,
						"legacy-post-boundary-projection",
						"legacy-post-boundary-prompt",
						snapshotB,
					),
				)
			}
			session = store.sessions[session.ID]
			session.Metadata = map[string]interface{}{
				"agent": map[string]interface{}{
					canonicalAvailabilityFactsInvalidatedMetadataKey:               true,
					canonicalAvailabilityFactsInvalidatedAfterMessageIDMetadataKey: boundary.ID,
					canonicalAvailabilityFactsInvalidatedAfterCreatedAtMetadataKey: boundary.CreatedAt.UTC().Format(time.RFC3339Nano),
				},
			}
			store.sessions[session.ID] = session

			applied, err := store.ApplyPassengerClarificationEventsV1(
				context.Background(),
				ApplyPassengerClarificationEventsV1Input{SessionID: session.ID},
			)
			if err != nil {
				t.Fatalf("bootstrap invalidated availability state: %v", err)
			}
			state := applied.AvailabilitySelectionState
			if state.Status != test.wantStatus || state.Snapshot.TripID != test.wantSelectedTrip {
				t.Fatalf(
					"boundary bootstrap status/trip=(%s,%q), want (%s,%q): %+v",
					state.Status,
					state.Snapshot.TripID,
					test.wantStatus,
					test.wantSelectedTrip,
					state,
				)
			}
		})
	}
}

func TestAvailabilitySelectionStateV1LegacyMarkerWithoutBoundaryFailsClosed(t *testing.T) {
	store := newFakeStore()
	now := availabilityTestObservedAt()
	session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
		Channel:       "WHATSAPP",
		ContactKey:    "legacy-marker-without-boundary",
		CustomerPhone: "legacy-marker-without-boundary",
		Metadata: map[string]interface{}{
			"memory": map[string]interface{}{
				canonicalAvailabilityFactsInvalidatedMetadataKey: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("seed legacy marker session: %v", err)
	}
	availability := availabilityOptionPromptFutureResultAt(now)
	snapshot := mustAvailabilitySelectionSnapshotV1ForTest(t, &availability, 1)
	oldSelection := createAvailabilitySelectionInboundForTest(
		t,
		store,
		session.ID,
		"legacy-marker-old-selection",
		now,
	)
	persistAvailabilitySelectionEventOnFakeMessageForTest(
		t,
		store,
		oldSelection.ID,
		materializedAvailabilitySelectionEventForTest(
			oldSelection.ID,
			"legacy-marker-old-projection",
			"legacy-marker-old-prompt",
			snapshot,
		),
	)

	applied, err := store.ApplyPassengerClarificationEventsV1(
		context.Background(),
		ApplyPassengerClarificationEventsV1Input{SessionID: session.ID},
	)
	if err != nil {
		t.Fatalf("bootstrap legacy marker: %v", err)
	}
	if applied.AvailabilitySelectionState.Status != AvailabilitySelectionStatusInvalidated ||
		applied.AvailabilitySelectionState.SelectedOptionIndex != 0 ||
		applied.AvailabilitySelectionState.Snapshot != (AvailabilitySelectionSnapshotV1{}) {
		t.Fatalf("legacy marker without boundary restored past authority: %+v", applied.AvailabilitySelectionState)
	}

	freshSelection := createAvailabilitySelectionInboundForTest(
		t,
		store,
		session.ID,
		"legacy-marker-fresh-explicit-selection",
		now.Add(time.Minute),
	)
	reopened, err := store.ApplyPassengerClarificationEventsV1(
		context.Background(),
		ApplyPassengerClarificationEventsV1Input{
			SessionID: session.ID,
			AvailabilitySelectionEvents: []AvailabilitySelectionEventV1{
				materializedAvailabilitySelectionEventForTest(
					freshSelection.ID,
					"legacy-marker-fresh-projection",
					"legacy-marker-fresh-prompt",
					snapshot,
				),
			},
		},
	)
	if err != nil {
		t.Fatalf("apply explicit materialization after legacy marker: %v", err)
	}
	if reopened.AvailabilitySelectionState.Status != AvailabilitySelectionStatusInvalidated ||
		reopened.AvailabilitySelectionState.SelectionEventMessageID != "" ||
		reopened.AvailabilitySelectionState.Snapshot != (AvailabilitySelectionSnapshotV1{}) {
		t.Fatalf("boundary-less legacy marker stopped failing closed after replay: %+v", reopened.AvailabilitySelectionState)
	}
}

func TestAvailabilitySelectionStateV1LegacyAuthorityRequiresExactStructuralSource(t *testing.T) {
	now := availabilityTestObservedAt()
	availability := availabilityOptionPromptFutureResultAt(now)
	promptA := legacyAvailabilityPromptMessageForStateTest(
		"legacy-prompt-a",
		availability,
		now,
	)
	promptB := legacyAvailabilityPromptMessageForStateTest(
		"legacy-prompt-b",
		availability,
		now.Add(time.Second),
	)
	selection := legacyAvailabilitySelectionMessageForStateTest(
		"legacy-selection-projection",
		availability,
		IntentSelectAvailabilityOption,
		now.Add(3*time.Second),
		promptA.ID,
	)
	selectionInbound := legacyAvailabilitySelectionInboundForStateTest(
		selection,
		now.Add(2*time.Second),
	)

	t.Run("exact preceding source becomes bookable", func(t *testing.T) {
		state := bootstrapAvailabilitySelectionStateFromHistoryForTest(
			[]Message{promptA, selectionInbound, selection},
		)
		if state.Status != AvailabilitySelectionStatusBookable ||
			state.AvailabilityPromptSourceMessageID != promptA.ID ||
			state.SelectionProjectionMessageID != selection.ID ||
			state.SelectionEventMessageID != selectionInbound.ID {
			t.Fatalf("legacy selection did not resolve exact source without projection authority: %+v", state)
		}
		if compareAvailabilitySelectionEventOrderV1(
			state.LastAppliedEventOrder,
			availabilitySelectionEventOrderFromMessageV1(selectionInbound, 0),
		) != 0 {
			t.Fatalf("legacy selection used projection order instead of inbound order: %+v", state.LastAppliedEventOrder)
		}
	})

	t.Run("structural prompt without canonical delivery never becomes bookable", func(t *testing.T) {
		for _, test := range []struct {
			name                string
			status              string
			hasDeliveryEvidence bool
		}{
			{name: "empty status with evidence", hasDeliveryEvidence: true},
			{name: "received status with evidence", status: "RECEIVED", hasDeliveryEvidence: true},
			{name: "processed status with evidence", status: "PROCESSED", hasDeliveryEvidence: true},
			{name: "unknown status with evidence", status: "UNKNOWN_STATUS", hasDeliveryEvidence: true},
			{name: "sent status without evidence", status: "SENT"},
			{name: "automation sent without evidence", status: messageStatusAutomationSent},
		} {
			t.Run(test.name, func(t *testing.T) {
				source := availabilityPromptAuthorityMessage(
					"unconfirmed-structural-"+strings.ReplaceAll(test.name, " ", "-"),
					test.status,
					availability,
					[]int{0},
				)
				if test.hasDeliveryEvidence {
					source.NormalizedPayload["delivery_recorded_at"] = now.UTC().Format(time.RFC3339Nano)
				} else {
					delete(source.NormalizedPayload, "delivery_recorded_at")
				}
				state := bootstrapAvailabilitySelectionStateFromHistoryForTest(
					[]Message{source, selectionInbound, selection},
				)
				if state.Status != AvailabilitySelectionStatusNone ||
					state.AvailabilityPromptSourceMessageID != "" ||
					state.SelectionEventMessageID != "" ||
					state.Snapshot != (AvailabilitySelectionSnapshotV1{}) {
					t.Fatalf("unconfirmed status %q reconstructed BOOKABLE: %+v", test.status, state)
				}
			})
		}
	})

	t.Run("malformed structural event never falls back to legacy facts", func(t *testing.T) {
		mutators := map[string]func(*Message){
			"payload normalized divergence": func(source *Message) {
				event := availabilityPromptAuthorityEventMap(
					t,
					source.NormalizedPayload[testAvailabilityPromptEventV1MessageKey],
				)
				asInterfaceSliceMaps(event["presented_options"])[0]["trip_id"] = "divergent-trip"
				source.NormalizedPayload[testAvailabilityPromptEventV1MessageKey] = event
			},
			"wrong source message id": func(source *Message) {
				for _, payload := range []map[string]interface{}{source.Payload, source.NormalizedPayload} {
					event := availabilityPromptAuthorityEventMap(
						t,
						payload[testAvailabilityPromptEventV1MessageKey],
					)
					event["source_message_id"] = "forged-source"
					payload[testAvailabilityPromptEventV1MessageKey] = event
				}
			},
			"decode failure": func(source *Message) {
				source.Payload[testAvailabilityPromptEventV1MessageKey] = "malformed"
				source.NormalizedPayload[testAvailabilityPromptEventV1MessageKey] = "malformed"
			},
		}
		for name, mutate := range mutators {
			t.Run(name, func(t *testing.T) {
				source := availabilityPromptAuthorityMessage(
					"malformed-structural-"+strings.ReplaceAll(name, " ", "-"),
					messageStatusAutomationSent,
					availability,
					[]int{0},
				)
				for _, payload := range []map[string]interface{}{source.Payload, source.NormalizedPayload} {
					payload["intent"] = string(IntentAvailabilitySearch)
					payload["template_name"] = string(TemplateAvailabilityList)
				}
				source.NormalizedPayload["delivery_recorded_at"] = now.UTC().Format(time.RFC3339Nano)
				mutate(&source)

				state := bootstrapAvailabilitySelectionStateFromHistoryForTest(
					[]Message{source, selectionInbound, selection},
				)
				if state.Status != AvailabilitySelectionStatusNone ||
					state.AvailabilityPromptSourceMessageID != "" ||
					state.SelectionEventMessageID != "" ||
					state.Snapshot != (AvailabilitySelectionSnapshotV1{}) {
					t.Fatalf("malformed event fell back to legacy authority: %+v", state)
				}
			})
		}
	})

	t.Run("bot delivery resolves structural draft with delivered identity", func(t *testing.T) {
		source := availabilityPromptAuthorityMessage(
			"legacy-structural-bot-draft",
			messageStatusAutomationSent,
			availability,
			[]int{0},
		)
		source.ReceivedAt = now
		source.CreatedAt = now
		source.Payload["mode"] = messageStatusAutomationDraft
		source.NormalizedPayload["mode"] = messageStatusAutomationDraft
		delivered := deliveredAvailabilityPromptProjectionForStateTest(
			t,
			source,
			"legacy-structural-bot-delivery",
			"BOT_AUTO_REPLY",
			"",
			"Texto entregue diferente do draft, com a mesma apresentação estrutural.",
			now.Add(time.Second),
		)
		botSelection := legacyAvailabilitySelectionProjectionWithPromptSourceForStateTest(selection, delivered.ID)
		state := bootstrapAvailabilitySelectionStateFromHistoryForTest(
			[]Message{source, delivered, selectionInbound, botSelection},
		)
		if state.Status != AvailabilitySelectionStatusBookable ||
			state.AvailabilityPromptSourceMessageID != delivered.ID ||
			state.SelectionProjectionMessageID != selection.ID ||
			state.SelectionEventMessageID != selectionInbound.ID {
			t.Fatalf("legacy replay did not reconcile BOT delivery identity and draft facts: %+v", state)
		}
	})

	t.Run("divergent bot delivery suppresses linked draft fallback", func(t *testing.T) {
		source := availabilityPromptAuthorityMessage(
			"legacy-divergent-bot-draft",
			messageStatusAutomationSent,
			availability,
			[]int{0},
		)
		source.ReceivedAt = now
		source.CreatedAt = now
		source.Payload["mode"] = messageStatusAutomationDraft
		source.NormalizedPayload["mode"] = messageStatusAutomationDraft
		delivered := deliveredAvailabilityPromptProjectionForStateTest(
			t,
			source,
			"legacy-divergent-bot-delivery",
			"BOT_AUTO_REPLY",
			"",
			source.Body,
			now.Add(time.Second),
		)
		divergentEvent := availabilityPromptAuthorityEventMap(
			t,
			delivered.Payload[testAvailabilityPromptEventV1MessageKey],
		)
		asInterfaceSliceMaps(divergentEvent["presented_options"])[0]["trip_id"] = "divergent-trip"
		delivered.Payload[testAvailabilityPromptEventV1MessageKey] =
			cloneMap(divergentEvent)
		delivered.NormalizedPayload[testAvailabilityPromptEventV1MessageKey] =
			cloneMap(divergentEvent)
		divergentSelection := legacyAvailabilitySelectionProjectionWithPromptSourceForStateTest(selection, delivered.ID)
		state := bootstrapAvailabilitySelectionStateFromHistoryForTest(
			[]Message{source, delivered, selectionInbound, divergentSelection},
		)
		if state.Status != AvailabilitySelectionStatusNone ||
			state.AvailabilityPromptSourceMessageID != "" ||
			state.SelectionEventMessageID != "" ||
			state.Snapshot != (AvailabilitySelectionSnapshotV1{}) {
			t.Fatalf("divergent delivery fell back to linked draft authority: %+v", state)
		}
	})

	t.Run("approved draft review resolves structural draft with delivered identity", func(t *testing.T) {
		source := availabilityPromptAuthorityMessage(
			"legacy-structural-reviewed-draft",
			messageStatusAutomationReviewed,
			availability,
			[]int{0},
		)
		source.ReceivedAt = now
		source.CreatedAt = now
		source.Payload["mode"] = messageStatusAutomationDraft
		source.Payload["review_action"] = "APPROVED_AS_IS"
		source.NormalizedPayload["mode"] = messageStatusAutomationDraft
		source.NormalizedPayload["review_action"] = "APPROVED_AS_IS"
		delivered := deliveredAvailabilityPromptProjectionForStateTest(
			t,
			source,
			"legacy-structural-reviewed-delivery",
			"DRAFT_REVIEW",
			"APPROVED_AS_IS",
			source.Body,
			now.Add(time.Second),
		)
		reviewedSelection := legacyAvailabilitySelectionProjectionWithPromptSourceForStateTest(selection, delivered.ID)
		state := bootstrapAvailabilitySelectionStateFromHistoryForTest(
			[]Message{source, delivered, selectionInbound, reviewedSelection},
		)
		if state.Status != AvailabilitySelectionStatusBookable ||
			state.AvailabilityPromptSourceMessageID != delivered.ID ||
			state.SelectionProjectionMessageID != selection.ID ||
			state.SelectionEventMessageID != selectionInbound.ID {
			t.Fatalf("legacy replay did not reconcile approved review identity and draft facts: %+v", state)
		}
	})

	t.Run("missing explicit selection identity stays none", func(t *testing.T) {
		identityMissing := legacyAvailabilitySelectionMessageForStateTest(
			"legacy-selection-missing-id",
			availability,
			IntentSelectAvailabilityOption,
			now.Add(3*time.Second),
		)
		delete(
			asMap(identityMissing.Payload[selectedAvailabilityResultPayloadKey]),
			selectedAvailabilitySelectionMessageIDPayloadKey,
		)
		delete(
			asMap(identityMissing.NormalizedPayload[selectedAvailabilityResultPayloadKey]),
			selectedAvailabilitySelectionMessageIDPayloadKey,
		)
		state := bootstrapAvailabilitySelectionStateFromHistoryForTest([]Message{promptA, identityMissing})
		if state.Status != AvailabilitySelectionStatusNone {
			t.Fatalf("legacy selection without explicit identity became authoritative: %+v", state)
		}
	})

	t.Run("nonexistent selection identity stays none", func(t *testing.T) {
		state := bootstrapAvailabilitySelectionStateFromHistoryForTest([]Message{promptA, selection})
		if state.Status != AvailabilitySelectionStatusNone {
			t.Fatalf("legacy selection with nonexistent inbound identity became authoritative: %+v", state)
		}
	})

	t.Run("outbound selection identity stays none", func(t *testing.T) {
		outboundIdentity := legacyAvailabilitySelectionMessageForStateTest(
			"legacy-selection-outbound-id",
			availability,
			IntentSelectAvailabilityOption,
			now.Add(3*time.Second),
			promptA.ID,
		)
		for _, payload := range []map[string]interface{}{outboundIdentity.Payload, outboundIdentity.NormalizedPayload} {
			asMap(payload[selectedAvailabilityResultPayloadKey])[selectedAvailabilitySelectionMessageIDPayloadKey] = promptA.ID
		}
		state := bootstrapAvailabilitySelectionStateFromHistoryForTest([]Message{promptA, outboundIdentity})
		if state.Status != AvailabilitySelectionStatusNone {
			t.Fatalf("legacy selection pointing at outbound message became authoritative: %+v", state)
		}
	})

	t.Run("selection identity after projection stays none", func(t *testing.T) {
		lateInbound := legacyAvailabilitySelectionInboundForStateTest(
			selection,
			now.Add(4*time.Second),
		)
		state := bootstrapAvailabilitySelectionStateFromHistoryForTest(
			[]Message{promptA, selection, lateInbound},
		)
		if state.Status != AvailabilitySelectionStatusNone {
			t.Fatalf("legacy selection accepted causally late inbound identity: %+v", state)
		}
	})

	t.Run("missing source stays none", func(t *testing.T) {
		state := bootstrapAvailabilitySelectionStateFromHistoryForTest(
			[]Message{selectionInbound, selection},
		)
		if state.Status != AvailabilitySelectionStatusNone {
			t.Fatalf("legacy selection without source became authoritative: %+v", state)
		}
	})

	t.Run("prompt source after selection stays none", func(t *testing.T) {
		latePrompt := legacyAvailabilityPromptMessageForStateTest(
			"legacy-prompt-after-selection",
			availability,
			now.Add(2500*time.Millisecond),
		)
		lateSourceSelection := legacyAvailabilitySelectionProjectionWithPromptSourceForStateTest(selection, latePrompt.ID)
		state := bootstrapAvailabilitySelectionStateFromHistoryForTest(
			[]Message{selectionInbound, latePrompt, lateSourceSelection},
		)
		if state.Status != AvailabilitySelectionStatusNone {
			t.Fatalf("legacy selection accepted a prompt emitted after the selection inbound: %+v", state)
		}
	})

	t.Run("additional compatible source does not override explicit source", func(t *testing.T) {
		state := bootstrapAvailabilitySelectionStateFromHistoryForTest(
			[]Message{promptA, promptB, selectionInbound, selection},
		)
		if state.Status != AvailabilitySelectionStatusBookable || state.AvailabilityPromptSourceMessageID != promptA.ID {
			t.Fatalf("explicit prompt source did not remain authoritative: %+v", state)
		}
	})

	t.Run("generic outbound with copied facts is not a prompt source", func(t *testing.T) {
		genericPrompt := legacyAvailabilityPromptMessageForStateTest(
			"legacy-generic-outbound",
			availability,
			now,
		)
		for _, payload := range []map[string]interface{}{genericPrompt.Payload, genericPrompt.NormalizedPayload} {
			delete(payload, "intent")
			delete(payload, "template_name")
		}
		genericSourceSelection := legacyAvailabilitySelectionProjectionWithPromptSourceForStateTest(selection, genericPrompt.ID)
		state := bootstrapAvailabilitySelectionStateFromHistoryForTest(
			[]Message{genericPrompt, selectionInbound, genericSourceSelection},
		)
		if state.Status != AvailabilitySelectionStatusNone {
			t.Fatalf("generic outbound carrying availability facts became prompt authority: %+v", state)
		}
	})

	t.Run("inbound structural event followed by projection stays none", func(t *testing.T) {
		inboundPrompt := availabilityPromptAuthorityMessage(
			"inbound-structural-prompt",
			"PROCESSED",
			availability,
			[]int{0},
		)
		inboundPrompt.Direction = "INBOUND"
		inboundSourceSelection := legacyAvailabilitySelectionProjectionWithPromptSourceForStateTest(selection, inboundPrompt.ID)
		state := bootstrapAvailabilitySelectionStateFromHistoryForTest(
			[]Message{inboundPrompt, selectionInbound, inboundSourceSelection},
		)
		if state.Status != AvailabilitySelectionStatusNone ||
			state.AvailabilityPromptSourceMessageID != "" ||
			state.SelectionEventMessageID != "" ||
			state.Snapshot != (AvailabilitySelectionSnapshotV1{}) {
			t.Fatalf("inbound structural prompt became BOOKABLE through later projection: %+v", state)
		}
		if _, _, ok := resolveBookingCreateSelectionFromState(
			"quero reservar opção 1",
			state,
		); ok {
			t.Fatal("booking_create opened from inbound structural prompt")
		}
		session := sessionWithAvailabilitySelectionStateForTest(
			Session{
				CustomerName:  "Cliente Teste",
				CustomerPhone: "5500000000000",
			},
			state,
		)
		if input, ok := parseBookingCreateInputWithPassengerState(
			session,
			[]Message{inboundPrompt, selectionInbound, inboundSourceSelection},
			"quero reservar opção 1",
			&availability,
			completePassengerStateForTest(1, 0),
		); ok ||
			input.SelectedOptionIndex != 0 ||
			input.TripID != "" ||
			input.BoardStopID != "" ||
			input.AlightStopID != "" ||
			len(input.Passengers) != 0 {
			t.Fatalf("inbound structural prompt produced BookingCreateInput: ok=%t input=%+v", ok, input)
		}
	})

	t.Run("selection projection is not a prompt source", func(t *testing.T) {
		projectionSource := legacyAvailabilitySelectionMessageForStateTest(
			"legacy-passenger-projection-source",
			availability,
			IntentPassengerCountReply,
			now.Add(time.Second),
		)
		projectionSourceSelection := legacyAvailabilitySelectionProjectionWithPromptSourceForStateTest(selection, projectionSource.ID)
		state := bootstrapAvailabilitySelectionStateFromHistoryForTest(
			[]Message{projectionSource, selectionInbound, projectionSourceSelection},
		)
		if state.Status != AvailabilitySelectionStatusNone {
			t.Fatalf("legacy selection projection became a prompt source: %+v", state)
		}
	})

	for _, test := range []struct {
		name   string
		intent Intent
	}{
		{name: "passenger reply", intent: IntentPassengerCountReply},
		{name: "passenger documents", intent: IntentPassengerDocumentsProvided},
		{name: "booking continuation", intent: IntentDocumentConfirmation},
	} {
		t.Run(test.name+" projection never materializes", func(t *testing.T) {
			projection := legacyAvailabilitySelectionMessageForStateTest(
				"legacy-"+strings.ReplaceAll(test.name, " ", "-")+"-projection",
				availability,
				test.intent,
				now.Add(3*time.Second),
				promptA.ID,
			)
			projectionInbound := legacyAvailabilitySelectionInboundForStateTest(
				projection,
				now.Add(2*time.Second),
			)
			state := bootstrapAvailabilitySelectionStateFromHistoryForTest(
				[]Message{promptA, projectionInbound, projection},
			)
			if state.Status != AvailabilitySelectionStatusNone {
				t.Fatalf("%s projection became authoritative: %+v", test.intent, state)
			}
			if _, _, ok := resolveBookingCreateSelectionFromState(
				"quero reservar opção 1",
				state,
			); ok {
				t.Fatal("booking_create opened without BOOKABLE authority")
			}
		})
	}

	t.Run("legacy rejection uses selection inbound causal order", func(t *testing.T) {
		rejection := availabilityAuthorityRejectionMessageForTest(
			"legacy-rejection-before-projection",
			promptA.ID,
			1,
			now.Add(2500*time.Millisecond),
		)
		state := bootstrapAvailabilitySelectionStateFromHistoryForTest(
			[]Message{promptA, selectionInbound, rejection, selection},
		)
		if state.Status != AvailabilitySelectionStatusRejected ||
			state.Tombstone == nil ||
			state.Tombstone.AvailabilityPromptSourceMessageID != promptA.ID {
			t.Fatalf("legacy projection order reopened a rejected selection: %+v", state)
		}
	})

	t.Run("read consumers require persisted reconstructed state", func(t *testing.T) {
		history := []Message{promptA, selectionInbound, selection}
		replayed := bootstrapAvailabilitySelectionStateFromHistoryForTest(history)
		if replayed.Status != AvailabilitySelectionStatusBookable {
			t.Fatalf("test replay did not reconstruct BOOKABLE authority: %+v", replayed)
		}
		if read := availabilitySelectionStateV1ForRead(Session{}, history); read.Status != AvailabilitySelectionStatusNone {
			t.Fatalf("bounded history became read authority without persisted replay: %+v", read)
		}
		session := sessionWithAvailabilitySelectionStateForTest(Session{}, replayed)
		if read := availabilitySelectionStateV1ForRead(session, nil); read.Status != AvailabilitySelectionStatusBookable {
			t.Fatalf("persisted validated replay was not consumed: %+v", read)
		}
	})
}

func TestAvailabilitySelectionStateV1CausalOrderBeatsLockOrder(t *testing.T) {
	t.Run("S2 applied before S1 remains S2", func(t *testing.T) {
		store := newFakeStore()
		now := availabilityTestObservedAt()
		session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
			Channel:       "WHATSAPP",
			ContactKey:    "causal-selection-order",
			CustomerPhone: "causal-selection-order",
		})
		if err != nil {
			t.Fatalf("seed causal selection session: %v", err)
		}
		messageS1 := createAvailabilitySelectionInboundForTest(
			t,
			store,
			session.ID,
			"causal-selection-s1",
			now,
		)
		messageS2 := createAvailabilitySelectionInboundForTest(
			t,
			store,
			session.ID,
			"causal-selection-s2",
			now.Add(time.Minute),
		)
		availabilityS1 := availabilityOptionPromptFutureResultAt(now)
		availabilityS2 := availabilityOptionPromptFutureResultAt(now.Add(24 * time.Hour))
		availabilityS2.Results[0].TripID = "causal-trip-s2"
		availabilityS2.Results[0].BoardStopID = "causal-board-s2"
		availabilityS2.Results[0].AlightStopID = "causal-alight-s2"
		snapshotS1 := mustAvailabilitySelectionSnapshotV1ForTest(t, &availabilityS1, 1)
		snapshotS2 := mustAvailabilitySelectionSnapshotV1ForTest(t, &availabilityS2, 1)

		if _, err := store.ApplyPassengerClarificationEventsV1(
			context.Background(),
			ApplyPassengerClarificationEventsV1Input{
				SessionID: session.ID,
				AvailabilitySelectionEvents: []AvailabilitySelectionEventV1{
					materializedAvailabilitySelectionEventForTest(
						messageS2.ID,
						"causal-projection-s2",
						"causal-prompt-s2",
						snapshotS2,
					),
				},
			},
		); err != nil {
			t.Fatalf("apply S2 first: %v", err)
		}
		applied, err := store.ApplyPassengerClarificationEventsV1(
			context.Background(),
			ApplyPassengerClarificationEventsV1Input{
				SessionID: session.ID,
				AvailabilitySelectionEvents: []AvailabilitySelectionEventV1{
					materializedAvailabilitySelectionEventForTest(
						messageS1.ID,
						"causal-projection-s1",
						"causal-prompt-s1",
						snapshotS1,
					),
				},
			},
		)
		if err != nil {
			t.Fatalf("apply S1 after S2: %v", err)
		}
		state := applied.AvailabilitySelectionState
		if state.Status != AvailabilitySelectionStatusBookable ||
			state.Snapshot.TripID != snapshotS2.TripID ||
			state.SelectionEventMessageID != messageS2.ID {
			t.Fatalf("lock order replaced newer S2 with older S1: %+v", state)
		}

		session = store.sessions[session.ID]
		metadata := cloneMap(session.Metadata)
		memory := cloneMap(asMap(metadata["memory"]))
		delete(memory, availabilitySelectionStateV1MemoryKey)
		metadata["memory"] = memory
		session.Metadata = metadata
		store.sessions[session.ID] = session
		restarted, err := store.ApplyPassengerClarificationEventsV1(
			context.Background(),
			ApplyPassengerClarificationEventsV1Input{SessionID: session.ID},
		)
		if err != nil {
			t.Fatalf("bootstrap causal state after restart: %v", err)
		}
		if restarted.AvailabilitySelectionState.Status != state.Status ||
			restarted.AvailabilitySelectionState.Snapshot != state.Snapshot ||
			restarted.AvailabilitySelectionState.SelectionEventMessageID != state.SelectionEventMessageID {
			t.Fatalf(
				"restart/bootstrap drifted from live causal state: live=%+v restarted=%+v",
				state,
				restarted.AvailabilitySelectionState,
			)
		}
		liveJSON, err := json.Marshal(state)
		if err != nil {
			t.Fatalf("marshal live causal state: %v", err)
		}
		restartedJSON, err := json.Marshal(restarted.AvailabilitySelectionState)
		if err != nil {
			t.Fatalf("marshal restarted causal state: %v", err)
		}
		if string(restartedJSON) != string(liveJSON) {
			t.Fatalf(
				"restart/bootstrap state differs byte-for-byte: live=%s restarted=%s",
				liveJSON,
				restartedJSON,
			)
		}
	})

	t.Run("new rejection applied before old materialization remains rejected", func(t *testing.T) {
		store := newFakeStore()
		now := availabilityTestObservedAt()
		session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
			Channel:       "WHATSAPP",
			ContactKey:    "causal-rejection-order",
			CustomerPhone: "causal-rejection-order",
		})
		if err != nil {
			t.Fatalf("seed causal rejection session: %v", err)
		}
		oldSelection := createAvailabilitySelectionInboundForTest(
			t,
			store,
			session.ID,
			"causal-old-selection",
			now,
		)
		newRejection := createAvailabilitySelectionInboundForTest(
			t,
			store,
			session.ID,
			"causal-new-rejection",
			now.Add(time.Minute),
		)
		availability := availabilityOptionPromptFutureResultAt(now)
		snapshot := mustAvailabilitySelectionSnapshotV1ForTest(t, &availability, 1)
		promptSource := "causal-shared-prompt"

		if _, err := store.ApplyPassengerClarificationEventsV1(
			context.Background(),
			ApplyPassengerClarificationEventsV1Input{
				SessionID: session.ID,
				AvailabilitySelectionEvents: []AvailabilitySelectionEventV1{{
					Type:                              AvailabilitySelectionEventRejected,
					MessageID:                         newRejection.ID,
					AvailabilityPromptSourceMessageID: promptSource,
					RejectedOptionIndexes:             []int{1},
				}},
			},
		); err != nil {
			t.Fatalf("apply new rejection first: %v", err)
		}
		applied, err := store.ApplyPassengerClarificationEventsV1(
			context.Background(),
			ApplyPassengerClarificationEventsV1Input{
				SessionID: session.ID,
				AvailabilitySelectionEvents: []AvailabilitySelectionEventV1{
					materializedAvailabilitySelectionEventForTest(
						oldSelection.ID,
						"causal-old-projection",
						promptSource,
						snapshot,
					),
				},
			},
		)
		if err != nil {
			t.Fatalf("apply old materialization after rejection: %v", err)
		}
		state := applied.AvailabilitySelectionState
		if state.Status != AvailabilitySelectionStatusRejected ||
			state.Tombstone == nil ||
			state.Tombstone.EventMessageID != newRejection.ID {
			t.Fatalf("old materialization reopened newer rejection: %+v", state)
		}
		if _, _, ok := resolveBookingCreateSelectionFromState(
			"quero reservar opção 1",
			state,
		); ok {
			t.Fatal("booking_create opened after causally newer rejection")
		}

		session = store.sessions[session.ID]
		metadata := cloneMap(session.Metadata)
		memory := cloneMap(asMap(metadata["memory"]))
		delete(memory, availabilitySelectionStateV1MemoryKey)
		metadata["memory"] = memory
		session.Metadata = metadata
		store.sessions[session.ID] = session
		restarted, err := store.ApplyPassengerClarificationEventsV1(
			context.Background(),
			ApplyPassengerClarificationEventsV1Input{SessionID: session.ID},
		)
		if err != nil {
			t.Fatalf("bootstrap causal rejection state after restart: %v", err)
		}
		liveJSON, err := json.Marshal(state)
		if err != nil {
			t.Fatalf("marshal live causal rejection state: %v", err)
		}
		restartedJSON, err := json.Marshal(restarted.AvailabilitySelectionState)
		if err != nil {
			t.Fatalf("marshal restarted causal rejection state: %v", err)
		}
		if string(restartedJSON) != string(liveJSON) {
			t.Fatalf(
				"rejection restart/bootstrap state differs byte-for-byte: live=%s restarted=%s",
				liveJSON,
				restartedJSON,
			)
		}
	})
}

func TestAvailabilitySelectionReplayOrderPermutationProperty(t *testing.T) {
	arrivalPermutations := availabilitySelectionArrivalPermutationsForTest(6)
	if len(arrivalPermutations) != 720 {
		t.Fatalf("expected complete 6-event arrival matrix with 720 permutations, got %d", len(arrivalPermutations))
	}
	replayObservedAt := availabilityTestObservedAt()
	var referenceStateJSON string

	for permutationIndex, arrivalOrder := range arrivalPermutations {
		store := newFakeStore()
		now := replayObservedAt
		session, err := store.UpsertSession(context.Background(), UpsertSessionInput{
			Channel:       "WHATSAPP",
			ContactKey:    "availability-replay-" + strconv.Itoa(permutationIndex),
			CustomerPhone: "availability-replay-" + strconv.Itoa(permutationIndex),
		})
		if err != nil {
			t.Fatalf("seed replay session for permutation %v: %v", arrivalOrder, err)
		}

		messageIDs := []string{
			"00000000-0000-0000-0000-000000000201",
			"00000000-0000-0000-0000-000000000202",
			"00000000-0000-0000-0000-000000000203",
			"00000000-0000-0000-0000-000000000204",
			"00000000-0000-0000-0000-000000000205",
			"00000000-0000-0000-0000-000000000206",
		}
		messages := make([]Message, len(messageIDs))
		for index, messageID := range messageIDs {
			messages[index] = seedAvailabilitySelectionReplayMessageForTest(
				store,
				session.ID,
				messageID,
				"replay-event-"+strconv.Itoa(index),
				now.Add(time.Duration(index)*time.Minute),
			)
		}

		availabilityA := availabilityOptionPromptFutureResultAt(now)
		availabilityB := availabilityOptionPromptFutureResultAt(now.Add(24 * time.Hour))
		availabilityB.Results[0].TripID = "replay-trip-b"
		availabilityB.Results[0].BoardStopID = "replay-board-b"
		availabilityB.Results[0].AlightStopID = "replay-alight-b"
		snapshotA := mustAvailabilitySelectionSnapshotV1ForTest(t, &availabilityA, 1)
		snapshotB := mustAvailabilitySelectionSnapshotV1ForTest(t, &availabilityB, 1)
		materializeB := materializedAvailabilitySelectionEventForTest(
			messages[3].ID,
			"replay-selection-projection-b",
			"replay-prompt-b",
			snapshotB,
		)

		inputs := []ApplyPassengerClarificationEventsV1Input{
			{
				SessionID: session.ID,
				AvailabilitySelectionEvents: []AvailabilitySelectionEventV1{
					materializedAvailabilitySelectionEventForTest(
						messages[0].ID,
						"replay-selection-projection-a",
						"replay-prompt-a",
						snapshotA,
					),
				},
			},
			{
				SessionID: session.ID,
				AvailabilitySelectionEvents: []AvailabilitySelectionEventV1{{
					Type:                              AvailabilitySelectionEventRejected,
					MessageID:                         messages[1].ID,
					AvailabilityPromptSourceMessageID: "replay-prompt-a",
					RejectedOptionIndexes:             []int{1},
				}},
			},
			{
				SessionID: session.ID,
				AvailabilitySelectionEvents: []AvailabilitySelectionEventV1{{
					Type:                 AvailabilitySelectionEventInvalidated,
					MessageID:            messages[2].ID,
					RejectedWholeContext: true,
					ReasonCode:           "REPLAY_PROPERTY_INVALIDATION",
				}},
			},
			{
				SessionID: session.ID,
				AvailabilitySelectionEvents: []AvailabilitySelectionEventV1{
					materializeB,
					materializeB,
				},
			},
			{
				SessionID: session.ID,
				AvailabilitySelectionEvents: []AvailabilitySelectionEventV1{{
					Type:                              AvailabilitySelectionEventRejected,
					MessageID:                         messages[4].ID,
					AvailabilityPromptSourceMessageID: "replay-prompt-b",
					RejectedOptionIndexes:             []int{1},
				}},
			},
			{
				SessionID: session.ID,
				AvailabilitySelectionEvents: []AvailabilitySelectionEventV1{{
					Type:                              AvailabilitySelectionEventMaterialized,
					MessageID:                         messages[5].ID,
					ProjectionMessageID:               "replay-copied-projection-b",
					AvailabilityPromptSourceMessageID: "replay-prompt-b",
					SelectedOptionIndex:               snapshotB.SelectedOptionIndex,
					Snapshot:                          snapshotB,
					MaterializesAuthority:             false,
				}},
			},
		}

		var liveState AvailabilitySelectionStateV1
		for _, inputIndex := range arrivalOrder {
			applied, applyErr := store.ApplyPassengerClarificationEventsV1(
				context.Background(),
				inputs[inputIndex],
			)
			if applyErr != nil {
				t.Fatalf("apply permutation %v input %d: %v", arrivalOrder, inputIndex, applyErr)
			}
			liveState = applied.AvailabilitySelectionState
		}

		if liveState.Status != AvailabilitySelectionStatusRejected ||
			liveState.Tombstone == nil ||
			liveState.Tombstone.EventMessageID != messages[4].ID ||
			liveState.SelectionEventMessageID != "" ||
			liveState.Snapshot != (AvailabilitySelectionSnapshotV1{}) ||
			len(liveState.AppliedEventIDs) != 6 {
			t.Fatalf("permutation %v produced wrong canonical state: %+v", arrivalOrder, liveState)
		}
		liveJSON := availabilitySelectionStateJSONForTest(t, liveState)
		if referenceStateJSON == "" {
			referenceStateJSON = liveJSON
		} else if liveJSON != referenceStateJSON {
			t.Fatalf(
				"arrival permutation %v changed canonical replay: reference=%s current=%s",
				arrivalOrder,
				referenceStateJSON,
				liveJSON,
			)
		}

		reloaded, err := store.GetSession(context.Background(), session.ID)
		if err != nil {
			t.Fatalf("reload permutation %v: %v", arrivalOrder, err)
		}
		reloadedState, ok := availabilitySelectionStateV1FromSession(reloaded)
		if !ok || availabilitySelectionStateJSONForTest(t, reloadedState) != liveJSON {
			t.Fatalf("reload differs for permutation %v: live=%+v reload=%+v", arrivalOrder, liveState, reloadedState)
		}

		reloaded.Metadata = cloneMap(reloaded.Metadata)
		memory := cloneMap(asMap(reloaded.Metadata["memory"]))
		delete(memory, availabilitySelectionStateV1MemoryKey)
		reloaded.Metadata["memory"] = memory
		store.sessions[reloaded.ID] = reloaded
		restarted, err := store.ApplyPassengerClarificationEventsV1(
			context.Background(),
			ApplyPassengerClarificationEventsV1Input{SessionID: session.ID},
		)
		if err != nil {
			t.Fatalf("restart permutation %v: %v", arrivalOrder, err)
		}
		if restartedJSON := availabilitySelectionStateJSONForTest(
			t,
			restarted.AvailabilitySelectionState,
		); restartedJSON != liveJSON {
			t.Fatalf("restart differs for permutation %v: live=%s restart=%s", arrivalOrder, liveJSON, restartedJSON)
		}
	}
}

func availabilitySelectionArrivalPermutationsForTest(size int) [][]int {
	current := make([]int, size)
	for index := range current {
		current[index] = index
	}
	permutations := make([][]int, 0)
	var visit func(int)
	visit = func(position int) {
		if position == len(current) {
			permutations = append(permutations, append([]int(nil), current...))
			return
		}
		for index := position; index < len(current); index++ {
			current[position], current[index] = current[index], current[position]
			visit(position + 1)
			current[position], current[index] = current[index], current[position]
		}
	}
	visit(0)
	return permutations
}

func availabilitySelectionStateJSONForTest(
	t *testing.T,
	state AvailabilitySelectionStateV1,
) string {
	t.Helper()
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal availability selection state: %v", err)
	}
	return string(raw)
}

func seedAvailabilitySelectionReplayMessageForTest(
	store *fakeStore,
	sessionID string,
	messageID string,
	body string,
	recordedAt time.Time,
) Message {
	message := Message{
		ID:               messageID,
		SessionID:        sessionID,
		Direction:        "INBOUND",
		Body:             body,
		ProcessingStatus: "BUFFERED_PENDING",
		ReceivedAt:       recordedAt,
		CreatedAt:        recordedAt,
	}
	store.messages[message.ID] = message
	store.messageOrder = append(store.messageOrder, message.ID)
	return message
}

func configForAvailabilitySelectionStateTest() config.Config {
	return config.Config{ChatDebounceWindowMS: 1500}
}

type droppingAvailabilitySelectionEventStore struct {
	*fakeStore
}

func (store *droppingAvailabilitySelectionEventStore) ApplyPassengerClarificationEventsV1(
	ctx context.Context,
	input ApplyPassengerClarificationEventsV1Input,
) (ApplyPassengerClarificationEventsV1Result, error) {
	input.AvailabilitySelectionEvents = nil
	return store.fakeStore.ApplyPassengerClarificationEventsV1(ctx, input)
}

type serializedAvailabilitySelectionFakeStore struct {
	*fakeStore
	mu sync.Mutex
}

func (store *serializedAvailabilitySelectionFakeStore) ApplyPassengerClarificationEventsV1(
	ctx context.Context,
	input ApplyPassengerClarificationEventsV1Input,
) (ApplyPassengerClarificationEventsV1Result, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.fakeStore.ApplyPassengerClarificationEventsV1(ctx, input)
}

func (store *serializedAvailabilitySelectionFakeStore) GetSession(
	ctx context.Context,
	sessionID string,
) (Session, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.fakeStore.GetSession(ctx, sessionID)
}

func mustAvailabilitySelectionSnapshotV1ForTest(
	t *testing.T,
	availability *AvailabilitySearchResult,
	index int,
) AvailabilitySelectionSnapshotV1 {
	t.Helper()
	snapshot, _, ok := availabilitySelectionSnapshotV1FromAvailabilityWithPresence(availability, index)
	if !ok {
		t.Fatalf("build availability selection snapshot index %d: %+v", index, availability)
	}
	return snapshot
}

func materializedAvailabilitySelectionEventForTest(
	messageID string,
	projectionMessageID string,
	promptSourceMessageID string,
	snapshot AvailabilitySelectionSnapshotV1,
) AvailabilitySelectionEventV1 {
	recordedAt := availabilityTestObservedAt()
	return AvailabilitySelectionEventV1{
		Type:                              AvailabilitySelectionEventMaterialized,
		MessageID:                         messageID,
		ProjectionMessageID:               projectionMessageID,
		AvailabilityPromptSourceMessageID: promptSourceMessageID,
		SelectedOptionIndex:               snapshot.SelectedOptionIndex,
		Snapshot:                          snapshot,
		MaterializesAuthority:             true,
		Order: AvailabilitySelectionEventOrderV1{
			ReceivedAt: recordedAt,
			CreatedAt:  recordedAt,
			MessageID:  messageID,
		},
	}
}

func orderedAvailabilitySelectionEventsForTest(
	events ...AvailabilitySelectionEventV1,
) []AvailabilitySelectionEventV1 {
	return orderedAvailabilitySelectionEventsForTestFrom(0, events...)
}

func orderedAvailabilitySelectionEventsForTestFrom(
	start int,
	events ...AvailabilitySelectionEventV1,
) []AvailabilitySelectionEventV1 {
	base := availabilityTestObservedAt().Add(-24 * time.Hour)
	ordered := append([]AvailabilitySelectionEventV1(nil), events...)
	for index := range ordered {
		recordedAt := base.Add(time.Duration(start+index) * time.Minute)
		ordered[index].Order = AvailabilitySelectionEventOrderV1{
			ReceivedAt: recordedAt,
			CreatedAt:  recordedAt,
			MessageID:  strings.TrimSpace(ordered[index].MessageID),
		}
	}
	return ordered
}

func createAvailabilitySelectionInboundForTest(
	t *testing.T,
	store *fakeStore,
	sessionID string,
	body string,
	at time.Time,
) Message {
	t.Helper()
	message, err := store.CreateMessage(context.Background(), CreateMessageInput{
		SessionID:        sessionID,
		Direction:        "INBOUND",
		Kind:             "TEXT",
		IdempotencyKey:   "availability-selection-" + body,
		Body:             body,
		ProcessingStatus: "BUFFERED_PENDING",
		ReceivedAt:       at,
	})
	if err != nil {
		t.Fatalf("create availability selection inbound: %v", err)
	}
	return message
}

func persistAvailabilitySelectionEventOnFakeMessageForTest(
	t *testing.T,
	store *fakeStore,
	messageID string,
	event AvailabilitySelectionEventV1,
) {
	t.Helper()
	message, ok := store.messages[messageID]
	if !ok {
		t.Fatalf("availability event message %q not found", messageID)
	}
	message.NormalizedPayload = cloneMap(message.NormalizedPayload)
	message.NormalizedPayload[availabilitySelectionEventsV1MessageKey] = []AvailabilitySelectionEventV1{event}
	store.messages[messageID] = message
}

func legacyAvailabilityPromptMessageForStateTest(
	messageID string,
	availability AvailabilitySearchResult,
	at time.Time,
) Message {
	payload := map[string]interface{}{
		"intent":        string(IntentAvailabilitySearch),
		"template_name": string(TemplateAvailabilityList),
		"tool_context": map[string]interface{}{
			toolNameAvailabilitySearch: buildAvailabilityToolResponsePayload(availability),
		},
	}
	normalizedPayload := cloneMap(payload)
	normalizedPayload["delivery_recorded_at"] = at.UTC().Format(time.RFC3339Nano)
	return Message{
		ID:                messageID,
		Direction:         "OUTBOUND",
		ProcessingStatus:  messageStatusAutomationSent,
		Payload:           payload,
		NormalizedPayload: normalizedPayload,
		ReceivedAt:        at,
		CreatedAt:         at,
	}
}

func legacyAvailabilitySelectionInboundForStateTest(
	projection Message,
	at time.Time,
) Message {
	selectionMessageID := strings.TrimSpace(asString(
		asMap(projection.Payload[selectedAvailabilityResultPayloadKey])[selectedAvailabilitySelectionMessageIDPayloadKey],
	))
	return Message{
		ID:               selectionMessageID,
		Direction:        "INBOUND",
		ProcessingStatus: "PROCESSED",
		ReceivedAt:       at,
		CreatedAt:        at,
	}
}

func legacyAvailabilitySelectionProjectionWithPromptSourceForStateTest(
	projection Message,
	promptSourceMessageID string,
) Message {
	projection.Payload = cloneMap(projection.Payload)
	projection.NormalizedPayload = cloneMap(projection.NormalizedPayload)
	for _, payload := range []map[string]interface{}{projection.Payload, projection.NormalizedPayload} {
		snapshot := cloneMap(asMap(payload[selectedAvailabilityResultPayloadKey]))
		snapshot[availabilityPromptSourceMessageIDPayloadKey] = strings.TrimSpace(promptSourceMessageID)
		payload[selectedAvailabilityResultPayloadKey] = snapshot
	}
	return projection
}

func legacyAvailabilitySelectionMessageForStateTest(
	messageID string,
	availability AvailabilitySearchResult,
	intent Intent,
	at time.Time,
	promptSourceMessageID ...string,
) Message {
	payload := persistedAvailabilitySelectionPayloadForTest(
		availability,
		1,
		availabilitySelectionProjectionAuthorityForTest{},
	)
	payload["intent"] = string(intent)
	snapshot := asMap(payload[selectedAvailabilityResultPayloadKey])
	snapshot[selectedAvailabilitySelectionMessageIDPayloadKey] = "legacy-selection-event-" + messageID
	if len(promptSourceMessageID) == 1 && strings.TrimSpace(promptSourceMessageID[0]) != "" {
		snapshot[availabilityPromptSourceMessageIDPayloadKey] = strings.TrimSpace(promptSourceMessageID[0])
	} else {
		delete(snapshot, availabilityPromptSourceMessageIDPayloadKey)
	}
	delete(snapshot, availabilitySelectionMaterializesAuthorityPayloadKey)
	normalizedPayload := cloneMap(payload)
	normalizedPayload["delivery_recorded_at"] = at.UTC().Format(time.RFC3339Nano)
	return Message{
		ID:                messageID,
		Direction:         "OUTBOUND",
		ProcessingStatus:  messageStatusAutomationSent,
		Payload:           payload,
		NormalizedPayload: normalizedPayload,
		ReceivedAt:        at,
		CreatedAt:         at,
	}
}

func deliveredAvailabilityPromptProjectionForStateTest(
	t *testing.T,
	source Message,
	messageID string,
	mode string,
	reviewAction string,
	body string,
	at time.Time,
) Message {
	t.Helper()
	event, ok := availabilityPromptEventForOutboundV1(source, messageID, true)
	if !ok {
		t.Fatalf("project availability prompt event from source %q", source.ID)
	}
	payload := map[string]interface{}{
		"mode":                                  mode,
		"draft_message_id":                      source.ID,
		testAvailabilityPromptEventV1MessageKey: event,
	}
	if reviewAction != "" {
		payload["review_action"] = reviewAction
	}
	normalizedPayload := cloneMap(payload)
	normalizedPayload["delivery_recorded_at"] = at.UTC().Format(time.RFC3339Nano)
	return Message{
		ID:                messageID,
		Direction:         "OUTBOUND",
		Body:              body,
		Payload:           cloneMap(payload),
		NormalizedPayload: normalizedPayload,
		ProcessingStatus:  messageStatusAutomationSent,
		ReceivedAt:        at,
		CreatedAt:         at,
	}
}
