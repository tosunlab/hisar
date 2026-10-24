package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// harnessName is the file name the scripted agent runs under: Hisar recognizes Claude Code's native install by it, so the
// agent is a harness program (identified by its path) and its dialogs offer the durations a harness gets, whatever
// terminal or agent runs this test.
const harnessName = "claude"

type agent struct {
	session *mcp.ClientSession
	// transcript is everything the agent sent and received, and hisar's standard error, for the canary scan.
	transcript *lockedBuffer
}

type lockedBuffer struct {
	mutex  sync.Mutex
	buffer bytes.Buffer
}

func (locked *lockedBuffer) Write(data []byte) (int, error) {
	locked.mutex.Lock()
	defer locked.mutex.Unlock()
	return locked.buffer.Write(data)
}

func (locked *lockedBuffer) Bytes() []byte {
	locked.mutex.Lock()
	defer locked.mutex.Unlock()
	return bytes.Clone(locked.buffer.Bytes())
}

type signInAnswer struct {
	Outcome string `json:"outcome"`
	Cleanup string `json:"cleanup"`
	Website string `json:"website"`
	Browser string `json:"browser"`
	Login   struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Username string `json:"username"`
	} `json:"login"`
	Steps []struct {
		Filled    []string `json:"filled"`
		Submitted bool     `json:"submitted"`
	} `json:"steps"`
	Message string `json:"message"`
	IsError bool   `json:"-"`
	Text    string `json:"-"`
}

func connectAgent(ctx context.Context, harnessPath, hisarPath, socketPath string, transcript *lockedBuffer) (*agent, error) {
	command := exec.Command(harnessPath, "relay", hisarPath, "mcp", "--no-reveal")
	command.Env = append(os.Environ(), "HISAR_CLI_SOCKET="+socketPath, "HISAR_NO_LAUNCH=1")
	command.Env = removeVariable(command.Env, "HISAR_STANDALONE")
	command.Stderr = transcript
	client := mcp.NewClient(&mcp.Implementation{Name: "agent-sign-in-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		return nil, err
	}
	return &agent{session: session, transcript: transcript}, nil
}

func removeVariable(environment []string, name string) []string {
	kept := environment[:0:0]
	for _, entry := range environment {
		if !strings.HasPrefix(entry, name+"=") {
			kept = append(kept, entry)
		}
	}
	return kept
}

func (client *agent) close() {
	client.session.Close()
}

func (client *agent) signIn(ctx context.Context, window cuaWindow, loginID string) (signInAnswer, error) {
	arguments := map[string]any{"pid": window.PID, "window_id": window.WindowID}
	if loginID != "" {
		arguments["login"] = loginID
	}
	encoded, _ := json.Marshal(arguments)
	client.transcript.Write(append([]byte("call sign_in "), append(encoded, '\n')...))
	result, err := client.session.CallTool(ctx, &mcp.CallToolParams{Name: "sign_in", Arguments: arguments})
	if err != nil {
		return signInAnswer{}, err
	}
	raw, _ := json.Marshal(result)
	client.transcript.Write(append(raw, '\n'))
	var answer signInAnswer
	if result.StructuredContent != nil {
		structured, _ := json.Marshal(result.StructuredContent)
		if err := json.Unmarshal(structured, &answer); err != nil {
			return signInAnswer{}, err
		}
	}
	answer.IsError = result.IsError
	for _, content := range result.Content {
		if text, isText := content.(*mcp.TextContent); isText {
			answer.Text += text.Text
		}
	}
	if answer.IsError && answer.Text == "" {
		return answer, errors.New("an error without a message")
	}
	return answer, nil
}

// relay runs the rest of the arguments as a child with this process's standard streams and exits as it does, so the
// child's parent is the harness program.
func relay(arguments []string) {
	command := exec.Command(arguments[0], arguments[1:]...)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := command.Run()
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		os.Exit(exit.ExitCode())
	case err != nil:
		io.WriteString(os.Stderr, err.Error()+"\n")
		os.Exit(1)
	}
}
