package migrationguard

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestRejectsInvalidFunctionMigrations(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want string
	}{
		{
			name: "public function without ACL",
			sql:  functionDDL("public.open_function()"),
			want: "must be followed by exact REVOKE EXECUTE",
		},
		{
			name: "function without schema",
			sql:  functionDDL("unqualified_function()"),
			want: "must explicitly use schema public",
		},
		{
			name: "incomplete hardening",
			sql: functionDDL("public.incomplete()") + `
revoke execute on function public.incomplete() from public, anon;
grant execute on function public.incomplete() to service_role;`,
			want: "authenticated",
		},
		{
			name: "missing service role policy",
			sql: functionDDL("public.no_service_role()") + `
revoke execute on function public.no_service_role() from public, anon, authenticated;`,
			want: "GRANT EXECUTE TO service_role",
		},
		{
			name: "overload with only one ACL",
			sql: functionDDL("public.overloaded(value text)") + functionDDL("public.overloaded(value integer)") + `
revoke execute on function public.overloaded(text) from public, anon, authenticated;
grant execute on function public.overloaded(text) to service_role;`,
			want: "public.overloaded(integer)",
		},
		{
			name: "function outside public",
			sql:  functionDDL("private.cross_schema()"),
			want: "outside public",
		},
		{
			name: "global default privileges",
			sql: `alter default privileges for role postgres
revoke execute on functions from public;`,
			want: "ALTER DEFAULT PRIVILEGES",
		},
		{
			name: "historical routine replacement",
			sql: `create or replace function public.refresh_manifest_data()
returns void language sql as $$ select $$;`,
			want: "separate historical-routine authorization",
		},
		{
			name: "wrong ACL signature",
			sql: functionDDL("public.signature_guard(value text)") + `
revoke execute on function public.signature_guard(integer) from public, anon, authenticated;
grant execute on function public.signature_guard(integer) to service_role;`,
			want: "public.signature_guard(text)",
		},
		{
			name: "hardening after transaction boundary",
			sql: functionDDL("public.non_atomic()") + `
commit;
revoke execute on function public.non_atomic() from public, anon, authenticated;
grant execute on function public.non_atomic() to service_role;`,
			want: "must start with exact BEGIN",
		},
		{
			name: "service role revoked after grant",
			sql: functionDDL("public.no_final_service_role()") + `
revoke execute on function public.no_final_service_role() from public, anon, authenticated;
grant execute on function public.no_final_service_role() to service_role;
revoke execute on function public.no_final_service_role() from service_role;`,
			want: "only permits REVOKE EXECUTE FROM PUBLIC, anon, authenticated",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			findings := CheckSQL("0024_fixture.sql", test.sql)
			if len(findings) == 0 {
				t.Fatalf("expected migration to be rejected")
			}
			if !strings.Contains(joinFindings(findings), test.want) {
				t.Fatalf("findings %q do not contain %q", joinFindings(findings), test.want)
			}
		})
	}
}

func TestAcceptsExactPublicFunctionPolicy(t *testing.T) {
	sql := `
begin;

-- DO $body$ BEGIN EXECUTE 'GRANT service_role TO anon'; END $body$;
-- Semicolons and policy-looking text in bodies must not confuse the gate.
create function public.secured_function(value text, retries integer default 1)
returns text
language plpgsql
as $body$
begin
  -- DO and ALTER GROUP text in a routine body are not migration commands.
  return value || '; alter default privileges on functions';
end;
$body$;

revoke execute on function public.secured_function(text, integer) from public, anon, authenticated;
grant execute on function public.secured_function(text, integer) to service_role;
commit;
`
	if findings := CheckSQL("0024_fixture.sql", sql); len(findings) != 0 {
		t.Fatalf("valid migration rejected: %s", joinFindings(findings))
	}
}

func TestAcceptsMultipleCanonicalFunctionBlocks(t *testing.T) {
	sql := `begin;
create function public.first_function()
returns void language sql as $$ select $$;
revoke execute on function public.first_function() from public, anon, authenticated;
grant execute on function public.first_function() to service_role;
create function public.second_function(value text)
returns text language sql as $$ select value $$;
revoke execute on function public.second_function(text) from public, anon, authenticated;
grant execute on function public.second_function(text) to service_role;
end;`

	if findings := CheckSQL("0024_fixture.sql", sql); len(findings) != 0 {
		t.Fatalf("multiple canonical FUNCTION blocks rejected: %s", joinFindings(findings))
	}
}

func TestRejectsSplitOrNonAdjacentACLBlock(t *testing.T) {
	sql := `begin;
create function public.split_acl()
returns void language sql as $$ select $$;
revoke execute on function public.split_acl() from public;
revoke execute on function public.split_acl() from anon, authenticated;
grant execute on function public.split_acl() to service_role;
commit;`

	findings := joinFindings(CheckSQL("0024_fixture.sql", sql))
	if !strings.Contains(findings, "must be followed by exact REVOKE") {
		t.Fatalf("split ACL block was accepted: %s", findings)
	}
}

