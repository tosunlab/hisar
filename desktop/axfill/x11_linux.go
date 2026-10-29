package axfill

import (
	"errors"
	"sync"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"github.com/jezek/xgb/xtest"
)

// The X modifier masks the driver checks: none may be held while it types, or a key would become a shortcut.
const (
	maskShift   = xproto.ModMaskShift
	maskLock    = xproto.ModMaskLock
	maskControl = xproto.ModMaskControl
	// maskOthers are Alt, Num Lock's usual Mod2 aside, Super and AltGr.
	maskOthers = xproto.ModMask1 | xproto.ModMask3 | xproto.ModMask4 | xproto.ModMask5
)

var (
	errNoDisplay  = errors.New("axfill: no X11 display")
	errKeyHeld    = errors.New("axfill: a modifier key is held")
	errNoXTest    = errors.New("axfill: the X server has no XTest extension")
	errBadWindow  = errors.New("axfill: the X window is gone")
	errNotPressed = errors.New("axfill: the X server did not take the key")
)

// display is the X11 connection the driver checks windows and types through; it reconnects after the server closed it.
type display struct {
	mutex      sync.Mutex
	connection *xgb.Conn
	root       xproto.Window
	atoms      map[string]xproto.Atom
	keyboard   keyboardMap
}

func (display *display) connect() (*xgb.Conn, error) {
	if display.connection != nil {
		return display.connection, nil
	}
	connection, err := xgb.NewConn()
	if err != nil {
		return nil, errNoDisplay
	}
	if err := xtest.Init(connection); err != nil {
		connection.Close()
		return nil, errNoXTest
	}
	display.connection, display.root = connection, xproto.Setup(connection).DefaultScreen(connection).Root
	display.atoms = map[string]xproto.Atom{}
	return connection, nil
}

// do runs work on the connection under the display's lock, dropping a connection the server closed.
func (display *display) do(work func(connection *xgb.Conn) error) error {
	display.mutex.Lock()
	defer display.mutex.Unlock()
	connection, err := display.connect()
	if err != nil {
		return err
	}
	err = work(connection)
	var protocolError xgb.Error
	if err != nil && !errors.As(err, &protocolError) && !errors.Is(err, errBadWindow) {
		// Not an X error reply: the connection itself failed, so the next call connects again.
		connection.Close()
		display.connection = nil
	}
	return err
}

func (display *display) atom(connection *xgb.Conn, name string) (xproto.Atom, error) {
	if atom, isKnown := display.atoms[name]; isKnown {
		return atom, nil
	}
	reply, err := xproto.InternAtom(connection, true, uint16(len(name)), name).Reply()
	if err != nil {
		return 0, err
	}
	display.atoms[name] = reply.Atom
	return reply.Atom, nil
}

func (display *display) property(connection *xgb.Conn, window xproto.Window, name string) (*xproto.GetPropertyReply, error) {
	atom, err := display.atom(connection, name)
	if err != nil || atom == xproto.AtomNone {
		return nil, err
	}
	reply, err := xproto.GetProperty(connection, false, window, atom, xproto.GetPropertyTypeAny, 0, 1024).Reply()
	var badWindow xproto.WindowError
	if errors.As(err, &badWindow) {
		return nil, errBadWindow
	}
	return reply, err
}

func readCardinal(reply *xproto.GetPropertyReply) (uint32, bool) {
	if reply == nil || reply.Format != 32 || len(reply.Value) < 4 {
		return 0, false
	}
	return xgb.Get32(reply.Value), true
}

// windowState is what X11 says about the target window.
type windowState struct {
	exists    bool
	pid       int
	isActive  bool
	isMapped  bool
	isHidden  bool
	isCapsOn  bool
	heldMasks uint16
	// title is _NET_WM_NAME, or WM_NAME for a window without it.
	title string
}

