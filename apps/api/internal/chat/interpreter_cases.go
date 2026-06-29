package chat

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const interpreterCasesFilePath = "testdata/interpreter_cases.jsonl"

type InterpreterCase struct {
	ID                          string   `json:"id"`
	Category                    string   `json:"category"`
	Phase                       string   `json:"phase"`
	ActivePromptKind            string   `json:"active_prompt_kind"`
	CurrentTurn                 string   `json:"current_turn"`
	ExpectedIntent              string   `json:"expected_intent"`
	ExpectedTurnMeaning         string   `json:"expected_turn_meaning,omitempty"`
	ExpectedTemplate            string   `json:"expected_template,omitempty"`
	ExpectedAction              string   `json:"expected_action,omitempty"`
	ExpectedSelectedOptionIndex *int     `json:"expected_selected_option_index,omitempty"`
	ExpectedPaymentPreference   string   `json:"expected_payment_preference,omitempty"`
	ExpectedPassengerCount      *int     `json:"expected_passenger_count,omitempty"`
	ExpectedChildUnder5Count    *int     `json:"expected_child_under_5_count,omitempty"`
	FallbackTemplate            string   `json:"fallback_template,omitempty"`
	ForbiddenIntents            []string `json:"forbidden_intents,omitempty"`
	Source                      string   `json:"source"`
	Notes                       string   `json:"notes,omitempty"`
}

func LoadInterpreterCases() ([]InterpreterCase, error) {
	file, err := openInterpreterCasesFile()
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return LoadInterpreterCasesFromReader(file)
}

func LoadInterpreterCasesFromReader(reader io.Reader) ([]InterpreterCase, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	seenIDs := map[string]struct{}{}
	var cases []InterpreterCase
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			return nil, fmt.Errorf("interpreter case line %d: empty line", lineNumber)
		}

		var item InterpreterCase
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			return nil, fmt.Errorf("interpreter case line %d: invalid JSON: %w", lineNumber, err)
		}
		item = normalizeInterpreterCase(item)
		if err := validateInterpreterCase(item); err != nil {
			return nil, fmt.Errorf("interpreter case line %d: %w", lineNumber, err)
		}
		if _, exists := seenIDs[item.ID]; exists {
			return nil, fmt.Errorf("interpreter case line %d: duplicate id %q", lineNumber, item.ID)
		}
		seenIDs[item.ID] = struct{}{}
		cases = append(cases, item)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan interpreter cases: %w", err)
	}

	return cases, nil
}

func openInterpreterCasesFile() (*os.File, error) {
	file, err := os.Open(interpreterCasesFilePath)
	if err == nil {
		return file, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("open interpreter cases: %w", err)
	}

	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		return nil, fmt.Errorf("open interpreter cases: %w", err)
	}
	file, sourceErr := os.Open(filepath.Join(filepath.Dir(sourceFile), interpreterCasesFilePath))
	if sourceErr != nil {
		return nil, fmt.Errorf("open interpreter cases: %w", sourceErr)
	}
	return file, nil
}

func validateInterpreterCase(item InterpreterCase) error {
	required := map[string]string{
		"id":                 item.ID,
		"category":           item.Category,
		"phase":              item.Phase,
		"active_prompt_kind": item.ActivePromptKind,
		"current_turn":       item.CurrentTurn,
		"expected_intent":    item.ExpectedIntent,
		"source":             item.Source,
	}
	for field, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("missing required field %q", field)
		}
	}
	return nil
}

func normalizeInterpreterCase(item InterpreterCase) InterpreterCase {
	item.ID = strings.TrimSpace(item.ID)
	item.Category = strings.TrimSpace(item.Category)
	item.Phase = strings.TrimSpace(item.Phase)
	item.ActivePromptKind = strings.TrimSpace(item.ActivePromptKind)
	item.CurrentTurn = strings.TrimSpace(item.CurrentTurn)
	item.ExpectedIntent = strings.TrimSpace(item.ExpectedIntent)
	item.ExpectedTurnMeaning = strings.TrimSpace(item.ExpectedTurnMeaning)
	item.ExpectedTemplate = strings.TrimSpace(item.ExpectedTemplate)
	item.ExpectedAction = strings.TrimSpace(item.ExpectedAction)
	item.ExpectedPaymentPreference = strings.TrimSpace(item.ExpectedPaymentPreference)
	item.FallbackTemplate = strings.TrimSpace(item.FallbackTemplate)
	item.Source = strings.TrimSpace(item.Source)
	item.Notes = strings.TrimSpace(item.Notes)
	item.ForbiddenIntents = trimInterpreterCaseStrings(item.ForbiddenIntents)
	return item
}

func trimInterpreterCaseStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}