func TestRejectsExecutableDoDollarQuotedBlock(t *testing.T) {
	sql := "begin;\n" + functionDDL("public.dynamic_acl_bypass()") + `
revoke execute on function public.dynamic_acl_bypass() from public, anon, authenticated;
grant execute on function public.dynamic_acl_bypass() to service_role;
do $body$
begin
  execute 'grant all privileges on function public.dynamic_acl_bypass() to anon';
end
$body$;
commit;`

	findings := joinFindings(CheckSQL("0024_fixture.sql", sql))
	if !strings.Contains(findings, "executable DO blocks") {
		t.Fatalf("dollar-quoted DO block was not rejected: %s", findings)
	}
}

func TestRejectsUnmodeledExecutableCalls(t *testing.T) {
	tests := []struct {
		name      string
		identity  string
		function  string
		statement string
	}{
		{
			name:     "select created function",
			identity: "dynamic_acl_call()",
			function: `create function public.dynamic_acl_call()
returns void language plpgsql as $body$
begin
  execute 'grant all privileges on function public.dynamic_acl_call() to anon';
end;
$body$;
`,
			statement: "select public.dynamic_acl_call();",
		},
		{
			name:      "call procedure",
			identity:  "call_guard()",
			function:  functionDDL("public.call_guard()"),
			statement: "call public.preexisting_procedure();",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sql := "begin;\n" + test.function + `
revoke execute on function public.` + test.identity + ` from public, anon, authenticated;
grant execute on function public.` + test.identity + ` to service_role;
` + test.statement + "\ncommit;"
			findings := joinFindings(CheckSQL("0024_fixture.sql", sql))
			if !strings.Contains(findings, "unmodeled statement") {
				t.Fatalf("unmodeled executable call %q was not rejected: %s", test.name, findings)
			}
		})
	}
}

func TestRejectsUnmodeledExecutableStatements(t *testing.T) {
	for _, test := range []struct {
		name      string
		statement string
	}{
		{name: "values", statement: "values (public.dynamic_acl_call());"},
		{name: "insert values", statement: "insert into audit_log(id) values (public.dynamic_acl_call());"},
		{name: "update", statement: "update audit_log set id = public.dynamic_acl_call();"},
		{name: "delete", statement: "delete from audit_log where public.dynamic_acl_call() is null;"},
		{name: "with", statement: "with invoked as (select public.dynamic_acl_call()) select 1;"},
		{name: "select literal", statement: "select 1;"},
	} {
		t.Run(test.name, func(t *testing.T) {
			sql := `begin;
create function public.dynamic_acl_call()
returns void language plpgsql as $body$
begin
  execute 'grant all privileges on function public.dynamic_acl_call() to anon';
end;
$body$;
revoke execute on function public.dynamic_acl_call() from public, anon, authenticated;
grant execute on function public.dynamic_acl_call() to service_role;
` + test.statement + `
commit;`
			findings := joinFindings(CheckSQL("0024_fixture.sql", sql))
			if !strings.Contains(findings, "unmodeled statement") {
				t.Fatalf("unmodeled executable statement %q was not rejected: %s", test.name, findings)
			}
		})
	}
}

func TestRejectsStatementAfterEscapeStringBody(t *testing.T) {
	sql := `begin;
create function public.escape_body()
returns void language sql as E'select 1 -- \'';
insert into public.probe(marker) values (E'\'');
revoke execute on function public.escape_body() from public, anon, authenticated;
grant execute on function public.escape_body() to service_role;
commit;`

	findings := joinFindings(CheckSQL("0024_fixture.sql", sql))
	if !strings.Contains(findings, "unmodeled statement") {
		t.Fatalf("statement after escape-string body was absorbed: %s", findings)
	}
}

func TestAcceptsEscapeStringFunctionBodyWithoutFollowingStatement(t *testing.T) {
	sql := `begin;
create function public.escape_body_only()
returns void language sql as E'select 1 -- \'';
revoke execute on function public.escape_body_only() from public, anon, authenticated;
grant execute on function public.escape_body_only() to service_role;
commit;`

	if findings := CheckSQL("0024_fixture.sql", sql); len(findings) != 0 {
		t.Fatalf("valid escape-string body was rejected: %s", joinFindings(findings))
	}
}

func TestAcceptsSimpleQuotedFunctionBody(t *testing.T) {
	sql := `begin;
create function public.simple_body()
returns text language sql as 'select ''ok''';
revoke execute on function public.simple_body() from public, anon, authenticated;
grant execute on function public.simple_body() to service_role;
commit;`

	if findings := CheckSQL("0024_fixture.sql", sql); len(findings) != 0 {
		t.Fatalf("valid simple-quoted body was rejected: %s", joinFindings(findings))
	}
}

