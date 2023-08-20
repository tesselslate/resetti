// Package cfg allows for reading the user's configuration.
package cfg

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/woofdoggo/resetti/internal/log"
	"github.com/woofdoggo/resetti/internal/res"
)

// The number of CPUs on the user's system.
var cpuCount int

// Delays contains various delays to make certain actions more consistent.
type Delays struct {
	WpPause   int `toml:"wp_pause"`      // WorldPreview F3+Esc
	IdlePause int `toml:"idle_pause"`    // Idle F3+Esc
	Unpause   int `toml:"unpause"`       // Unpause on focus
	Stretch   int `toml:"stretch"`       // Resize
	GhostPie  int `toml:"ghost_pie_fix"` // Ghost pie fix
	Warp      int `toml:"warp_pointer"`  // Warp pointer fix
}

// Hooks contains various commands to run whenever the user performs certain
// actions.
type Hooks struct {
	Reset      string `toml:"reset"`       // Command to run on ingame reset
	AltRes     string `toml:"alt_res"`     // Command to run on alternate resolution
	NormalRes  string `toml:"normal_res"`  // Command to run on normal resolution
	WallLock   string `toml:"wall_lock"`   // Command to run on wall reset
	WallUnlock string `toml:"wall_unlock"` // Command to run on wall unlock
	WallPlay   string `toml:"wall_play"`   // Command to run on wall play
	WallReset  string `toml:"wall_reset"`  // Command to run on wall reset
}

// Keybinds contains the user's keybindings.
type Keybinds map[Bind]ActionList

// Obs contains the user's OBS websocket connection information.
type Obs struct {
	Enabled   bool    `toml:"enabled"`    // Mandatory for wall
	Port      uint16  `toml:"port"`       // Connection port
	Password  string  `toml:"password"`   // Password, can be left blank if unused
	Port2     *uint16 `toml:"port_2"`     // Verification connection port
	Password2 *string `toml:"password_2"` // Verification connection password
}

