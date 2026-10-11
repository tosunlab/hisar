// Package windowplacement computes where Hisar's approval window goes, in Cocoa screen coordinates: points, origin at the bottom left of the primary display, Y up.
package windowplacement

import "math"

const CompactMargin = 16

type Point struct {
	X, Y float64
}

type Rectangle struct {
	X, Y, Width, Height float64
}

func (rectangle Rectangle) getRight() float64 { return rectangle.X + rectangle.Width }

func (rectangle Rectangle) getTop() float64 { return rectangle.Y + rectangle.Height }

// containsPoint includes the top edge: the pointer at the very top of a display reports Y equal to its top.
func (rectangle Rectangle) containsPoint(point Point) bool {
	return point.X >= rectangle.X && point.X < rectangle.getRight() && point.Y > rectangle.Y && point.Y <= rectangle.getTop()
}

func (rectangle Rectangle) calculateIntersectionArea(other Rectangle) float64 {
	width := math.Min(rectangle.getRight(), other.getRight()) - math.Max(rectangle.X, other.X)
	height := math.Min(rectangle.getTop(), other.getTop()) - math.Max(rectangle.Y, other.Y)
	if width <= 0 || height <= 0 {
		return 0
	}
	return width * height
}

// Display is an NSScreen: VisibleFrame excludes the menu bar (and the notch below it) and the Dock.
type Display struct {
	Frame        Rectangle
	VisibleFrame Rectangle
	ScaleFactor  float64
}

// SelectPointerDisplay returns the display under the pointer, else the first one (the primary display).
func SelectPointerDisplay(allDisplay []Display, pointer Point) (Display, bool) {
	for _, display := range allDisplay {
		if display.Frame.containsPoint(pointer) {
			return display, true
		}
	}
	if len(allDisplay) == 0 {
		return Display{}, false
	}
	return allDisplay[0], true
}

// SelectWindowDisplay returns the display holding most of the window, else the first one.
func SelectWindowDisplay(allDisplay []Display, windowFrame Rectangle) (Display, bool) {
	if len(allDisplay) == 0 {
		return Display{}, false
	}
	selectedDisplay := allDisplay[0]
	largestArea := 0.0
	for _, display := range allDisplay {
		if area := display.Frame.calculateIntersectionArea(windowFrame); area > largestArea {
			selectedDisplay, largestArea = display, area
		}
	}
	return selectedDisplay, true
}

// CalculateTopRightFrame puts the window at the top right of the display's visible frame, like a notification.
func CalculateTopRightFrame(display Display, width, height float64) Rectangle {
	visibleFrame := display.VisibleFrame
	return CalculateAnchoredFrame(display, Point{X: visibleFrame.getRight() - CompactMargin, Y: visibleFrame.getTop() - CompactMargin}, width, height)
}

// CalculateAnchoredFrame keeps the window's top-right corner at the anchor, moved inside the visible frame if needed.
func CalculateAnchoredFrame(display Display, topRightAnchor Point, width, height float64) Rectangle {
	visibleFrame := display.VisibleFrame
	width, height = limitSize(visibleFrame, width, height)
	right := math.Min(topRightAnchor.X, visibleFrame.getRight()-CompactMargin)
	right = math.Max(right, visibleFrame.X+CompactMargin+width)
	top := math.Min(topRightAnchor.Y, visibleFrame.getTop()-CompactMargin)
	top = math.Max(top, visibleFrame.Y+CompactMargin+height)
	return alignToPixel(display, Rectangle{X: right - width, Y: top - height, Width: width, Height: height})
}

// CalculateCenteredFrame centers the window in the display's visible frame.
func CalculateCenteredFrame(display Display, width, height float64) Rectangle {
	visibleFrame := display.VisibleFrame
	width, height = limitSize(visibleFrame, width, height)
	return alignToPixel(display, Rectangle{
		X:      visibleFrame.X + (visibleFrame.Width-width)/2,
		Y:      visibleFrame.Y + (visibleFrame.Height-height)/2,
		Width:  width,
		Height: height,
	})
}

type Request struct {
	Width, Height float64
	IsCompact     bool
	// IsShowing places the window on the pointer's display; otherwise a resize keeps it where it is.
	IsShowing bool
}

func CalculateWindowFrame(allDisplay []Display, pointer Point, currentFrame Rectangle, request Request) (Rectangle, bool) {
	var display Display
	var isFound bool
	if request.IsShowing {
		display, isFound = SelectPointerDisplay(allDisplay, pointer)
	} else {
		display, isFound = SelectWindowDisplay(allDisplay, currentFrame)
	}
	if !isFound {
		return Rectangle{}, false
	}
	switch {
	case !request.IsCompact:
		return CalculateCenteredFrame(display, request.Width, request.Height), true
	case request.IsShowing:
		return CalculateTopRightFrame(display, request.Width, request.Height), true
	default:
		return CalculateAnchoredFrame(display, Point{X: currentFrame.getRight(), Y: currentFrame.getTop()}, request.Width, request.Height), true
	}
}

func limitSize(visibleFrame Rectangle, width, height float64) (float64, float64) {
	return math.Max(1, math.Min(width, visibleFrame.Width-2*CompactMargin)), math.Max(1, math.Min(height, visibleFrame.Height-2*CompactMargin))
}

func alignToPixel(display Display, frame Rectangle) Rectangle {
	scaleFactor := display.ScaleFactor
	if scaleFactor <= 0 {
		scaleFactor = 1
	}
	alignValue := func(value float64) float64 { return math.Round(value*scaleFactor) / scaleFactor }
	right, top := alignValue(frame.getRight()), alignValue(frame.getTop())
	width, height := alignValue(frame.Width), alignValue(frame.Height)
	return Rectangle{X: right - width, Y: top - height, Width: width, Height: height}
}
