package reset

import (
	_ "embed"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
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
	windows, err := c.GetWindowList()
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
	log.Printf("Root: %d\n", x.GetRootWindow())
	log.Println("WM properties:")
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
func runCgroupScript(conf *cfg.Profile) error {
	// Check if the script needs to be run. Start by making sure the cgroup
	// folders exist.
	baseGroups := []string{
		"idle",
		"low",
		"mid",
		"high",
		"active",
	}
	var checkFolders []string
	if !conf.AdvancedWall.CcxSplit {
		checkFolders = baseGroups
	} else {
		checkFolders = make([]string, 0, len(baseGroups)*2)
		for _, v := range baseGroups {
			checkFolders = append(checkFolders, v+"0")
			checkFolders = append(checkFolders, v+"1")
		}
	}
	needsRun := false
	for _, folder := range checkFolders {
		stat, err := os.Stat("/sys/fs/cgroup/resetti/" + folder)
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
	path, err := cfg.GetFolder()
	if err != nil {
		return errors.Wrap(err, "get config folder")
	}
	path += "/cgroup_setup.sh"
	if err = os.WriteFile(path, cgroup_script, 0644); err != nil {
		return errors.Wrap(err, "write cgroup script")
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
	subgroups := strings.Join(checkFolders, " ")
	cmd := exec.Command(suidBin, "sh", path, subgroups)
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

// writeCgroups writes the CPU set for each cgroup.
func writeCgroups(conf *cfg.Profile) error {
	cgroups := []string{
		"idle",
		"low",
		"mid",
		"high",
		"active",
	}
	aff := []int{
		conf.AdvancedWall.CpusIdle,
		conf.AdvancedWall.CpusLow,
		conf.AdvancedWall.CpusMid,
		conf.AdvancedWall.CpusHigh,
		conf.AdvancedWall.CpusActive,
	}

	// Set the available CPUs for each cgroup.
	if !conf.AdvancedWall.CcxSplit {
		for i, cgroup := range cgroups {
			path := fmt.Sprintf("/sys/fs/cgroup/resetti/%s/cpuset.cpus", cgroup)
			d := fmt.Sprintf("0-%d", aff[i]-1)
			err := os.WriteFile(path, []byte(d), 0644)
			if err != nil {
				return errors.Wrapf(err, "write cgroup %s", cgroup)
			}
		}
	} else {
		stripe := func(start int) string {
			list := make([]string, 0)
			for start < runtime.NumCPU() {
				list = append(list, strconv.Itoa(start))
				start += 2
			}
			return strings.Join(list, ",")
		}

		for _, cgroup := range cgroups {
			path := fmt.Sprintf("/sys/fs/cgroup/resetti/%s0/cpuset.cpus", cgroup)
			err := os.WriteFile(path, []byte(stripe(0)), 0644)
			if err != nil {
				return errors.Wrapf(err, "write cgroup %s0", cgroup)
			}

			path = fmt.Sprintf("/sys/fs/cgroup/resetti/%s1/cpuset.cpus", cgroup)
			err = os.WriteFile(path, []byte(stripe(1)), 0644)
			if err != nil {
				return errors.Wrapf(err, "write cgroup %s1", cgroup)
			}
		}
	}
	return nil
}
