package chat

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestValidateTravelQueryMeaningV2AcceptsGroundedAvailabilitySearch(t *testing.T) {
	input := validTravelQueryV2AvailabilityInput()

	got := ValidateTravelQueryMeaningV2(input)

	assertTravelQueryV2Accepted(t, got)
}

func TestValidateTravelQueryMeaningV2RejectsZeroValueAndUnknownIntent(t *testing.T) {
	for _, proposal := range []TravelQueryMeaningV2{
		{},
		{
			Intent:             TravelQueryIntentUnknown,
			TurnMeaning:        TurnMeaningNewRequest,
			DatePreference:     DatePreference{Mode: DateModeUnspecified},
			OptionReference:    OptionReference{Kind: OptionReferenceNone},
			RouteCoverage:      RouteCoverageMeaning{Mode: RouteCoverageModeUnspecified},
			SeatRequest:        SeatRequestNone,
			InstitutionalTopic: InstitutionalTopicNone,
			Confidence:         0.95,
		},
	} {
		got := ValidateTravelQueryMeaningV2(TravelQueryValidationInputV2{
			Proposal:                 proposal,
			CurrentTurn:              "opaque non-empty content",
			State:                    CanonicalConversationState{Phase: ConversationPhaseDiscovery},
			ExistingDecisionStrength: DecisionStrengthFallback,
		})

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonUnknownIntent)
	}
}

func TestValidateTravelQueryMeaningV2RejectsEmptyCurrentTurn(t *testing.T) {
	input := validTravelQueryV2AvailabilityInput()
	input.CurrentTurn = "   "

	got := ValidateTravelQueryMeaningV2(input)

	assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonEmptyCurrentTurn)
}

func TestValidateTravelQueryMeaningV2CurrentTurnContentDoesNotChangeStructuredValidation(t *testing.T) {
	inputs := []TravelQueryValidationInputV2{
		validTravelQueryV2AvailabilityInput(),
		travelQueryV2SelectionInput(2),
	}
	inputs[1].Proposal.OptionReference = OptionReference{Kind: OptionReferenceIndex, Index: 3}

	for index, input := range inputs {
		input.CurrentTurn = "opaque content alpha"
		first := ValidateTravelQueryMeaningV2(input)
		input.CurrentTurn = "opaque content beta"
		second := ValidateTravelQueryMeaningV2(input)

		if !reflect.DeepEqual(first, second) {
			t.Fatalf("case %d changed validation from current-turn content: first=%+v second=%+v", index, first, second)
		}
	}
}

func TestValidateTravelQueryMeaningV2ValidatesConfidence(t *testing.T) {
	for _, confidence := range []float64{-0.01, 1.01, math.NaN(), math.Inf(1)} {
		t.Run(fmt.Sprintf("invalid_%v", confidence), func(t *testing.T) {
			input := validTravelQueryV2AvailabilityInput()
			input.Proposal.Confidence = confidence

			got := ValidateTravelQueryMeaningV2(input)

			assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonInvalidConfidence)
		})
	}

	t.Run("low confidence requires clarification", func(t *testing.T) {
		input := validTravelQueryV2AvailabilityInput()
		input.Proposal.Confidence = 0.42

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonLowConfidenceRequiresClarification)
	})

	t.Run("low confidence clarification is semantically valid", func(t *testing.T) {
		input := validTravelQueryV2AvailabilityInput()
		input.Proposal.Destination = nil
		input.Proposal.Confidence = 0.42
		input.Proposal.NeedsClarification = true
		input.Proposal.MissingFields = []string{"destination"}

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Accepted(t, got)
	})
}

func TestValidateTravelQueryMeaningV2RejectsInvalidEnums(t *testing.T) {
	input := validTravelQueryV2AvailabilityInput()
	input.Proposal.SeatRequest = SeatRequestMode("RESERVE_NOW")

	got := ValidateTravelQueryMeaningV2(input)

	assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonInvalidEnumValue)
}

func TestValidateTravelQueryMeaningV2RejectsEmptyDiscriminatorEnums(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*TravelQueryValidationInputV2)
	}{
		{
			name: "date mode",
			mutate: func(input *TravelQueryValidationInputV2) {
				input.Proposal.DatePreference.Mode = ""
			},
		},
		{
			name: "option reference kind",
			mutate: func(input *TravelQueryValidationInputV2) {
				input.Proposal.OptionReference.Kind = ""
			},
		},
		{
			name: "route coverage mode",
			mutate: func(input *TravelQueryValidationInputV2) {
				input.Proposal.RouteCoverage.Mode = ""
			},
		},
		{
			name: "seat request mode",
			mutate: func(input *TravelQueryValidationInputV2) {
				input.Proposal.SeatRequest = ""
			},
		},
		{
			name: "institutional topic",
			mutate: func(input *TravelQueryValidationInputV2) {
				input.Proposal.InstitutionalTopic = ""
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validTravelQueryV2AvailabilityInput()
			test.mutate(&input)

			got := ValidateTravelQueryMeaningV2(input)

			assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonInvalidEnumValue)
		})
	}
}

func TestValidateTravelQueryMeaningV2KeepsSemanticValidationIndependentFromRuntimePhase(t *testing.T) {
	input := validTravelQueryV2AvailabilityInput()
	input.State.Phase = ConversationPhaseBooked

	got := ValidateTravelQueryMeaningV2(input)

	assertTravelQueryV2Accepted(t, got)
}

func TestLocationRoleValidationRejectsStructuralContradictions(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*TravelQueryValidationInputV2)
	}{
		{
			name: "origin field has destination role",
			mutate: func(input *TravelQueryValidationInputV2) {
				input.Proposal.Origin.Role = LocationRoleDestination
			},
		},
		{
			name: "same location is origin and destination",
			mutate: func(input *TravelQueryValidationInputV2) {
				input.Proposal.Destination = &LocationMeaning{Name: "Seara/SC", Role: LocationRoleDestination}
			},
		},
		{
			name: "destination field has origin role",
			mutate: func(input *TravelQueryValidationInputV2) {
				input.Proposal.Destination.Role = LocationRoleOrigin
			},
		},
		{
			name: "mentioned location has unknown role",
			mutate: func(input *TravelQueryValidationInputV2) {
				input.Proposal.MentionedLocations = []LocationMeaning{{Name: "Santa Cecilia/SC", Role: LocationRoleUnknown}}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validTravelQueryV2AvailabilityInput()
			test.mutate(&input)

			got := ValidateTravelQueryMeaningV2(input)

			assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonContradictoryLocationRole)
		})
	}
}

func TestLocationRoleValidationUsesStructuredProposalWithoutReadingCurrentTurn(t *testing.T) {
	input := validTravelQueryV2AvailabilityInput()
	input.CurrentTurn = "opaque non-empty content"

	got := ValidateTravelQueryMeaningV2(input)

	assertTravelQueryV2Accepted(t, got)
}

func TestLocationRoleValidationUsesStructuredCanonicalState(t *testing.T) {
	t.Run("accepts roles compatible with canonical state", func(t *testing.T) {
		input := validTravelQueryV2BusinessRouteInput()
		input.State.Route.Origin = "Fraiburgo/SC"
		input.State.Route.Destination = "Santa Inês/MA"

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Accepted(t, got)
	})

	t.Run("rejects roles contradicted by canonical state", func(t *testing.T) {
		input := validTravelQueryV2BusinessRouteInput()
		input.State.Route.Origin = "Fraiburgo/SC"
		input.State.Route.Destination = "Santa Inês/MA"
		input.Proposal.Origin = &LocationMeaning{Name: "Santa Inês/MA", Role: LocationRoleOrigin}
		input.Proposal.Destination = &LocationMeaning{Name: "Fraiburgo/SC", Role: LocationRoleDestination}

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonLocationNotGrounded)
	})

	t.Run("current turn does not replace canonical state", func(t *testing.T) {
		input := validTravelQueryV2BusinessRouteInput()
		input.CurrentTurn = "opaque contradictory-looking content"
		input.State.Route.Origin = "Fraiburgo/SC"
		input.State.Route.Destination = "Santa Inês/MA"

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Accepted(t, got)
	})
}

func TestValidateTravelQueryMeaningV2RejectsLocationOutsideCatalog(t *testing.T) {
	input := validTravelQueryV2AvailabilityInput()
	input.Proposal.Destination = &LocationMeaning{Name: "Atlantis/SC", Role: LocationRoleDestination}

	got := ValidateTravelQueryMeaningV2(input)

	assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonLocationNotInCatalog)
}

func TestValidateTravelQueryMeaningV2AcceptsUnambiguousCatalogLocationWithoutRouteFacts(t *testing.T) {
	input := validTravelQueryV2AvailabilityInput()
	input.Proposal.Destination = &LocationMeaning{Name: "Florianopolis/SC", Role: LocationRoleDestination}
	input.LocationCatalog = append(input.LocationCatalog, TravelQueryLocationEvidenceV2{
		CanonicalName: "Florianopolis/SC",
		StopID:        "SC_FLORIANOPOLIS",
	})

	got := ValidateTravelQueryMeaningV2(input)

	assertTravelQueryV2Accepted(t, got)
}

