package migrationguard

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type tokenKind uint8

const (
	tokenWord tokenKind = iota
	tokenQuotedIdentifier
	tokenString
	tokenDollarString
	tokenNumber
	tokenSymbol
)

type sqlToken struct {
	kind   tokenKind
	raw    string
	value  string
	offset int
}

type sqlStatement struct {
	tokens     []sqlToken
	terminated bool
	offset     int
}

func lexStatements(sql string) ([]sqlStatement, error) {
	tokens, err := lexSQL(sql)
	if err != nil {
		return nil, err
	}

	var statements []sqlStatement
	start := 0
	for index, token := range tokens {
		if !token.isSymbol(";") {
			continue
		}
		if index > start {
			statements = append(statements, sqlStatement{
				tokens:     tokens[start:index],
				terminated: true,
				offset:     tokens[start].offset,
			})
		}
		start = index + 1
	}
	if start < len(tokens) {
		statements = append(statements, sqlStatement{
			tokens: tokens[start:],
			offset: tokens[start].offset,
		})
	}
	return statements, nil
}

func lexSQL(sql string) ([]sqlToken, error) {
	if !utf8.ValidString(sql) {
		return nil, fmt.Errorf("SQL input is not valid UTF-8")
	}
	for index := 0; index < len(sql); index++ {
		if sql[index] == 0 {
			return nil, fmt.Errorf("byte %d: NUL is not valid SQL input", index)
		}
	}
	var tokens []sqlToken
	for index := 0; index < len(sql); {
		if isSQLSpace(sql[index]) {
			index++
			continue
		}
		if strings.HasPrefix(sql[index:], "--") {
			index = skipLineComment(sql, index+2)
			continue
		}
		if strings.HasPrefix(sql[index:], "/*") {
			end, err := skipBlockComment(sql, index)
			if err != nil {
				return nil, err
			}
			index = end
			continue
		}
		if escapeStringPrefix(sql, index) {
			end, err := scanSingleQuoted(sql, index+1, true)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, sqlToken{kind: tokenString, raw: sql[index:end], offset: index})
			index = end
			continue
		}
		if sql[index] == '\'' {
			end, err := scanSingleQuoted(sql, index, false)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, sqlToken{kind: tokenString, raw: sql[index:end], offset: index})
			index = end
			continue
		}
		if sql[index] == '"' {
			end, err := scanQuotedIdentifier(sql, index)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, sqlToken{kind: tokenQuotedIdentifier, raw: sql[index:end], offset: index})
			index = end
			continue
		}
		if delimiter, ok := dollarDelimiter(sql, index); ok {
			end, err := scanDollarString(sql, index, delimiter)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, sqlToken{kind: tokenDollarString, raw: sql[index:end], offset: index})
			index = end
			continue
		}
		if sql[index] == '$' && index+1 < len(sql) && isASCIIDigit(sql[index+1]) {
			end := index + 2
			for end < len(sql) && isASCIIDigit(sql[end]) {
				end++
			}
			tokens = append(tokens, sqlToken{kind: tokenSymbol, raw: sql[index:end], value: sql[index:end], offset: index})
			index = end
			continue
		}
		if isIdentifierStartAt(sql, index) {
			end, err := scanIdentifier(sql, index)
			if err != nil {
				return nil, err
			}
			raw := sql[index:end]
			tokens = append(tokens, sqlToken{kind: tokenWord, raw: raw, value: strings.ToLower(raw), offset: index})
			index = end
			continue
		}
		if isASCIIDigit(sql[index]) {
			end := index + 1
			for end < len(sql) && isASCIIDigit(sql[end]) {
				end++
			}
			tokens = append(tokens, sqlToken{kind: tokenNumber, raw: sql[index:end], value: sql[index:end], offset: index})
			index = end
			continue
		}

		_, size := utf8.DecodeRuneInString(sql[index:])
		if size == 0 || size == 1 && sql[index] >= utf8.RuneSelf {
			return nil, fmt.Errorf("byte %d: invalid UTF-8 in SQL input", index)
		}
		raw := sql[index : index+size]
		tokens = append(tokens, sqlToken{kind: tokenSymbol, raw: raw, value: raw, offset: index})
		index += size
	}
	return tokens, nil
}

func skipLineComment(sql string, index int) int {
	for index < len(sql) && sql[index] != '\n' && sql[index] != '\r' {
		index++
	}
	return index
}

func skipBlockComment(sql string, start int) (int, error) {
	depth := 1
	for index := start + 2; index < len(sql); {
		switch {
		case strings.HasPrefix(sql[index:], "/*"):
			depth++
			index += 2
		case strings.HasPrefix(sql[index:], "*/"):
			depth--
			index += 2
			if depth == 0 {
				return index, nil
			}
		default:
			index++
		}
	}
	return 0, fmt.Errorf("byte %d: unterminated block comment", start)
}

