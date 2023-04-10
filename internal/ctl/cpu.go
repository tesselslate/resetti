package ctl

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/mc"
	"golang.org/x/exp/slices"
)

// Affinity arguments
var (
	forceCgroups  = slices.Contains(os.Args, "--force-cgroups")
	dontOverwrite = slices.Contains(os.Args, "--keep-script")
)

//go:embed cgroup_setup.sh
var cgroupScript []byte

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
	count int   // CPU count
	l1    []int // Per-CPU L1 cache ID (for core detection)
	l3    []int // Per-CPU L3 cache ID (for CCX detection)
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
			getCoreIds(&topo),
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

// getCcxIds returns the CPU IDs for each CCX on the user's CPU.
func getCcxIds(topo *cpuTopology) [2][]int {
	// TODO: I don't think there are any consumer CPUs with more than 2 CCXs but
	// that should probably get handled at some point.
	// TODO: Sort the list to group by core as well (e.g. 0,0,1,1,..), which
	// might help performance a bit more on groups with less CPUs allocated.
	var ccx [2][]int
	for id, l3 := range topo.l3 {
		ccx[l3] = append(ccx[l3], id)
	}
	return ccx
}

// getCoreIds returns the CPU IDs for each core on the user's CPU.
func getCoreIds(topo *cpuTopology) [][]int {
	// TODO: A 128 core maximum is a safe assumption for now, but this function
	// should just be made better.
	// NOTE: L1 cache IDs can get skipped. On my 5900X, 6-7 are skipped. It goes
	// from 5 to 8.
	cores := make([][]int, 128)
	for id, l1 := range topo.l1 {
		cores[l1] = append(cores[l1], id)
	}
	for i := len(cores) - 1; i >= 0; i -= 1 {
		if len(cores[i]) == 0 {
			cores = slices.Delete(cores, i, i+1)
		}
	}
	return cores
}

// getCpuTopology returns information about the user's CPU cache topology.
func getCpuTopology() (cpuTopology, error) {
	topo := cpuTopology{count: runtime.NumCPU()}
	for i := 0; i < topo.count; i += 1 {
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
	return topo, nil
}

// prepareCgroups prompts the user for root privileges and runs the cgroup
// setup script (if necessary) and assigns the correct CPU sets to each cgroup.
func prepareCgroups(conf *cfg.Profile, topo *cpuTopology, instances int) error {
	// Check if the cgroup setup script needs to be run.
	var shouldExist []string
	switch conf.Wall.Perf.Affinity {
	case "advanced":
		if conf.Wall.Perf.Adv.CcxSplit {
			for _, name := range baseNames {
				shouldExist = append(shouldExist, name+"0", name+"1")
			}
		} else {
			shouldExist = baseNames[:]
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
		path, err := cfg.GetDirectory()
		if err != nil {
			return fmt.Errorf("get config directory: %w", err)
		}
		path += "/cgroup_setup.sh"
		_, err = os.Stat(path)
		if !dontOverwrite || err != nil {
			if err := os.WriteFile(path, cgroupScript, 0644); err != nil {
				return fmt.Errorf("write cgroup script: %w", err)
			}
		}
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
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("run cgroup script: %w", err)
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
		cpus := [...]int{
			conf.Wall.Perf.Adv.CpusIdle,
			conf.Wall.Perf.Adv.CpusLow,
			conf.Wall.Perf.Adv.CpusMid,
			conf.Wall.Perf.Adv.CpusHigh,
			conf.Wall.Perf.Adv.CpusActive,
		}
		if conf.Wall.Perf.Adv.CcxSplit {
			ccx := getCcxIds(topo)
			for id, name := range baseNames {
				cpuCount := cpus[id]
				if err := writeCpuSet(name+"0", ccx[0][:cpuCount]); err != nil {
					return fmt.Errorf("write cpus for %s0: %w", name, err)
				}
				if err := writeCpuSet(name+"1", ccx[1][:cpuCount]); err != nil {
					return fmt.Errorf("write cpus for %s1: %w", name, err)
				}
			}
		} else {
			// Flatten the per-core CPU list. We want to pick CPUs on the same
			// core one after another to improve locality.
			var cpus []int
			for _, core := range getCoreIds(topo) {
				cpus = append(cpus, core...)
			}
			for id, name := range baseNames {
				cpuCount := cpus[id]
				if err := writeCpuSet(name, cpus[:cpuCount]); err != nil {
					return fmt.Errorf("write cpus for %s: %w", name, err)
				}
			}
		}
	case "sequence":
		cores := getCoreIds(topo)
		for i := 0; i < instances; i += 1 {
			if err := writeCpuSet(fmt.Sprintf("inst%d", i), cores[i]); err != nil {
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
