// NOTE: a deliberately narrow extension was added for atendimento v2 (migration
// 0023). Outside FUNCTION migrations it accepts ONLY: CREATE TABLE IF NOT EXISTS
// <name> (...) with nothing after the closing parenthesis, CREATE [UNIQUE] INDEX
// IF NOT EXISTS <name> ON <table> ..., and REVOKE ALL ON TABLE ... FROM anon,
// authenticated [, public]. Everything else (GRANT on tables, ALTER, DROP, CREATE
// TABLE AS, DML) stays unmodeled and fails closed. Nothing on the FUNCTION path
// was relaxed: mixing these statements with FUNCTIONs is rejected.

// Package migrationguard enforces the deliberately small FUNCTION-DDL policy
// introduced after migration 0022. Unsupported syntax fails closed and needs
// separate review; this package is not a general PostgreSQL parser.
package migrationguard

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const lastHistoricalMigration = 22

var (
	canonicalMigration    = regexp.MustCompile(`^([0-9]{4})_[a-zA-Z0-9][a-zA-Z0-9_.-]*\.sql$`)
	numericMigrationStart = regexp.MustCompile(`^[0-9]+`)
)

// CheckDir checks top-level numbered SQL migrations newer than 0022.
func CheckDir(dir string) []error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []error{fmt.Errorf("read migrations directory: %w", err)}
	}

	var names []string
	var findings []error
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".sql") {
			continue
		}
		number, ok := migrationNumber(entry.Name())
		if !ok {
			if numericMigrationStart.MatchString(entry.Name()) {
				findings = append(findings, fmt.Errorf("%s: numeric migration name must follow NNNN_description.sql", entry.Name()))
			}
			continue
		}
		if number > lastHistoricalMigration {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		contents, readErr := os.ReadFile(filepath.Join(dir, name))
		if readErr != nil {
			findings = append(findings, fmt.Errorf("%s: read migration: %w", name, readErr))
			continue
		}
		findings = append(findings, CheckSQL(name, string(contents))...)
	}
	return findings
}

// CheckSQL checks one migration already known to be newer than 0022.
func CheckSQL(filename, sql string) []error {
	statements, err := lexStatements(sql)
	if err != nil {
		return []error{fmt.Errorf("%s: lexical error: %w", filename, err)}
	}
	parsed := make([]parsedStatement, len(statements))
	hasFunction := false
	for index, statement := range statements {
		parsed[index] = parseStatement(statement)
		if parsed[index].kind == statementCreateFunction {
			hasFunction = true
		}
	}

	findings := globalFindings(filename, parsed, hasFunction)
	if !hasFunction {
		return append(findings, standaloneFunctionACLFindings(filename, parsed)...)
	}
	return append(findings, functionMigrationFindings(filename, parsed)...)
}

func standaloneFunctionACLFindings(filename string, statements []parsedStatement) []error {
	var findings []error
	for _, statement := range statements {
		if statement.kind != statementFunctionACL {
			continue
		}
		if statement.err != nil {
			findings = append(findings, fmt.Errorf("%s: %w", filename, statement.err))
			continue
		}
		findings = append(findings, fmt.Errorf("%s: ACL targets FUNCTION %s not created in the same migration", filename, statement.acl.signature))
	}
	return findings
}

func globalFindings(filename string, statements []parsedStatement, functionStateMachineActive bool) []error {
	var findings []error
	for _, statement := range statements {
		switch statement.kind {
		case statementUnknown:
			findings = append(findings, fmt.Errorf("%s: unmodeled statement at byte %d is not allowed after migration 0022", filename, statement.offset))
		case statementBegin, statementCommit, statementUnsupportedTransaction:
			if !functionStateMachineActive {
				findings = append(findings, fmt.Errorf("%s: transaction statement at byte %d is only allowed when consumed by canonical FUNCTION migration framing", filename, statement.offset))
			}
		case statementFunctionDefaults:
			findings = append(findings, fmt.Errorf("%s: ALTER DEFAULT PRIVILEGES for FUNCTION is outside lot 2A option B", filename))
		case statementAlterRoutine:
			findings = append(findings, fmt.Errorf("%s: ALTER FUNCTION/ROUTINE is outside lot 2A option B and requires separate authorization", filename))
		case statementCreateProcedure:
			findings = append(findings, fmt.Errorf("%s: CREATE PROCEDURE is outside lot 2A option B and requires separate authorization", filename))
		case statementExtensionMutation:
			findings = append(findings, fmt.Errorf("%s: EXTENSION DDL can create, move, update, or remove routines and requires separate authorization", filename))
		case statementDropRoutine:
			findings = append(findings, fmt.Errorf("%s: DROP FUNCTION/ROUTINE is outside lot 2A option B and requires separate authorization", filename))
		case statementAtendimentoTable:
			if functionStateMachineActive {
				findings = append(findings, fmt.Errorf("%s: table DDL at byte %d is not allowed in a migration that also declares FUNCTIONs", filename, statement.offset))
			}
		case statementDo:
			findings = append(findings, fmt.Errorf("%s: executable DO blocks cannot be validated safely after migration 0022", filename))
		case statementAlterGroup:
			findings = append(findings, fmt.Errorf("%s: ALTER GROUP role-membership mutations are not allowed after migration 0022", filename))
		case statementRoleMembership, statementExecutionRoleMutation:
			findings = append(findings, fmt.Errorf("%s: execution-role or role-membership mutations are not allowed after migration 0022", filename))
		case statementUnsupportedFunctionACL:
			findings = append(findings, fmt.Errorf("%s: unsupported FUNCTION/ROUTINE ACL; only exact EXECUTE forms are authorized", filename))
		}
	}
	return findings
}

