package reset

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/jezek/xgb/xproto"
	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/obs"
	"github.com/woofdoggo/resetti/internal/x11"
	"golang.org/x/sys/unix"
)

// connectObs attempts to connect to OBS.
func connectObs(ctx context.Context, conf cfg.Profile, instanceCount int) (*obs.Client, <-chan error, error) {
	obs := &obs.Client{}
	errch, err := obs.Connect(ctx, fmt.Sprintf("localhost:%d", conf.Obs.Port), conf.Obs.Password)
	if err != nil {
		return nil, nil, err
	}
	err = obs.SetSceneCollection(fmt.Sprintf("resetti - %d multi", instanceCount))
	if err != nil {
		return nil, nil, err
	}
	return obs, errch, nil
}

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
		transform, err := o.GetSceneItemTransform(
			"Wall",
			fmt.Sprintf("MC %d", i+1),
		)
		if err != nil {
			return 0, 0, err
		}
		xs = appendUnique(xs, transform.X)
		ys = appendUnique(ys, transform.Y)
	}
	return uint16(len(xs)), uint16(len(ys)), nil
}

// makeCpuSet returns a unix.CPUSet instance where the first N CPUs are active.
func makeCpuSet(n int) unix.CPUSet {
	set := unix.CPUSet{}
	for i := 0; i < n; i++ {
		set.Set(i)
	}
	return set
}

// printDebugInfo prints some debug information to the log.
func printDebugInfo(x *x11.Client, instances []mc.Instance) {
	log.Printf("Running %d instances\n", len(instances))
	log.Printf("Root: %d\n", x.RootWindow())
	log.Println("WM properties:")
	log.Printf("_NET_WM_NAME: %s", x.GetWmName())
	log.Printf("_NET_SUPPORTED: %s", x.GetWmSupported())
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
func setSources(o *obs.Client, instances []mc.Instance) error {
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
	}
	return nil
}
