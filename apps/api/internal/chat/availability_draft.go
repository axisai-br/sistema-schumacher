package chat

import (
	"context"
	"strings"
	"time"
)

const (
	availabilityDraftStatusActive    = "ACTIVE"
	availabilityDraftStatusCompleted = "COMPLETED"
	availabilityDraftPeriodMonth     = "MONTH"
	availabilityDraftTemplateMissing = "AVAILABILITY_DRAFT_MISSING_SLOTS"
	pendingQuestionStatusActive      = "ACTIVE"
	pendingQuestionStatusCompleted   = "COMPLETED"

	pendingQuestionAskAvailabilityRouteAndQty  = "ASK_AVAILABILITY_ROUTE_AND_QTY"
	pendingQuestionAskAvailabilityRoute        = "ASK_AVAILABILITY_ROUTE"
	pendingQuestionAskAvailabilityQty          = "ASK_AVAILABILITY_QTY"
	pendingQuestionAskAvailabilityOrigin       = "ASK_AVAILABILITY_ORIGIN"
	pendingQuestionAskAvailabilityDestination  = "ASK_AVAILABILITY_DESTINATION"
	availabilityDraftTemplateSearchUnavailable = "AVAILABILITY_SEARCH_UNAVAILABLE"
)

type AvailabilityDraft struct {
	Status       string
	PeriodType   string
	Month        int
	Year         int
	DateFrom     *time.Time
	DateTo       *time.Time
	Origin       string
	Destination  string
	Qty          int
	MissingSlots []string
	UpdatedAt    time.Time
}

type availabilityMonthAlias struct {
	name  string
	month int
}

var availabilityMonthAliases = []availabilityMonthAlias{
	{name: "janeiro", month: 1},
	{name: "fevereiro", month: 2},
	{name: "marco", month: 3},
	{name: "abril", month: 4},
	{name: "maio", month: 5},
	{name: "junho", month: 6},
	{name: "julho", month: 7},
	{name: "agosto", month: 8},
	{name: "setembro", month: 9},
	{name: "outubro", month: 10},
	{name: "novembro", month: 11},
	{name: "dezembro", month: 12},
}

func parseAvailabilityDraftFromText(text string, now time.Time) (AvailabilityDraft, bool) {
	body := NormalizeIncomingCustomerText(text)
	if strings.TrimSpace(body) == "" {
		return AvailabilityDraft{}, false
	}

	draft := AvailabilityDraft{}
	found := false
	if periodType, month, year, dateFrom, dateTo, ok := parseMonthPeriod(body, now); ok {
		draft.PeriodType = periodType
		draft.Month = month
		draft.Year = year
		draft.DateFrom = &dateFrom
		draft.DateTo = &dateTo
		found = true
	}
	if origin, destination, ok := parseAvailabilityDraftRoute(body); ok {
		draft.Origin = origin
		draft.Destination = destination
		found = true
	}
	if qty := parseAvailabilityDraftQuantity(body); qty > 0 {
		draft.Qty = qty
		found = true
	}
	if !found {
		return AvailabilityDraft{}, false
	}
	draft.Status = availabilityDraftStatusActive
	draft.UpdatedAt = now.UTC()
	draft.MissingSlots = availabilityDraftMissingSlots(draft)
	return draft, true
}

func parseMonthPeriod(text string, now time.Time) (string, int, int, time.Time, time.Time, bool) {
	folded := foldChatText(text)
	if strings.TrimSpace(folded) == "" {
		return "", 0, 0, time.Time{}, time.Time{}, false
	}

	selectedMonth := 0
	selectedIndex := len(folded) + 1
	for _, alias := range availabilityMonthAliases {
		index := strings.Index(folded, " "+alias.name+" ")
		if index < 0 || index >= selectedIndex {
			continue
		}
		selectedMonth = alias.month
		selectedIndex = index
	}
	if selectedMonth == 0 {
		return "", 0, 0, time.Time{}, time.Time{}, false
	}

	observed := now.UTC()
	year := observed.Year()
	if time.Month(selectedMonth) < observed.Month() {
		year++
	}
	dateFrom := time.Date(year, time.Month(selectedMonth), 1, 0, 0, 0, 0, time.UTC)
	dateTo := time.Date(year, time.Month(selectedMonth)+1, 0, 0, 0, 0, 0, time.UTC)
	return availabilityDraftPeriodMonth, selectedMonth, year, dateFrom, dateTo, true
}

