package chat

import (
	"reflect"
	"testing"
)

func TestTravelQueryMeaningV2EnumValues(t *testing.T) {
	tests := []struct {
		name string
		got  []string
		want []string
	}{
		{
			name: "travel query intent",
			got: []string{
				string(TravelQueryIntentUnknown),
				string(TravelQueryIntentAvailabilitySearch),
				string(TravelQueryIntentSelectAvailabilityOption),
				string(TravelQueryIntentRouteCoverage),
				string(TravelQueryIntentSeatRequest),
				string(TravelQueryIntentInstitutionalQuestion),
			},
			want: []string{"UNKNOWN", "AVAILABILITY_SEARCH", "SELECT_AVAILABILITY_OPTION", "ROUTE_COVERAGE", "SEAT_REQUEST", "INSTITUTIONAL_QUESTION"},
		},
		{
			name: "location role",
			got: []string{
				string(LocationRoleUnknown),
				string(LocationRoleOrigin),
				string(LocationRoleDestination),
				string(LocationRoleVia),
				string(LocationRoleNearbyReference),
			},
			want: []string{"UNKNOWN", "ORIGIN", "DESTINATION", "VIA", "NEARBY_REFERENCE"},
		},
		{
			name: "date mode",
			got: []string{
				string(DateModeUnspecified),
				string(DateModeExact),
				string(DateModeEarliestAvailable),
				string(DateModeAnyAvailable),
			},
			want: []string{"UNSPECIFIED", "EXACT", "EARLIEST_AVAILABLE", "ANY_AVAILABLE"},
		},
		{
			name: "option reference kind",
			got: []string{
				string(OptionReferenceNone),
				string(OptionReferenceIndex),
				string(OptionReferenceDate),
				string(OptionReferenceDeictic),
			},
			want: []string{"NONE", "INDEX", "DATE", "DEICTIC"},
		},
		{
			name: "route coverage mode",
			got: []string{
				string(RouteCoverageModeUnspecified),
				string(RouteCoverageModeExactStop),
				string(RouteCoverageModeNearbyReference),
			},
			want: []string{"UNSPECIFIED", "EXACT_STOP", "NEARBY_REFERENCE"},
		},
		{
			name: "seat request mode",
			got: []string{
				string(SeatRequestNone),
				string(SeatRequestBookTravel),
				string(SeatRequestChooseSpecificSeat),
			},
			want: []string{"NONE", "BOOK_TRAVEL", "CHOOSE_SPECIFIC_SEAT"},
		},
		{
			name: "institutional topic",
			got: []string{
				string(InstitutionalTopicNone),
				string(InstitutionalTopicCompanyInfo),
				string(InstitutionalTopicContact),
				string(InstitutionalTopicAddress),
				string(InstitutionalTopicBusinessHours),
			},
			want: []string{"NONE", "COMPANY_INFO", "CONTACT", "ADDRESS", "BUSINESS_HOURS"},
		},
		{
			name: "decision strength",
			got: []string{
				string(DecisionStrengthStrong),
				string(DecisionStrengthWeak),
				string(DecisionStrengthFallback),
			},
			want: []string{"STRONG", "WEAK", "FALLBACK"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !reflect.DeepEqual(test.got, test.want) {
				t.Fatalf("unexpected enum values: got %v, want %v", test.got, test.want)
			}
		})
	}
}

