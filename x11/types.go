package x11

import "github.com/jezek/xgb/xproto"

// Attributes contains various window attributes.
type Attributes struct {
	Pid   uint32
	Class []string
}

// Keymod represents modifiers held down for a keypress.
type Keymod uint16

// Key represents the contents of a keypress.
type Key struct {
	Code xproto.Keycode
	Mod  Keymod
}

// KeyEvent represents a single key event.
type KeyEvent struct {
	Key       Key
	State     KeyState
	Timestamp xproto.Timestamp
}

// KeyState represents the state of a keypress.
type KeyState int

// ButtonEvent represents a single mouse click event.
type ButtonEvent struct {
	X         int16
	Y         int16
	State     uint16
	Timestamp xproto.Timestamp
}

// MoveEvent represents a single mouse motion event.
type MoveEvent struct {
	X         int16
	Y         int16
	State     uint16
	Timestamp xproto.Timestamp
}
