package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"unicode"
)

// escapeControlCharacters shows every control character other than a newline or a tab as an escape, so text other
// people wrote, such as names, vault names and item fields, cannot drive the terminal it is printed to.
func escapeControlCharacters(text string) string {
	if !strings.ContainsFunc(text, isEscapedControl) {
		return text
	}
	var builder strings.Builder
	for _, character := range text {
		switch {
		case character < 0x100 && isEscapedControl(character):
			fmt.Fprintf(&builder, `\x%02x`, character)
		case isEscapedControl(character):
			fmt.Fprintf(&builder, `\u%04x`, character)
		default:
			builder.WriteRune(character)
		}
	}
	return builder.String()
}

func isEscapedControl(character rune) bool {
	return unicode.IsControl(character) && character != '\n' && character != '\t'
}

// terminalSafeWriter escapes control characters in everything written through it.
type terminalSafeWriter struct {
	output io.Writer
}

func (writer terminalSafeWriter) Write(text []byte) (int, error) {
	if _, err := io.WriteString(writer.output, escapeControlCharacters(string(text))); err != nil {
		return 0, err
	}
	return len(text), nil
}

// newTableWriter lines up columns of text for a person to read, with control characters escaped.
func newTableWriter(output io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(terminalSafeWriter{output: output}, 0, 4, 2, ' ', 0)
}

// standardError is standard error for messages to a person, with control characters escaped.
func standardError() io.Writer {
	return terminalSafeWriter{output: os.Stderr}
}
