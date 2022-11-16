package reset

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/jezek/xgb/xproto"
	"github.com/woofdoggo/resetti/internal/x11"
)

const (
	StMenu       int = iota // Main menu
	StGenerating            // World generating (dirt screen)
	StPreview               // World preview
	StIdle                  // Instance finished generating
	StIngame                // Instance being played
	StFrozenGen             // Frozen world generation
	StFrozenIdle            // Frozen idle
)

type Instance struct {
	Id       int           // Instance number
	Pid      uint32        // Process ID
	Wid      xproto.Window // Window ID
	Dir      string        // .minecraft directory
	Version  int           // Minecraft version
	ResetKey x11.Key       // The Atum reset key.
}

type InstanceState struct {
	State    int        // General state (generating, preview, e.t.c.)
	Progress int        // World generation progress
	Spawn    [2]float64 // Spawn location (only relevant for setseed)
}

func (i *InstanceState) String() string {
	states := []string{
		"Menu",
		"Generating",
		"Preview",
		"Idle",
		"Ingame",
		"Frozen (Gen)",
		"Frozen (Idle)",
	}
	switch i.State {
	default:
		return states[i.State]
	case StGenerating, StPreview:
		return fmt.Sprintf("%s (%d%%)", states[i.State], i.Progress)
	}
}

// findInstances returns a list of all running Minecraft instances.
func findInstances(x *x11.Client) ([]Instance, error) {
	instances := make([]Instance, 0)
	windows, err := x.GetAllWindows()
	if err != nil {
		return nil, err
	}
	for _, win := range windows {
		// Check window class.
		class, err := x.GetWindowClass(win)
		if err != nil {
			continue
		}
		if !strings.Contains(class, "Minecraft") {
			continue
		}

		// Get window PID.
		pid, err := x.GetWindowPid(win)
		if err != nil {
			continue
		}

		pid_str := strconv.Itoa(int(pid))
		cmd := exec.Command("pwdx", pid_str)
		stdout, err := cmd.Output()
		if err != nil {
			continue
		}

		gameDir := strings.Split(string(stdout), ":")[1]
		if gameDir == "" {
			continue
		}
		gameDir = strings.Trim(gameDir, "\n")
		gameDir = strings.Trim(gameDir, " ")

		// Get instance ID.
		file, err := os.ReadFile(fmt.Sprintf("%s/instance_num", gameDir))
		if err != nil {
			continue
		}
		id, err := strconv.Atoi(strings.Trim(string(file), "\n"))
		if err != nil {
			continue
		}

		// Get game version.
		verstr := strings.Split(
			strings.Split(class, " ")[1],
			".",
		)[1]
		version, err := strconv.Atoi(verstr)
		if err != nil {
			continue
		}
		if version < 14 {
			// Versions before 1.14 are unsupported.
			// TODO: Adjust minimum allowed version to 1.7.x once support
			// is added.
			continue
		}
		options, err := os.ReadFile(gameDir + "/options.txt")
		if err != nil {
			continue
		}
		var resetKey *x11.Key = nil
		for _, line := range strings.Split(string(options), "\n") {
			if !strings.Contains(line, "key_Create New World") {
				continue
			}
			splits := strings.Split(line, ".")
			if len(splits) <= 1 {
				break
			}
			key := splits[len(splits)-1]
			if key == "unknown" {
				break
			}
			resetKey = &x11.Key{}
			err := resetKey.UnmarshalTOML(key)
			if err != nil {
				return nil, fmt.Errorf("unable to determine atum reset key: %s", err)
			}
		}
		if resetKey == nil {
			resetKey = &x11.Key{
				Code: x11.KeyF6,
			}
		}
		instance := Instance{
			Id:       id,
			Pid:      pid,
			Wid:      win,
			Dir:      gameDir,
			Version:  version,
			ResetKey: *resetKey,
		}
		instances = append(instances, instance)
	}

	// Sort instances.
	if len(instances) == 0 {
		return nil, errors.New("no instances found")
	}
	sort.Slice(instances, func(i, j int) bool {
		return instances[i].Id < instances[j].Id
	})
	if instances[0].Id != 0 {
		return nil, errors.New("no instance with id 0")
	}
	for i, v := range instances {
		if v.Id != i {
			return nil, errors.New("instances do not have sequential IDs")
		}
	}
	return instances, nil
}
