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
	stroke, _, isFound := keyboard.findWithOther(keysym)
	return stroke, isFound
}

// findWithOther is find, with the keysym at the key's other level.
func (keyboard keyboardMap) findWithOther(keysym uint32) (keystroke, uint32, bool) {
	if keyboard.keysymsPerKeycode <= 0 {
		return keystroke{}, 0, false
	}
	count := len(keyboard.keysyms) / keyboard.keysymsPerKeycode
	for level := 0; level < 2 && level < keyboard.keysymsPerKeycode; level++ {
		for index := range count {
			if keyboard.keysyms[index*keyboard.keysymsPerKeycode+level] != keysym {
				continue
			}
			other := uint32(0)
			if keyboard.keysymsPerKeycode > 1 {
				other = keyboard.keysyms[index*keyboard.keysymsPerKeycode+1-level]
			}
			return keystroke{keycode: keyboard.minimumKeycode + byte(index), isShifted: level == 1}, other, true
		}
	}
	return keystroke{}, 0, false
}

// keysymOf is the X keysym of a character: Latin-1 characters are their own keysym, others 0x01000000 plus the code
// point.
func keysymOf(character rune) uint32 {
	if (character >= 0x20 && character <= 0x7e) || (character >= 0xa0 && character <= 0xff) {
		return uint32(character)
	}
	return 0x01000000 | uint32(character)
}

// otherCase is a letter's other case, or the letter itself when it has one case only.
func otherCase(character rune) rune {
	if upper := unicode.ToUpper(character); upper != character {
		return upper
	}
	return unicode.ToLower(character)
}

// plan is the keystrokes that type text with the current layout. Caps Lock makes X type a key's other level when its
// two levels are a letter's two cases, so such a key gets the opposite Shift; a key whose other level is something
// else, as ß and ? on a German keyboard, keeps its Shift. Any character the layout has no key for fails the whole plan.
func (keyboard keyboardMap) plan(text string, isCapsLock bool) ([]keystroke, error) {
	strokes := make([]keystroke, 0, len(text))
	for _, character := range text {
		stroke, other, isFound := keyboard.findWithOther(keysymOf(character))
		if !isFound {
			return nil, errUntypable
		}
		if isCapsLock && otherCase(character) != character && other == keysymOf(otherCase(character)) {
			stroke.isShifted = !stroke.isShifted
		}
		strokes = append(strokes, stroke)
	}
	return strokes, nil
}
