package ctl

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/log"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/res"
	"golang.org/x/exp/slices"
)

// Affinity arguments
var forceCgroups = slices.Contains(os.Args, "--force-cgroups")

// A list of common setuid binaries. Used for getting root privileges to run
// the cgroup setup script.
var suidBinaries = [...]string{
	"sudo",
	"doas",
	"pkexec",
}

// CpuManager controls how much CPU time is given to each instance bsaed on its state.
type CpuManager interface {
	// Run handles state updates and moves instances between affinity groups.
	Run(context.Context, *sync.WaitGroup)

	// SetPriority sets the priority state of the given instance.
	SetPriority(int, bool)

	// Update updates the affinity state of the given instance as needed based on
	// the state change.
	Update(mc.Update)
}

// cpuTopology contains information about the user's CPU cache topology.
type cpuTopology struct {
	l1 []int // Per-CPU L1 cache ID (for core detection)
	l3 []int // Per-CPU L3 cache ID (for CCX detection)

	Ccx   [][]int
	Cores [][]int

	CpuCount int
	CcxCount int
}

// priorityUpdate contains an update to the priority of a single instance.
type priorityUpdate struct {
	id    int
	state bool
}

// NewCpuManager creates a new cpuManager for the given instances and config
// profile. If necessary, it prompts the user for root permission and runs the
// cgroup creation script.
func NewCpuManager(instances []mc.InstanceInfo, states []mc.State, conf *cfg.Profile) (CpuManager, error) {
	pids := make([]int, 0, len(instances))
	for _, instance := range instances {
		pids = append(pids, int(instance.Pid))
	}
	topo, err := getCpuTopology()
	if err != nil {
		return nil, fmt.Errorf("get cpu topology: %w", err)
	}
	if err := prepareCgroups(conf, &topo, len(instances)); err != nil {
		return nil, err
	}
	switch conf.Wall.Perf.Affinity {
	case "advanced":
		cpuStates := make([]cpuState, 0, len(instances))
		for _, state := range states {
			cpuStates = append(cpuStates, cpuState{state, affIdle, false})
		}
		manager := advancedCpuManager{
			false,
			pids,
			cpuStates,
			conf,
			make(chan priorityUpdate, bufferSize*len(instances)),
			make(chan int, bufferSize*len(instances)),
			make(chan mc.Update, bufferSize*len(instances)),
		}
		manager.initGroups()
		return &manager, nil
	case "sequence":
		mgrStates := make([]mc.State, len(states))
		copy(mgrStates, states)
		manager := sequenceCpuManager{
			false,
			pids,
			mgrStates,
			topo.Cores,
			nil,
			conf,
			make(chan priorityUpdate, bufferSize*len(instances)),
			make(chan mc.Update, bufferSize*len(instances)),
		}
		if err := manager.initGroups(); err != nil {
			return nil, fmt.Errorf("init groups: %w", err)
		}
		return &manager, nil
	default:
		panic("invalid affinity type uncaught")
	}
}

// getCpuTopology returns information about the user's CPU topology.
func getCpuTopology() (cpuTopology, error) {
	// Determine CPU cache topology.
	topo := cpuTopology{CpuCount: cfg.GetCpuCount()}
	for i := 0; i < topo.CpuCount; i += 1 {
		cacheDir := fmt.Sprintf("/sys/devices/system/cpu/cpu%d/cache/", i)
		l1, l3 := -1, -1
		entries, err := os.ReadDir(cacheDir)
		if err != nil {
			return topo, fmt.Errorf("read %s: %w", cacheDir, err)
		}
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), "index") {
				continue
			}
			dir := cacheDir + entry.Name()
			raw, err := os.ReadFile(dir + "/id")
			if err != nil {
				return topo, fmt.Errorf("read cpu%d/cache/%s/id: %w", i, entry.Name(), err)
			}
			id, err := strconv.Atoi(strings.TrimSuffix(string(raw), "\n"))
			if err != nil {
				return topo, fmt.Errorf("convert cpu%d/cache/%s/id: %w", i, entry.Name(), err)
			}
			raw, err = os.ReadFile(dir + "/level")
			if err != nil {
				return topo, fmt.Errorf("read cpu%d/cache/%s/level: %w", i, entry.Name(), err)
			}
			lvl, err := strconv.Atoi(strings.TrimSuffix(string(raw), "\n"))
			if err != nil {
				return topo, fmt.Errorf("convert cpu%d/cache/%s/id: %w", i, entry.Name(), err)
			}
			switch lvl {
			case 1:
				if l1 != -1 && l1 != id {
					return topo, fmt.Errorf("different L1i and L1d ids on cpu %d (%d vs %d)", i, l1, id)
				}
				l1 = id
			case 3:
				l3 = id
			default:
				continue
			}
		}
		if l1 == -1 {
			return topo, fmt.Errorf("no L1 cache for cpu %d", i)
		}
		if l3 == -1 {
			return topo, fmt.Errorf("no L3 cache for cpu %d", i)
		}
		topo.l1 = append(topo.l1, l1)
		topo.l3 = append(topo.l3, l3)
	}

	// Group CPUs into cores and CCXs based on cache topology.
	topo.populateCores()
	topo.populateCcx()
	topo.CcxCount = len(topo.Ccx)

	log.Info("Found CPU topology: %d CPUs, %d cores, %d CCXs", topo.CpuCount, len(topo.Cores), topo.CcxCount)

	return topo, nil
}

// growLength increases the length of the given slice.
func growLength[S []E, E any](slice S, to int) S {
	current := len(slice)
	if current >= to {
		return slice
	}
	return append(slice, make(S, to-current)...)
}

