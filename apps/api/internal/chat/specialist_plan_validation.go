package chat

import "strings"

const (
	specialistPlanActionTool     = "tool"
	specialistPlanActionTemplate = "template"
	specialistPlanActionClarify  = "clarify"
	specialistPlanActionNoop     = "noop"
)

type SpecialistPlanValidationResult struct {
	Valid                bool
	Reasons              []string
	ApprovedToolRequests []SpecialistToolRequest
	ReplyPlan            *CustomerReplyPlan
}

type specialistPlanValidationInput struct {
	Domain             string
	Action             string
	MissingFields      []string
	ToolRequests       []SpecialistToolRequest
	CustomerFacingText string
	ReplyPlan          *CustomerReplyPlan
	State              CanonicalConversationState
}

func validateSpecialistPlan(input specialistPlanValidationInput) SpecialistPlanValidationResult {
	reasons := []string{}
	action := strings.ToLower(strings.TrimSpace(input.Action))
	if action == "" {
		if len(input.ToolRequests) > 0 {
			action = specialistPlanActionTool
		} else if input.ReplyPlan != nil {
			action = specialistPlanActionTemplate
		}
	}
	if !specialistPlanActionAllowed(action) {
		reasons = append(reasons, "invalid_action")
	}
	if strings.TrimSpace(input.CustomerFacingText) != "" {
		reasons = append(reasons, "customer_facing_text_not_allowed")
	}
	if action == specialistPlanActionClarify && len(input.MissingFields) == 0 && input.ReplyPlan == nil {
		reasons = append(reasons, "clarify_missing_fields")
	}
	approved := make([]SpecialistToolRequest, 0, len(input.ToolRequests))
	if action == specialistPlanActionTool {
		if len(input.ToolRequests) == 0 {
			reasons = append(reasons, "tool_request_required")
		}
		for _, request := range input.ToolRequests {
			normalized := normalizeSpecialistToolRequest(request)
			requestReasons := validateSpecialistToolRequest(normalized, input.Domain, input.State)
			if len(requestReasons) > 0 {
				reasons = append(reasons, requestReasons...)
				continue
			}
			approved = append(approved, normalized)
		}
	}
	if len(reasons) > 0 {
		return SpecialistPlanValidationResult{Valid: false, Reasons: reasons}
	}
	return SpecialistPlanValidationResult{
		Valid:                true,
		ApprovedToolRequests: approved,
		ReplyPlan:            input.ReplyPlan,
	}
}

func specialistPlanActionAllowed(action string) bool {
	switch action {
	case specialistPlanActionTool, specialistPlanActionTemplate, specialistPlanActionClarify, specialistPlanActionNoop:
		return true
	default:
		return false
	}
}

func normalizeSpecialistToolRequest(request SpecialistToolRequest) SpecialistToolRequest {
	request.ToolName = strings.ToLower(strings.TrimSpace(request.ToolName))
	request.ReasonCode = strings.TrimSpace(request.ReasonCode)
	request.Origin = strings.TrimSpace(request.Origin)
	request.Destination = strings.TrimSpace(request.Destination)
	request.PackageName = strings.TrimSpace(request.PackageName)
	request.TripDate = strings.TrimSpace(request.TripDate)
	request.BookingID = strings.TrimSpace(request.BookingID)
	request.ReservationCode = strings.TrimSpace(request.ReservationCode)
	request.PaymentMethod = strings.TrimSpace(request.PaymentMethod)
	request.CustomerDocument = strings.TrimSpace(request.CustomerDocument)
	return request
}

func validateSpecialistToolRequest(request SpecialistToolRequest, domain string, state CanonicalConversationState) []string {
	reasons := []string{}
	if request.ToolName == "" {
		return append(reasons, "tool_name_required")
	}
	if !toolAllowedForSpecialistDomain(request.ToolName, domain) {
		reasons = append(reasons, "tool_not_allowed_for_specialist")
	}
	switch request.ToolName {
	case toolNameAvailabilitySearch:
		if strings.TrimSpace(request.Origin) == "" && strings.TrimSpace(request.Destination) == "" && strings.TrimSpace(request.PackageName) == "" {
			reasons = append(reasons, "availability_route_required")
		}
	case toolNamePricingQuote:
		if len(asMap(state.LastToolFacts[toolNameAvailabilitySearch])) == 0 {
			reasons = append(reasons, "pricing_requires_availability_facts")
		}
	case toolNameBookingCreate:
		if state.Phase != ConversationPhaseBookingPending {
			reasons = append(reasons, "booking_create_wrong_phase")
		}
		if strings.TrimSpace(state.Route.TripID) == "" {
			reasons = append(reasons, "booking_create_requires_trip")
		}
		if state.Passengers.ExpectedCount <= 0 || !state.Passengers.DocumentsCollected {
			reasons = append(reasons, "booking_create_requires_passenger_data")
		}
	case toolNameBookingCancel:
		if !agentDecisionHasBookingContext(state) && request.BookingID == "" && request.ReservationCode == "" {
			reasons = append(reasons, "booking_cancel_requires_booking_context")
		}
	case toolNamePaymentStatus, toolNamePaymentCreate:
		if !agentDecisionHasBookingContext(state) && request.BookingID == "" && request.ReservationCode == "" {
			reasons = append(reasons, "payment_requires_booking_context")
		}
		if request.ToolName == toolNamePaymentCreate && strings.TrimSpace(request.PaymentMethod) == "" {
			reasons = append(reasons, "payment_method_required")
		}
	}
	return reasons
}

func toolAllowedForSpecialistDomain(toolName string, domain string) bool {
	switch strings.ToLower(strings.TrimSpace(domain)) {
	case jsonDecisionDomainGeneral:
		switch toolName {
		case toolNameAvailabilitySearch, toolNamePricingQuote, toolNameDocumentExtract:
			return true
		}
	case jsonDecisionDomainScheduling:
		switch toolName {
		case toolNameBookingLookup, toolNameBookingCreate, toolNameBookingCancel, toolNameRescheduleLookup:
			return true
		}
	case jsonDecisionDomainPayments:
		switch toolName {
		case toolNameBookingLookup, toolNamePaymentStatus, toolNamePaymentCreate:
			return true
		}
	}
	return false
}

func specialistToolRequestToAvailabilityInput(request SpecialistToolRequest) AvailabilitySearchInput {
	return availabilityInputFromSchedulingPlan(SchedulingActionPlan{
		ToolName:    request.ToolName,
		Origin:      request.Origin,
		Destination: request.Destination,
		PackageName: request.PackageName,
		TripDate:    request.TripDate,
		Qty:         request.Qty,
		Limit:       request.Limit,
	})
}

func specialistToolRequestToPaymentCreateInput(request SpecialistToolRequest) PaymentCreateInput {
	return paymentCreateInputFromPlan(PaymentActionPlan{
		ToolName:         request.ToolName,
		BookingID:        request.BookingID,
		ReservationCode:  request.ReservationCode,
		PaymentMethod:    request.PaymentMethod,
		CustomerDocument: request.CustomerDocument,
		Amount:           request.Amount,
	})
}
