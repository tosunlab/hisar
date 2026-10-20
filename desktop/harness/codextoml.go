package harness

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Codex's config.toml is the user's file, with their comments and layout. Hisar reads only as much TOML as it takes to
// find each table and key, and changes the bytes of its own table alone.

// tomlStatement is a table header or a key/value pair of a TOML document.
type tomlStatement struct {
	// start and end are the statement's bytes, from the start of its line to after its line ending.
	start, end    int
	isHeader      bool
	isArrayHeader bool
	// path is a header's table, or a key/value's table followed by its dotted key.
	path []string
	// value is a key/value's value as written.
	value string
}

var errCodexLayout = errors.New("Codex's config.toml lists its MCP servers in a form Hisar cannot change. Add Hisar's server there by hand")

type tomlScanner struct {
	content []byte
	pos     int
}

func (scanner *tomlScanner) errorf(format string, arguments ...any) error {
	line := bytes.Count(scanner.content[:scanner.pos], []byte("\n")) + 1
	return fmt.Errorf("Codex's config.toml is not valid TOML at line %d: %s", line, fmt.Sprintf(format, arguments...))
}

func (scanner *tomlScanner) isAtEnd() bool { return scanner.pos >= len(scanner.content) }

func (scanner *tomlScanner) peek(offset int) byte {
	if scanner.pos+offset >= len(scanner.content) {
		return 0
	}
	return scanner.content[scanner.pos+offset]
}

func (scanner *tomlScanner) hasPrefix(prefix string) bool {
	return bytes.HasPrefix(scanner.content[scanner.pos:], []byte(prefix))
}

func (scanner *tomlScanner) skipBlank() {
	for scanner.peek(0) == ' ' || scanner.peek(0) == '\t' {
		scanner.pos++
	}
}

func (scanner *tomlScanner) isAtLineEnd() bool {
	return scanner.isAtEnd() || scanner.peek(0) == '\n' || scanner.hasPrefix("\r\n")
}

// endLine passes a comment and the line ending; anything else after a statement is an error.
func (scanner *tomlScanner) endLine() error {
	scanner.skipBlank()
	if scanner.peek(0) == '#' {
		for !scanner.isAtLineEnd() {
			scanner.pos++
		}
	}
	switch {
	case scanner.isAtEnd():
	case scanner.peek(0) == '\n':
		scanner.pos++
	case scanner.hasPrefix("\r\n"):
		scanner.pos += 2
	default:
		return scanner.errorf("unexpected %q", scanner.peek(0))
	}
	return nil
}

func parseTOML(content []byte) ([]tomlStatement, error) {
	scanner := &tomlScanner{content: content}
	if scanner.hasPrefix("\uFEFF") {
		scanner.pos += len("\uFEFF")
	}
	var statements []tomlStatement
	var table []string
	for !scanner.isAtEnd() {
		start := scanner.pos
		scanner.skipBlank()
		if scanner.isAtLineEnd() || scanner.peek(0) == '#' {
			if err := scanner.endLine(); err != nil {
				return nil, err
			}
			continue
		}
		statement := tomlStatement{start: start}
		if scanner.peek(0) == '[' {
			statement.isHeader = true
			statement.isArrayHeader = scanner.peek(1) == '['
			closing := "]"
			scanner.pos++
			if statement.isArrayHeader {
				closing = "]]"
				scanner.pos++
			}
			path, err := scanner.parseKey()
			if err != nil {
				return nil, err
			}
			scanner.skipBlank()
			if !scanner.hasPrefix(closing) {
				return nil, scanner.errorf("a table name without %q", closing)
			}
			scanner.pos += len(closing)
			table, statement.path = path, path
		} else {
			key, err := scanner.parseKey()
			if err != nil {
				return nil, err
			}
			scanner.skipBlank()
			if scanner.peek(0) != '=' {
				return nil, scanner.errorf("a key without \"=\"")
			}
			scanner.pos++
			scanner.skipBlank()
			valueStart := scanner.pos
			if err := scanner.skipValue(); err != nil {
				return nil, err
			}
			statement.value = strings.TrimRight(string(scanner.content[valueStart:scanner.pos]), " \t")
			if statement.value == "" {
				return nil, scanner.errorf("a key without a value")
			}
			statement.path = append(slices.Clone(table), key...)
		}
		if err := scanner.endLine(); err != nil {
			return nil, err
		}
		statement.end = scanner.pos
		statements = append(statements, statement)
	}
	return statements, nil
}