func TestTravelQueryMeaningV2ContractShape(t *testing.T) {
	type fieldShape struct {
		name   string
		typeOf reflect.Type
	}
	want := []fieldShape{
		{name: "Intent", typeOf: reflect.TypeOf(TravelQueryIntent(""))},
		{name: "TurnMeaning", typeOf: reflect.TypeOf(TurnMeaning(""))},
		{name: "Origin", typeOf: reflect.TypeOf((*LocationMeaning)(nil))},
		{name: "Destination", typeOf: reflect.TypeOf((*LocationMeaning)(nil))},
		{name: "MentionedLocations", typeOf: reflect.TypeOf([]LocationMeaning(nil))},
		{name: "DatePreference", typeOf: reflect.TypeOf(DatePreference{})},
		{name: "OptionReference", typeOf: reflect.TypeOf(OptionReference{})},
		{name: "RouteCoverage", typeOf: reflect.TypeOf(RouteCoverageMeaning{})},
		{name: "SeatRequest", typeOf: reflect.TypeOf(SeatRequestMode(""))},
		{name: "InstitutionalTopic", typeOf: reflect.TypeOf(InstitutionalTopic(""))},
		{name: "NeedsClarification", typeOf: reflect.TypeOf(false)},
		{name: "MissingFields", typeOf: reflect.TypeOf([]string(nil))},
		{name: "Confidence", typeOf: reflect.TypeOf(float64(0))},
		{name: "Reasons", typeOf: reflect.TypeOf([]string(nil))},
	}

	contract := reflect.TypeOf(TravelQueryMeaningV2{})
	if contract.NumField() != len(want) {
		t.Fatalf("unexpected contract field count: got %d, want %d", contract.NumField(), len(want))
	}
	for index, expected := range want {
		field := contract.Field(index)
		if field.Name != expected.name || field.Type != expected.typeOf {
			t.Fatalf("unexpected field %d: got %s %s, want %s %s", index, field.Name, field.Type, expected.name, expected.typeOf)
		}
	}
}

func TestTravelQueryMeaningV2ZeroValueIsSafe(t *testing.T) {
	var got TravelQueryMeaningV2

	if got.Intent != "" || got.TurnMeaning != "" || got.Origin != nil || got.Destination != nil {
		t.Fatalf("zero value identifies an intent, turn meaning, or route: %+v", got)
	}
	if got.DatePreference.Mode != "" || got.OptionReference.Kind != "" || got.RouteCoverage.Mode != "" {
		t.Fatalf("zero value identifies a date, option, or route coverage mode: %+v", got)
	}
	if got.SeatRequest != "" || got.InstitutionalTopic != "" {
		t.Fatalf("zero value identifies an actionable semantic mode: %+v", got)
	}
	if got.NeedsClarification || got.Confidence != 0 {
		t.Fatalf("zero value requests clarification or reports confidence: %+v", got)
	}
	if got.MentionedLocations != nil || got.MissingFields != nil || got.Reasons != nil {
		t.Fatalf("zero value allocates semantic evidence: %+v", got)
	}
}

func TestTravelQueryMeaningV2UsesExplicitLocationRoles(t *testing.T) {
	got := TravelQueryMeaningV2{
		Intent:      TravelQueryIntentAvailabilitySearch,
		TurnMeaning: TurnMeaningNewRequest,
		Origin:      &LocationMeaning{Name: "Seara", Role: LocationRoleOrigin},
		Destination: &LocationMeaning{Name: "Florianopolis", Role: LocationRoleDestination},
		MentionedLocations: []LocationMeaning{
			{Name: "Santa Cecilia", Role: LocationRoleVia},
			{Name: "Lebon Regis", Role: LocationRoleNearbyReference},
		},
	}

	if got.Origin.Role != LocationRoleOrigin || got.Destination.Role != LocationRoleDestination {
		t.Fatalf("origin and destination roles were not preserved: %+v", got)
	}
	if len(got.MentionedLocations) != 2 || got.MentionedLocations[0].Role != LocationRoleVia || got.MentionedLocations[1].Role != LocationRoleNearbyReference {
		t.Fatalf("mentioned location roles were not preserved as a list: %+v", got.MentionedLocations)
	}
}

func TestTravelQueryMeaningV2RepresentsDatePreferences(t *testing.T) {
	tests := []struct {
		name string
		got  DatePreference
	}{
		{name: "exact", got: DatePreference{Mode: DateModeExact, ExactDate: "2026-07-21"}},
		{name: "earliest available", got: DatePreference{Mode: DateModeEarliestAvailable}},
		{name: "any available", got: DatePreference{Mode: DateModeAnyAvailable}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			meaning := TravelQueryMeaningV2{DatePreference: test.got}
			if meaning.DatePreference != test.got {
				t.Fatalf("date preference was not preserved: got %+v, want %+v", meaning.DatePreference, test.got)
			}
		})
	}
}

func TestTravelQueryMeaningV2RepresentsOptionReferences(t *testing.T) {
	tests := []struct {
		name string
		got  OptionReference
	}{
		{name: "index", got: OptionReference{Kind: OptionReferenceIndex, Index: 1}},
		{name: "date", got: OptionReference{Kind: OptionReferenceDate, Date: "2026-07-21"}},
		{name: "deictic", got: OptionReference{Kind: OptionReferenceDeictic}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			meaning := TravelQueryMeaningV2{OptionReference: test.got}
			if meaning.OptionReference != test.got {
				t.Fatalf("option reference was not preserved: got %+v, want %+v", meaning.OptionReference, test.got)
			}
		})
	}
}

