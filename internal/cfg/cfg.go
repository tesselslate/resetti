// Package cfg provides the various configuration types used by resetti,
// along with functionality for reading and writing resetti's configuration
// file.
package cfg

import (
	_ "embed"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/woofdoggo/resetti/internal/x11"
)

//go:embed default.toml
var defaultConfig string

var globalConfig Config

type Config struct {
	General ConfigGeneral `toml:"general"`
	Obs     ConfigObs     `toml:"obs"`
	Reset   ConfigReset   `toml:"reset"`
	Mc      ConfigMc      `toml:"minecraft"`
	Keys    ConfigKeys    `toml:"keybinds"`
	Wall    ConfigWall    `toml:"wall"`
	SSG     ConfigSSG     `toml:"setseed"`
}

type ConfigGeneral struct {
	Type        string `toml:"type"`
	CountResets bool   `toml:"count_resets"`
	CountPath   string `toml:"resets_file"`
	Affinity    string `toml:"affinity"`
}

type ConfigObs struct {
	Enabled  bool   `toml:"enabled"`
	Port     uint16 `toml:"port"`
	Password string `toml:"password"`
}

type ConfigReset struct {
	SetSettings  bool `toml:"set_settings"`
	Delay        int  `toml:"delay"`
	UnpauseFocus bool `toml:"unpause_on_focus"`
}

type ConfigMc struct {
	Fov  int `toml:"fov"`
	Rd   int `toml:"rd"`
	Sens int `toml:"sensitivity"`
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
