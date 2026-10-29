package axfill

import (
	"errors"
	"slices"
	"testing"
)

// usKeyboard is a slice of a US layout: keycode 38 types a and A, 11 types 2 and @, 65 a space.
var usKeyboard = keyboardMap{minimumKeycode: 8, keysymsPerKeycode: 2, keysyms: func() []uint32 {
	keysyms := make([]uint32, 2*(70-8+1))
	set := func(keycode int, plain, shifted uint32) {
		keysyms[2*(keycode-8)], keysyms[2*(keycode-8)+1] = plain, shifted
	}
	set(38, 'a', 'A')
	set(11, '2', '@')
	set(65, ' ', ' ')
	set(50, keysymShiftL, 0)
	return keysyms
}()}

func TestPlanTypesEachCharacterWithItsShift(t *testing.T) {
	strokes, err := usKeyboard.plan("aA@2 ", false)
	expected := []keystroke{{38, false}, {38, true}, {11, true}, {11, false}, {65, false}}
	if err != nil || !slices.Equal(strokes, expected) {
		t.Errorf("%v, %v; want %v", strokes, err, expected)
	}
	if shift, isFound := usKeyboard.find(keysymShiftL); !isFound || shift.keycode != 50 {
		t.Errorf("Shift: %v, %v", shift, isFound)
	}
}

// With Caps Lock on X types a letter key's other case, so letters get the opposite Shift and nothing else changes.
func TestPlanUndoesCapsLockForLettersOnly(t *testing.T) {
	strokes, err := usKeyboard.plan("aA@2", true)
	expected := []keystroke{{38, true}, {38, false}, {11, true}, {11, false}}
	if err != nil || !slices.Equal(strokes, expected) {
		t.Errorf("%v, %v; want %v", strokes, err, expected)
	}
}

// A character the layout has no key for fails the whole plan, so Hisar types none of the value.
func TestPlanRefusesAValueWithAnUntypableCharacter(t *testing.T) {
	for _, value := range []string{"aé", "a€", "a\n"} {
		if strokes, err := usKeyboard.plan(value, false); !errors.Is(err, errUntypable) || strokes != nil {
			t.Errorf("%q: %v, %v", value, strokes, err)
		}
	}
}
