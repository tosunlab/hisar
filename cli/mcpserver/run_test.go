package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tryhisar/hisar/core/cliprotocol"
)

func TestRunWithSecretsRunsAShellCommandLine(t *testing.T) {
	requireUnix(t)
	app := &fakeApp{}
	server := createTestServer(t, app)
	shell := `printf '%s\n' "$TOKEN" | cat; false || echo "fallback" && echo done >&2`
	output := decodeOutput[RunOutput](t, server.callTool(t, "run_with_secrets", map[string]any{
		"shell": shell,
		"env":   map[string]string{"TOKEN": "hisar://Work/Stripe/api%20key"},
	}))
	if output.Stdout != "<concealed by hisar>\nfallback\n" || output.Stderr != "done\n" || output.ExitCode != 0 {
		t.Errorf("output = %+v", output)
	}
	if request := app.getLastRequest(); !slices.Equal(request.CommandLine, []string{"/bin/sh", "-c", shell}) {
		t.Errorf("the approval shows %q", request.CommandLine)
	}
}

func TestRunWithSecretsReadsEnvFilesAsHisarRunDoes(t *testing.T) {
	requireUnix(t)
	app := &fakeApp{}
	directory := t.TempDir()
	os.WriteFile(filepath.Join(directory, ".env"), []byte("# the app\nAPI_KEY=hisar://Work/Stripe/api%20key\nMODE=development\nDATABASE_URL=hisar://Work/Postgres/url\n"), 0o600)
	overrides := filepath.Join(t.TempDir(), "test.env")
	os.WriteFile(overrides, []byte("MODE=test\nHOME=/from/the/file\n"), 0o600)
	server := createTestServer(t, app)
	output := decodeOutput[RunOutput](t, server.callTool(t, "run_with_secrets", map[string]any{
		"shell":            `echo "$API_KEY $MODE $HOME $DATABASE_URL"`,
		"envFiles":         []string{".env", overrides},
		"env":              map[string]string{"DATABASE_URL": "hisar://Work/Postgres/admin%20url"},
		"workingDirectory": directory,
	}))
	if output.Stdout != "<concealed by hisar> test /nowhere <concealed by hisar>\n" {
		t.Errorf("output = %+v", output)
	}
	request := app.getLastRequest()
	if !slices.Equal(request.References, []string{"hisar://Work/Stripe/api%20key", "hisar://Work/Postgres/admin%20url"}) || request.WorkingDirectory != directory {
		t.Errorf("app request = %+v", request)
	}
}

func TestRunWithSecretsSendsStdin(t *testing.T) {
	requireUnix(t)
	app := &fakeApp{}
	server := createTestServer(t, app)
	output := decodeOutput[RunOutput](t, server.callTool(t, "run_with_secrets", map[string]any{
		"shell": `read -r password; test "$password" = "` + testSecret + `" && echo "logged in with $password"`,
		"stdin": "hisar://Work/Docker/password",
	}))
	if output.Stdout != "logged in with <concealed by hisar>\n" || output.ExitCode != 0 {
		t.Errorf("output = %+v", output)
	}
	if request := app.getLastRequest(); !slices.Equal(request.References, []string{"hisar://Work/Docker/password"}) {
		t.Errorf("the approval does not list the stdin secret: %+v", request)
	}

	output = decodeOutput[RunOutput](t, server.callTool(t, "run_with_secrets", map[string]any{
		"command": []string{"cat"},
		"env":     map[string]string{"TOKEN": "hisar://Work/Stripe/api%20key"},
		"stdin":   "plain text\n",
	}))
	if output.Stdout != "plain text\n" {
		t.Errorf("literal stdin: %+v", output)
	}
	output = decodeOutput[RunOutput](t, server.callTool(t, "run_with_secrets", map[string]any{
		"command": []string{"cat"},
		"env":     map[string]string{"TOKEN": "hisar://Work/Stripe/api%20key"},
	}))
	if output.Stdout != "" || output.ExitCode != 0 {
		t.Errorf("no stdin: %+v", output)
	}
}

func TestRunWithSecretsWarnsAboutValuesTooShortToMask(t *testing.T) {
	requireUnix(t)
	app := &fakeApp{answer: func(request cliprotocol.Request) (any, error) {
		return cliprotocol.ResolveSecretResult{Values: []string{"abc", "xyz"}}, nil
	}}
	server := createTestServer(t, app)
	output := decodeOutput[RunOutput](t, server.callTool(t, "run_with_secrets", map[string]any{
		"command": []string{"true"},
		"env":     map[string]string{"PIN": "hisar://Work/Phone/pin"},
		"stdin":   "hisar://Work/Door/code",
	}))
	if !slices.Equal(output.Warnings, []string{"PIN is shorter than 4 bytes and is not masked", "stdin is shorter than 4 bytes and is not masked"}) {
		t.Errorf("warnings = %q", output.Warnings)
	}
}

