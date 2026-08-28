package migrationguard

import (
	"fmt"
	"strings"
)

type statementKind uint8

const (
	statementUnknown statementKind = iota
	statementBegin
	statementCommit
	statementUnsupportedTransaction
	statementCreateFunction
	statementCreateProcedure
	statementExtensionMutation
	statementFunctionACL
	statementFunctionDefaults
	statementAlterRoutine
	statementDropRoutine
	statementDo
	statementAlterGroup
	statementRoleMembership
	statementExecutionRoleMutation
	statementUnsupportedFunctionACL
)

type parsedStatement struct {
	kind        statementKind
	declaration declaration
	acl         aclChange
	err         error
	offset      int
	terminated  bool
}

type declaration struct {
	schema, name, signature string
	orReplace               bool
}

type aclChange struct {
	action, signature string
	roles             []string
	modifier          bool
}

func parseStatement(statement sqlStatement) parsedStatement {
	parsed := parsedStatement{offset: statement.offset, terminated: statement.terminated}
	tokens := statement.tokens
	if exactKeywords(tokens, "begin") {
		parsed.kind = statementBegin
		return parsed
	}
	if exactKeywords(tokens, "commit") || exactKeywords(tokens, "end") {
		parsed.kind = statementCommit
		return parsed
	}
	if startsWithAnyKeyword(tokens, "begin", "start", "commit", "end", "rollback", "abort") {
		parsed.kind = statementUnsupportedTransaction
		return parsed
	}
	if startsWithKeywords(tokens, "alter", "default", "privileges") && containsKeywordSequence(tokens, "on", "functions") ||
		startsWithKeywords(tokens, "alter", "default", "privileges") && containsKeywordSequence(tokens, "on", "routines") {
		parsed.kind = statementFunctionDefaults
		return parsed
	}
	if startsWithKeywords(tokens, "alter", "function") || startsWithKeywords(tokens, "alter", "routine") || startsWithKeywords(tokens, "alter", "procedure") {
		parsed.kind = statementAlterRoutine
		return parsed
	}
	if startsWithKeywords(tokens, "drop", "function") || startsWithKeywords(tokens, "drop", "routine") || startsWithKeywords(tokens, "drop", "procedure") {
		parsed.kind = statementDropRoutine
		return parsed
	}
	if startsWithKeywords(tokens, "do") {
		parsed.kind = statementDo
		return parsed
	}
	if startsWithKeywords(tokens, "alter", "group") {
		parsed.kind = statementAlterGroup
		return parsed
	}
	if executionRoleMutation(tokens) {
		parsed.kind = statementExecutionRoleMutation
		return parsed
	}
	if startsWithAnyPair(tokens, []string{"create", "alter", "drop"}, []string{"extension"}) {
		parsed.kind = statementExtensionMutation
		return parsed
	}
	if startsWithKeywords(tokens, "create", "procedure") || startsWithKeywords(tokens, "create", "or", "replace", "procedure") {
		parsed.kind = statementCreateProcedure
		return parsed
	}

	declaration, isFunction, err := parseCreateFunction(tokens)
	if isFunction {
		parsed.kind = statementCreateFunction
		parsed.declaration = declaration
		parsed.err = err
		return parsed
	}
	acl, isACL, err := parseFunctionACL(tokens)
	if isACL {
		parsed.kind = statementFunctionACL
		parsed.acl = acl
		parsed.err = err
		return parsed
	}
	if functionACLMutation(tokens) {
		parsed.kind = statementUnsupportedFunctionACL
		return parsed
	}
	if roleMembershipMutation(tokens) {
		parsed.kind = statementRoleMembership
		return parsed
	}
	return parsed
}

