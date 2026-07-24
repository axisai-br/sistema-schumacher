package chat

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	ErrAgentToolFailed        = errors.New("chat agent tool execution failed")
	ErrAgentToolNotConfigured = errors.New("chat agent tool not configured")

	locationMentionPattern = regexp.MustCompile(`(?i)\b([\p{L}][\p{L}' ]{1,60}?)[/\s,-]+([a-z]{2})\b`)
	routeFromToPattern     = regexp.MustCompile(`(?i)\bde\s+(.+?)\s+para\s+(.+?)(?:\s+(?:em|dia|para)\b|$)`)
	isoDatePattern         = regexp.MustCompile(`\b(\d{4})-(\d{2})-(\d{2})\b`)
	brDatePattern          = regexp.MustCompile(`\b(\d{1,2})/(\d{1,2})(?:/(\d{4}))?\b`)
	qtyPattern             = regexp.MustCompile(`\b(\d{1,2})\s*(?:pessoas?|passagens?|assentos?|lugares?)\b`)
	bookingIDPattern       = regexp.MustCompile(`(?i)\bBK-[A-Z0-9]+\b`)
	reservationCodePattern = regexp.MustCompile(`\b[A-Z0-9]{8}\b`)

	confirmedRouteParenthesesPattern = regexp.MustCompile(`(?i)\b(?:sa[ií]da|saindo|origem)\s+de\s+([\p{L}' -]{2,60})\s*\(([A-Z]{2})\)\s+(?:para|->|→)\s+([\p{L}' -]{2,60})\s*\(([A-Z]{2})\)`)
	confirmedRouteSlashPattern       = regexp.MustCompile(`(?i)\b([\p{L}' -]{2,60})\s*/\s*([A-Z]{2})\s*(?:->|→|para)\s*([\p{L}' -]{2,60})\s*/\s*([A-Z]{2})\b`)
	confirmedRouteSaindoPattern      = regexp.MustCompile(`(?i)(?:^|[[:punct:]])\s*(?:sa[ií]da|saindo|origem)\s+de\s+([\p{L}' -]{2,60})\s+(?:para|->|→)\s+([\p{L}' -]{2,60})(?:\b|$)`)
	confirmedRouteBarePattern        = regexp.MustCompile(`(?i)(?:^|[[:punct:]])\s*de\s+([\p{L}' -]{2,60})\s+(?:para|->|→)\s+([\p{L}' -]{2,60})(?:\b|$)`)
	originDestinationPattern         = regexp.MustCompile(`(?i)\borigem\s+([\p{L}' -]{2,60})\s+destino\s+([\p{L}' -]{2,60})(?:\b|$)`)
	routeOriginSelectionPattern      = regexp.MustCompile(`(?i)(?:^|[[:space:][:punct:]])\s*(?:sa[ií]da|saindo|origem)\s+de\s+([\p{L}' -]{2,60})(?:\s*\(([A-Z]{2})\)|\s*/\s*([A-Z]{2}))?(?:\b|$)`)
	routeQualifierPrefixPattern      = regexp.MustCompile(`(?i)^(?:para|de|origem|saindo de|sa[ií]da de)\s+`)
)

const (
	toolNameAvailabilitySearch       = "availability_search"
	toolNamePricingQuote             = "pricing_quote"
	toolNameBookingLookup            = "booking_lookup"
	toolNameBookingCreate            = "booking_create"
	toolNameBookingCancel            = "booking_cancel"
	toolNameRescheduleLookup         = "reschedule_lookup"
	toolNamePaymentStatus            = "payment_status"
	toolNamePaymentCreate            = "payment_create"
	toolNameDocumentExtract          = "document_extract"
	toolNameBookingPassengerSnapshot = "booking_passenger_snapshot"
)

var supportedStopIDs = map[string]string{
	"chapeco/sc":         "SC_CHAPECO",
	"santa ines/ma":      "MA_SANTA_INES",
	"moncao/ma":          "MA_MONCAO",
	"igarape do meio/ma": "MA_IGARAPE_DO_MEIO",
}

const (
	packageToSantaCatarina = "Pacote p/ Santa Catarina"
	packageToMaranhao      = "Pacote p/ Maranhao"
)

var brazilianStateCodes = map[string]struct{}{
	"AC": {}, "AL": {}, "AP": {}, "AM": {}, "BA": {}, "CE": {}, "DF": {},
	"ES": {}, "GO": {}, "MA": {}, "MT": {}, "MS": {}, "MG": {}, "PA": {},
	"PB": {}, "PR": {}, "PE": {}, "PI": {}, "RJ": {}, "RN": {}, "RS": {},
	"RO": {}, "RR": {}, "SC": {}, "SP": {}, "SE": {}, "TO": {},
}

var scPackageDestinations = map[string]string{
	"fraiburgo":    "Fraiburgo/SC",
	"monte carlo":  "Monte Carlo/SC",
	"videira":      "Videira/SC",
	"campos novos": "Campos Novos/SC",
	"chapeco":      "Chapeco/SC",
	"concordia":    "Concordia/SC",
	"ipumirim":     "Ipumirim/SC",
	"petrolandia":  "Petrolandia/SC",
	"ituporanga":   "Ituporanga/SC",
	"seara":        "Seara/SC",
}

var maPackageDestinations = map[string]string{
	"moncao":          "Moncao/MA",
	"santa ines":      "Santa Ines/MA",
	"igarape do meio": "Igarape do Meio/MA",
}

var canonicalStopIDsByLocation = map[string]string{
	"fraiburgo/sc":    "SC_FRAIBURGO",
	"monte carlo/sc":  "SC_MONTECARLO",
	"videira/sc":      "SC_VIDEIRA",
	"campos novos/sc": "SC_CAMPOSNOVOS",
	"chapeco/sc":      "SC_CHAPECO",
	"concordia/sc":    "SC_CONCORDIA",
	"ipumirim/sc":     "SC_IPUMIRIM",
	"petrolandia/sc":  "SC_PETROLANDIA",
	"ituporanga/sc":   "SC_ITUPORANGA",
	"seara/sc":        "SC_SEARA",

	"moncao/ma":          "MA_MONCAO",
	"santa ines/ma":      "MA_SANTA_INES",
	"igarape do meio/ma": "MA_IGARAPE_DO_MEIO",
}

var routeIDByPackageName = map[string]string{
	"pacote p/maranhao":       "SC_MA",
	"pacote p/santa catarina": "MA_SC",
}

type agentToolContext struct {
	Calls                    []ToolCall
	Availability             *AvailabilitySearchResult
	Pricing                  *PricingQuoteResult
	Booking                  *BookingLookupResult
	BookingCreate            *BookingCreateResult
	BookingCancel            *BookingCancelResult
	Reschedule               *RescheduleAssistResult
	Payments                 *PaymentStatusResult
	PaymentCreate            *PaymentCreateResult
	DocumentExtract          *DocumentExtractResult
	BookingPassengerSnapshot *bookingPassengerSnapshot
}

type inferredRouteContext struct {
	Origin         string
	Destination    string
	PackageName    string
	RouteDirection string
	BroadState     string
}

func (s *Service) canSearchAvailability() bool {
	return s.availability != nil && s.availability.Enabled()
}

func (s *Service) canSearchPricing() bool {
	return s.pricing != nil && s.pricing.Enabled()
}

func (s *Service) canSearchBookings() bool {
	return s.bookings != nil && s.bookings.Enabled()
}

func (s *Service) canCreateBookings() bool {
	return s.bookingCreate != nil && s.bookingCreate.Enabled()
}

func (s *Service) canSearchReschedules() bool {
	return s.reschedules != nil && s.reschedules.Enabled()
}

func (s *Service) canSearchPayments() bool {
	return s.payments != nil && s.payments.Enabled()
}

func (s *Service) canCreatePayments() bool {
	return s.paymentCreate != nil && s.paymentCreate.Enabled()
}

func (s *Service) canCancelBookings() bool {
	return s.bookingCancel != nil && s.bookingCancel.Enabled()
}

