// This program serves as a simple benchmarker for your world loading speeds.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/x11"
)

type Options struct {
	Affinity      string
	Fancy         bool
	InstanceCount int
	ResetCount    int
	ResetAt       int
	PauseAfter    bool
	Pprof         bool
}

func main() {
	opts := Options{}
	flag.StringVar(
		&opts.Affinity,
		"affinity",
		"none",
		"The affinity type to use (sequence, ccx, none).",
	)
	flag.BoolVar(
		&opts.Fancy,
		"fancy",
		false,
		"Show a fancy progress display or plain text output.",
	)
	flag.IntVar(
		&opts.InstanceCount,
		"instances",
		0,
		"The number of instances to use. Set to 0 to use all instances.",
	)
	flag.IntVar(
		&opts.ResetCount,
		"reset-count",
		2000,
		"The number of resets to perform.",
	)
	flag.IntVar(
		&opts.ResetAt,
		"reset-percent",
		0,
		"What percent to reset instances at. 0 for preview, 100 for full load.",
	)
	flag.BoolVar(
		&opts.PauseAfter,
		"pause-after",
		true,
		"Whether or not to pause all instances before exiting.",
	)
	flag.BoolVar(
		&opts.Pprof,
		"profile",
		false,
		"Whether or not to collect profiling information.",
	)
	flag.Parse()
	os.Exit(run(opts))
}

func run(opts Options) int {
	// Setup
	x, err := x11.NewClient()
	if err != nil {
		log.Fatalln(err)
	}
	instances, err := mc.FindInstances(&x)
	if err != nil {
		log.Fatalln(err)
	}
	if opts.InstanceCount != 0 && len(instances) < opts.InstanceCount {
		if len(instances) < opts.InstanceCount {
			log.Fatalf("Found %d of %d instances\n", len(instances), opts.InstanceCount)
		}
		instances = instances[:opts.InstanceCount]
	}
	if err := setupCgroups(opts.Affinity, instances); err != nil {
		log.Fatalln(err)
	}
	evtch := make(chan mc.Update, 16*len(instances))
	errch := make(chan error, len(instances))
	readers := make([]mc.StateReader, 0, len(instances))
	states := make([]mc.State, 0, len(instances))
	for _, inst := range instances {
		reader, state, err := mc.CreateStateReader(inst)
		if err != nil {
			log.Fatalln(err)
		}
		readers = append(readers, reader)
		states = append(states, state)
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatalln(err)
	}
	for _, reader := range readers {
		if err := watcher.Add(reader.Path()); err != nil {
			log.Fatalln(err)
		}
	}
	go readStates(readers, watcher, states, evtch, errch)

	// Profiling
	if opts.Pprof {
		profile, err := os.OpenFile(fmt.Sprintf("/tmp/resetti-prof-%d", rand.Uint64()), os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			log.Fatalln(err)
		}
		if err = pprof.StartCPUProfile(profile); err != nil {
			log.Fatalln(err)
		}
		defer func() {
			pprof.StopCPUProfile()
			fmt.Println("Wrote profile:", profile.Name())
			_ = profile.Close()
		}()
	}

	// Log output
	if opts.Fancy {
		fh, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0644)
		if err != nil {
			log.Fatalln(err)
		}
		log.SetOutput(fh)
		defer func() {
			_ = fh.Close()
			// Print a newline since the progress bar does not.
			fmt.Println()
		}()
	} else {
		log.SetOutput(os.Stderr)
	}

	// Warmup instances
	for _, instance := range instances {
		if err := x.Click(instance.Wid); err != nil {
			log.Fatalln(err)
		}
	}
	time.Sleep(100 * time.Millisecond)
	for _, instance := range instances {
		x.SendKeyPress(instance.ResetKey.Code, instance.Wid)
	}

	resets := 0
	var start time.Time
	printProgress := func(instance int) {
		since := time.Since(start)
		if !opts.Fancy {
			fmt.Printf("%d\t%d\t%d\n", resets, instance, since.Milliseconds())
			return
		}
		rps := float64(resets) / since.Seconds()
		rem := opts.ResetCount - resets
		est := float64(rem) / rps
		fmt.Printf(
			"\r\x1B[K (%d/%d)\t%.1f / %.1fs\t(%.1f/s)",
			resets,
			opts.ResetCount,
			since.Seconds(),
			since.Seconds()+est,
			rps,
		)
	}

	// Main loop
	start = time.Now()
	for resets != opts.ResetCount {
		select {
		case update := <-evtch:
			last := states[update.Id]
			next := update.State
			states[update.Id] = update.State
			switch opts.ResetAt {
			case 100:
				if last.Type != mc.StIdle && next.Type == mc.StIdle {
					x.SendKeyPress(
						instances[update.Id].ResetKey.Code,
						instances[update.Id].Wid,
					)
					resets += 1
					printProgress(update.Id)
				}
			case 0:
				if last.Type != mc.StPreview && next.Type == mc.StPreview {
					x.SendKeyPress(
						instances[update.Id].PreviewKey.Code,
						instances[update.Id].Wid,
					)
					resets += 1
					printProgress(update.Id)
				}
			default:
				if next.Type != mc.StDirt && next.Progress >= opts.ResetAt && last.Progress < opts.ResetAt {
					x.SendKeyPress(
						instances[update.Id].ResetKey.Code,
						instances[update.Id].Wid,
					)
					resets += 1
					printProgress(update.Id)
				}
			}
		case err := <-errch:
			log.Fatalln(err)
		}
	}
	if !opts.PauseAfter {
		return 0
	}

	// Cleanup
	paused := 0
	for paused != len(instances) {
		select {
		case update := <-evtch:
			last := states[update.Id].Type
			next := update.State.Type
			states[update.Id] = update.State
			if last != mc.StIdle && next == mc.StIdle {
				time.Sleep(50 * time.Millisecond)
				x.SendKeyPress(
					x11.KeyEsc.Code,
					instances[update.Id].Wid,
				)
				paused += 1
			}
		case err := <-errch:
			log.Fatalln(err)
		}
	}
	return 0
}