func TestRejectsDollarQuoteInsideUnquotedIdentifier(t *testing.T) {
	sql := `begin;
create function public.cover$tag$()
returns void language sql as $$ select $$;
create function public.cover()
returns void language sql as $$ select $$;
create function public.decoy$tag$()
returns void language sql as $$ select $$;
revoke execute on function public.cover() from public, anon, authenticated;
grant execute on function public.cover() to service_role;
commit;`

	findings := joinFindings(CheckSQL("0024_fixture.sql", sql))
	if findings == "" {
		t.Fatal("dollar-quote text inside unquoted identifiers hid unhardened FUNCTIONs")
	}
	if !strings.Contains(findings, "without dollar signs") {
		t.Fatalf("dollar-sign identifiers were not rejected canonically: %s", findings)
	}
	statements, err := lexStatements(sql)
	if err != nil {
		t.Fatalf("lex mandatory RED: %v", err)
	}
	if len(statements) != 7 {
		t.Fatalf("mandatory RED has %d statements after lexing, want 7", len(statements))
	}
}

func TestRejectsDollarQuoteAfterPostgreSQLIdentifierContinuation(t *testing.T) {
	sql := "create table public.cover\u0301$tag$(id int); grant service_role to anon; create table public.decoy\u0301$tag$(id int);"

	findings := joinFindings(CheckSQL("0024_fixture.sql", sql))
	if !strings.Contains(findings, "role-membership mutations") {
		t.Fatalf("combining identifier continuation hid membership mutation: %s", findings)
	}
	statements, err := lexStatements(sql)
	if err != nil {
		t.Fatalf("lex PostgreSQL identifier boundary RED: %v", err)
	}
	if len(statements) != 3 {
		t.Fatalf("PostgreSQL-valid RED has %d statements after lexing, want 3", len(statements))
	}
}

func TestRejectsACLForDifferentTypeTokenization(t *testing.T) {
	sql := `begin;
create function public.f(value double precision)
returns void language sql as $$ select $$;
revoke execute on function public.f(doubleprecision) from public, anon, authenticated;
grant execute on function public.f(doubleprecision) to service_role;
commit;`

	findings := joinFindings(CheckSQL("0024_fixture.sql", sql))
	if !strings.Contains(findings, "public.f(double precision)") {
		t.Fatalf("ACL for distinct overload was accepted as exact hardening: %s", findings)
	}
}

func TestAcceptsExactCanonicalTypeIdentities(t *testing.T) {
	tests := []struct {
		name        string
		declaration string
		acl         string
	}{
		{name: "compound type with optional name", declaration: "value double precision", acl: "double precision"},
		{name: "schema qualified", declaration: "value pg_catalog.int8", acl: "pg_catalog.int8"},
		{name: "array", declaration: "values text[]", acl: "text[]"},
		{name: "typmod", declaration: "amount numeric(10, 2)", acl: "numeric(10, 2)"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sql := "begin;\n" + functionDDL("public.type_identity("+test.declaration+")") + `
revoke execute on function public.type_identity(` + test.acl + `) from public, anon, authenticated;
grant execute on function public.type_identity(` + test.acl + `) to service_role;
commit;`
			if findings := CheckSQL("0024_fixture.sql", sql); len(findings) != 0 {
				t.Fatalf("exact canonical type identity was rejected: %s", joinFindings(findings))
			}
		})
	}
}

func TestCheckSQLFailsClosedOnLexicalError(t *testing.T) {
	findings := joinFindings(CheckSQL("0024_fixture.sql", `begin; select $body$unterminated`))
	if !strings.Contains(findings, "lexical error") || !strings.Contains(findings, "unterminated dollar-quoted string") {
		t.Fatalf("lexical error was not reported fail-closed: %s", findings)
	}
}

func TestAcceptsEmptyOrCommentOnlyMigration(t *testing.T) {
	for _, sql := range []string{
		"",
		" \t\n\r\f\v",
		"-- no executable statement\n",
		"/* outer /* nested */ comment */",
	} {
		if findings := CheckSQL("0024_empty.sql", sql); len(findings) != 0 {
			t.Fatalf("empty or comment-only migration was rejected: %s", joinFindings(findings))
		}
	}
}

func TestRejectsCreateProcedureAfter0022(t *testing.T) {
	findings := joinFindings(CheckSQL("0024_fixture.sql", `create procedure public.not_authorized() language sql as $$ select $$;`))
	if !strings.Contains(findings, "CREATE PROCEDURE") {
		t.Fatalf("CREATE PROCEDURE was not rejected: %s", findings)
	}
}