func (s *Service) resolveAgentToolContext(ctx context.Context, session Session, history []Message, memory map[string]interface{}) (agentToolContext, error) {
	currentTurn := NormalizeIncomingCustomerText(asString(memory["current_turn_body"]))
	context := agentToolContext{}
	if s.canSearchReschedules() {
		searchInput, ok := parseRescheduleAssistInput(currentTurn, time.Now().UTC())
		if ok {
			startedAt := time.Now().UTC()
			requestPayload := buildRescheduleAssistRequestPayload(searchInput)
			result, err := s.reschedules.Search(ctx, searchInput)
			finishedAt := time.Now().UTC()
			finishedAtPtr := &finishedAt
			if err != nil {
				call, recordErr := s.store.CreateToolCall(ctx, CreateToolCallInput{
					SessionID:      session.ID,
					ToolName:       toolNameRescheduleLookup,
					RequestPayload: requestPayload,
					Status:         "FAILED",
					ErrorCode:      "RESCHEDULE_LOOKUP_ERROR",
					ErrorMessage:   strings.TrimSpace(err.Error()),
					StartedAt:      startedAt,
					FinishedAt:     finishedAtPtr,
				})
				if recordErr != nil {
					return agentToolContext{}, recordErr
				}
				return agentToolContext{Calls: []ToolCall{call}}, fmt.Errorf("%w: %v", ErrAgentToolFailed, err)
			}

			call, err := s.store.CreateToolCall(ctx, CreateToolCallInput{
				SessionID:       session.ID,
				ToolName:        toolNameRescheduleLookup,
				RequestPayload:  requestPayload,
				ResponsePayload: buildRescheduleAssistResponsePayload(result),
				Status:          "COMPLETED",
				StartedAt:       startedAt,
				FinishedAt:      finishedAtPtr,
			})
			if err != nil {
				return agentToolContext{}, err
			}

			context.Calls = append(context.Calls, call)
			context.Reschedule = &result
			return context, nil
		}
	}
	if s.canSearchBookings() {
		searchInput, ok := parseBookingLookupInput(currentTurn)
		if ok {
			startedAt := time.Now().UTC()
			requestPayload := buildBookingLookupRequestPayload(searchInput)
			result, err := s.bookings.Search(ctx, searchInput)
			finishedAt := time.Now().UTC()
			finishedAtPtr := &finishedAt
			if err != nil {
				call, recordErr := s.store.CreateToolCall(ctx, CreateToolCallInput{
					SessionID:      session.ID,
					ToolName:       toolNameBookingLookup,
					RequestPayload: requestPayload,
					Status:         "FAILED",
					ErrorCode:      "BOOKING_LOOKUP_ERROR",
					ErrorMessage:   strings.TrimSpace(err.Error()),
					StartedAt:      startedAt,
					FinishedAt:     finishedAtPtr,
				})
				if recordErr != nil {
					return agentToolContext{}, recordErr
				}
				return agentToolContext{Calls: []ToolCall{call}}, fmt.Errorf("%w: %v", ErrAgentToolFailed, err)
			}

			call, err := s.store.CreateToolCall(ctx, CreateToolCallInput{
				SessionID:       session.ID,
				ToolName:        toolNameBookingLookup,
				RequestPayload:  requestPayload,
				ResponsePayload: buildBookingLookupResponsePayload(result),
				Status:          "COMPLETED",
				StartedAt:       startedAt,
				FinishedAt:      finishedAtPtr,
			})
			if err != nil {
				return agentToolContext{}, err
			}

			context.Calls = append(context.Calls, call)
			context.Booking = &result

			if s.canCancelBookings() && looksLikeBookingCancelIntent(currentTurn) && len(result.Results) > 0 {
				cancelInput, ok := parseBookingCancelInput(history, currentTurn, context.Booking, nil)
				if ok {
					return s.executeBookingCancelTool(ctx, session, context, cancelInput)
				}
			}

			if s.canCreatePayments() && looksLikePaymentCreateIntent(currentTurn) && len(result.Results) > 0 {
				createInput, ok := parsePaymentCreateInput(session, history, currentTurn, context.Booking, nil)
				if ok {
					return s.executePaymentCreateTool(ctx, session, context, createInput)
				}
			}

			if s.canSearchPayments() && looksLikePaymentLookupIntent(currentTurn) && len(result.Results) > 0 {
				paymentInput := PaymentStatusInput{
					BookingID:       strings.TrimSpace(result.Results[0].ID),
					ReservationCode: strings.TrimSpace(result.Results[0].ReservationCode),
					Limit:           5,
				}
				paymentStartedAt := time.Now().UTC()
				paymentRequestPayload := buildPaymentStatusRequestPayload(paymentInput)
				paymentResult, paymentErr := s.payments.Search(ctx, paymentInput)
				paymentFinishedAt := time.Now().UTC()
				paymentFinishedAtPtr := &paymentFinishedAt
				if paymentErr != nil {
					call, recordErr := s.store.CreateToolCall(ctx, CreateToolCallInput{
						SessionID:      session.ID,
						ToolName:       toolNamePaymentStatus,
						RequestPayload: paymentRequestPayload,
						Status:         "FAILED",
						ErrorCode:      "PAYMENT_STATUS_ERROR",
						ErrorMessage:   strings.TrimSpace(paymentErr.Error()),
						StartedAt:      paymentStartedAt,
						FinishedAt:     paymentFinishedAtPtr,
					})
					if recordErr != nil {
						return agentToolContext{}, recordErr
					}
					context.Calls = append(context.Calls, call)
					return context, fmt.Errorf("%w: %v", ErrAgentToolFailed, paymentErr)
				}

				call, err := s.store.CreateToolCall(ctx, CreateToolCallInput{
					SessionID:       session.ID,
					ToolName:        toolNamePaymentStatus,
					RequestPayload:  paymentRequestPayload,
					ResponsePayload: buildPaymentStatusResponsePayload(paymentResult),
					Status:          "COMPLETED",
					StartedAt:       paymentStartedAt,
					FinishedAt:      paymentFinishedAtPtr,
				})
				if err != nil {
					return agentToolContext{}, err
				}
				context.Calls = append(context.Calls, call)
				context.Payments = &paymentResult
			}

			return context, nil
		}
	}

	if context, used, err := s.resolveContextualActionTools(ctx, session, history, currentTurn, context); err != nil || used {
		return context, err
	}

	if !s.canSearchAvailability() {
		return context, nil
	}

	searchInput, ok := parseAvailabilitySearchInput(history, currentTurn, time.Now().UTC())
	if !ok {
		return context, nil
	}

	startedAt := time.Now().UTC()
	requestPayload := buildAvailabilityToolRequestPayload(searchInput)
	result, err := s.availability.Search(ctx, searchInput)
	finishedAt := time.Now().UTC()
	finishedAtPtr := &finishedAt
	if err != nil {
		call, recordErr := s.store.CreateToolCall(ctx, CreateToolCallInput{
			SessionID:      session.ID,
			ToolName:       toolNameAvailabilitySearch,
			RequestPayload: requestPayload,
			Status:         "FAILED",
			ErrorCode:      "AVAILABILITY_SEARCH_ERROR",
			ErrorMessage:   strings.TrimSpace(err.Error()),
			StartedAt:      startedAt,
			FinishedAt:     finishedAtPtr,
		})
		if recordErr != nil {
			return agentToolContext{}, recordErr
		}
		return agentToolContext{Calls: []ToolCall{call}}, fmt.Errorf("%w: %v", ErrAgentToolFailed, err)
	}

	call, err := s.store.CreateToolCall(ctx, CreateToolCallInput{
		SessionID:       session.ID,
		ToolName:        toolNameAvailabilitySearch,
		RequestPayload:  requestPayload,
		ResponsePayload: buildAvailabilityToolResponsePayload(result),
		Status:          "COMPLETED",
		StartedAt:       startedAt,
		FinishedAt:      finishedAtPtr,
	})
	if err != nil {
		return agentToolContext{}, err
	}
	context.Calls = append(context.Calls, call)
	context.Availability = &result

	if s.canSearchPricing() && looksLikePricingQuoteIntent(currentTurn) && len(result.Results) > 0 {
		pricingInput := buildPricingQuoteInput(result)
		pricingStartedAt := time.Now().UTC()
		pricingRequestPayload := buildPricingQuoteRequestPayload(pricingInput)
		pricingResult, pricingErr := s.pricing.Search(ctx, pricingInput)
		pricingFinishedAt := time.Now().UTC()
		pricingFinishedAtPtr := &pricingFinishedAt
		if pricingErr != nil {
			call, recordErr := s.store.CreateToolCall(ctx, CreateToolCallInput{
				SessionID:      session.ID,
				ToolName:       toolNamePricingQuote,
				RequestPayload: pricingRequestPayload,
				Status:         "FAILED",
				ErrorCode:      "PRICING_QUOTE_ERROR",
				ErrorMessage:   strings.TrimSpace(pricingErr.Error()),
				StartedAt:      pricingStartedAt,
				FinishedAt:     pricingFinishedAtPtr,
			})
			if recordErr != nil {
				return agentToolContext{}, recordErr
			}
			context.Calls = append(context.Calls, call)
			return context, fmt.Errorf("%w: %v", ErrAgentToolFailed, pricingErr)
		}

		call, err := s.store.CreateToolCall(ctx, CreateToolCallInput{
			SessionID:       session.ID,
			ToolName:        toolNamePricingQuote,
			RequestPayload:  pricingRequestPayload,
			ResponsePayload: buildPricingQuoteResponsePayload(pricingResult),
			Status:          "COMPLETED",
			StartedAt:       pricingStartedAt,
			FinishedAt:      pricingFinishedAtPtr,
		})
		if err != nil {
			return agentToolContext{}, err
		}
		context.Calls = append(context.Calls, call)
		context.Pricing = &pricingResult
	}

	if s.canCreateBookings() {
		createInput, ok := parseBookingCreateInputWithPassengerState(
			session,
			history,
			currentTurn,
			context.Availability,
			passengerClarificationStateV1OrDefault(session),
		)
		if ok {
			startedAt := time.Now().UTC()
			requestPayload := buildBookingCreateRequestPayload(createInput)
			createResult, createErr := s.bookingCreate.Create(ctx, createInput)
			finishedAt := time.Now().UTC()
			finishedAtPtr := &finishedAt
			if createErr != nil {
				call, recordErr := s.store.CreateToolCall(ctx, CreateToolCallInput{
					SessionID:      session.ID,
					ToolName:       toolNameBookingCreate,
					RequestPayload: requestPayload,
					Status:         "FAILED",
					ErrorCode:      "BOOKING_CREATE_ERROR",
					ErrorMessage:   strings.TrimSpace(createErr.Error()),
					StartedAt:      startedAt,
					FinishedAt:     finishedAtPtr,
				})
				if recordErr != nil {
					return agentToolContext{}, recordErr
				}
				context.Calls = append(context.Calls, call)
				return context, fmt.Errorf("%w: %v", ErrAgentToolFailed, createErr)
			}

			call, err := s.store.CreateToolCall(ctx, CreateToolCallInput{
				SessionID:       session.ID,
				ToolName:        toolNameBookingCreate,
				RequestPayload:  requestPayload,
				ResponsePayload: buildBookingCreateResponsePayload(createResult),
				Status:          "COMPLETED",
				StartedAt:       startedAt,
				FinishedAt:      finishedAtPtr,
			})
			if err != nil {
				return agentToolContext{}, err
			}
			context.Calls = append(context.Calls, call)
			context.BookingCreate = &createResult
		}
	}

	return context, nil
}

func (s *Service) resolveContextualActionTools(ctx context.Context, session Session, history []Message, currentTurn string, context agentToolContext) (agentToolContext, bool, error) {
	if s.canCreatePayments() {
		createInput, ok := parsePaymentCreateInput(session, history, currentTurn, nil, nil)
		if ok {
			updated, err := s.executePaymentCreateTool(ctx, session, context, createInput)
			return updated, true, err
		}
	}
	if s.canCancelBookings() {
		cancelInput, ok := parseBookingCancelInput(history, currentTurn, nil, nil)
		if ok {
			updated, err := s.executeBookingCancelTool(ctx, session, context, cancelInput)
			return updated, true, err
		}
	}
	if s.canCreateBookings() {
		passengerState := passengerClarificationStateV1OrDefault(session)
		createInput, ok := parseBookingCreateFromDocumentConfirmationWithPassengerState(session, history, currentTurn, passengerState)
		if ok {
			updated, err := s.executeBookingCreateTool(ctx, session, context, createInput)
			return updated, true, err
		}
		createInput, ok = parseBookingCreateInputWithPassengerState(session, history, currentTurn, nil, passengerState)
		if ok {
			updated, err := s.executeBookingCreateTool(ctx, session, context, createInput)
			return updated, true, err
		}
	}
	return context, false, nil
}

