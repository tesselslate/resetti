// Package cfg allows for reading the user's configuration.
package cfg

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"runtime"

	"github.com/BurntSushi/toml"
)

//go:embed default.toml
var defaultProfile []byte

// Delays contains various delays to make certain actions more consistent.
type Delays struct {
	WpPause   int `toml:"wp_pause"`      // WorldPreview F3+Esc
	IdlePause int `toml:"idle_pause"`    // Idle F3+Esc
	Unpause   int `toml:"unpause"`       // Unpause on focus
	Stretch   int `toml:"stretch"`       // Resize
	GhostPie  int `toml:"ghost_pie_fix"` // Ghost pie fix
}

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
type Keybinds map[Bind]ActionList

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
		Affinity string `toml:"affinity"`

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

// Profile contains an entire configuration profile.
type Profile struct {
	ResetCount   string `toml:"reset_count"` // Reset counter path
	UnpauseFocus bool   `toml:"unpause_focus"`

	Delay    Delays   `toml:"delay"`
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

	// Check resolution settings.
	if !validateRectangle(conf.Wall.StretchRes) {
		return errors.New("invalid stretched resolution")
	}
	if !validateRectangle(conf.Wall.UnstretchRes) {
		return errors.New("invalid active resolution")
	}
	stretch := conf.Wall.StretchRes != nil
	unstretch := conf.Wall.UnstretchRes != nil
	if (stretch || unstretch) && (!stretch || !unstretch) {
		return errors.New("need both stretched and active resolution")
	}

	// TODO: Check instance hiding settings (implement hiding first)

	// Check affinity settings.
	switch conf.Wall.Performance.Affinity {
	case "", "sequence":
		break
	case "advanced":
		maxCpu := runtime.NumCPU()
		if conf.Wall.Performance.CcxSplit {
			maxCpu /= 2
		}
		if conf.Wall.Performance.CpusIdle > maxCpu {
			return fmt.Errorf("invalid idle cpu count (%d > %d)", conf.Wall.Performance.CpusIdle, maxCpu)
		}
		if conf.Wall.Performance.CpusLow > maxCpu {
			return fmt.Errorf("invalid low cpu count (%d > %d)", conf.Wall.Performance.CpusLow, maxCpu)
		}
		if conf.Wall.Performance.CpusMid > maxCpu {
			return fmt.Errorf("invalid mid cpu count (%d > %d)", conf.Wall.Performance.CpusMid, maxCpu)
		}
		if conf.Wall.Performance.CpusHigh > maxCpu {
			return fmt.Errorf("invalid high cpu count (%d > %d)", conf.Wall.Performance.CpusHigh, maxCpu)
		}
		if conf.Wall.Performance.CpusActive > maxCpu {
			return fmt.Errorf("invalid active cpu count (%d > %d)", conf.Wall.Performance.CpusActive, maxCpu)
		}
	default:
		return fmt.Errorf("invalid affinity setting %q", conf.Wall.Performance.Affinity)
	}
	return nil
}

// validateRectangle ensures the rectangle has a size.
func validateRectangle(r *Rectangle) bool {
	return r == nil || r.W > 0 && r.H > 0
}

// UnmarshalTOML implements toml.Unmarshaler.
func (r *Rectangle) UnmarshalTOML(value any) error {
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