func TestRejectsCreateExtensionAfter0022(t *testing.T) {
	findings := joinFindings(CheckSQL("0024_fixture.sql", `create extension pgcrypto with schema public;`))
	if !strings.Contains(findings, "EXTENSION DDL") {
		t.Fatalf("CREATE EXTENSION was not rejected: %s", findings)
	}
}

func TestRejectsAlterDropExtensionAfter0022(t *testing.T) {
	for _, statement := range []string{
		"alter extension pgcrypto update;",
		"alter extension pgcrypto set schema public;",
		"drop extension pgcrypto cascade;",
	} {
		findings := joinFindings(CheckSQL("0024_fixture.sql", statement))
		if !strings.Contains(findings, "EXTENSION DDL") {
			t.Fatalf("EXTENSION mutation %q was not rejected: %s", statement, findings)
		}
	}
}

func TestRejectsVerticalTabSeparatedSecurityDDL(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want string
	}{
		{name: "create extension", sql: "create\vextension pgcrypto with schema public;", want: "EXTENSION DDL"},
		{name: "alter extension", sql: "alter\vextension pgcrypto update;", want: "EXTENSION DDL"},
		{name: "drop extension", sql: "drop\vextension pgcrypto cascade;", want: "EXTENSION DDL"},
		{name: "create function", sql: "create\vfunction public.vertical_tab() returns void language sql as $$ select $$;", want: "FUNCTION"},
		{name: "drop owned", sql: "drop\vowned by postgres cascade;", want: "execution-role or role-membership mutations"},
		{name: "reassign owned", sql: "reassign\vowned by postgres to anon;", want: "execution-role or role-membership mutations"},
		{name: "alter group", sql: "alter\vgroup service_role add user anon;", want: "ALTER GROUP"},
		{name: "drop function", sql: "drop\vfunction public.preexisting();", want: "DROP FUNCTION/ROUTINE"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			findings := joinFindings(CheckSQL("0024_fixture.sql", test.sql))
			if !strings.Contains(findings, test.want) {
				t.Fatalf("vertical tab bypassed %s classification: %s", test.name, findings)
			}
		})
	}
}

func TestRejectsUnknownStatementsAfter0022(t *testing.T) {
	tests := []struct {
		name string
		sql  string
	}{
		{name: "create aggregate", sql: "create aggregate public.total(integer) (sfunc = int4pl, stype = integer);"},
		{name: "alter aggregate", sql: "alter\taggregate public.total(integer) rename to renamed_total;"},
		{name: "drop aggregate", sql: "drop\naggregate public.total(integer);"},
		{name: "alter table add column", sql: "alter\rtable public.probe add\fcolumn marker integer;"},
		{name: "drop schema cascade", sql: "drop\vschema private cascade;"},
		{name: "create trigger", sql: "create trigger probe after insert on public.probe execute function public.audit_probe();"},
		{name: "executable DML", sql: "insert into public.probe(marker) values (1);"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			const filename = "0024_unknown.sql"
			findings := CheckSQL(filename, test.sql)
			if len(findings) != 1 {
				t.Fatalf("got %d findings, want 1: %s", len(findings), joinFindings(findings))
			}
			want := filename + ": unmodeled statement at byte 0"
			if !strings.Contains(findings[0].Error(), want) {
				t.Fatalf("finding %q does not contain filename and position %q", findings[0], want)
			}
		})
	}
}

func TestRejectsUnknownStatementsAcrossPostgreSQLWhitespace(t *testing.T) {
	for _, separator := range []struct {
		name  string
		value string
	}{
		{name: "space", value: " "},
		{name: "horizontal tab", value: "\t"},
		{name: "newline", value: "\n"},
		{name: "carriage return", value: "\r"},
		{name: "form feed", value: "\f"},
		{name: "vertical tab", value: "\v"},
	} {
		t.Run(separator.name, func(t *testing.T) {
			sql := "create" + separator.value + "aggregate public.total(integer) (sfunc = int4pl, stype = integer);"
			if findings := CheckSQL("0024_unknown.sql", sql); len(findings) != 1 {
				t.Fatalf("got %d findings, want 1: %s", len(findings), joinFindings(findings))
			}
		})
	}
}

func TestReportsEveryUnknownStatementWithItsPosition(t *testing.T) {
	const filename = "0024_multiple_unknown.sql"
	sql := `create aggregate public.total(integer) (sfunc = int4pl, stype = integer);

alter table public.probe add column marker integer;`

	findings := CheckSQL(filename, sql)
	if len(findings) != 2 {
		t.Fatalf("got %d findings, want 2: %s", len(findings), joinFindings(findings))
	}
	for index, offset := range []int{0, strings.Index(sql, "alter table")} {
		want := filename + ": unmodeled statement at byte " + strconv.Itoa(offset)
		if !strings.Contains(findings[index].Error(), want) {
			t.Fatalf("finding %d %q does not contain %q", index, findings[index], want)
		}
	}
}

