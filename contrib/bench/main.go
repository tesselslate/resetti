// This program serves as a simple benchmarker for your world loading speeds.
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/x11"
)

type Options struct {
	Affinity       string
	InstanceCount  int
	ResetCount     int
	ResetOnPreview bool
	PauseAfter     bool
	Pprof          bool
}

func main() {
	opts := Options{}
	flag.StringVar(
		&opts.Affinity,
		"affinity",
		"none",
		"The affinity type to use (sequence, ccx, none).",
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
	flag.BoolVar(
		&opts.ResetOnPreview,
		"reset-preview",
		true,
		"Whether to reset instances as soon as they reach the preview or finish generating.",
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
		fmt.Println(err)
		return 1
	}
	instances, err := mc.FindInstances(&x)
	if err != nil {
		fmt.Println(err)
		return 1
	}
	if opts.InstanceCount != 0 && len(instances) < opts.InstanceCount {
		if len(instances) < opts.InstanceCount {
			fmt.Printf("Found %d of %d instances\n", len(instances), opts.InstanceCount)
			return 1
		}
		instances = instances[:opts.InstanceCount]
	}
	if err := setupCgroups(opts.Affinity, instances); err != nil {
		fmt.Println(err)
		return 1
	}
	evtch := make(chan mc.Update, 16*len(instances))
	errch := make(chan error, len(instances))
	reader, states, err := mc.NewLogReader(instances)
	if err != nil {
		fmt.Println(err)
		return 1
	}
	go reader.Run(errch, evtch)

	// Profiling
	if opts.Pprof {
		profile, err := os.OpenFile(fmt.Sprintf("/tmp/resetti-prof-%d", rand.Uint64()), os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			fmt.Println(err)
			return 1
		}
		if err = pprof.StartCPUProfile(profile); err != nil {
			fmt.Println(err)
			return 1
		}
		defer func() {
			pprof.StopCPUProfile()
			fmt.Println("Wrote profile:", profile.Name())
			_ = profile.Close()
		}()
	}

	// Warmup instances
	for _, instance := range instances {
		if err := x.Click(instance.Wid); err != nil {
			fmt.Println(err)
			return 1
		}
	}
	time.Sleep(100 * time.Millisecond)
	for _, instance := range instances {
		x.SendKeyPress(instance.ResetKey.Code, instance.Wid, x.GetCurrentTime())
	}

	// Main loop
	resets := 0
	start := time.Now()
	for resets != opts.ResetCount {
		select {
		case update := <-evtch:
			last := states[update.Id].State
			next := update.State.State
			states[update.Id] = update.State
			if opts.ResetOnPreview && last != mc.StPreview && next == mc.StPreview {
				x.SendKeyPress(
					instances[update.Id].PreviewKey.Code,
					instances[update.Id].Wid,
					x.GetCurrentTime(),
				)
				resets += 1
				fmt.Println(resets, time.Since(start))
			} else if last != mc.StIdle && next == mc.StIdle {
				x.SendKeyPress(
					instances[update.Id].ResetKey.Code,
					instances[update.Id].Wid,
					x.GetCurrentTime(),
				)
				resets += 1
				fmt.Println(resets, time.Since(start))
			}
		case err := <-errch:
			fmt.Println(err)
			return 1
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
			last := states[update.Id].State
			next := update.State.State
			states[update.Id] = update.State
			if last != mc.StIdle && next == mc.StIdle {
				time.Sleep(50 * time.Millisecond)
				x.SendKeyPress(
					x11.KeyEsc,
					instances[update.Id].Wid,
					x.GetCurrentTime(),
				)
				paused += 1
			}
		case err := <-errch:
			fmt.Println(err)
			return 1
		}
	}
	return 0
}

func setupCgroups(affinity string, instances []mc.InstanceInfo) error {
	makeGroups := func(groups ...string) error {
		needsRun := false
		for _, folder := range groups {
			stat, err := os.Stat("/sys/fs/cgroup/resetti/" + folder)
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
		return errors.Wrap(cmd.Run(), "run cgroup script")
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
