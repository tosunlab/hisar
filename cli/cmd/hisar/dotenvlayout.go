package main

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var dotenvByteOrderMark = string(rune(0xfeff))

// dotenvLayout keeps a .env file's exact text so that chosen values can be replaced and everything else stays as it was.
type dotenvLayout struct {
	allLine         []string
	lineEnding      string
	hasByteOrder    bool
	hasFinalNewline bool
	allEntry        []dotenvEntry
}

// dotenvEntry spans allLine[startLine..endLine]; replacing it keeps prefix (indent, export, name, =) and suffix (a comment).
type dotenvEntry struct {
	name      string
	value     string
	startLine int
	endLine   int
	prefix    string
	quote     byte
	suffix    string
}

func parseDotenvLayout(path, content string) (*dotenvLayout, error) {
	layout := &dotenvLayout{lineEnding: "\n"}
	if rest, hasByteOrder := strings.CutPrefix(content, dotenvByteOrderMark); hasByteOrder {
		content, layout.hasByteOrder = rest, true
	}
	if strings.Contains(content, "\r\n") {
		layout.lineEnding = "\r\n"
	}
	content = strings.ReplaceAll(content, "\r\n", "\n")
	layout.hasFinalNewline = strings.HasSuffix(content, "\n")
	layout.allLine = strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	if content == "" {
		layout.allLine = nil
	}
	for index := 0; index < len(layout.allLine); index++ {
		line := layout.allLine[index]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		entry, err := parseDotenvEntry(layout.allLine, index)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %s", path, index+1, err)
		}
		layout.allEntry = append(layout.allEntry, entry)
		index = entry.endLine
	}
	return layout, nil
}