func functionMigrationFindings(filename string, statements []parsedStatement) []error {
	var findings []error
	consumed := make([]bool, len(statements))
	if len(statements) == 0 {
		return []error{fmt.Errorf("%s: empty FUNCTION migration", filename)}
	}

	if statements[0].kind != statementBegin {
		findings = append(findings, fmt.Errorf("%s: FUNCTION migration must start with exact BEGIN", filename))
	} else {
		consumed[0] = true
		if !statements[0].terminated {
			findings = append(findings, fmt.Errorf("%s: BEGIN must end with semicolon", filename))
		}
	}
	last := len(statements) - 1
	if statements[last].kind != statementCommit {
		findings = append(findings, fmt.Errorf("%s: FUNCTION migration requires exact COMMIT or END as its final statement", filename))
	} else {
		consumed[last] = true
		if !statements[last].terminated {
			findings = append(findings, fmt.Errorf("%s: COMMIT or END must end with semicolon", filename))
		}
	}

	seen := make(map[string]bool)
	for index, statement := range statements {
		if statement.kind != statementCreateFunction {
			continue
		}
		consumed[index] = true
		declaration := statement.declaration
		if statement.err != nil {
			findings = append(findings, fmt.Errorf("%s: %w", filename, statement.err))
			continue
		}
		if !statement.terminated {
			findings = append(findings, fmt.Errorf("%s: FUNCTION %s statement must end with semicolon", filename, declaration.signature))
		}
		if declaration.orReplace {
			findings = append(findings, fmt.Errorf("%s: CREATE OR REPLACE FUNCTION %s requires a separate historical-routine authorization", filename, declaration.signature))
		}
		if declaration.schema == "" {
			findings = append(findings, fmt.Errorf("%s: FUNCTION %s must explicitly use schema public", filename, declaration.signature))
		} else if declaration.schema != "public" {
			findings = append(findings, fmt.Errorf("%s: FUNCTION %s is outside public and requires separate authorization", filename, declaration.signature))
		}
		if seen[declaration.signature] {
			findings = append(findings, fmt.Errorf("%s: FUNCTION %s is declared more than once", filename, declaration.signature))
		}
		seen[declaration.signature] = true

		revokeIndex := index + 1
		if revokeIndex >= len(statements) || !canonicalRevoke(statements[revokeIndex], declaration.signature) {
			findings = append(findings, fmt.Errorf("%s: FUNCTION %s must be followed by exact REVOKE EXECUTE FROM PUBLIC, anon, authenticated", filename, declaration.signature))
		} else {
			consumed[revokeIndex] = true
		}
		grantIndex := index + 2
		if grantIndex >= len(statements) || !canonicalGrant(statements[grantIndex], declaration.signature) {
			findings = append(findings, fmt.Errorf("%s: FUNCTION %s must be followed by direct GRANT EXECUTE TO service_role", filename, declaration.signature))
		} else {
			consumed[grantIndex] = true
		}
	}

	for index, statement := range statements {
		if consumed[index] {
			continue
		}
		switch statement.kind {
		case statementCreateFunction:
			// Already diagnosed above.
		case statementFunctionACL:
			findings = append(findings, extraACLFinding(filename, statement))
		case statementBegin, statementCommit, statementUnsupportedTransaction:
			findings = append(findings, fmt.Errorf("%s: FUNCTION migrations allow one exact BEGIN and one final COMMIT or END", filename))
		case statementFunctionDefaults, statementCreateProcedure, statementExtensionMutation, statementAlterRoutine, statementDropRoutine, statementDo,
			statementAlterGroup, statementRoleMembership, statementExecutionRoleMutation,
			statementUnsupportedFunctionACL, statementAtendimentoTable:
			// globalFindings already emitted the specific finding.
		case statementUnknown:
			// globalFindings already emitted the fail-closed finding.
		}
	}
	return findings
}

func extraACLFinding(filename string, statement parsedStatement) error {
	if statement.err != nil {
		return fmt.Errorf("%s: %w", filename, statement.err)
	}
	acl := statement.acl
	if acl.modifier {
		return fmt.Errorf("%s: FUNCTION %s ACL modifiers are not authorized", filename, acl.signature)
	}
	if acl.action == "grant" && !equalStrings(acl.roles, []string{"service_role"}) {
		return fmt.Errorf("%s: FUNCTION %s only permits direct GRANT EXECUTE TO service_role", filename, acl.signature)
	}
	if acl.action == "revoke" && !equalStrings(acl.roles, []string{"public", "anon", "authenticated"}) {
		return fmt.Errorf("%s: FUNCTION %s only permits REVOKE EXECUTE FROM PUBLIC, anon, authenticated", filename, acl.signature)
	}
	return fmt.Errorf("%s: FUNCTION ACL must be part of the adjacent canonical block for %s", filename, acl.signature)
}

func canonicalRevoke(statement parsedStatement, signature string) bool {
	return statement.kind == statementFunctionACL && statement.err == nil && statement.terminated &&
		statement.acl.action == "revoke" && statement.acl.signature == signature && !statement.acl.modifier &&
		equalStrings(statement.acl.roles, []string{"public", "anon", "authenticated"})
}

func canonicalGrant(statement parsedStatement, signature string) bool {
	return statement.kind == statementFunctionACL && statement.err == nil && statement.terminated &&
		statement.acl.action == "grant" && statement.acl.signature == signature && !statement.acl.modifier &&
		equalStrings(statement.acl.roles, []string{"service_role"})
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func migrationNumber(name string) (int, bool) {
	match := canonicalMigration.FindStringSubmatch(name)
	if match == nil {
		return 0, false
	}
	number, err := strconv.Atoi(match[1])
	return number, err == nil
}