func mergeAvailabilityDraft(existing, update AvailabilityDraft) AvailabilityDraft {
	merged := existing
	if strings.TrimSpace(merged.Status) == "" {
		merged.Status = availabilityDraftStatusActive
	}
	if strings.TrimSpace(update.Status) != "" {
		merged.Status = update.Status
	}
	if strings.TrimSpace(update.PeriodType) != "" {
		merged.PeriodType = update.PeriodType
	}
	if update.Month > 0 {
		merged.Month = update.Month
	}
	if update.Year > 0 {
		merged.Year = update.Year
	}
	if update.DateFrom != nil {
		merged.DateFrom = update.DateFrom
	}
	if update.DateTo != nil {
		merged.DateTo = update.DateTo
	}
	if origin := strings.TrimSpace(update.Origin); origin != "" {
		merged.Origin = origin
	}
	if destination := strings.TrimSpace(update.Destination); destination != "" {
		merged.Destination = destination
	}
	if update.Qty > 0 {
		merged.Qty = update.Qty
	}
	if !update.UpdatedAt.IsZero() {
		merged.UpdatedAt = update.UpdatedAt.UTC()
	}
	if merged.UpdatedAt.IsZero() {
		merged.UpdatedAt = time.Now().UTC()
	}
	merged.MissingSlots = availabilityDraftMissingSlots(merged)
	return merged
}

func availabilityDraftMissingSlots(d AvailabilityDraft) []string {
	missing := make([]string, 0, 4)
	if d.DateFrom == nil || d.DateTo == nil {
		missing = append(missing, "period")
	}
	if strings.TrimSpace(d.Origin) == "" {
		missing = append(missing, "origin")
	}
	if strings.TrimSpace(d.Destination) == "" {
		missing = append(missing, "destination")
	}
	if d.Qty <= 0 {
		missing = append(missing, "qty")
	}
	return missing
}

func availabilityDraftReady(d AvailabilityDraft) bool {
	return len(availabilityDraftMissingSlots(d)) == 0
}

func availabilityDraftToSearchInput(d AvailabilityDraft) (AvailabilitySearchInput, bool) {
	if !availabilityDraftReady(d) {
		return AvailabilitySearchInput{}, false
	}
	input := AvailabilitySearchInput{
		Origin:      strings.TrimSpace(d.Origin),
		Destination: strings.TrimSpace(d.Destination),
		DateFrom:    d.DateFrom,
		DateTo:      d.DateTo,
		Qty:         d.Qty,
		Limit:       8,
	}
	input.PackageName = packageNameForRouteDirection(inferRouteDirectionFromDestination(input.Destination))
	if input.Qty <= 0 {
		input.Qty = 1
	}
	return enrichAvailabilitySearchInput(input), true
}

func loadAvailabilityDraft(session Session) (AvailabilityDraft, bool) {
	if draft, ok := loadAvailabilityDraftFromMap(asMap(session.Metadata["availability_draft"])); ok {
		return draft, true
	}
	return loadAvailabilityDraftFromPendingQuestion(session)
}

func loadAvailabilityDraftFromMap(raw map[string]interface{}) (AvailabilityDraft, bool) {
	if len(raw) == 0 {
		return AvailabilityDraft{}, false
	}
	status := strings.ToUpper(strings.TrimSpace(asString(raw["status"])))
	if status != availabilityDraftStatusActive {
		return AvailabilityDraft{}, false
	}

	draft := AvailabilityDraft{
		Status:       status,
		PeriodType:   strings.ToUpper(strings.TrimSpace(asString(raw["period_type"]))),
		Month:        readInt(raw["month"]),
		Year:         readInt(raw["year"]),
		Origin:       strings.TrimSpace(asString(raw["origin"])),
		Destination:  strings.TrimSpace(asString(raw["destination"])),
		Qty:          readInt(raw["qty"]),
		MissingSlots: asStringSlice(raw["missing_slots"]),
	}
	if parsed := parseAvailabilityDraftDate(asString(raw["date_from"])); parsed != nil {
		draft.DateFrom = parsed
	}
	if parsed := parseAvailabilityDraftDate(asString(raw["date_to"])); parsed != nil {
		draft.DateTo = parsed
	}
	if parsed := firstParsedTime(raw["updated_at"]); parsed != nil {
		draft.UpdatedAt = parsed.UTC()
	}
	if len(draft.MissingSlots) == 0 {
		draft.MissingSlots = availabilityDraftMissingSlots(draft)
	}
	return draft, true
}