func parseCreateFunction(tokens []sqlToken) (declaration, bool, error) {
	if len(tokens) < 2 || !tokens[0].isKeyword("create") {
		return declaration{}, false, nil
	}
	index := 1
	orReplace := false
	if index+1 < len(tokens) && tokens[index].isKeyword("or") && tokens[index+1].isKeyword("replace") {
		orReplace = true
		index += 2
	}
	if index >= len(tokens) || !tokens[index].isKeyword("function") {
		return declaration{}, false, nil
	}
	index++
	if index >= len(tokens) || tokens[index].kind != tokenWord {
		return declaration{orReplace: orReplace}, true, fmt.Errorf("unsupported FUNCTION declaration; use unquoted schema-qualified canonical DDL")
	}
	first := tokens[index].value
	index++
	schema, name := "", first
	if index < len(tokens) && tokens[index].isSymbol(".") {
		index++
		if index >= len(tokens) || tokens[index].kind != tokenWord {
			return declaration{schema: first, orReplace: orReplace}, true, fmt.Errorf("unsupported FUNCTION declaration; use unquoted schema-qualified canonical DDL")
		}
		schema, name = first, tokens[index].value
		index++
	}
	result := declaration{schema: schema, name: name, orReplace: orReplace}
	if !isCanonicalIdentifier(name) || schema != "" && !isCanonicalIdentifier(schema) {
		return result, true, fmt.Errorf("unsupported FUNCTION declaration; use canonical unquoted identifiers without dollar signs")
	}
	if index >= len(tokens) || !tokens[index].isSymbol("(") {
		return result, true, fmt.Errorf("unsupported FUNCTION %s: missing argument list", qualified(schema, name))
	}
	arguments, close, err := parenthesizedTokens(tokens, index)
	if err != nil {
		return result, true, fmt.Errorf("unsupported FUNCTION %s: %w", qualified(schema, name), err)
	}
	if close >= len(tokens) {
		return result, true, fmt.Errorf("unsupported FUNCTION %s: missing definition after argument list", qualified(schema, name))
	}
	types, err := declarationTypes(arguments)
	if err != nil {
		return result, true, fmt.Errorf("unsupported FUNCTION %s arguments: %w", qualified(schema, name), err)
	}
	result.signature = signature(qualified(schema, name), types)
	return result, true, nil
}

func parseFunctionACL(tokens []sqlToken) (aclChange, bool, error) {
	if len(tokens) < 4 || !tokens[0].isKeyword("grant") && !tokens[0].isKeyword("revoke") ||
		!tokens[1].isKeyword("execute") || !tokens[2].isKeyword("on") || !tokens[3].isKeyword("function") {
		return aclChange{}, false, nil
	}
	action := tokens[0].value
	index := 4
	if index+3 >= len(tokens) || tokens[index].kind != tokenWord || !tokens[index+1].isSymbol(".") || tokens[index+2].kind != tokenWord || !tokens[index+3].isSymbol("(") {
		return aclChange{}, true, fmt.Errorf("unsupported FUNCTION ACL; use an exact unquoted public signature")
	}
	schema, name := tokens[index].value, tokens[index+2].value
	if !isCanonicalIdentifier(schema) || !isCanonicalIdentifier(name) {
		return aclChange{}, true, fmt.Errorf("unsupported FUNCTION ACL; use canonical unquoted identifiers without dollar signs")
	}
	arguments, close, err := parenthesizedTokens(tokens, index+3)
	if err != nil {
		return aclChange{}, true, fmt.Errorf("unsupported FUNCTION ACL: %w", err)
	}
	types, err := aclTypes(arguments)
	if err != nil {
		return aclChange{}, true, fmt.Errorf("unsupported FUNCTION ACL arguments: %w", err)
	}
	boundary := "from"
	if action == "grant" {
		boundary = "to"
	}
	if close >= len(tokens) || !tokens[close].isKeyword(boundary) {
		return aclChange{}, true, fmt.Errorf("FUNCTION ACL must use %s followed by a comma-separated role list", strings.ToUpper(boundary))
	}
	roles, modifier, err := parseRoleList(tokens[close+1:])
	if err != nil {
		return aclChange{}, true, err
	}
	return aclChange{
		action:    action,
		signature: signature(schema+"."+name, types),
		roles:     roles,
		modifier:  modifier,
	}, true, nil
}

func declarationTypes(tokens []sqlToken) ([]string, error) {
	parts, err := splitTokenArguments(tokens)
	if err != nil {
		return nil, err
	}
	var types []string
	for _, part := range parts {
		part = stripTokenDefault(part)
		if len(part) == 0 {
			return nil, fmt.Errorf("empty argument")
		}
		mode := ""
		if part[0].kind == tokenWord && isOneOf(part[0].value, "in", "out", "inout", "variadic") {
			mode = part[0].value
			part = part[1:]
		}
		if mode == "out" {
			continue
		}
		if len(part) == 0 {
			return nil, fmt.Errorf("missing argument type")
		}
		if hasArgumentName(part) {
			part = part[1:]
		}
		normalized, err := normalizeType(part)
		if err != nil {
			return nil, err
		}
		types = append(types, normalized)
	}
	return types, nil
}

