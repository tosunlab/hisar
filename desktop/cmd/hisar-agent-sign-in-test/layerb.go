package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

type hermesRuntime struct {
	command     string
	home        string
	environment []string
	userConfig  string
	userHash    [32]byte
}

func (current *run) prepareHermes() (*hermesRuntime, error) {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	launcherPath := filepath.Join(userHome, ".local", "bin", "hermes")
	if info, err := os.Stat(launcherPath); err != nil || info.Mode()&0o111 == 0 {
		return nil, errors.New("the real Hermes launcher is not installed")
	}
	home := filepath.Join(current.directory, "hermes")
	for _, path := range []string{home, current.paths.Home, filepath.Join(current.directory, "tmp")} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return nil, err
		}
	}
	credentialPath := os.Getenv("AGENT_SIGN_IN_MODEL_ENV")
	if credentialPath == "" {
		credentialPath = "/tmp/hisar-e2e/model.env"
	}
	info, err := os.Stat(credentialPath)
	if err != nil {
		return nil, err
	}
	if info.Mode().Perm() != 0o600 {
		return nil, errors.New("model.env must have mode 600")
	}
	credentials, err := os.ReadFile(credentialPath)
	if err != nil {
		return nil, err
	}
	apiKey := ""
	for _, line := range strings.Split(string(credentials), "\n") {
		name, value, found := strings.Cut(strings.TrimPrefix(strings.TrimSpace(line), "export "), "=")
		if found && strings.TrimSpace(name) == "ANTHROPIC_API_KEY" {
			apiKey = strings.Trim(strings.TrimSpace(value), "\"'")
		}
	}
	if apiKey == "" {
		return nil, errors.New("model.env contains no Anthropic API key")
	}
	if err := os.WriteFile(filepath.Join(home, ".env"), credentials, 0o600); err != nil {
		return nil, err
	}
	config := "model:\n  provider: anthropic\n  default: claude-sonnet-4-6\ncomputer_use:\n  no_overlay: true\nsecurity:\n  allow_lazy_installs: true\nupdates:\n  check: false\nmemory:\n  memory_enabled: false\n  user_profile_enabled: false\n"
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte(config), 0o600); err != nil {
		return nil, err
	}
	sandbox := `(version 1) (allow default) (deny file-write* (subpath ` + strconv.Quote(filepath.Join(userHome, ".hermes")) + `))`
	command := filepath.Join(current.directory, "tools", "hermes")
	launcher := "#!/bin/sh\nexec /usr/bin/sandbox-exec -p " + shellQuote(sandbox) + " " + shellQuote(launcherPath) + " \"$@\"\n"
	if err := os.WriteFile(command, []byte(launcher), 0o700); err != nil {
		return nil, err
	}
	path := filepath.Dir(command) + ":" + filepath.Join(userHome, ".local", "bin") + ":/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin"
	profile := "export HERMES_HOME=" + shellQuote(home) + "\nexport PATH=" + shellQuote(path) + "\n"
	if err := os.WriteFile(filepath.Join(current.paths.Home, ".zshenv"), []byte(profile), 0o600); err != nil {
		return nil, err
	}
	userConfig := filepath.Join(userHome, ".hermes", "config.yaml")
	original, err := os.ReadFile(userConfig)
	if err != nil {
		return nil, err
	}
	runtime := &hermesRuntime{command: command, home: home, userConfig: userConfig, userHash: sha256.Sum256(original), environment: []string{
		"HOME=" + current.paths.Home, "SHELL=/bin/zsh", "PATH=" + path, "HERMES_HOME=" + home,
		"ANTHROPIC_API_KEY=" + apiKey, "HISAR_CLI_SOCKET=" + current.paths.CLISocket, "HISAR_NO_LAUNCH=1",
		"HERMES_CUA_DRIVER_CMD=" + current.cua.path, "TMPDIR=" + filepath.Join(current.directory, "tmp"), "CUA_DRIVER_RS_TELEMETRY_ENABLED=0",
	}}
	driverLog, err := os.OpenFile(filepath.Join(current.directory, "logs", "hermes-cua-daemon.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, err
	}
	driver := exec.Command(current.cua.path, "serve", "--no-overlay", "--no-permissions-gate")
	driver.Env = []string{"HOME=" + current.paths.Home, "PATH=" + path, "CUA_DRIVER_RS_TELEMETRY_ENABLED=0"}
	driver.Stdout, driver.Stderr = driverLog, driverLog
	if err := driver.Start(); err != nil {
		driverLog.Close()
		return nil, err
	}
	driverExited := make(chan struct{})
	go func() {
		driver.Wait()
		close(driverExited)
	}()
	current.onExit(func() {
		driver.Process.Signal(os.Interrupt)
		select {
		case <-driverExited:
		case <-time.After(10 * time.Second):
			driver.Process.Kill()
			<-driverExited
		}
		driverLog.Close()
	})
	for _, action := range []string{"status", "doctor"} {
		output, err := runtime.execute("computer-use", action)
		fmt.Printf("Isolated Hermes computer-use %s:\n%s\n", action, output)
		if err != nil {
			return nil, err
		}
	}
	version, err := runtime.execute("--version")
	if err != nil {
		return nil, err
	}
	if !bytes.Contains(version, []byte("Hermes Agent v0.22.2")) {
		return nil, fmt.Errorf("expected Hermes 0.22.2, got %s", version)
	}
	current.report.HermesVersion = strings.Split(string(version), "\n")[0]
	fmt.Printf("Hermes user config before SHA256=%x; private profile=%s\n", runtime.userHash, runtime.home)
	return runtime, nil
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }

func (runtime *hermesRuntime) execute(arguments ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, runtime.command, arguments...)
	command.Env, command.Dir = runtime.environment, runtime.home
	var diagnostic bytes.Buffer
	command.Stderr = &diagnostic
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("Hermes command failed: %w: %s%s", err, output, diagnostic.String())
	}
	return output, nil
}

func (current *browserRun) connectHermes(runtime *hermesRuntime) error {
	command := exec.Command("/bin/zsh", "-lic", "hermes config path")
	command.Env = []string{"HOME=" + current.paths.Home, "SHELL=/bin/zsh", "PATH=/usr/bin:/bin"}
	output, err := command.Output()
	if err != nil {
		return fmt.Errorf("proving Hermes config destination: %w: %s", err, output)
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	target := strings.TrimSpace(lines[len(lines)-1])
	if target != filepath.Join(runtime.home, "config.yaml") {
		return fmt.Errorf("login shell selected config %q instead of the private Hermes config", target)
	}
	fmt.Printf("Proved Settings Connect destination=%s\n", target)
	if err := current.desktop.showMainWindow(); err != nil {
		return err
	}
	pid := current.desktop.pid()
	for _, selector := range []string{"role=AXButton;name=Settings", "role=AXButton;name=Developer tools", "role=AXButton;name=Connect;after=Hermes"} {
		if err := current.waitAndPress(pid, "window=Hisar;"+selector); err != nil {
			return err
		}
	}
	var config struct {
		Command string            `json:"command"`
		Args    []string          `json:"args"`
		Env     map[string]string `json:"env"`
	}
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(300 * time.Millisecond) {
		output, err := runtime.execute("config", "get", "mcp_servers.hisar", "--json", "--raw")
		if err == nil && json.Unmarshal(output, &config) == nil && config.Command != "" {
			break
		}
	}
	if config.Command != filepath.Join(current.directory, "Helpers", "hisar") || strings.Join(config.Args, " ") != "mcp --no-reveal" || config.Env["HISAR_CLI_SOCKET"] != current.paths.CLISocket {
		return errors.New("Settings did not connect Hermes to the private CLI and socket")
	}
	return current.ax.press(pid, "window=Hisar;role=AXButton;name=Close settings")
}

type hermesMessage struct {
	Role      string `json:"role"`
	ToolName  string `json:"tool_name"`
	Content   string `json:"content"`
	ToolCalls string `json:"tool_calls"`
}

func (runtime *hermesRuntime) messages(after int) ([]hermesMessage, error) {
	output, err := exec.Command("sqlite3", "-json", filepath.Join(runtime.home, "state.db"), fmt.Sprintf("SELECT role,coalesce(tool_name,'') AS tool_name,coalesce(content,'') AS content,coalesce(tool_calls,'') AS tool_calls FROM messages WHERE id>%d ORDER BY id", after)).Output()
	if err != nil {
		return nil, err
	}
	var messages []hermesMessage
	if err := json.Unmarshal(output, &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

func (runtime *hermesRuntime) lastMessageID() int {
	output, _ := exec.Command("sqlite3", "-readonly", filepath.Join(runtime.home, "state.db"), "SELECT coalesce(max(id),0) FROM messages").Output()
	id, _ := strconv.Atoi(strings.TrimSpace(string(output)))
	return id
}

func (current *browserRun) runLayerB(runtime *hermesRuntime, only []string) {
	if err := current.connectHermes(runtime); err != nil {
		current.fatal(err)
	}
	if output, err := exec.Command("yabai", "-m", "window", strconv.FormatUint(uint64(current.browser.window.WindowID), 10), "--focus").CombinedOutput(); err != nil {
		current.fatal(fmt.Errorf("presenting the owned browser before Hermes: %w: %s", err, output))
	}
	for _, test := range []struct {
		name, path, title string
		posts             int
		refusal           bool
	}{
		{"plain", "/plain", "Plain sign-in", 1, false},
		{"two-step", "/step1", "Sign in, step 1", 2, false},
		{"totp", "/totp", "Sign in with a code", 2, false},
		{"wrong-password", "/wrong", "Sign in", 1, false},
		{"phishing", "/phish", "Sign in", 0, true},
	} {
		if len(only) > 0 && !slices.Contains(only, test.name) {
			continue
		}
		started := time.Now()
		pageURL := current.site.Main + test.path
		if test.refusal {
			pageURL = current.site.Other + test.path
		}
		err := current.browser.showPage(current.cua, current.site, pageURL, test.title)
		detail := ""
		if err == nil {
			detail, err = current.hermesScenario(runtime, test.name, test.posts, test.refusal)
		}
		if found := current.captureBrowser("hermes-"+test.name, current.browser); found != "" {
			err = errors.Join(err, errors.New(found))
		}
		if err != nil {
			detail += "; " + err.Error()
		}
		current.report.add(result{Layer: "B", Browser: current.app.Name, Scenario: test.name, Passed: err == nil, Detail: detail, Elapsed: time.Since(started)})
	}
	data, err := os.ReadFile(runtime.userConfig)
	if err != nil || sha256.Sum256(data) != runtime.userHash {
		current.fatal(errors.New("user Hermes config changed"))
	}
	fmt.Printf("Hermes user config after SHA256=%x; unchanged\n", sha256.Sum256(data))
}

func (current *browserRun) hermesScenario(runtime *hermesRuntime, name string, wantPosts int, refusal bool) (detail string, err error) {
	before := current.postCount(current.site)
	lastID := runtime.lastMessageID()
	prompt := fmt.Sprintf("This is an isolated website sign-in test. Work ONLY with the existing browser PID %d and window_id %d. Never activate, focus_app, raise, bring_to_front, move the pointer, switch Spaces, or touch other windows. First call computer_use with action=capture, mode=som, pid=%d, window_id=%d without focusing anything. Then call Hisar's sign_in tool for that exact pid and window_id using login=%s. The human test harness answers Hisar's approval UI. Do not type or read credentials yourself. Do not use a shell, browser automation, or any other tools. After sign_in returns, capture this same browser window through computer_use again and report the actual outcome. If the site rejects the password, do not retry. If Hisar refuses the phishing origin, do not bypass it. Call sign_in even if you expect refusal.", current.browser.pid, current.browser.window.WindowID, current.browser.pid, current.browser.window.WindowID, current.fixture.ID)
	logPath := filepath.Join(current.directory, "logs", "hermes-"+name+".log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return "", err
	}
	defer logFile.Close()
	defer func() {
		if err == nil {
			return
		}
		data, _ := os.ReadFile(logPath)
		text := current.redactHermes(runtime, string(data))
		if len(text) > 16000 {
			text = text[len(text)-16000:]
		}
		fmt.Printf("Hermes diagnostic %s:\n%s\n", name, text)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, runtime.command, "chat", "--oneshot", "--query-file", "-", "--toolsets", "computer_use,hisar", "--max-turns", "8", "--verbose")
	command.Env, command.Dir, command.Stdin, command.Stdout, command.Stderr = runtime.environment, runtime.home, strings.NewReader(prompt), logFile, logFile
	if err := command.Start(); err != nil {
		return "", err
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	approvals := 0
	for {
		select {
		case err = <-done:
			if err != nil {
				return "", fmt.Errorf("Hermes exited: %w; transcript=%s", err, logPath)
			}
			goto finished
		case <-time.After(250 * time.Millisecond):
			tree, readErr := current.approval().read()
			if readErr != nil {
				cancel()
				<-done
				return "", readErr
			}
			if tree != nil {
				fmt.Printf("Hermes approval AX %s:\n%s\n", name, current.redactHermes(runtime, tree.text()))
				if refusal {
					current.approval().deny()
					cancel()
					<-done
					return "", errors.New("phishing sign-in asked for approval")
				}
				missing := []string{}
				for _, expected := range []string{"Hermes", "wants to sign in to " + current.site.Main, current.fixture.Username} {
					if !strings.Contains(tree.text(), expected) {
						missing = append(missing, expected)
					}
				}
				if len(missing) > 0 {
					current.approval().deny()
					cancel()
					<-done
					return "", fmt.Errorf("approval did not identify the expected Hermes website sign-in; missing=%q", missing)
				}
				if err := current.approval().approve("", "Only this time", current.account.Password); err != nil {
					current.approval().deny()
					cancel()
					<-done
					return "", err
				}
				approvals++
			}
		}
	}
finished:
	messages, err := runtime.messages(lastID)
	if err != nil {
		return "", err
	}
	for _, message := range messages {
		if message.ToolCalls != "" || message.Role == "tool" {
			encoded, _ := json.Marshal(message)
			text := current.redactHermes(runtime, string(encoded))
			if len(text) > 6000 {
				text = text[:6000] + "[truncated]"
			}
			fmt.Printf("Hermes persisted record %s: %s\n", name, text)
		}
	}
	captures, signIns, capturesBefore, capturesAfter := 0, 0, 0, 0
	sawSignInResult := false
	tools := []string{}
	var answer signInAnswer
	for _, message := range messages {
		if message.Role == "tool" && strings.HasSuffix(message.ToolName, "sign_in") {
			answer, err = parseHermesSignIn(message.Content)
			if err != nil {
				return "", err
			}
			sawSignInResult = true
		}
		if message.Role == "tool" && message.ToolName == "computer_use" {
			if !hermesCaptureSucceeded(message.Content) {
				return "", errors.New("Hermes computer_use capture did not succeed")
			}
			if sawSignInResult {
				capturesAfter++
			} else {
				capturesBefore++
			}
		}
		if message.ToolCalls == "" {
			continue
		}
		calls, err := parseHermesCalls(message.ToolCalls)
		if err != nil {
			return "", err
		}
		for _, call := range calls {
			tools = append(tools, call.Name)
			switch call.Name {
			case "tool_search", "tool_describe":
			case "mcp__hisar__sign_in":
				var args struct {
					PID      int    `json:"pid"`
					WindowID uint32 `json:"window_id"`
					Login    string `json:"login"`
				}
				if err := json.Unmarshal(call.Arguments, &args); err != nil {
					return "", err
				}
				if args.PID != current.browser.pid || args.WindowID != current.browser.window.WindowID || args.Login != current.fixture.ID {
					return "", errors.New("Hermes attempted a wrong-target sign_in")
				}
				signIns++
			case "computer_use":
				var args struct {
					Action       string `json:"action"`
					PID          int    `json:"pid"`
					WindowID     uint32 `json:"window_id"`
					BringToFront bool   `json:"bring_to_front"`
				}
				if err := json.Unmarshal(call.Arguments, &args); err != nil {
					return "", err
				}
				if args.Action != "capture" || args.PID != current.browser.pid || args.WindowID != current.browser.window.WindowID || args.BringToFront {
					return "", errors.New("Hermes attempted a non-capture or wrong-target computer-use action")
				}
				captures++
			default:
				return "", fmt.Errorf("Hermes attempted an unexpected tool %q", call.Name)
			}
		}
	}
	posts := current.site.waitForPosts(before, wantPosts, 3*time.Second)
	if len(posts) != wantPosts || signIns != 1 || captures < 2 || capturesBefore < 1 || capturesAfter < 1 || captures != capturesBefore+capturesAfter || (!refusal && approvals != 1) {
		return "", fmt.Errorf("Hermes actions=%v captures=%d successfulBefore=%d successfulAfter=%d signIns=%d approvals=%d posts=%d, want %d posts", tools, captures, capturesBefore, capturesAfter, signIns, approvals, len(posts), wantPosts)
	}
	if refusal {
		if !answer.IsError {
			return "", errors.New("Hisar did not refuse the phishing origin")
		}
	} else if name == "wrong-password" {
		if answer.IsError || answer.Outcome != "not-accepted" || answer.Cleanup != "fields-cleared" {
			return "", errors.New("Hisar did not report not-accepted/fields-cleared")
		}
	} else if answer.IsError || answer.Outcome != "submitted" || answer.Cleanup != "page-left" || len(answer.Steps) != wantPosts {
		return "", errors.New("Hisar did not report the expected submitted/page-left steps")
	}
	if !refusal {
		found := false
		for _, post := range posts {
			if post.holds(current.fixture.Password) {
				found = true
			}
		}
		if !found {
			return "", errors.New("fixture did not receive its seeded password")
		}
	}
	if name == "wrong-password" {
		recording := filepath.Join(current.directory, "recording")
		if err := current.cua.recordSubmission(current.browser.window, recording); err != nil {
			return "", err
		}
		again := current.site.waitForPosts(before+wantPosts, 1, 5*time.Second)
		if len(again) != 1 || !strings.Contains(string(again[0].JSON), `"username":""`) || !strings.Contains(string(again[0].JSON), `"password":""`) {
			return "", errors.New("wrong-password page retained credentials after Hermes sign-in")
		}
		for _, name := range []string{"action.json", "before.png", "after.png"} {
			paths, err := filepath.Glob(filepath.Join(recording, "turn-*", name))
			if err != nil || len(paths) == 0 {
				return "", fmt.Errorf("cua recording contains no %s", name)
			}
		}
	}
	return fmt.Sprintf("Hermes tools=%v; %d captures, %d sign_in, %d UI approvals, %d fixture posts; Hisar outcome=%s cleanup=%s refused=%t", tools, captures, signIns, approvals, len(posts), answer.Outcome, answer.Cleanup, answer.IsError), nil
}

func (current *browserRun) redactHermes(runtime *hermesRuntime, text string) string {
	for _, entry := range runtime.environment {
		if key, value, found := strings.Cut(entry, "="); found && key == "ANTHROPIC_API_KEY" && value != "" {
			text = strings.ReplaceAll(text, value, "[model API key]")
		}
	}
	scan := newScanner(current.canaries, "", "")
	for needle, kind := range scan.needles {
		text = strings.ReplaceAll(text, needle, "["+kind+"]")
	}
	for code := range scan.codes {
		text = strings.ReplaceAll(text, code, "[one-time code]")
	}
	return text
}

type hermesCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func parseHermesCalls(raw string) ([]hermesCall, error) {
	var records []struct {
		Function hermesCall `json:"function"`
	}
	if err := json.Unmarshal([]byte(raw), &records); err != nil {
		return nil, err
	}
	var calls []hermesCall
	for _, record := range records {
		call := record.Function
		var encoded string
		if json.Unmarshal(call.Arguments, &encoded) == nil {
			call.Arguments = json.RawMessage(encoded)
		}
		if call.Name == "tool_call" {
			var broker struct {
				Calls []hermesCall `json:"calls"`
			}
			if err := json.Unmarshal(call.Arguments, &broker); err != nil {
				return nil, err
			}
			if len(broker.Calls) == 0 {
				return nil, errors.New("Hermes tool_call broker contains no calls")
			}
			calls = append(calls, broker.Calls...)
		} else {
			calls = append(calls, call)
		}
	}
	return calls, nil
}

func parseHermesSignIn(content string) (signInAnswer, error) {
	content = strings.TrimSpace(content)
	wrapped := strings.HasPrefix(content, "<untrusted_tool_result ")
	if wrapped {
		start := strings.IndexByte(content, '{')
		if start < 0 {
			return signInAnswer{}, errors.New("Hermes tool result wrapper contains no JSON")
		}
		content = content[start:]
	}
	var result struct {
		Structured json.RawMessage `json:"structuredContent"`
		Error      string          `json:"error"`
	}
	decoder := json.NewDecoder(strings.NewReader(content))
	if err := decoder.Decode(&result); err != nil {
		return signInAnswer{}, err
	}
	trailer := strings.TrimSpace(content[decoder.InputOffset():])
	if (!wrapped && trailer != "") || (wrapped && trailer != "</untrusted_tool_result>") {
		return signInAnswer{}, errors.New("unexpected Hermes tool result trailer")
	}
	if result.Error != "" {
		return signInAnswer{IsError: true, Text: result.Error}, nil
	}
	var answer signInAnswer
	if len(result.Structured) == 0 {
		return answer, errors.New("Hermes sign_in result contains no structuredContent")
	}
	if err := json.Unmarshal(result.Structured, &answer); err != nil {
		return answer, err
	}
	return answer, nil
}

func hermesCaptureSucceeded(content string) bool {
	if strings.HasPrefix(content, "capture mode=som ") && strings.Contains(content, "shareable screenshot saved to ") {
		return true
	}
	var capture struct {
		Mode   string `json:"mode"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
	}
	return json.Unmarshal([]byte(content), &capture) == nil && capture.Mode == "som" && capture.Width > 0 && capture.Height > 0
}
