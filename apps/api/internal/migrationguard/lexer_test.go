package migrationguard

import (
	"strings"
	"testing"
)

func TestLexerDollarQuoteBoundaries(t *testing.T) {
	sql := `select public.cover$tag$();
select ($tag$body; -- opaque$tag$);
select $1;
select $$plain; body$$;
select $outer$nested $inner$ text $inner$ body$outer$;`
	statements, err := lexStatements(sql)
	if err != nil {
		t.Fatalf("lex dollar boundaries: %v", err)
	}
	if len(statements) != 5 {
		t.Fatalf("got %d statements, want 5", len(statements))
	}
	if got := statements[0].tokens[3]; got.kind != tokenWord || got.value != "cover$tag$" {
		t.Fatalf("identifier containing dollar tag split incorrectly: %#v", got)
	}
	if !hasTokenKind(statements[1].tokens, tokenDollarString) {
		t.Fatal("valid tagged dollar string was not tokenized")
	}
	if got := statements[2].tokens[1]; got.kind != tokenSymbol || got.value != "$1" {
		t.Fatalf("positional parameter classified incorrectly: %#v", got)
	}
	if !hasTokenKind(statements[3].tokens, tokenDollarString) {
		t.Fatal("valid untagged dollar string was not tokenized")
	}
	if !hasTokenKind(statements[4].tokens, tokenDollarString) {
		t.Fatal("nested differently tagged text escaped the outer dollar string")
	}
}

func TestLexerPostgreSQLMultibyteIdentifierBoundary(t *testing.T) {
	sql := "create table public.cover\u0301$tag$(id int); grant service_role to anon; create table public.decoy\u0301$tag$(id int);"
	statements, err := lexStatements(sql)
	if err != nil {
		t.Fatalf("lex PostgreSQL multibyte identifier: %v", err)
	}
	if len(statements) != 3 {
		t.Fatalf("got %d statements, want 3", len(statements))
	}
	if got := statements[0].tokens[4]; got.kind != tokenWord || got.raw != "cover\u0301$tag$" {
		t.Fatalf("multibyte identifier containing dollar tag split incorrectly: %#v", got)
	}
}

func TestLexerStringsQuotedIdentifiersAndComments(t *testing.T) {
	sql := `select 'semi; and ''quote''', E'backslash\\ and \\''';
select "x--y;$tag$""z";
/* outer ; /* nested ; */ still outer */ select 1;
-- ignored ; $tag$
select 2;`
	statements, err := lexStatements(sql)
	if err != nil {
		t.Fatalf("lex strings, identifiers, and comments: %v", err)
	}
	if len(statements) != 4 {
		t.Fatalf("got %d statements, want 4", len(statements))
	}
	if !hasTokenKind(statements[0].tokens, tokenString) {
		t.Fatal("string literals were not preserved as tokens")
	}
	if !hasTokenKind(statements[1].tokens, tokenQuotedIdentifier) {
		t.Fatal("quoted identifier was not preserved as a token")
	}
}

func TestLexerPostgreSQLWhitespace(t *testing.T) {
	for _, test := range []struct {
		name      string
		separator string
	}{
		{name: "space", separator: " "},
		{name: "horizontal tab", separator: "\t"},
		{name: "newline", separator: "\n"},
		{name: "carriage return", separator: "\r"},
		{name: "form feed", separator: "\f"},
		{name: "vertical tab", separator: "\v"},
		{name: "combined", separator: " \t\n\r\f\v"},
	} {
		t.Run(test.name, func(t *testing.T) {
			statements, err := lexStatements("create" + test.separator + "extension pgcrypto;")
			if err != nil {
				t.Fatalf("lex PostgreSQL whitespace: %v", err)
			}
			if len(statements) != 1 || len(statements[0].tokens) != 3 ||
				!statements[0].tokens[0].isKeyword("create") || !statements[0].tokens[1].isKeyword("extension") {
				t.Fatalf("separator %q did not preserve keyword boundaries: %#v", test.separator, statements)
			}
		})
	}
}

