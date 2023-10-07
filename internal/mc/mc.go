// Package mc implements facilities for detecting and working with
// Minecraft instances.
package mc

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jezek/xgb/xproto"
	"github.com/tesselslate/resetti/internal/x11"
)

// List of WorldPreview jar hashes that support wpstateout.txt.
var wpHashes = map[string]bool{
	"c8893b913e0e9692ba1344e067f611ddd26a9779": true, // 1.15.2 build 3
	"042d4fa41bef26b5727d6977820a37aad829f2af": true, // 1.16.1 build 3
	"5398cb2adf4ddf99fa96e479b68d54c7d0ad9f0c": true, // 1.16.1 build 4
}

// InstanceInfo contains information about how to interact with a Minecraft
// instance, such as its game directory and window ID.
type InstanceInfo struct {
	Id         int            // Instance number
	Pid        uint32         // Process ID
	Wid        xproto.Window  // Window ID
	Dir        string         // .minecraft directory
	Version    int            // Minecraft version
	ModernWp   bool           // Has wpstateout.txt WorldPreview
	ResetKey   xproto.Keycode // Atum reset key
	PreviewKey xproto.Keycode // Leave preview key
}

// FindInstances returns a sorted list of all running Minecraft instances,
// or an error if the running instances do not form a list.
func FindInstances(x *x11.Client) ([]InstanceInfo, error) {
	instances := make([]InstanceInfo, 0)
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
				instances = append(instances, info)
			} else {
				return instances, fmt.Errorf("unusable instance: %w", err)
			}
		}
	}

	// Sort instances.
	return sortInstances(instances)
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

	// Get instance ID.
	rawId, err := os.ReadFile(fmt.Sprintf("%s/instance_num", pwd))
	if err != nil {
		return InstanceInfo{}, false, err
	}
	id, err := strconv.Atoi(strings.TrimSuffix(string(rawId), "\n"))
	if err != nil {
		return InstanceInfo{}, false, err
	}

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
	previewKey := x11.KeyH
	for _, line := range strings.Split(string(options), "\n") {
		// Only parse this keybind if it is the reset or leave preview key.
		isResetKey := strings.Contains(line, "key_Create New World")
		isPreviewKey := strings.Contains(line, "key_Leave Preview")
		if !isResetKey && !isPreviewKey {
			continue
		}

		// Parse the key.
		keyName := strings.Split(line, ":")[1]
		keyName = strings.TrimPrefix(keyName, "key.keyboard.")
		if keyName == "unknown" {
			return InstanceInfo{}, true, fmt.Errorf("atum's \"Create New World\" and/or world preview's \"Leave Preview\" keybinds were unbound (set them to any key)")
		}
		keycode, ok := x11.KeycodesMc[keyName]
		if !ok {
			return InstanceInfo{}, true, fmt.Errorf("\"reset\" or \"leave preview\" keybinds were set to an unknown keycode %s", keyName)
		}

		// Store it.
		if isResetKey {
			resetKey = keycode
		} else {
			previewKey = keycode
		}
	}

	return InstanceInfo{
		id,
		pid,
		win,
		pwd,
		version,
		modernWp,
		resetKey,
		previewKey,
	}, true, nil
}

// hasModernWp determines whether or not the instance has a WorldPreview build
// with wpstateout.txt.
func hasModernWp(dir string) (bool, error) {
	entries, err := os.ReadDir(dir + "/mods")
	if err != nil {
		return false, fmt.Errorf("read dir: %w", err)
	}
	for _, entry := range entries {
		// This is probably a safe assumption.
		if !strings.Contains(strings.ToLower(entry.Name()), "preview") {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".jar") {
			continue
		}
		jar, err := os.ReadFile(dir + "/mods/" + entry.Name())
		if err != nil {
			return false, fmt.Errorf("read %s: %w", entry.Name(), err)
		}
		hash := sha1.Sum(jar)
		if wpHashes[hex.EncodeToString(hash[:])] {
			return true, nil
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

// sortInstances returns a sorted list of all open instances, or an error if
// some instances are missing or out of order.
func sortInstances(instances []InstanceInfo) ([]InstanceInfo, error) {
	// Return an error if no instances were found.
	if len(instances) == 0 {
		return nil, errors.New("no instances found")
	}

	// Sort the instances based on ID.
	sort.Slice(instances, func(i, j int) bool {
		return instances[i].Id < instances[j].Id
	})

	// Ensure that all instances are present.
	maxId := 0
	for _, instance := range instances {
		if instance.Id > maxId {
			maxId = instance.Id
		}
	}

	found := make([]bool, maxId+1)
	sorted := true
	for index, instance := range instances {
		if instance.Id != index {
			sorted = false
		} else {
			found[instance.Id] = true
		}
	}

	if sorted {
		return instances, nil
	} else {
		missing := []string{fmt.Sprintf("Expected %d instances", maxId+1)}
		for index, exists := range found {
			if !exists {
				missing = append(missing, fmt.Sprintf("Missing instance %d", index+1))
			}
		}
		return nil, errors.New(strings.Join(missing, "\n"))
	}
}