func TestValidateTravelQueryMeaningV2RejectsLocationContradictedByStructuredState(t *testing.T) {
	input := validTravelQueryV2AvailabilityInput()
	input.State.Route.Origin = "Seara/MA"
	input.State.Route.Destination = "Videira/SC"
	input.LocationCatalog = append(input.LocationCatalog, TravelQueryLocationEvidenceV2{
		CanonicalName: "Seara/MA",
		StopID:        "MA_SEARA",
	})

	got := ValidateTravelQueryMeaningV2(input)

	assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonLocationNotGrounded)
}

func TestValidateTravelQueryMeaningV2GroundsRouteEndpointsByProposedRole(t *testing.T) {
	t.Run("canonical state", func(t *testing.T) {
		input := validTravelQueryV2AvailabilityInput()
		input.State.Route.Origin = "Seara/SC"
		input.State.Route.Destination = "Videira/SC"
		input.Proposal.Origin = &LocationMeaning{Name: "Videira/SC", Role: LocationRoleOrigin}
		input.Proposal.Destination = &LocationMeaning{Name: "Seara/SC", Role: LocationRoleDestination}

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonLocationNotGrounded)
	})

	t.Run("current visible facts", func(t *testing.T) {
		input := travelQueryV2SelectionInput(1)
		input.Proposal.Origin = &LocationMeaning{Name: "Videira/SC", Role: LocationRoleOrigin}
		input.Proposal.Destination = &LocationMeaning{Name: "Seara/SC", Role: LocationRoleDestination}

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonLocationNotGrounded)
	})
}

func TestValidateTravelQueryMeaningV2PrefersCurrentFactsOverStaleRouteState(t *testing.T) {
	staleOrigin := TravelQueryLocationEvidenceV2{CanonicalName: "Fraiburgo/SC", StopID: "SC_FRAIBURGO"}
	staleDestination := TravelQueryLocationEvidenceV2{CanonicalName: "Santa Inês/MA", StopID: "MA_SANTA_INES"}

	t.Run("accepts endpoints from current facts despite stale state", func(t *testing.T) {
		input := travelQueryV2SelectionInput(1)
		input.State.Route.Origin = staleOrigin.CanonicalName
		input.State.Route.Destination = staleDestination.CanonicalName
		input.Proposal.Origin = &LocationMeaning{Name: "Seara/SC", Role: LocationRoleOrigin}
		input.Proposal.Destination = &LocationMeaning{Name: "Videira/SC", Role: LocationRoleDestination}
		input.LocationCatalog = append(input.LocationCatalog, staleOrigin, staleDestination)

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Accepted(t, got)
	})

	t.Run("rejects endpoints grounded only by stale state", func(t *testing.T) {
		input := travelQueryV2SelectionInput(1)
		input.State.Route.Origin = staleOrigin.CanonicalName
		input.State.Route.Destination = staleDestination.CanonicalName
		input.Proposal.Origin = &LocationMeaning{Name: staleOrigin.CanonicalName, Role: LocationRoleOrigin}
		input.Proposal.Destination = &LocationMeaning{Name: staleDestination.CanonicalName, Role: LocationRoleDestination}
		input.LocationCatalog = append(input.LocationCatalog, staleOrigin, staleDestination)

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonLocationNotGrounded)
	})
}

func TestValidateTravelQueryMeaningV2RejectsApplicableInvalidAvailabilityFactsWithoutStateFallback(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*TravelQueryValidationInputV2)
	}{
		{
			name: "unknown StopID with compatible state",
			mutate: func(input *TravelQueryValidationInputV2) {
				input.AvailabilityFacts.VisibleOptions[0].DestinationStopID = "SC_UNKNOWN"
			},
		},
		{
			name: "ambiguous StopID with compatible state",
			mutate: func(input *TravelQueryValidationInputV2) {
				input.LocationCatalog = append(input.LocationCatalog, TravelQueryLocationEvidenceV2{
					CanonicalName: "Videira/PR",
					StopID:        "SC_VIDEIRA",
				})
			},
		},
		{
			name: "divergent display with compatible state",
			mutate: func(input *TravelQueryValidationInputV2) {
				input.AvailabilityFacts.VisibleOptions[0].DestinationDisplayName = "Seara/SC"
			},
		},
		{
			name: "incomplete option with compatible state",
			mutate: func(input *TravelQueryValidationInputV2) {
				input.AvailabilityFacts.VisibleOptions[0].TripID = ""
			},
		},
		{
			name: "invalid date with compatible state",
			mutate: func(input *TravelQueryValidationInputV2) {
				input.AvailabilityFacts.VisibleOptions[0].TripDate = "2030-02-31"
			},
		},
		{
			name: "past date with compatible state",
			mutate: func(input *TravelQueryValidationInputV2) {
				input.AvailabilityFacts.VisibleOptions[0].TripDate = input.ObservedAt.AddDate(0, 0, -1).Format("2006-01-02")
			},
		},
		{
			name: "inconsistent option count with compatible state",
			mutate: func(input *TravelQueryValidationInputV2) {
				input.ActivePrompt.AvailabilityOptionCount++
			},
		},
		{
			name: "empty current option list with compatible state",
			mutate: func(input *TravelQueryValidationInputV2) {
				input.AvailabilityFacts.VisibleOptions = nil
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := travelQueryV2SelectionInput(1)
			input.State.Route.Origin = "Seara/SC"
			input.State.Route.Destination = "Videira/SC"
			input.Proposal.Origin = &LocationMeaning{Name: "Seara/SC", Role: LocationRoleOrigin}
			input.Proposal.Destination = &LocationMeaning{Name: "Videira/SC", Role: LocationRoleDestination}
			test.mutate(&input)

			got := ValidateTravelQueryMeaningV2(input)

			assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonCurrentAvailabilityFactsInvalid)
		})
	}

	t.Run("invalid current facts reject proposal without option reference", func(t *testing.T) {
		input := travelQueryV2SelectionInput(1)
		input.State.Route.Origin = "Seara/SC"
		input.State.Route.Destination = "Videira/SC"
		input.Proposal.Intent = TravelQueryIntentAvailabilitySearch
		input.Proposal.TurnMeaning = TurnMeaningNewRequest
		input.Proposal.Origin = &LocationMeaning{Name: "Seara/SC", Role: LocationRoleOrigin}
		input.Proposal.Destination = &LocationMeaning{Name: "Videira/SC", Role: LocationRoleDestination}
		input.Proposal.OptionReference = OptionReference{Kind: OptionReferenceNone}
		input.AvailabilityFacts.VisibleOptions[0].DestinationStopID = "SC_UNKNOWN"

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonCurrentAvailabilityFactsInvalid)
	})
}

func TestValidateTravelQueryMeaningV2ClassifiesInvalidFactsBeforeRouteClarification(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*TravelQueryValidationInputV2)
	}{
		{
			name: "unknown StopID",
			mutate: func(input *TravelQueryValidationInputV2) {
				input.AvailabilityFacts.VisibleOptions[0].DestinationStopID = "SC_UNKNOWN"
			},
		},
		{
			name: "divergent display",
			mutate: func(input *TravelQueryValidationInputV2) {
				input.AvailabilityFacts.VisibleOptions[0].DestinationDisplayName = "Seara/SC"
			},
		},
		{
			name: "incomplete option",
			mutate: func(input *TravelQueryValidationInputV2) {
				input.AvailabilityFacts.VisibleOptions[0].TripID = ""
			},
		},
		{
			name: "invalid date",
			mutate: func(input *TravelQueryValidationInputV2) {
				input.AvailabilityFacts.VisibleOptions[0].TripDate = "2030-02-31"
			},
		},
	}

	newInput := func() TravelQueryValidationInputV2 {
		input := travelQueryV2SelectionInput(1)
		input.Proposal.Intent = TravelQueryIntentAvailabilitySearch
		input.Proposal.TurnMeaning = TurnMeaningNewRequest
		input.Proposal.Origin = nil
		input.Proposal.Destination = nil
		input.Proposal.OptionReference = OptionReference{Kind: OptionReferenceNone}
		input.Proposal.NeedsClarification = true
		input.Proposal.MissingFields = []string{"origin", "destination"}
		input.State.Route.Origin = "Seara/SC"
		input.State.Route.Destination = "Videira/SC"
		return input
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := newInput()
			test.mutate(&input)

			got := ValidateTravelQueryMeaningV2(input)

			assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonCurrentAvailabilityFactsInvalid)
		})
	}

	t.Run("empty turn keeps precedence over invalid facts", func(t *testing.T) {
		input := newInput()
		input.CurrentTurn = "   "
		input.AvailabilityFacts.VisibleOptions[0].DestinationStopID = "SC_UNKNOWN"

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonEmptyCurrentTurn)
	})

	t.Run("strong decision keeps precedence over invalid facts", func(t *testing.T) {
		input := newInput()
		input.ExistingDecisionStrength = DecisionStrengthStrong
		input.AvailabilityFacts.VisibleOptions[0].DestinationStopID = "SC_UNKNOWN"

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonStrongDecisionProtected)
	})
}

