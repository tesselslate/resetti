// This program serves as a simple benchmarker for your world loading speeds.
package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/tesselslate/resetti/internal/cfg"
	"github.com/tesselslate/resetti/internal/ctl"
	"github.com/tesselslate/resetti/internal/log"
	"github.com/tesselslate/resetti/internal/mc"
	"github.com/tesselslate/resetti/internal/res"
	"github.com/tesselslate/resetti/internal/x11"
)

type Options struct {
	Affinity      string
	CcxCount      int
	Fancy         bool
	InstanceCount int
	ResetCount    int
	ResetAt       int
	PauseAfter    bool
	Pprof         bool
}

func main() {
	if err := res.WriteResources(); err != nil {
		fmt.Println("Failed to write resources:", err)
	}
	opts := Options{}
	flag.StringVar(
		&opts.Affinity,
		"affinity",
		"none",
		"The affinity type to use (sequence, ccx, none).",
	)
	flag.IntVar(
		&opts.CcxCount,
		"ccx",
		2,
		"The number of CCXs to split across for CCX affinity.",
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
	logger := log.DefaultLogger(log.INFO, "", false)
	defer func() {
		logger.Close()
	}()
	// Setup
	x, err := x11.NewClient()
	if err != nil {
		log.Error(err.Error())
	}
	instances, err := mc.FindInstances(&x)
	if err != nil {
		log.Error(err.Error())
	}
	if opts.InstanceCount != 0 {
		if len(instances) < opts.InstanceCount {
			log.Error("Found %d of %d instances\n", len(instances), opts.InstanceCount)
		}
		instances = instances[:opts.InstanceCount]
	}
	evtch := make(chan mc.Update, 16*len(instances))
	errch := make(chan error, len(instances))
	conf := &cfg.Profile{}
	switch opts.Affinity {
	case "sequence":
		conf.Wall.Enabled = true
		conf.Wall.Perf.Affinity = "sequence"
	case "ccx":
		conf.Wall.Enabled = true
		conf.Wall.Perf.Affinity = "advanced"
		conf.Wall.Perf.Adv.CcxSplit = opts.CcxCount
		conf.Wall.Perf.Adv.CpusHigh = runtime.NumCPU() / opts.CcxCount
	}
	mgr, err := mc.NewManager(instances, conf, &x)
	if err != nil {
		log.Error(err.Error())
	}
	states := mgr.GetStates()
	if opts.Affinity == "sequence" || opts.Affinity == "ccx" {
		_, err := ctl.NewCpuManager(instances, states, conf)
		if err != nil {
			log.Error(err.Error())
		}
	}

	// Profiling
	if opts.Pprof {
		profile, err := os.OpenFile(fmt.Sprintf("/tmp/resetti-prof-%d", rand.Uint64()), os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			log.Error(err.Error())
		}
		if err = pprof.StartCPUProfile(profile); err != nil {
			log.Error(err.Error())
		}
		defer func() {
			pprof.StopCPUProfile()
			fmt.Println("Wrote profile:", profile.Name())
			_ = profile.Close()
		}()
	}

	// Setup log outputs
	if !opts.Fancy {
		logger.SetConsole(false)
	}

	// Warmup instances
	time.Sleep(100 * time.Millisecond)
	for _, instance := range instances {
		x.SendKeyPress(instance.ResetKey, instance.Wid)
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
	go mgr.Run(context.Background(), evtch, errch)
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
						instances[update.Id].ResetKey,
						instances[update.Id].Wid,
					)
					resets += 1
					printProgress(update.Id)
				}
			case 0:
				if last.Type != mc.StPreview && next.Type == mc.StPreview {
					x.SendKeyPress(
						instances[update.Id].PreviewKey,
						instances[update.Id].Wid,
					)
					resets += 1
					printProgress(update.Id)
				}
			default:
				if next.Type != mc.StDirt && next.Progress >= opts.ResetAt && last.Progress < opts.ResetAt {
					x.SendKeyPress(
						instances[update.Id].ResetKey,
						instances[update.Id].Wid,
					)
					resets += 1
					printProgress(update.Id)
				}
			}
		case err := <-errch:
			log.Error(err.Error())
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
				paused += 1
			}
		case err := <-errch:
			log.Error(err.Error())
		}
	}
	return 0
}