func (s *Service) executeBookingCreateTool(ctx context.Context, session Session, context agentToolContext, input BookingCreateInput) (agentToolContext, error) {
	startedAt := time.Now().UTC()
	requestPayload := buildBookingCreateRequestPayload(input)
	createResult, createErr := s.bookingCreate.Create(ctx, input)
	finishedAt := time.Now().UTC()
	finishedAtPtr := &finishedAt
	if createErr != nil {
		call, recordErr := s.store.CreateToolCall(ctx, CreateToolCallInput{
			SessionID:      session.ID,
			ToolName:       toolNameBookingCreate,
			RequestPayload: requestPayload,
			Status:         "FAILED",
			ErrorCode:      "BOOKING_CREATE_ERROR",
			ErrorMessage:   strings.TrimSpace(createErr.Error()),
			StartedAt:      startedAt,
			FinishedAt:     finishedAtPtr,
		})
		if recordErr != nil {
			return agentToolContext{}, recordErr
		}
		context.Calls = append(context.Calls, call)
		return context, fmt.Errorf("%w: %v", ErrAgentToolFailed, createErr)
	}

	call, err := s.store.CreateToolCall(ctx, CreateToolCallInput{
		SessionID:       session.ID,
		ToolName:        toolNameBookingCreate,
		RequestPayload:  requestPayload,
		ResponsePayload: buildBookingCreateResponsePayload(createResult),
		Status:          "COMPLETED",
		StartedAt:       startedAt,
		FinishedAt:      finishedAtPtr,
	})
	if err != nil {
		return agentToolContext{}, err
	}
	context.Calls = append(context.Calls, call)
	context.BookingCreate = &createResult
	return context, nil
}

func (s *Service) executePaymentCreateTool(ctx context.Context, session Session, context agentToolContext, input PaymentCreateInput) (agentToolContext, error) {
	startedAt := time.Now().UTC()
	requestPayload := buildPaymentCreateRequestPayload(input)
	createResult, createErr := s.paymentCreate.Create(ctx, input)
	finishedAt := time.Now().UTC()
	finishedAtPtr := &finishedAt
	if createErr != nil {
		call, recordErr := s.store.CreateToolCall(ctx, CreateToolCallInput{
			SessionID:      session.ID,
			ToolName:       toolNamePaymentCreate,
			RequestPayload: requestPayload,
			Status:         "FAILED",
			ErrorCode:      "PAYMENT_CREATE_ERROR",
			ErrorMessage:   strings.TrimSpace(createErr.Error()),
			StartedAt:      startedAt,
			FinishedAt:     finishedAtPtr,
		})
		if recordErr != nil {
			return agentToolContext{}, recordErr
		}
		context.Calls = append(context.Calls, call)
		return context, fmt.Errorf("%w: %v", ErrAgentToolFailed, createErr)
	}

	call, err := s.store.CreateToolCall(ctx, CreateToolCallInput{
		SessionID:       session.ID,
		ToolName:        toolNamePaymentCreate,
		RequestPayload:  requestPayload,
		ResponsePayload: buildPaymentCreateResponsePayload(createResult),
		Status:          "COMPLETED",
		StartedAt:       startedAt,
		FinishedAt:      finishedAtPtr,
	})
	if err != nil {
		return agentToolContext{}, err
	}
	context.Calls = append(context.Calls, call)
	context.PaymentCreate = &createResult
	return context, nil
}

func (s *Service) executeBookingCancelTool(ctx context.Context, session Session, context agentToolContext, input BookingCancelInput) (agentToolContext, error) {
	startedAt := time.Now().UTC()
	requestPayload := buildBookingCancelRequestPayload(input)
	cancelResult, cancelErr := s.bookingCancel.Cancel(ctx, input)
	finishedAt := time.Now().UTC()
	finishedAtPtr := &finishedAt
	if cancelErr != nil {
		call, recordErr := s.store.CreateToolCall(ctx, CreateToolCallInput{
			SessionID:      session.ID,
			ToolName:       toolNameBookingCancel,
			RequestPayload: requestPayload,
			Status:         "FAILED",
			ErrorCode:      "BOOKING_CANCEL_ERROR",
			ErrorMessage:   strings.TrimSpace(cancelErr.Error()),
			StartedAt:      startedAt,
			FinishedAt:     finishedAtPtr,
		})
		if recordErr != nil {
			return agentToolContext{}, recordErr
		}
		context.Calls = append(context.Calls, call)
		return context, fmt.Errorf("%w: %v", ErrAgentToolFailed, cancelErr)
	}

	call, err := s.store.CreateToolCall(ctx, CreateToolCallInput{
		SessionID:       session.ID,
		ToolName:        toolNameBookingCancel,
		RequestPayload:  requestPayload,
		ResponsePayload: buildBookingCancelResponsePayload(cancelResult),
		Status:          "COMPLETED",
		StartedAt:       startedAt,
		FinishedAt:      finishedAtPtr,
	})
	if err != nil {
		return agentToolContext{}, err
	}
	context.Calls = append(context.Calls, call)
	context.BookingCancel = &cancelResult
	return context, nil
}

func parseAvailabilitySearchInput(history []Message, text string, observedAt time.Time) (AvailabilitySearchInput, bool) {
	body := NormalizeIncomingCustomerText(text)
	if body == "" {
		return AvailabilitySearchInput{}, false
	}
	folded := strings.Join(strings.Fields(foldChatText(body)), " ")
	if looksLikeNegatedAvailabilitySelection(folded) {
		return AvailabilitySearchInput{}, false
	}
	if input, ok := parseSupportedCityPairAvailabilityInput(body, observedAt); ok {
		return enrichAvailabilitySearchInput(input), true
	}
	historyContext := inferLatestRouteContextFromHistory(history)

	if input, ok := parseAvailabilityDateSelectionInput(history, body, observedAt); ok {
		return enrichAvailabilitySearchInput(input), true
	}

	if input, ok := parseDirectAvailabilitySearchInput(body, observedAt); ok {
		return enrichAvailabilitySearchInput(input), true
	}

	if input, ok := parseOriginAnswerAvailabilitySearchInput(history, body, observedAt); ok {
		return enrichAvailabilitySearchInput(input), true
	}

	currentContext := inferRouteContextFromText(body)
	if looksLikeGenericAvailabilityQuestion(body) &&
		strings.TrimSpace(currentContext.Origin) == "" &&
		strings.TrimSpace(currentContext.Destination) == "" &&
		strings.TrimSpace(currentContext.PackageName) == "" &&
		strings.TrimSpace(currentContext.BroadState) == "" {
		if origin, destination, found := lastConfirmedRouteFromHistory(history); found {
			input := AvailabilitySearchInput{
				Origin:      origin,
				Destination: destination,
				PackageName: historyContext.PackageName,
				TripDate:    extractTripDate(body, observedAt),
				Qty:         extractPassengerQuantity(body),
				Limit:       8,
			}
			if strings.TrimSpace(input.PackageName) == "" {
				input.PackageName = packageNameForRouteDirection(inferRouteDirectionFromDestination(destination))
			}
			if input.Qty <= 0 {
				input.Qty = 1
			}
			return input, true
		}
		return AvailabilitySearchInput{}, false
	}

	if input, ok := parseContextualAvailabilitySearchInput(historyContext, body, observedAt); ok {
		return input, true
	}

	if !looksLikePackageLevelAvailabilityIntent(body) {
		return AvailabilitySearchInput{}, false
	}

	context := mergeInferredRouteContext(
		inferConversationTurnRouteContext(body, historyContext),
		historyContext,
	)
	if strings.TrimSpace(context.PackageName) == "" {
		return AvailabilitySearchInput{}, false
	}

	input := AvailabilitySearchInput{
		PackageName: context.PackageName,
		TripDate:    extractTripDate(body, observedAt),
		Qty:         extractPassengerQuantity(body),
		Limit:       8,
	}
	if input.Qty <= 0 {
		input.Qty = 1
	}
	return input, true
}

func parseVerifyAllOptionsAvailabilityInput(history []Message, observedAt time.Time) (AvailabilitySearchInput, string) {
	context := inferVerifyAllOptionsRouteContext(history)
	input := AvailabilitySearchInput{
		Origin:      strings.TrimSpace(context.Origin),
		Destination: strings.TrimSpace(context.Destination),
		PackageName: strings.TrimSpace(context.PackageName),
		TripDate:    latestTripDateFromHistory(history, observedAt),
		Qty:         1,
		Limit:       8,
	}
	if input.PackageName == "" {
		input.PackageName = packageNameForRouteDirection(inferRouteDirectionFromDestination(input.Destination))
	}
	input = enrichAvailabilitySearchInput(input)
	switch {
	case strings.TrimSpace(input.Destination) == "" && strings.TrimSpace(input.PackageName) == "":
		return input, "destination"
	case strings.TrimSpace(input.Origin) == "":
		return input, "origin"
	case strings.TrimSpace(input.Destination) == "":
		return input, "destination"
	default:
		return input, ""
	}
}

func inferVerifyAllOptionsRouteContext(history []Message) inferredRouteContext {
	context := inferLatestRouteContextFromHistory(history)
	for i := 0; i < len(history); i++ {
		body := messageTurnText(history[i])
		if strings.TrimSpace(body) == "" {
			continue
		}
		if context.Destination == "" {
			if city, ok := findSingleSupportedCityInText(body, scPackageDestinations); ok {
				context.Destination = city
				context.RouteDirection = "TO_SC"
				context.PackageName = packageToSantaCatarina
			} else if city, ok := findSingleSupportedCityInText(body, maPackageDestinations); ok {
				context.Destination = city
				context.RouteDirection = "TO_MA"
				context.PackageName = packageToMaranhao
			}
		}
		if context.RouteDirection == "" && context.Destination != "" {
			context.RouteDirection = inferRouteDirectionFromDestination(context.Destination)
		}
		if context.PackageName == "" {
			context.PackageName = packageNameForRouteDirection(context.RouteDirection)
		}
		if context.Origin == "" {
			switch context.RouteDirection {
			case "TO_SC":
				if city, ok := findSingleSupportedCityInText(body, maPackageDestinations); ok {
					context.Origin = city
				}
			case "TO_MA":
				if city, ok := findSingleSupportedCityInText(body, scPackageDestinations); ok {
					context.Origin = city
				}
			}
		}
	}
	return context
}

func latestTripDateFromHistory(history []Message, observedAt time.Time) *time.Time {
	for i := len(history) - 1; i >= 0; i-- {
		if tripDate := extractTripDate(messageTurnText(history[i]), observedAt); tripDate != nil {
			return tripDate
		}
	}
	return nil
}

func parseOriginAnswerAvailabilitySearchInput(history []Message, text string, observedAt time.Time) (AvailabilitySearchInput, bool) {
	originState, questionIndex, ok := lastAssistantOriginQuestionState(history)
	if !ok {
		return AvailabilitySearchInput{}, false
	}
	origin := inferOriginFromCurrentTurnAnswer(history, text)
	if origin == "" || !strings.HasSuffix(strings.ToUpper(origin), "/"+originState) {
		return AvailabilitySearchInput{}, false
	}

	priorContext := inferLatestRouteContextFromHistory(history[:questionIndex])
	destination := strings.TrimSpace(priorContext.Destination)
	if destination == "" {
		destination = inferLatestDestinationBeforeOriginQuestion(history)
	}
	if destination == "" {
		destination = inferDestinationFromOriginQuestion(history[questionIndex].Body, originState)
	}
	if destination == "" || strings.HasSuffix(strings.ToUpper(destination), "/"+originState) {
		return AvailabilitySearchInput{}, false
	}

	routeDirection := inferRouteDirectionFromDestination(destination)
	packageName := packageNameForRouteDirection(routeDirection)
	if packageName == "" {
		packageName = priorContext.PackageName
	}
	if packageName == "" {
		return AvailabilitySearchInput{}, false
	}

	input := AvailabilitySearchInput{
		Origin:      origin,
		Destination: destination,
		PackageName: packageName,
		TripDate:    extractTripDate(text, observedAt),
		Qty:         extractPassengerQuantity(text),
		Limit:       8,
	}
	if input.Qty <= 0 {
		input.Qty = 1
	}
	return enrichAvailabilitySearchInput(input), true
}

