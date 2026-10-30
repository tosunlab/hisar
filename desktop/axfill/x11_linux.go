package axfill

import (
	"errors"
	"sync"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/res"
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
	errNoDisplay = errors.New("axfill: no X11 display")
	errKeyHeld   = errors.New("axfill: a modifier key is held")
	errNoXTest   = errors.New("axfill: the X server has no XTest extension")
	errNoXRes    = errors.New("axfill: the X server has no X-Resource extension")
	// errFocusMoved: the X input focus is not in the target window, or belongs to another process, so a key would
	// reach another client.
	errFocusMoved = errors.New("axfill: the keyboard focus left the target window")
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
	// X-Resource reports the process behind a window as the server knows it; _NET_WM_PID is what the client says.
	if err := res.Init(connection); err != nil {
		connection.Close()
		return nil, errNoXRes
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
	if err != nil && !errors.As(err, &protocolError) && !errors.Is(err, errBadWindow) && !errors.Is(err, errFocusMoved) {
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
	exists bool
	// pid is the process of the client that created the window, as the X server knows it (X-Resource), or 0 for a
	// client that is not local.
	pid       int
	isActive  bool
	isMapped  bool
	isHidden  bool
	isCapsOn  bool
	heldMasks uint16
	// title is _NET_WM_NAME, or WM_NAME for a window without it.
	title string
	// extents are the window's position on the screen and its size.
	extents rect
}

// readOwnerPID is the process of the local client that created window, as the X server knows it; 0 when the server
// does not know one.
func readOwnerPID(connection *xgb.Conn, window xproto.Window) (int, error) {
	reply, err := res.QueryClientIds(connection, 1, []res.ClientIdSpec{{Client: uint32(window), Mask: res.ClientIdMaskLocalClientPID}}).Reply()
	if err != nil {
		return 0, err
	}
	for _, id := range reply.Ids {
		if id.Spec.Mask == res.ClientIdMaskLocalClientPID && len(id.Value) == 1 {
			return int(id.Value[0]), nil
		}
	}
	return 0, nil
}

// readWindow reads the target window: the process that created it, whether it is _NET_ACTIVE_WINDOW and viewable and
// not hidden, its title and extents, and the modifier keys held now.
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
		if state.pid, err = readOwnerPID(connection, target); err != nil {
			return err
		}
		geometry, err := xproto.GetGeometry(connection, xproto.Drawable(target)).Reply()
		if err != nil {
			return err
		}
		origin, err := xproto.TranslateCoordinates(connection, target, display.root, 0, 0).Reply()
		if err != nil {
			return err
		}
		state.extents = rect{X: float64(origin.DstX), Y: float64(origin.DstY), Width: float64(geometry.Width), Height: float64(geometry.Height)}
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

// press types one key through XTest, with Control or Shift held around it. XTest keys go to the window with the X
// input focus, so with the server grabbed, which keeps every other client from moving the focus meanwhile, press checks
// that the focus is the target window or inside it and belongs to pid, and only then sends the key. Every modifier it
// pressed is released whatever fails. It waits until the server has handled the key, so the next check sees what it did.
func (display *display) press(stroke, control, shift keystroke, isControlled bool, target uint32, pid int) error {
	return display.do(func(connection *xgb.Conn) error {
		if err := xproto.GrabServerChecked(connection).Check(); err != nil {
			return err
		}
		err := display.pressInFocus(connection, stroke, control, shift, isControlled, xproto.Window(target), pid)
		ungrabError := xproto.UngrabServerChecked(connection).Check()
		if _, syncError := xproto.GetInputFocus(connection).Reply(); syncError != nil && ungrabError == nil {
			ungrabError = syncError
		}
		return errors.Join(err, ungrabError)
	})
}

func (display *display) pressInFocus(connection *xgb.Conn, stroke, control, shift keystroke, isControlled bool, target xproto.Window, pid int) error {
	if err := checkFocus(connection, target, pid); err != nil {
		return err
	}
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
	var pressed []byte
	var err error
	for _, modifier := range modifiers {
		if err = send(xproto.KeyPress, modifier); err != nil {
			break
		}
		pressed = append(pressed, modifier)
	}
	if err == nil {
		err = send(xproto.KeyPress, stroke.keycode)
		err = errors.Join(err, send(xproto.KeyRelease, stroke.keycode))
	}
	for index := len(pressed) - 1; index >= 0; index-- {
		err = errors.Join(err, send(xproto.KeyRelease, pressed[index]))
	}
	return err
}

// maximumWindowDepth bounds the walk from the focus window up to the root.
const maximumWindowDepth = 64

// checkFocus requires the X input focus to be target or a window inside it, created by pid.
func checkFocus(connection *xgb.Conn, target xproto.Window, pid int) error {
	focus, err := xproto.GetInputFocus(connection).Reply()
	if err != nil {
		return err
	}
	// None and PointerRoot name no window: the key would go wherever the pointer is, or nowhere.
	if focus.Focus == xproto.WindowNone || focus.Focus == xproto.InputFocusPointerRoot {
		return errFocusMoved
	}
	if owner, err := readOwnerPID(connection, focus.Focus); err != nil || owner != pid {
		return errFocusMoved
	}
	window := focus.Focus
	for range maximumWindowDepth {
		if window == target {
			return nil
		}
		tree, err := xproto.QueryTree(connection, window).Reply()
		if err != nil || tree.Parent == xproto.WindowNone || window == tree.Root {
			return errFocusMoved
		}
		window = tree.Parent
	}
	return errFocusMoved
}
