// Package cfg allows for reading the user's configuration.
package cfg

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/jezek/xgb/xproto"
	"github.com/woofdoggo/resetti/internal/x11"
)

// go:embed default.toml
var defaultProfile []byte

// Hooks contains various commands to run whenever the user performs certain
// actions.
type Hooks struct {
	Reset      string `toml:"reset"`       // Command to run on ingame reset
	WallLock   string `toml:"wall_lock"`   // Command to run on wall reset
	WallUnlock string `toml:"wall_unlock"` // Command to run on wall unlock
	WallPlay   string `toml:"wall_play"`   // Command to run on wall play
	WallReset  string `toml:"wall_reset"`  // Command to run on wall reset
}

// Keybinds contains the user's keybindings.
type Keybinds struct {
	Focus           Bind `toml:"focus"`             // Focus instance/projector
	Reset           Bind `toml:"reset"`             // Reset all / reset current
	WallLock        Bind `toml:"wall_lock"`         // (Un)lock instance
	WallPlay        Bind `toml:"wall_play"`         // Play instance
	WallReset       Bind `toml:"wall_reset"`        // Reset from wall
	WallResetOthers Bind `toml:"wall_reset_others"` // Focus reset from wall
}

// Obs contains the user's OBS websocket connection information.
type Obs struct {
	Enabled  bool   `toml:"enabled"`  // Mandatory for wall
	Port     uint16 `toml:"port"`     // Connection port
	Password string `toml:"password"` // Password, can be left blank if unused
}

// Wall contains the user's wall settings.
type Wall struct {
	Enabled     bool `toml:"enabled"`      // Whether to use multi or wall
	GotoLocked  bool `toml:"goto_locked"`  // Also known as wall bypass
	GracePeriod int  `toml:"grace_period"` // Milliseconds to wait after preview before a reset can occur

	StretchRes   *Rectangle `toml:"stretch_res"` // Inactive resolution
	UnstretchRes *Rectangle `toml:"play_res"`    // Active resolution
	UseF1        bool       `toml:"use_f1"`

	// Instance hiding (dirt cover) settings.
	Hiding struct {
		// What criteria to use when determining when to show the instance.
		// Only valid options are "percentage" and "delay".
		ShowMethod string `toml:"show_method"`

		// When to show the instances (either milliseconds for delay or
		// generation percentage for percentage.)
		ShowAt int `toml:"show_at"`
	} `toml:"hiding"`

	// Performance settings.
	Performance struct {
		// Optional. Overrides the default sleepbg.lock path ($HOME)
		SleepbgPath string `toml:"sleepbg_path"`

		// Whether or not to use affinity.
		Affinity bool `toml:"affinity"`

		// If enabled, halves the amount of CPU cores available to affinity
		// groups and instead creates double the amount of groups (half for
		// each CCX.)
		CcxSplit bool `toml:"ccx_split"`

		CpusIdle   int `toml:"affinity_idle"`   // CPUs for idle group
		CpusLow    int `toml:"affinity_low"`    // CPUs for low group
		CpusMid    int `toml:"affinity_mid"`    // CPUs for mid group
		CpusHigh   int `toml:"affinity_high"`   // CPUs for high group
		CpusActive int `toml:"affinity_active"` // CPUs for active group

		// The number of milliseconds to wait after an instance finishes
		// generating to move it from the mid group to the idle group.
		// A value of 0 disables this functionality.
		BurstLength int `toml:"burst_length"`

		// The world generation percentage at which instances are moved from
		// the high group to the low group.
		LowThreshold int `toml:"low_threshold"`
	} `toml:"performance"`
}

// Bind represents a single keybinding.
type Bind struct {
	// Keyboard modifiers.
	Mod x11.Keymod

	// Keycode, if any.
	Key *xproto.Keycode

	// Mouse button, if any.
	Mouse *xproto.Button
}

// Profile contains an entire configuration profile.
type Profile struct {
	Delay        int    `toml:"delay"`       // Delay between certain actions
	ResetCount   string `toml:"reset_count"` // Reset counter path
	UnpauseFocus bool   `toml:"unpause_focus"`

	Hooks    Hooks    `toml:"hooks"`
	Keybinds Keybinds `toml:"keybinds"`
	Obs      Obs      `toml:"obs"`
	Wall     Wall     `toml:"wall"`
}

// Rectangle is a rectangle. That's it.
type Rectangle struct {
	X, Y, W, H uint32
}