func parseAvailabilityDateSelectionInput(history []Message, text string, observedAt time.Time) (AvailabilitySearchInput, bool) {
	folded := strings.Join(strings.Fields(foldChatText(text)), " ")
	if looksLikeNegatedAvailabilitySelection(folded) {
		return AvailabilitySearchInput{}, false
	}
	latest, sourceHistoryIndex, ok := findLatestAvailabilityContextWithSource(history)
	if !ok || len(latest.Results) == 0 {
		return AvailabilitySearchInput{}, false
	}
	selected, ok := resolveAvailabilityDateSelection(latest.Results, text, observedAt)
	if !ok {
		return AvailabilitySearchInput{}, false
	}
	if latestAvailabilitySelectionEvidence(history).rejectsAvailabilityOptionForPrompt(
		availabilityPromptSourceMessageIDAtHistoryIndex(history, sourceHistoryIndex),
		sourceHistoryIndex,
		0,
		selected.TripDate,
	) {
		return AvailabilitySearchInput{}, false
	}

	tripDate := parseISODatePtr(selected.TripDate)
	if tripDate == nil {
		tripDate = extractTripDate(text, observedAt)
	}
	origin := strings.TrimSpace(selected.OriginDisplayName)
	if origin == "" {
		origin = strings.TrimSpace(latest.Filter.Origin)
	}
	destination := strings.TrimSpace(selected.DestinationDisplayName)
	if destination == "" {
		destination = strings.TrimSpace(latest.Filter.Destination)
	}
	packageName := strings.TrimSpace(selected.PackageName)
	if packageName == "" {
		packageName = strings.TrimSpace(latest.Filter.PackageName)
	}
	if packageName == "" {
		packageName = packageNameForRouteDirection(inferRouteDirectionFromDestination(destination))
	}
	if origin == "" || destination == "" || packageName == "" || tripDate == nil {
		return AvailabilitySearchInput{}, false
	}

	input := AvailabilitySearchInput{
		Origin:      origin,
		Destination: destination,
		PackageName: packageName,
		TripDate:    tripDate,
		Qty:         latest.Filter.Qty,
		Limit:       8,
	}
	if input.Qty <= 0 {
		input.Qty = extractPassengerQuantity(text)
	}
	if input.Qty <= 0 {
		input.Qty = 1
	}
	return enrichAvailabilitySearchInput(input), true
}

func resolveAvailabilityDateSelection(results []AvailabilitySearchItem, text string, observedAt time.Time) (AvailabilitySearchItem, bool) {
	if len(results) == 0 {
		return AvailabilitySearchItem{}, false
	}
	body := NormalizeIncomingCustomerText(text)
	if body == "" {
		return AvailabilitySearchItem{}, false
	}

	if selectedDate := extractTripDate(body, observedAt); selectedDate != nil {
		for _, item := range results {
			itemDate := parseISODatePtr(item.TripDate)
			if sameCalendarDate(itemDate, selectedDate) {
				return item, true
			}
		}
	}
	if day := extractDayOfMonthDateSelection(body); day > 0 {
		for _, item := range results {
			itemDate := parseISODatePtr(item.TripDate)
			if itemDate != nil && itemDate.UTC().Day() == day {
				return item, true
			}
		}
	}
	if index := extractSelectedOptionIndex(body); index > 0 {
		if index <= len(results) {
			return results[index-1], true
		}
		return AvailabilitySearchItem{}, false
	}
	return AvailabilitySearchItem{}, false
}

func parseContextualAvailabilitySearchInput(historyContext inferredRouteContext, text string, observedAt time.Time) (AvailabilitySearchInput, bool) {
	body := NormalizeIncomingCustomerText(text)
	if body == "" {
		return AvailabilitySearchInput{}, false
	}

	currentContext := inferConversationTurnRouteContext(body, historyContext)
	merged := mergeInferredRouteContext(currentContext, historyContext)
	if strings.TrimSpace(merged.PackageName) == "" || strings.TrimSpace(merged.Destination) == "" {
		return AvailabilitySearchInput{}, false
	}
	if strings.TrimSpace(merged.Origin) == "" {
		return AvailabilitySearchInput{}, false
	}

	tripDate := extractTripDate(body, observedAt)
	destinationContextPresent := currentContext.Destination != ""
	if !destinationContextPresent && tripDate == nil && !looksLikeShortDateFollowUp(body) {
		return AvailabilitySearchInput{}, false
	}

	input := AvailabilitySearchInput{
		Origin:      merged.Origin,
		Destination: merged.Destination,
		PackageName: merged.PackageName,
		TripDate:    tripDate,
		Qty:         extractPassengerQuantity(body),
		Limit:       8,
	}
	if input.Qty <= 0 {
		input.Qty = 1
	}
	return enrichAvailabilitySearchInput(input), true
}

func parseSupportedCityPairAvailabilityInput(text string, observedAt time.Time) (AvailabilitySearchInput, bool) {
	body := NormalizeIncomingCustomerText(text)
	if body == "" {
		return AvailabilitySearchInput{}, false
	}

	scCity, scIndex, okSC := findSingleSupportedCityMention(body, scPackageDestinations)
	maCity, maIndex, okMA := findSingleSupportedCityMention(body, maPackageDestinations)
	if !okSC || !okMA || scIndex == maIndex {
		return AvailabilitySearchInput{}, false
	}

	input := AvailabilitySearchInput{
		TripDate: extractTripDate(body, observedAt),
		Qty:      extractPassengerQuantity(body),
		Limit:    8,
	}
	if input.Qty <= 0 {
		input.Qty = 1
	}
	if scIndex < maIndex {
		input.Origin = scCity
		input.Destination = maCity
		input.PackageName = packageToMaranhao
	} else {
		input.Origin = maCity
		input.Destination = scCity
		input.PackageName = packageToSantaCatarina
	}
	return enrichAvailabilitySearchInput(input), true
}

func findSingleSupportedCityMention(text string, candidates map[string]string) (string, int, bool) {
	folded := foldChatText(text)
	if folded == "" {
		return "", -1, false
	}

	matchCount := 0
	selected := ""
	selectedIndex := -1
	for key, canonical := range candidates {
		needle := " " + foldChatDestinationKey(key) + " "
		index := strings.Index(folded, needle)
		if index < 0 {
			continue
		}
		matchCount++
		selected = canonical
		selectedIndex = index
	}
	if matchCount != 1 {
		return "", -1, false
	}
	return selected, selectedIndex, true
}

func parseDirectAvailabilitySearchInput(text string, observedAt time.Time) (AvailabilitySearchInput, bool) {
	body := NormalizeIncomingCustomerText(text)
	if body == "" {
		return AvailabilitySearchInput{}, false
	}
	if !looksLikeExplicitRouteQuery(body) {
		locations := extractCanonicalLocations(body)
		if len(locations) < 2 {
			locations = extractSupportedPackageLocationsInOrder(body)
		}
		if len(locations) < 2 {
			return AvailabilitySearchInput{}, false
		}
		folded := foldChatText(body)
		if !strings.Contains(folded, " para ") && !strings.Contains(body, "->") && !strings.Contains(body, "→") {
			return AvailabilitySearchInput{}, false
		}

		origin := locations[0]
		destination := locations[1]
		if origin == "" || destination == "" || strings.EqualFold(origin, destination) {
			return AvailabilitySearchInput{}, false
		}

		input := AvailabilitySearchInput{
			Origin:      origin,
			Destination: destination,
			TripDate:    extractTripDate(body, observedAt),
			Qty:         extractPassengerQuantity(body),
			Limit:       5,
		}
		if input.Qty <= 0 {
			input.Qty = 1
		}
		return enrichAvailabilitySearchInput(input), true
	}

	origin, destination, ok := extractExplicitRouteFromText(body)
	if !ok {
		return AvailabilitySearchInput{}, false
	}
	if origin == "" || destination == "" || strings.EqualFold(origin, destination) {
		return AvailabilitySearchInput{}, false
	}

	input := AvailabilitySearchInput{
		Origin:      origin,
		Destination: destination,
		TripDate:    extractTripDate(body, observedAt),
		Qty:         extractPassengerQuantity(body),
		Limit:       5,
	}
	if input.Qty <= 0 {
		input.Qty = 1
	}
	return input, true
}

func looksLikePackageLevelAvailabilityIntent(text string) bool {
	folded := foldChatText(text)
	if folded == "" {
		return false
	}
	return looksLikeShortDateFollowUp(text) || looksLikeBroadStateScheduleLookup(text)
}

func looksLikeGenericAvailabilityQuestion(text string) bool {
	folded := foldChatText(text)
	if folded == "" {
		return false
	}

	patterns := []string{
		"quais datas disponiveis",
		"quais datas",
		"tem data",
		"tem datas",
		"quais horarios",
		"quando sai",
		"quando tem",
		"tem vaga",
		"tem vagas",
		"tem disponibilidade",
	}
	for _, pattern := range patterns {
		if strings.Contains(folded, pattern) {
			return true
		}
	}
	return false
}

func looksLikeExplicitRouteQuery(text string) bool {
	_, _, ok := extractExplicitRouteFromText(text)
	return ok
}

func extractExplicitRouteFromText(text string) (string, string, bool) {
	body := strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if body == "" {
		return "", "", false
	}

	if match := confirmedRouteParenthesesPattern.FindStringSubmatch(body); len(match) == 5 {
		origin := normalizeRouteEndpoint(match[1], match[2])
		destination := normalizeRouteEndpoint(match[3], match[4])
		if origin != "" && destination != "" {
			return origin, destination, true
		}
	}
	if match := confirmedRouteSlashPattern.FindStringSubmatch(body); len(match) == 5 {
		origin := normalizeRouteEndpoint(match[1], match[2])
		destination := normalizeRouteEndpoint(match[3], match[4])
		if origin != "" && destination != "" {
			return origin, destination, true
		}
	}
	if match := originDestinationPattern.FindStringSubmatch(body); len(match) == 3 {
		origin := normalizeRouteEndpoint(match[1], "")
		destination := normalizeRouteEndpoint(match[2], "")
		if origin != "" && destination != "" {
			return origin, destination, true
		}
	}
	if match := confirmedRouteSaindoPattern.FindStringSubmatch(body); len(match) == 3 {
		origin := normalizeRouteEndpoint(match[1], "")
		destination := normalizeRouteEndpoint(match[2], "")
		if origin != "" && destination != "" {
			return origin, destination, true
		}
	}
	if match := confirmedRouteBarePattern.FindStringSubmatch(body); len(match) == 3 {
		origin := normalizeRouteEndpoint(match[1], "")
		destination := normalizeRouteEndpoint(match[2], "")
		if origin != "" && destination != "" {
			return origin, destination, true
		}
	}
	return "", "", false
}

