// Package affinity provides simple functions for querying and modifying
// the affinity of a process. This can give a minor performance boost
// when running multiple instances by assigning specific instances to
// certain CPU cores.
package affinity

// #include "affinity.c"
// #cgo LDFLAGS: -Wl,--allow-multiple-definition
import "C"
import "fmt"

// GetProcAffinity returns the affinity/CPU mask of the given process.
func GetProcAffinity(pid uint64) (uint64, error) {
	res := C.get_proc_affinity(C.ulong(pid))

	if res == 0 {
		str := C.GoString(C.get_error())
		return 0, fmt.Errorf(str)
	}

	return uint64(res), nil
}

// GetProcCount returns the number of available CPU cores.
func GetProcCount() uint64 {
	return uint64(C.get_proc_count())
}

// SetProcAffinity sets the affinity/CPU mask of the given process.
func SetProcAffinity(pid uint64, cpus uint64) error {
	res := C.set_proc_affinity(C.ulong(pid), C.ulong(cpus))

	if res == -1 {
		str := C.GoString(C.get_error())
		return fmt.Errorf(str)
	}

	return nil
}
