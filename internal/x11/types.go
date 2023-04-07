package x11

import (
	"github.com/jezek/xgb/xproto"
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

// Important keys
var (
	KeyEsc = Key{Code: 9}
	KeyF1  = Key{Code: 67}
	KeyF3  = Key{Code: 69}
	KeyF6  = Key{Code: 72}
	KeyH   = Key{Code: 43}
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
	State     InputState
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

// Time implements Event.
func (e ButtonEvent) Time() uint32 {
	return e.Timestamp
}

// Time implements Event.
func (e FocusEvent) Time() uint32 {
	return e.Timestamp
}

// Time implements Event.
func (e KeyEvent) Time() uint32 {
	return e.Timestamp
}

// Time implements Event.
func (e MoveEvent) Time() uint32 {
	return e.Timestamp
}
