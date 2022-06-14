package manager

import (
	"errors"
	"fmt"
	"runtime"

	"github.com/woofdoggo/resetti/internal/mc"
	"golang.org/x/sys/unix"
)

var ErrLowCPU = errors.New("not enough CPUs to put each instance on its own core")

// setAffinity sets the process affinity of each provided instances based
// on the user's settings.
func setAffinity(instances []mc.Instance, conf string) error {
	var alternate bool
	switch conf {
	case "alternate":
		alternate = true
	case "sequence":
		alternate = false
	case "":
		return nil
	default:
		return fmt.Errorf("invalid affinity config: %s", conf)
	}
	cpus := runtime.NumCPU()
	if alternate && cpus/2 < len(instances) {
		return ErrLowCPU
	}
	if cpus < len(instances) {
		return ErrLowCPU
	}
	for idx, inst := range instances {
		set := unix.CPUSet{}
		if alternate {
			set.Set(idx * 2)
		} else {
			set.Set(idx)
		}
		err := unix.SchedSetaffinity(int(inst.Pid), &set)
		if err != nil {
			return err
		}
	}
	return nil
}