func looksLikeAvailabilityIntent(text string, locationCount int) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return false
	}

	keywords := []string{
		"dispon",
		"vaga",
		"assento",
		"lugar",
		"horario",
		"horario",
		"saida",
		"saída",
		"valor",
		"preco",
		"preço",
		"passagem",
	}
	for _, keyword := range keywords {
		if strings.Contains(lower, keyword) {
			return true
		}
	}

	return locationCount >= 2 && strings.Contains(lower, " para ")
}

func looksLikePricingQuoteIntent(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return false
	}

	keywords := []string{
		"valor",
		"preco",
		"preço",
		"quanto",
		"custa",
		"cotacao",
		"cotação",
		"tarifa",
	}
	for _, keyword := range keywords {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}

func extractCanonicalLocations(text string) []string {
	text = NormalizeIncomingCustomerText(text)
	matches := locationMentionPattern.FindAllStringSubmatch(text, -1)
	seen := make(map[string]struct{}, len(matches))
	items := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) != 3 {
			continue
		}
		location := normalizeLocationDisplayName(match[0])
		if location == "" {
			continue
		}
		if _, ok := seen[strings.ToUpper(location)]; ok {
			continue
		}
		seen[strings.ToUpper(location)] = struct{}{}
		items = append(items, location)
	}
	return items
}

