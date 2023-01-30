package reset

import (
	_ "embed"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/jezek/xgb/xproto"
	"github.com/pkg/errors"
	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/obs"
	"github.com/woofdoggo/resetti/internal/x11"
)

//go:embed scripts/cgroup_setup.sh
var cgroup_script []byte

// findProjector finds the OBS wall projector (if open.)
func findProjector(c *x11.Client) (xproto.Window, error) {
	windows, err := c.GetAllWindows()
	if err != nil {
		return 0, err
	}
	for _, win := range windows {
		title, err := c.GetWindowTitle(win)
		if err != nil {
			continue
		}
		if strings.Contains(title, "Projector (Scene) - Wall") {
			return win, nil
		}
	}
	return 0, errors.New("no projector")
}

// getWallSize returns the dimensions of the user's wall.
func getWallSize(o *obs.Client, instances int) (uint16, uint16, error) {
	appendUnique := func(slice []float64, item float64) []float64 {
		for _, v := range slice {
			if item == v {
				return slice
			}
		}
		return append(slice, item)
	}
	xs, ys := make([]float64, 0), make([]float64, 0)
	for i := 0; i < instances; i++ {
		x, y, _, _, err := o.GetSceneItemTransform(
			"Wall",
			fmt.Sprintf("MC %d", i+1),
		)
		if err != nil {
			return 0, 0, err
		}
		xs = appendUnique(xs, x)
		ys = appendUnique(ys, y)
	}
	return uint16(len(xs)), uint16(len(ys)), nil
}

// printDebugInfo prints some debug information to the log.
func printDebugInfo(x *x11.Client, instances []mc.Instance) {
	log.Printf("Running %d instances\n", len(instances))
	log.Printf("Root: %d\n", x.RootWindow())
	log.Println("WM properties:")
	log.Printf("_NET_WM_NAME: %s", x.GetWmName())
	supported, err := x.GetWmSupported()
	if err != nil {
		log.Printf("Failed to get _NET_SUPPORTED: %s\n", err)
	} else {
		log.Printf("_NET_SUPPORTED: %s", strings.Join(supported, ", "))
	}
	for id, inst := range instances {
		log.Printf(
			"Instance %d, wid %d, pid %d version %d\n",
			id,
			inst.Wid,
			inst.Pid,
			inst.Version,
		)
		dir, err := os.ReadDir(inst.Dir + "/mods")
		if err != nil {
			log.Printf("Failed to get mods: %s\n", err)
			continue
		}
		for _, entry := range dir {
			name := entry.Name()
			atum := strings.Contains(name, "atum")
			fastreset := strings.Contains(name, "fast-reset")
			worldpreview := strings.Contains(name, "worldpreview")
			if atum || fastreset || worldpreview {
				log.Println(name)
			}
		}
	}
}

// runCgroupScript runs the cgroup setup script.
func runCgroupScript() error {
	// Check if the script needs to be run. Start by making sure the cgroup
	// folders exist.
	checkFolders := []string{
		"/sys/fs/cgroup/resetti",
		"/sys/fs/cgroup/resetti/idle",
		"/sys/fs/cgroup/resetti/low",
		"/sys/fs/cgroup/resetti/mid",
		"/sys/fs/cgroup/resetti/high",
		"/sys/fs/cgroup/resetti/active",
	}
	needsRun := false
	for _, folder := range checkFolders {
		stat, err := os.Stat(folder)
		if err != nil || !stat.IsDir() {
			needsRun = true
			break
		}
	}
	if !needsRun {
		log.Println("Skipped cgroup script.")
		return nil
	}

	// Check for the script's existence.
	// TODO: Notify the user if the script is not a match (e.g. needs to
	// be updated)
	path, err := cfg.GetFolder()
	if err != nil {
		return errors.Wrap(err, "get config folder")
	}
	path += "/cgroup_setup.sh"
	if _, err := os.Stat(path); err != nil {
		if !os.IsNotExist(err) {
			return errors.Wrap(err, "find cgroup script")
		}
		if err = os.WriteFile(path, cgroup_script, 0644); err != nil {
			return errors.Wrap(err, "write cgroup script")
		}
	}

	// Determine the user's suid binary.
	suidBin, ok := os.LookupEnv("RESETTI_SUID_BINARY")
	if !ok {
		// TODO: More? pkexec, etc
		options := []string{"sudo", "doas"}
		for _, option := range options {
			cmd := exec.Command(option)
			err = cmd.Run()
			if !errors.Is(err, exec.ErrNotFound) {
				suidBin = option
				break
			}
		}
	}
	if suidBin == "" {
		return errors.Wrap(err, "no suid binary found")
	}

	// Run the script.
	cmd := exec.Command(suidBin, "sh", path)
	return errors.Wrap(cmd.Run(), "run cgroup script")
}

// runHook runs the given command.
func runHook(str string) {
	if str == "" {
		return
	}
	splits := strings.Split(str, " ")
	var cmd *exec.Cmd
	if len(splits) == 1 {
		cmd = exec.Command(splits[0])
	} else {
		cmd = exec.Command(splits[0], splits[1:]...)
	}
	err := cmd.Run()
	if err != nil {
		log.Printf("runCmd err: %s\n", err)
		return
	}
}

// setSources sets the correct window captures for each Minecraft source.
func setSources(o *obs.Client, instances []mc.Instance, usingMovingWall bool) error {
	for i, v := range instances {
		err := o.SetSourceSettings(
			fmt.Sprintf("MC %d", i+1),
			obs.StringMap{
				"capture_window": strconv.Itoa(int(v.Wid)),
			},
			true,
		)
		if err != nil {
			return err
		}
		if usingMovingWall {
			err = o.SetSourceSettings(
				fmt.Sprintf("MC %d LockedView", i+1),
				obs.StringMap{
					"capture_window": strconv.Itoa(int(v.Wid)),
				},
				true,
			)
			if err != nil {
				return err
			}
			err = o.SetSourceSettings(
				fmt.Sprintf("MC %d FullView", i+1),
				obs.StringMap{
					"capture_window": strconv.Itoa(int(v.Wid)),
				},
				true,
			)
			if err != nil {
				return err
			}
			err = o.SetSourceSettings(
				fmt.Sprintf("MC %d LoadingView", i+1),
				obs.StringMap{
					"capture_window": strconv.Itoa(int(v.Wid)),
				},
				true,
			)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
