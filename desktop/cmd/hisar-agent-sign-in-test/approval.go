package main

import (
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"
)

const approvalTitle = "Hisar Access Requested"

var durations = []string{"Only this time", "For 1 hour", "For 1 day", "Always"}

type approvalWindow struct {
	ax  axTool
	pid int
}

func (window approvalWindow) selector(conditions string) string {
	return "window=" + approvalTitle + ";" + conditions
}

func (window approvalWindow) read() (axTree, error) {
	tree, err := window.ax.window(window.pid, approvalTitle)
	if err != nil || !strings.Contains(tree.text(), " wants to ") {
		return nil, err
	}
	return tree, nil
}

func (window approvalWindow) wait(timeout time.Duration) (axTree, error) {
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); time.Sleep(300 * time.Millisecond) {
		tree, err := window.read()
		if err != nil {
			return nil, err
		}
		if tree != nil {
			return tree, nil
		}
	}
	return nil, errors.New("no approval dialog appeared")
}

func (window approvalWindow) getDuration() (string, error) {
	tree, err := window.read()
	if err != nil {
		return "", err
	}
	for _, element := range tree {
		if element.Role == "AXPopUpButton" {
			return element.Value, nil
		}
	}
	return "", errors.New("the dialog has no duration menu")
}

func (window approvalWindow) chooseDuration(label string) error {
	current, err := window.getDuration()
	if err != nil {
		return err
	}
	if !slices.Contains(durations, label) {
		return fmt.Errorf("unknown approval duration %q", label)
	}
	if current == label {
		return nil
	}
	chosen, err := window.moveDuration(slices.Index(durations, label) - slices.Index(durations, current))
	if err != nil {
		return err
	}
	if chosen != label {
		return fmt.Errorf("the menu shows %q after choosing %q", chosen, label)
	}
	return nil
}

func (window approvalWindow) listDurations() ([]string, error) {
	if err := window.chooseDuration(durations[0]); err != nil {
		return nil, err
	}
	shown := []string{durations[0]}
	for {
		chosen, err := window.moveDuration(1)
		if err != nil {
			return nil, err
		}
		if chosen == shown[len(shown)-1] {
			return shown, window.chooseDuration(durations[0])
		}
		shown = append(shown, chosen)
	}
}

func (window approvalWindow) moveDuration(offset int) (string, error) {
	output, err := window.ax.run("menu", strconv.Itoa(window.pid), window.selector("role=AXPopUpButton"), strconv.Itoa(offset))
	return strings.TrimSpace(string(output)), err
}

func (window approvalWindow) choosePassword() error {
	tree, err := window.read()
	if err != nil {
		return err
	}
	if _, isOffered := tree.find("AXButton", "or use your master password"); isOffered {
		if err := window.ax.press(window.pid, window.selector("role=AXButton;name=or use your master password")); err != nil {
			return err
		}
		time.Sleep(300 * time.Millisecond)
	}
	return nil
}

func (window approvalWindow) typePassword(password string) error {
	if err := window.ax.focus(window.pid, window.selector("role=AXTextField;name=Master password")); err != nil {
		return err
	}
	return window.ax.typeText(window.pid, password)
}

func (window approvalWindow) pressApprove() error {
	return window.ax.press(window.pid, window.selector("role=AXButton;name=Approve"))
}

func (window approvalWindow) approve(loginText, duration, password string) error {
	if loginText != "" {
		if err := window.ax.press(window.pid, window.selector("role=AXRadioButton;name~"+loginText)); err != nil {
			return err
		}
	}
	if err := window.chooseDuration(duration); err != nil {
		return err
	}
	if err := window.choosePassword(); err != nil {
		return err
	}
	if err := window.typePassword(password); err != nil {
		return err
	}
	return window.pressApprove()
}

func (window approvalWindow) deny() error {
	return window.ax.press(window.pid, window.selector("role=AXButton;name=Deny"))
}

func touchIDRestarts(pid int, since time.Time) (cancelled, started int, err error) {
	output, err := exec.Command("log", "show", "--style", "compact", "--start", since.Format("2006-01-02 15:04:05"),
		"--predicate", `process == "coreauthd"`).Output()
	if err != nil {
		return 0, 0, fmt.Errorf("log show: %w", err)
	}
	process := strconv.Itoa(pid)
	for line := range strings.Lines(string(output)) {
		switch {
		case strings.Contains(line, "canceling running authentication") && strings.Contains(line, "pid:"+process+","):
			cancelled++
		case strings.Contains(line, "ContextProxy["+process+":") && strings.Contains(line, "] created"):
			started++
		}
	}
	return cancelled, started, nil
}
