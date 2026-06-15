package bookings

import (
	"os"
	"strings"
	"testing"
)

func TestPassengerAdditionalIdentityMigrationUsesDateAndConstraints(t *testing.T) {
	sql := readTestFile(t, "../../migrations/0020_passenger_additional_identity_fields.sql")

	mustContainAll(t, sql,
		"add column if not exists cpf text",
		"add column if not exists rg text",
		"add column if not exists cnh text",
		"add column if not exists birth_date date",
		"add column if not exists birth_certificate_number text",
		"add column if not exists birth_city text",
		"cpf ~ '^[0-9]{11}$'",
		"birth_certificate_number ~ '^[0-9]{32}$'",
		"birth_date between date '1900-01-01' and current_date",
		"alter column birth_date type date",
	)
	if strings.Contains(strings.ToLower(sql), "birth_date text") {
		t.Fatalf("birth_date must be DATE, not text")
	}
}

func TestRepositoryPassengerSQLPersistsAndReadsAdditionalIdentityFields(t *testing.T) {
	source := readTestFile(t, "repository.go")

	mustContainAll(t, source,
		"document, document_type, cpf, rg, cnh",
		"nullif($6, ''), nullif($7, ''), nullif($8, '')",
		"nullif($9, '')::date",
		"inputPassenger.CPF, inputPassenger.RG, inputPassenger.CNH",
		"coalesce(p.cpf, ''), coalesce(p.rg, ''), coalesce(p.cnh, '')",
		"coalesce(to_char(p.birth_date, 'YYYY-MM-DD'), '')",
	)
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}

func mustContainAll(t *testing.T, text string, parts ...string) {
	t.Helper()
	for _, part := range parts {
		if !strings.Contains(text, part) {
			t.Fatalf("expected text to contain %q", part)
		}
	}
}
