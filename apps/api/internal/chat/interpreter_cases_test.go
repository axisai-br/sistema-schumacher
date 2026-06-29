package chat

import (
	"os"
	"strings"
	"testing"
)

func TestInterpreterCasesLoadVersionedCorpus(t *testing.T) {
	cases := loadInterpreterCasesForTest(t)
	raw, err := os.ReadFile(interpreterCasesFilePath)
	if err != nil {
		t.Fatalf("read interpreter cases JSONL: %v", err)
	}

	if got, want := len(cases), countInterpreterCaseLines(string(raw)); got != want {
		t.Fatalf("expected loader to return %d cases, got %d", want, got)
	}
	if len(cases) < 26 {
		t.Fatalf("expected at least 26 canonical cases, got %d", len(cases))
	}
}

func TestInterpreterCasesValidateRequiredFields(t *testing.T) {
	cases := loadInterpreterCasesForTest(t)
	seenIDs := map[string]struct{}{}
	for _, item := range cases {
		if item.ID == "" {
			t.Fatalf("case has empty id: %+v", item)
		}
		if _, exists := seenIDs[item.ID]; exists {
			t.Fatalf("duplicate case id %q", item.ID)
		}
		seenIDs[item.ID] = struct{}{}
		if item.Category == "" {
			t.Fatalf("case %s has empty category", item.ID)
		}
		if item.Phase == "" {
			t.Fatalf("case %s has empty phase", item.ID)
		}
		if item.ActivePromptKind == "" {
			t.Fatalf("case %s has empty active_prompt_kind", item.ID)
		}
		if item.CurrentTurn == "" {
			t.Fatalf("case %s has empty current_turn", item.ID)
		}
		if item.ExpectedIntent == "" {
			t.Fatalf("case %s has empty expected_intent", item.ID)
		}
		if item.Source == "" {
			t.Fatalf("case %s has empty source", item.ID)
		}
	}
}

func TestInterpreterCasesIncludeMinimumCategories(t *testing.T) {
	cases := loadInterpreterCasesForTest(t)
	seen := map[string]bool{}
	for _, item := range cases {
		seen[item.Category] = true
	}

	for _, category := range []string{
		"reservation_help",
		"unsupported_package",
		"availability_date",
		"availability_option",
		"contextual_fallback",
		"passenger_count",
		"child_under_5",
		"lap_child_assignment",
		"passenger_documents",
		"document_confirmation",
		"payment_preference",
		"payer_cpf",
		"human_support",
		"booking_cancel",
	} {
		if !seen[category] {
			t.Fatalf("expected category %q in interpreter corpus", category)
		}
	}
}

func TestInterpreterCasesIncludeProductionHotfixCases(t *testing.T) {
	cases := loadInterpreterCasesForTest(t)

	reservationHelp := interpreterCaseByID(t, cases, "reservation_help_001")
	if reservationHelp.CurrentTurn != "como faço pra fazer uma reserva?" {
		t.Fatalf("unexpected reservation help turn: %+v", reservationHelp)
	}
	if reservationHelp.ExpectedIntent != string(IntentAvailabilitySearch) ||
		reservationHelp.ExpectedTurnMeaning != string(TurnMeaningNewRequest) ||
		reservationHelp.ExpectedTemplate != string(TemplateAskReservationRouteSC) ||
		!interpreterCaseForbiddenIntent(reservationHelp, string(IntentUnsupportedPackage)) {
		t.Fatalf("reservation hotfix case is missing expected protection: %+v", reservationHelp)
	}

	for _, id := range []string{
		"unsupported_package_001",
		"unsupported_package_002",
		"unsupported_package_003",
	} {
		item := interpreterCaseByID(t, cases, id)
		if item.ExpectedIntent != string(IntentUnsupportedPackage) {
			t.Fatalf("expected unsupported package case %s to protect %s, got %+v", id, IntentUnsupportedPackage, item)
		}
	}
}

func TestInterpreterCasesRejectInvalidJSONL(t *testing.T) {
	_, err := LoadInterpreterCasesFromReader(strings.NewReader(`{"id":"ok","category":"reservation_help","phase":"DISCOVERY","active_prompt_kind":"UNKNOWN","current_turn":"oi","expected_intent":"GREETING","source":"test"}
{invalid-json}`))
	if err == nil {
		t.Fatal("expected invalid JSONL error")
	}
	if !strings.Contains(err.Error(), "line 2") || !strings.Contains(err.Error(), "invalid JSON") {
		t.Fatalf("expected clear invalid JSON error with line number, got %v", err)
	}
}

func TestInterpreterCasesRejectDuplicateIDs(t *testing.T) {
	_, err := LoadInterpreterCasesFromReader(strings.NewReader(`{"id":"duplicate","category":"reservation_help","phase":"DISCOVERY","active_prompt_kind":"UNKNOWN","current_turn":"oi","expected_intent":"GREETING","source":"test"}
{"id":"duplicate","category":"reservation_help","phase":"DISCOVERY","active_prompt_kind":"UNKNOWN","current_turn":"ola","expected_intent":"GREETING","source":"test"}`))
	if err == nil {
		t.Fatal("expected duplicate id error")
	}
	if !strings.Contains(err.Error(), "line 2") || !strings.Contains(err.Error(), "duplicate id") {
		t.Fatalf("expected duplicate id error with line number, got %v", err)
	}
}

func TestInterpreterCasesRejectMissingRequiredFields(t *testing.T) {
	_, err := LoadInterpreterCasesFromReader(strings.NewReader(`{"id":"missing-current-turn","category":"reservation_help","phase":"DISCOVERY","active_prompt_kind":"UNKNOWN","expected_intent":"GREETING","source":"test"}`))
	if err == nil {
		t.Fatal("expected missing required field error")
	}
	if !strings.Contains(err.Error(), "current_turn") {
		t.Fatalf("expected current_turn error, got %v", err)
	}
}

func loadInterpreterCasesForTest(t *testing.T) []InterpreterCase {
	t.Helper()
	cases, err := LoadInterpreterCases()
	if err != nil {
		t.Fatalf("load interpreter cases: %v", err)
	}
	return cases
}

func countInterpreterCaseLines(raw string) int {
	count := 0
	for _, line := range strings.Split(raw, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

func interpreterCaseByID(t *testing.T, cases []InterpreterCase, id string) InterpreterCase {
	t.Helper()
	for _, item := range cases {
		if item.ID == id {
			return item
		}
	}
	t.Fatalf("expected case %q in interpreter corpus", id)
	return InterpreterCase{}
}

func interpreterCaseForbiddenIntent(item InterpreterCase, intent string) bool {
	for _, forbidden := range item.ForbiddenIntents {
		if forbidden == intent {
			return true
		}
	}
	return false
}