func loadAvailabilityDraftFromPendingQuestion(session Session) (AvailabilityDraft, bool) {
	pending := asMap(session.Metadata["pending_question"])
	if len(pending) == 0 {
		return AvailabilityDraft{}, false
	}
	status := strings.ToUpper(strings.TrimSpace(asString(pending["status"])))
	if status != pendingQuestionStatusActive {
		return AvailabilityDraft{}, false
	}
	if !strings.EqualFold(strings.TrimSpace(asString(pending["intent"])), string(IntentAvailabilitySearch)) {
		return AvailabilityDraft{}, false
	}
	if !isAvailabilityPendingQuestionType(asString(pending["type"])) {
		return AvailabilityDraft{}, false
	}
	context := asMap(pending["context"])
	return loadAvailabilityDraftFromMap(asMap(context["availability_draft"]))
}

func availabilityDraftMetadata(d AvailabilityDraft) map[string]interface{} {
	d.Status = firstNonEmpty(strings.ToUpper(strings.TrimSpace(d.Status)), availabilityDraftStatusActive)
	d.PeriodType = strings.ToUpper(strings.TrimSpace(d.PeriodType))
	if d.UpdatedAt.IsZero() {
		d.UpdatedAt = time.Now().UTC()
	}
	d.MissingSlots = availabilityDraftMissingSlots(d)

	item := map[string]interface{}{
		"status":        d.Status,
		"period_type":   d.PeriodType,
		"month":         d.Month,
		"year":          d.Year,
		"origin":        strings.TrimSpace(d.Origin),
		"destination":   strings.TrimSpace(d.Destination),
		"qty":           d.Qty,
		"missing_slots": d.MissingSlots,
		"updated_at":    d.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if d.DateFrom != nil {
		item["date_from"] = d.DateFrom.UTC().Format("2006-01-02")
	}
	if d.DateTo != nil {
		item["date_to"] = d.DateTo.UTC().Format("2006-01-02")
	}
	return map[string]interface{}{"availability_draft": item}
}

func availabilityDraftWithPendingQuestionMetadata(d AvailabilityDraft, question string) map[string]interface{} {
	metadata := availabilityDraftMetadata(d)
	if len(availabilityDraftMissingSlots(d)) > 0 {
		metadata["pending_question"] = availabilityDraftPendingQuestion(d, question)
	}
	return metadata
}

func availabilityDraftWithCompletedPendingQuestionMetadata(d AvailabilityDraft) map[string]interface{} {
	metadata := availabilityDraftMetadata(d)
	now := d.UpdatedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	metadata["pending_question"] = map[string]interface{}{
		"status":         pendingQuestionStatusCompleted,
		"domain":         "availability",
		"intent":         string(IntentAvailabilitySearch),
		"expected_slots": []string{},
		"answered_at":    now.UTC().Format(time.RFC3339Nano),
	}
	return metadata
}

func availabilityDraftPendingQuestion(d AvailabilityDraft, question string) map[string]interface{} {
	askedAt := d.UpdatedAt
	if askedAt.IsZero() {
		askedAt = time.Now().UTC()
	}
	d.UpdatedAt = askedAt.UTC()
	return map[string]interface{}{
		"status":         pendingQuestionStatusActive,
		"type":           availabilityDraftPendingQuestionType(d),
		"domain":         "availability",
		"intent":         string(IntentAvailabilitySearch),
		"question":       strings.TrimSpace(question),
		"expected_slots": availabilityDraftMissingSlots(d),
		"asked_at":       askedAt.UTC().Format(time.RFC3339Nano),
		"context": map[string]interface{}{
			"availability_draft": availabilityDraftMetadata(d)["availability_draft"],
		},
	}
}

func clearAvailabilityDraftMetadata() map[string]interface{} {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return map[string]interface{}{
		"availability_draft": map[string]interface{}{
			"status":        availabilityDraftStatusCompleted,
			"missing_slots": []string{},
			"updated_at":    now,
		},
		"pending_question": map[string]interface{}{
			"status":         pendingQuestionStatusCompleted,
			"domain":         "availability",
			"intent":         string(IntentAvailabilitySearch),
			"expected_slots": []string{},
			"answered_at":    now,
		},
	}
}

func availabilityDraftPendingQuestionType(d AvailabilityDraft) string {
	missing := map[string]bool{}
	for _, slot := range availabilityDraftMissingSlots(d) {
		missing[slot] = true
	}
	switch {
	case missing["origin"] && missing["destination"] && missing["qty"]:
		return pendingQuestionAskAvailabilityRouteAndQty
	case missing["origin"] && missing["destination"]:
		return pendingQuestionAskAvailabilityRoute
	case missing["qty"]:
		return pendingQuestionAskAvailabilityQty
	case missing["origin"]:
		return pendingQuestionAskAvailabilityOrigin
	case missing["destination"]:
		return pendingQuestionAskAvailabilityDestination
	default:
		return ""
	}
}

func isAvailabilityPendingQuestionType(value string) bool {
	switch strings.TrimSpace(value) {
	case pendingQuestionAskAvailabilityRouteAndQty,
		pendingQuestionAskAvailabilityRoute,
		pendingQuestionAskAvailabilityQty,
		pendingQuestionAskAvailabilityOrigin,
		pendingQuestionAskAvailabilityDestination:
		return true
	default:
		return false
	}
}

func availabilityDraftTurnInterpretation(session Session, text string, observedAt time.Time) (map[string]interface{}, bool) {
	update, parsed := parseAvailabilityDraftFromText(text, observedAt)
	existing, hasExisting := loadAvailabilityDraft(session)
	if !hasExisting && (!parsed || !availabilityDraftCanStartFromText(update, text)) {
		return nil, false
	}
	if hasExisting && !parsed {
		return nil, false
	}
	merged := update
	source := "availability_draft_seed"
	if hasExisting {
		merged = mergeAvailabilityDraft(existing, update)
		source = "availability_draft_update"
	}
	return availabilityDraftInterpretationMetadata(merged, source), true
}

func (s *Service) resolveAvailabilityDraftTurn(ctx context.Context, session Session, history []Message, currentTurn string, observedAt time.Time) (Session, agentToolContext, *RunAgentResult, bool, error) {
	if shouldBypassAvailabilityDraftTurn(currentTurn) {
		updatedSession, err := s.store.UpdateSessionMetadata(ctx, UpdateSessionMetadataInput{
			SessionID: session.ID,
			Metadata:  clearAvailabilityDraftMetadata(),
		})
		if err != nil {
			return session, agentToolContext{}, nil, false, err
		}
		return updatedSession, agentToolContext{}, nil, false, nil
	}
	if availabilityDraftHasSelectedTrip(session, history, currentTurn) {
		return session, agentToolContext{}, nil, false, nil
	}

	update, parsed := parseAvailabilityDraftFromText(currentTurn, observedAt)
	existing, hasExisting := loadAvailabilityDraft(session)
	if !hasExisting {
		if !parsed || !availabilityDraftCanStartFromText(update, currentTurn) {
			return session, agentToolContext{}, nil, false, nil
		}
	} else if !parsed {
		existing.UpdatedAt = observedAt.UTC()
		existing.MissingSlots = availabilityDraftMissingSlots(existing)
		if availabilityDraftReady(existing) {
			return session, agentToolContext{}, nil, false, nil
		}
		updatedSession, err := s.store.UpdateSessionMetadata(ctx, UpdateSessionMetadataInput{
			SessionID: session.ID,
			Metadata:  availabilityDraftWithPendingQuestionMetadata(existing, buildAvailabilityDraftMissingSlotsReply(existing)),
		})
		if err != nil {
			return session, agentToolContext{}, nil, false, err
		}
		run := buildAvailabilityDraftMissingSlotsRun(existing)
		return updatedSession, agentToolContext{}, &run, true, nil
	}

	draft := update
	if hasExisting {
		draft = mergeAvailabilityDraft(existing, update)
	}
	draft.Status = availabilityDraftStatusActive
	draft.UpdatedAt = observedAt.UTC()
	draft.MissingSlots = availabilityDraftMissingSlots(draft)

	if !availabilityDraftReady(draft) {
		updatedSession, err := s.store.UpdateSessionMetadata(ctx, UpdateSessionMetadataInput{
			SessionID: session.ID,
			Metadata:  availabilityDraftWithPendingQuestionMetadata(draft, buildAvailabilityDraftMissingSlotsReply(draft)),
		})
		if err != nil {
			return session, agentToolContext{}, nil, false, err
		}
		run := buildAvailabilityDraftMissingSlotsRun(draft)
		return updatedSession, agentToolContext{}, &run, true, nil
	}

	input, ok := availabilityDraftToSearchInput(draft)
	if !ok {
		return session, agentToolContext{}, nil, false, nil
	}
	if !s.canSearchAvailability() {
		updatedSession, err := s.store.UpdateSessionMetadata(ctx, UpdateSessionMetadataInput{
			SessionID: session.ID,
			Metadata:  availabilityDraftWithCompletedPendingQuestionMetadata(draft),
		})
		if err != nil {
			return session, agentToolContext{}, nil, false, err
		}
		run := buildAvailabilityDraftSearchUnavailableRun(draft)
		return updatedSession, agentToolContext{}, &run, true, nil
	}

	context, err := s.executeAvailabilitySearchIntentTool(ctx, session, input)
	if err != nil {
		return session, context, nil, true, err
	}
	updatedSession, err := s.store.UpdateSessionMetadata(ctx, UpdateSessionMetadataInput{
		SessionID: session.ID,
		Metadata:  clearAvailabilityDraftMetadata(),
	})
	if err != nil {
		return session, context, nil, true, err
	}
	if context.Availability == nil {
		return updatedSession, context, nil, true, nil
	}

	decision := IntentDecision{
		Intent:            IntentAvailabilitySearch,
		Source:            "availability_draft",
		Action:            "tool",
		AvailabilityInput: &input,
	}
	run := buildAvailabilityTemplateDraftRun(decision, *context.Availability)
	return updatedSession, context, &run, true, nil
}

func shouldBypassAvailabilityDraftTurn(currentTurn string) bool {
	return looksLikeHumanSupportIntent(currentTurn) || looksLikeBookingCancelIntent(currentTurn)
}

func parseAvailabilityDraftRoute(text string) (string, string, bool) {
	locations := extractSupportedPackageLocationsInOrder(text)
	if len(locations) >= 2 {
		origin := strings.TrimSpace(locations[0])
		destination := strings.TrimSpace(locations[1])
		return origin, destination, origin != "" && destination != "" && !strings.EqualFold(origin, destination)
	}
	if origin, destination, ok := extractExplicitRouteFromText(text); ok {
		return origin, destination, true
	}
	return "", "", false
}

func parseAvailabilityDraftQuantity(text string) int {
	if qty := extractPassengerQuantity(text); qty > 0 {
		return qty
	}
	folded := strings.Join(strings.Fields(foldChatText(text)), " ")
	if folded == "" {
		return 0
	}
	if match := passengerWordQtyPattern.FindStringSubmatch(folded); len(match) == 2 {
		return passengerWordNumber(match[1])
	}
	return 0
}

func availabilityDraftCanStartFromText(update AvailabilityDraft, text string) bool {
	if update.DateFrom == nil || update.DateTo == nil {
		return false
	}
	folded := foldChatText(text)
	if strings.TrimSpace(folded) == "" {
		return false
	}
	if strings.TrimSpace(update.Origin) != "" || strings.TrimSpace(update.Destination) != "" {
		return true
	}
	return containsAnyFolded(
		strings.Join(strings.Fields(folded), " "),
		"passagem",
		"passagens",
		"datas",
		"data",
		"mes",
		"em "+availabilityDraftMonthName(update.Month),
		"para "+availabilityDraftMonthName(update.Month),
	)
}

func availabilityDraftHasSelectedTrip(session Session, history []Message, currentTurn string) bool {
	if extractSelectedOptionIndex(currentTurn) > 0 && hasPreviousAvailabilityList(history) {
		return true
	}
	draft := collectBookingDraftContext(session, history, currentTurn)
	return draft.SelectedOptionIndex > 0 || strings.TrimSpace(draft.TripID) != ""
}

func buildAvailabilityDraftMissingSlotsRun(draft AvailabilityDraft) RunAgentResult {
	reply := buildAvailabilityDraftMissingSlotsReply(draft)
	requestPayload := map[string]interface{}{
		"mode":               "TEMPLATE_FIRST_REPLY",
		"template_name":      availabilityDraftTemplateMissing,
		"intent":             string(IntentAvailabilitySearch),
		"action":             "clarify",
		"availability_draft": availabilityDraftMetadata(draft)["availability_draft"],
	}
	if len(availabilityDraftMissingSlots(draft)) > 0 {
		requestPayload["pending_question"] = availabilityDraftPendingQuestion(draft, reply)
	}
	return RunAgentResult{
		ReplyText:       reply,
		Model:           "template_realizer",
		RequestPayload:  requestPayload,
		ResponsePayload: requestPayload,
	}
}

func buildAvailabilityDraftSearchUnavailableRun(draft AvailabilityDraft) RunAgentResult {
	reply := buildAvailabilityDraftSearchUnavailableReply(draft)
	requestPayload := map[string]interface{}{
		"mode":               "TEMPLATE_FIRST_REPLY",
		"template_name":      availabilityDraftTemplateSearchUnavailable,
		"intent":             string(IntentAvailabilitySearch),
		"action":             "search_unavailable",
		"availability_draft": availabilityDraftMetadata(draft)["availability_draft"],
	}
	return RunAgentResult{
		ReplyText:       reply,
		Model:           "template_realizer",
		RequestPayload:  requestPayload,
		ResponsePayload: requestPayload,
	}
}

func buildAvailabilityDraftSearchUnavailableReply(draft AvailabilityDraft) string {
	route := availabilityDraftRouteText(draft)
	period := availabilityDraftPeriodText(draft)

	switch {
	case route != "" && period != "":
		return "Tenho os dados da busca para " + route + " em " + period + ", mas nao consegui consultar a disponibilidade agora. Tente novamente em instantes ou fale com o suporte."
	case route != "":
		return "Tenho os dados da busca para " + route + ", mas nao consegui consultar a disponibilidade agora. Tente novamente em instantes ou fale com o suporte."
	case period != "":
		return "Tenho o periodo da busca para " + period + ", mas nao consegui consultar a disponibilidade agora. Tente novamente em instantes ou fale com o suporte."
	default:
		return "Nao consegui consultar a disponibilidade agora. Tente novamente em instantes ou fale com o suporte."
	}
}

func buildAvailabilityDraftMissingSlotsReply(draft AvailabilityDraft) string {
	missing := map[string]bool{}
	for _, slot := range availabilityDraftMissingSlots(draft) {
		missing[slot] = true
	}
	period := availabilityDraftPeriodText(draft)
	route := availabilityDraftRouteText(draft)

	switch {
	case missing["origin"] && missing["destination"] && missing["qty"]:
		if period != "" {
			return "Para " + period + ", de qual cidade voce pretende sair e para qual cidade quer ir? Para quantas pessoas?"
		}
		return "De qual cidade voce pretende sair e para qual cidade quer ir? Para quantas pessoas?"
	case missing["origin"] && missing["destination"]:
		if period != "" {
			return "Para " + period + ", de qual cidade voce pretende sair e para qual cidade quer ir?"
		}
		return "De qual cidade voce pretende sair e para qual cidade quer ir?"
	case missing["qty"]:
		if route != "" && period != "" {
			return "Perfeito - " + route + " em " + period + ". Para quantas pessoas?"
		}
		if route != "" {
			return "Perfeito - " + route + ". Para quantas pessoas?"
		}
		return "Para quantas pessoas?"
	case missing["origin"]:
		if draft.Destination != "" && period != "" {
			return "Perfeito - destino " + draft.Destination + " em " + period + ". De qual cidade voce pretende sair?"
		}
		return "De qual cidade voce pretende sair?"
	case missing["destination"]:
		if draft.Origin != "" && period != "" {
			return "Perfeito - saindo de " + draft.Origin + " em " + period + ". Para qual cidade quer ir?"
		}
		return "Para qual cidade quer ir?"
	default:
		if route != "" && period != "" {
			return "Perfeito - " + route + " em " + period + ". Para quantas pessoas?"
		}
		return "Para qual trecho e para quantas pessoas?"
	}
}

func availabilityDraftInterpretationMetadata(draft AvailabilityDraft, source string) map[string]interface{} {
	item := availabilityDraftMetadata(draft)["availability_draft"]
	return map[string]interface{}{
		"source":             source,
		"intent":             string(IntentAvailabilitySearch),
		"availability_draft": item,
	}
}

func availabilityDraftPeriodText(draft AvailabilityDraft) string {
	if strings.EqualFold(strings.TrimSpace(draft.PeriodType), availabilityDraftPeriodMonth) && draft.Month > 0 {
		return availabilityDraftMonthName(draft.Month)
	}
	return ""
}

func availabilityDraftRouteText(draft AvailabilityDraft) string {
	origin := strings.TrimSpace(draft.Origin)
	destination := strings.TrimSpace(draft.Destination)
	if origin != "" && destination != "" {
		return origin + " para " + destination
	}
	return strings.TrimSpace(origin + destination)
}

func availabilityDraftMonthName(month int) string {
	names := []string{
		"",
		"janeiro",
		"fevereiro",
		"marco",
		"abril",
		"maio",
		"junho",
		"julho",
		"agosto",
		"setembro",
		"outubro",
		"novembro",
		"dezembro",
	}
	if month < 1 || month >= len(names) {
		return ""
	}
	return names[month]
}

func parseAvailabilityDraftDate(value string) *time.Time {
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
