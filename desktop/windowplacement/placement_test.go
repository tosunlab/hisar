package windowplacement

import "testing"

var builtInDisplay = Display{
	Frame:        Rectangle{X: 0, Y: 0, Width: 1800, Height: 1169},
	VisibleFrame: Rectangle{X: 0, Y: 0, Width: 1800, Height: 1130},
	ScaleFactor:  2,
}

func assertInsideVisibleFrame(t *testing.T, display Display, frame Rectangle) {
	t.Helper()
	visibleFrame := display.VisibleFrame
	if frame.X < visibleFrame.X+CompactMargin || frame.Y < visibleFrame.Y+CompactMargin ||
		frame.getRight() > visibleFrame.getRight()-CompactMargin || frame.getTop() > visibleFrame.getTop()-CompactMargin {
		t.Fatalf("frame %+v is not inside %+v with a %v point margin", frame, visibleFrame, CompactMargin)
	}
}

func assertFrame(t *testing.T, got, want Rectangle) {
	t.Helper()
	if got != want {
		t.Fatalf("frame %+v, want %+v", got, want)
	}
}

func TestTopRightFrameSitsBelowMenuBarAndNotch(t *testing.T) {
	frame := CalculateTopRightFrame(builtInDisplay, 380, 120)
	assertFrame(t, frame, Rectangle{X: 1800 - 16 - 380, Y: 1130 - 16 - 120, Width: 380, Height: 120})
	assertInsideVisibleFrame(t, builtInDisplay, frame)
}

func TestTopRightFrameStaysLeftOfDockOnTheRight(t *testing.T) {
	display := Display{
		Frame:        Rectangle{X: 0, Y: 0, Width: 1512, Height: 982},
		VisibleFrame: Rectangle{X: 0, Y: 0, Width: 1440, Height: 949},
		ScaleFactor:  2,
	}
	frame := CalculateTopRightFrame(display, 380, 120)
	assertFrame(t, frame, Rectangle{X: 1440 - 16 - 380, Y: 949 - 16 - 120, Width: 380, Height: 120})
	assertInsideVisibleFrame(t, display, frame)
}

func TestTopRightFrameOnSecondaryDisplayLeftOfPrimary(t *testing.T) {
	secondaryDisplay := Display{
		Frame:        Rectangle{X: -2560, Y: -200, Width: 2560, Height: 1440},
		VisibleFrame: Rectangle{X: -2560, Y: -200, Width: 2560, Height: 1415},
		ScaleFactor:  1,
	}
	frame := CalculateTopRightFrame(secondaryDisplay, 380, 120)
	assertFrame(t, frame, Rectangle{X: -16 - 380, Y: 1215 - 16 - 120, Width: 380, Height: 120})
	assertInsideVisibleFrame(t, secondaryDisplay, frame)
}

func TestTopRightFrameOnSecondaryDisplayAbovePrimary(t *testing.T) {
	secondaryDisplay := Display{
		Frame:        Rectangle{X: -400, Y: 1169, Width: 2560, Height: 1440},
		VisibleFrame: Rectangle{X: -400, Y: 1169, Width: 2560, Height: 1415},
		ScaleFactor:  1,
	}
	frame := CalculateTopRightFrame(secondaryDisplay, 380, 120)
	assertFrame(t, frame, Rectangle{X: 2160 - 16 - 380, Y: 2584 - 16 - 120, Width: 380, Height: 120})
	assertInsideVisibleFrame(t, secondaryDisplay, frame)
}

func TestFrameIsAlignedToRetinaPixels(t *testing.T) {
	display := Display{
		Frame:        Rectangle{X: 0, Y: 0, Width: 1800.5, Height: 1169},
		VisibleFrame: Rectangle{X: 0, Y: 0, Width: 1800.5, Height: 1130.25},
		ScaleFactor:  2,
	}
	frame := CalculateTopRightFrame(display, 380.3, 120)
	for _, value := range []float64{frame.X, frame.Y, frame.Width, frame.Height} {
		if value*2 != float64(int(value*2)) {
			t.Fatalf("frame %+v is not on the pixel grid", frame)
		}
	}
}

