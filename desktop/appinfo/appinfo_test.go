package appinfo

import (
	"errors"
	"testing"
)

func TestFindApp(t *testing.T) {
	allPathByPID := map[int]string{
		500: "/usr/bin/ssh",
		400: "/bin/zsh",
		300: "/System/Applications/Utilities/Terminal.app/Contents/MacOS/Terminal",
		200: "/Applications/Visual Studio Code.app/Contents/Frameworks/Code Helper (Plugin).app/Contents/MacOS/Code Helper (Plugin)",
	}
	allParentByPID := map[int]int{500: 400, 400: 300, 300: 1, 200: 1, 600: 1}
	getProcessPath := func(pid int) (string, error) {
		if path, found := allPathByPID[pid]; found {
			return path, nil
		}
		return "", errors.New("no such process")
	}
	getParentPID := func(pid int) (int, error) {
		if parentPID, found := allParentByPID[pid]; found {
			return parentPID, nil
		}
		return 0, errors.New("no such process")
	}

	app, isFound := FindApp(500, getProcessPath, getParentPID)
	if !isFound || app.Path != "/System/Applications/Utilities/Terminal.app" || app.Name == "" {
		t.Errorf("ssh in Terminal: %+v %v", app, isFound)
	}
	if app, _ := FindApp(200, getProcessPath, getParentPID); app.Path != "/Applications/Visual Studio Code.app" {
		t.Errorf("helper app: %+v", app)
	}
	if _, isFound := FindApp(600, getProcessPath, getParentPID); isFound {
		t.Error("a process without an app found one")
	}
	if _, isFound := FindApp(999, getProcessPath, getParentPID); isFound {
		t.Error("an unknown process found an app")
	}
	looping := func(pid int) (int, error) { return pid + 1, nil }
	if _, isFound := FindApp(2, func(int) (string, error) { return "/usr/bin/x", nil }, looping); isFound {
		t.Error("an endless ancestry found an app")
	}
}