// readStates begins reading the states of each instance.
// TODO: just unduplicate this whole thing. make some sort of state reader manager
// in the mc package and use that in the instance manager
func readStates(readers []mc.StateReader, watcher *fsnotify.Watcher, statesOrig []mc.State, evtch chan<- mc.Update, errch chan<- error) {
	states := make([]mc.State, len(statesOrig))
	copy(states, statesOrig)
	paths := make(map[string]int)
	for idx, reader := range readers {
		paths[reader.Path()] = idx
	}
	for {
		select {
		case evt, ok := <-watcher.Events:
			if !ok {
				errch <- errors.New("watcher events closed")
				return
			}
			id := paths[evt.Name]
			switch evt.Op {
			case fsnotify.Write:
				// Process any updates to the state file.
				state, updated, err := readers[id].Process()
				if err != nil {
					log.Printf("process log (%d) failed: %s", id, err)
					continue
				}
				if !updated {
					continue
				}

				// Only modify the fields that the state reader knows about.
				states[id].Type = state.Type
				states[id].Progress = state.Progress
				states[id].Menu = state.Menu

				// The stWorld state should only ever be handled internally.
				// Update it to the appropriate public state before notifying
				// the frontend.
				if state.Type == 5 {
					// TODO: find a way to make stWorld public more nicely. see above TODO
					states[id].Type = mc.StIdle
				}
				evtch <- mc.Update{State: states[id], Id: id}
			default:
				err := readers[id].ProcessEvent(evt.Op)
				if err != nil {
					errch <- fmt.Errorf("process event (%d) failed: %w", id, err)
					return
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				errch <- fmt.Errorf("watcher died: %w", err)
				return
			}
			log.Printf("Non-fatal watcher error: %s\n", err)
		}
	}
}

// setupCgroups performs any necessary setup for the affinity cgroups.
func setupCgroups(affinity string, instances []mc.InstanceInfo) error {
	makeGroups := func(groups ...string) error {
		needsRun := false
		for _, dir := range groups {
			stat, err := os.Stat("/sys/fs/cgroup/resetti/" + dir)
			if err != nil || !stat.IsDir() {
				needsRun = true
				break
			}
		}
		if !needsRun {
			return nil
		}

		// Determine the user's suid binary.
		suidBin, ok := os.LookupEnv("RESETTI_SUID_BINARY")
		if !ok {
			// TODO: More? pkexec, etc
			options := []string{"sudo", "doas"}
			for _, option := range options {
				cmd := exec.Command(option)
				err := cmd.Run()
				if !errors.Is(err, exec.ErrNotFound) {
					suidBin = option
					break
				}
			}
		}
		if suidBin == "" {
			return errors.New("no suid binary found")
		}

		// Run the script.
		subgroups := strings.Join(groups, " ")
		cmd := exec.Command(suidBin, "sh", "groups.sh", subgroups)
		err := cmd.Run()
		return fmt.Errorf("run cgroup script: %w", err)
	}
	writeCpuSet := func(cgroup string, cpus []int) error {
		list := make([]string, 0, len(cpus))
		for _, cpu := range cpus {
			list = append(list, strconv.Itoa(cpu))
		}
		return os.WriteFile(
			fmt.Sprintf("/sys/fs/cgroup/resetti/%s/cpuset.cpus", cgroup),
			[]byte(strings.Join(list, ",")),
			0644,
		)
	}

	switch affinity {
	case "sequence":
		groups := make([]string, 0)
		if len(instances)*2 > runtime.NumCPU() {
			return errors.New("not enough cpus")
		}
		for i := 0; i < len(instances); i += 1 {
			groups = append(groups, fmt.Sprintf("inst%d", i))
		}
		if err := makeGroups(groups...); err != nil {
			return err
		}
		for i, group := range groups {
			if err := writeCpuSet(group, []int{i, i + runtime.NumCPU()/2}); err != nil {
				return err
			}
		}
		for i := 0; i < len(instances); i += 1 {
			err := os.WriteFile(
				fmt.Sprintf("/sys/fs/cgroup/resetti/inst%d/cgroup.procs", i),
				[]byte(strconv.Itoa(int(instances[i].Pid))),
				0644,
			)
			if err != nil {
				return err
			}
		}
		return nil
	case "ccx":
		if err := makeGroups("ccx0", "ccx1"); err != nil {
			return err
		}
		ccx0, ccx1 := make([]int, 0), make([]int, 0)
		for i := 0; i < runtime.NumCPU()/2; i += 1 {
			ccx0 = append(ccx0, i)
			ccx1 = append(ccx1, i+runtime.NumCPU()/2)
		}
		if err := writeCpuSet("ccx0", ccx0); err != nil {
			return err
		}
		if err := writeCpuSet("ccx1", ccx1); err != nil {
			return err
		}

		for i := 0; i < len(instances); i += 1 {
			var group string
			if i < len(instances)/2 {
				group = "ccx0"
			} else {
				group = "ccx1"
			}
			err := os.WriteFile(
				fmt.Sprintf("/sys/fs/cgroup/resetti/%s/cgroup.procs", group),
				[]byte(strconv.Itoa(int(instances[i].Pid))),
				0644,
			)
			if err != nil {
				return err
			}
		}
		return nil
	case "none":
		return nil
	default:
		return errors.New("invalid affinity")
	}
}
