package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type cuaDriver struct {
	path       string
	transcript *bytes.Buffer
}

type cuaWindow struct {
	WindowID       uint32 `json:"window_id"`
	PID            int    `json:"pid"`
	AppName        string `json:"app_name"`
	Title          string `json:"title"`
	IsOnScreen     bool   `json:"is_on_screen"`
	OnCurrentSpace bool   `json:"on_current_space"`
	SpaceIDs       []int  `json:"space_ids"`
	Bounds         struct {
		Width  float64 `json:"width"`
		Height float64 `json:"height"`
	} `json:"bounds"`
}

func (driver cuaDriver) call(tool string, arguments any) ([]byte, error) {
	encoded, _ := json.Marshal(arguments)
	command := exec.Command(driver.path, "call", tool, string(encoded), "--socket", driverSocket())
	command.Env = append(os.Environ(), "HOME="+filepath.Join(filepath.Dir(filepath.Dir(driverSocket())), "driver-home"))
	output, err := command.CombinedOutput()
	if driver.transcript != nil {
		fmt.Fprintf(driver.transcript, "== %s %s\n%s\n", tool, encoded, output)
	}
	if err != nil {
		return output, fmt.Errorf("cua-driver %s: %v: %s", tool, err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

func (driver cuaDriver) version() string {
	command := exec.Command(driver.path, "--version")
	command.Env = append(os.Environ(), "HOME="+filepath.Join(filepath.Dir(filepath.Dir(driverSocket())), "driver-home"))
	output, _ := command.Output()
	return strings.TrimSpace(string(output))
}

func (driver cuaDriver) listWindows(pid int) ([]cuaWindow, error) {
	output, err := driver.call("list_windows", map[string]any{"pid": pid})
	if err != nil {
		return nil, err
	}
	var listed struct {
		Windows []cuaWindow `json:"windows"`
	}
	if err := json.Unmarshal(output, &listed); err != nil {
		return nil, fmt.Errorf("list_windows answered %s", output)
	}
	return listed.Windows, nil
}

func (driver cuaDriver) waitForWindow(pid int, timeout time.Duration, isTitle func(string) bool) (cuaWindow, error) {
	var last []cuaWindow
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); time.Sleep(150 * time.Millisecond) {
		windows, err := driver.listWindows(pid)
		if err != nil {
			return cuaWindow{}, err
		}
		last = windows
		for _, window := range windows {
			if window.Bounds.Width > 200 && window.Bounds.Height > 200 && isTitle(window.Title) {
				return window, nil
			}
		}
	}
	return cuaWindow{}, fmt.Errorf("process %d showed no matching window: %+v", pid, last)
}

func (driver cuaDriver) captureWindow(window cuaWindow, screenshotPath string) (string, error) {
	output, err := driver.call("get_window_state", map[string]any{"pid": window.PID, "window_id": window.WindowID, "screenshot_out_file": screenshotPath})
	return string(output), err
}

func (driver cuaDriver) recordSubmission(window cuaWindow, directory string) (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.Command(driver.path, "mcp", "--socket", driverSocket())
	command.Env = append(os.Environ(), "HOME="+filepath.Join(filepath.Dir(filepath.Dir(driverSocket())), "driver-home"))
	var diagnostic bytes.Buffer
	command.Stderr = &diagnostic
	defer func() {
		if driver.transcript != nil {
			driver.transcript.Write(diagnostic.Bytes())
		}
	}()
	client := mcp.NewClient(&mcp.Implementation{Name: "sign-in-recording-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		return err
	}
	defer session.Close()
	call := func(name string, arguments map[string]any) ([]byte, error) {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(result)
		if driver.transcript != nil {
			fmt.Fprintf(driver.transcript, "== recording %s\n%s\n", name, raw)
		}
		if result.IsError {
			return nil, fmt.Errorf("recording %s failed: %s", name, raw)
		}
		if result.StructuredContent != nil {
			return json.Marshal(result.StructuredContent)
		}
		for _, content := range result.Content {
			if text, ok := content.(*mcp.TextContent); ok {
				return []byte(text.Text), err
			}
		}
		return nil, errors.New("cua recording tool returned no content")
	}
	if _, err := call("start_recording", map[string]any{"output_dir": directory, "record_video": false}); err != nil {
		return err
	}
	defer func() {
		_, stopErr := call("stop_recording", map[string]any{})
		err = errors.Join(err, stopErr)
	}()
	output, err := call("get_window_state", map[string]any{"pid": window.PID, "window_id": window.WindowID, "include_screenshot": false, "query": "Sign in"})
	if err != nil {
		return err
	}
	token, err := findButtonToken(output, "Sign in")
	if err != nil {
		return err
	}
	_, err = call("click", map[string]any{"pid": window.PID, "window_id": window.WindowID, "element_token": token})
	return err
}

// findButtonToken reads a get_window_state answer for the element_token of the button labelled label: each CLI call is
// its own session, so only the token, which the daemon resolves, outlives the call that read it.
func findButtonToken(output []byte, label string) (string, error) {
	var state struct {
		Elements []struct {
			Token string `json:"element_token"`
			Role  string `json:"role"`
			Label string `json:"label"`
		} `json:"elements"`
	}
	if err := json.Unmarshal(output, &state); err != nil {
		return "", fmt.Errorf("get_window_state answered %.200s", output)
	}
	for _, element := range state.Elements {
		if element.Role == "AXButton" && strings.TrimSpace(element.Label) == label {
			return element.Token, nil
		}
	}
	return "", fmt.Errorf("the window shows no %s button", label)
}

type space struct {
	Index              int  `json:"index"`
	Display            int  `json:"display"`
	HasFocus           bool `json:"has-focus"`
	IsVisible          bool `json:"is-visible"`
	IsNativeFullscreen bool `json:"is-native-fullscreen"`
}

func parseSpaces(output []byte) ([]space, error) {
	var spaces []space
	if err := json.Unmarshal(output, &spaces); err != nil {
		return nil, fmt.Errorf("yabai listed spaces as %.200s", output)
	}
	return spaces, nil
}
