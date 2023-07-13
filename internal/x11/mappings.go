package x11

import "github.com/jezek/xgb/xproto"

// Buttons is a list of buttons used for config parsing.
var Buttons = map[string]xproto.Button{
	"lmb":         xproto.ButtonIndex1,
	"leftclick":   xproto.ButtonIndex1,
	"leftmouse":   xproto.ButtonIndex1,
	"mouse1":      xproto.ButtonIndex1,
	"m1":          xproto.ButtonIndex1,
	"mmb":         xproto.ButtonIndex2,
	"middleclick": xproto.ButtonIndex2,
	"middlemouse": xproto.ButtonIndex2,
	"mouse2":      xproto.ButtonIndex2,
	"m2":          xproto.ButtonIndex2,
	"rmb":         xproto.ButtonIndex3,
	"rightclick":  xproto.ButtonIndex3,
	"rightmouse":  xproto.ButtonIndex3,
	"mouse3":      xproto.ButtonIndex3,
	"m3":          xproto.ButtonIndex3,
	"mouse4":      xproto.ButtonIndex4,
	"m4":          xproto.ButtonIndex4,
	"mouse5":      xproto.ButtonIndex5,
	"m5":          xproto.ButtonIndex5,
}

// Keycodes is a list of keycodes used for config parsing.
var Keycodes = map[string]xproto.Keycode{
	// Keys
	"0":            19,
	"1":            10,
	"2":            11,
	"3":            12,
	"4":            13,
	"5":            14,
	"6":            15,
	"7":            16,
	"8":            17,
	"9":            18,
	"a":            38,
	"b":            56,
	"c":            54,
	"d":            40,
	"e":            26,
	"f":            41,
	"g":            42,
	"h":            43,
	"i":            31,
	"j":            44,
	"k":            45,
	"l":            46,
	"m":            58,
	"n":            57,
	"o":            32,
	"p":            33,
	"q":            24,
	"r":            27,
	"s":            39,
	"t":            28,
	"u":            30,
	"v":            55,
	"w":            25,
	"x":            53,
	"y":            29,
	"z":            52,
	"f1":           67,
	"f2":           68,
	"f3":           69,
	"f4":           70,
	"f5":           71,
	"f6":           72,
	"f7":           73,
	"f8":           74,
	"f9":           75,
	"f10":          76,
	"f11":          95,
	"f12":          96,
	"down":         116,
	"left":         113,
	"right":        114,
	"up":           111,
	"apostrophe":   48,
	"grave": 		49,
	"backslash":    51,
	"comma":        59,
	"equal":        21,
	"minus":        20,
	"period":       60,
	"semicolon":    47,
	"slash":        61,
	"space":        65,
	"tab":          23,
	"enter":        36,
	"return":       36,
	"escape":       9,
	"esc":          9,
	"backspace":    22,
	"delete":       119,
	"del":          119,
	"end":          115,
	"home":         110,
	"insert":       118,
	"ins":          118,
	"pause":        127,
	"menu":         135,
	"print.screen": 107,
	"printscreen":  107,
}

// KeycodesMc is a list of keycodes used for parsing Minecraft options.
var KeycodesMc = map[string]xproto.Keycode{
	"0":               19,
	"1":               10,
	"2":               11,
	"3":               12,
	"4":               13,
	"5":               14,
	"6":               15,
	"7":               16,
	"8":               17,
	"9":               18,
	"a":               38,
	"b":               56,
	"c":               54,
	"d":               40,
	"e":               26,
	"f":               41,
	"g":               42,
	"h":               43,
	"i":               31,
	"j":               44,
	"k":               45,
	"l":               46,
	"m":               58,
	"n":               57,
	"o":               32,
	"p":               33,
	"q":               24,
	"r":               27,
	"s":               39,
	"t":               28,
	"u":               30,
	"v":               55,
	"w":               25,
	"x":               53,
	"y":               29,
	"z":               52,
	"f1":              67,
	"f2":              68,
	"f3":              69,
	"f4":              70,
	"f5":              71,
	"f6":              72,
	"f7":              73,
	"f8":              74,
	"f9":              75,
	"f10":             76,
	"f11":             95,
	"f12":             96,
	"keypad.0":        90,
	"keypad.1":        87,
	"keypad.2":        88,
	"keypad.3":        89,
	"keypad.4":        83,
	"keypad.5":        84,
	"keypad.6":        85,
	"keypad.7":        79,
	"keypad.8":        80,
	"keypad.9":        81,
	"keypad.add":      86,
	"keypad.decimal":  91,
	"keypad.enter":    104,
	"keypad.equal":    125,
	"keypad.multiply": 63,
	"keypad.divide":   106,
	"keypad.subtract": 82,
	"down":            116,
	"left":            113,
	"right":           114,
	"up":              111,
	"apostrophe":      48,
	"accent.grave":	   49,
	"backslash":       51,
	"comma":           59,
	"equal":           21,
	"left.bracket":    34,
	"minus":           20,
	"period":          60,
	"right.bracket":   35,
	"semicolon":       47,
	"slash":           61,
	"space":           65,
	"tab":             23,
	"enter":           36,
	"escape":          9,
	"backspace":       22,
	"delete":          119,
	"end":             115,
	"home":            110,
	"insert":          118,
	"pause":           127,
	"menu":            135,
	"print.screen":    107,
}

// Keycodes is a list of modifier keycodes used for config parsing.
var Modifiers = map[string]xproto.Keycode{
	"ctrl":     37,
	"control":  37,
	"lctrl":    37,
	"lcontrol": 37,
	"shift":    50,
	"lshift":   50,
	"rshift":   62,
	"alt":      64,
	"lalt":     64,
	"rctrl":    105,
	"rcontrol": 105,
}
