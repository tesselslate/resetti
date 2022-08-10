package manager

import (
	"errors"
	"fmt"
	"runtime"

	"github.com/woofdoggo/resetti/internal/mc"
	"golang.org/x/sys/unix"
)

var ErrLowCPU = errors.New("not enough CPUs for this affinity setting")

// setAffinity sets the process affinity of each provided instances based
// on the user's settings.
func setAffinity(instances []mc.Instance, conf string) error {
	cpus := runtime.NumCPU()
	switch conf {
	case "alternate":
		if len(instances) > cpus/2 {
			return ErrLowCPU
		}
		for idx, inst := range instances {
			set := unix.CPUSet{}
			set.Set(idx * 2)
			err := unix.SchedSetaffinity(int(inst.Pid), &set)
			if err != nil {
				return err
			}
		}
	case "sequence":
		if len(instances) > cpus {
			return ErrLowCPU
		}
		for idx, inst := range instances {
			set := unix.CPUSet{}
			set.Set(idx * 2)
			err := unix.SchedSetaffinity(int(inst.Pid), &set)
			if err != nil {
				return err
			}
		}
	case "double":
		if len(instances) > cpus/2 {
			return ErrLowCPU
		}
		for idx, inst := range instances {
			set := unix.CPUSet{}
			set.Set(idx * 2)
			set.Set(idx*2 + 1)
			err := unix.SchedSetaffinity(int(inst.Pid), &set)
			if err != nil {
				return err
			}
		}
	case "":
		return nil
	default:
		return fmt.Errorf("invalid affinity config: %s", conf)
	}
	return nil
}