func TestTallContentIsLimitedToVisibleFrame(t *testing.T) {
	frame := CalculateTopRightFrame(builtInDisplay, 380, 5000)
	if frame.Height != 1130-2*16 {
		t.Fatalf("height %v", frame.Height)
	}
	assertInsideVisibleFrame(t, builtInDisplay, frame)
}

func TestHeightChangeKeepsTopRightAnchored(t *testing.T) {
	shownFrame := CalculateTopRightFrame(builtInDisplay, 380, 120)
	grownFrame, isFound := CalculateWindowFrame([]Display{builtInDisplay}, Point{}, shownFrame, Request{Width: 380, Height: 260, IsCompact: true})
	if !isFound {
		t.Fatal("no display")
	}
	if grownFrame.getRight() != shownFrame.getRight() || grownFrame.getTop() != shownFrame.getTop() || grownFrame.Height != 260 {
		t.Fatalf("grown %+v from %+v", grownFrame, shownFrame)
	}
	assertInsideVisibleFrame(t, builtInDisplay, grownFrame)
}

func TestAnchoredFrameIsMovedBackInsideVisibleFrame(t *testing.T) {
	frame := CalculateAnchoredFrame(builtInDisplay, Point{X: 3208 + 380, Y: 1169}, 380, 120)
	assertFrame(t, frame, Rectangle{X: 1800 - 16 - 380, Y: 1130 - 16 - 120, Width: 380, Height: 120})
}

func TestShowingUsesPointerDisplay(t *testing.T) {
	secondaryDisplay := Display{
		Frame:        Rectangle{X: 1800, Y: 0, Width: 2560, Height: 1440},
		VisibleFrame: Rectangle{X: 1800, Y: 0, Width: 2560, Height: 1415},
		ScaleFactor:  1,
	}
	displays := []Display{builtInDisplay, secondaryDisplay}
	staleFrame := Rectangle{X: 100, Y: 100, Width: 380, Height: 120}
	frame, _ := CalculateWindowFrame(displays, Point{X: 3000, Y: 700}, staleFrame, Request{Width: 380, Height: 120, IsCompact: true, IsShowing: true})
	assertFrame(t, frame, Rectangle{X: 4360 - 16 - 380, Y: 1415 - 16 - 120, Width: 380, Height: 120})

	frame, _ = CalculateWindowFrame(displays, Point{X: 900, Y: 1169}, staleFrame, Request{Width: 380, Height: 120, IsCompact: true, IsShowing: true})
	assertInsideVisibleFrame(t, builtInDisplay, frame)
}

func TestPointerOutsideEveryDisplayFallsBackToPrimary(t *testing.T) {
	display, isFound := SelectPointerDisplay([]Display{builtInDisplay}, Point{X: -50, Y: -50})
	if !isFound || display != builtInDisplay {
		t.Fatalf("display %+v", display)
	}
	if _, isFound := SelectPointerDisplay(nil, Point{}); isFound {
		t.Fatal("found a display in an empty list")
	}
}

func TestFullDialogIsCenteredOnWindowDisplay(t *testing.T) {
	secondaryDisplay := Display{
		Frame:        Rectangle{X: -1920, Y: 0, Width: 1920, Height: 1080},
		VisibleFrame: Rectangle{X: -1920, Y: 0, Width: 1920, Height: 1055},
		ScaleFactor:  1,
	}
	compactFrame := CalculateTopRightFrame(secondaryDisplay, 380, 120)
	frame, _ := CalculateWindowFrame([]Display{builtInDisplay, secondaryDisplay}, Point{X: 900, Y: 500}, compactFrame, Request{Width: 480, Height: 400})
	assertFrame(t, frame, Rectangle{X: -1920 + (1920-480)/2, Y: 328, Width: 480, Height: 400})
}