func TestRejectsACLThatRestoresRestrictedAccess(t *testing.T) {
	sql := "begin;\n" + functionDDL("public.regression()") + `
revoke execute on function public.regression() from public, anon, authenticated;
grant execute on function public.regression() to service_role;
	grant execute on function public.regression() to authenticated;
commit;`
	findings := CheckSQL("0024_fixture.sql", sql)
	if !strings.Contains(joinFindings(findings), "only permits direct GRANT EXECUTE TO service_role") {
		t.Fatalf("restricted grant was not rejected: %s", joinFindings(findings))
	}
}

func TestRejectsUnsupportedFunctionACLVariant(t *testing.T) {
	tests := []struct {
		name      string
		statement string
		want      string
	}{
		{
			name:      "grant all privileges",
			statement: "grant all privileges on function public.acl_variant() to anon;",
			want:      "unsupported FUNCTION/ROUTINE ACL",
		},
		{
			name:      "routine ACL",
			statement: "grant execute on routine public.acl_variant() to anon;",
			want:      "unsupported FUNCTION/ROUTINE ACL",
		},
		{
			name:      "grant other role",
			statement: "grant execute on function public.acl_variant() to reporting_role;",
			want:      "only permits direct GRANT EXECUTE TO service_role",
		},
		{
			name:      "revoke service role",
			statement: "revoke execute on function public.acl_variant() from service_role;",
			want:      "only permits REVOKE EXECUTE FROM PUBLIC, anon, authenticated",
		},
		{
			name:      "revoke cascade",
			statement: "revoke execute on function public.acl_variant() from anon cascade;",
			want:      "ACL modifiers are not authorized",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sql := "begin;\n" + functionDDL("public.acl_variant()") + `
revoke execute on function public.acl_variant() from public, anon, authenticated;
grant execute on function public.acl_variant() to service_role;
` + test.statement + "\ncommit;"
			findings := joinFindings(CheckSQL("0024_fixture.sql", sql))
			if !strings.Contains(findings, test.want) {
				t.Fatalf("ACL variant was not rejected; want %q in %s", test.want, findings)
			}
		})
	}
}

func TestRejectsPostCreationFunctionMutations(t *testing.T) {
	tests := []struct {
		name     string
		mutation string
	}{
		{name: "owner", mutation: "alter function public.mutated() owner to anon;"},
		{name: "schema", mutation: "alter function public.mutated() set schema private;"},
		{name: "routine owner", mutation: "alter routine public.mutated() owner to anon;"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sql := "begin;\n" + functionDDL("public.mutated()") + `
revoke execute on function public.mutated() from public, anon, authenticated;
grant execute on function public.mutated() to service_role;
` + test.mutation + "\ncommit;"
			findings := joinFindings(CheckSQL("0024_fixture.sql", sql))
			if !strings.Contains(findings, "ALTER FUNCTION/ROUTINE") {
				t.Fatalf("post-creation %s mutation was not rejected: %s", test.name, findings)
			}
		})
	}
}

func TestRejectsDropRoutineMutations(t *testing.T) {
	for _, test := range []struct {
		name      string
		statement string
	}{
		{name: "function", statement: "drop function public.preexisting();"},
		{name: "routine", statement: "drop routine public.preexisting();"},
		{name: "procedure", statement: "drop procedure public.preexisting();"},
	} {
		t.Run(test.name, func(t *testing.T) {
			sql := "begin;\n" + functionDDL("public.drop_guard()") + `
revoke execute on function public.drop_guard() from public, anon, authenticated;
grant execute on function public.drop_guard() to service_role;
	` + test.statement + "\ncommit;"
			findings := joinFindings(CheckSQL("0024_fixture.sql", sql))
			if !strings.Contains(findings, "DROP FUNCTION/ROUTINE") {
				t.Fatalf("routine removal %q was not rejected: %s", test.statement, findings)
			}
		})
	}
}

