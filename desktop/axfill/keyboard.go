package axfill

import (
	"errors"
	"unicode"
)

// The X keysyms the Linux driver types besides text (X11/keysymdef.h).
const (
	keysymBackSpace = 0xff08
	keysymReturn    = 0xff0d
	keysymShiftL    = 0xffe1
	keysymControlL  = 0xffe3
	keysymA         = 0x0061
)

// errUntypable is a character no key of the keyboard's current layout types, without AltGr or a dead key; Hisar types
// nothing of a value that has one.
var errUntypable = errors.New("axfill: the keyboard layout has no key for a character of the value")

// keyboardMap is the X server's keyboard mapping: keysymsPerKeycode keysyms for each keycode from minimumKeycode.
type keyboardMap struct {
	minimumKeycode    byte
	keysymsPerKeycode int
	keysyms           []uint32
}

// keystroke is one key Hisar presses, with Shift held around it when isShifted.
type keystroke struct {
	keycode   byte
	isShifted bool
}

// find is the keystroke that types keysym: the first keycode with it at level 1 (no modifier) or level 2 (Shift).
func (keyboard keyboardMap) find(keysym uint32) (keystroke, bool) {
	if keyboard.keysymsPerKeycode <= 0 {
		return keystroke{}, false
	}
	count := len(keyboard.keysyms) / keyboard.keysymsPerKeycode
	for level := 0; level < 2 && level < keyboard.keysymsPerKeycode; level++ {
		for index := range count {
			if keyboard.keysyms[index*keyboard.keysymsPerKeycode+level] == keysym {
				return keystroke{keycode: keyboard.minimumKeycode + byte(index), isShifted: level == 1}, true
			}
		}
	}
	return keystroke{}, false
}

// keysymOf is the X keysym of a character: Latin-1 characters are their own keysym, others 0x01000000 plus the code
// point.
func keysymOf(character rune) uint32 {
	if (character >= 0x20 && character <= 0x7e) || (character >= 0xa0 && character <= 0xff) {
		return uint32(character)
	}
	return 0x01000000 | uint32(character)
}

// plan is the keystrokes that type text with the current layout. With Caps Lock on, X types a letter's other case, so
// a letter key gets the opposite Shift. Any character the layout has no key for fails the whole plan.
func (keyboard keyboardMap) plan(text string, isCapsLock bool) ([]keystroke, error) {
	strokes := make([]keystroke, 0, len(text))
	for _, character := range text {
		stroke, isFound := keyboard.find(keysymOf(character))
		if !isFound {
			return nil, errUntypable
		}
		if isCapsLock && unicode.IsLetter(character) && unicode.ToUpper(character) != unicode.ToLower(character) {
			stroke.isShifted = !stroke.isShifted
		}
		strokes = append(strokes, stroke)
	}
	return strokes, nil
}