func TestValidateTravelQueryMeaningV2DistinguishesUnavailableFromCurrentAvailabilityFacts(t *testing.T) {
	t.Run("absent facts allow compatible state", func(t *testing.T) {
		input := validTravelQueryV2AvailabilityInput()
		input.State.Route.Origin = "Seara/SC"
		input.State.Route.Destination = "Videira/SC"

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Accepted(t, got)
	})

	t.Run("stale facts allow compatible state", func(t *testing.T) {
		input := travelQueryV2SelectionInput(1)
		input.State.Route.Origin = "Seara/SC"
		input.State.Route.Destination = "Videira/SC"
		input.Proposal.Intent = TravelQueryIntentAvailabilitySearch
		input.Proposal.TurnMeaning = TurnMeaningNewRequest
		input.Proposal.Origin = &LocationMeaning{Name: "Seara/SC", Role: LocationRoleOrigin}
		input.Proposal.Destination = &LocationMeaning{Name: "Videira/SC", Role: LocationRoleDestination}
		input.Proposal.OptionReference = OptionReference{Kind: OptionReferenceNone}
		input.AvailabilityFacts.SourceMessageID = "prompt-stale-v2"
		input.AvailabilityFacts.SourceMessageReceivedAt = input.ActivePrompt.SourceMessageReceivedAt.Add(-time.Minute)

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Accepted(t, got)
	})

	t.Run("valid current facts override divergent state", func(t *testing.T) {
		input := travelQueryV2SelectionInput(1)
		input.State.Route.Origin = "Fraiburgo/SC"
		input.State.Route.Destination = "Santa Inês/MA"
		input.Proposal.Origin = &LocationMeaning{Name: "Seara/SC", Role: LocationRoleOrigin}
		input.Proposal.Destination = &LocationMeaning{Name: "Videira/SC", Role: LocationRoleDestination}
		input.LocationCatalog = append(input.LocationCatalog,
			TravelQueryLocationEvidenceV2{CanonicalName: "Fraiburgo/SC", StopID: "SC_FRAIBURGO"},
			TravelQueryLocationEvidenceV2{CanonicalName: "Santa Inês/MA", StopID: "MA_SANTA_INES"},
		)

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Accepted(t, got)
	})

	t.Run("valid index uses only selected option facts", func(t *testing.T) {
		input := travelQueryV2SelectionInput(2)
		input.LocationCatalog = append(input.LocationCatalog, TravelQueryLocationEvidenceV2{
			CanonicalName: "Santa Cecilia/SE",
			StopID:        "SE_SANTA_CECILIA",
		})
		input.AvailabilityFacts.VisibleOptions[0].DestinationStopID = "SC_SANTA_CECILIA"
		input.AvailabilityFacts.VisibleOptions[0].DestinationDisplayName = "Santa Cecilia/SC"
		input.AvailabilityFacts.VisibleOptions[1].DestinationStopID = "SE_SANTA_CECILIA"
		input.AvailabilityFacts.VisibleOptions[1].DestinationDisplayName = "Santa Cecilia/SE"
		input.Proposal.Origin = &LocationMeaning{Name: "Seara/SC", Role: LocationRoleOrigin}
		input.Proposal.Destination = &LocationMeaning{Name: "Santa Cecilia/SE", Role: LocationRoleDestination}
		input.Proposal.OptionReference = OptionReference{Kind: OptionReferenceIndex, Index: 2}

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Accepted(t, got)
	})
}

func TestValidateTravelQueryMeaningV2DoesNotInventStateForAmbiguousCity(t *testing.T) {
	input := validTravelQueryV2AvailabilityInput()
	input.LocationCatalog = append(input.LocationCatalog, TravelQueryLocationEvidenceV2{
		CanonicalName: "Seara/MA",
		StopID:        "MA_SEARA",
	})

	got := ValidateTravelQueryMeaningV2(input)

	assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonLocationNotGrounded)

	t.Run("accepts clarification while ambiguous origin remains unset", func(t *testing.T) {
		input := validTravelQueryV2AvailabilityInput()
		input.Proposal.Origin = nil
		input.Proposal.NeedsClarification = true
		input.Proposal.MissingFields = []string{"origin"}
		input.LocationCatalog = append(input.LocationCatalog, TravelQueryLocationEvidenceV2{
			CanonicalName: "Seara/MA",
			StopID:        "MA_SEARA",
		})

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Accepted(t, got)
	})

	t.Run("explicit canonical state is unambiguous evidence", func(t *testing.T) {
		input := validTravelQueryV2AvailabilityInput()
		input.State.Route.Origin = "Seara/SC"
		input.State.Route.Destination = "Videira/SC"
		input.LocationCatalog = append(input.LocationCatalog, TravelQueryLocationEvidenceV2{
			CanonicalName: "Seara/MA",
			StopID:        "MA_SEARA",
		})

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Accepted(t, got)
	})
}

func TestValidateTravelQueryMeaningV2ValidatesStructuredCatalogIdentity(t *testing.T) {
	longLocation := TravelQueryLocationEvidenceV2{
		CanonicalName: "Santa Cecilia do Pavao/PR",
		StopID:        "PR_SANTA_CECILIA_DO_PAVAO",
	}

	t.Run("accepts exact long canonical city", func(t *testing.T) {
		input := validTravelQueryV2AvailabilityInput()
		input.Proposal.Origin = nil
		input.Proposal.Destination = &LocationMeaning{Name: longLocation.CanonicalName, Role: LocationRoleDestination}
		input.LocationCatalog = append(input.LocationCatalog, longLocation)

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Accepted(t, got)
	})

	t.Run("accepts independent short canonical city", func(t *testing.T) {
		input := validTravelQueryV2AvailabilityInput()
		input.Proposal.Origin = nil
		input.Proposal.Destination = &LocationMeaning{Name: "Santa Cecilia/SC", Role: LocationRoleDestination}
		input.LocationCatalog = append(input.LocationCatalog, longLocation)

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Accepted(t, got)
	})

	t.Run("rejects structured state variant absent from catalog", func(t *testing.T) {
		input := validTravelQueryV2AvailabilityInput()
		input.Proposal.Origin = nil
		input.Proposal.Destination = &LocationMeaning{Name: "Santa Cecilia do Pavao/SC", Role: LocationRoleDestination}
		input.LocationCatalog = append(input.LocationCatalog, longLocation)

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonLocationNotInCatalog)
	})
}

func TestValidateTravelQueryMeaningV2DoesNotApplyASRCorrectionsToCanonicalCatalogIdentity(t *testing.T) {
	t.Run("rejects Freiburg when only Fraiburgo is cataloged", func(t *testing.T) {
		input := validTravelQueryV2BusinessRouteInput()
		input.Proposal.Origin = &LocationMeaning{Name: "Freiburg/SC", Role: LocationRoleOrigin}

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonLocationNotInCatalog)
	})

	t.Run("accepts the exact cataloged canonical identity", func(t *testing.T) {
		input := validTravelQueryV2BusinessRouteInput()

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Accepted(t, got)
	})
}

func TestValidateTravelQueryMeaningV2RequiresUnambiguousStructuredFactsForHomonymousCity(t *testing.T) {
	seLocation := TravelQueryLocationEvidenceV2{
		CanonicalName: "Santa Cecilia/SE",
		StopID:        "SE_SANTA_CECILIA",
	}

	t.Run("rejects homonymous proposal without state or current facts", func(t *testing.T) {
		input := validTravelQueryV2AvailabilityInput()
		input.Proposal.Origin = nil
		input.Proposal.Destination = &LocationMeaning{Name: "Santa Cecilia/SC", Role: LocationRoleDestination}
		input.LocationCatalog = append(input.LocationCatalog, seLocation)

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonLocationNotGrounded)
	})

	t.Run("accepts homonymous proposal grounded by canonical state", func(t *testing.T) {
		input := validTravelQueryV2AvailabilityInput()
		input.State.Route.Destination = "Santa Cecilia/SE"
		input.Proposal.Origin = nil
		input.Proposal.Destination = &LocationMeaning{Name: seLocation.CanonicalName, Role: LocationRoleDestination}
		input.LocationCatalog = append(input.LocationCatalog, seLocation)

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Accepted(t, got)
	})

	t.Run("rejects collectively ambiguous current facts", func(t *testing.T) {
		input := travelQueryV2SelectionInput(2)
		input.AvailabilityFacts.VisibleOptions[0].DestinationStopID = "SC_SANTA_CECILIA"
		input.AvailabilityFacts.VisibleOptions[0].DestinationDisplayName = "Santa Cecilia/SC"
		input.AvailabilityFacts.VisibleOptions[1].DestinationStopID = "SE_SANTA_CECILIA"
		input.AvailabilityFacts.VisibleOptions[1].DestinationDisplayName = "Santa Cecilia/SE"
		input.Proposal.Origin = &LocationMeaning{Name: "Seara/SC", Role: LocationRoleOrigin}
		input.Proposal.Destination = &LocationMeaning{Name: "Santa Cecilia/SC", Role: LocationRoleDestination}
		input.Proposal.OptionReference = OptionReference{Kind: OptionReferenceNone}
		input.Proposal.NeedsClarification = true
		input.Proposal.MissingFields = []string{"option_reference"}
		input.LocationCatalog = append(input.LocationCatalog, seLocation)

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonLocationNotGrounded)
	})

	t.Run("accepts homonymous endpoint from option resolved by index", func(t *testing.T) {
		input := travelQueryV2SelectionInput(2)
		input.AvailabilityFacts.VisibleOptions[0].DestinationStopID = "SC_SANTA_CECILIA"
		input.AvailabilityFacts.VisibleOptions[0].DestinationDisplayName = "Santa Cecilia/SC"
		input.AvailabilityFacts.VisibleOptions[1].DestinationStopID = "SE_SANTA_CECILIA"
		input.AvailabilityFacts.VisibleOptions[1].DestinationDisplayName = "Santa Cecilia/SE"
		input.Proposal.Origin = &LocationMeaning{Name: "Seara/SC", Role: LocationRoleOrigin}
		input.Proposal.Destination = &LocationMeaning{Name: "Santa Cecilia/SE", Role: LocationRoleDestination}
		input.Proposal.OptionReference = OptionReference{Kind: OptionReferenceIndex, Index: 2}
		input.LocationCatalog = append(input.LocationCatalog, seLocation)

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Accepted(t, got)
	})
}

