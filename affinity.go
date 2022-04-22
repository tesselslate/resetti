package main

// #include "affinity.c"
// #cgo LDFLAGS: -Wl,--allow-multiple-definition
import "C"
import "fmt"

func GetProcAffinity(pid uint64) (uint64, error) {
	res := C.get_proc_affinity(C.ulong(pid))

	if res == 0 {
		str := C.GoString(C.get_error())
		return 0, fmt.Errorf(str)
	}

	return uint64(res), nil
}

func GetProcCount() uint64 {
	return uint64(C.get_proc_count())
}

func SetProcAffinity(pid uint64, cpus uint64) error {
	res := C.set_proc_affinity(C.ulong(pid), C.ulong(cpus))

	if res == -1 {
		str := C.GoString(C.get_error())
		return fmt.Errorf(str)
	}

	return nil
}
