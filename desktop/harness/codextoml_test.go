package harness

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"
)

const testHisarPath = "/Applications/Hisar.app/Contents/Helpers/hisar"

const hisarTable = `[mcp_servers.hisar]
command = "/Applications/Hisar.app/Contents/Helpers/hisar"
args = ["mcp", "--no-reveal"]
startup_timeout_sec = 15
tool_timeout_sec = 4200
`

func TestSetCodexServerKeepsEveryOtherByte(t *testing.T) {
	userConfig := `# Codex settings
model = "gpt-6.1-sol" # the default
approval_policy = "on-request"
notify = [
  "say",   # comma ] inside a comment
  "[mcp_servers.hisar]",
]
instructions = """
[mcp_servers.hisar]
command = "not a table"
"""
path = 'C:\tools\[x]'
tui.theme = "dark"

[mcp_servers.github]
command = "github-mcp"
args = ["stdio"]

[mcp_servers.github.env]
TOKEN = "x"

[profiles."deep work"]
model = "o4"
`
	for _, test := range []struct {
		name  string
		input string
		want  string
	}{
		{"no file yet", "", hisarTable},
		{"comments, strings that look like tables and other servers", userConfig, userConfig + "\n" + hisarTable},
		{"no newline at the end", "model = \"o4\"", "model = \"o4\"\n\n" + hisarTable},
		{
			"Hisar's table in the middle, its arguments over lines",
			"# servers\n[mcp_servers.hisar]\ncommand = \"" + testHisarPath + "\"\nargs = [\n  \"mcp\",\n  \"--no-reveal\", # Hisar\n]\nstartup_timeout_sec = 15\ntool_timeout_sec = 4200 # an hour and more\n\n# GitHub\n[mcp_servers.github]\ncommand = \"gh\"\n",
			"# servers\n" + hisarTable + "\n# GitHub\n[mcp_servers.github]\ncommand = \"gh\"\n",
		},
		{
			"Hisar's table with a quoted name and a literal string",
			"[ mcp_servers . \"hisar\" ]\ntool_timeout_sec = 4200\ncommand = '" + testHisarPath + "'\nargs = ['mcp', '--no-reveal']\nstartup_timeout_sec = 15\n",
			hisarTable,
		},
		{
			"another server written with dotted keys",
			"[mcp_servers]\ngithub.command = \"gh\"\ngithub.args = []\n",
			"[mcp_servers]\ngithub.command = \"gh\"\ngithub.args = []\n\n" + hisarTable,
		},
		{"Windows line endings", "model = \"o4\"\r\n[mcp_servers.github]\r\ncommand = \"gh\"\r\n", "model = \"o4\"\r\n[mcp_servers.github]\r\ncommand = \"gh\"\r\n\r\n" + strings.ReplaceAll(hisarTable, "\n", "\r\n")},
	} {
		t.Run(test.name, func(t *testing.T) {
			edited, err := setCodexServer([]byte(test.input), testHisarPath)
			if err != nil {
				t.Fatalf("setCodexServer() error = %v", err)
			}
			if string(edited) != test.want {
				t.Fatalf("setCodexServer() =\n%s\nwant\n%s", edited, test.want)
			}
			again, err := setCodexServer(edited, testHisarPath)
			if err != nil || string(again) != string(edited) {
				t.Fatalf("setCodexServer() a second time = %q, %v; want no change", again, err)
			}
		})
	}
}