func TestValidateTravelQueryMeaningV2UsesCurrentAvailabilityStopIDsAsEndpointIdentity(t *testing.T) {
	seLocation := TravelQueryLocationEvidenceV2{
		CanonicalName: "Santa Cecilia/SE",
		StopID:        "SE_SANTA_CECILIA",
	}
	alternateVideiraStop := TravelQueryLocationEvidenceV2{
		CanonicalName: "Videira/SC",
		StopID:        "SC_VIDEIRA_ALTERNATE",
	}

	t.Run("rejects display that diverges from destination StopID", func(t *testing.T) {
		input := travelQueryV2SelectionInput(1)
		input.LocationCatalog = append(input.LocationCatalog, seLocation)
		input.AvailabilityFacts.VisibleOptions[0].DestinationStopID = seLocation.StopID
		input.AvailabilityFacts.VisibleOptions[0].DestinationDisplayName = "Santa Cecilia/SC"
		input.Proposal.Origin = &LocationMeaning{Name: "Seara/SC", Role: LocationRoleOrigin}
		input.Proposal.Destination = &LocationMeaning{Name: "Santa Cecilia/SC", Role: LocationRoleDestination}

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonCurrentAvailabilityFactsInvalid)
	})

	t.Run("uses destination StopID when display omits UF", func(t *testing.T) {
		input := travelQueryV2SelectionInput(1)
		input.LocationCatalog = append(input.LocationCatalog, seLocation)
		input.AvailabilityFacts.VisibleOptions[0].DestinationStopID = seLocation.StopID
		input.AvailabilityFacts.VisibleOptions[0].DestinationDisplayName = "Santa Cecilia"
		input.Proposal.Origin = &LocationMeaning{Name: "Seara/SC", Role: LocationRoleOrigin}
		input.Proposal.Destination = &LocationMeaning{Name: seLocation.CanonicalName, Role: LocationRoleDestination}

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Accepted(t, got)
	})

	t.Run("rejects facts with unknown destination StopID", func(t *testing.T) {
		input := travelQueryV2SelectionInput(1)
		input.AvailabilityFacts.VisibleOptions[0].DestinationStopID = "SC_UNKNOWN"
		input.Proposal.Origin = nil
		input.Proposal.Destination = nil

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonCurrentAvailabilityFactsInvalid)
	})

	t.Run("rejects facts with ambiguous destination StopID", func(t *testing.T) {
		input := travelQueryV2SelectionInput(1)
		input.LocationCatalog = append(input.LocationCatalog, TravelQueryLocationEvidenceV2{
			CanonicalName: "Videira/PR",
			StopID:        "SC_VIDEIRA",
		})
		input.Proposal.Origin = nil
		input.Proposal.Destination = nil

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonCurrentAvailabilityFactsInvalid)
	})

	t.Run("does not collectively ground different StopIDs for the same canonical endpoint", func(t *testing.T) {
		input := travelQueryV2SelectionInput(2)
		input.LocationCatalog = append(input.LocationCatalog, alternateVideiraStop)
		input.AvailabilityFacts.VisibleOptions[1].DestinationStopID = alternateVideiraStop.StopID
		input.Proposal.Origin = &LocationMeaning{Name: "Seara/SC", Role: LocationRoleOrigin}
		input.Proposal.Destination = &LocationMeaning{Name: "Videira/SC", Role: LocationRoleDestination}
		input.Proposal.OptionReference = OptionReference{Kind: OptionReferenceNone}
		input.Proposal.NeedsClarification = true
		input.Proposal.MissingFields = []string{"option_reference"}

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonLocationNotGrounded)
	})

	t.Run("uses only the selected option StopIDs for a valid index", func(t *testing.T) {
		input := travelQueryV2SelectionInput(2)
		input.LocationCatalog = append(input.LocationCatalog, alternateVideiraStop)
		input.AvailabilityFacts.VisibleOptions[1].DestinationStopID = alternateVideiraStop.StopID
		input.Proposal.Origin = &LocationMeaning{Name: "Seara/SC", Role: LocationRoleOrigin}
		input.Proposal.Destination = &LocationMeaning{Name: "Videira/SC", Role: LocationRoleDestination}
		input.Proposal.OptionReference = OptionReference{Kind: OptionReferenceIndex, Index: 2}

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Accepted(t, got)
	})
}

func TestValidateTravelQueryMeaningV2ValidatesExactDate(t *testing.T) {
	tests := []struct {
		name   string
		date   string
		reason TravelQueryValidationReasonCode
	}{
		{name: "calendar invalid", date: "2026-02-31", reason: TravelQueryValidationReasonInvalidExactDate},
		{name: "past", date: "2026-07-13", reason: TravelQueryValidationReasonExactDateInPast},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validTravelQueryV2AvailabilityInput()
			input.ObservedAt = time.Date(2026, 7, 14, 12, 0, 0, 0, time.FixedZone("BRT", -3*60*60))
			input.Proposal.DatePreference = DatePreference{Mode: DateModeExact, ExactDate: test.date}

			got := ValidateTravelQueryMeaningV2(input)

			assertTravelQueryV2Rejected(t, got, test.reason)
		})
	}

	t.Run("today is valid", func(t *testing.T) {
		input := validTravelQueryV2AvailabilityInput()
		input.ObservedAt = time.Date(2026, 7, 14, 23, 0, 0, 0, time.FixedZone("BRT", -3*60*60))
		input.Proposal.DatePreference = DatePreference{Mode: DateModeExact, ExactDate: "2026-07-14"}

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Accepted(t, got)
	})

	t.Run("observed at is required", func(t *testing.T) {
		input := validTravelQueryV2AvailabilityInput()
		input.ObservedAt = time.Time{}
		input.Proposal.DatePreference = DatePreference{Mode: DateModeExact, ExactDate: "2026-07-14"}

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonObservedAtRequired)
	})
}

func TestEarliestAvailableValidationRequiresMinimumCatalogRoute(t *testing.T) {
	t.Run("rejects without route", func(t *testing.T) {
		input := validTravelQueryV2AvailabilityInput()
		input.Proposal.Origin = nil
		input.Proposal.Destination = nil
		input.Proposal.DatePreference = DatePreference{Mode: DateModeEarliestAvailable}

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonEarliestAvailableRouteRequired)
	})

	t.Run("accepts route from canonical state", func(t *testing.T) {
		input := validTravelQueryV2AvailabilityInput()
		input.Proposal.Origin = nil
		input.Proposal.Destination = nil
		input.Proposal.DatePreference = DatePreference{Mode: DateModeEarliestAvailable}
		input.State.Route.Origin = "Seara/SC"
		input.State.Route.Destination = "Videira/SC"

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Accepted(t, got)
	})
}

func TestEarliestAvailableValidationUsesAvailabilityFactsRoutePrecedence(t *testing.T) {
	newInput := func(optionCount int) TravelQueryValidationInputV2 {
		input := travelQueryV2SelectionInput(optionCount)
		input.Proposal.Intent = TravelQueryIntentAvailabilitySearch
		input.Proposal.TurnMeaning = TurnMeaningNewRequest
		input.Proposal.Origin = nil
		input.Proposal.Destination = nil
		input.Proposal.OptionReference = OptionReference{Kind: OptionReferenceNone}
		input.Proposal.DatePreference = DatePreference{Mode: DateModeEarliestAvailable}
		return input
	}

	t.Run("valid agreeing facts override divergent state", func(t *testing.T) {
		input := newInput(2)
		input.State.Route.Origin = "Fraiburgo/SC"
		input.State.Route.Destination = "Santa Inês/MA"
		input.LocationCatalog = append(input.LocationCatalog,
			TravelQueryLocationEvidenceV2{CanonicalName: "Fraiburgo/SC", StopID: "SC_FRAIBURGO"},
			TravelQueryLocationEvidenceV2{CanonicalName: "Santa Inês/MA", StopID: "MA_SANTA_INES"},
		)

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Accepted(t, got)
	})

	t.Run("valid divergent facts reject stale compatible state", func(t *testing.T) {
		input := newInput(2)
		input.State.Route.Origin = "Seara/SC"
		input.State.Route.Destination = "Videira/SC"
		input.AvailabilityFacts.VisibleOptions[1].DestinationStopID = "SC_SANTA_CECILIA"
		input.AvailabilityFacts.VisibleOptions[1].DestinationDisplayName = "Santa Cecilia/SC"

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonEarliestAvailableRouteRequired)
	})

	t.Run("not applicable facts allow compatible state", func(t *testing.T) {
		input := newInput(1)
		input.State.Route.Origin = "Seara/SC"
		input.State.Route.Destination = "Videira/SC"
		input.AvailabilityFacts.SourceMessageID = "prompt-stale-v2"
		input.AvailabilityFacts.SourceMessageReceivedAt = input.ActivePrompt.SourceMessageReceivedAt.Add(-time.Minute)

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Accepted(t, got)
	})

	t.Run("invalid facts never use compatible state", func(t *testing.T) {
		input := newInput(1)
		input.State.Route.Origin = "Seara/SC"
		input.State.Route.Destination = "Videira/SC"
		input.AvailabilityFacts.VisibleOptions[0].DestinationStopID = "SC_UNKNOWN"

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonCurrentAvailabilityFactsInvalid)
	})
}

