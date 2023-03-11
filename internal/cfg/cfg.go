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
		Focus x11.Key `toml:"focus"`
		Reset x11.Key `toml:"reset"`
	} `toml:"keybinds"`
	Wall struct {
		HideGui         bool   `toml:"hide_gui"`
		StretchWindows  bool   `toml:"stretch_windows"`
		StretchWidth    uint32 `toml:"stretch_width"`
		StretchHeight   uint32 `toml:"stretch_height"`
		UnstretchWidth  uint32 `toml:"unstretch_width"`
		UnstretchHeight uint32 `toml:"unstretch_height"`
		ResizeX         uint32 `toml:"resize_x"`
		ResizeY         uint32 `toml:"resize_y"`
		UseMouse        bool   `toml:"use_mouse"`
		GoToLocked      bool   `toml:"goto_locked"`
		SleepBgLock     bool   `toml:"sleepbg_lock"`
		SleepBgLockPath string `toml:"sleepbg_lock_path"`
	} `toml:"wall"`
	AdvancedWall struct {
		Affinity     bool `toml:"affinity"`
		CcxSplit     bool `toml:"ccx_split"`
		CpusIdle     int  `toml:"affinity_idle"`
		CpusLow      int  `toml:"affinity_low"`
		CpusMid      int  `toml:"affinity_mid"`
		CpusHigh     int  `toml:"affinity_high"`
		CpusActive   int  `toml:"affinity_active"`
		LowThreshold int  `toml:"low_threshold"`
	} `toml:"advanced_wall"`
	MovingWall struct {
		UseMovingWall     bool    `toml:"use_moving_wall"`
		ResetFirstLoaded  x11.Key `toml:"reset_first_loaded"`
		LockFirstLoaded   x11.Key `toml:"lock_first_loaded"`
		UnlockFirstLocked x11.Key `toml:"unlock_first_loaded"`
		PlayFirstLocked   x11.Key `toml:"play_first_locked"`
		PlayFirstLoaded   x11.Key `toml:"play_first_loaded"`
	} `toml:"moving_wall"`
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