func TestTravelQueryMeaningV2DistinguishesSeatRequests(t *testing.T) {
	bookTravel := TravelQueryMeaningV2{
		Intent:      TravelQueryIntentSeatRequest,
		SeatRequest: SeatRequestBookTravel,
	}
	chooseSeat := TravelQueryMeaningV2{
		Intent:      TravelQueryIntentSeatRequest,
		SeatRequest: SeatRequestChooseSpecificSeat,
	}

	if bookTravel.SeatRequest == chooseSeat.SeatRequest {
		t.Fatalf("booking travel and choosing a specific seat must be distinct: book=%q choose=%q", bookTravel.SeatRequest, chooseSeat.SeatRequest)
	}
}

func TestTravelQueryMeaningV2RepresentsInstitutionalQuestion(t *testing.T) {
	got := TravelQueryMeaningV2{
		Intent:             TravelQueryIntentInstitutionalQuestion,
		InstitutionalTopic: InstitutionalTopicAddress,
	}

	if got.Intent != TravelQueryIntentInstitutionalQuestion || got.InstitutionalTopic != InstitutionalTopicAddress {
		t.Fatalf("institutional question was not preserved: %+v", got)
	}
}

func TestTravelQueryMeaningV2RepresentsRouteCoverage(t *testing.T) {
	tests := []RouteCoverageMeaning{
		{QueryLocation: "Santa Cecilia", Mode: RouteCoverageModeExactStop},
		{QueryLocation: "Lebon Regis", Mode: RouteCoverageModeNearbyReference},
	}

	for _, want := range tests {
		got := TravelQueryMeaningV2{
			Intent:        TravelQueryIntentRouteCoverage,
			RouteCoverage: want,
		}
		if got.RouteCoverage != want {
			t.Fatalf("route coverage meaning was not preserved: got %+v, want %+v", got.RouteCoverage, want)
		}
	}
}

func TestDecisionStrengthValuesRemainDistinct(t *testing.T) {
	values := map[DecisionStrength]struct{}{
		DecisionStrengthStrong:   {},
		DecisionStrengthWeak:     {},
		DecisionStrengthFallback: {},
	}
	if len(values) != 3 {
		t.Fatalf("decision strengths must remain distinct: %v", values)
	}
}

func TestTravelQueryMeaningV2RepresentsClarificationAndEvidence(t *testing.T) {
	got := TravelQueryMeaningV2{
		Intent:             TravelQueryIntentAvailabilitySearch,
		NeedsClarification: true,
		MissingFields:      []string{"destination"},
		Confidence:         0.42,
		Reasons:            []string{"destination_not_identified"},
	}

	if !got.NeedsClarification || !reflect.DeepEqual(got.MissingFields, []string{"destination"}) {
		t.Fatalf("clarification state was not preserved: %+v", got)
	}
	if got.Confidence != 0.42 || !reflect.DeepEqual(got.Reasons, []string{"destination_not_identified"}) {
		t.Fatalf("confidence and reasons were not preserved: %+v", got)
	}
}

func TestTravelQueryMeaningV2KeepsAcknowledgementAsTurnMeaning(t *testing.T) {
	var turnMeaning TurnMeaning = TurnMeaningAcknowledgement
	got := TravelQueryMeaningV2{TurnMeaning: turnMeaning}

	if got.TurnMeaning != TurnMeaningAcknowledgement {
		t.Fatalf("acknowledgement was not preserved as turn meaning: %+v", got)
	}
	for _, intent := range []TravelQueryIntent{
		TravelQueryIntentUnknown,
		TravelQueryIntentAvailabilitySearch,
		TravelQueryIntentSelectAvailabilityOption,
		TravelQueryIntentRouteCoverage,
		TravelQueryIntentSeatRequest,
		TravelQueryIntentInstitutionalQuestion,
	} {
		if string(intent) == string(TurnMeaningAcknowledgement) {
			t.Fatalf("acknowledgement must not be a travel query intent: %q", intent)
		}
	}
}