func TestSetCodexServerLeavesAnotherHisarServer(t *testing.T) {
	for _, test := range []struct {
		name  string
		input string
	}{
		{"another command", "[mcp_servers.hisar]\ncommand = \"hisar\"\nargs = [\"mcp\", \"--no-reveal\"]\nstartup_timeout_sec = 15\ntool_timeout_sec = 4200\n"},
		{"get_secret on", "[mcp_servers.hisar]\ncommand = \"" + testHisarPath + "\"\nargs = [\"mcp\"]\nstartup_timeout_sec = 15\ntool_timeout_sec = 4200\n"},
		{"a key Hisar does not write", hisarTable + "enabled = false\n"},
		{"environment variables passed through", hisarTable + "env_vars = [\"CUSTOM_VAR\"]\n"},
		{"a working folder", hisarTable + "cwd = \"/tmp\"\n"},
		{"another tool timeout", strings.Replace(hisarTable, "tool_timeout_sec = 4200", "tool_timeout_sec = 60", 1)},
		{"no timeouts", "[mcp_servers.hisar]\ncommand = \"" + testHisarPath + "\"\nargs = [\"mcp\", \"--no-reveal\"]\n"},
		{"environment variables", hisarTable + "\n[mcp_servers.hisar.env]\nA = \"b\"\n"},
		{"dotted keys at the top", "mcp_servers.hisar.command = \"" + testHisarPath + "\"\n"},
		{"dotted keys in mcp_servers", "[mcp_servers]\nhisar.command = \"" + testHisarPath + "\"\n"},
		{"an inline table in mcp_servers", "[mcp_servers]\nhisar = { command = \"" + testHisarPath + "\" }\n"},
		{"dotted keys in the table", hisarTable + "env.A = \"b\"\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := setCodexServer([]byte(test.input), testHisarPath); !errors.Is(err, errConflict) {
				t.Fatalf("setCodexServer() error = %v; want errConflict", err)
			}
		})
	}
}

func TestSetCodexServerRefusesWhatItCannotRead(t *testing.T) {
	for _, test := range []struct {
		name      string
		input     string
		wantError string
	}{
		{"an array that is not closed", "model = \"o4\"\nargs = [\n\"mcp\",\n", "Codex's config.toml is not valid TOML at line 4: an array or table that is not closed"},
		{"a string that is not closed", "model = \"o4\n", "Codex's config.toml is not valid TOML at line 1: a string that is not closed"},
		{"a backslash that ends the file", "model = \"o4\\", "Codex's config.toml is not valid TOML at line 1: a string that is not closed"},
		{"a backslash that ends a line", "model = \"o4\\\n\"\n", "Codex's config.toml is not valid TOML at line 1: a string that is not closed"},
		{"a backslash that ends a key", "\"model\\", "Codex's config.toml is not valid TOML at line 1: a string that is not closed"},
		{"a backslash that ends an array", "args = [\n\"mcp\\", "Codex's config.toml is not valid TOML at line 2: a string that is not closed"},
		{"a backslash that ends a multi-line string", "notes = \"\"\"\nline\\", "Codex's config.toml is not valid TOML at line 2: a multi-line string that is not closed"},
		{"a carriage return without a line feed", "model = 1\r", "Codex's config.toml is not valid TOML at line 1: unexpected '\\r'"},
		{"a key without a value", "model\n", "Codex's config.toml is not valid TOML at line 1: a key without \"=\""},
		{"text after a table name", "[mcp_servers] model\n", "Codex's config.toml is not valid TOML at line 1: unexpected 'm'"},
		{"servers on one line", "mcp_servers = { github = { command = \"gh\" } }\n", errCodexLayout.Error()},
		{"servers as an array", "[[mcp_servers]]\ncommand = \"gh\"\n", errCodexLayout.Error()},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := setCodexServer([]byte(test.input), testHisarPath); err == nil || err.Error() != test.wantError {
				t.Fatalf("setCodexServer() error = %v; want %q", err, test.wantError)
			}
		})
	}
}

