package chat

// TravelQueryIntent identifies the travel-domain request expressed in the
// current turn. It is intentionally separate from StructuredIntent V1.
type TravelQueryIntent string

const (
	TravelQueryIntentUnknown                  TravelQueryIntent = "UNKNOWN"
	TravelQueryIntentAvailabilitySearch       TravelQueryIntent = "AVAILABILITY_SEARCH"
	TravelQueryIntentSelectAvailabilityOption TravelQueryIntent = "SELECT_AVAILABILITY_OPTION"
	TravelQueryIntentRouteCoverage            TravelQueryIntent = "ROUTE_COVERAGE"
	TravelQueryIntentSeatRequest              TravelQueryIntent = "SEAT_REQUEST"
	TravelQueryIntentInstitutionalQuestion    TravelQueryIntent = "INSTITUTIONAL_QUESTION"
)

// TurnMeaningAcknowledgement classifies acknowledgements as turn meaning,
// rather than as a travel-domain intent.
const TurnMeaningAcknowledgement TurnMeaning = "ACKNOWLEDGEMENT"

// LocationRole describes how a mentioned location participates in the query.
type LocationRole string

const (
	LocationRoleUnknown         LocationRole = "UNKNOWN"
	LocationRoleOrigin          LocationRole = "ORIGIN"
	LocationRoleDestination     LocationRole = "DESTINATION"
	LocationRoleVia             LocationRole = "VIA"
	LocationRoleNearbyReference LocationRole = "NEARBY_REFERENCE"
)

// LocationMeaning preserves a location mention and its semantic role. Name is
// language-level data and never an executable stop, route, or database ID.
type LocationMeaning struct {
	Name string
	Role LocationRole
}

// DateMode describes whether a query has an exact or relative date preference.
type DateMode string

const (
	DateModeUnspecified       DateMode = "UNSPECIFIED"
	DateModeExact             DateMode = "EXACT"
	DateModeEarliestAvailable DateMode = "EARLIEST_AVAILABLE"
	DateModeAnyAvailable      DateMode = "ANY_AVAILABLE"
)

// DatePreference represents a date mode and, for DateModeExact, an ISO date.
// ExactDate remains semantic input; this contract does not resolve trips.
type DatePreference struct {
	Mode      DateMode
	ExactDate string
}

// OptionReferenceKind describes how the customer referred to an option.
type OptionReferenceKind string

const (
	OptionReferenceNone    OptionReferenceKind = "NONE"
	OptionReferenceIndex   OptionReferenceKind = "INDEX"
	OptionReferenceDate    OptionReferenceKind = "DATE"
	OptionReferenceDeictic OptionReferenceKind = "DEICTIC"
)

// OptionReference preserves an option reference without resolving it against
// availability facts. Index is one-based when Kind is OptionReferenceIndex.
type OptionReference struct {
	Kind  OptionReferenceKind
	Index int
	Date  string
}

// RouteCoverageMode distinguishes an exact stop query from a nearby-reference
// query. It does not assert that the route is covered.
type RouteCoverageMode string

const (
	RouteCoverageModeUnspecified     RouteCoverageMode = "UNSPECIFIED"
	RouteCoverageModeExactStop       RouteCoverageMode = "EXACT_STOP"
	RouteCoverageModeNearbyReference RouteCoverageMode = "NEARBY_REFERENCE"
)

// RouteCoverageMeaning describes the location and match mode being queried.
// Actual coverage requires later validation against current operational facts.
type RouteCoverageMeaning struct {
	QueryLocation string
	Mode          RouteCoverageMode
}

// SeatRequestMode distinguishes booking travel from choosing a specific seat.
type SeatRequestMode string

const (
	SeatRequestNone               SeatRequestMode = "NONE"
	SeatRequestBookTravel         SeatRequestMode = "BOOK_TRAVEL"
	SeatRequestChooseSpecificSeat SeatRequestMode = "CHOOSE_SPECIFIC_SEAT"
)

// InstitutionalTopic classifies non-operational questions about the company.
type InstitutionalTopic string

const (
	InstitutionalTopicNone          InstitutionalTopic = "NONE"
	InstitutionalTopicCompanyInfo   InstitutionalTopic = "COMPANY_INFO"
	InstitutionalTopicContact       InstitutionalTopic = "CONTACT"
	InstitutionalTopicAddress       InstitutionalTopic = "ADDRESS"
	InstitutionalTopicBusinessHours InstitutionalTopic = "BUSINESS_HOURS"
)

// DecisionStrength records the authority of a deterministic decision. Runtime
// arbitration is outside this contract and outside Slice 3.6F-A.
type DecisionStrength string

const (
	DecisionStrengthStrong   DecisionStrength = "STRONG"
	DecisionStrengthWeak     DecisionStrength = "WEAK"
	DecisionStrengthFallback DecisionStrength = "FALLBACK"
)

// TravelQueryMeaningV2 is the local, side-effect-free semantic contract for a
// travel query. It contains no provider, HTTP, persistence, or operational IDs.
type TravelQueryMeaningV2 struct {
	Intent             TravelQueryIntent
	TurnMeaning        TurnMeaning
	Origin             *LocationMeaning
	Destination        *LocationMeaning
	MentionedLocations []LocationMeaning
	DatePreference     DatePreference
	OptionReference    OptionReference
	RouteCoverage      RouteCoverageMeaning
	SeatRequest        SeatRequestMode
	InstitutionalTopic InstitutionalTopic
	NeedsClarification bool
	MissingFields      []string
	Confidence         float64
	Reasons            []string
}
