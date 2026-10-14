//go:build darwin && !ios && cgo

package windowplacement

import "testing"

func TestTopRightFrameOnEveryConnectedDisplay(t *testing.T) {
	displays := ReadDisplays()
	if len(displays) == 0 {
		t.Skip("no display connected")
	}
	for _, display := range displays {
		frame := CalculateTopRightFrame(display, 380, 120)
		t.Logf("display frame %+v visible frame %+v scale %v: card %+v", display.Frame, display.VisibleFrame, display.ScaleFactor, frame)
		assertInsideVisibleFrame(t, display, frame)
	}
}
