// Package mc implements facilities for detecting and working with
// Minecraft instances.
package mc

import (
	"archive/zip"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jezek/xgb/xproto"
	"github.com/tesselslate/resetti/internal/x11"
)

// List of mod class names that indicate state output support.
var stateOutputClasses = map[string]bool{
	"me/voidxwalker/worldpreview/StateOutputHelper.class": true,
	"xyz/tildejustin/stateoutput/":                        true,
	"dev/tildejustin/stateoutput/":                        true,
}

// InstanceInfo contains information about how to interact with a Minecraft
// instance, such as its game directory and window ID.
type InstanceInfo struct {
	Pid      uint32         // Process ID
	Wid      xproto.Window  // Window ID
	Dir      string         // .minecraft directory
	Version  int            // Minecraft version
	ModernWp bool           // Has wpstateout.txt WorldPreview
	ResetKey xproto.Keycode // Atum reset key
}

// FindInstance returns the running Minecraft instance,
// or an error if it doesn't find any.
func FindInstance(x *x11.Client) (InstanceInfo, error) {
	windows := x.GetWindowList()

	// Check every window to see if it is a Minecraft instance.
	for _, win := range windows {
		// Skip this window if it is not a Minecraft instance.
		if !isMinecraftWindow(x, win) {
			continue
		}

		// Get the info for this instance.
		info, was_instance, err := getInstanceInfo(x, win)
		if was_instance {
			if err == nil {
				return info, nil
			} else {
				return InstanceInfo{}, fmt.Errorf("unusable instance: %w", err)
			}
		}
	}
	return InstanceInfo{}, fmt.Errorf("no instance found")
}

// getInstanceInfo attempts to gather information about the given Minecraft
// instance.
func getInstanceInfo(x *x11.Client, win xproto.Window) (InstanceInfo, bool, error) {
	// Get process ID.
	pid, err := x.GetWindowPid(win)
	if err != nil {
		return InstanceInfo{}, false, err
	}

	// Get instance directory.
	rawPwd, err := filepath.EvalSymlinks(fmt.Sprintf("/proc/%d/cwd", pid))
	if err != nil {
		return InstanceInfo{}, false, err
	}
	pwd := string(rawPwd)

	// Get game version.
	title, err := x.GetWindowTitle(win)
	if err != nil {
		return InstanceInfo{}, false, err
	}
	versionString := strings.Split(
		strings.Split(title, " ")[1],
		".",
	)[1]
	version, err := strconv.Atoi(versionString)
	if err != nil {
		return InstanceInfo{}, false, err
	}
	if version < 14 {
		return InstanceInfo{}, false, errors.New("only 1.14 and newer are currently supported")
	}

	// Determine if the instance has wpstateout.txt.
	modernWp, err := hasModernWp(pwd)
	if err != nil {
		return InstanceInfo{}, true, fmt.Errorf("has modern wp: %w", err)
	}

	// Get the Atum and WorldPreview keys from the user's options.
	options, err := os.ReadFile(pwd + "/options.txt")
	if err != nil {
		return InstanceInfo{}, true, fmt.Errorf("couldn't open instance options.txt: %w", err)
	}
	resetKey := x11.KeyF6
	for _, line := range strings.Split(string(options), "\n") {
		// Only parse this keybind if it is the Atum reset key.
		isResetKey := strings.Contains(line, "key_Create New World")
		if !isResetKey {
			continue
		}

		// Parse the key.
		keyName := strings.Split(line, ":")[1]
		keyName = strings.TrimPrefix(keyName, "key.keyboard.")
		if keyName == "unknown" {
			return InstanceInfo{}, true, fmt.Errorf("atum's \"Create New World\" keybind was unbound (set it to any key)")
		}
		keycode, ok := x11.KeycodesMc[keyName]
		if !ok {
			return InstanceInfo{}, true, fmt.Errorf("atum's \"Create New World\" keybind was set to an unknown keycode %s", keyName)
		}

		// Store it.
		if isResetKey {
			resetKey = keycode
		}
	}

	return InstanceInfo{
		pid,
		win,
		pwd,
		version,
		modernWp,
		resetKey,
	}, true, nil
}

// hasModernWp determines whether or not the instance has a WorldPreview build
// with wpstateout.txt.
func hasModernWp(dir string) (bool, error) {
	entries, err := os.ReadDir(dir + "/mods")
	if err != nil {
		return false, fmt.Errorf("read dir: %w", err)
	}

	checkZip := func(name string) (bool, error) {
		archive, err := zip.OpenReader(dir + "/mods/" + name)
		if err != nil {
			return false, fmt.Errorf("open zip %q: %w", name, err)
		}
		defer func() {
			_ = archive.Close()
		}()

		for _, file := range archive.File {
			if stateOutputClasses[file.Name] {
				return true, nil
			}
		}
		return false, nil
	}

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".jar") {
			continue
		}
		ok, err := checkZip(entry.Name())
		if ok || err != nil {
			return ok, err
		}
	}
	return false, nil
}

// isMinecraftWindow determines whether or not the window is a Minecraft
// window.
func isMinecraftWindow(x *x11.Client, win xproto.Window) bool {
	// Check that the window has "Minecraft" in its class.
	//
	// There are more checks which could be performed here (e.g. checking that
	// the executable is java, and that the process working directory is a
	// valid Minecraft directory), but any false positives are weeded out when
	// getting instance info.
	class, err := x.GetWindowClass(win)
	if err != nil {
		return false
	}
	if !strings.Contains(class, "Minecraft") {
		return false
	}
	return true
}
