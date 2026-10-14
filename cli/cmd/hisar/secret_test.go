package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/cliprotocol"
)

func answerSecretRequest(request cliprotocol.Request) (any, *cliprotocol.ResponseError) {
	if request.Type != cliprotocol.TypeResolveSecret {
		return answerFakeVaultRequest(request)
	}
	valuesByReference := map[string]string{
		"hisar://Personal/GitHub/password":    "github-password",
		"hisar://Personal/GitHub/API%20token": "token-value",
	}
	values := []string{}
	for _, reference := range request.References {
		value, isFound := valuesByReference[reference]
		if !isFound {
			return nil, cliprotocol.NewError(cliprotocol.ErrorNotFound, reference+`: no item in vault "Personal" is titled "Missing"`)
		}
		values = append(values, value)
	}
	return cliprotocol.ResolveSecretResult{Values: values}, nil
}

func TestReadThroughTheApp(t *testing.T) {
	app := startFakeApp(t)
	app.answer = answerSecretRequest
	stdout, _, err := runCapturingOutput(t, false, "", "read", "hisar://Personal/GitHub/password")
	if err != nil || stdout != "github-password" {
		t.Fatalf("read: %q, %v", stdout, err)
	}
	if stdout, _, _ := runCapturingOutput(t, false, "", "read", "hisar://Personal/GitHub/API%20token", "--no-newline"); stdout != "token-value" {
		t.Errorf("read --no-newline: %q", stdout)
	}
	requests := app.findRequest(cliprotocol.TypeResolveSecret)
	if len(requests) != 2 || !slices.Equal(requests[0].References, []string{"hisar://Personal/GitHub/password"}) || len(requests[0].CommandLine) != 0 {
		t.Errorf("requests %+v", requests)
	}
	if !slices.Equal(app.listRequestTypes(), []string{"hello", "secrets.resolve", "hello", "secrets.resolve"}) {
		t.Errorf("request types %v", app.listRequestTypes())
	}
}

func TestReadReportsBadReferencesWithoutAsking(t *testing.T) {
	app := startFakeApp(t)
	app.answer = answerSecretRequest
	if _, _, err := runCapturingOutput(t, false, "", "read", "hisar://Personal/GitHub"); err == nil || !strings.Contains(err.Error(), "hisar://Personal/GitHub: a secret reference has three parts") {
		t.Errorf("malformed: %v", err)
	}
	if _, _, err := runCapturingOutput(t, false, "", "read", "Personal/GitHub/password"); err == nil || !strings.Contains(err.Error(), "is not a secret reference") {
		t.Errorf("not a reference: %v", err)
	}
	if len(app.listRequestTypes()) != 0 {
		t.Errorf("asked the app: %v", app.listRequestTypes())
	}
	_, _, err := runCapturingOutput(t, false, "", "read", "hisar://Personal/Missing/password")
	var responseError *cliprotocol.ResponseError
	if !errors.As(err, &responseError) || responseError.Code != cliprotocol.ErrorNotFound || !strings.Contains(err.Error(), "hisar://Personal/Missing/password: no item") {
		t.Errorf("missing: %v", err)
	}
	if _, _, err := runCapturingOutput(t, false, "", "read"); !errors.Is(err, errUsage) {
		t.Errorf("no reference: %v", err)
	}
}

func TestRunThroughTheAppSendsOneRequest(t *testing.T) {
	app := startFakeApp(t)
	app.answer = answerSecretRequest
	environmentFile := filepath.Join(t.TempDir(), ".env")
	os.WriteFile(environmentFile, []byte("SECRET=hisar://Personal/GitHub/password\nTOKEN=hisar://Personal/GitHub/API%20token\nAGAIN=hisar://Personal/GitHub/password\nEXIT_CODE=4\n"), 0o600)
	t.Setenv(helperEnvironment, "print")
	stdout, stderr, err := runCapturingOutput(t, false, "", "run", "--env-file", environmentFile, "--", getTestBinary(t), "-test.run=none")
	var exitCode exitCodeError
	if !errors.As(err, &exitCode) || exitCode != 4 {
		t.Fatalf("exit: %v", err)
	}
	if stdout != "stdout: <concealed by hisar>\n" || stderr != "Approve hisar in this terminal in Hisar's approval window…\nstderr: <concealed by hisar>\n" {
		t.Errorf("stdout %q, stderr %q", stdout, stderr)
	}
	requests := app.findRequest(cliprotocol.TypeResolveSecret)
	if len(requests) != 1 || !slices.Equal(requests[0].References, []string{"hisar://Personal/GitHub/password", "hisar://Personal/GitHub/API%20token"}) ||
		!slices.Equal(requests[0].CommandLine, []string{getTestBinary(t), "-test.run=none"}) {
		t.Errorf("requests %+v", requests)
	}
}

func TestRunWithoutReferencesNeverAsksTheApp(t *testing.T) {
	app := startFakeApp(t)
	t.Setenv(helperEnvironment, "print")
	t.Setenv("SECRET", "not a reference")
	stdout, stderr, err := runCapturingOutput(t, false, "", "run", getTestBinary(t))
	if err != nil || stdout != "stdout: not a reference\n" || !strings.HasPrefix(stderr, "hisar: no secret references found\n") {
		t.Errorf("stdout %q, stderr %q, %v", stdout, stderr, err)
	}
	if len(app.listRequestTypes()) != 0 {
		t.Errorf("asked the app: %v", app.listRequestTypes())
	}
}
