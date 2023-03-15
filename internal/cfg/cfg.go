package cfg

import (
	_ "embed"
	"errors"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/woofdoggo/resetti/internal/x11"
)

//go:embed default.toml
var defaultConfig string

type Box struct {
	X      int
	Y      int
	Width  int
	Height int
}

type Profile struct {
	General struct {
		ResetType   string `toml:"type"`
		CountResets bool   `toml:"count_resets"`
		CountPath   string `toml:"resets_file"`
	} `toml:"general"`
	Hooks struct {
		WallReset string `toml:"wall_reset"`
		Reset     string `toml:"reset"`
		Lock      string `toml:"lock"`
		Unlock    string `toml:"unlock"`
	} `toml:"hooks"`
	Obs struct {
		Enabled  bool   `toml:"enabled"`
		Port     uint16 `toml:"port"`
		Password string `toml:"password"`
	} `toml:"obs"`
	Reset struct {
		Delay        int  `toml:"delay"`
		PauseDelay   int  `toml:"pause_delay"`
		UnpauseFocus bool `toml:"unpause_on_focus"`
	} `toml:"reset"`
	Keys struct {
		Focus           x11.Key    `toml:"focus"`
		Reset           x11.Key    `toml:"reset"`
		WallReset       x11.Keymod `toml:"wall_reset"`
		WallResetOthers x11.Keymod `toml:"wall_reset_others"`
		WallPlay        x11.Keymod `toml:"wall_play"`
		WallLock        x11.Keymod `toml:"wall_lock"`
	} `toml:"keybinds"`
	Wall struct {
		HideGui         bool   `toml:"hide_gui"`
		StretchWindows  bool   `toml:"stretch_windows"`
		StretchRes      Box    `toml:"stretch_res"`
		UnstretchRes    Box    `toml:"unstretch_res"`
		UseMouse        bool   `toml:"use_mouse"`
		GoToLocked      bool   `toml:"goto_locked"`
		SleepBgLock     bool   `toml:"sleepbg_lock"`
		SleepBgLockPath string `toml:"sleepbg_lock_path"`
		GracePeriod     int    `toml:"grace_period"`
		InstanceHiding  bool   `toml:"hide_instances"`
		ShowDelay       int    `toml:"show_delay"`
	} `toml:"wall"`
	Moving struct {
		Enabled   bool   `toml:"enabled"`
		FocusSize string `toml:"focus_size"`
	} `toml:"moving"`
	AdvancedWall struct {
		Affinity     bool `toml:"affinity"`
		CcxSplit     bool `toml:"ccx_split"`
		CpusIdle     int  `toml:"affinity_idle"`
		CpusLow      int  `toml:"affinity_low"`
		CpusMid      int  `toml:"affinity_mid"`
		CpusHigh     int  `toml:"affinity_high"`
		CpusActive   int  `toml:"affinity_active"`
		BurstLength  int  `toml:"burst_length"`
		LowThreshold int  `toml:"low_threshold"`
	} `toml:"advanced_wall"`
}

// GetFolder returns the path to the user's configuration folder.
func GetFolder() (string, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		cfgDir = home + "/.config"
	}
	cfgPath := cfgDir + "/resetti/"
	return cfgPath, nil
}

// GetProfile returns a specific configuration profile.
func GetProfile(name string) (Profile, error) {
	dir, err := GetFolder()
	if err != nil {
		return Profile{}, err
	}
	file, err := os.ReadFile(dir + name + ".toml")
	if err != nil {
		return Profile{}, err
	}
	conf := Profile{}
	err = toml.Unmarshal(file, &conf)
	if err != nil {
		return Profile{}, err
	}

	// TODO: validate config

	// Set SleepBackground lock path.
	if conf.Wall.SleepBgLock && conf.Wall.SleepBgLockPath == "" {
		userDir, err := os.UserHomeDir()
		if err != nil {
			return Profile{}, fmt.Errorf("failed to get user dir for sleepbg lock: %s", err)
		}
		conf.Wall.SleepBgLockPath = userDir
	}
	conf.Wall.SleepBgLockPath += "/sleepbg.lock"

	// Update grace period.
	if conf.Wall.InstanceHiding {
		conf.Wall.GracePeriod += conf.Wall.ShowDelay
	}

	return conf, nil
}

// MakeProfile makes a new profile with the default configuration.
func MakeProfile(name string) error {
	dir, err := GetFolder()
	if err != nil {
		return err
	}
	if stat, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			err := os.Mkdir(dir, 0644)
			if err != nil {
				return fmt.Errorf("failed to create config dir: %s", err)
			}
		} else if !stat.IsDir() {
			return errors.New("config path is not a directory")
		}
	}
	return os.WriteFile(
		dir+name+".toml",
		[]byte(defaultConfig),
		0644,
	)
}

func (b *Box) UnmarshalTOML(value any) error {
	str, ok := value.(string)
	if !ok {
		return errors.New("value not a string")
	}
	n, err := fmt.Sscanf(str, "%dx%d+%d,%d", &b.Width, &b.Height, &b.X, &b.Y)
	if err != nil {
		return err
	}
	if n != 4 {
		return errors.New("missing value")
	}
	return nil
}
