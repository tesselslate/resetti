// Package config provides the various configuration types used by resetti,
// along with functionality for reading and writing resetti's configuration
// file.
package cfg

import (
	"os"

	"gopkg.in/yaml.v2"
)

// Config contains all of the configuration for resetti.
type Config struct {
	OBS struct {
		Enabled  bool   `yaml:"enabled"`
		Port     uint16 `yaml:"port"`
		Password string `yaml:"password"` // If empty, no authentication will be used.
	} `yaml:"obs"` // The settings to use for resetti's OBS integration.
	Keys struct {
		Reset string `yaml:"reset"`
		Focus string `yaml:"focus"`
	} `yaml:"keys"` // The hotkeys to use for resetti's actions.
	DataPath string `yaml:"data_path"` // The path to resetti's log directory.
}

// McSettings contains the user's preferred Minecraft settings for
// automatically adjusting them when resetting.
type McSettings struct {
	Fov        uint8 `yaml:"fov"`
	Render     uint8 `yaml:"rd"`
	Sensitivty uint8 `yaml:"sensitivity"`
}

// ResetSettings contains the user's settings for resetting instances.
type ResetSettings struct {
	LowRd       bool        // Whether or not to keep instances on low render distance while paused.
	Mc          *McSettings // The Minecraft settings to use.
	SetSettings bool        // Whether or not Minecraft settings should be reset automatically.
}

// GetConfig attempts to read the user's configuration file and return it
// in its parsed form.
func GetConfig() (*Config, error) {
	// Get configuration path.
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}

		cfgDir = home + "/.config"
	}

	cfgPath := cfgDir + "/resetti.yml"

	// If the configuration file does not exist, return a blank configuration.
	// TODO: Create a better default configuration.
	if _, err := os.Stat(cfgPath); err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		} else {
			return nil, err
		}
	}

	// If the configuration file exists, read it.
	cfgBytes, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, err
	}

	var cfg Config
	err = yaml.Unmarshal(cfgBytes, &cfg)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}