func aclTypes(tokens []sqlToken) ([]string, error) {
	parts, err := splitTokenArguments(tokens)
	if err != nil {
		return nil, err
	}
	types := make([]string, 0, len(parts))
	for _, part := range parts {
		normalized, err := normalizeType(part)
		if err != nil {
			return nil, err
		}
		types = append(types, normalized)
	}
	return types, nil
}

func splitTokenArguments(tokens []sqlToken) ([][]sqlToken, error) {
	if len(tokens) == 0 {
		return nil, nil
	}
	var parts [][]sqlToken
	start, parentheses, brackets := 0, 0, 0
	for index, token := range tokens {
		switch {
		case token.isSymbol("("):
			parentheses++
		case token.isSymbol(")"):
			parentheses--
		case token.isSymbol("["):
			brackets++
		case token.isSymbol("]"):
			brackets--
		case token.isSymbol(",") && parentheses == 0 && brackets == 0:
			parts = append(parts, tokens[start:index])
			start = index + 1
		}
		if parentheses < 0 || brackets < 0 {
			return nil, fmt.Errorf("unbalanced argument type")
		}
	}
	if parentheses != 0 || brackets != 0 {
		return nil, fmt.Errorf("unbalanced argument type")
	}
	parts = append(parts, tokens[start:])
	return parts, nil
}

func stripTokenDefault(tokens []sqlToken) []sqlToken {
	parentheses, brackets := 0, 0
	for index, token := range tokens {
		switch {
		case token.isSymbol("("):
			parentheses++
		case token.isSymbol(")"):
			parentheses--
		case token.isSymbol("["):
			brackets++
		case token.isSymbol("]"):
			brackets--
		case parentheses == 0 && brackets == 0 && (token.isSymbol("=") || token.isKeyword("default")):
			return tokens[:index]
		}
	}
	return tokens
}

func hasArgumentName(tokens []sqlToken) bool {
	if len(tokens) < 2 || tokens[0].kind != tokenWord {
		return false
	}
	if isCompoundTypeStart(tokens) || tokens[1].isSymbol(".") || tokens[1].isSymbol("[") || tokens[1].isSymbol("(") {
		return false
	}
	return true
}

func isCompoundTypeStart(tokens []sqlToken) bool {
	if len(tokens) < 2 || tokens[0].kind != tokenWord || tokens[1].kind != tokenWord {
		return false
	}
	first, second := tokens[0].value, tokens[1].value
	return first == "interval" || first == "double" && second == "precision" ||
		first == "character" && second == "varying" ||
		first == "bit" && second == "varying" ||
		(first == "timestamp" || first == "time") && (second == "with" || second == "without")
}

func normalizeType(tokens []sqlToken) (string, error) {
	if len(tokens) == 0 {
		return "", fmt.Errorf("empty argument type")
	}
	var result strings.Builder
	previousWordLike := false
	for _, token := range tokens {
		switch token.kind {
		case tokenWord, tokenNumber:
			if token.kind == tokenWord && !isCanonicalIdentifier(token.value) {
				return "", fmt.Errorf("non-canonical type token %q is not supported", token.raw)
			}
			if previousWordLike {
				result.WriteByte(' ')
			}
			result.WriteString(token.value)
			previousWordLike = true
		case tokenSymbol:
			if !isOneOf(token.value, ".", "[", "]", "(", ")", ",") {
				return "", fmt.Errorf("unsupported token %q in argument type", token.raw)
			}
			result.WriteString(token.value)
			previousWordLike = false
		default:
			return "", fmt.Errorf("quoted or literal argument type is not supported")
		}
	}
	if result.Len() == 0 {
		return "", fmt.Errorf("empty argument type")
	}
	return result.String(), nil
}

func parenthesizedTokens(tokens []sqlToken, open int) ([]sqlToken, int, error) {
	depth := 0
	for index := open; index < len(tokens); index++ {
		switch {
		case tokens[index].isSymbol("("):
			depth++
		case tokens[index].isSymbol(")"):
			depth--
			if depth == 0 {
				return tokens[open+1 : index], index + 1, nil
			}
		}
	}
	return nil, 0, fmt.Errorf("unterminated argument list")
}

