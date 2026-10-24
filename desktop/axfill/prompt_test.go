package axfill

import "testing"

type measuredPrompt struct {
	frame    rect
	close    promptControl
	controls []promptControl
}

var (
	braveFrame  = []string{"BubbleFrameView", "NonClientView", "RootView"}
	braveField  = append([]string{"ControlElementsContainer", "EditablePasswordCombobox", "View", "View", "PasswordSaveUpdateView", "DialogClientView"}, braveFrame...)
	braveRow    = append([]string{"DialogClientView::ButtonRowContainer", "DialogClientView"}, braveFrame...)
	braveClose  = promptControl{ID: 1, Role: "AXButton", Classes: []string{"ImageButton"}, AncestorClasses: braveFrame, Frame: rect{1392, 146, 24, 22}}
	edgeDialog  = []string{"PasswordBubbleViewBase", "DialogClientView", "BubbleFrameView", "NonClientView", "RootView"}
	edgeField   = append([]string{"ControlElementsContainer", "EditableCombobox", "View", "View", "View"}, edgeDialog...)
	edgeButtons = append([]string{"View"}, edgeDialog...)
	edgeClose   = promptControl{ID: 11, Role: "AXButton", Classes: []string{"ImageButton"}, AncestorClasses: append([]string{"View", "View"}, edgeDialog...), Frame: rect{991, 146, 24, 24}}
)

// measuredPrompts are the save-password prompts Brave 1.98 and Edge 156 showed after the fixture's plain and return
// forms were submitted, measured on 2026-10-23, with every button and checkbox in them.
var measuredPrompts = map[string]measuredPrompt{
	"Brave": {frame: rect{988, 126, 448, 370}, close: braveClose, controls: []promptControl{
		braveClose,
		{ID: 2, Role: "AXCheckBox", Classes: []string{"ToggleImageButton"}, AncestorClasses: braveField, Frame: rect{1386, 352, 24, 24}},
		{ID: 3, Role: "AXButton", Classes: []string{"MdTextButton"}, AncestorClasses: braveRow, Frame: rect{1008, 398, 99, 32}},
		{ID: 4, Role: "AXButton", Classes: []string{"MdTextButton"}, AncestorClasses: braveRow, Frame: rect{1210, 398, 99, 32}},
		{ID: 5, Role: "AXButton", Classes: []string{"MdTextButton"}, AncestorClasses: braveRow, Frame: rect{1317, 398, 99, 32}},
	}},
	"Edge": {frame: rect{711, 130, 320, 280}, close: edgeClose, controls: []promptControl{
		edgeClose,
		{ID: 12, Role: "AXButton", Classes: []string{"Arrow"}, AncestorClasses: edgeField, Frame: rect{987, 254, 24, 24}},
		{ID: 13, Role: "AXCheckBox", Classes: []string{"ToggleImageButton"}, AncestorClasses: edgeField, Frame: rect{987, 318, 24, 24}},
		{ID: 14, Role: "AXButton", Classes: []string{"MdTextButton"}, AncestorClasses: edgeButtons, Frame: rect{855, 362, 52, 32}},
		{ID: 15, Role: "AXButton", Classes: []string{"LabelButton"}, AncestorClasses: append([]string{"View"}, edgeButtons...), Frame: rect{915, 362, 75, 32}},
		{ID: 16, Role: "AXPopUpButton", Classes: []string{"MenuButton"}, AncestorClasses: append([]string{"View"}, edgeButtons...), Frame: rect{991, 362, 24, 32}},
	}},
}

func TestTheCloseControlIsTheBubblesImageButton(t *testing.T) {
	for name, prompt := range measuredPrompts {
		if found, isFound := findCloseControl(prompt.frame, prompt.controls); !isFound || found.ID != prompt.close.ID {
			t.Errorf("%s: found %+v, %v, want the close button", name, found, isFound)
		}
	}
}

func TestNoOtherControlIsTakenForTheCloseControl(t *testing.T) {
	for name, prompt := range measuredPrompts {
		others := []promptControl{}
		for _, control := range prompt.controls {
			if control.ID != prompt.close.ID {
				others = append(others, control)
				if found, isFound := findCloseControl(prompt.frame, []promptControl{control}); isFound {
					t.Errorf("%s: took %+v for the close control", name, found)
				}
			}
		}
		if found, isFound := findCloseControl(prompt.frame, others); isFound {
			t.Errorf("%s without its close button: took %+v", name, found)
		}
	}
}

func TestAnUnclearCloseControlIsNone(t *testing.T) {
	prompt := measuredPrompts["Brave"]
	second := braveClose
	second.ID = 7
	second.Frame.X -= 30
	outsideTheBubble := braveClose
	outsideTheBubble.AncestorClasses = []string{"View", "RootView"}
	atTheBottom := braveClose
	atTheBottom.Frame.Y = 450
	pastTheEdge := braveClose
	pastTheEdge.Frame.X = 1430
	for name, controls := range map[string][]promptControl{
		"two image buttons":                  {braveClose, second},
		"an image button outside the bubble": {outsideTheBubble},
		"an image button at the bottom":      {atTheBottom},
		"an image button past the edge":      {pastTheEdge},
		"no controls":                        nil,
	} {
		if found, isFound := findCloseControl(prompt.frame, controls); isFound {
			t.Errorf("%s: took %+v for the close control", name, found)
		}
	}
}