func TestValidateTravelQueryMeaningV2ValidatesStructuredCurrentOptionIndex(t *testing.T) {
	t.Run("accepts positive index inside current facts", func(t *testing.T) {
		input := travelQueryV2SelectionInput(2)
		input.Proposal.OptionReference = OptionReference{Kind: OptionReferenceIndex, Index: 2}
		input.CurrentTurn = "opaque non-empty content"

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Accepted(t, got)
	})

	t.Run("rejects non positive index shape", func(t *testing.T) {
		input := travelQueryV2SelectionInput(2)
		input.Proposal.OptionReference = OptionReference{Kind: OptionReferenceIndex, Index: 0}

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonInvalidOptionReference)
	})

	t.Run("rejects index outside current facts", func(t *testing.T) {
		input := travelQueryV2SelectionInput(2)
		input.Proposal.OptionReference = OptionReference{Kind: OptionReferenceIndex, Index: 3}

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonOptionIndexOutOfRange)
	})

	t.Run("requires current facts", func(t *testing.T) {
		input := travelQueryV2SelectionInput(2)
		input.Proposal.OptionReference = OptionReference{Kind: OptionReferenceIndex, Index: 2}
		input.AvailabilityFacts = TravelQueryAvailabilityFactsV2{}

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonCurrentAvailabilityFactsRequired)
	})

	t.Run("accepts grounded route context without granting selection side effect", func(t *testing.T) {
		input := travelQueryV2SelectionInput(2)
		input.Proposal.OptionReference = OptionReference{Kind: OptionReferenceIndex, Index: 2}
		input.Proposal.Origin = &LocationMeaning{Name: "Seara/SC", Role: LocationRoleOrigin}
		input.Proposal.Destination = &LocationMeaning{Name: "Videira/SC", Role: LocationRoleDestination}

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Accepted(t, got)
	})
}

func TestValidateTravelQueryMeaningV2AcceptsStructuredOptionClarificationWithoutParsingCurrentTurn(t *testing.T) {
	input := travelQueryV2SelectionInput(2)
	input.Proposal.OptionReference = OptionReference{Kind: OptionReferenceNone}
	input.Proposal.NeedsClarification = true
	input.Proposal.MissingFields = []string{"option_reference"}
	input.CurrentTurn = "opaque non-empty content"

	got := ValidateTravelQueryMeaningV2(input)

	assertTravelQueryV2Accepted(t, got)
}

func TestValidateTravelQueryMeaningV2RejectsAbsentOrStaleAvailabilityFacts(t *testing.T) {
	t.Run("absent facts", func(t *testing.T) {
		input := travelQueryV2SelectionInput(2)
		input.AvailabilityFacts = TravelQueryAvailabilityFactsV2{}

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonCurrentAvailabilityFactsRequired)
	})

	t.Run("stale source message", func(t *testing.T) {
		input := travelQueryV2SelectionInput(2)
		input.History = append(input.History, markAvailabilityPromptDeliveredForTest(Message{
			ID:               "prompt-newer",
			Direction:        "OUTBOUND",
			Body:             "1. Nova opcao\n\nQual opcao voce prefere?",
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       input.AvailabilityFacts.SourceMessageReceivedAt.Add(time.Minute),
		}))

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonCurrentAvailabilityFactsRequired)
	})

	t.Run("past option makes current facts invalid", func(t *testing.T) {
		input := travelQueryV2SelectionInput(1)
		input.AvailabilityFacts.VisibleOptions[0].TripDate = input.ObservedAt.AddDate(0, 0, -1).Format("2006-01-02")

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonCurrentAvailabilityFactsInvalid)
	})

	tests := []struct {
		name   string
		mutate func(*AvailabilitySearchItem)
	}{
		{
			name: "empty option",
			mutate: func(option *AvailabilitySearchItem) {
				*option = AvailabilitySearchItem{}
			},
		},
		{
			name: "missing trip identity",
			mutate: func(option *AvailabilitySearchItem) {
				option.TripID = ""
			},
		},
		{
			name: "missing route identity",
			mutate: func(option *AvailabilitySearchItem) {
				option.BoardStopID = ""
			},
		},
		{
			name: "missing arrival stop identity",
			mutate: func(option *AvailabilitySearchItem) {
				option.AlightStopID = ""
			},
		},
		{
			name: "missing origin endpoint identity",
			mutate: func(option *AvailabilitySearchItem) {
				option.OriginStopID = ""
			},
		},
		{
			name: "missing destination endpoint identity",
			mutate: func(option *AvailabilitySearchItem) {
				option.DestinationStopID = ""
			},
		},
		{
			name: "missing date",
			mutate: func(option *AvailabilitySearchItem) {
				option.TripDate = ""
			},
		},
		{
			name: "invalid date",
			mutate: func(option *AvailabilitySearchItem) {
				option.TripDate = "2030-02-31"
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := travelQueryV2SelectionInput(1)
			test.mutate(&input.AvailabilityFacts.VisibleOptions[0])

			got := ValidateTravelQueryMeaningV2(input)

			assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonCurrentAvailabilityFactsInvalid)
		})
	}
}

func TestValidateTravelQueryMeaningV2ValidatesOptionReferenceShapeBeforeClarification(t *testing.T) {
	tests := []struct {
		name      string
		reference OptionReference
	}{
		{name: "none with index", reference: OptionReference{Kind: OptionReferenceNone, Index: 999}},
		{name: "none with date", reference: OptionReference{Kind: OptionReferenceNone, Date: "invalid"}},
		{name: "index without positive index", reference: OptionReference{Kind: OptionReferenceIndex}},
		{name: "index with date", reference: OptionReference{Kind: OptionReferenceIndex, Index: 1, Date: "2030-07-15"}},
		{name: "date without date", reference: OptionReference{Kind: OptionReferenceDate}},
		{name: "date with invalid date", reference: OptionReference{Kind: OptionReferenceDate, Date: "2030-02-31"}},
		{name: "date with index", reference: OptionReference{Kind: OptionReferenceDate, Index: 1, Date: "2030-07-15"}},
		{name: "deictic with index", reference: OptionReference{Kind: OptionReferenceDeictic, Index: 1}},
		{name: "deictic with date", reference: OptionReference{Kind: OptionReferenceDeictic, Date: "2030-07-15"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := travelQueryV2SelectionInput(1)
			input.Proposal.OptionReference = test.reference
			input.Proposal.NeedsClarification = true
			input.Proposal.MissingFields = []string{"option_reference"}

			got := ValidateTravelQueryMeaningV2(input)

			assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonInvalidOptionReference)
		})
	}
}

func TestValidateTravelQueryMeaningV2ValidatesDateAndDeicticOptionReferences(t *testing.T) {
	t.Run("accepts one visible date", func(t *testing.T) {
		input := travelQueryV2SelectionInput(2)
		input.Proposal.OptionReference = OptionReference{
			Kind: OptionReferenceDate,
			Date: input.AvailabilityFacts.VisibleOptions[0].TripDate,
		}

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Accepted(t, got)
	})

	t.Run("rejects false clarification for one matching date", func(t *testing.T) {
		input := travelQueryV2SelectionInput(2)
		input.Proposal.OptionReference = OptionReference{
			Kind: OptionReferenceDate,
			Date: input.AvailabilityFacts.VisibleOptions[0].TripDate,
		}
		input.Proposal.NeedsClarification = true
		input.Proposal.MissingFields = []string{"option_reference"}

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonIncoherentClarification)
	})

	t.Run("accepts clarification for two options on same date", func(t *testing.T) {
		input := travelQueryV2SelectionInput(2)
		date := input.AvailabilityFacts.VisibleOptions[0].TripDate
		input.AvailabilityFacts.VisibleOptions[1].TripDate = date
		input.Proposal.OptionReference = OptionReference{Kind: OptionReferenceDate, Date: date}
		input.Proposal.NeedsClarification = true
		input.Proposal.MissingFields = []string{"option_reference"}

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Accepted(t, got)
	})

	t.Run("rejects missing date", func(t *testing.T) {
		input := travelQueryV2SelectionInput(2)
		input.Proposal.OptionReference = OptionReference{Kind: OptionReferenceDate}

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonInvalidOptionReference)
	})

	t.Run("rejects invalid date", func(t *testing.T) {
		input := travelQueryV2SelectionInput(2)
		input.Proposal.OptionReference = OptionReference{Kind: OptionReferenceDate, Date: "2030-02-31"}

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonInvalidOptionReference)
	})

	t.Run("rejects date outside visible facts", func(t *testing.T) {
		input := travelQueryV2SelectionInput(2)
		input.Proposal.OptionReference = OptionReference{
			Kind: OptionReferenceDate,
			Date: input.ObservedAt.AddDate(0, 0, 30).Format("2006-01-02"),
		}

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonOptionDateNotVisible)
	})

	t.Run("rejects ambiguous deictic reference", func(t *testing.T) {
		input := travelQueryV2SelectionInput(2)
		input.Proposal.OptionReference = OptionReference{Kind: OptionReferenceDeictic}

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonAmbiguousOptionReference)
	})

	t.Run("accepts ambiguous deictic only as clarification", func(t *testing.T) {
		input := travelQueryV2SelectionInput(2)
		input.Proposal.OptionReference = OptionReference{Kind: OptionReferenceDeictic}
		input.Proposal.NeedsClarification = true
		input.Proposal.MissingFields = []string{"option_reference"}

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Accepted(t, got)
	})

	t.Run("accepts deictic reference with one visible option", func(t *testing.T) {
		input := travelQueryV2SelectionInput(1)
		input.Proposal.OptionReference = OptionReference{Kind: OptionReferenceDeictic}

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Accepted(t, got)
	})

	t.Run("rejects false clarification for resolvable deictic reference", func(t *testing.T) {
		input := travelQueryV2SelectionInput(1)
		input.Proposal.OptionReference = OptionReference{Kind: OptionReferenceDeictic}
		input.Proposal.NeedsClarification = true
		input.Proposal.MissingFields = []string{"option_reference"}

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonIncoherentClarification)
	})
}

