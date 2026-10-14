package dotenv

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var environmentNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type Variable struct {
	Name  string
	Value string
}

func ReadFile(path string) ([]Variable, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(path, string(content))
}

// Parse reads dotenv syntax; errors name the file and line but never a value.
func Parse(path, content string) ([]Variable, error) {
	content = strings.TrimPrefix(content, "\uFEFF")
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")
	var variables []Variable
	for index := 0; index < len(lines); index++ {
		lineNumber := index + 1
		line := strings.TrimSpace(lines[index])
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if rest, isExported := strings.CutPrefix(line, "export"); isExported && rest != "" && (rest[0] == ' ' || rest[0] == '\t') {
			line = strings.TrimSpace(rest)
		}
		name, rawValue, hasEquals := strings.Cut(line, "=")
		name = strings.TrimSpace(name)
		if !hasEquals {
			return nil, fmt.Errorf("%s:%d: expected NAME=value", path, lineNumber)
		}
		if !environmentNamePattern.MatchString(name) {
			return nil, fmt.Errorf("%s:%d: %q is not a valid variable name", path, lineNumber, name)
		}
		rawValue = strings.TrimLeft(rawValue, " \t")
		if rawValue == "" || (rawValue[0] != '"' && rawValue[0] != '\'') {
			variables = append(variables, Variable{Name: name, Value: parseUnquotedValue(rawValue)})
			continue
		}
		value, consumedLineCount, err := parseQuotedValue(rawValue, lines[index+1:])
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %s %s", path, lineNumber, name, err)
		}
		index += consumedLineCount
		variables = append(variables, Variable{Name: name, Value: value})
	}
	return variables, nil
}

func parseUnquotedValue(rawValue string) string {
	for index := 1; index < len(rawValue); index++ {
		if rawValue[index] == '#' && (rawValue[index-1] == ' ' || rawValue[index-1] == '\t') {
			rawValue = rawValue[:index]
			break
		}
	}
	if strings.HasPrefix(rawValue, "#") {
		return ""
	}
	return strings.TrimSpace(rawValue)
}

// parseQuotedValue reads a single- or double-quoted value that may continue on the following lines.
func parseQuotedValue(rawValue string, followingLines []string) (string, int, error) {
	quote := rawValue[0]
	text := rawValue[1:]
	consumedLineCount := 0
	var builder strings.Builder
	for {
		for index := 0; index < len(text); index++ {
			character := text[index]
			if character == quote {
				rest := strings.TrimSpace(text[index+1:])
				if rest != "" && !strings.HasPrefix(rest, "#") {
					return "", 0, errorAfterQuote
				}
				return builder.String(), consumedLineCount, nil
			}
			if character == '\\' && quote == '"' && index+1 < len(text) {
				index++
				switch text[index] {
				case 'n':
					builder.WriteByte('\n')
				case 'r':
					builder.WriteByte('\r')
				case 't':
					builder.WriteByte('\t')
				case '"', '\\', '$':
					builder.WriteByte(text[index])
				default:
					builder.WriteByte('\\')
					builder.WriteByte(text[index])
				}
				continue
			}
			builder.WriteByte(character)
		}
		if consumedLineCount >= len(followingLines) {
			return "", 0, errorUnterminatedQuote
		}
		builder.WriteByte('\n')
		text = followingLines[consumedLineCount]
		consumedLineCount++
	}
}

type dotenvError string

func (err dotenvError) Error() string { return string(err) }

const (
	errorAfterQuote        = dotenvError("has text after its closing quote")
	errorUnterminatedQuote = dotenvError("has a quote that is never closed")
)
