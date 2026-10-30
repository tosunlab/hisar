package axfill

import "slices"

type rect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// buttonRole is a button's role in a promptControl: the macOS role, which the Linux driver gives AT-SPI's push buttons.
const buttonRole = "AXButton"

// promptControl is a control of a browser prompt as its accessibility tree describes it. Classes are the Views class
// names Chromium reports for the control, and AncestorClasses those of the views around it, nearest first; both are
// the same in every language.
type promptControl struct {
	ID              uint64   `json:"id"`
	Role            string   `json:"role"`
	Classes         []string `json:"classes"`
	AncestorClasses []string `json:"ancestorClasses"`
	Frame           rect     `json:"frame"`
}

// closeCorner is how far from the prompt's top and right edges its close control may lie; Brave and Chrome put it 20
// points in, Edge 16.
const closeCorner = 48

// findCloseControl is a Chromium prompt's close control: its only image button, inside the bubble's frame view and in
// its top-right corner. Brave and Chrome hold it in the frame view itself and Edge in the header of its password
// bubble; Save, Update, Never, No Thanks and Not now are text or label buttons, the eye is a toggle and Edge's menus
// are arrow and menu buttons, so none of them is it. A prompt with two image buttons has no close control.
func findCloseControl(prompt rect, controls []promptControl) (promptControl, bool) {
	var found []promptControl
	for _, control := range controls {
		if control.Role == buttonRole && slices.Equal(control.Classes, []string{"ImageButton"}) {
			found = append(found, control)
		}
	}
	if len(found) != 1 || !slices.Contains(found[0].AncestorClasses, "BubbleFrameView") || !isInTopRightCorner(prompt, found[0].Frame) {
		return promptControl{}, false
	}
	return found[0], true
}

func isInTopRightCorner(prompt, control rect) bool {
	fromRight := (prompt.X + prompt.Width) - (control.X + control.Width)
	fromTop := control.Y - prompt.Y
	return fromRight >= 0 && fromRight <= closeCorner && fromTop >= 0 && fromTop <= closeCorner
}
