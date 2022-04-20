package main

import (
	"os"

	"gopkg.in/yaml.v2"
)

type Config struct {
	OBS struct {
		Enabled  bool   `yaml:"enabled"`
		Port     uint16 `yaml:"port"`
		Password string `yaml:"password"`
	} `yaml:"obs"`
	Keys struct {
		Reset string `yaml:"reset"`
		Focus string `yaml:"focus"`
	} `yaml:"keys"`
	DataPath string `yaml:"data_path"`
}

func GetConfig() (*Config, error) {
	// get configuration path
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}

		cfgDir = home + "/.config"
	}

	cfgPath := cfgDir + "/resetti.yml"

	// if configuration file does not exist, return a blank config
	if _, err := os.Stat(cfgPath); err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		} else {
			return nil, err
		}
	}

	// otherwise, read it
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
