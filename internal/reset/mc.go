package reset

import (
	"errors"
	"fmt"
	"os"
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
)

type Instance struct {
	Id      int           // Instance number
	Pid     uint32        // Process ID
	Wid     xproto.Window // Window ID
	Dir     string        // .minecraft directory
	Version int           // Minecraft version
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
	}
	switch i.State {
	case StMenu, StIdle, StIngame:
		return states[i.State]
	default:
		return fmt.Sprintf("%s (%d%%)", states[i.State], i.Progress)
	}
}

// FindInstances returns a list of all running Minecraft instances.
func FindInstances(x *x11.Client) ([]Instance, error) {
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

		// Find game directory
		// NOTE: This method has only been tested with MultiMC and PolyMC.
		// Not guaranteed to work on other launchers (e.g. stock)
		file, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		if err != nil {
			continue
		}
		args := strings.Split(string(file), "\x00")
		gameDir := ""
		for _, arg := range args {
			if !strings.Contains(arg, "-Djava.library.path") {
				continue
			}
			gameDir = strings.ReplaceAll(
				strings.Split(arg, "=")[1],
				"natives", ".minecraft",
			)
			break
		}
		if gameDir == "" {
			continue
		}

		// Get instance ID.
		file, err = os.ReadFile(fmt.Sprintf("%s/instance_num", gameDir))
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
		instance := Instance{
			Id:      id,
			Pid:     pid,
			Wid:     win,
			Dir:     gameDir,
			Version: version,
		}
		instances = append(instances, instance)
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