// prepareCgroups prompts the user for root privileges and runs the cgroup
// setup script (if necessary) and assigns the correct CPU sets to each cgroup.
func prepareCgroups(conf *cfg.Profile, topo *cpuTopology, instances int) error {
	// Check if the cgroup setup script needs to be run.
	var shouldExist []string
	switch conf.Wall.Perf.Affinity {
	case "advanced":
		for i := 0; i < conf.Wall.Perf.Adv.CcxSplit; i += 1 {
			for _, name := range baseNames {
				shouldExist = append(shouldExist, name+strconv.Itoa(i))
			}
		}
	case "sequence":
		for i := 0; i < instances; i += 1 {
			shouldExist = append(shouldExist, fmt.Sprintf("inst%d", i))
		}
	}
	shouldRun := false
	for _, group := range shouldExist {
		stat, err := os.Stat("/sys/fs/cgroup/resetti/" + group)
		if err != nil || !stat.IsDir() {
			shouldRun = true
			break
		}
	}

	// Run the cgroup script if necessary.
	if shouldRun || forceCgroups {
		path := res.GetDataDirectory() + res.CgroupScriptPath
		var suidBin string
		for _, bin := range suidBinaries {
			cmd := exec.Command(bin)
			if !errors.Is(cmd.Run(), exec.ErrNotFound) {
				suidBin = bin
				break
			}
		}
		if suidBin == "" {
			return errors.New("no suid binary found")
		}
		cmd := exec.Command(suidBin, "sh", path, strings.Join(shouldExist, " "))
		buf := bytes.Buffer{}
		cmd.Stderr = &buf
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("run cgroup script: %w (%s)", err, strings.TrimSuffix(buf.String(), "\n"))
		}
	}

	// Assign the correct CPU sets to each cgroup.
	err := writeCgroups(conf, topo, instances)
	if err != nil {
		return fmt.Errorf("write cgroups: %w", err)
	}
	return nil
}

// writeCgroups writes the correct CPU sets to each cgroup for the user's
// affinity config.
func writeCgroups(conf *cfg.Profile, topo *cpuTopology, instances int) error {
	switch conf.Wall.Perf.Affinity {
	case "advanced":
		if conf.Wall.Perf.Adv.CcxSplit > topo.CcxCount {
			return fmt.Errorf(
				"mismatched CCX split and CPU topology (%d vs %d)",
				conf.Wall.Perf.Adv.CcxSplit,
				topo.CcxCount,
			)
		}
		if conf.Wall.Perf.Adv.CcxSplit == 0 {
			conf.Wall.Perf.Adv.CcxSplit = topo.CcxCount
		}
		cpus := [...]int{
			conf.Wall.Perf.Adv.CpusIdle,
			conf.Wall.Perf.Adv.CpusLow,
			conf.Wall.Perf.Adv.CpusMid,
			conf.Wall.Perf.Adv.CpusHigh,
			conf.Wall.Perf.Adv.CpusActive,
		}
		for id, name := range baseNames {
			cpuCount := cpus[id]
			for ccx := 0; ccx < topo.CcxCount; ccx += 1 {
				group := name + strconv.Itoa(ccx)
				if err := writeCpuSet(group, topo.Ccx[ccx][:cpuCount]); err != nil {
					return fmt.Errorf("write cpus for %s: %w", group, err)
				}
			}
		}
	case "sequence":
		for i := 0; i < instances; i += 1 {
			if err := writeCpuSet(fmt.Sprintf("inst%d", i), topo.Cores[i]); err != nil {
				return fmt.Errorf("write cpus for inst%d: %w", i, err)
			}
		}
	}
	return nil
}

// writeCpuSet assigns the given CPU set to the given cgroup.
func writeCpuSet(group string, cpus []int) error {
	cpusString := make([]string, 0, len(cpus))
	for _, cpu := range cpus {
		cpusString = append(cpusString, strconv.Itoa(cpu))
	}
	return os.WriteFile(
		fmt.Sprintf("/sys/fs/cgroup/resetti/%s/cpuset.cpus", group),
		[]byte(strings.Join(cpusString, ",")),
		0644,
	)
}

// populateCcx generates a list of CPUs per CCX based on the cache topology.
func (t *cpuTopology) populateCcx() {
	// Group by CCX.
	for id, l3 := range t.l3 {
		t.Ccx = growLength(t.Ccx, l3+1)
		t.Ccx[l3] = append(t.Ccx[l3], id)
	}

	// Sort each CCX by core (L1) ID to improve locality.
	for _, cpus := range t.Ccx {
		// Slice headers are captured by value but the data is behind a pointer,
		// so SortFunc mutates the underlying data for ccx. We don't need to
		// rewrite the slice headers.
		slices.SortFunc(cpus, func(a, b int) bool {
			return t.l1[b] > t.l1[a]
		})
	}

	// Delete any empty/skipped CCX IDs.
	// I hope this doesn't happen on any CPU but I'm sure it will if someone
	// decides to run this on an EPYC one or something.
	for i := len(t.Ccx) - 1; i >= 0; i -= 1 {
		if len(t.Ccx[i]) == 0 {
			t.Ccx = slices.Delete(t.Ccx, i, i+1)
		}
	}
}

// populateCores generates a list of CPUs per core based on the cache topology.
func (t *cpuTopology) populateCores() {
	// L1 cache IDs can get skipped. On my 5900X, 6-7 are skipped. It goes
	// from 5 to 8.
	for id, l1 := range t.l1 {
		t.Cores = growLength(t.Cores, l1+1)
		t.Cores[l1] = append(t.Cores[l1], id)
	}

	// Delete any empty/skipped core IDs.
	for i := len(t.Cores) - 1; i >= 0; i -= 1 {
		if len(t.Cores[i]) == 0 {
			t.Cores = slices.Delete(t.Cores, i, i+1)
		}
	}
}
