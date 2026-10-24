package harness

import (
	"errors"
	"os"
	"strings"
	"testing"
)

var testHermesServer = hermesServer{hisarPath: testHisarPath}

const hermesEntryText = `  hisar:
    command: "/Applications/Hisar.app/Contents/Helpers/hisar"
    args: ["mcp", "--no-reveal"]
    timeout: 4200
    connect_timeout: 60
`

// TestSetHermesServerKeepsEveryOtherByte uses a copy of a config.yaml that Hermes 0.22.2 wrote and its user changed:
// 2,402 lines, most of them comments, with no mcp_servers but a commented-out example of it. The comments' words and
// the user's values are replaced, the layout is not.
func TestSetHermesServerKeepsEveryOtherByte(t *testing.T) {
	content, err := os.ReadFile("testdata/hermes/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	edited, err := setHermesServer(content, testHermesServer)
	if err != nil {
		t.Fatal(err)
	}
	if want := string(content) + "\nmcp_servers:\n" + hermesEntryText; string(edited) != want {
		t.Fatalf("edited config.yaml ends with %q", edited[len(content)-20:])
	}
	again, err := setHermesServer(edited, testHermesServer)
	if err != nil || string(again) != string(edited) {
		t.Fatalf("second setHermesServer() changed the file: %v", err)
	}
	removed, err := removeHermesServer(edited, testHermesServer)
	if err != nil || string(removed) != string(content) {
		t.Fatalf("removeHermesServer() = %v; want the file as it was", err)
	}
}

func TestSetHermesServerAddsToTheUsersServers(t *testing.T) {
	for _, test := range []struct {
		name, input, want string
	}{
		{
			"after the last server, before the next key's comments",
			"model: x\nmcp_servers:\n  github:\n    command: gh\n    args:\n    - mcp\n\n# Terminal\nterminal:\n  backend: local\n",
			"model: x\nmcp_servers:\n  github:\n    command: gh\n    args:\n    - mcp\n" + hermesEntryText + "\n# Terminal\nterminal:\n  backend: local\n",
		},
		{
			"indented by four, with a comment inside the last server",
			"mcp_servers:\n    time:\n        command: uvx\n        # the server's own comment\n    # a comment for the next server\n",
			"mcp_servers:\n    time:\n        command: uvx\n        # the server's own comment\n    hisar:\n        command: \"/Applications/Hisar.app/Contents/Helpers/hisar\"\n        args: [\"mcp\", \"--no-reveal\"]\n        timeout: 4200\n        connect_timeout: 60\n    # a comment for the next server\n",
		},
		{
			"after a block scalar whose text looks like comments and keys",
			"mcp_servers:\n  shell:\n    command: sh\n    script: |\n      # not a comment\n      hisar: not a key\n\nother: 1\n",
			"mcp_servers:\n  shell:\n    command: sh\n    script: |\n      # not a comment\n      hisar: not a key\n" + hermesEntryText + "\nother: 1\n",
		},
		{
			"between the user's comments on its servers",
			"model: x\nmcp_servers:\n  # before first\n  other:\n    command: other\n  # after last\nnext: true\n",
			"model: x\nmcp_servers:\n  # before first\n  other:\n    command: other\n" + hermesEntryText + "  # after last\nnext: true\n",
		},
		{
			"under an empty mcp_servers",
			"mcp_servers:\nmodel: x\n",
			"mcp_servers:\n" + hermesEntryText + "model: x\n",
		},
		{
			"at the end of a file without a final newline",
			"mcp_servers:\n  time:\n    command: uvx",
			"mcp_servers:\n  time:\n    command: uvx\n" + hermesEntryText,
		},
		{"into an empty file", "", "mcp_servers:\n" + hermesEntryText},
		{"after comments only", "# nothing yet\n", "# nothing yet\n\nmcp_servers:\n" + hermesEntryText},
		{
			"beside servers merged from an anchor",
			"base: &servers\n  time:\n    command: uvx\nmcp_servers:\n  <<: *servers\n",
			"base: &servers\n  time:\n    command: uvx\nmcp_servers:\n  <<: *servers\n" + hermesEntryText,
		},
		{
			"with CRLF line endings",
			"model: x\r\nmcp_servers:\r\n  time:\r\n    command: uvx\r\n",
			"model: x\r\nmcp_servers:\r\n  time:\r\n    command: uvx\r\n" + strings.ReplaceAll(hermesEntryText, "\n", "\r\n"),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			edited, err := setHermesServer([]byte(test.input), testHermesServer)
			if err != nil || string(edited) != test.want {
				t.Fatalf("setHermesServer() = %v\n%s\nwant\n%s", err, edited, test.want)
			}
			removed, err := removeHermesServer(edited, testHermesServer)
			if err != nil || strings.Contains(string(removed), "hisar:\n") || strings.Contains(string(removed), "Hisar.app") {
				t.Fatalf("removeHermesServer() = %v\n%s", err, removed)
			}
		})
	}
}