// parseKey reads a dotted key of bare and quoted parts.
func (scanner *tomlScanner) parseKey() ([]string, error) {
	var path []string
	for {
		scanner.skipBlank()
		var part string
		switch scanner.peek(0) {
		case '"', '\'':
			start := scanner.pos
			if err := scanner.skipString(); err != nil {
				return nil, err
			}
			value, isString := parseTOMLString(string(scanner.content[start:scanner.pos]))
			if !isString {
				return nil, scanner.errorf("a key in a multi-line string")
			}
			part = value
		default:
			start := scanner.pos
			for isBareKeyByte(scanner.peek(0)) {
				scanner.pos++
			}
			if scanner.pos == start {
				return nil, scanner.errorf("a missing key")
			}
			part = string(scanner.content[start:scanner.pos])
		}
		path = append(path, part)
		scanner.skipBlank()
		if scanner.peek(0) != '.' {
			return path, nil
		}
		scanner.pos++
	}
}

func isBareKeyByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-'
}

// skipValue passes a value up to its line's end or comment; arrays may span lines and hold comments.
func (scanner *tomlScanner) skipValue() error {
	depth := 0
	for {
		switch {
		case scanner.isAtEnd():
			if depth > 0 {
				return scanner.errorf("an array or table that is not closed")
			}
			return nil
		case scanner.isAtLineEnd() || scanner.peek(0) == '#':
			if depth == 0 {
				return nil
			}
			if scanner.peek(0) == '#' {
				for !scanner.isAtLineEnd() {
					scanner.pos++
				}
				continue
			}
			scanner.pos++
		case scanner.peek(0) == '\r':
			// TOML allows a carriage return only in a line ending, which the case above takes.
			return scanner.errorf("unexpected %q", scanner.peek(0))
		case scanner.peek(0) == '"' || scanner.peek(0) == '\'':
			if err := scanner.skipString(); err != nil {
				return err
			}
		case scanner.peek(0) == '[' || scanner.peek(0) == '{':
			depth++
			scanner.pos++
		case scanner.peek(0) == ']' || scanner.peek(0) == '}':
			if depth == 0 {
				return scanner.errorf("unexpected %q", scanner.peek(0))
			}
			depth--
			scanner.pos++
		default:
			scanner.pos++
		}
	}
}

// skipString passes a basic or literal string, single-line or multi-line.
func (scanner *tomlScanner) skipString() error {
	quote := scanner.peek(0)
	isBasic := quote == '"'
	delimiter := strings.Repeat(string(quote), 3)
	if scanner.hasPrefix(delimiter) {
		scanner.pos += 3
		for {
			switch {
			case scanner.isAtEnd():
				return scanner.errorf("a multi-line string that is not closed")
			case isBasic && scanner.peek(0) == '\\':
				// The escaped byte, unless the backslash ends the file.
				scanner.pos = min(scanner.pos+2, len(scanner.content))
			case scanner.hasPrefix(delimiter):
				// A multi-line string may end with one or two quotes of its own right before its delimiter.
				run := 0
				for scanner.peek(run) == quote {
					run++
				}
				if run > 5 {
					return scanner.errorf("too many quotes")
				}
				scanner.pos += run
				return nil
			default:
				scanner.pos++
			}
		}
	}
	scanner.pos++
	for {
		switch {
		case scanner.isAtLineEnd():
			return scanner.errorf("a string that is not closed")
		case isBasic && scanner.peek(0) == '\\':
			// A backslash escapes a byte of its line only: a single-line string ends with its line.
			scanner.pos++
			if !scanner.isAtLineEnd() {
				scanner.pos++
			}
		case scanner.peek(0) == quote:
			scanner.pos++
			return nil
		default:
			scanner.pos++
		}
	}
}