// FuzzSetCodexServer: whatever config.toml holds, setCodexServer refuses it or returns it with Hisar's table added at the
// end or put in place of Hisar's own, and every other byte as it was.
func FuzzSetCodexServer(f *testing.F) {
	for _, seed := range []string{
		"",
		hisarTable,
		"model = \"o4\"",
		"# servers\n" + hisarTable + "\n# GitHub\n[mcp_servers.github]\ncommand = \"gh\"\n",
		"model = \"o4\"\r\n[mcp_servers.github]\r\ncommand = \"gh\"\r\n",
		"[ mcp_servers . \"hisar\" ]\ntool_timeout_sec = 4200\ncommand = '" + testHisarPath + "'\nargs = ['mcp', '--no-reveal']\nstartup_timeout_sec = 15\n",
		"notify = [\n  \"say\", # ]\n]\ninstructions = \"\"\"\n[mcp_servers.hisar]\n\"\"\"\npath = 'C:\\tools'\n",
		hisarTable + "env_vars = [\"CUSTOM_VAR\"]\n",
		"model = \"o4\\",
		"\"model\\",
		"notes = \"\"\"\nline\\",
		"args = ['a', \"b\\",
	} {
		f.Add([]byte(seed))
	}
	serverPath := []string{"mcp_servers", mcpServerName}
	f.Fuzz(func(t *testing.T, content []byte) {
		edited, err := setCodexServer(content, testHisarPath)
		if err != nil {
			return
		}
		server, err := findCodexServer(edited, testHisarPath)
		if err != nil || !server.isOurs {
			t.Fatalf("the edited file holds no table of Hisar's: %+v, %v\n%q", server, err, edited)
		}
		table := string(edited[server.start:server.end])
		if table != codexServerTable(testHisarPath, "\n") && table != codexServerTable(testHisarPath, "\r\n") {
			t.Fatalf("Hisar's table = %q", table)
		}
		before, after := edited[:server.start], edited[server.end:]
		if len(after) == 0 && bytes.HasPrefix(before, content) {
			if separator := before[len(content):]; len(bytes.Trim(separator, "\r\n")) != 0 || len(separator) > 4 {
				t.Fatalf("added %q before Hisar's table", separator)
			}
			return
		}
		if !bytes.HasPrefix(content, before) || !bytes.HasSuffix(content, after) || len(before)+len(after) > len(content) {
			t.Fatalf("changed bytes outside Hisar's table:\n%q\nbecame\n%q", content, edited)
		}
		start, end := len(before), len(content)-len(after)
		statements, err := parseTOML(content)
		if err != nil {
			t.Fatal(err)
		}
		for _, statement := range statements {
			isInside := statement.start >= start && statement.end <= end
			isOutside := statement.end <= start || statement.start >= end
			if !isInside && !isOutside || isInside && !hasPathPrefix(statement.path, serverPath) ||
				statement.start == start && !(statement.isHeader && len(statement.path) == len(serverPath)) {
				t.Fatalf("replaced %q, which is not Hisar's table, in\n%q", content[start:end], content)
			}
		}
	})
}

func TestQuotedHisarPathReadsBack(t *testing.T) {
	for _, path := range []string{testHisarPath, `/Users/me/My "Apps"/Hisar.app/Contents/Helpers/hisar`, `/tmp/back\slash/tab	hisar`, "/Applications/Hisar Ünicode.app/hisar"} {
		edited, err := setCodexServer(nil, path)
		if err != nil {
			t.Fatal(err)
		}
		statements, err := parseTOML(edited)
		if err != nil {
			t.Fatalf("parseTOML(%q) error = %v", edited, err)
		}
		command, isString := parseTOMLString(statements[1].value)
		if !isString || command != path {
			t.Fatalf("command reads back as %q, %v; want %q", command, isString, path)
		}
		if _, err := setCodexServer(edited, path); err != nil {
			t.Fatalf("setCodexServer() over its own table for %q error = %v", path, err)
		}
	}
}

func TestParseTOMLStringArray(t *testing.T) {
	for _, test := range []struct {
		raw     string
		want    []string
		isArray bool
	}{
		{`[]`, []string{}, true},
		{`["mcp", '--no-reveal',]`, []string{"mcp", "--no-reveal"}, true},
		{"[\n  \"a\", # first\n  \"b\\u00e9\"\n]", []string{"a", "bé"}, true},
		{`["a", 1]`, nil, false},
		{`["a"] x`, nil, false},
		{`"a"`, nil, false},
		{`["a\`, nil, false},
	} {
		got, isArray := parseTOMLStringArray(test.raw)
		if isArray != test.isArray || !slices.Equal(got, test.want) {
			t.Errorf("parseTOMLStringArray(%q) = %q, %v; want %q, %v", test.raw, got, isArray, test.want, test.isArray)
		}
	}
}