func TestRemoveHermesServerRemovesOnlyItsLines(t *testing.T) {
	for _, test := range []struct {
		name, input, want string
	}{
		{"keeps the other servers", "mcp_servers:\n  time:\n    command: uvx\n" + hermesEntryText + "model: x\n", "mcp_servers:\n  time:\n    command: uvx\nmodel: x\n"},
		{"keeps mcp_servers with a comment of the user's", "mcp_servers: # mine\n" + hermesEntryText + "model: x\n", "mcp_servers: # mine\nmodel: x\n"},
		{"removes mcp_servers that held only Hisar's entry", "model: x\nmcp_servers:\n" + hermesEntryText + "tts:\n  provider: edge\n", "model: x\ntts:\n  provider: edge\n"},
		{"leaves a file without Hisar's entry", "mcp_servers:\n  time:\n    command: uvx\n", "mcp_servers:\n  time:\n    command: uvx\n"},
		{"keeps the user's comments around the other servers", "model: x\nmcp_servers:\n  # before first\n  other:\n    command: other\n" + hermesEntryText + "  # after last\nnext: true\n", "model: x\nmcp_servers:\n  # before first\n  other:\n    command: other\n  # after last\nnext: true\n"},
		{"keeps mcp_servers with a comment of the user's after Hisar's entry", "mcp_servers:\n" + hermesEntryText + "  # user mcp section comment\nnext: true\n", "mcp_servers:\n  # user mcp section comment\nnext: true\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			removed, err := removeHermesServer([]byte(test.input), testHermesServer)
			if err != nil || string(removed) != test.want {
				t.Fatalf("removeHermesServer() = %v\n%s\nwant\n%s", err, removed, test.want)
			}
		})
	}
}

func TestHermesReformattedEntryIsStillHisars(t *testing.T) {
	reformatted := "mcp_servers:\n  hisar:\n    timeout: 4200\n    command: /Applications/Hisar.app/Contents/Helpers/hisar\n    args:\n    - mcp\n    - --no-reveal\n    connect_timeout: 60\n  time:\n    command: uvx\n"
	if edited, err := setHermesServer([]byte(reformatted), testHermesServer); err != nil || string(edited) != reformatted {
		t.Fatalf("setHermesServer() = %v\n%s", err, edited)
	}
	removed, err := removeHermesServer([]byte(reformatted), testHermesServer)
	if want := "mcp_servers:\n  time:\n    command: uvx\n"; err != nil || string(removed) != want {
		t.Fatalf("removeHermesServer() = %v\n%s", err, removed)
	}
}

func TestSetHermesServerLeavesAnotherHisarServer(t *testing.T) {
	for _, test := range []struct {
		name, input string
	}{
		{"another command", "mcp_servers:\n  hisar:\n    command: hisar\n    args: [mcp]\n"},
		{"Hisar's entry with a shorter timeout", strings.Replace("mcp_servers:\n"+hermesEntryText, "4200", "300", 1)},
		{"Hisar's entry turned off", "mcp_servers:\n" + hermesEntryText + "    enabled: false\n"},
		{"Hisar's entry with an environment", "mcp_servers:\n" + hermesEntryText + "    env:\n      HISAR_CLI_SOCKET: /tmp/dev.sock\n"},
		{"Hisar's entry with the timeout as a string", strings.Replace("mcp_servers:\n"+hermesEntryText, "4200", `"4200"`, 1)},
		{"a server in flow style", "mcp_servers: {hisar: {command: hisar}}\n"},
		{"a server merged into mcp_servers", "defaults: &servers\n  hisar:\n    command: user-owned\n    args: [mcp]\nmcp_servers:\n  <<: *servers\n"},
		{"Hisar's entry merged into mcp_servers", "defaults: &servers\n" + hermesEntryText + "mcp_servers:\n  <<: *servers\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := setHermesServer([]byte(test.input), testHermesServer); !errors.Is(err, errConflict) {
				t.Fatalf("setHermesServer() error = %v; want errConflict", err)
			}
			if removed, err := removeHermesServer([]byte(test.input), testHermesServer); err != nil || string(removed) != test.input {
				t.Fatalf("removeHermesServer() = %v\n%s", err, removed)
			}
		})
	}
}