// readWindow reads the target window: its _NET_WM_PID, whether it is _NET_ACTIVE_WINDOW and viewable and not hidden,
// and the modifier keys held now.
func (display *display) readWindow(window uint32) (windowState, error) {
	var state windowState
	err := display.do(func(connection *xgb.Conn) error {
		target := xproto.Window(window)
		attributes, err := xproto.GetWindowAttributes(connection, target).Reply()
		if err != nil {
			var badWindow xproto.WindowError
			if errors.As(err, &badWindow) {
				return nil
			}
			return err
		}
		state.exists = true
		state.isMapped = attributes.MapState == xproto.MapStateViewable
		pid, err := display.property(connection, target, "_NET_WM_PID")
		if err != nil {
			return err
		}
		if value, isValue := readCardinal(pid); isValue {
			state.pid = int(value)
		}
		active, err := display.property(connection, display.root, "_NET_ACTIVE_WINDOW")
		if err != nil {
			return err
		}
		if value, isValue := readCardinal(active); isValue {
			state.isActive = value == window
		}
		for _, name := range []string{"_NET_WM_NAME", "WM_NAME"} {
			title, err := display.property(connection, target, name)
			if err != nil {
				return err
			}
			if title != nil && title.Format == 8 && len(title.Value) > 0 {
				state.title = string(title.Value)
				break
			}
		}
		wmState, err := display.property(connection, target, "_NET_WM_STATE")
		if err != nil {
			return err
		}
		hidden, err := display.atom(connection, "_NET_WM_STATE_HIDDEN")
		if err != nil {
			return err
		}
		if wmState != nil && wmState.Format == 32 {
			for offset := 0; offset+4 <= len(wmState.Value); offset += 4 {
				if xproto.Atom(xgb.Get32(wmState.Value[offset:])) == hidden {
					state.isHidden = true
				}
			}
		}
		pointer, err := xproto.QueryPointer(connection, display.root).Reply()
		if err != nil {
			return err
		}
		state.heldMasks = pointer.Mask
		state.isCapsOn = pointer.Mask&maskLock != 0
		return nil
	})
	return state, err
}

// readKeyboard reads the keyboard mapping again, since the user may switch layouts between sign-ins.
func (display *display) readKeyboard() (keyboardMap, error) {
	var keyboard keyboardMap
	err := display.do(func(connection *xgb.Conn) error {
		setup := xproto.Setup(connection)
		count := byte(setup.MaxKeycode - setup.MinKeycode + 1)
		reply, err := xproto.GetKeyboardMapping(connection, setup.MinKeycode, count).Reply()
		if err != nil {
			return err
		}
		keysyms := make([]uint32, len(reply.Keysyms))
		for index, keysym := range reply.Keysyms {
			keysyms[index] = uint32(keysym)
		}
		keyboard = keyboardMap{minimumKeycode: byte(setup.MinKeycode), keysymsPerKeycode: int(reply.KeysymsPerKeycode), keysyms: keysyms}
		return nil
	})
	return keyboard, err
}

// press types one key through XTest, with Control or Shift held around it, and waits until the server has handled it,
// so the next check sees what it did.
func (display *display) press(stroke keystroke, control keystroke, shift keystroke, isControlled bool) error {
	return display.do(func(connection *xgb.Conn) error {
		send := func(kind byte, keycode byte) error {
			return xtest.FakeInputChecked(connection, kind, keycode, 0, display.root, 0, 0, 0).Check()
		}
		var modifiers []byte
		if isControlled {
			modifiers = append(modifiers, control.keycode)
		}
		if stroke.isShifted {
			modifiers = append(modifiers, shift.keycode)
		}
		for _, modifier := range modifiers {
			if err := send(xproto.KeyPress, modifier); err != nil {
				return err
			}
		}
		pressError := send(xproto.KeyPress, stroke.keycode)
		releaseError := send(xproto.KeyRelease, stroke.keycode)
		for index := len(modifiers) - 1; index >= 0; index-- {
			if err := send(xproto.KeyRelease, modifiers[index]); err != nil && releaseError == nil {
				releaseError = err
			}
		}
		if _, err := xproto.GetInputFocus(connection).Reply(); err != nil {
			return err
		}
		return errors.Join(pressError, releaseError)
	})
}