func parseRoleList(tokens []sqlToken) ([]string, bool, error) {
	if len(tokens) == 0 {
		return nil, false, fmt.Errorf("FUNCTION ACL role list is empty")
	}
	var roles []string
	expectRole := true
	for index := 0; index < len(tokens); index++ {
		token := tokens[index]
		if expectRole {
			if token.kind != tokenWord || !isCanonicalIdentifier(token.value) {
				return nil, false, fmt.Errorf("FUNCTION ACL role list must contain unquoted role names")
			}
			roles = append(roles, token.value)
			expectRole = false
			continue
		}
		if token.isSymbol(",") {
			expectRole = true
			continue
		}
		modifier := containsKeywordSequence(tokens[index:], "with", "grant", "option") ||
			token.isKeyword("cascade") || token.isKeyword("restrict")
		if modifier {
			return roles, true, nil
		}
		return nil, false, fmt.Errorf("FUNCTION ACL role list must contain comma-separated unquoted role names")
	}
	if expectRole {
		return nil, false, fmt.Errorf("FUNCTION ACL role list has a trailing comma")
	}
	return roles, false, nil
}

func functionACLMutation(tokens []sqlToken) bool {
	if len(tokens) == 0 || !tokens[0].isKeyword("grant") && !tokens[0].isKeyword("revoke") {
		return false
	}
	for index := 1; index < len(tokens); index++ {
		if !tokens[index].isKeyword("on") {
			continue
		}
		next := index + 1
		if next < len(tokens) && tokens[next].isKeyword("all") {
			next++
		}
		return next < len(tokens) && isOneOf(tokens[next].value, "function", "functions", "routine", "routines", "procedure", "procedures") && tokens[next].kind == tokenWord
	}
	return false
}

func roleMembershipMutation(tokens []sqlToken) bool {
	if len(tokens) == 0 || !tokens[0].isKeyword("grant") && !tokens[0].isKeyword("revoke") {
		return false
	}
	boundary := "to"
	if tokens[0].isKeyword("revoke") {
		boundary = "from"
	}
	for _, token := range tokens[1:] {
		if token.isKeyword("on") {
			return false
		}
		if token.isKeyword(boundary) {
			return true
		}
	}
	return false
}

func executionRoleMutation(tokens []sqlToken) bool {
	return startsWithKeywords(tokens, "set", "role") ||
		startsWithKeywords(tokens, "set", "local", "role") ||
		startsWithKeywords(tokens, "set", "session", "role") ||
		startsWithKeywords(tokens, "set", "session", "authorization") ||
		startsWithKeywords(tokens, "set", "local", "session", "authorization") ||
		startsWithKeywords(tokens, "set", "session", "session", "authorization") ||
		startsWithKeywords(tokens, "reset", "role") ||
		startsWithKeywords(tokens, "reset", "session", "authorization") ||
		startsWithKeywords(tokens, "reassign", "owned") ||
		startsWithKeywords(tokens, "drop", "owned") ||
		startsWithAnyPair(tokens, []string{"create", "alter", "drop"}, []string{"role", "user", "group"})
}

func exactKeywords(tokens []sqlToken, values ...string) bool {
	return len(tokens) == len(values) && startsWithKeywords(tokens, values...)
}

func startsWithKeywords(tokens []sqlToken, values ...string) bool {
	if len(tokens) < len(values) {
		return false
	}
	for index, value := range values {
		if !tokens[index].isKeyword(value) {
			return false
		}
	}
	return true
}

func startsWithAnyKeyword(tokens []sqlToken, values ...string) bool {
	if len(tokens) == 0 {
		return false
	}
	for _, value := range values {
		if tokens[0].isKeyword(value) {
			return true
		}
	}
	return false
}

func startsWithAnyPair(tokens []sqlToken, first, second []string) bool {
	if len(tokens) < 2 || tokens[0].kind != tokenWord || tokens[1].kind != tokenWord {
		return false
	}
	return isOneOf(tokens[0].value, first...) && isOneOf(tokens[1].value, second...)
}

func containsKeywordSequence(tokens []sqlToken, values ...string) bool {
	for index := 0; index+len(values) <= len(tokens); index++ {
		if startsWithKeywords(tokens[index:], values...) {
			return true
		}
	}
	return false
}

func isCanonicalIdentifier(value string) bool {
	if value == "" || !isASCIILetter(value[0]) && value[0] != '_' {
		return false
	}
	for index := 1; index < len(value); index++ {
		if !isASCIILetter(value[index]) && !isASCIIDigit(value[index]) && value[index] != '_' {
			return false
		}
	}
	return value == strings.ToLower(value)
}

func isOneOf(value string, choices ...string) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}

func qualified(schema, name string) string {
	if schema == "" {
		return name
	}
	return schema + "." + name
}

func signature(name string, types []string) string {
	return name + "(" + strings.Join(types, ",") + ")"
}
