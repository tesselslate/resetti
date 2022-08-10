// Package cfg provides the various configuration types used by resetti,
// along with functionality for reading and writing resetti's configuration
// file.
package cfg

import (
	_ "embed"
	"errors"
	"os"
	"runtime"

	"github.com/BurntSushi/toml"
	"github.com/woofdoggo/resetti/internal/x11"
)

//go:embed default.toml
var defaultConfig string

var globalConfig Config

type Config struct {
	General  ConfigGeneral  `toml:"general"`
	Affinity ConfigAffinity `toml:"affinity"`
	Hooks    ConfigHooks    `toml:"hooks"`
	Obs      ConfigObs      `toml:"obs"`
	Reset    ConfigReset    `toml:"reset"`
	Keys     ConfigKeys     `toml:"keybinds"`
	Wall     ConfigWall     `toml:"wall"`
	SSG      ConfigSSG      `toml:"setseed"`
}

type ConfigGeneral struct {
	Type        string `toml:"type"`
	CountResets bool   `toml:"count_resets"`
	CountPath   string `toml:"resets_file"`
}

type ConfigAffinity struct {
	Enabled    bool   `toml:"enabled"`
	Mode       string `toml:"mode"`
	CpuIdle    int    `toml:"cpus_idle"`
	CpuFast    int    `toml:"cpus_genfast"`
	CpuSlow    int    `toml:"cpus_genslow"`
	CpuActive  int    `toml:"cpus_active"`
	Reallocate string `toml:"reallocate"`
}

type ConfigHooks struct {
	WallReset string `toml:"wall_reset"`
	Reset     string `toml:"reset"`
	Lock      string `toml:"lock"`
	Unlock    string `toml:"unlock"`
}

type ConfigObs struct {
	Enabled  bool   `toml:"enabled"`
	Port     uint16 `toml:"port"`
	Password string `toml:"password"`
}

type ConfigReset struct {
	Delay        int  `toml:"delay"`
	UnpauseFocus bool `toml:"unpause_on_focus"`
}

type ConfigKeys struct {
	Focus           x11.Key    `toml:"focus"`
	Reset           x11.Key    `toml:"reset"`
	WallReset       x11.Keymod `toml:"wall_reset"`
	WallResetOthers x11.Keymod `toml:"wall_reset_others"`
	WallPlay        x11.Keymod `toml:"wall_play"`
	WallLock        x11.Keymod `toml:"wall_lock"`
}

type ConfigWall struct {
	StretchWindows bool   `toml:"stretch_windows"`
	StretchWidth   uint16 `toml:"stretch_width"`
	StretchHeight  uint16 `toml:"stretch_height"`
	UseMouse       bool   `toml:"use_mouse"`
	GoToLocked     bool   `toml:"goto_locked"`
	NoPlayGen      bool   `toml:"no_play_generating"`
}

type ConfigSSG struct {
	Seed   string  `toml:"seed"`
	SpawnX float64 `toml:"spawn_x"`
	SpawnZ float64 `toml:"spawn_z"`
	Radius float64 `toml:"radius"`
}

// GetConfig returns the global configuration.
func GetConfig() Config {
	return globalConfig
}

// GetPath returns the path to the user's configuration folder.
func GetPath() (string, error) {
	// Get configuration path.
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

// GetProfile returns the configuration of a specific profile.
func GetProfile(name string) (*Config, error) {
	dir, err := GetPath()
	if err != nil {
		return nil, err
	}
	contents, err := os.ReadFile(dir + name + ".toml")
	if err != nil {
		return nil, err
	}
	c := &Config{}
	err = toml.Unmarshal(contents, &c)
	if err != nil {
		return nil, err
	}
	// Validate profile.
	if c.General.Type != "standard" && c.General.Type != "wall" && c.General.Type != "setseed" {
		return nil, errors.New("invalid resetter type")
	}
	if c.Affinity.Enabled {
		mode := c.Affinity.Mode
		if mode != "sequence" && mode != "alternate" && mode != "double" && mode != "advanced" {
			return nil, errors.New("invalid affinity mode")
		}
		if mode == "advanced" {
			if c.General.Type != "wall" {
				return nil, errors.New("cannot use advanced affinity without wall")
			}
			cpus := runtime.NumCPU()
			count := c.Affinity.CpuIdle +
				c.Affinity.CpuFast +
				c.Affinity.CpuSlow +
				c.Affinity.CpuActive
			if count > cpus {
				return nil, errors.New("affinity CPU count is greater than number of available CPUs")
			}
			if c.Affinity.Reallocate != "none" && c.Affinity.Reallocate != "genslow" && c.Affinity.Reallocate != "genfast" {
				return nil, errors.New("invalid CPU reallocation")
			}
		}
	}
	return c, nil
}

// GetProfiles returns a list of all available profiles.
func GetProfiles() ([]string, error) {
	dir, err := GetPath()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	res := make([]string, 0)
	for _, v := range entries {
		if v.IsDir() {
			continue
		}
		res = append(res, v.Name())
	}
	return res, nil
}

// LoadProfile loads the given configuration profile.
func LoadProfile(name string) error {
	conf, err := GetProfile(name)
	if err != nil {
		return err
	}
	globalConfig = *conf
	return nil
}

// MakeProfile makes a new profile with the default configuration.
func MakeProfile(name string) error {
	dir, err := GetPath()
	if err != nil {
		return err
	}
	return os.WriteFile(
		dir+name+".toml",
		[]byte(defaultConfig),
		0644,
	)
}
