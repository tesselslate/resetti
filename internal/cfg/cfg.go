// Package cfg allows for reading the user's configuration.
package cfg

import (
	_ "embed"
	"errors"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/tesselslate/resetti/internal/log"
	"github.com/tesselslate/resetti/internal/res"
)

// Hooks contains various commands to run whenever the user performs certain
// actions.
type Hooks struct {
	Reset     string        `toml:"reset"`      // Command to run on ingame reset
	AltRes    AltResHook    `toml:"alt_res"`    // Command to run on alternate resolution
	NormalRes NormalResHook `toml:"normal_res"` // Command to run on normal resolution
}

// Keybinds contains the user's keybindings.
type Keybinds map[Bind]ActionList

// Profile contains an entire configuration profile.
type Profile struct {
	PollRate  int        `toml:"poll_rate"` // Polling rate for input handling
	NormalRes *Rectangle `toml:"play_res"`  // Normal resolution
	AltRes    AltRes     `toml:"alt_res"`   // Alternate ingame resolution

	Hooks    Hooks    `toml:"hooks"`
	Keybinds Keybinds `toml:"keybinds"`
}

// Rectangle is a rectangle. That's it.
type Rectangle struct {
	X, Y int32
	W, H uint32
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

	// Check resolution settings.
	if !validateRectangle(conf.NormalRes) {
		return errors.New("invalid playing resolution")
	}
	for idx, res := range conf.AltRes {
		if !validateRectangle(&res) {
			if len(conf.AltRes) == 1 {
				return errors.New("invalid alternate resolution")
			} else {
				return fmt.Errorf("invalid alternate resolution %v at index %d", res, idx)
			}
		}
	}
	alt := conf.AltRes != nil
	normal := conf.NormalRes != nil
	if alt && !normal {
		return errors.New("need both alternate and playing resolution")
	}

	return nil
}

// parseRectangle attempts to parse the string representation of a Rectangle.
func parseRectangle(raw string) (Rectangle, error) {
	r := Rectangle{}
	n, err := fmt.Sscanf(raw, "%dx%d+%d,%d", &r.W, &r.H, &r.X, &r.Y)
	if err != nil {
		return r, err
	}
	if n != 4 {
		return r, errors.New("missing rectangle dimensions")
	}
	return r, nil
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
	rect, err := parseRectangle(str)
	if err != nil {
		return err
	}
	*r = rect
	return nil
}
