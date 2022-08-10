package manager

import (
	"errors"
	"fmt"
	"runtime"

	"github.com/woofdoggo/resetti/internal/cfg"
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
			set.Set(idx)
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

type affinitySets struct {
	Idle   unix.CPUSet
	Slow   unix.CPUSet
	Fast   unix.CPUSet
	Active unix.CPUSet
	slow   unix.CPUSet
	fast   unix.CPUSet
}

func (a *affinitySets) reallocate(active bool) {
	conf := cfg.GetConfig()
	cpus := runtime.NumCPU()
	switch conf.Affinity.Reallocate {
	case "none":
		return
	case "genfast":
		a.Fast = a.fast
		if active {
			for i := 0; i < cpus; i++ {
				if a.Active.IsSet(i) {
					a.Fast.Set(i)
				}
			}
		}
	case "genslow":
		a.Slow = a.slow
		if active {
			for i := 0; i < cpus; i++ {
				if a.Active.IsSet(i) {
					a.Slow.Set(i)
				}
			}
		}
		a.Fast = a.fast
		if active {
			for i := 0; i < cpus; i++ {
				if a.Active.IsSet(i) {
					a.Fast.Set(i)
				}
			}
		}
	}
}

func newAffinitySet() affinitySets {
	sets := affinitySets{}
	conf := cfg.GetConfig()
	i := 0
	sets.Idle = makeCpuSet(&i, conf.Affinity.CpuIdle)
	sets.Slow = makeCpuSet(&i, conf.Affinity.CpuSlow)
	sets.slow = sets.Slow
	i -= conf.Affinity.CpuSlow
	sets.Fast = makeCpuSet(&i, conf.Affinity.CpuFast+conf.Affinity.CpuSlow)
	sets.fast = sets.Fast
	sets.Active = makeCpuSet(&i, conf.Affinity.CpuActive)
	return sets
}

func makeCpuSet(start *int, count int) unix.CPUSet {
	set := unix.CPUSet{}
	for i := 0; i < count; i++ {
		set.Set(i + *start)
	}
	*start += count
	return set
}