func escapeStringPrefix(sql string, index int) bool {
	if index+1 >= len(sql) || sql[index] != 'e' && sql[index] != 'E' || sql[index+1] != '\'' {
		return false
	}
	return index == 0 || !isIdentifierPartBefore(sql, index)
}

func scanSingleQuoted(sql string, quote int, escape bool) (int, error) {
	for index := quote + 1; index < len(sql); {
		if escape && sql[index] == '\\' {
			index++
			if index >= len(sql) {
				return 0, fmt.Errorf("byte %d: unterminated escape string", quote-1)
			}
			index++
			continue
		}
		if !escape && sql[index] == '\\' {
			return 0, fmt.Errorf("byte %d: backslash in standard string is unsupported; use E'...'", index)
		}
		if sql[index] != '\'' {
			index++
			continue
		}
		if index+1 < len(sql) && sql[index+1] == '\'' {
			index += 2
			continue
		}
		return index + 1, nil
	}
	return 0, fmt.Errorf("byte %d: unterminated string literal", quote)
}

func scanQuotedIdentifier(sql string, start int) (int, error) {
	for index := start + 1; index < len(sql); {
		if sql[index] != '"' {
			index++
			continue
		}
		if index+1 < len(sql) && sql[index+1] == '"' {
			index += 2
			continue
		}
		return index + 1, nil
	}
	return 0, fmt.Errorf("byte %d: unterminated quoted identifier", start)
}

func dollarDelimiter(sql string, start int) (string, bool) {
	if sql[start] != '$' || start > 0 && isIdentifierPartBefore(sql, start) {
		return "", false
	}
	if start+1 < len(sql) && sql[start+1] == '$' {
		return "$$", true
	}
	index := start + 1
	if index >= len(sql) || !isIdentifierStartAt(sql, index) {
		return "", false
	}
	for index < len(sql) {
		if sql[index] == '$' {
			return sql[start : index+1], true
		}
		if !isDollarTagPartAt(sql, index) {
			return "", false
		}
		_, size := utf8.DecodeRuneInString(sql[index:])
		index += size
	}
	return "", false
}

func scanDollarString(sql string, start int, delimiter string) (int, error) {
	for index := start + len(delimiter); index+len(delimiter) <= len(sql); index++ {
		if sql[index:index+len(delimiter)] == delimiter {
			return index + len(delimiter), nil
		}
	}
	return 0, fmt.Errorf("byte %d: unterminated dollar-quoted string %s", start, delimiter)
}

func scanIdentifier(sql string, start int) (int, error) {
	index := start
	for index < len(sql) && isIdentifierPartAt(sql, index) {
		_, size := utf8.DecodeRuneInString(sql[index:])
		if size == 1 && sql[index] >= utf8.RuneSelf {
			return 0, fmt.Errorf("byte %d: invalid UTF-8 in identifier", index)
		}
		index += size
	}
	return index, nil
}

func isIdentifierStartAt(sql string, index int) bool {
	if index >= len(sql) {
		return false
	}
	return isPostgreSQLIdentifierStartByte(sql[index])
}

func isIdentifierPartAt(sql string, index int) bool {
	if index >= len(sql) {
		return false
	}
	return isPostgreSQLIdentifierStartByte(sql[index]) || isASCIIDigit(sql[index]) || sql[index] == '$'
}

func isDollarTagPartAt(sql string, index int) bool {
	if index >= len(sql) {
		return false
	}
	return isPostgreSQLIdentifierStartByte(sql[index]) || isASCIIDigit(sql[index])
}

func isIdentifierPartBefore(sql string, index int) bool {
	if index <= 0 {
		return false
	}
	previous := sql[index-1]
	return isPostgreSQLIdentifierStartByte(previous) || isASCIIDigit(previous) || previous == '$'
}

// PostgreSQL 15's scanner accepts ASCII letters, underscore, and every
// high-bit byte at the start of an unquoted identifier. Since lexSQL validates
// the complete input as UTF-8 first, using the same byte class preserves every
// multibyte code point (including combining marks) as identifier content.
func isPostgreSQLIdentifierStartByte(value byte) bool {
	return isASCIILetter(value) || value == '_' || value >= utf8.RuneSelf
}

func isSQLSpace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\n' || value == '\r' || value == '\f' || value == '\v'
}

func isASCIIDigit(value byte) bool {
	return value >= '0' && value <= '9'
}

func isASCIILetter(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}

func (token sqlToken) isKeyword(value string) bool {
	return token.kind == tokenWord && token.value == value
}

func (token sqlToken) isSymbol(value string) bool {
	return token.kind == tokenSymbol && token.value == value
}