func TestRejectsExecutionRoleAndMembershipMutations(t *testing.T) {
	tests := []struct {
		name   string
		before string
		after  string
		want   string
	}{
		{name: "set role", before: "set role anon;", want: "execution-role or role-membership mutations"},
		{name: "session authorization", before: "set session authorization anon;", want: "execution-role or role-membership mutations"},
		{name: "reassign owner", after: "reassign owned by postgres to anon;", want: "execution-role or role-membership mutations"},
		{name: "role membership", after: "grant service_role to anon;", want: "execution-role or role-membership mutations"},
		{name: "alter group add user", after: "alter group service_role add user anon;", want: "ALTER GROUP role-membership mutations"},
		{name: "alter group drop user", after: "alter group service_role drop user authenticated;", want: "ALTER GROUP role-membership mutations"},
		{name: "create group", after: "create group elevated;", want: "execution-role or role-membership mutations"},
		{name: "drop group", after: "drop group elevated;", want: "execution-role or role-membership mutations"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sql := "begin;\n" + test.before + "\n" + functionDDL("public.owner_guard()") + `
revoke execute on function public.owner_guard() from public, anon, authenticated;
grant execute on function public.owner_guard() to service_role;
` + test.after + "\ncommit;"
			findings := joinFindings(CheckSQL("0024_fixture.sql", sql))
			if !strings.Contains(findings, test.want) {
				t.Fatalf("authority mutation %q was not rejected: %s", test.name, findings)
			}
		})
	}
}

func TestRejectsDropOwnedWithoutFunctionDDL(t *testing.T) {
	findings := joinFindings(CheckSQL("0024_fixture.sql", "drop owned by postgres cascade;"))
	if !strings.Contains(findings, "execution-role or role-membership mutations") {
		t.Fatalf("DROP OWNED was not rejected as a preexisting-object mutation: %s", findings)
	}
}

func TestRejectsAlterGroupMembershipWithoutFunctionDDL(t *testing.T) {
	for _, statement := range []string{
		"alter group service_role add user anon;",
		"alter group service_role drop user authenticated;",
	} {
		findings := joinFindings(CheckSQL("0024_fixture.sql", statement))
		if !strings.Contains(findings, "ALTER GROUP role-membership mutations") {
			t.Fatalf("standalone membership mutation %q was not rejected: %s", statement, findings)
		}
	}
}

func TestRejectsGrantRevokeMembershipWithoutFunctionDDL(t *testing.T) {
	for _, test := range []struct {
		name      string
		statement string
	}{
		{name: "grant", statement: "grant service_role to anon;"},
		{name: "revoke", statement: "revoke service_role from anon;"},
		{name: "grant to quoted on", statement: `grant service_role to "on";`},
		{name: "revoke from quoted on", statement: `revoke service_role from "on";`},
		{name: "quoted comment marker before grant", statement: "create table \"x--y\"(id int); grant service_role to anon;"},
	} {
		t.Run(test.name, func(t *testing.T) {
			findings := joinFindings(CheckSQL("0024_fixture.sql", test.statement))
			if !strings.Contains(findings, "role-membership mutations") {
				t.Fatalf("standalone membership mutation %q was not rejected: %s", test.statement, findings)
			}
		})
	}
}

func TestClassifiesObjectACLAsUnknownNotRoleMembership(t *testing.T) {
	sql := `grant select on table "on" to anon;`
	findings := joinFindings(CheckSQL("0024_fixture.sql", sql))
	if !strings.Contains(findings, "unmodeled statement") {
		t.Fatalf("unknown object ACL did not fail closed: %s", findings)
	}
	if strings.Contains(findings, "role-membership mutations") {
		t.Fatalf("object ACL was confused with role membership: %s", findings)
	}
}

func TestRejectsFunctionWithoutExplicitTransaction(t *testing.T) {
	acl := `
revoke execute on function public.non_transactional() from public, anon, authenticated;
grant execute on function public.non_transactional() to service_role;`
	tests := []struct {
		name string
		sql  string
		want string
	}{
		{
			name: "no begin",
			sql:  functionDDL("public.non_transactional()") + acl,
			want: "must start with exact BEGIN",
		},
		{
			name: "commit before ACL",
			sql:  "begin;\n" + functionDDL("public.non_transactional()") + "commit;\n" + acl,
			want: "requires exact COMMIT or END as its final statement",
		},
		{
			name: "missing commit",
			sql:  "begin;\n" + functionDDL("public.non_transactional()") + acl,
			want: "requires exact COMMIT or END",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			findings := joinFindings(CheckSQL("0024_fixture.sql", test.sql))
			if !strings.Contains(findings, test.want) {
				t.Fatalf("transaction violation was not rejected; want %q in %s", test.want, findings)
			}
		})
	}
}

func TestRejectsTransactionsWithoutFunction(t *testing.T) {
	for _, test := range []struct {
		name string
		sql  string
	}{
		{name: "begin", sql: "begin;"},
		{name: "commit", sql: "commit;"},
		{name: "end", sql: "end;"},
		{name: "start transaction", sql: "start transaction;"},
		{name: "rollback", sql: "rollback;"},
		{name: "abort", sql: "abort;"},
	} {
		t.Run(test.name, func(t *testing.T) {
			const filename = "0024_transaction.sql"
			findings := CheckSQL(filename, test.sql)
			if len(findings) != 1 {
				t.Fatalf("got %d findings, want 1: %s", len(findings), joinFindings(findings))
			}
			want := filename + ": transaction statement at byte 0"
			if !strings.Contains(findings[0].Error(), want) {
				t.Fatalf("finding %q does not contain filename and position %q", findings[0], want)
			}
		})
	}
}

