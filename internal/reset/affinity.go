package reset

import (
	"golang.org/x/sys/unix"
)

// makeCpuSet returns a CPUSet where the given number of CPUs are activated.
func makeCpuSet(num int) unix.CPUSet {
	set := unix.CPUSet{}
	for i := 0; i < num; i++ {
		set.Set(i)
	}
	return set
}
