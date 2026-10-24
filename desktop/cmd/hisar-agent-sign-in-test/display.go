package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"slices"
	"strconv"
)

// displayLayout is the native preflight snapshot in Quartz coordinates.
type displayLayout struct {
	Display       int          `json:"display"`
	VisibleSpace  int          `json:"visibleSpace"`
	OtherSpace    int          `json:"otherSpace"`
	BuiltInSpaces []int        `json:"builtInSpaces"`
	Frame         displayFrame `json:"frame"`
}

type displayFrame struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

func (frame displayFrame) contains(other displayFrame) bool {
	return other.W > 0 && other.H > 0 && other.X >= frame.X && other.Y >= frame.Y &&
		other.X+other.W <= frame.X+frame.W && other.Y+other.H <= frame.Y+frame.H
}

func readDisplayLayout() (displayLayout, error) {
	var layout displayLayout
	if err := json.Unmarshal([]byte(os.Getenv("HISAR_TEST_DISPLAY_LAYOUT")), &layout); err != nil {
		return layout, fmt.Errorf("display preflight: %w", err)
	}
	if layout.Display == 0 || layout.VisibleSpace == layout.OtherSpace ||
		!slices.Contains(layout.BuiltInSpaces, layout.VisibleSpace) || !slices.Contains(layout.BuiltInSpaces, layout.OtherSpace) ||
		layout.Frame.W < 1180 || layout.Frame.H < 830 {
		return layout, fmt.Errorf("display preflight has no suitable built-in display: %+v", layout)
	}
	return layout, nil
}

func (launched *browser) placeOnDisplay(driver cuaDriver, layout displayLayout) error {
	windowID := strconv.Itoa(int(launched.window.WindowID))
	output, err := exec.Command("yabai", "-m", "query", "--windows", "--window", windowID).Output()
	if err != nil {
		return fmt.Errorf("reading the owned browser's tiling state: %w", err)
	}
	var shown struct {
		IsFloating bool `json:"is-floating"`
	}
	if err := json.Unmarshal(output, &shown); err != nil {
		return err
	}
	if !shown.IsFloating {
		if output, err := exec.Command("yabai", "-m", "window", windowID, "--toggle", "float").CombinedOutput(); err != nil {
			return fmt.Errorf("floating the owned browser window: %w: %s", err, output)
		}
	}
	if _, err := driver.call("set_window_frame", map[string]any{
		"pid": launched.pid, "window_id": launched.window.WindowID,
		"x": layout.Frame.X + 80, "y": layout.Frame.Y + 80, "width": 1100, "height": 750,
	}); err != nil {
		return err
	}
	return launched.verifyInitialFrame(layout)
}

func (launched *browser) verifyInitialFrame(layout displayLayout) error {
	output, err := exec.Command("yabai", "-m", "query", "--windows", "--window", strconv.Itoa(int(launched.window.WindowID))).Output()
	if err != nil {
		return fmt.Errorf("reading the first browser frame: %w", err)
	}
	var shown struct {
		Frame displayFrame `json:"frame"`
	}
	if err := json.Unmarshal(output, &shown); err != nil {
		return err
	}
	if !layout.Frame.contains(shown.Frame) {
		return fmt.Errorf("%s first observed window frame is outside the built-in display: %+v", launched.app.Name, shown.Frame)
	}
	log.Printf("%s window %d first observed built-in frame %+v", launched.app.Name, launched.window.WindowID, shown.Frame)
	return nil
}

func driverSocket() string {
	return os.Getenv("HISAR_TEST_CUA_SOCKET")
}