// installFakeDirenv puts a direnv on PATH that allows an .envrc containing "allowed", blocks any other, and
// whose exec sets FROM_DIRENV.
func installFakeDirenv(t *testing.T) {
	t.Helper()
	binDirectory := t.TempDir()
	script := `#!/bin/sh
case "$1" in
status)
	if [ -f .envrc ]; then
		allowed=1
		grep -q allowed .envrc && allowed=0
		printf '{"config":{},"state":{"foundRC":{"allowed":%d,"path":"%s/.envrc"},"loadedRC":null}}\n' "$allowed" "$PWD"
	else
		printf '{"config":{},"state":{"foundRC":null,"loadedRC":null}}\n'
	fi
	;;
exec)
	shift 2
	export FROM_DIRENV=yes
	exec "$@"
	;;
esac
`
	if err := os.WriteFile(filepath.Join(binDirectory, "direnv"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDirectory+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestRunWithSecretsGoesThroughDirenvOnlyForAnAllowedEnvrc(t *testing.T) {
	requireUnix(t)
	installFakeDirenv(t)
	app := &fakeApp{}
	server := createTestServer(t, app)
	allowedDirectory, blockedDirectory, plainDirectory := t.TempDir(), t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(allowedDirectory, ".envrc"), []byte("# allowed\n"), 0o600)
	os.WriteFile(filepath.Join(blockedDirectory, ".envrc"), []byte("# blocked\n"), 0o600)
	shell := `echo "direnv=$FROM_DIRENV token=$TOKEN"`
	cases := []struct {
		directory   string
		stdout      string
		commandLine []string
	}{
		{allowedDirectory, "direnv=yes token=<concealed by hisar>\n", []string{"direnv", "exec", allowedDirectory, "/bin/sh", "-c", shell}},
		{blockedDirectory, "direnv= token=<concealed by hisar>\n", []string{"/bin/sh", "-c", shell}},
		{plainDirectory, "direnv= token=<concealed by hisar>\n", []string{"/bin/sh", "-c", shell}},
	}
	for _, testCase := range cases {
		output := decodeOutput[RunOutput](t, server.callTool(t, "run_with_secrets", map[string]any{
			"shell":            shell,
			"env":              map[string]string{"TOKEN": "hisar://Work/Stripe/api%20key"},
			"workingDirectory": testCase.directory,
		}))
		if output.Stdout != testCase.stdout {
			t.Errorf("%s: output = %+v", testCase.directory, output)
		}
		if request := app.getLastRequest(); !slices.Equal(request.CommandLine, testCase.commandLine) {
			t.Errorf("%s: the approval shows %q", testCase.directory, request.CommandLine)
		}
	}
}

func TestRunWithSecretsReportsMaskedOutputAsProgress(t *testing.T) {
	requireUnix(t)
	server := createTestServer(t, &fakeApp{})
	firstHalf, secondHalf := testSecret[:7], testSecret[7:]
	params := &mcp.CallToolParams{Name: "run_with_secrets", Arguments: map[string]any{
		"shell": `echo first; sleep 1; printf %s ` + firstHalf + `; sleep 1; echo ` + secondHalf + `; echo "$TOKEN" >&2`,
		"env":   map[string]string{"TOKEN": "hisar://Work/Stripe/api%20key"},
	}}
	params.SetProgressToken("run-1")
	result, err := server.session.CallTool(context.Background(), params)
	if err != nil {
		t.Fatal(err)
	}
	output := decodeOutput[RunOutput](t, result)
	if output.Stdout != "first\n<concealed by hisar>\n" || output.Stderr != "<concealed by hisar>\n" {
		t.Errorf("output = %+v", output)
	}
	isComplete := func() bool {
		messages := ""
		for _, progress := range server.listProgress() {
			messages += progress.Message
		}
		return strings.Count(messages, "<concealed by hisar>") == 2
	}
	waitFor(t, "both secrets arrive as progress", isComplete)
	progress := server.listProgress()
	if len(progress) < 3 || progress[0].Message != "Waiting for approval in Hisar" || progress[1].Message != "first\n" {
		t.Fatalf("progress = %+v", progress)
	}
	for index, notification := range progress {
		if notification.ProgressToken != "run-1" || notification.Progress != float64(index+1) {
			t.Errorf("notification %d = %+v", index, notification)
		}
		if strings.Contains(notification.Message, firstHalf) || strings.Contains(notification.Message, secondHalf) {
			t.Errorf("notification %d shows part of the secret: %q", index, notification.Message)
		}
	}
}

func TestRunWithSecretsSendsNoProgressWithoutAToken(t *testing.T) {
	requireUnix(t)
	server := createTestServer(t, &fakeApp{})
	decodeOutput[RunOutput](t, server.callTool(t, "run_with_secrets", map[string]any{
		"command": []string{"echo", "hello"},
		"env":     map[string]string{"TOKEN": "hisar://Work/Stripe/api%20key"},
	}))
	if progress := server.listProgress(); len(progress) != 0 {
		t.Errorf("progress without a token: %+v", progress)
	}
}

func TestProgressStreamKeepsCharactersWhole(t *testing.T) {
	stream := &progressStream{}
	stream.Write([]byte("caf\xc3"))
	if text := stream.take(false); text != "caf" {
		t.Errorf("before the character is whole: %q", text)
	}
	stream.Write([]byte("\xa9 "))
	if text := stream.take(false); text != "é " {
		t.Errorf("after: %q", text)
	}
	stream.Write([]byte(strings.Repeat("a", maximumProgressLength+10)))
	if text := stream.take(true); !strings.HasPrefix(text, "[… 10 bytes not shown …]\naaa") || len(text) > maximumProgressLength+40 {
		t.Errorf("long output: %d bytes, %q", len(text), text[:40])
	}
}