func TestRejectsTransactionPairsWithoutFunction(t *testing.T) {
	for _, test := range []struct {
		name string
		sql  string
	}{
		{name: "begin commit", sql: "begin; commit;"},
		{name: "begin end", sql: "begin; end;"},
	} {
		t.Run(test.name, func(t *testing.T) {
			findings := CheckSQL("0024_transaction.sql", test.sql)
			if len(findings) != 2 {
				t.Fatalf("got %d findings, want 2: %s", len(findings), joinFindings(findings))
			}
			for index, offset := range []int{0, strings.Index(test.sql, " ") + 1} {
				want := "transaction statement at byte " + strconv.Itoa(offset)
				if !strings.Contains(findings[index].Error(), want) {
					t.Fatalf("finding %d %q does not contain %q", index, findings[index], want)
				}
			}
		})
	}
}

func TestRejectsTransactionAdjacentToUnknownStatement(t *testing.T) {
	for _, test := range []struct {
		name string
		sql  string
	}{
		{name: "transaction before unknown", sql: "begin; create table public.probe(id integer);"},
		{name: "transaction after unknown", sql: "create table public.probe(id integer); commit;"},
	} {
		t.Run(test.name, func(t *testing.T) {
			findings := joinFindings(CheckSQL("0024_transaction.sql", test.sql))
			if !strings.Contains(findings, "transaction statement") || !strings.Contains(findings, "unmodeled statement") {
				t.Fatalf("transaction and unknown statement did not both fail closed: %s", findings)
			}
		})
	}
}

func TestTransactionPolicyPreservesCanonicalFunctionFraming(t *testing.T) {
	sql := "begin;\n" + functionDDL("public.transaction_guard()") + `
revoke execute on function public.transaction_guard() from public, anon, authenticated;
grant execute on function public.transaction_guard() to service_role;
commit;`
	if findings := CheckSQL("0024_transaction.sql", sql); len(findings) != 0 {
		t.Fatalf("canonical FUNCTION framing was rejected: %s", joinFindings(findings))
	}
}

func TestRejectsIncompleteOrExtraFunctionTransactionFraming(t *testing.T) {
	canonicalBlock := "begin;\n" + functionDDL("public.transaction_guard()") + `
revoke execute on function public.transaction_guard() from public, anon, authenticated;
grant execute on function public.transaction_guard() to service_role;`
	for _, test := range []struct {
		name string
		sql  string
	}{
		{name: "missing final transaction", sql: canonicalBlock},
		{name: "extra rollback", sql: canonicalBlock + "\nrollback;\ncommit;"},
		{name: "extra start transaction", sql: "start transaction;\n" + canonicalBlock + "\ncommit;"},
	} {
		t.Run(test.name, func(t *testing.T) {
			findings := joinFindings(CheckSQL("0024_transaction.sql", test.sql))
			if !strings.Contains(findings, "FUNCTION migration") && !strings.Contains(findings, "FUNCTION migrations") {
				t.Fatalf("invalid FUNCTION transaction framing was accepted: %s", findings)
			}
		})
	}
}

func TestAcceptsEndAsCommitAlias(t *testing.T) {
	sql := "begin;\n" + functionDDL("public.end_alias()") + `
revoke execute on function public.end_alias() from public, anon, authenticated;
grant execute on function public.end_alias() to service_role;
end;`
	if findings := CheckSQL("0024_fixture.sql", sql); len(findings) != 0 {
		t.Fatalf("END transaction alias rejected: %s", joinFindings(findings))
	}
}

func TestCheckDirEnforcesOnlyMigrationsAfter0022(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "0022_historical.sql", functionDDL("historical_unqualified()")+"create extension pgcrypto with schema public;")
	writeFixture(t, dir, "0024_valid.sql", "begin;\n"+functionDDL("public.future()")+`
revoke execute on function public.future() from public, anon, authenticated;
grant execute on function public.future() to service_role;
commit;`)
	writeFixture(t, dir, "README.sql", functionDDL("ignored_unversioned()"))

	if findings := CheckDir(dir); len(findings) != 0 {
		t.Fatalf("directory rejected: %s", joinFindings(findings))
	}
}

func TestCheckDirRejectsInvalidMigrationAfter0022(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "0023_invalid.sql", functionDDL("public.not_hardened()"))

	findings := CheckDir(dir)
	if len(findings) == 0 || !strings.Contains(joinFindings(findings), "public.not_hardened()") {
		t.Fatalf("new invalid migration was not rejected: %s", joinFindings(findings))
	}
}

