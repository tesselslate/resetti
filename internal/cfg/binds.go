package cfg

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/jezek/xgb/xproto"
	"github.com/tesselslate/resetti/internal/x11"
)

// Keybind actions
const (
	ActionIngameFocus int = iota
	ActionIngameReset
	ActionIngameRes
	ActionWallFocus
	ActionWallResetAll
	ActionWallLock
	ActionWallPlay
	ActionWallReset
	ActionWallResetOthers
	ActionWallPlayFirstLocked
)

// Mapping of action names -> action types
var actionNames = map[string]int{
	"ingame_focus":           ActionIngameFocus,
	"ingame_reset":           ActionIngameReset,
	"ingame_toggle_res":      ActionIngameRes,
	"wall_focus":             ActionWallFocus,
	"wall_reset_all":         ActionWallResetAll,
	"wall_lock":              ActionWallLock,
	"wall_play":              ActionWallPlay,
	"wall_reset":             ActionWallReset,
	"wall_reset_others":      ActionWallResetOthers,
	"wall_play_first_locked": ActionWallPlayFirstLocked,
}

// Keybind parsing regexes
var keyRegexp = regexp.MustCompile(`^code(\d+)$`)
var numRegexp = regexp.MustCompile(`\((\d+)\)$`)

// Action represents a single keybind action.
type Action struct {
	// The type of action.
	Type int

	// Extra detail for the action (e.g. instance number.)
	Extra *int
}

// ActionList contains a list of actions to perform when a keybind is pressed.
type ActionList struct {
	IngameActions []Action
	WallActions   []Action
}

// Bind represents a single keybinding.
type Bind struct {
	Button   *xproto.Button    // The button for this keybind (if any.)
	Key      *xproto.Keycode   // The key for this keybind (if any.)
	Mods     [4]xproto.Keycode // The list of key modifiers for this keybind (if any.)
	ModCount int               // The number of modifiers in use.

	// String representation.
	str string
}

// UnmarshalTOML implements toml.Unmarshaler.
func (a *ActionList) UnmarshalTOML(value any) error {
	actionsRaw, ok := value.([]any)
	if !ok {
		return errors.New("action list was not a string array")
	}
	var actions []string
	for _, raw := range actionsRaw {
		str, ok := raw.(string)
		if !ok {
			return errors.New("action list contained non-string value")
		}
		actions = append(actions, str)
	}
	uniqueWall := make(map[Action]bool)
	uniqueGame := make(map[Action]bool)
	for _, actionStr := range actions {
		if typ, ok := actionNames[actionStr]; ok {
			if typ < ActionWallFocus {
				a.IngameActions = append(a.IngameActions, Action{typ, nil})
				uniqueGame[Action{typ, nil}] = true
			} else {
				a.WallActions = append(a.WallActions, Action{typ, nil})
				uniqueWall[Action{typ, nil}] = true
			}
		} else {
			loc := numRegexp.FindStringIndex(actionStr)
			if loc == nil {
				return fmt.Errorf("invalid action %q", actionStr)
			}
			num, err := strconv.Atoi(actionStr[loc[0]+1 : loc[1]-1])
			if err != nil {
				return fmt.Errorf("failed to parse number in %q", actionStr)
			}
			// Subtract 1 for 0-based indexing.
			num -= 1
			typ := actionStr[:loc[0]]
			if typ, ok := actionNames[typ]; ok {
				if typ >= ActionWallLock && typ <= ActionWallResetOthers {
					a.WallActions = append(a.WallActions, Action{typ, &num})
					uniqueWall[Action{typ, &num}] = true
				} else {
					return fmt.Errorf("action %q cannot have number", actionStr)
				}
			} else {
				return fmt.Errorf("invalid action %q", actionStr)
			}
		}
	}
	if len(uniqueWall)+len(uniqueGame) != len(actions) {
		return fmt.Errorf("duplicate action in bind %v", actions)
	}
	return nil
}

// String implements Stringer.
func (b *Bind) String() string {
	return b.str
}

// UnmarshalTOML implements toml.Unmarshaler.
func (b *Bind) UnmarshalTOML(value any) error {
	str, ok := value.(string)
	if !ok {
		return errors.New("bind value was not a string")
	}
	if str == "" {
		return nil
	}
	for _, split := range strings.Split(str, "-") {
		split = strings.ToLower(split)
		if key, ok := x11.Keycodes[split]; ok {
			if b.Key != nil {
				return errors.New("more than one key")
			}
			b.Key = &key
		} else if mod, ok := x11.Modifiers[split]; ok {
			if b.ModCount == 4 {
				return errors.New("too many modifiers (max of 4)")
			}
			b.Mods[b.ModCount] = mod
			b.ModCount += 1
		} else if button, ok := x11.Buttons[split]; ok {
			if b.Button != nil {
				return errors.New("more than one button")
			}
			b.Button = &button
		} else if keyRegexp.MatchString(split) {
			num, err := strconv.Atoi(split[4:])
			if err != nil {
				return fmt.Errorf("failed to parse code in %q", split)
			}
			if b.Key != nil {
				return errors.New("more than one key")
			}
			keycode := xproto.Keycode(num)
			b.Key = &keycode
		} else {
			return fmt.Errorf("unrecognized keybind element %q", split)
		}
	}
	if b.Key != nil && b.Button != nil {
		return errors.New("can only use one key or button per bind")
	}
	b.str = str
	return nil
}

// UnmarshalTOML implements toml.Unmarshaler.
func (k *Keybinds) UnmarshalTOML(value any) error {
	m, ok := value.(map[string]any)
	if !ok {
		return errors.New("bindings value was not a map")
	}
	*k = make(Keybinds)
	for bindStr, actionStr := range m {
		var bind Bind
		var actionList ActionList

		if err := bind.UnmarshalTOML(bindStr); err != nil {
			return fmt.Errorf("parse bind: %w", err)
		}
		if err := actionList.UnmarshalTOML(actionStr); err != nil {
			return fmt.Errorf("parse action list: %w", err)
		}
		(*k)[bind] = actionList
	}
	return nil
}
