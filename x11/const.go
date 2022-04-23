package x11

import "github.com/jezek/xgb/xproto"

const (
	KeyBackslash xproto.Keycode = 51
	KeyEnter     xproto.Keycode = 104
	KeyEscape    xproto.Keycode = 9
	KeyF         xproto.Keycode = 41
	KeyF3        xproto.Keycode = 69
	KeyLeft      xproto.Keycode = 113
	KeyRight     xproto.Keycode = 114
	KeyTab       xproto.Keycode = 23

	KeyShift xproto.Keycode = 50
)

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

const (
	KeyUp    KeyState = 0
	KeyDown  KeyState = 1
	KeyPress KeyState = 2
)
