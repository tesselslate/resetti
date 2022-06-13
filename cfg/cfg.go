// Package cfg provides the various configuration types used by resetti,
// along with functionality for reading and writing resetti's configuration
// file.
package cfg

import (
	"os"

	"github.com/BurntSushi/toml"
	"github.com/woofdoggo/resetti/x11"
)

const (
	KeyReset int = 0
	KeyFocus int = 1
)

type Config struct {
	General ConfigGeneral `toml:"general"`
	Obs     ConfigObs     `toml:"obs"`
	Reset   ConfigReset   `toml:"reset"`
	Mc      ConfigMc      `toml:"minecraft"`
	Keys    ConfigKeys    `toml:"keybinds"`
	Wall    ConfigWall    `toml:"wall"`
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
	SetSettings bool `toml:"set_settings"`
	Delay       int  `toml:"delay"`
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
	StretchWindows bool `toml:"stretch_windows"`
	UseMouse       bool `toml:"use_mouse"`
}

// GetConfig attempts to read the user's configuration file and return it
// in its parsed form.
func GetConfig() (*Config, error) {
	cfgPath, err := GetPath()
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(cfgPath); err != nil {
		return nil, err
	}

	// If the configuration file exists, read it.
	cfgBytes, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, err
	}

	var cfg Config
	err = toml.Unmarshal(cfgBytes, &cfg)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
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
