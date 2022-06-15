package x11

import (
	"errors"

	"github.com/jezek/xgb/xproto"
)

var ErrConnectionDied = errors.New("connection died")

const (
	KeyBackspace xproto.Keycode = 22
	KeyEnter     xproto.Keycode = 104
	KeyEscape    xproto.Keycode = 9
	KeyLeft      xproto.Keycode = 113
	KeyMinus     xproto.Keycode = 20
	KeyRight     xproto.Keycode = 114
	KeyTab       xproto.Keycode = 23
	KeyA         xproto.Keycode = 38
	KeyB         xproto.Keycode = 56
	KeyC         xproto.Keycode = 54
	KeyD         xproto.Keycode = 40
	KeyE         xproto.Keycode = 26
	KeyF         xproto.Keycode = 41
	KeyG         xproto.Keycode = 42
	KeyH         xproto.Keycode = 43
	KeyI         xproto.Keycode = 31
	KeyJ         xproto.Keycode = 44
	KeyK         xproto.Keycode = 45
	KeyL         xproto.Keycode = 46
	KeyM         xproto.Keycode = 58
	KeyN         xproto.Keycode = 57
	KeyO         xproto.Keycode = 32
	KeyP         xproto.Keycode = 33
	KeyQ         xproto.Keycode = 24
	KeyR         xproto.Keycode = 27
	KeyS         xproto.Keycode = 39
	KeyT         xproto.Keycode = 28
	KeyU         xproto.Keycode = 30
	KeyV         xproto.Keycode = 55
	KeyW         xproto.Keycode = 25
	KeyX         xproto.Keycode = 53
	KeyY         xproto.Keycode = 29
	KeyZ         xproto.Keycode = 52
	Key1         xproto.Keycode = 10
	Key2         xproto.Keycode = 11
	Key3         xproto.Keycode = 12
	Key4         xproto.Keycode = 13
	Key5         xproto.Keycode = 14
	Key6         xproto.Keycode = 15
	Key7         xproto.Keycode = 16
	Key8         xproto.Keycode = 17
	Key9         xproto.Keycode = 18
	Key0         xproto.Keycode = 19
	KeyF1        xproto.Keycode = 67
	KeyF2        xproto.Keycode = 68
	KeyF3        xproto.Keycode = 69
	KeyF4        xproto.Keycode = 70
	KeyF5        xproto.Keycode = 71
	KeyF6        xproto.Keycode = 72
	KeyF7        xproto.Keycode = 73
	KeyF8        xproto.Keycode = 74
	KeyF9        xproto.Keycode = 75
	KeyF10       xproto.Keycode = 76
	KeyF11       xproto.Keycode = 95
	KeyF12       xproto.Keycode = 96
	KeyAlt       xproto.Keycode = 64
	KeyCtrl      xproto.Keycode = 37
	KeyShift     xproto.Keycode = 50
)

const (
	ModShift Keymod = 1 << 0
	ModLock  Keymod = 1 << 1
	ModCtrl  Keymod = 1 << 2
	Mod1     Keymod = 1 << 3
	Mod2     Keymod = 1 << 4
	Mod3     Keymod = 1 << 5
	Mod4     Keymod = 1 << 6
	Mod5     Keymod = 1 << 7
	ModNone  Keymod = 0
)

const (
	KeyUp    KeyState = 0
	KeyDown  KeyState = 1
	KeyPress KeyState = 2
)

var mods = map[string]Keymod{
	"ctrl":    ModCtrl,
	"control": ModCtrl,
	"shift":   ModShift,
	"alt":     Mod1,
	"mod1":    Mod1,
	"mod2":    Mod2,
	"mod3":    Mod3,
	"mod4":    Mod4,
	"mod5":    Mod5,
	"modlock": ModLock,
}

var keys = map[string]xproto.Keycode{
	"a":   KeyA,
	"b":   KeyB,
	"c":   KeyC,
	"d":   KeyD,
	"e":   KeyE,
	"f":   KeyF,
	"g":   KeyG,
	"h":   KeyH,
	"i":   KeyI,
	"j":   KeyJ,
	"k":   KeyK,
	"l":   KeyL,
	"m":   KeyM,
	"n":   KeyN,
	"o":   KeyO,
	"p":   KeyP,
	"q":   KeyQ,
	"r":   KeyR,
	"s":   KeyS,
	"t":   KeyT,
	"u":   KeyU,
	"v":   KeyV,
	"w":   KeyW,
	"x":   KeyX,
	"y":   KeyY,
	"z":   KeyZ,
	"0":   Key0,
	"1":   Key1,
	"2":   Key2,
	"3":   Key3,
	"4":   Key4,
	"5":   Key5,
	"6":   Key6,
	"7":   Key7,
	"8":   Key8,
	"9":   Key9,
	"F1":  KeyF1,
	"F2":  KeyF2,
	"F3":  KeyF3,
	"F4":  KeyF4,
	"F5":  KeyF5,
	"F6":  KeyF6,
	"F7":  KeyF7,
	"F8":  KeyF8,
	"F9":  KeyF9,
	"F10": KeyF10,
	"F11": KeyF11,
	"F12": KeyF12,
}
