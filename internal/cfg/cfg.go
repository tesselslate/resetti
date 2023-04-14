// Package cfg allows for reading the user's configuration.
package cfg

import (
	_ "embed"
	"errors"
	"fmt"
	"log"
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
	Enabled        bool `toml:"enabled"`         // Whether to use multi or wall
	ConfinePointer bool `toml:"confine_pointer"` // Whether or not to confine the pointer to the projector
	GotoLocked     bool `toml:"goto_locked"`     // Also known as wall bypass
	GracePeriod    int  `toml:"grace_period"`    // Milliseconds to wait after preview before a reset can occur

	StretchRes   *Rectangle `toml:"stretch_res"` // Inactive resolution
	UnstretchRes *Rectangle `toml:"play_res"`    // Active resolution
    ThinRes *Rectangle `toml:"thin_res"`
	UseF1        bool       `toml:"use_f1"`

	// Preview percentage to show instance at.
	ShowAt int `toml:"show_at"`

	// Instance moving settings.
	Moving struct {
		Enabled bool    `toml:"enabled"`
		Locks   *Group  `toml:"locks"`  // Locked group
		Groups  []Group `toml:"groups"` // Normal groups
        Gaps   bool `toml:"use_gaps"`
	} `toml:"moving"`

	// Performance settings.
	Perf struct {
		// Optional. Overrides the default sleepbg.lock path ($HOME)
		SleepbgPath string `toml:"sleepbg_path"`

		// Whether or not to use affinity.
		Affinity string `toml:"affinity"`

		// Seq affinity settings.
		Seq struct {
			// The number of CPUs to give to the active instance.
			ActiveCpus int `toml:"active_cpus"`

			// The number of CPUs to give to background instances.
			BackgroundCpus int `toml:"background_cpus"`

			// The number of CPUs to give to locked instances.
			LockCpus int `toml:"lock_cpus"`
		} `toml:"sequence"`

		// Adv affinity settings.
		Adv struct {
			CcxSplit int `toml:"ccx_split"`

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
		} `toml:"advanced"`
	} `toml:"performance"`
}

// Profile contains an entire configuration profile.
type Profile struct {
	ResetCount   string `toml:"reset_count"` // Reset counter path
	UnpauseFocus bool   `toml:"unpause_focus"`
	PollRate     int    `toml:"poll_rate"`

	Delay    Delays   `toml:"delay"`
	Hooks    Hooks    `toml:"hooks"`
	Keybinds Keybinds `toml:"keybinds"`
	Obs      Obs      `toml:"obs"`
	Wall     Wall     `toml:"wall"`
}

// Group represents a group of instances for moving.
type Group struct {
	// The space this group occupies on the wall scene.
	Space Rectangle `toml:"position"`

	Width  uint32 `toml:"width"`  // Width of the group, in instances.
	Height uint32 `toml:"height"` // Height of the group, in instances.
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
	// Make sure polling rate is fine.
	if conf.PollRate <= 0 {
		return errors.New("invalid polling rate")
	}
	if conf.PollRate <= 10 {
		log.Println("Warning: Very low poll rate in config. Consider increasing.")
	}

	// Fix up the sleepbg.lock path.
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("no $HOME")
	}
	if conf.Wall.Perf.SleepbgPath == "" {
		conf.Wall.Perf.SleepbgPath = home
	}
	conf.Wall.Perf.SleepbgPath += "/sleepbg.lock"

	// Check resolution settings.
	if !validateRectangle(conf.Wall.StretchRes) {
		return errors.New("invalid stretched resolution")
	}
	if !validateRectangle(conf.Wall.UnstretchRes) {
		return errors.New("invalid active resolution")
	}
    if !validateRectangle(conf.Wall.ThinRes) {
		return errors.New("invalid thin resolution")
	}
	stretch := conf.Wall.StretchRes != nil
	unstretch := conf.Wall.UnstretchRes != nil
	if (stretch || unstretch) && (!stretch || !unstretch) {
		return errors.New("need both stretched and active resolution")
	}

	// TODO moving

	// Check affinity settings.
	switch conf.Wall.Perf.Affinity {
	case "":
		break
	case "sequence":
		maxCpu := runtime.NumCPU()
		seq := conf.Wall.Perf.Seq
		if seq.ActiveCpus > maxCpu {
			return fmt.Errorf("invalid active cpu count %d", seq.ActiveCpus)
		}
		if seq.BackgroundCpus > maxCpu {
			return fmt.Errorf("invalid background cpu count %d", seq.BackgroundCpus)
		}
		if seq.LockCpus > maxCpu {
			return fmt.Errorf("invalid lock cpu count %d", seq.LockCpus)
		}
	case "advanced":
		if conf.Wall.Perf.Adv.CcxSplit <= 0 {
			return fmt.Errorf("invalid ccx split %d", conf.Wall.Perf.Adv.CcxSplit)
		}

		maxCpu := runtime.NumCPU()
		adv := conf.Wall.Perf.Adv
		maxCpu /= adv.CcxSplit
		if adv.CpusIdle > maxCpu {
			return fmt.Errorf("invalid idle cpu count (%d > %d)", adv.CpusIdle, maxCpu)
		}
		if adv.CpusLow > maxCpu {
			return fmt.Errorf("invalid low cpu count (%d > %d)", adv.CpusLow, maxCpu)
		}
		if adv.CpusMid > maxCpu {
			return fmt.Errorf("invalid mid cpu count (%d > %d)", adv.CpusMid, maxCpu)
		}
		if adv.CpusHigh > maxCpu {
			return fmt.Errorf("invalid high cpu count (%d > %d)", adv.CpusHigh, maxCpu)
		}
		if adv.CpusActive > maxCpu {
			return fmt.Errorf("invalid active cpu count (%d > %d)", adv.CpusActive, maxCpu)
		}
	default:
		return fmt.Errorf("invalid affinity setting %q", conf.Wall.Perf.Affinity)
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