// parseTOMLString decodes a single-line basic or literal string written as raw, and nothing else.
func parseTOMLString(raw string) (string, bool) {
	if len(raw) < 2 || strings.HasPrefix(raw, `"""`) || strings.HasPrefix(raw, "'''") {
		return "", false
	}
	quote := raw[0]
	if (quote != '"' && quote != '\'') || raw[len(raw)-1] != quote {
		return "", false
	}
	body := raw[1 : len(raw)-1]
	if quote == '\'' {
		return body, !strings.ContainsRune(body, '\'')
	}
	var decoded strings.Builder
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c == '"' {
			return "", false
		}
		if c != '\\' {
			decoded.WriteByte(c)
			continue
		}
		if i+1 >= len(body) {
			return "", false
		}
		i++
		switch body[i] {
		case 'b':
			decoded.WriteByte('\b')
		case 't':
			decoded.WriteByte('\t')
		case 'n':
			decoded.WriteByte('\n')
		case 'f':
			decoded.WriteByte('\f')
		case 'r':
			decoded.WriteByte('\r')
		case 'e':
			decoded.WriteByte(0x1b)
		case '"', '\\':
			decoded.WriteByte(body[i])
		case 'u', 'U':
			length := 4
			if body[i] == 'U' {
				length = 8
			}
			if i+1+length > len(body) {
				return "", false
			}
			code, err := strconv.ParseUint(body[i+1:i+1+length], 16, 32)
			if err != nil || !utf8.ValidRune(rune(code)) {
				return "", false
			}
			decoded.WriteRune(rune(code))
			i += length
		default:
			return "", false
		}
	}
	return decoded.String(), true
}

// parseTOMLStringArray decodes an array of single-line strings, which may span lines and hold comments.
func parseTOMLStringArray(raw string) ([]string, bool) {
	scanner := &tomlScanner{content: []byte(raw)}
	skipSpace := func() {
		for !scanner.isAtEnd() {
			switch c := scanner.peek(0); {
			case c == ' ' || c == '\t' || c == '\n' || c == '\r':
				scanner.pos++
			case c == '#':
				for !scanner.isAtLineEnd() {
					scanner.pos++
				}
			default:
				return
			}
		}
	}
	if scanner.peek(0) != '[' {
		return nil, false
	}
	scanner.pos++
	allValue := []string{}
	for {
		skipSpace()
		if scanner.peek(0) == ']' {
			scanner.pos++
			if !scanner.isAtEnd() {
				return nil, false
			}
			return allValue, true
		}
		start := scanner.pos
		if c := scanner.peek(0); (c != '"' && c != '\'') || scanner.skipString() != nil {
			return nil, false
		}
		value, isString := parseTOMLString(raw[start:scanner.pos])
		if !isString {
			return nil, false
		}
		allValue = append(allValue, value)
		skipSpace()
		switch scanner.peek(0) {
		case ',':
			scanner.pos++
		case ']':
		default:
			return nil, false
		}
	}
}

// quoteTOML writes value as a basic string.
func quoteTOML(value string) string {
	var quoted strings.Builder
	quoted.WriteByte('"')
	for _, r := range value {
		switch {
		case r == '"' || r == '\\':
			quoted.WriteByte('\\')
			quoted.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&quoted, `\u%04X`, r)
		default:
			quoted.WriteRune(r)
		}
	}
	quoted.WriteByte('"')
	return quoted.String()
}

// Hisar's table gives the app startup room to answer and a tool timeout past run_with_secrets' one hour, so an approval
// and a long command are not cut off at Codex's default of 60 seconds.
const (
	codexStartupTimeoutSeconds = 15
	codexToolTimeoutSeconds    = 4200
)

// codexServerKeys are the keys of Hisar's table. A table with another key, without one of them, or with another value
// is the user's.
var codexServerKeys = []string{"command", "args", "startup_timeout_sec", "tool_timeout_sec"}

