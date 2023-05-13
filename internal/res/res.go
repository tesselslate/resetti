// Package res contains various resources embedded within resetti that are
// used elsewhere.
package res

import (
	"crypto/sha1"
	_ "embed"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

const (
	CgroupScriptPath  = "/cgroup_setup.sh"
	DefaultConfigPath = "/default.toml"
	ObsScriptPath     = "/scene-setup.lua"
)

// CgroupScript contains the cgroups setup shell script.
//
//go:embed cgroup_setup.sh
var CgroupScript []byte

// DefaultConfig contains the example configuration.
//
//go:embed default.toml
var DefaultConfig []byte

// ObsScript contains the OBS scene collection generator script.
//
//go:embed scene-setup.lua
var ObsScript []byte

// dataDir contains the directory in which resources are stored. It is assigned
// by WriteResources on startup.
var dataDir string

// This variable is intended for packagers. It can be modified using LDFLAGS.
// Set this variable at build time if you want to change the location where
// various extra files (cgroups setup script, OBS scene generator, etc) can
// be found.
var overrideDataDir string

// getDataDirectory returns the path to the data directory for resetti.
// If an override was specified at build time, it will be used. Otherwise,
// $XDG_DATA_HOME/resetti or $HOME/.local/share/resetti will be used.
func getDataDirectory() (string, error) {
	if overrideDataDir != "" {
		return overrideDataDir, nil
	}

	dir, ok := os.LookupEnv("XDG_DATA_HOME")
	if ok {
		return dir + "/resetti", nil
	}
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return dir + "/.local/share/resetti", nil
}

// GetDataDirectory returns the path to the data directory for resetti.
// If an override was specified at build time, it will be used. Otherwise,
// $XDG_DATA_HOME/resetti or $HOME/.local/share/resetti will be used.
func GetDataDirectory() string {
	return dataDir
}

// WriteResources writes various resources to disk on startup if needed.
func WriteResources() error {
	dir, err := getDataDirectory()
	if err != nil {
		return fmt.Errorf("get data dir: %w", err)
	}
	dataDir = dir

	if overrideDataDir != "" {
		return nil
	}
	_, err = os.Stat(dataDir)
	if os.IsNotExist(err) {
		if err := os.Mkdir(dataDir, 0755); err != nil {
			return fmt.Errorf("failed to create data dir: %w", err)
		}
	}
	if err := unix.Access(dataDir, unix.W_OK); err != nil {
		return fmt.Errorf("access data dir: %w", err)
	}

	resources := map[string][]byte{
		CgroupScriptPath:  CgroupScript,
		DefaultConfigPath: DefaultConfig,
		ObsScriptPath:     ObsScript,
	}
	for name, contents := range resources {
		// Only overwrite if changed.
		_, err = os.Stat(dataDir + name)
		if err == nil {
			file, err := os.ReadFile(dataDir + name)
			if err != nil {
				return fmt.Errorf("read %s: %w", name, err)
			}
			if sha1.Sum(contents) == sha1.Sum(file) {
				continue
			}
		}

		if err := os.WriteFile(dataDir+name, contents, 0644); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	return nil
}