// Wall contains the user's wall settings.
type Wall struct {
	Enabled        bool `toml:"enabled"`         // Whether to use multi or wall
	ConfinePointer bool `toml:"confine_pointer"` // Whether or not to confine the pointer to the projector
	GotoLocked     bool `toml:"goto_locked"`     // Also known as wall bypass
	ResetUnlock    bool `toml:"reset_unlock"`    // Reset on unlock
	GracePeriod    int  `toml:"grace_period"`    // Milliseconds to wait after preview before a reset can occur

	StretchRes *Rectangle `toml:"stretch_res"` // Inactive resolution
	UseF1      bool       `toml:"use_f1"`

	// Preview percentage to freeze instances at.
	FreezeAt int `toml:"freeze_at"`

	// Preview percentage to show instances at.
	ShowAt int `toml:"show_at"`

	WallWindow string `toml:"wall_window"` // Name of the wall window

	// Instance moving settings.
	Moving struct {
		Enabled         bool    `toml:"enabled"`
		ResetBeforePlay bool    `toml:"force_reset_before_play"` // Force user to keep all but first group empty
		Gaps            bool    `toml:"use_gaps"`                // Whether to leave gaps when instances are locked
		Locks           *Group  `toml:"locks"`                   // Locked group
		Groups          []Group `toml:"groups"`                  // Normal groups
	} `toml:"moving"`

	// Performance settings.
	Perf struct {
		// Optional. Overrides the default sleepbg.lock path ($HOME)
		SleepbgPath string `toml:"sleepbg_path"`

		// Whether or not to use affinity.
		Affinity string `toml:"affinity"`

		// Sequential affinity settings.
		Seq struct {
			// The number of CPUs to give to the active instance.
			ActiveCpus int `toml:"active_cpus"`

			// The number of CPUs to give to background instances.
			BackgroundCpus int `toml:"background_cpus"`

			// The number of CPUs to give to locked instances.
			LockCpus int `toml:"lock_cpus"`
		} `toml:"sequence"`

		// Advanced affinity settings.
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
	ResetCount   string     `toml:"reset_count"`   // Reset counter path
	UnpauseFocus bool       `toml:"unpause_focus"` // Whether to unpause on focus
	PollRate     int        `toml:"poll_rate"`     // Polling rate for input handling
	NormalRes    *Rectangle `toml:"play_res"`      // Normal resolution
	AltRes       *Rectangle `toml:"alt_res"`       // Alternate ingame resolution

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

	// Whether instances in this group can be clicked on.
	Cosmetic bool `toml:"cosmetic"`

	Width  uint32 `toml:"width"`  // Width of the group, in instances.
	Height uint32 `toml:"height"` // Height of the group, in instances.
}

// Rectangle is a rectangle. That's it.
type Rectangle struct {
	X, Y, W, H uint32
}

// getCpuCount finds the user's CPU count through /sys.
func getCpuCount() (int, error) {
	if cpuCount != 0 {
		panic("CPU count already found")
	}

	// I will absolutely lose it if this file can contain multiple segments
	// on certain CPUs.
	poss, err := os.ReadFile("/sys/devices/system/cpu/present")
	if err != nil {
		return 0, fmt.Errorf("read file: %w", err)
	}
	a, b, _ := strings.Cut(strings.TrimSuffix(string(poss), "\n"), "-")
	x, err := strconv.Atoi(a)
	if err != nil {
		return 0, fmt.Errorf("convert online list: %w", err)
	}
	y, err := strconv.Atoi(b)
	if err != nil {
		return 0, fmt.Errorf("convert online list: %w", err)
	}
	cpuCount = y - x + 1 // e.g. (0-23) == 24
	return cpuCount, nil
}

// GetCpuCount returns the number of CPUs on the user's system.
func GetCpuCount() int {
	if cpuCount == 0 {
		// The benchmark will never cause getCpuCount to get called.
		n, err := getCpuCount()
		if err != nil {
			panic(err)
		}
		cpuCount = n
		return n
	}
	return cpuCount
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
	stat, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			err := os.Mkdir(dir, 0755)
			if err != nil {
				return fmt.Errorf("create config directory: %w", err)
			}
		}
	} else {
		if !stat.IsDir() {
			return fmt.Errorf("config directory (%s) is not a directory", dir)
		}
	}
	return os.WriteFile(
		dir+name+".toml",
		[]byte(res.DefaultConfig),
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
		log.Warn("Very low poll rate in config. Consider increasing.")
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
	if !validateRectangle(conf.NormalRes) {
		return errors.New("invalid playing resolution")
	}
	if !validateRectangle(conf.AltRes) {
		return errors.New("invalid alternate resolution")
	}
	alt := conf.AltRes != nil
	normal := conf.NormalRes != nil
	stretch := conf.Wall.StretchRes != nil
	if stretch && !normal {
		return errors.New("need both stretched and playing resolution")
	}
	if alt && !normal {
		return errors.New("need both alternate and playing resolution")
	}

	// Check moving settings.
	if conf.Wall.Moving.Enabled {
		if len(conf.Wall.Moving.Groups) < 1 {
			return errors.New("need at least one moving group")
		}
		for _, group := range conf.Wall.Moving.Groups {
			if group.Width*group.Height <= 0 {
				return errors.New("each group must have at least one instance")
			}
			if !validateRectangle(&group.Space) {
				return errors.New("each group must occupy a non-zero amount of space")
			}
		}
		locks := conf.Wall.Moving.Locks
		if locks != nil {
			if locks.Width*locks.Height <= 0 {
				return errors.New("lock group must have at least one instance")
			}
			if !validateRectangle(&locks.Space) {
				return errors.New("lock group must occupy a non-zero amount of space")
			}
		}
	}

	// Check affinity settings.
	maxCpu, err := getCpuCount()
	if err != nil {
		return fmt.Errorf("get cpu count: %w", err)
	}
	switch conf.Wall.Perf.Affinity {
	case "":
		break
	case "sequence":
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

	// Fill missing configuration options
	if conf.Wall.WallWindow == "" {
		conf.Wall.WallWindow = "Projector (Scene) - Wall"
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