func TestCoverageValidationKeepsLookupTruthOutsideSemanticAcceptance(t *testing.T) {
	t.Run("accepts exact stop query without claiming route coverage", func(t *testing.T) {
		input := validTravelQueryV2CoverageInput("Santa Cecilia/SC", RouteCoverageModeExactStop, LocationRoleVia)

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Accepted(t, got)
	})

	t.Run("accepts nearby reference without inventing exact stop", func(t *testing.T) {
		input := validTravelQueryV2CoverageInput("Lebon Regis/SC", RouteCoverageModeNearbyReference, LocationRoleNearbyReference)

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Accepted(t, got)
	})

	t.Run("rejects exact stop query without stop evidence", func(t *testing.T) {
		input := validTravelQueryV2CoverageInput("Santa Cecilia/SC", RouteCoverageModeExactStop, LocationRoleVia)
		for index := range input.LocationCatalog {
			if input.LocationCatalog[index].CanonicalName == "Santa Cecilia/SC" {
				input.LocationCatalog[index].StopID = ""
			}
		}

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonRouteCoverageEvidenceRequired)
	})

	t.Run("rejects coverage location outside catalog", func(t *testing.T) {
		input := validTravelQueryV2CoverageInput("Atlantis/SC", RouteCoverageModeExactStop, LocationRoleVia)

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonLocationNotInCatalog)
	})

	t.Run("nearby role cannot satisfy exact stop mode", func(t *testing.T) {
		input := validTravelQueryV2CoverageInput("Lebon Regis/SC", RouteCoverageModeExactStop, LocationRoleNearbyReference)

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonContradictoryLocationRole)
	})

	t.Run("exact stop requires via role", func(t *testing.T) {
		input := validTravelQueryV2CoverageInput("Santa Cecilia/SC", RouteCoverageModeExactStop, LocationRoleNearbyReference)

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonContradictoryLocationRole)
	})

	t.Run("nearby reference requires nearby role", func(t *testing.T) {
		input := validTravelQueryV2CoverageInput("Santa Cecilia/SC", RouteCoverageModeNearbyReference, LocationRoleVia)

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonContradictoryLocationRole)
	})
}

func TestValidateTravelQueryMeaningV2AcceptsPureAcknowledgement(t *testing.T) {
	input := validTravelQueryV2AcknowledgementInput()

	got := ValidateTravelQueryMeaningV2(input)

	assertTravelQueryV2Accepted(t, got)
}

func TestValidateTravelQueryMeaningV2RejectsAcknowledgementWithOperationalTravelIntent(t *testing.T) {
	tests := []struct {
		name   string
		intent TravelQueryIntent
	}{
		{name: "availability search", intent: TravelQueryIntentAvailabilitySearch},
		{name: "select availability option", intent: TravelQueryIntentSelectAvailabilityOption},
		{name: "route coverage", intent: TravelQueryIntentRouteCoverage},
		{name: "seat request", intent: TravelQueryIntentSeatRequest},
		{name: "institutional question", intent: TravelQueryIntentInstitutionalQuestion},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validTravelQueryV2AcknowledgementInput()
			input.Proposal.Intent = test.intent

			got := ValidateTravelQueryMeaningV2(input)

			assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonInconsistentSemanticCombination)
		})
	}
}

func TestValidateTravelQueryMeaningV2RejectsAcknowledgementWithTravelPayload(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*TravelQueryValidationInputV2)
	}{
		{name: "origin", mutate: func(input *TravelQueryValidationInputV2) {
			input.Proposal.Origin = &LocationMeaning{Name: "Seara/SC", Role: LocationRoleOrigin}
		}},
		{name: "destination", mutate: func(input *TravelQueryValidationInputV2) {
			input.Proposal.Destination = &LocationMeaning{Name: "Videira/SC", Role: LocationRoleDestination}
		}},
		{name: "mentioned location", mutate: func(input *TravelQueryValidationInputV2) {
			input.Proposal.MentionedLocations = []LocationMeaning{{Name: "Santa Cecilia/SC", Role: LocationRoleVia}}
		}},
		{name: "date", mutate: func(input *TravelQueryValidationInputV2) {
			input.Proposal.DatePreference = DatePreference{Mode: DateModeExact, ExactDate: "2030-07-15"}
		}},
		{name: "option", mutate: func(input *TravelQueryValidationInputV2) {
			input.Proposal.OptionReference = OptionReference{Kind: OptionReferenceIndex, Index: 1}
		}},
		{name: "coverage", mutate: func(input *TravelQueryValidationInputV2) {
			input.Proposal.RouteCoverage = RouteCoverageMeaning{Mode: RouteCoverageModeExactStop, QueryLocation: "Santa Cecilia/SC"}
		}},
		{name: "seat", mutate: func(input *TravelQueryValidationInputV2) {
			input.Proposal.SeatRequest = SeatRequestChooseSpecificSeat
		}},
		{name: "institutional", mutate: func(input *TravelQueryValidationInputV2) {
			input.Proposal.InstitutionalTopic = InstitutionalTopicAddress
		}},
		{name: "clarification", mutate: func(input *TravelQueryValidationInputV2) {
			input.Proposal.NeedsClarification = true
			input.Proposal.MissingFields = []string{"origin"}
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validTravelQueryV2AcknowledgementInput()
			test.mutate(&input)

			got := ValidateTravelQueryMeaningV2(input)

			assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonInconsistentSemanticCombination)
		})
	}
}

func TestInstitutionalValidationRequiresConsistentTopicAndNoTravelPayload(t *testing.T) {
	t.Run("accepts typed institutional question", func(t *testing.T) {
		input := validTravelQueryV2InstitutionalInput()

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Accepted(t, got)
	})

	t.Run("rejects missing institutional topic", func(t *testing.T) {
		input := validTravelQueryV2InstitutionalInput()
		input.Proposal.InstitutionalTopic = InstitutionalTopicNone

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonInvalidInstitutionalTopic)
	})

	t.Run("rejects institutional topic on availability intent", func(t *testing.T) {
		input := validTravelQueryV2AvailabilityInput()
		input.Proposal.InstitutionalTopic = InstitutionalTopicAddress

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonInconsistentSemanticCombination)
	})

	t.Run("rejects public table semantics mixed into institutional intent", func(t *testing.T) {
		input := validTravelQueryV2InstitutionalInput()
		input.Proposal.DatePreference = DatePreference{Mode: DateModeAnyAvailable}

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonInconsistentSemanticCombination)
	})
}

func TestSeatValidationNeverReturnsOperationalAuthorization(t *testing.T) {
	t.Run("accepts specific seat as semantic meaning only", func(t *testing.T) {
		input := validTravelQueryV2SeatInput(SeatRequestChooseSpecificSeat)

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Accepted(t, got)
	})

	t.Run("rejects specific seat mixed with trip selection", func(t *testing.T) {
		input := validTravelQueryV2SeatInput(SeatRequestChooseSpecificSeat)
		input.Proposal.OptionReference = OptionReference{Kind: OptionReferenceIndex, Index: 1}

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonInconsistentSemanticCombination)
	})

	t.Run("rejects seat intent without mode", func(t *testing.T) {
		input := validTravelQueryV2SeatInput(SeatRequestNone)

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonInvalidSeatRequest)
	})

	t.Run("accepts route context without turning specific seat into booking", func(t *testing.T) {
		input := validTravelQueryV2SeatInput(SeatRequestChooseSpecificSeat)
		input.Proposal.Origin = &LocationMeaning{Name: "Seara/SC", Role: LocationRoleOrigin}
		input.Proposal.Destination = &LocationMeaning{Name: "Videira/SC", Role: LocationRoleDestination}
		input.LocationCatalog = travelQueryV2LocationCatalog()

		got := ValidateTravelQueryMeaningV2(input)

		assertTravelQueryV2Accepted(t, got)
	})
}