func parseDotenvEntry(allLine []string, startLine int) (dotenvEntry, error) {
	line := allLine[startLine]
	entry := dotenvEntry{startLine: startLine, endLine: startLine}
	position := len(line) - len(strings.TrimLeft(line, " \t"))
	if rest, isExported := strings.CutPrefix(line[position:], "export"); isExported && rest != "" && (rest[0] == ' ' || rest[0] == '\t') {
		position = len(line) - len(strings.TrimLeft(rest, " \t"))
	}
	equals := strings.IndexByte(line[position:], '=')
	if equals < 0 {
		return entry, fmt.Errorf("expected NAME=value")
	}
	entry.name = strings.TrimSpace(line[position : position+equals])
	if !environmentNamePattern.MatchString(entry.name) {
		return entry, fmt.Errorf("%q is not a valid variable name", entry.name)
	}
	valueStart := position + equals + 1
	valueStart += len(line[valueStart:]) - len(strings.TrimLeft(line[valueStart:], " \t"))
	entry.prefix = line[:valueStart]
	rawValue := line[valueStart:]
	if rawValue == "" || (rawValue[0] != '"' && rawValue[0] != '\'') {
		valueText := rawValue
		for index := range len(rawValue) {
			if rawValue[index] == '#' && (index == 0 || rawValue[index-1] == ' ' || rawValue[index-1] == '\t') {
				valueText = rawValue[:index]
				break
			}
		}
		valueEnd := valueStart + len(strings.TrimRight(valueText, " \t"))
		entry.value = strings.TrimSpace(valueText)
		entry.suffix = line[valueEnd:]
		return entry, nil
	}
	entry.quote = rawValue[0]
	var builder strings.Builder
	text, lineIndex, textStart := rawValue[1:], startLine, valueStart+1
	for {
		for index := 0; index < len(text); index++ {
			character := text[index]
			if character == entry.quote {
				rest := strings.TrimSpace(text[index+1:])
				if rest != "" && !strings.HasPrefix(rest, "#") {
					return entry, errorAfterQuote
				}
				entry.value, entry.endLine = builder.String(), lineIndex
				entry.suffix = allLine[lineIndex][textStart+index+1:]
				return entry, nil
			}
			if character == '\\' && entry.quote == '"' && index+1 < len(text) {
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
		if lineIndex+1 >= len(allLine) {
			return entry, errorUnterminatedQuote
		}
		builder.WriteByte('\n')
		lineIndex++
		text, textStart = allLine[lineIndex], 0
	}
}

// render writes the file back, replacing the value of every entry named in allReferenceByName.
func (layout *dotenvLayout) render(allReferenceByName map[string]string) string {
	allLine := []string{}
	nextEntry := 0
	for index := 0; index < len(layout.allLine); index++ {
		if nextEntry < len(layout.allEntry) && layout.allEntry[nextEntry].startLine == index {
			entry := layout.allEntry[nextEntry]
			nextEntry++
			if reference, isReplaced := allReferenceByName[entry.name]; isReplaced {
				quote := ""
				if entry.quote != 0 {
					quote = string(entry.quote)
				}
				allLine = append(allLine, entry.prefix+quote+reference+quote+entry.suffix)
				index = entry.endLine
				continue
			}
		}
		allLine = append(allLine, layout.allLine[index])
	}
	content := strings.Join(allLine, layout.lineEnding)
	if layout.hasFinalNewline {
		content += layout.lineEnding
	}
	if layout.hasByteOrder {
		content = dotenvByteOrderMark + content
	}
	return content
}

// dotenvVariable is one name of the file: its effective (last) value and how many lines set it.
type dotenvVariable struct {
	name            string
	value           string
	occurrenceCount int
	hasOtherValue   bool
	isSecretLooking bool
	reason          string
}

func (layout *dotenvLayout) listAllVariable() []dotenvVariable {
	allVariable := []dotenvVariable{}
	indexByName := map[string]int{}
	for _, entry := range layout.allEntry {
		index, isSeen := indexByName[entry.name]
		if !isSeen {
			indexByName[entry.name] = len(allVariable)
			allVariable = append(allVariable, dotenvVariable{name: entry.name, value: entry.value, occurrenceCount: 1})
			continue
		}
		variable := &allVariable[index]
		variable.occurrenceCount++
		variable.hasOtherValue = variable.hasOtherValue || variable.value != entry.value
		variable.value = entry.value
	}
	for index := range allVariable {
		allVariable[index].isSecretLooking, allVariable[index].reason = classifyDotenvVariable(allVariable[index].name, allVariable[index].value)
	}
	return allVariable
}

var (
	allSecretWord = map[string]bool{
		"SECRET": true, "SECRETS": true, "TOKEN": true, "TOKENS": true, "PASSWORD": true, "PASSWD": true, "PWD": true, "PASS": true,
		"PASSPHRASE": true, "KEY": true, "APIKEY": true, "PRIVATE": true, "CREDENTIAL": true, "CREDENTIALS": true, "AUTH": true,
		"SALT": true, "DSN": true, "COOKIE": true, "CERT": true, "SIGNING": true, "WEBHOOK": true,
	}
	allPlainName = map[string]bool{
		"PORT": true, "HOST": true, "HOSTNAME": true, "NODE_ENV": true, "ENV": true, "ENVIRONMENT": true, "APP_ENV": true,
		"RAILS_ENV": true, "RACK_ENV": true, "FLASK_ENV": true, "GO_ENV": true, "DEBUG": true, "LOG_LEVEL": true, "LOGLEVEL": true,
		"TZ": true, "LANG": true, "LC_ALL": true, "NODE_OPTIONS": true, "PYTHONPATH": true,
	}
	allPlainSuffix   = []string{"_PORT", "_HOST", "_HOSTNAME", "_ENV", "_MODE", "_LEVEL", "_REGION", "_TIMEOUT", "_VERSION", "_DEBUG", "_ENABLED", "_NAME"}
	allAddressSuffix = []string{"_URL", "_URI", "_ENDPOINT", "_DOMAIN", "_ORIGIN", "_ADDR", "_ADDRESS"}
	numberPattern    = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?$`)
	allPlainWord     = map[string]bool{"true": true, "false": true, "yes": true, "no": true, "on": true, "off": true}
	allSecretQuery   = []string{"token", "key", "secret", "password", "pass", "sig", "signature", "auth", "credential"}
)

// classifyDotenvVariable says whether a value looks secret; configuration such as PORT, NODE_ENV or a URL without
// credentials stays a plain value unless the user chooses otherwise.
func classifyDotenvVariable(name, value string) (bool, string) {
	upperName := strings.ToUpper(name)
	for _, word := range strings.Split(upperName, "_") {
		if allSecretWord[word] {
			return true, "the name looks secret"
		}
	}
	if parsedURL, err := url.Parse(value); err == nil && strings.Contains(value, "://") && parsedURL.Host != "" {
		if password, hasPassword := parsedURL.User.Password(); hasPassword && password != "" {
			return true, "a URL with a password"
		}
		for queryName := range parsedURL.Query() {
			for _, secretQuery := range allSecretQuery {
				if strings.Contains(strings.ToLower(queryName), secretQuery) {
					return true, "a URL with a credential"
				}
			}
		}
		return false, "a URL without credentials"
	}
	if allPlainName[upperName] {
		return false, "configuration"
	}
	for _, suffix := range append(append([]string{}, allPlainSuffix...), allAddressSuffix...) {
		if strings.HasSuffix(upperName, suffix) {
			return false, "configuration"
		}
	}
	if numberPattern.MatchString(value) || allPlainWord[strings.ToLower(value)] {
		return false, "a number or switch"
	}
	return true, "may be secret"
}
