package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDisplayPreflightRejectsUnsafeLayouts(t *testing.T) {
	for _, test := range []struct {
		name      string
		here      int
		other     int
		width     float64
		wantError bool
	}{
		{"built-in visible seven", 7, 6, 1512, false},
		{"built-in visible six", 6, 7, 1512, false},
		{"built-in visible eight", 8, 6, 1512, false},
		{"external focused space", 4, 6, 1512, true},
		{"another display space", 7, 4, 1512, true},
		{"same space", 7, 7, 1512, true},
		{"too narrow", 7, 6, 1000, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			layout := displayLayout{Display: 2, VisibleSpace: test.here, OtherSpace: test.other, BuiltInSpaces: []int{6, 7, 8}, Frame: displayFrame{X: -1512, W: test.width, H: 982}}
			encoded, err := json.Marshal(layout)
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv("HISAR_TEST_DISPLAY_LAYOUT", string(encoded))
			_, err = readDisplayLayout()
			if (err != nil) != test.wantError {
				t.Fatalf("readDisplayLayout error = %v, wantError %v", err, test.wantError)
			}
		})
	}
	t.Setenv("HISAR_TEST_DISPLAY_LAYOUT", "")
	if _, err := readDisplayLayout(); err == nil {
		t.Fatal("missing display guard snapshot was accepted")
	}
}

func TestDisplayFrameRejectsExternalIntersection(t *testing.T) {
	frame := displayFrame{X: -1512, W: 1512, H: 982}
	for _, test := range []struct {
		name   string
		window displayFrame
		want   bool
	}{
		{"browser inside", displayFrame{X: -1432, Y: 80, W: 1100, H: 750}, true},
		{"exact right boundary", displayFrame{X: -1100, Y: 80, W: 1100, H: 750}, true},
		{"one point on external", displayFrame{X: -1099, Y: 80, W: 1100, H: 750}, false},
		{"below built-in", displayFrame{X: -1432, Y: 500, W: 1100, H: 750}, false},
		{"no bounds", displayFrame{}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := frame.contains(test.window); got != test.want {
				t.Fatalf("contains(%+v) = %v, want %v", test.window, got, test.want)
			}
		})
	}
}

func TestBrowserAdmissionRejectsVisibleExternalWindows(t *testing.T) {
	for _, browserName := range []string{"Brave", "Chrome"} {
		t.Run(browserName, func(t *testing.T) {
			directory := t.TempDir()
			tool := filepath.Join(directory, "yabai")
			script := "#!/bin/sh\ncase \"$*\" in\n" +
				"'-m query --windows --window 123') printf '%s\\n' '{\"frame\":{\"x\":10,\"y\":10,\"w\":1100,\"h\":750},\"space\":4}';;\n" +
				"'-m query --spaces') printf '%s\\n' '[{\"index\":4,\"display\":1,\"is-visible\":true}]';;\n" +
				"*) exit 7;;\nesac\n"
			if err := os.WriteFile(tool, []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", directory)
			shown := browser{app: browserApp{Name: browserName}, window: cuaWindow{WindowID: 123}}
			err := shown.verifyInitialFrame(displayLayout{Frame: displayFrame{X: -1512, W: 1512, H: 982}})
			if err == nil || !strings.Contains(err.Error(), "outside") {
				t.Fatalf("unsafe browser admission = %v", err)
			}
		})
	}
}

func TestChromeAdmissionRejectsHiddenExternalWindow(t *testing.T) {
	directory := t.TempDir()
	tool := filepath.Join(directory, "yabai")
	script := "#!/bin/sh\nprintf '%s\\n' '{\"frame\":{\"x\":10,\"y\":10,\"w\":1100,\"h\":750},\"space\":2}'\n"
	if err := os.WriteFile(tool, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory)
	shown := browser{app: browserApp{Name: "Chrome"}, window: cuaWindow{WindowID: 123}}
	if err := shown.verifyInitialFrame(displayLayout{Frame: displayFrame{X: -1512, W: 1512, H: 982}}); err == nil {
		t.Fatal("hidden external window was admitted")
	}
}

func TestSpaceParsingUsesVisibilityNotFocus(t *testing.T) {
	spaces, err := parseSpaces([]byte(`[{"index":4,"display":1,"has-focus":true,"is-visible":true},{"index":6,"display":2,"has-focus":false,"is-visible":false},{"index":7,"display":2,"has-focus":false,"is-visible":true}]`))
	if err != nil {
		t.Fatal(err)
	}
	if spaces[1].IsVisible || !spaces[2].IsVisible {
		t.Fatalf("built-in visibility = %+v", spaces)
	}
	t.Setenv("HISAR_TEST_DISPLAY_LAYOUT", `{"display":2,"visibleSpace":7,"otherSpace":6,"builtInSpaces":[6,7,8],"frame":{"x":-1512,"y":0,"w":1512,"h":982}}`)
	here, other, err := findSpaces()
	if err != nil || here != 7 || other != 6 {
		t.Fatalf("findSpaces = %d, %d, %v", here, other, err)
	}
}

func TestBrowserPlacementFloatsOnlyTheOwnedWindow(t *testing.T) {
	directory := t.TempDir()
	floating := filepath.Join(directory, "floating")
	placed := filepath.Join(directory, "placed")
	request := filepath.Join(directory, "request.json")
	yabai := "#!/bin/sh\ncase \"$*\" in\n" +
		"'-m query --windows --window 456')\n" +
		"if [ -f " + placed + " ]; then printf '%s\\n' '{\"is-floating\":true,\"frame\":{\"x\":80,\"y\":80,\"w\":1100,\"h\":750}}';\n" +
		"else printf '%s\\n' '{\"is-floating\":false,\"frame\":{\"x\":1137,\"y\":50,\"w\":500,\"h\":463}}'; fi;;\n" +
		"'-m window 456 --toggle float') : > " + floating + ";;\n" +
		"*) exit 7;;\nesac\n"
	driver := "#!/bin/sh\n[ \"$1\" = call ] && [ \"$2\" = set_window_frame ] && [ -f " + floating + " ] || exit 8\n" +
		"printf '%s' \"$3\" > " + request + "\n: > " + placed + "\nprintf '{}'\n"
	for name, script := range map[string]string{"yabai": yabai, "driver": driver} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", directory)
	t.Setenv("HISAR_TEST_CUA_SOCKET", filepath.Join(directory, "s", "cua.sock"))
	shown := browser{pid: 123, window: cuaWindow{WindowID: 456}}
	if err := shown.placeOnDisplay(cuaDriver{path: filepath.Join(directory, "driver")}, displayLayout{Frame: displayFrame{W: 1512, H: 982}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(request)
	if err != nil {
		t.Fatal(err)
	}
	var frame struct {
		PID      int     `json:"pid"`
		WindowID uint32  `json:"window_id"`
		X        float64 `json:"x"`
		Y        float64 `json:"y"`
		Width    float64 `json:"width"`
		Height   float64 `json:"height"`
	}
	if err := json.Unmarshal(data, &frame); err != nil {
		t.Fatal(err)
	}
	if frame.PID != 123 || frame.WindowID != 456 || frame.X != 80 || frame.Y != 80 || frame.Width != 1100 || frame.Height != 750 {
		t.Fatalf("placed the wrong window or frame: %+v", frame)
	}
}