func TestValidateTravelQueryMeaningV2RequiresClosedDecisionStrength(t *testing.T) {
	tests := []struct {
		name     string
		strength DecisionStrength
		reason   TravelQueryValidationReasonCode
	}{
		{name: "empty", strength: "", reason: TravelQueryValidationReasonInvalidEnumValue},
		{name: "unknown", strength: DecisionStrength("UNKNOWN"), reason: TravelQueryValidationReasonInvalidEnumValue},
		{name: "strong", strength: DecisionStrengthStrong, reason: TravelQueryValidationReasonStrongDecisionProtected},
		{name: "weak", strength: DecisionStrengthWeak},
		{name: "fallback", strength: DecisionStrengthFallback},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validTravelQueryV2AvailabilityInput()
			input.ExistingDecisionStrength = test.strength

			got := ValidateTravelQueryMeaningV2(input)

			if test.reason != "" {
				assertTravelQueryV2Rejected(t, got, test.reason)
				return
			}
			assertTravelQueryV2Accepted(t, got)
		})
	}
}

func TestValidateTravelQueryMeaningV2RequiresCoherentMissingFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*TravelQueryValidationInputV2)
	}{
		{
			name: "missing field without clarification",
			mutate: func(input *TravelQueryValidationInputV2) {
				input.Proposal.MissingFields = []string{"destination"}
			},
		},
		{
			name: "clarification without missing field",
			mutate: func(input *TravelQueryValidationInputV2) {
				input.Proposal.NeedsClarification = true
			},
		},
		{
			name: "declared field is already present",
			mutate: func(input *TravelQueryValidationInputV2) {
				input.Proposal.NeedsClarification = true
				input.Proposal.MissingFields = []string{"destination"}
			},
		},
		{
			name: "unknown missing field",
			mutate: func(input *TravelQueryValidationInputV2) {
				input.Proposal.NeedsClarification = true
				input.Proposal.MissingFields = []string{"booking_id"}
			},
		},
		{
			name: "duplicate missing field",
			mutate: func(input *TravelQueryValidationInputV2) {
				input.Proposal.Destination = nil
				input.Proposal.NeedsClarification = true
				input.Proposal.MissingFields = []string{"destination", "destination"}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validTravelQueryV2AvailabilityInput()
			test.mutate(&input)

			got := ValidateTravelQueryMeaningV2(input)

			assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonIncoherentClarification)
		})
	}
}

func TestValidateTravelQueryMeaningV2MissingFieldsAreExactAndCaseSensitive(t *testing.T) {
	tests := []struct {
		name    string
		missing string
		accept  bool
	}{
		{name: "origin exact", missing: "origin", accept: true},
		{name: "origin exact after trim", missing: "  origin  ", accept: true},
		{name: "origin uppercase", missing: "ORIGIN"},
		{name: "origin title case", missing: "Origin"},
		{name: "unknown value", missing: "booking_id"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validTravelQueryV2AvailabilityInput()
			input.Proposal.Origin = nil
			input.Proposal.NeedsClarification = true
			input.Proposal.MissingFields = []string{test.missing}

			got := ValidateTravelQueryMeaningV2(input)
			if test.accept {
				assertTravelQueryV2Accepted(t, got)
				return
			}
			assertTravelQueryV2Rejected(t, got, TravelQueryValidationReasonIncoherentClarification)
		})
	}
}

func TestValidateTravelQueryMeaningV2AcceptsCoherentClarificationForMissingTypedDetail(t *testing.T) {
	tests := []struct {
		name    string
		intent  TravelQueryIntent
		missing string
	}{
		{name: "route coverage", intent: TravelQueryIntentRouteCoverage, missing: "route_coverage"},
		{name: "seat request", intent: TravelQueryIntentSeatRequest, missing: "seat_request"},
		{name: "institutional topic", intent: TravelQueryIntentInstitutionalQuestion, missing: "institutional_topic"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := TravelQueryValidationInputV2{
				Proposal: TravelQueryMeaningV2{
					Intent:             test.intent,
					TurnMeaning:        TurnMeaningNewRequest,
					DatePreference:     DatePreference{Mode: DateModeUnspecified},
					OptionReference:    OptionReference{Kind: OptionReferenceNone},
					RouteCoverage:      RouteCoverageMeaning{Mode: RouteCoverageModeUnspecified},
					SeatRequest:        SeatRequestNone,
					InstitutionalTopic: InstitutionalTopicNone,
					NeedsClarification: true,
					MissingFields:      []string{test.missing},
					Confidence:         0.42,
				},
				CurrentTurn:              "opaque non-empty content",
				State:                    CanonicalConversationState{Phase: ConversationPhaseDiscovery},
				ExistingDecisionStrength: DecisionStrengthFallback,
			}

			got := ValidateTravelQueryMeaningV2(input)

			assertTravelQueryV2Accepted(t, got)
		})
	}
}

func TestTravelQueryValidationV2ReasonCodesAreClosedAndResultHasNoOperationalFields(t *testing.T) {
	reasons := []TravelQueryValidationReasonCode{
		TravelQueryValidationReasonEmptyCurrentTurn,
		TravelQueryValidationReasonUnknownIntent,
		TravelQueryValidationReasonInvalidEnumValue,
		TravelQueryValidationReasonInvalidConfidence,
		TravelQueryValidationReasonLowConfidenceRequiresClarification,
		TravelQueryValidationReasonIncoherentClarification,
		TravelQueryValidationReasonInconsistentSemanticCombination,
		TravelQueryValidationReasonContradictoryLocationRole,
		TravelQueryValidationReasonLocationNotInCatalog,
		TravelQueryValidationReasonLocationNotGrounded,
		TravelQueryValidationReasonObservedAtRequired,
		TravelQueryValidationReasonInvalidExactDate,
		TravelQueryValidationReasonExactDateInPast,
		TravelQueryValidationReasonEarliestAvailableRouteRequired,
		TravelQueryValidationReasonCurrentAvailabilityFactsRequired,
		TravelQueryValidationReasonCurrentAvailabilityFactsInvalid,
		TravelQueryValidationReasonInvalidOptionReference,
		TravelQueryValidationReasonOptionReferenceMismatch,
		TravelQueryValidationReasonOptionIndexOutOfRange,
		TravelQueryValidationReasonOptionDateNotVisible,
		TravelQueryValidationReasonAmbiguousOptionReference,
		TravelQueryValidationReasonInvalidRouteCoverage,
		TravelQueryValidationReasonRouteCoverageEvidenceRequired,
		TravelQueryValidationReasonInvalidSeatRequest,
		TravelQueryValidationReasonInvalidInstitutionalTopic,
		TravelQueryValidationReasonStrongDecisionProtected,
	}
	seen := map[TravelQueryValidationReasonCode]struct{}{}
	for _, reason := range reasons {
		if !travelQueryValidationReasonCodeAllowed(reason) {
			t.Fatalf("reason code is not in the closed allowlist: %q", reason)
		}
		if _, duplicate := seen[reason]; duplicate {
			t.Fatalf("duplicate reason code: %q", reason)
		}
		seen[reason] = struct{}{}
	}
	if travelQueryValidationReasonCodeAllowed("ARBITRARY_REASON") {
		t.Fatal("arbitrary reason code must not be allowed")
	}

	resultType := reflect.TypeOf(TravelQueryValidationResultV2{})
	if resultType.NumField() != 2 || resultType.Field(0).Name != "Status" || resultType.Field(1).Name != "ReasonCodes" {
		t.Fatalf("validation result must contain only status and reason codes: %v", resultType)
	}
	for _, field := range []string{"Tool", "Booking", "Selection", "Handoff", "Message", "Template", "StateMutation", "SideEffect"} {
		if _, exists := resultType.FieldByName(field); exists {
			t.Fatalf("validation result exposes operational field %q", field)
		}
	}
}

func TestValidateTravelQueryMeaningV2DoesNotMutateInputs(t *testing.T) {
	input := travelQueryV2SelectionInput(2)
	input.State.LastToolFacts = map[string]interface{}{
		"availability_search": map[string]interface{}{"source": "must-remain-untouched"},
	}
	input.Proposal.Reasons = []string{"semantic_evidence"}
	before, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal input before validation: %v", err)
	}

	_ = ValidateTravelQueryMeaningV2(input)

	after, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal input after validation: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("validator mutated its input:\nbefore=%s\nafter=%s", before, after)
	}
}

func validTravelQueryV2AvailabilityInput() TravelQueryValidationInputV2 {
	return TravelQueryValidationInputV2{
		Proposal: TravelQueryMeaningV2{
			Intent:             TravelQueryIntentAvailabilitySearch,
			TurnMeaning:        TurnMeaningNewRequest,
			Origin:             &LocationMeaning{Name: "Seara/SC", Role: LocationRoleOrigin},
			Destination:        &LocationMeaning{Name: "Videira/SC", Role: LocationRoleDestination},
			DatePreference:     DatePreference{Mode: DateModeUnspecified},
			OptionReference:    OptionReference{Kind: OptionReferenceNone},
			RouteCoverage:      RouteCoverageMeaning{Mode: RouteCoverageModeUnspecified},
			SeatRequest:        SeatRequestNone,
			InstitutionalTopic: InstitutionalTopicNone,
			Confidence:         0.95,
		},
		CurrentTurn:              "opaque non-empty content",
		State:                    CanonicalConversationState{Phase: ConversationPhaseDiscovery},
		ObservedAt:               time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC),
		LocationCatalog:          travelQueryV2LocationCatalog(),
		ExistingDecisionStrength: DecisionStrengthFallback,
	}
}