func TestCheckDirRejectsExtensionAfter0022(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "0023_extension.sql", "create extension pgcrypto with schema public;")

	findings := joinFindings(CheckDir(dir))
	if !strings.Contains(findings, "EXTENSION DDL") {
		t.Fatalf("versioned EXTENSION migration was not rejected: %s", findings)
	}
}

func TestCheckDirRejectsMalformedNumericMigrationNames(t *testing.T) {
	for _, name := range []string{
		"0023.sql",
		"0023-add_function.sql",
		"1_unhardened.sql",
		"999999999999999999999999_unhardened.sql",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeFixture(t, dir, name, functionDDL("public.filename_bypass()"))
			findings := joinFindings(CheckDir(dir))
			if !strings.Contains(findings, "numeric migration name") {
				t.Fatalf("malformed numeric migration %q was ignored: %s", name, findings)
			}
		})
	}
}

func TestRepositoryMigrationsFollowFunctionPolicy(t *testing.T) {
	if findings := CheckDir(filepath.Join("..", "..", "migrations")); len(findings) != 0 {
		t.Fatalf("repository migration gate failed:\n%s", joinFindings(findings))
	}
}

func functionDDL(identity string) string {
	return "create function " + identity + `
returns void language sql as $$ select $$;
`
}

func writeFixture(t *testing.T, dir, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func joinFindings(findings []error) string {
	parts := make([]string, len(findings))
	for index, finding := range findings {
		parts[index] = finding.Error()
	}
	return strings.Join(parts, "\n")
}

func TestAcceptsAtendimentoTableDDL(t *testing.T) {
	sql := `create table if not exists atd_x (
  id uuid primary key default gen_random_uuid(),
  a text not null default 'x',
  b uuid references other(id) on delete cascade,
  unique (a, b)
);
create index if not exists idx_atd_x_a on atd_x (a) where a is not null;
create unique index if not exists uq_atd_x_b on public.atd_x (b);
revoke all on table atd_x, atd_y from anon, authenticated;
revoke all on table public.atd_x from public, anon, authenticated;`
	if findings := CheckSQL("0024_tables.sql", sql); len(findings) != 0 {
		t.Fatalf("unexpected findings: %s", joinFindings(findings))
	}
}

func TestRejectsOutsideAtendimentoTableDDL(t *testing.T) {
	tests := map[string]string{
		"grant select":           "grant select on table x to anon;",
		"grant all":              "grant all on table x to service_role;",
		"alter table":            "alter table x add column y int;",
		"drop table":             "drop table if exists x;",
		"create without if":      "create table x (id int);",
		"create table as":        "create table if not exists x as select 1;",
		"create table as parens": "create table if not exists x (id int) as select 1;",
		"trailing clause":        "create table if not exists x (id int) partition by range (id);",
		"trailing inherits":      "create table if not exists x (id int) inherits (y);",
		"temp table":             "create temp table if not exists x (id int);",
		"quoted name":            `create table if not exists "X" (id int);`,
		"index without if":       "create index idx on x (a);",
		"index without name":     "create index if not exists on x (a);",
		"concurrently":           "create index concurrently if not exists idx on x (a);",
		"revoke other role":      "revoke all on table x from service_role;",
		"revoke extra role":      "revoke all on table x from anon, authenticated, service_role;",
		"revoke only anon":       "revoke all on table x from anon;",
		"revoke select":          "revoke select on table x from anon, authenticated;",
		"revoke cascade":         "revoke all on table x from anon, authenticated cascade;",
		"revoke no table kw":     "revoke all on x from anon, authenticated;",
		"insert":                 "insert into x values (1);",
	}
	for name, sql := range tests {
		t.Run(name, func(t *testing.T) {
			findings := CheckSQL("0024_bad.sql", sql)
			if len(findings) == 0 || !strings.Contains(joinFindings(findings), "unmodeled statement") {
				t.Fatalf("expected unmodeled-statement finding, got: %s", joinFindings(findings))
			}
		})
	}
}

func TestRejectsAtendimentoTableDDLMixedWithFunction(t *testing.T) {
	sql := "begin;\n" + functionDDL("public.mixed()") + `
revoke execute on function public.mixed() from public, anon, authenticated;
grant execute on function public.mixed() to service_role;
create table if not exists atd_x (id int);
commit;`
	findings := joinFindings(CheckSQL("0024_mixed.sql", sql))
	if !strings.Contains(findings, "table DDL") {
		t.Fatalf("mixed migration was not rejected: %s", findings)
	}
}

func TestAtendimentoMigration0023PassesGuard(t *testing.T) {
	contents, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0023_atendimento_v2.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if findings := CheckSQL("0023_atendimento_v2.sql", string(contents)); len(findings) != 0 {
		t.Fatalf("0023 rejected: %s", joinFindings(findings))
	}
}
