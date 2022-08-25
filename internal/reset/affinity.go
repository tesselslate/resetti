package reset

import (
	"github.com/woofdoggo/resetti/internal/cfg"
	"golang.org/x/sys/unix"
)

// setSimpleAffinity sets the CPU affinity of each instance when using a
// simple affinity setting.
func setSimpleAffinity(c cfg.Profile, instances []Instance) error {
	switch c.General.Affinity {
	case "sequence":
		for idx, inst := range instances {
			set := unix.CPUSet{}
			set.Set(idx)
			err := unix.SchedSetaffinity(int(inst.Pid), &set)
			if err != nil {
				return err
			}
		}
	case "alternate":
		for idx, inst := range instances {
			set := unix.CPUSet{}
			set.Set(idx * 2)
			err := unix.SchedSetaffinity(int(inst.Pid), &set)
			if err != nil {
				return err
			}
		}
	case "double":
		for idx, inst := range instances {
			set := unix.CPUSet{}
			set.Set(idx * 2)
			set.Set(idx*2 + 1)
			err := unix.SchedSetaffinity(int(inst.Pid), &set)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// makeCpuSet returns a CPUSet where the given number of CPUs are activated.
func makeCpuSet(num int) unix.CPUSet {
	set := unix.CPUSet{}
	for i := 0; i < num; i++ {
		set.Set(i)
	}
	return set
}