func validTravelQueryV2BusinessRouteInput() TravelQueryValidationInputV2 {
	input := validTravelQueryV2AvailabilityInput()
	input.Proposal.Origin = &LocationMeaning{Name: "Fraiburgo/SC", Role: LocationRoleOrigin}
	input.Proposal.Destination = &LocationMeaning{Name: "Santa Inês/MA", Role: LocationRoleDestination}
	input.LocationCatalog = append(input.LocationCatalog,
		TravelQueryLocationEvidenceV2{CanonicalName: "Fraiburgo/SC", StopID: "SC_FRAIBURGO"},
		TravelQueryLocationEvidenceV2{CanonicalName: "Santa Inês/MA", StopID: "MA_SANTA_INES"},
	)
	return input
}

func travelQueryV2SelectionInput(optionCount int) TravelQueryValidationInputV2 {
	observedAt := time.Date(2030, 7, 14, 12, 0, 0, 0, time.UTC)
	receivedAt := observedAt.Add(-time.Minute)
	body := travelQueryV2AvailabilityPromptBody(optionCount)
	options := make([]AvailabilitySearchItem, 0, optionCount)
	for index := 0; index < optionCount; index++ {
		options = append(options, AvailabilitySearchItem{
			TripID:                 fmt.Sprintf("trip-v2-%d", index+1),
			BoardStopID:            fmt.Sprintf("board-v2-%d", index+1),
			AlightStopID:           fmt.Sprintf("alight-v2-%d", index+1),
			OriginStopID:           "SC_SEARA",
			DestinationStopID:      "SC_VIDEIRA",
			OriginDisplayName:      "Seara/SC",
			DestinationDisplayName: "Videira/SC",
			TripDate:               observedAt.AddDate(0, 0, index+1).Format("2006-01-02"),
		})
	}
	return TravelQueryValidationInputV2{
		Proposal: TravelQueryMeaningV2{
			Intent:             TravelQueryIntentSelectAvailabilityOption,
			TurnMeaning:        TurnMeaningAnswerToQuestion,
			DatePreference:     DatePreference{Mode: DateModeUnspecified},
			OptionReference:    OptionReference{Kind: OptionReferenceIndex, Index: 1},
			RouteCoverage:      RouteCoverageMeaning{Mode: RouteCoverageModeUnspecified},
			SeatRequest:        SeatRequestNone,
			InstitutionalTopic: InstitutionalTopicNone,
			Confidence:         0.95,
		},
		CurrentTurn: "opaque non-empty content",
		State: CanonicalConversationState{
			Phase: ConversationPhaseTripSelection,
		},
		ActivePrompt: ActivePromptContext{
			Kind:                    ActivePromptAvailabilityOptionChoice,
			Phase:                   ConversationPhaseTripSelection,
			SourceMessageID:         "prompt-current-v2",
			SourceMessageBody:       body,
			SourceMessageReceivedAt: receivedAt,
			AvailabilityOptionCount: optionCount,
			HasAvailabilityList:     true,
		},
		History: markAvailabilityPromptHistoryDeliveredForTest([]Message{{
			ID:               "prompt-current-v2",
			Direction:        "OUTBOUND",
			Body:             body,
			ProcessingStatus: messageStatusAutomationSent,
			ReceivedAt:       receivedAt,
		}}),
		ObservedAt: observedAt,
		AvailabilityFacts: TravelQueryAvailabilityFactsV2{
			SourceMessageID:         "prompt-current-v2",
			SourceMessageReceivedAt: receivedAt,
			VisibleOptions:          options,
		},
		LocationCatalog:          travelQueryV2LocationCatalog(),
		ExistingDecisionStrength: DecisionStrengthFallback,
	}
}

func travelQueryV2AvailabilityPromptBody(optionCount int) string {
	var builder strings.Builder
	builder.WriteString("Encontrei estas opcoes:\n")
	for index := 1; index <= optionCount; index++ {
		builder.WriteString(fmt.Sprintf("%d. Opcao %d\n", index, index))
	}
	builder.WriteString("\nQual opcao voce prefere?")
	return builder.String()
}

func validTravelQueryV2CoverageInput(location string, mode RouteCoverageMode, role LocationRole) TravelQueryValidationInputV2 {
	return TravelQueryValidationInputV2{
		Proposal: TravelQueryMeaningV2{
			Intent:             TravelQueryIntentRouteCoverage,
			TurnMeaning:        TurnMeaningNewRequest,
			MentionedLocations: []LocationMeaning{{Name: location, Role: role}},
			DatePreference:     DatePreference{Mode: DateModeUnspecified},
			OptionReference:    OptionReference{Kind: OptionReferenceNone},
			RouteCoverage:      RouteCoverageMeaning{QueryLocation: location, Mode: mode},
			SeatRequest:        SeatRequestNone,
			InstitutionalTopic: InstitutionalTopicNone,
			Confidence:         0.95,
		},
		CurrentTurn:              "opaque non-empty content",
		State:                    CanonicalConversationState{Phase: ConversationPhaseDiscovery},
		ObservedAt:               time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC),
		LocationCatalog:          travelQueryV2LocationCatalog(),
		ExistingDecisionStrength: DecisionStrengthFallback,
	}
}

func validTravelQueryV2InstitutionalInput() TravelQueryValidationInputV2 {
	return TravelQueryValidationInputV2{
		Proposal: TravelQueryMeaningV2{
			Intent:             TravelQueryIntentInstitutionalQuestion,
			TurnMeaning:        TurnMeaningNewRequest,
			DatePreference:     DatePreference{Mode: DateModeUnspecified},
			OptionReference:    OptionReference{Kind: OptionReferenceNone},
			RouteCoverage:      RouteCoverageMeaning{Mode: RouteCoverageModeUnspecified},
			SeatRequest:        SeatRequestNone,
			InstitutionalTopic: InstitutionalTopicAddress,
			Confidence:         0.95,
		},
		CurrentTurn:              "opaque non-empty content",
		State:                    CanonicalConversationState{Phase: ConversationPhasePaidFull},
		ExistingDecisionStrength: DecisionStrengthFallback,
	}
}

func validTravelQueryV2AcknowledgementInput() TravelQueryValidationInputV2 {
	return TravelQueryValidationInputV2{
		Proposal: TravelQueryMeaningV2{
			Intent:             TravelQueryIntentUnknown,
			TurnMeaning:        TurnMeaningAcknowledgement,
			DatePreference:     DatePreference{Mode: DateModeUnspecified},
			OptionReference:    OptionReference{Kind: OptionReferenceNone},
			RouteCoverage:      RouteCoverageMeaning{Mode: RouteCoverageModeUnspecified},
			SeatRequest:        SeatRequestNone,
			InstitutionalTopic: InstitutionalTopicNone,
			Confidence:         0.95,
		},
		CurrentTurn:              "opaque non-empty content",
		State:                    CanonicalConversationState{Phase: ConversationPhaseDiscovery},
		ObservedAt:               time.Date(2030, 7, 14, 12, 0, 0, 0, time.UTC),
		LocationCatalog:          travelQueryV2LocationCatalog(),
		ExistingDecisionStrength: DecisionStrengthFallback,
	}
}

func validTravelQueryV2SeatInput(mode SeatRequestMode) TravelQueryValidationInputV2 {
	return TravelQueryValidationInputV2{
		Proposal: TravelQueryMeaningV2{
			Intent:             TravelQueryIntentSeatRequest,
			TurnMeaning:        TurnMeaningNewRequest,
			DatePreference:     DatePreference{Mode: DateModeUnspecified},
			OptionReference:    OptionReference{Kind: OptionReferenceNone},
			RouteCoverage:      RouteCoverageMeaning{Mode: RouteCoverageModeUnspecified},
			SeatRequest:        mode,
			InstitutionalTopic: InstitutionalTopicNone,
			Confidence:         0.95,
		},
		CurrentTurn:              "opaque non-empty content",
		State:                    CanonicalConversationState{Phase: ConversationPhaseDiscovery},
		ExistingDecisionStrength: DecisionStrengthFallback,
	}
}

func travelQueryV2LocationCatalog() []TravelQueryLocationEvidenceV2 {
	return []TravelQueryLocationEvidenceV2{
		{CanonicalName: "Seara/SC", StopID: "SC_SEARA"},
		{CanonicalName: "Videira/SC", StopID: "SC_VIDEIRA"},
		{CanonicalName: "Santa Cecilia/SC", StopID: "SC_SANTA_CECILIA"},
		{CanonicalName: "Lebon Regis/SC"},
	}
}

func assertTravelQueryV2Accepted(t *testing.T, got TravelQueryValidationResultV2) {
	t.Helper()
	if !got.Accepted() || got.Status != TravelQueryValidationStatusAccepted {
		t.Fatalf("expected accepted semantic proposal, got %+v", got)
	}
	if len(got.ReasonCodes) != 0 {
		t.Fatalf("accepted proposal must not contain reject reasons, got %+v", got)
	}
}

func assertTravelQueryV2Rejected(t *testing.T, got TravelQueryValidationResultV2, reason TravelQueryValidationReasonCode) {
	t.Helper()
	if got.Accepted() || got.Status != TravelQueryValidationStatusRejected {
		t.Fatalf("expected rejected semantic proposal, got %+v", got)
	}
	if len(got.ReasonCodes) != 1 || got.ReasonCodes[0] != reason {
		t.Fatalf("expected reason %q, got %+v", reason, got)
	}
}
