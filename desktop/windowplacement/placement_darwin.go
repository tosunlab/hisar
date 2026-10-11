//go:build darwin && !ios && cgo

package windowplacement

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework AppKit -framework Foundation
#import <AppKit/AppKit.h>

typedef struct {
	double x, y, width, height;
} PlacementRectangle;

typedef struct {
	PlacementRectangle frame;
	PlacementRectangle visibleFrame;
	double scaleFactor;
} PlacementDisplay;

static PlacementRectangle convertRectangle(NSRect rectangle) {
	return (PlacementRectangle){rectangle.origin.x, rectangle.origin.y, rectangle.size.width, rectangle.size.height};
}

static int readAllDisplay(PlacementDisplay* allDisplay, int capacity) {
	@autoreleasepool {
		NSArray<NSScreen*>* allScreen = [NSScreen screens];
		int count = 0;
		for (NSScreen* screen in allScreen) {
			if (count == capacity) {
				break;
			}
			allDisplay[count].frame = convertRectangle(screen.frame);
			allDisplay[count].visibleFrame = convertRectangle(screen.visibleFrame);
			allDisplay[count].scaleFactor = screen.backingScaleFactor;
			count++;
		}
		return count;
	}
}

static void readPointerLocation(double* x, double* y) {
	NSPoint location = [NSEvent mouseLocation];
	*x = location.x;
	*y = location.y;
}

static PlacementRectangle readWindowFrame(void* window) {
	return convertRectangle(((__bridge NSWindow*)window).frame);
}

static void setWindowFrame(void* window, PlacementRectangle frame) {
	[(__bridge NSWindow*)window setFrame:NSMakeRect(frame.x, frame.y, frame.width, frame.height) display:YES animate:NO];
}
*/
import "C"

import "unsafe"

const maximumDisplayCount = 32

func ReadAllDisplay() []Display {
	var allCDisplay [maximumDisplayCount]C.PlacementDisplay
	count := int(C.readAllDisplay(&allCDisplay[0], maximumDisplayCount))
	allDisplay := make([]Display, 0, count)
	for _, cDisplay := range allCDisplay[:count] {
		allDisplay = append(allDisplay, Display{
			Frame:        convertCRectangle(cDisplay.frame),
			VisibleFrame: convertCRectangle(cDisplay.visibleFrame),
			ScaleFactor:  float64(cDisplay.scaleFactor),
		})
	}
	return allDisplay
}

func ReadPointerLocation() Point {
	var x, y C.double
	C.readPointerLocation(&x, &y)
	return Point{X: float64(x), Y: float64(y)}
}

func ReadWindowFrame(nativeWindow unsafe.Pointer) Rectangle {
	return convertCRectangle(C.readWindowFrame(nativeWindow))
}

// PlaceWindow sets the NSWindow's frame for the request; it must run on the main thread.
func PlaceWindow(nativeWindow unsafe.Pointer, request Request) bool {
	if nativeWindow == nil {
		return false
	}
	frame, isFound := CalculateWindowFrame(ReadAllDisplay(), ReadPointerLocation(), ReadWindowFrame(nativeWindow), request)
	if !isFound {
		return false
	}
	C.setWindowFrame(nativeWindow, C.PlacementRectangle{x: C.double(frame.X), y: C.double(frame.Y), width: C.double(frame.Width), height: C.double(frame.Height)})
	return true
}

func convertCRectangle(rectangle C.PlacementRectangle) Rectangle {
	return Rectangle{X: float64(rectangle.x), Y: float64(rectangle.y), Width: float64(rectangle.width), Height: float64(rectangle.height)}
}