func extractSupportedPackageLocationsInOrder(text string) []string {
	folded := " " + strings.Join(strings.Fields(foldChatText(text)), " ") + " "
	type match struct {
		index   int
		display string
	}
	matches := make([]match, 0, len(scPackageDestinations)+len(maPackageDestinations))
	for key, display := range scPackageDestinations {
		if index := strings.Index(folded, " "+key+" "); index >= 0 {
			matches = append(matches, match{index: index, display: display})
		}
	}
	for key, display := range maPackageDestinations {
		if index := strings.Index(folded, " "+key+" "); index >= 0 {
			matches = append(matches, match{index: index, display: display})
		}
	}
	if len(matches) == 0 {
		return nil
	}
	for i := 0; i < len(matches)-1; i++ {
		for j := i + 1; j < len(matches); j++ {
			if matches[j].index < matches[i].index {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}
	seen := map[string]struct{}{}
	locations := make([]string, 0, len(matches))
	for _, item := range matches {
		key := strings.ToUpper(item.display)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		locations = append(locations, item.display)
	}
	return locations
}

func inferLatestRouteContextFromHistory(history []Message) inferredRouteContext {
	context := inferredRouteContext{}
	for _, message := range history {
		body := strings.TrimSpace(message.Body)
		if body == "" {
			continue
		}
		turnContext := inferConversationTurnRouteContext(body, context)
		context = mergeInferredRouteContext(turnContext, context)
	}
	return context
}

func lastAssistantAskedOriginInMaranhao(history []Message) bool {
	state, _, ok := lastAssistantOriginQuestionState(history)
	return ok && state == "MA"
}

func lastAssistantAskedOriginInSantaCatarina(history []Message) bool {
	state, _, ok := lastAssistantOriginQuestionState(history)
	return ok && state == "SC"
}

func lastAssistantOriginQuestionState(history []Message) (string, int, bool) {
	for i := len(history) - 1; i >= 0; i-- {
		message := history[i]
		if !isAssistantRouteQuestionMessage(message) {
			continue
		}
		switch {
		case assistantAskedOriginInState(message.Body, "MA"):
			return "MA", i, true
		case assistantAskedOriginInState(message.Body, "SC"):
			return "SC", i, true
		}
	}
	return "", -1, false
}

func isAssistantRouteQuestionMessage(message Message) bool {
	direction := strings.TrimSpace(message.Direction)
	return strings.EqualFold(direction, "OUTBOUND") || strings.EqualFold(direction, "BOT")
}

func assistantAskedOriginInState(text string, state string) bool {
	folded := foldChatText(text)
	if folded == "" {
		return false
	}

	hasState := false
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "MA":
		hasState = strings.Contains(folded, " maranhao ") || strings.Contains(folded, " ma ")
	case "SC":
		hasState = strings.Contains(folded, " santa catarina ") || strings.Contains(folded, " sc ")
	}
	if !hasState || !strings.Contains(folded, " cidade ") {
		return false
	}
	return strings.Contains(folded, " sair ") ||
		strings.Contains(folded, " sai ") ||
		strings.Contains(folded, " saida ") ||
		strings.Contains(folded, " saindo ") ||
		strings.Contains(folded, " origem ")
}

func inferOriginFromCurrentTurnAnswer(history []Message, currentTurn string) string {
	state, _, ok := lastAssistantOriginQuestionState(history)
	if !ok {
		return ""
	}
	switch state {
	case "MA":
		return inferKnownLocationFromCandidates(currentTurn, maPackageDestinations)
	case "SC":
		return inferKnownLocationFromCandidates(currentTurn, scPackageDestinations)
	default:
		return ""
	}
}

func inferLatestDestinationBeforeOriginQuestion(history []Message) string {
	_, questionIndex, ok := lastAssistantOriginQuestionState(history)
	if !ok {
		return ""
	}
	context := inferLatestRouteContextFromHistory(history[:questionIndex])
	return context.Destination
}

func inferDestinationFromOriginQuestion(text string, originState string) string {
	switch strings.ToUpper(strings.TrimSpace(originState)) {
	case "MA":
		destination, _ := findSingleSupportedCityInText(text, scPackageDestinations)
		return destination
	case "SC":
		destination, _ := findSingleSupportedCityInText(text, maPackageDestinations)
		return destination
	default:
		return ""
	}
}

func lastConfirmedRouteFromHistory(history []Message) (string, string, bool) {
	for i := len(history) - 1; i >= 0; i-- {
		message := history[i]
		if !strings.EqualFold(strings.TrimSpace(message.Direction), "OUTBOUND") &&
			!strings.EqualFold(strings.TrimSpace(message.Direction), "BOT") {
			continue
		}
		if origin, destination, ok := parseConfirmedRouteMessage(message.Body); ok {
			return origin, destination, true
		}
		if match := routeFromToPattern.FindStringSubmatch(message.Body); len(match) == 3 {
			origin := normalizeLocationDisplayName(match[1])
			destination := normalizeLocationDisplayName(match[2])
			if origin != "" && destination != "" {
				return origin, destination, true
			}
		}
		context := inferRouteContextFromText(message.Body)
		if context.Origin != "" && context.Destination != "" {
			return context.Origin, context.Destination, true
		}
		origin, destination, ok := extractExplicitRouteFromText(message.Body)
		if ok {
			return origin, destination, true
		}
	}
	return "", "", false
}

func parseConfirmedRouteMessage(text string) (string, string, bool) {
	body := strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if body == "" {
		return "", "", false
	}

	folded := foldChatText(body)
	if !strings.Contains(folded, " saida de ") && !strings.Contains(folded, " saindo de ") && !strings.Contains(folded, " origem de ") {
		return "", "", false
	}
	lower := strings.ToLower(body)
	paraIndex := strings.Index(lower, " para ")
	if paraIndex < 0 {
		return "", "", false
	}
	before := lower[:paraIndex]
	deIndex := strings.LastIndex(before, " de ")
	if deIndex < 0 {
		return "", "", false
	}

	originPart := strings.TrimSpace(body[deIndex+4 : paraIndex])
	destinationPart := strings.TrimSpace(body[paraIndex+6:])
	if cut := strings.IndexAny(destinationPart, ".!?"); cut >= 0 {
		destinationPart = strings.TrimSpace(destinationPart[:cut])
	}

	origin := normalizeLocationDisplayName(originPart)
	destination := normalizeLocationDisplayName(destinationPart)
	if origin == "" || destination == "" {
		return "", "", false
	}
	return origin, destination, true
}

func inferRouteContextFromText(text string) inferredRouteContext {
	body := NormalizeIncomingCustomerText(text)
	if body == "" {
		return inferredRouteContext{}
	}

	context := inferredRouteContext{}
	if origin, destination, ok := extractExplicitRouteFromText(body); ok {
		context.Origin = origin
		context.Destination = destination
	}
	if context.Origin == "" {
		if match := routeOriginSelectionPattern.FindStringSubmatch(body); len(match) >= 2 {
			uf := ""
			if len(match) >= 3 && strings.TrimSpace(match[2]) != "" {
				uf = match[2]
			}
			if len(match) >= 4 && strings.TrimSpace(match[3]) != "" {
				uf = match[3]
			}
			if origin := normalizeRouteEndpoint(match[1], uf); origin != "" {
				context.Origin = origin
			}
		}
	}
	if context.Destination == "" {
		if destination, ok := inferExplicitTravelDestination(body); ok {
			context.Destination = normalizeSupportedPackageLocation(destination)
		}
	}
	if context.Destination != "" {
		context.RouteDirection = inferRouteDirectionFromDestination(context.Destination)
		context.PackageName = packageNameForRouteDirection(context.RouteDirection)
	}
	if context.PackageName == "" {
		foldedContext := inferRouteContextFromFoldedText(foldChatText(body))
		context = mergeInferredRouteContext(context, foldedContext)
	}
	return context
}

func inferConversationTurnRouteContext(text string, base inferredRouteContext) inferredRouteContext {
	context := inferRouteContextFromText(text)
	mergedBase := mergeInferredRouteContext(context, base)
	selection := mergeInferredRouteContext(
		inferOriginSelectionContext(text, mergedBase),
		inferDestinationSelectionContext(text, mergedBase),
	)
	return mergeInferredRouteContext(context, selection)
}

func inferOriginSelectionContext(text string, base inferredRouteContext) inferredRouteContext {
	if strings.TrimSpace(base.RouteDirection) == "" && strings.TrimSpace(base.PackageName) == "" {
		return inferredRouteContext{}
	}
	if _, _, ok := extractExplicitRouteFromText(text); ok {
		return inferredRouteContext{}
	}

	direction := strings.TrimSpace(base.RouteDirection)
	if direction == "" {
		return inferredRouteContext{}
	}
	origin := inferKnownPackageOrigin(text, direction)
	if origin == "" {
		return inferredRouteContext{}
	}
	return inferredRouteContext{
		Origin:         origin,
		PackageName:    packageNameForRouteDirection(direction),
		RouteDirection: direction,
		BroadState:     base.BroadState,
	}
}

func inferDestinationSelectionContext(text string, base inferredRouteContext) inferredRouteContext {
	if strings.TrimSpace(base.RouteDirection) == "" && strings.TrimSpace(base.PackageName) == "" {
		return inferredRouteContext{}
	}
	if _, _, ok := extractExplicitRouteFromText(text); ok {
		return inferredRouteContext{}
	}

	direction := strings.TrimSpace(base.RouteDirection)
	if direction == "" {
		return inferredRouteContext{}
	}
	destination := inferKnownPackageDestination(text, direction)
	if destination == "" {
		return inferredRouteContext{}
	}
	return inferredRouteContext{
		Destination:    destination,
		PackageName:    packageNameForRouteDirection(direction),
		RouteDirection: direction,
		BroadState:     base.BroadState,
	}
}

func inferKnownPackageDestination(text string, direction string) string {
	folded := foldChatText(NormalizeIncomingCustomerText(text))
	if folded == "" {
		return ""
	}

	var candidates map[string]string
	switch strings.ToUpper(strings.TrimSpace(direction)) {
	case "TO_SC":
		candidates = scPackageDestinations
	case "TO_MA":
		candidates = maPackageDestinations
	default:
		return ""
	}

	matchCount := 0
	selected := ""
	for key, canonical := range candidates {
		if strings.Contains(folded, " "+key+" ") {
			matchCount++
			selected = canonical
		}
	}
	if matchCount != 1 {
		return ""
	}
	return selected
}

func inferKnownPackageOrigin(text string, direction string) string {
	folded := foldChatText(NormalizeIncomingCustomerText(text))
	if folded == "" {
		return ""
	}

	var candidates map[string]string
	switch strings.ToUpper(strings.TrimSpace(direction)) {
	case "TO_SC":
		candidates = maPackageDestinations
	case "TO_MA":
		candidates = scPackageDestinations
	default:
		return ""
	}

	matchCount := 0
	selected := ""
	for key, canonical := range candidates {
		if strings.Contains(folded, " "+key+" ") {
			matchCount++
			selected = canonical
		}
	}
	if matchCount != 1 {
		return ""
	}
	return selected
}

func inferKnownLocationFromCandidates(text string, candidates map[string]string) string {
	folded := foldChatText(NormalizeIncomingCustomerText(text))
	if folded == "" {
		return ""
	}

	matchCount := 0
	selected := ""
	for key, canonical := range candidates {
		if strings.Contains(folded, " "+key+" ") {
			matchCount++
			selected = canonical
		}
	}
	if matchCount != 1 {
		return ""
	}
	return selected
}

func inferRouteContextFromFoldedText(folded string) inferredRouteContext {
	context := inferredRouteContext{}
	switch detectBroadTravelState(folded) {
	case "SC":
		context.BroadState = "SC"
		context.RouteDirection = "TO_SC"
		context.PackageName = packageToSantaCatarina
	case "MA":
		context.BroadState = "MA"
		context.RouteDirection = "TO_MA"
		context.PackageName = packageToMaranhao
	}
	return context
}

func mergeInferredRouteContext(primary inferredRouteContext, fallback inferredRouteContext) inferredRouteContext {
	merged := primary
	if merged.Origin == "" {
		merged.Origin = fallback.Origin
	}
	if merged.Destination == "" {
		merged.Destination = fallback.Destination
	}
	if merged.PackageName == "" {
		merged.PackageName = fallback.PackageName
	}
	if merged.RouteDirection == "" {
		merged.RouteDirection = fallback.RouteDirection
	}
	if merged.BroadState == "" {
		merged.BroadState = fallback.BroadState
	}
	return merged
}

func inferRouteDirectionFromDestination(destination string) string {
	upper := strings.ToUpper(strings.TrimSpace(destination))
	switch {
	case strings.HasSuffix(upper, "/SC"):
		return "TO_SC"
	case strings.HasSuffix(upper, "/MA"):
		return "TO_MA"
	default:
		return ""
	}
}

func packageNameForRouteDirection(direction string) string {
	switch strings.ToUpper(strings.TrimSpace(direction)) {
	case "TO_SC":
		return packageToSantaCatarina
	case "TO_MA":
		return packageToMaranhao
	default:
		return ""
	}
}

func looksLikeShortDateFollowUp(text string) bool {
	folded := foldChatText(text)
	if folded == "" {
		return false
	}
	patterns := []string{
		"pra quando",
		"para quando",
		"quais datas",
		"que datas",
		"datas disponiveis",
		"tem vaga quando",
		"tem vagas quando",
	}
	for _, pattern := range patterns {
		if strings.Contains(folded, pattern) {
			return true
		}
	}
	return folded == "datas" || folded == "quando"
}

func looksLikeBroadStateScheduleLookup(text string) bool {
	folded := foldChatText(text)
	if folded == "" {
		return false
	}
	keywords := []string{
		"data",
		"datas",
		"quando",
		"horario",
		"horarios",
		"saida",
		"saidas",
	}
	for _, keyword := range keywords {
		if strings.Contains(folded, keyword) {
			return true
		}
	}
	return false
}

func detectBroadTravelState(folded string) string {
	text := " " + strings.Join(strings.Fields(folded), " ") + " "

	switch {
	case strings.Contains(text, " santa catarina ") ||
		strings.Contains(text, " sc "):
		return "SC"

	case strings.Contains(text, " maranhao ") ||
		strings.Contains(text, " ma "):
		return "MA"

	default:
		return ""
	}
}

func foldChatText(text string) string {
	replacer := strings.NewReplacer(
		"á", "a", "à", "a", "â", "a", "ã", "a",
		"é", "e", "ê", "e",
		"í", "i",
		"ó", "o", "ô", "o", "õ", "o",
		"ú", "u",
		"ç", "c",
		"/", " ", ",", " ", ".", " ", "?", " ", "!", " ", "(", " ", ")", " ", "-", " ", ":", " ", ";", " ",
	)
	folded := strings.ToLower(strings.TrimSpace(text))
	folded = replacer.Replace(folded)
	folded = strings.Join(strings.Fields(folded), " ")
	return " " + folded + " "
}

func normalizeLocationDisplayName(value string) string {
	value = NormalizeIncomingCustomerText(value)
	raw := strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if raw == "" {
		return ""
	}
	raw = routeQualifierPrefixPattern.ReplaceAllString(raw, "")
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	if open := strings.LastIndex(raw, "("); open >= 0 && strings.HasSuffix(raw, ")") {
		city := strings.TrimSpace(strings.TrimRight(raw[:open], "-, "))
		uf := strings.ToUpper(strings.TrimSpace(strings.TrimSuffix(raw[open+1:], ")")))
		if city != "" && isBrazilianStateCode(uf) {
			parts := strings.Fields(strings.ToLower(city))
			for i, part := range parts {
				runes := []rune(part)
				if len(runes) == 0 {
					continue
				}
				parts[i] = strings.ToUpper(string(runes[0])) + string(runes[1:])
			}
			return strings.Join(parts, " ") + "/" + uf
		}
	}

	match := locationMentionPattern.FindStringSubmatch(raw)
	if len(match) != 3 {
		return ""
	}
	city := strings.TrimSpace(strings.TrimRight(match[1], "-, "))
	uf := strings.ToUpper(strings.TrimSpace(match[2]))
	if city == "" || uf == "" || !isBrazilianStateCode(uf) {
		return ""
	}

	parts := strings.Fields(strings.ToLower(city))
	for i, part := range parts {
		runes := []rune(part)
		if len(runes) == 0 {
			continue
		}
		parts[i] = strings.ToUpper(string(runes[0])) + string(runes[1:])
	}
	return strings.Join(parts, " ") + "/" + uf
}

func normalizeSupportedPackageLocation(value string) string {
	folded := foldChatText(NormalizeIncomingCustomerText(value))
	for key, canonical := range scPackageDestinations {
		if strings.Contains(folded, " "+foldChatDestinationKey(key)+" ") {
			return canonical
		}
	}
	for key, canonical := range maPackageDestinations {
		if strings.Contains(folded, " "+foldChatDestinationKey(key)+" ") {
			return canonical
		}
	}
	return normalizeLocationDisplayName(value)
}

func canonicalStopIDForLocation(value string) string {
	key := normalizeCanonicalLocationKey(normalizeLocationDisplayName(value))
	if key == "" {
		return ""
	}
	return canonicalStopIDsByLocation[key]
}

func routeIDForPackageName(packageName string) string {
	key := normalizeCanonicalLocationKey(packageName)
	if key == "" {
		return ""
	}
	return routeIDByPackageName[key]
}

func normalizeCanonicalLocationKey(value string) string {
	replacer := strings.NewReplacer(
		"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
		"é", "e", "è", "e", "ê", "e", "ë", "e",
		"í", "i", "ì", "i", "î", "i", "ï", "i",
		"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
		"ú", "u", "ù", "u", "û", "u", "ü", "u",
		"ç", "c",
	)
	folded := strings.ToLower(NormalizeIncomingCustomerText(value))
	folded = replacer.Replace(folded)
	folded = strings.TrimSpace(folded)
	if folded == "" {
		return ""
	}
	folded = strings.Trim(folded, " .,:;!?")
	folded = strings.ReplaceAll(folded, " /", "/")
	folded = strings.ReplaceAll(folded, "/ ", "/")
	return strings.Join(strings.Fields(folded), " ")
}

func enrichAvailabilitySearchInput(input AvailabilitySearchInput) AvailabilitySearchInput {
	if input.OriginStopID == "" {
		input.OriginStopID = canonicalStopIDForLocation(input.Origin)
	}
	if input.DestinationStopID == "" {
		input.DestinationStopID = canonicalStopIDForLocation(input.Destination)
	}
	if input.RouteID == "" {
		input.RouteID = routeIDForPackageName(input.PackageName)
	}
	return input
}

func normalizeRouteEndpoint(city string, uf string) string {
	city = strings.Join(strings.Fields(strings.TrimSpace(city)), " ")
	uf = strings.ToUpper(strings.TrimSpace(uf))
	if city == "" {
		return ""
	}
	if uf != "" {
		if !isBrazilianStateCode(uf) {
			return ""
		}
		return normalizeLocationDisplayName(city + "/" + uf)
	}

	if state := lookupKnownLocationState(city); state != "" {
		return normalizeLocationDisplayName(city + "/" + state)
	}
	return ""
}

func lookupKnownLocationState(city string) string {
	folded := foldChatText(city)
	if folded == "" {
		return ""
	}

	for key := range scPackageDestinations {
		if strings.Contains(folded, " "+foldChatDestinationKey(key)+" ") {
			return "SC"
		}
	}
	for key := range maPackageDestinations {
		if strings.Contains(folded, " "+foldChatDestinationKey(key)+" ") {
			return "MA"
		}
	}
	return ""
}

func isBrazilianStateCode(uf string) bool {
	_, ok := brazilianStateCodes[strings.ToUpper(strings.TrimSpace(uf))]
	return ok
}

func extractTripDate(text string, observedAt time.Time) *time.Time {
	if match := isoDatePattern.FindStringSubmatch(text); len(match) == 4 {
		if parsed, err := time.Parse("2006-01-02", match[0]); err == nil {
			parsed = parsed.UTC()
			return &parsed
		}
	}
	if match := brDatePattern.FindStringSubmatch(text); len(match) >= 3 {
		day, _ := strconv.Atoi(match[1])
		month, _ := strconv.Atoi(match[2])
		year := observedAt.UTC().Year()
		if len(match) >= 4 && strings.TrimSpace(match[3]) != "" {
			if parsedYear, err := strconv.Atoi(match[3]); err == nil {
				year = parsedYear
			}
		}
		parsed := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
		if parsed.Month() == time.Month(month) && parsed.Day() == day {
			return &parsed
		}
	}
	return nil
}

func parseISODatePtr(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil
	}
	parsed = parsed.UTC()
	return &parsed
}