func TestLexerVerticalTabInsideOpaqueContexts(t *testing.T) {
	sql := "select\v'plain\v;';\nselect\v\"quoted\v;\";\n-- comment \v ;\nselect\v1;\n/* block \v ; */ select\v$body$body \v ;$body$;"
	statements, err := lexStatements(sql)
	if err != nil {
		t.Fatalf("lex vertical tabs in opaque contexts: %v", err)
	}
	if len(statements) != 4 {
		t.Fatalf("got %d statements, want 4", len(statements))
	}
	if !hasTokenKind(statements[0].tokens, tokenString) ||
		!hasTokenKind(statements[1].tokens, tokenQuotedIdentifier) ||
		!hasTokenKind(statements[3].tokens, tokenDollarString) {
		t.Fatalf("opaque contexts were not preserved: %#v", statements)
	}
	for _, statement := range statements {
		for _, token := range statement.tokens {
			if token.raw == "\v" {
				t.Fatalf("vertical tab escaped whitespace handling at byte %d", token.offset)
			}
		}
	}
}

func TestLexerFailsClosedOnUnterminatedInput(t *testing.T) {
	for _, test := range []struct {
		name string
		sql  string
	}{
		{name: "standard string", sql: "select 'unterminated"},
		{name: "escape string", sql: `select E'unterminated\`},
		{name: "quoted identifier", sql: `select "unterminated`},
		{name: "block comment", sql: `select 1 /* unterminated`},
		{name: "dollar string", sql: `select $tag$unterminated`},
		{name: "case-sensitive dollar tag", sql: `select $Tag$body$tag$;`},
		{name: "NUL in literal", sql: "select 'before\x00after';"},
		{name: "invalid UTF-8 in comment", sql: "select 1; -- " + string([]byte{0xff})},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := lexStatements(test.sql); err == nil {
				t.Fatalf("unterminated %s was accepted", test.name)
			}
		})
	}
}

func TestLexerRejectsAmbiguousBackslashInStandardString(t *testing.T) {
	if _, err := lexStatements(`select 'ambiguous\\value';`); err == nil || !strings.Contains(err.Error(), "use E") {
		t.Fatalf("standard string backslash did not fail closed: %v", err)
	}
}

func TestLexerReportsTokenOffsets(t *testing.T) {
	tokens, err := lexSQL("  begin;\ncommit;")
	if err != nil {
		t.Fatal(err)
	}
	want := []int{2, 7, 9, 15}
	if len(tokens) != len(want) {
		t.Fatalf("got %d tokens, want %d", len(tokens), len(want))
	}
	for index, offset := range want {
		if tokens[index].offset != offset {
			t.Fatalf("token %d offset = %d, want %d", index, tokens[index].offset, offset)
		}
	}
}

func FuzzLexer(f *testing.F) {
	for _, seed := range []string{
		"begin; commit;",
		`select public.cover$tag$(); select $tag$body;$tag$;`,
		`select E'\\''; select 'a''b';`,
		`select "x--y;$tag$"; /* outer /* inner */ outer */ select 1;`,
		"select $1; select $$body;$$;",
		"create\vextension pgcrypto; drop\vowned by postgres;",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, sql string) {
		tokens, err := lexSQL(sql)
		if err != nil {
			return
		}
		previousEnd := 0
		for index, token := range tokens {
			if token.offset < previousEnd {
				t.Fatalf("token %d overlaps previous token", index)
			}
			if token.raw == "" || token.offset+len(token.raw) > len(sql) {
				t.Fatalf("token %d has invalid span: %#v", index, token)
			}
			if sql[token.offset:token.offset+len(token.raw)] != token.raw {
				t.Fatalf("token %d raw text does not match source", index)
			}
			previousEnd = token.offset + len(token.raw)
		}
		if _, err := lexStatements(sql); err != nil {
			t.Fatalf("second lexical pass diverged: %v", err)
		}
	})
}

func hasTokenKind(tokens []sqlToken, kind tokenKind) bool {
	for _, token := range tokens {
		if token.kind == kind {
			return true
		}
	}
	return false
}
