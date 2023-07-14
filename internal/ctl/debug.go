package ctl

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/woofdoggo/resetti/internal/log"
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
		case "m", "mgr":
			d.printManager()
		}
	}
}

func (d *debugLogger) printAll() {
	d.printFrontend()
	d.printGc()
	d.printInput()
	d.printManager()
}

func (d *debugLogger) printFrontend() {
	s := &strings.Builder{}
	s.WriteString("\nFrontend: \n")
	switch f := d.host.frontend.(type) {
	case *Wall:
		fmt.Fprintf(s, "Wall size: %dx%d\n", f.wallWidth, f.wallHeight)
		fmt.Fprintf(s, "Active: %d\n", f.active)
		fmt.Fprintf(s, "Locks: %v\n", f.locks)
		fmt.Fprintf(s, "Last mouse ID: %d", f.lastMouseId)
	case *MovingWall:
		fmt.Fprintf(s, "Queue (%d): %v\n", len(f.queue), f.queue)
		fmt.Fprintf(s, "Locks (%d): %v\n", len(f.locks), f.locks)
		fmt.Fprintf(s, "Last hitbox: %v", f.lastHitbox)
	}
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

func (d *debugLogger) printManager() {
	states := d.host.manager.GetStates()
	s := &strings.Builder{}
	s.WriteString("\nManager: \n")
	for i, state := range states {
		fmt.Fprintf(s, "%d\t%s\t%d\t%s", i, stateNames[state.Type], state.Progress, state.LastPreview.Format("15:04:05.9999"))
	}
	log.Debug(s.String())
}