func sameCalendarDate(a *time.Time, b *time.Time) bool {
	if a == nil || b == nil {
		return false
	}
	aUTC := a.UTC()
	bUTC := b.UTC()
	return aUTC.Year() == bUTC.Year() && aUTC.Month() == bUTC.Month() && aUTC.Day() == bUTC.Day()
}

func extractDayOfMonthDateSelection(text string) int {
	folded := foldChatText(text)
	if folded == "" || !strings.Contains(folded, " dia ") {
		return 0
	}
	fields := strings.Fields(folded)
	for i := 0; i < len(fields)-1; i++ {
		if fields[i] != "dia" {
			continue
		}
		day, err := strconv.Atoi(fields[i+1])
		if err == nil && day >= 1 && day <= 31 {
			return day
		}
	}
	return 0
}

func extractPassengerQuantity(text string) int {
	match := qtyPattern.FindStringSubmatch(text)
	if len(match) != 2 {
		return 0
	}
	value, err := strconv.Atoi(match[1])
	if err != nil || value <= 0 {
		return 0
	}
	return value
}

func buildAvailabilityToolRequestPayload(input AvailabilitySearchInput) map[string]interface{} {
	payload := map[string]interface{}{
		"origin":      input.Origin,
		"destination": input.Destination,
		"qtd":         input.Qty,
		"limit":       input.Limit,
		"only_active": true,
	}
	if input.OriginStopID != "" {
		payload["origin_stop_id"] = input.OriginStopID
	}
	if input.DestinationStopID != "" {
		payload["destination_stop_id"] = input.DestinationStopID
	}
	if input.RouteID != "" {
		payload["route_id"] = input.RouteID
	}
	if input.TripDate != nil {
		payload["trip_date"] = input.TripDate.UTC().Format("2006-01-02")
	}
	if input.DateFrom != nil {
		payload["date_from"] = input.DateFrom.UTC().Format("2006-01-02")
	}
	if input.DateTo != nil {
		payload["date_to"] = input.DateTo.UTC().Format("2006-01-02")
	}
	if input.PackageName != "" {
		payload["package_name"] = input.PackageName
	}
	return payload
}

func buildAvailabilityToolResponsePayload(result AvailabilitySearchResult) map[string]interface{} {
	items := make([]map[string]interface{}, 0, len(result.Results))
	for _, item := range result.Results {
		items = append(items, map[string]interface{}{
			"segment_id":               item.SegmentID,
			"trip_id":                  item.TripID,
			"route_id":                 item.RouteID,
			"board_stop_id":            item.BoardStopID,
			"alight_stop_id":           item.AlightStopID,
			"origin_stop_id":           item.OriginStopID,
			"destination_stop_id":      item.DestinationStopID,
			"origin_display_name":      item.OriginDisplayName,
			"destination_display_name": item.DestinationDisplayName,
			"origin_depart_time":       item.OriginDepartTime,
			"trip_date":                item.TripDate,
			"seats_available":          item.SeatsAvailable,
			"price":                    item.Price,
			"currency":                 item.Currency,
			"status":                   item.Status,
			"trip_status":              item.TripStatus,
			"package_name":             item.PackageName,
		})
	}

	payload := map[string]interface{}{
		"result_count": len(result.Results),
		"results":      items,
	}
	if result.Filter.Origin != "" {
		payload["origin"] = result.Filter.Origin
	}
	if result.Filter.Destination != "" {
		payload["destination"] = result.Filter.Destination
	}
	if result.Filter.PackageName != "" {
		payload["package_name"] = result.Filter.PackageName
	}
	if result.Filter.TripDate != nil {
		payload["trip_date"] = result.Filter.TripDate.UTC().Format("2006-01-02")
	}
	if result.Filter.DateFrom != nil {
		payload["date_from"] = result.Filter.DateFrom.UTC().Format("2006-01-02")
	}
	if result.Filter.DateTo != nil {
		payload["date_to"] = result.Filter.DateTo.UTC().Format("2006-01-02")
	}
	if result.Filter.Qty > 0 {
		payload["qtd"] = result.Filter.Qty
	}
	return payload
}

func buildPricingQuoteInput(result AvailabilitySearchResult) PricingQuoteInput {
	candidates := make([]PricingQuoteCandidate, 0, len(result.Results))
	for index, item := range result.Results {
		if index >= 3 {
			break
		}
		candidates = append(candidates, PricingQuoteCandidate{
			TripID:                 item.TripID,
			RouteID:                item.RouteID,
			BoardStopID:            item.BoardStopID,
			AlightStopID:           item.AlightStopID,
			OriginStopID:           item.OriginStopID,
			DestinationStopID:      item.DestinationStopID,
			OriginDisplayName:      item.OriginDisplayName,
			DestinationDisplayName: item.DestinationDisplayName,
			OriginDepartTime:       item.OriginDepartTime,
			TripDate:               item.TripDate,
		})
	}
	return PricingQuoteInput{
		FareMode:   "AUTO",
		Candidates: candidates,
	}
}

func buildPricingQuoteRequestPayload(input PricingQuoteInput) map[string]interface{} {
	candidates := make([]map[string]interface{}, 0, len(input.Candidates))
	for _, candidate := range input.Candidates {
		candidates = append(candidates, map[string]interface{}{
			"trip_id":                  candidate.TripID,
			"route_id":                 candidate.RouteID,
			"board_stop_id":            candidate.BoardStopID,
			"alight_stop_id":           candidate.AlightStopID,
			"origin_stop_id":           candidate.OriginStopID,
			"destination_stop_id":      candidate.DestinationStopID,
			"origin_display_name":      candidate.OriginDisplayName,
			"destination_display_name": candidate.DestinationDisplayName,
			"origin_depart_time":       candidate.OriginDepartTime,
			"trip_date":                candidate.TripDate,
		})
	}
	return map[string]interface{}{
		"fare_mode":  input.FareMode,
		"candidates": candidates,
	}
}

func buildPricingQuoteResponsePayload(result PricingQuoteResult) map[string]interface{} {
	items := make([]map[string]interface{}, 0, len(result.Results))
	for _, item := range result.Results {
		items = append(items, map[string]interface{}{
			"trip_id":                  item.TripID,
			"route_id":                 item.RouteID,
			"board_stop_id":            item.BoardStopID,
			"alight_stop_id":           item.AlightStopID,
			"origin_stop_id":           item.OriginStopID,
			"destination_stop_id":      item.DestinationStopID,
			"origin_display_name":      item.OriginDisplayName,
			"destination_display_name": item.DestinationDisplayName,
			"origin_depart_time":       item.OriginDepartTime,
			"trip_date":                item.TripDate,
			"base_amount":              item.BaseAmount,
			"calc_amount":              item.CalcAmount,
			"final_amount":             item.FinalAmount,
			"currency":                 item.Currency,
			"fare_mode":                item.FareMode,
		})
	}
	return map[string]interface{}{
		"fare_mode":    result.Filter.FareMode,
		"result_count": len(result.Results),
		"results":      items,
	}
}

func parseBookingLookupInput(text string) (BookingLookupInput, bool) {
	body := strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if body == "" || !looksLikeBookingLookupIntent(body) {
		return BookingLookupInput{}, false
	}

	bookingID, reservationCode := extractBookingIdentifiers(body)
	if bookingID == "" && reservationCode == "" {
		return BookingLookupInput{}, false
	}

	return BookingLookupInput{
		BookingID:       bookingID,
		ReservationCode: reservationCode,
		Limit:           3,
	}, true
}

func looksLikeBookingLookupIntent(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return false
	}

	keywords := []string{
		"reserva",
		"booking",
		"codigo",
		"código",
		"status",
		"confirmad",
		"pagament",
		"expir",
		"pix",
	}
	for _, keyword := range keywords {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}

func parseRescheduleAssistInput(text string, observedAt time.Time) (RescheduleAssistInput, bool) {
	body := strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if body == "" || !looksLikeRescheduleIntent(body) {
		return RescheduleAssistInput{}, false
	}

	bookingID, reservationCode := extractBookingIdentifiers(body)
	requestedTripDate := extractTripDate(body, observedAt)
	if bookingID == "" && reservationCode == "" {
		return RescheduleAssistInput{}, false
	}
	if requestedTripDate == nil {
		return RescheduleAssistInput{}, false
	}

	requestedOrigin, requestedDestination := extractRequestedRoute(body)
	input := RescheduleAssistInput{
		BookingID:            bookingID,
		ReservationCode:      reservationCode,
		RequestedTripDate:    requestedTripDate,
		RequestedOrigin:      requestedOrigin,
		RequestedDestination: requestedDestination,
		RequestedQty:         extractPassengerQuantity(body),
	}
	return input, true
}

func looksLikeRescheduleIntent(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return false
	}

	keywords := []string{
		"reagend",
		"remarc",
		"mudar a data",
		"mudar data",
		"trocar a data",
		"trocar data",
		"alterar a data",
		"alterar data",
	}
	for _, keyword := range keywords {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}

func looksLikePaymentLookupIntent(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return false
	}

	keywords := []string{
		"pagament",
		"pix",
		"cobranc",
		"cobranç",
		"paguei",
		"pago",
		"quitad",
		"checkout",
	}
	for _, keyword := range keywords {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}

func extractBookingIdentifiers(text string) (string, string) {
	bookingID := strings.ToUpper(strings.TrimSpace(bookingIDPattern.FindString(text)))
	reservationCode := ""
	for _, token := range reservationCodePattern.FindAllString(strings.ToUpper(text), -1) {
		if token == "" || token == bookingID || !containsLetter(token) || !containsDigit(token) {
			continue
		}
		reservationCode = token
		break
	}
	return bookingID, reservationCode
}

