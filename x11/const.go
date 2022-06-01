package x11

import (
	"errors"

	"github.com/jezek/xgb/xproto"
)

var ErrConnectionDied = errors.New("connection died")

const (
	KeyBackslash xproto.Keycode = 51
	KeyEnter     xproto.Keycode = 104
	KeyEscape    xproto.Keycode = 9
	KeyF         xproto.Keycode = 41
	KeyF3        xproto.Keycode = 69
	KeyF11       xproto.Keycode = 95
	KeyH         xproto.Keycode = 43
	KeyLeft      xproto.Keycode = 113
	KeyRight     xproto.Keycode = 114
	KeyTab       xproto.Keycode = 23
	Key1         xproto.Keycode = 10
	Key2         xproto.Keycode = 11
	Key3         xproto.Keycode = 12
	Key4         xproto.Keycode = 13
	Key5         xproto.Keycode = 14
	Key6         xproto.Keycode = 15
	Key7         xproto.Keycode = 16
	Key8         xproto.Keycode = 17
	Key9         xproto.Keycode = 18

	KeyAlt   xproto.Keycode = 64
	KeyCtrl  xproto.Keycode = 37
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
