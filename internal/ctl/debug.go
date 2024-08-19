package ctl

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/tesselslate/resetti/internal/log"
)

// Instance state names
var stateNames = [...]string{
	"menu",
	"dirt",
	"preview",
	"idle",
	"ingame",
	"world",
}

// debugLogger can be used to print out debugging information and various
// statistics about resetti's operation.
type debugLogger struct {
	host *Controller
}

// Run starts reading stdin and printing debug information as the user requests.
func (d *debugLogger) Run() {
	reader := bufio.NewReader(os.Stdin)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				continue
			}
			log.Error("debugLogger.readStdin failed: %s\n", err)
			continue
		}
		switch strings.TrimSuffix(line, "\n") {
		case "a", "all":
			d.printAll()
		case "f", "frontend":
			d.printFrontend()
		case "g", "gc":
			d.printGc()
		case "i", "input":
			d.printInput()
		}
	}
}

func (d *debugLogger) printAll() {
	d.printFrontend()
	d.printGc()
	d.printInput()
}

func (d *debugLogger) printFrontend() {
	s := &strings.Builder{}
	s.WriteString("\nFrontend: \n")
	log.Debug(s.String())
}

func (d *debugLogger) printGc() {
	mem := runtime.MemStats{}
	runtime.ReadMemStats(&mem)
	s := &strings.Builder{}
	s.WriteString("\nGC: \n")
	fmt.Fprintf(s, "Heap size: %.2f MB\n", float64(mem.Sys)/1e7)
	fmt.Fprintf(s, "Live objects: %d\n", mem.HeapObjects)
	fmt.Fprintf(s, "Mallocs/frees: %d/%d\n", mem.Mallocs, mem.Frees)
	fmt.Fprintf(s, "Total alloc: %.2f MB\n", float64(mem.TotalAlloc)/1e7)
	fmt.Fprintf(s, "Current alloc: %.2f MB\n", float64(mem.HeapAlloc)/1e7)
	fmt.Fprintf(s, "Pause time: %.4f ms\n", float64(mem.PauseTotalNs)/1e7)
	fmt.Fprintf(s, "GC time: %.4f%%\n", mem.GCCPUFraction)
	fmt.Fprintf(s, "GC cycles: %d", mem.NumGC)
	log.Debug(s.String())
}

func (d *debugLogger) printInput() {
	s := &strings.Builder{}
	s.WriteString("\nInput: \n")
	fmt.Fprintf(s, "Last binds: %+v\n", d.host.inputMgr.lastBinds)
	fmt.Fprintf(s, "Last fail window: %d", d.host.inputMgr.lastFailWindow)
	log.Debug(s.String())
}