func extractRequestedRoute(text string) (string, string) {
	origin, destination, ok := extractExplicitRouteFromText(text)
	if !ok {
		return "", ""
	}
	return origin, destination
}

func containsLetter(value string) bool {
	for _, char := range value {
		if (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') {
			return true
		}
	}
	return false
}

func containsDigit(value string) bool {
	for _, char := range value {
		if char >= '0' && char <= '9' {
			return true
		}
	}
	return false
}

func buildBookingLookupRequestPayload(input BookingLookupInput) map[string]interface{} {
	payload := map[string]interface{}{
		"limit": input.Limit,
	}
	if input.BookingID != "" {
		payload["booking_id"] = input.BookingID
	}
	if input.ReservationCode != "" {
		payload["reservation_code"] = input.ReservationCode
	}
	return payload
}

func buildBookingLookupResponsePayload(result BookingLookupResult) map[string]interface{} {
	items := make([]map[string]interface{}, 0, len(result.Results))
	for _, item := range result.Results {
		payload := map[string]interface{}{
			"booking_id":       item.ID,
			"trip_id":          item.TripID,
			"status":           item.Status,
			"reservation_code": item.ReservationCode,
			"total_amount":     item.TotalAmount,
			"deposit_amount":   item.DepositAmount,
			"remainder_amount": item.RemainderAmount,
			"passenger_name":   item.PassengerName,
			"passenger_phone":  item.PassengerPhone,
			"seat_number":      item.SeatNumber,
			"created_at":       item.CreatedAt.UTC().Format(time.RFC3339Nano),
		}
		if item.ExpiresAt != nil {
			payload["expires_at"] = item.ExpiresAt.UTC().Format(time.RFC3339Nano)
		}
		items = append(items, payload)
	}

	payload := map[string]interface{}{
		"result_count": len(result.Results),
		"results":      items,
	}
	if result.Filter.BookingID != "" {
		payload["booking_id"] = result.Filter.BookingID
	}
	if result.Filter.ReservationCode != "" {
		payload["reservation_code"] = result.Filter.ReservationCode
	}
	return payload
}

func buildRescheduleAssistRequestPayload(input RescheduleAssistInput) map[string]interface{} {
	payload := map[string]interface{}{}
	if input.BookingID != "" {
		payload["booking_id"] = input.BookingID
	}
	if input.ReservationCode != "" {
		payload["reservation_code"] = input.ReservationCode
	}
	if input.RequestedTripDate != nil {
		payload["requested_trip_date"] = input.RequestedTripDate.UTC().Format("2006-01-02")
	}
	if input.RequestedOrigin != "" {
		payload["requested_origin"] = input.RequestedOrigin
	}
	if input.RequestedDestination != "" {
		payload["requested_destination"] = input.RequestedDestination
	}
	if input.RequestedQty > 0 {
		payload["requested_qty"] = input.RequestedQty
	}
	return payload
}

func buildRescheduleAssistResponsePayload(result RescheduleAssistResult) map[string]interface{} {
	payload := map[string]interface{}{
		"mode":                  result.Mode,
		"next_step":             result.NextStep,
		"human_review_required": result.HumanReviewRequired,
		"can_auto_reschedule":   result.CanAutoReschedule,
		"options_count":         len(result.Options),
	}
	if len(result.Errors) > 0 {
		payload["errors"] = result.Errors
	}
	if result.MessageForAgent != "" {
		payload["message_for_agent"] = result.MessageForAgent
	}
	if len(result.FieldsRequiredForManualCompletion) > 0 {
		payload["fields_required_for_manual_completion"] = result.FieldsRequiredForManualCompletion
	}
	if result.Booking != nil {
		payload["booking"] = map[string]interface{}{
			"booking_id":       result.Booking.ID,
			"trip_id":          result.Booking.TripID,
			"status":           result.Booking.Status,
			"reservation_code": result.Booking.ReservationCode,
		}
	}
	payload["current"] = map[string]interface{}{
		"origin":          result.Current.Origin,
		"destination":     result.Current.Destination,
		"trip_date":       result.Current.TripDate,
		"passenger_count": result.Current.PassengerCount,
	}
	payload["requested"] = map[string]interface{}{
		"origin":        result.Requested.Origin,
		"destination":   result.Requested.Destination,
		"trip_date":     result.Requested.TripDate,
		"qtd_passagens": result.Requested.Qty,
	}

	options := make([]map[string]interface{}, 0, len(result.Options))
	for _, option := range result.Options {
		options = append(options, map[string]interface{}{
			"trip_id":         option.TripID,
			"trip_date":       option.TripDate,
			"hora":            option.DepartureTime,
			"origin":          option.Origin,
			"destination":     option.Destination,
			"board_stop_id":   option.BoardStopID,
			"alight_stop_id":  option.AlightStopID,
			"seats_available": option.SeatsAvailable,
			"price":           option.Price,
			"currency":        option.Currency,
			"package_name":    option.PackageName,
		})
	}
	payload["options"] = options
	return payload
}

func buildPaymentStatusRequestPayload(input PaymentStatusInput) map[string]interface{} {
	payload := map[string]interface{}{
		"limit": input.Limit,
	}
	if input.BookingID != "" {
		payload["booking_id"] = input.BookingID
	}
	if input.ReservationCode != "" {
		payload["reservation_code"] = input.ReservationCode
	}
	return payload
}

func buildPaymentStatusResponsePayload(result PaymentStatusResult) map[string]interface{} {
	items := make([]map[string]interface{}, 0, len(result.Results))
	totalAmount := 0.0
	paidAmount := 0.0
	latestStatus := ""
	for index, item := range result.Results {
		payload := map[string]interface{}{
			"payment_id":   item.ID,
			"booking_id":   item.BookingID,
			"amount":       item.Amount,
			"method":       item.Method,
			"status":       item.Status,
			"provider":     item.Provider,
			"provider_ref": item.ProviderRef,
			"created_at":   item.CreatedAt.UTC().Format(time.RFC3339Nano),
		}
		if item.PaidAt != nil {
			payload["paid_at"] = item.PaidAt.UTC().Format(time.RFC3339Nano)
		}
		items = append(items, payload)
		totalAmount += item.Amount
		if strings.EqualFold(item.Status, "PAID") {
			paidAmount += item.Amount
		}
		if index == 0 {
			latestStatus = strings.TrimSpace(item.Status)
		}
	}

	payload := map[string]interface{}{
		"result_count":  len(result.Results),
		"results":       items,
		"total_amount":  totalAmount,
		"paid_amount":   paidAmount,
		"latest_status": latestStatus,
	}
	if result.Filter.BookingID != "" {
		payload["booking_id"] = result.Filter.BookingID
	}
	if result.Filter.ReservationCode != "" {
		payload["reservation_code"] = result.Filter.ReservationCode
	}
	return payload
}

func (s *Service) executeAvailabilitySearchIntentTool(ctx context.Context, session Session, input AvailabilitySearchInput) (agentToolContext, error) {
	if !s.canSearchAvailability() {
		return agentToolContext{}, nil
	}

	input = enrichAvailabilitySearchInput(input)
	startedAt := time.Now().UTC()
	requestPayload := buildAvailabilityToolRequestPayload(input)

	result, err := s.availability.Search(ctx, input)
	finishedAt := time.Now().UTC()
	finishedAtPtr := &finishedAt

	if err != nil {
		call, recordErr := s.store.CreateToolCall(ctx, CreateToolCallInput{
			SessionID:      session.ID,
			ToolName:       toolNameAvailabilitySearch,
			RequestPayload: requestPayload,
			Status:         "FAILED",
			ErrorCode:      "AVAILABILITY_SEARCH_ERROR",
			ErrorMessage:   strings.TrimSpace(err.Error()),
			StartedAt:      startedAt,
			FinishedAt:     finishedAtPtr,
		})
		if recordErr != nil {
			return agentToolContext{}, recordErr
		}
		return agentToolContext{Calls: []ToolCall{call}}, fmt.Errorf("%w: %v", ErrAgentToolFailed, err)
	}

	call, err := s.store.CreateToolCall(ctx, CreateToolCallInput{
		SessionID:       session.ID,
		ToolName:        toolNameAvailabilitySearch,
		RequestPayload:  requestPayload,
		ResponsePayload: buildAvailabilityToolResponsePayload(result),
		Status:          "COMPLETED",
		StartedAt:       startedAt,
		FinishedAt:      finishedAtPtr,
	})
	if err != nil {
		return agentToolContext{}, err
	}

	return agentToolContext{
		Calls:        []ToolCall{call},
		Availability: &result,
	}, nil
}

func (s *Service) executePricingQuoteFromAvailabilityTool(ctx context.Context, session Session, context agentToolContext, currentTurn string) (agentToolContext, error) {
	if !s.canSearchPricing() || context.Availability == nil || !looksLikePricingQuoteIntent(currentTurn) || len(context.Availability.Results) == 0 {
		return context, nil
	}

	pricingInput := buildPricingQuoteInput(*context.Availability)
	pricingStartedAt := time.Now().UTC()
	pricingRequestPayload := buildPricingQuoteRequestPayload(pricingInput)
	pricingResult, pricingErr := s.pricing.Search(ctx, pricingInput)
	pricingFinishedAt := time.Now().UTC()
	pricingFinishedAtPtr := &pricingFinishedAt
	if pricingErr != nil {
		call, recordErr := s.store.CreateToolCall(ctx, CreateToolCallInput{
			SessionID:      session.ID,
			ToolName:       toolNamePricingQuote,
			RequestPayload: pricingRequestPayload,
			Status:         "FAILED",
			ErrorCode:      "PRICING_QUOTE_ERROR",
			ErrorMessage:   strings.TrimSpace(pricingErr.Error()),
			StartedAt:      pricingStartedAt,
			FinishedAt:     pricingFinishedAtPtr,
		})
		if recordErr != nil {
			return agentToolContext{}, recordErr
		}
		context.Calls = append(context.Calls, call)
		return context, fmt.Errorf("%w: %v", ErrAgentToolFailed, pricingErr)
	}

	call, err := s.store.CreateToolCall(ctx, CreateToolCallInput{
		SessionID:       session.ID,
		ToolName:        toolNamePricingQuote,
		RequestPayload:  pricingRequestPayload,
		ResponsePayload: buildPricingQuoteResponsePayload(pricingResult),
		Status:          "COMPLETED",
		StartedAt:       pricingStartedAt,
		FinishedAt:      pricingFinishedAtPtr,
	})
	if err != nil {
		return agentToolContext{}, err
	}
	context.Calls = append(context.Calls, call)
	context.Pricing = &pricingResult
	return context, nil
}
