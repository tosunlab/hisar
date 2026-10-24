package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// axTool is scripts/agent-sign-in/ax.swift, built: it reads and operates the development app's windows through
// Accessibility and the keyboard, as a user does.
type axTool struct {
	path string
}

type axElement struct {
	Role        string `json:"role"`
	Subrole     string `json:"subrole"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Value       string `json:"value"`
	Enabled     bool   `json:"enabled"`
	Focused     bool   `json:"focused"`
	Window      string `json:"window"`
	Depth       int    `json:"depth"`
}

func (element axElement) name() string {
	for _, name := range []string{element.Title, element.Description, element.Value} {
		if name = strings.TrimSpace(name); name != "" {
			return name
		}
	}
	return ""
}

type axTree []axElement

func (tool axTool) run(arguments ...string) ([]byte, error) {
	output, err := exec.Command(tool.path, arguments...).CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("ax %s: %v: %s", strings.Join(arguments, " "), err, bytes.TrimSpace(output))
	}
	return output, nil
}

func (tool axTool) window(pid int, title string) (axTree, error) {
	output, err := tool.run("tree", strconv.Itoa(pid))
	if err != nil {
		return nil, err
	}
	tree := axTree{}
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 1<<16), 1<<22)
	for scanner.Scan() {
		var element axElement
		if err := json.Unmarshal(scanner.Bytes(), &element); err != nil {
			return nil, fmt.Errorf("ax tree: %w: %s", err, scanner.Bytes())
		}
		if element.Window == title {
			tree = append(tree, element)
		}
	}
	return tree, nil
}

func (tree axTree) text() string {
	var text strings.Builder
	for _, element := range tree {
		if name := element.name(); name != "" {
			text.WriteString(name + "\n")
		}
	}
	return text.String()
}

func (tree axTree) find(role, name string) (axElement, bool) {
	for _, element := range tree {
		if element.Role == role && element.name() == name {
			return element, true
		}
	}
	return axElement{}, false
}

func (tool axTool) press(pid int, selector string) error {
	_, err := tool.run("press", strconv.Itoa(pid), selector)
	return err
}

func (tool axTool) focus(pid int, selector string) error {
	_, err := tool.run("focus", strconv.Itoa(pid), selector)
	return err
}

func (tool axTool) typeText(pid int, text string) error {
	_, err := tool.run("type", strconv.Itoa(pid), text)
	return err
}
