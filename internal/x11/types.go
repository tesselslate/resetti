package x11

import (
	"strconv"
	"strings"

	"github.com/jezek/xgb/xproto"
	"github.com/pkg/errors"
)

// Key modifiers
const (
	ModShift Keymod = 1 << 0
	ModLock         = 1 << 1
	ModCtrl         = 1 << 2
	Mod1            = 1 << 3
	Mod2            = 1 << 4
	Mod3            = 1 << 5
	Mod4            = 1 << 6
	Mod5            = 1 << 7
	ModNone         = 0
)

// Key/button states
const (
	StateDown InputState = iota
	StateUp
)

// Important key codes
const (
	KeyEsc = 9
	KeyF1  = 67
	KeyF3  = 69
	KeyF6  = 72
	KeyH   = 43
)

// InputState represents the state of a button or key (up or down.)
type InputState int

// Key represents the key in a keybinding or keypress event.
type Key struct {
	Code xproto.Keycode
	Mod  Keymod
}

// Keymod represents a key modifier.
type Keymod uint16

// Point represents a point on the X screen.
type Point struct {
	X int16
	Y int16
}

// Event represents a single user input, such as a button click or window focus
// change.
type Event interface {
	// The time the Event occurred at, in X server time (milliseconds with an
	// arbitrary start point.)
	Time() uint32
}

// ButtonEvent represents a single mouse button press.
type ButtonEvent struct {
	Button    xproto.Button
	Mod       Keymod
	Point     Point
	Timestamp uint32
	Window    xproto.Window
}

// FocusEvent represents a window focus change.
type FocusEvent struct {
	Timestamp uint32
	Window    xproto.Window
}

// KeyEvent represents a single key press or release.
type KeyEvent struct {
	Key       Key
	State     InputState
	Timestamp uint32
}

// MoveEvent represents a single mouse movement.
type MoveEvent struct {
	Mod       Keymod
	Point     Point
	Timestamp uint32
	Window    xproto.Window
}

func (e ButtonEvent) Time() uint32 {
	return e.Timestamp
}

func (e FocusEvent) Time() uint32 {
	return e.Timestamp
}

func (e KeyEvent) Time() uint32 {
	return e.Timestamp
}

func (e MoveEvent) Time() uint32 {
	return e.Timestamp
}

func (k *Key) UnmarshalTOML(value any) error {
	str, ok := value.(string)
	if !ok {
		return errors.New("value not a string")
	}
	components := strings.Split(str, "-")
	for _, component := range components {
		component = strings.ToLower(component)
		if key, ok := keycodesToml[component]; ok {
			k.Code = key
		} else if mod, ok := keymods[component]; ok {
			k.Mod |= mod
		} else if strings.HasPrefix(component, "code") {
			code, err := strconv.Atoi(component[4:])
			if err != nil {
				return errors.Wrap(err, "convert key code")
			}
			if code > 255 || code < 0 {
				return errors.New("key code out of bounds (0 <= N <= 255)")
			}
			k.Code = xproto.Keycode(code)
		} else {
			return errors.Errorf("invalid key component %s", component)
		}
	}
	return nil
}

func (m *Keymod) UnmarshalTOML(value any) error {
	str, ok := value.(string)
	if !ok {
		return errors.New("value not a string")
	}
	if str == "" {
		return nil
	}
	components := strings.Split(str, "-")
	for _, component := range components {
		component = strings.ToLower(component)
		if mod, ok := keymods[component]; ok {
			*m |= mod
		} else {
			return errors.Errorf("invalid key component %s", component)
		}
	}
	return nil
}