func TestRemoveHermesServerKeepsAServerItShadows(t *testing.T) {
	input := "defaults: &servers\n  hisar:\n    command: user-owned\nmcp_servers:\n  <<: *servers\n" + hermesEntryText
	if edited, err := setHermesServer([]byte(input), testHermesServer); err != nil || string(edited) != input {
		t.Fatalf("setHermesServer() = %v\n%s", err, edited)
	}
	if removed, err := removeHermesServer([]byte(input), testHermesServer); !errors.Is(err, errHermesLayout) {
		t.Fatalf("removeHermesServer() = %v\n%s; want errHermesLayout", err, removed)
	}
}

func TestHermesConfigWithAKeyTwiceIsRefused(t *testing.T) {
	for _, test := range []struct {
		name, input string
	}{
		{"mcp_servers twice, Hisar's entry in the first", "mcp_servers:\n" + hermesEntryText + "mcp_servers:\n  hisar:\n    command: user-owned\n"},
		{"mcp_servers twice, without Hisar's entry", "mcp_servers:\n  other: {}\nmcp_servers:\n  different: {}\n"},
		{"hisar twice", "mcp_servers:\n" + hermesEntryText + "  hisar:\n    command: user-owned\n"},
		{"a key twice inside Hisar's entry", "mcp_servers:\n" + hermesEntryText + "    timeout: 30\n"},
		{"a key twice elsewhere", "model:\n  default: a\n  default: b\nmcp_servers:\n" + hermesEntryText},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := setHermesServer([]byte(test.input), testHermesServer); !errors.Is(err, errHermesDuplicateKey) {
				t.Fatalf("setHermesServer() error = %v; want errHermesDuplicateKey", err)
			}
			if _, err := removeHermesServer([]byte(test.input), testHermesServer); !errors.Is(err, errHermesDuplicateKey) {
				t.Fatalf("removeHermesServer() error = %v; want errHermesDuplicateKey", err)
			}
		})
	}
}

func TestSetHermesServerRefusesWhatItCannotChange(t *testing.T) {
	for _, test := range []struct {
		name, input string
		wantErr     string
	}{
		{"servers in flow style", "mcp_servers: {time: {command: uvx}}\n", errHermesLayout.Error()},
		{"servers as a list", "mcp_servers:\n- time\n", errHermesLayout.Error()},
		{"servers through an alias", "base: &servers\n  time:\n    command: uvx\nmcp_servers: *servers\n", errHermesLayout.Error()},
		{"servers with an anchor copied elsewhere", "mcp_servers: &servers\n  time:\n    command: uvx\ncopy: *servers\n", errHermesLayout.Error()},
		{"a top level that merges another mapping", "base: &base\n  mcp_servers:\n    hisar:\n      command: user-owned\n<<: *base\n", errHermesLayout.Error()},
		{"tab indentation", "mcp_servers:\n\ttime:\n\t\tcommand: uvx\n", "Hermes' config.yaml is not valid YAML"},
		{"servers set to null by name", "mcp_servers: null\n", errHermesLayout.Error()},
		{"a top level in flow style", "{model: x}\n", errHermesLayout.Error()},
		{"a list at the top", "- model\n", errHermesLayout.Error()},
		// The line after the quoted value's start is not indented, so only the parse of the edited file shows that the
		// entry would have gone inside the string.
		{"a quoted value that goes on at the margin", "mcp_servers:\n  time:\n    command: \"uv\nx\"\n", errHermesLayout.Error()},
		{"two documents", "model: x\n---\nmodel: y\n", errHermesLayout.Error()},
		{"invalid YAML", "model: [x\n", "Hermes' config.yaml is not valid YAML"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := setHermesServer([]byte(test.input), testHermesServer); err == nil || err.Error() != test.wantErr {
				t.Fatalf("setHermesServer() error = %v; want %q", err, test.wantErr)
			}
		})
	}
}

func TestHermesEntryCarriesACustomSocket(t *testing.T) {
	server := hermesServer{hisarPath: `/Users/developer/My "Apps"/Hisar.app/Contents/Helpers/hisar`, socketPath: "/tmp/hisar dev/cli.sock"}
	edited, err := setHermesServer([]byte("model: x\n"), server)
	want := "model: x\n\nmcp_servers:\n  hisar:\n    command: \"/Users/developer/My \\\"Apps\\\"/Hisar.app/Contents/Helpers/hisar\"\n    args: [\"mcp\", \"--no-reveal\"]\n    timeout: 4200\n    connect_timeout: 60\n    env:\n      HISAR_CLI_SOCKET: \"/tmp/hisar dev/cli.sock\"\n"
	if err != nil || string(edited) != want {
		t.Fatalf("setHermesServer() = %v\n%s", err, edited)
	}
	if _, err := setHermesServer(edited, testHermesServer); !errors.Is(err, errConflict) {
		t.Fatalf("an entry for another socket error = %v; want errConflict", err)
	}
}