func codexServerTable(hisarPath, newline string) string {
	quotedArguments := make([]string, len(mcpServerArguments))
	for i, argument := range mcpServerArguments {
		quotedArguments[i] = quoteTOML(argument)
	}
	lines := []string{
		"[mcp_servers." + mcpServerName + "]",
		"command = " + quoteTOML(hisarPath),
		"args = [" + strings.Join(quotedArguments, ", ") + "]",
		fmt.Sprintf("startup_timeout_sec = %d", codexStartupTimeoutSeconds),
		fmt.Sprintf("tool_timeout_sec = %d", codexToolTimeoutSeconds),
	}
	return strings.Join(lines, newline) + newline
}

// codexServer is the MCP server named hisar in config.toml: none, Hisar's own table at start:end, which holds exactly
// what codexServerTable writes, or the user's.
type codexServer struct {
	isFound, isOurs bool
	start, end      int
}

func findCodexServer(content []byte, hisarPath string) (codexServer, error) {
	statements, err := parseTOML(content)
	if err != nil {
		return codexServer{}, err
	}
	serverPath := []string{"mcp_servers", mcpServerName}
	server := codexServer{}
	values := map[string]string{}
	isInServerTable := false
	for _, statement := range statements {
		if statement.isHeader {
			isInServerTable = false
		}
		switch {
		case len(statement.path) < len(serverPath) && slices.Equal(statement.path, serverPath[:len(statement.path)]) &&
			(statement.isArrayHeader || !statement.isHeader):
			// mcp_servers = { … } or [[mcp_servers]]: no table can be added below it.
			return codexServer{}, errCodexLayout
		case !hasPathPrefix(statement.path, serverPath):
		case statement.isHeader && !statement.isArrayHeader && len(statement.path) == len(serverPath) && !server.isFound:
			server, isInServerTable = codexServer{isFound: true, start: statement.start, end: statement.end}, true
		case !statement.isHeader && isInServerTable && len(statement.path) == len(serverPath)+1:
			values[statement.path[len(serverPath)]] = statement.value
			server.end = statement.end
		default:
			return codexServer{isFound: true}, nil
		}
	}
	server.isOurs = server.isFound && isCodexServerOurs(values, hisarPath)
	return server, nil
}

// setCodexServer returns content with Hisar's [mcp_servers.hisar] table, added at the end or replacing Hisar's own;
// every other byte stays. A server named hisar that Hisar did not write is errConflict.
func setCodexServer(content []byte, hisarPath string) ([]byte, error) {
	server, err := findCodexServer(content, hisarPath)
	if err != nil {
		return nil, err
	}
	if server.isFound && !server.isOurs {
		return nil, errConflict
	}
	newline := "\n"
	if index := bytes.IndexByte(content, '\n'); index > 0 && content[index-1] == '\r' {
		newline = "\r\n"
	}
	table := codexServerTable(hisarPath, newline)
	if server.isOurs {
		return slices.Concat(content[:server.start], []byte(table), content[server.end:]), nil
	}
	edited := slices.Clone(content)
	if len(edited) > 0 {
		if !bytes.HasSuffix(edited, []byte("\n")) {
			edited = append(edited, newline...)
		}
		edited = append(edited, newline...)
	}
	return append(edited, table...), nil
}

func isCodexServerOurs(values map[string]string, hisarPath string) bool {
	for key := range values {
		if !slices.Contains(codexServerKeys, key) {
			return false
		}
	}
	command, isCommand := parseTOMLString(values["command"])
	arguments, isArguments := parseTOMLStringArray(values["args"])
	startupTimeout, startupErr := strconv.ParseFloat(values["startup_timeout_sec"], 64)
	toolTimeout, toolErr := strconv.ParseFloat(values["tool_timeout_sec"], 64)
	return isCommand && command == hisarPath && isArguments && slices.Equal(arguments, mcpServerArguments) &&
		startupErr == nil && startupTimeout == codexStartupTimeoutSeconds && toolErr == nil && toolTimeout == codexToolTimeoutSeconds
}

func hasPathPrefix(path, prefix []string) bool {
	return len(path) >= len(prefix) && slices.Equal(path[:len(prefix)], prefix)
}