// GetDirectory returns the path to the user's configuration directory.
func GetDirectory() (string, error) {
	// UserConfigDir automatically checks for $XDG_CONFIG_HOME and falls back
	// to $HOME/.config, so we don't need to do any special checks ourselves.
	xdgDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return xdgDir + "/resetti/", nil
}

// GetProfile returns a parsed configuration profile.
func GetProfile(name string) (Profile, error) {
	dir, err := GetDirectory()
	if err != nil {
		return Profile{}, fmt.Errorf("get config directory: %w", err)
	}
	file, err := os.ReadFile(dir + name + ".toml")
	if err != nil {
		return Profile{}, fmt.Errorf("read config file: %w", err)
	}
	profile := Profile{}
	if err = toml.Unmarshal(file, &profile); err != nil {
		return Profile{}, fmt.Errorf("parse config file: %w", err)
	}
	if err = validateProfile(&profile); err != nil {
		return Profile{}, fmt.Errorf("validate config: %w", err)
	}
	return profile, nil
}

// MakeProfile makes a new configuration profile with the given name and the
// default settings.
func MakeProfile(name string) error {
	dir, err := GetDirectory()
	if err != nil {
		return fmt.Errorf("get config directory: %w", err)
	}
	if stat, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			err := os.Mkdir(dir, 0644)
			if err != nil {
				return fmt.Errorf("create config directory: %w", err)
			}
		}
		if !stat.IsDir() {
			return fmt.Errorf("config directory (%s) is not a directory", dir)
		}
	}
	return os.WriteFile(
		dir+name+".toml",
		[]byte(defaultProfile),
		0644,
	)
}

// validateProfile ensures that the user's configuration profile does not have
// any illegal or invalid settings.
func validateProfile(conf *Profile) error {
	// Fix up the sleepbg.lock path.
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("no $HOME")
	}
	if conf.Wall.Performance.SleepbgPath == "" {
		conf.Wall.Performance.SleepbgPath = home
	}
	conf.Wall.Performance.SleepbgPath += "/sleepbg.lock"

	// Validate the config.
	return nil
}

// MatchButton returns whether or not the given button event matches this bind.
func (b Bind) MatchButton(evt x11.ButtonEvent) bool {
	if b.Mouse == nil {
		return false
	}
	return *b.Mouse == evt.Button && b.Mod == evt.Mod
}

// MatchKey returns whether or not the given key event matches this bind.
func (b Bind) MatchKey(evt x11.KeyEvent) bool {
	if b.Key == nil {
		return false
	}
	return *b.Key == evt.Key.Code && b.Mod == evt.Key.Mod
}

// MatchMove returns whether or not the given move event matches this bind.
func (b Bind) MatchMove(evt x11.MoveEvent) bool {
	if b.Mouse == nil {
		return false
	}
	var buttonMask x11.Keymod
	switch *b.Mouse {
	case xproto.ButtonIndex1:
		buttonMask = xproto.ButtonMask1
	case xproto.ButtonIndex2:
		buttonMask = xproto.ButtonMask2
	case xproto.ButtonIndex3:
		buttonMask = xproto.ButtonMask3
	case xproto.ButtonIndex4:
		buttonMask = xproto.ButtonMask4
	case xproto.ButtonIndex5:
		buttonMask = xproto.ButtonMask5
	}
	return b.Mod|buttonMask == evt.Mod
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
	keyCount := 0
	buttonCount := 0
	for _, split := range strings.Split(str, "-") {
		split = strings.ToLower(split)
		if mod, ok := x11.Keymods[split]; ok {
			b.Mod |= mod
		} else if key, ok := x11.Keycodes[split]; ok {
			b.Key = &key
			keyCount += 1
		} else if button, ok := x11.Buttons[split]; ok {
			b.Mouse = &button
			buttonCount += 1
		} else {
			return fmt.Errorf("unrecognized keybind element %q", split)
		}
	}
	if keyCount+buttonCount == 0 {
		return errors.New("no key or button")
	} else if keyCount+buttonCount > 1 {
		return errors.New("more than one key or button")
	}
	return nil
}

// UnmarshalTOML implements toml.Unmarshaler.
func (r *Rectangle) UnmarshalTOML(value any) error {
	// TODO: allow empty
	str, ok := value.(string)
	if !ok {
		return errors.New("rectangle value was not a string")
	}
	n, err := fmt.Sscanf(str, "%dx%d+%d,%d", &r.W, &r.H, &r.X, &r.Y)
	if err != nil {
		return err
	}
	if n != 4 {
		return errors.New("missing rectangle dimensions")
	}
	return nil
}
