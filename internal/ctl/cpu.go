package ctl

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/mc"
)

// Affinity groups
const (
	// affIdle is the affinity group used for instances which have finished
	// generating a world and are now paused.
	affIdle int = iota

	// affLow is the affinity group used for instances which have crossed the
	// low_threshold config option.
	affLow

	// affMid is the affinity group used for instances which have not yet
	// crossed the low_threshold config option when the user is ingame.
	//
	// Instances may also be temporarily moved to the mid group after they
	// finish world generation to allow more time for chunks to render
	// (referred to below as the "burst" period.)
	affMid

	// affHigh is the affinity group used for instances which have not yet
	// crossed the low_threshold config option when the user is not ingame,
	// as well as those which have been given priority.
	affHigh

	// affActive is the affinity group used for the currently focused instance
	// (if one exists.)
	affActive
)

// Affinity group names
var baseNames = [...]string{
	"idle",
	"low",
	"mid",
	"high",
	"active",
}

//go:embed cgroup_setup.sh
var cgroupScript []byte

// A list of common setuid binaries. Used for getting root privileges to run
// the cgroup setup script.
var suidBinaries = [...]string{
	"sudo",
	"doas",
	"pkexec",
}

// cpuManager controls how much CPU time is given to each instance based on its
// state. Currently, this is done via the use of cgroups and CPU affinity.
//
// Each instance can be in one of several affinity groups and is moved between
// groups when updates to its state are received. See the documentation for the
// affinity group constants for more information on each group.
type cpuManager struct {
	anyActive bool // Whether there is an active instance
	pids      []int
	states    []cpuState

	conf        *cfg.Profile
	priority    chan priorityUpdate
	removeBurst chan int
	updates     chan mc.Update
}

// cpuState contains the state of a given instance as well as its affinity
// properties.
type cpuState struct {
	mc.State
	group    int  // Affinity group
	priority bool // Move to high priority when appropriate
}

// priorityUpdate contains an update to the priority of a single instance.
type priorityUpdate struct {
	id    int
	state bool
}

// newCpuManager creates a new cpuManager for the given instances and config
// profile. If necessary, it prompts the user for root permission and runs the
// cgroup creation script.
func newCpuManager(instances []mc.InstanceInfo, states []mc.State, conf *cfg.Profile) (cpuManager, error) {
	if err := prepareCgroups(conf); err != nil {
		return cpuManager{}, err
	}
	pids := make([]int, 0, len(instances))
	for _, instance := range instances {
		pids = append(pids, int(instance.Pid))
	}
	cpuStates := make([]cpuState, 0, len(instances))
	for _, state := range states {
		cpuStates = append(cpuStates, cpuState{state, affIdle, false})
	}
	manager := cpuManager{
		false,
		pids,
		cpuStates,
		conf,
		make(chan priorityUpdate, bufferSize*len(instances)),
		make(chan int, bufferSize*len(instances)),
		make(chan mc.Update, bufferSize*len(instances)),
	}
	return manager, nil
}

// prepareCgroups prompts the user for root privileges and runs the cgroup
// setup script (if necessary) and assigns the correct CPU sets to each cgroup.
func prepareCgroups(conf *cfg.Profile) error {
	// Check if the cgroup setup script needs to be run.
	var shouldExist []string
	if conf.Wall.Performance.CcxSplit {
		for _, name := range baseNames {
			shouldExist = append(shouldExist, name+"0", name+"1")
		}
	} else {
		shouldExist = baseNames[:]
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
	if shouldRun {
		// TODO: Allow for modifying the script (or at least don't rewrite it
		// every time even when it has not been modified)
		path, err := cfg.GetDirectory()
		if err != nil {
			return fmt.Errorf("get config directory: %w", err)
		}
		path += "/cgroup_setup.sh"
		if err := os.WriteFile(path, cgroupScript, 0644); err != nil {
			return fmt.Errorf("write cgroup script: %w", err)
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
	cpus := [...]int{
		conf.Wall.Performance.CpusIdle,
		conf.Wall.Performance.CpusLow,
		conf.Wall.Performance.CpusMid,
		conf.Wall.Performance.CpusHigh,
		conf.Wall.Performance.CpusActive,
	}
	for idx, group := range baseNames {
		// If CCX splitting is disabled, assign 0, 1, ..., N-1
		// If it is enabled:
		// base0 - assign 0, 2, ..., (N-1) * 2
		// base1 - assign 1, 3, ..., (N*2) - 1
		set := make([]int, cpus[idx])
		for cpu := 0; cpu < cpus[idx]; cpu += 1 {
			set = append(set, cpu)
		}
		if conf.Wall.Performance.CcxSplit {
			for idx := range set {
				set[idx] *= 2
			}
			if err := writeCpuSet(group+"0", set); err != nil {
				return err
			}
			for idx := range set {
				set[idx] += 1
			}
			if err := writeCpuSet(group+"1", set); err != nil {
				return err
			}
		} else {
			if err := writeCpuSet(group, set); err != nil {
				return err
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

// Update updates the affinity group of the given instance as needed based on
// the state change.
func (c *cpuManager) Update(update mc.Update) {
	c.updates <- update
}

// SetPriority sets the priority state of the given instance.
func (c *cpuManager) SetPriority(id int, state bool) {
	c.priority <- priorityUpdate{id, state}
}

// handleUpdate handles a single instance state update and moves the instance
// between groups as needed.
func (c *cpuManager) handleUpdate(update mc.Update) {
	changed := c.states[update.Id].Type != update.State.Type

	switch update.State.Type {
	case mc.StIdle:
		if c.conf.Wall.Performance.BurstLength == 0 {
			c.moveInstance(update.Id, affIdle)
			return
		}
		if changed {
			c.moveInstance(update.Id, affMid)
			go func() {
				<-time.After(time.Millisecond * time.Duration(c.conf.Wall.Performance.BurstLength))
				c.removeBurst <- update.Id
			}()
		}
	case mc.StDirt:
		if c.anyActive {
			c.moveInstance(update.Id, affMid)
		} else {
			c.moveInstance(update.Id, affHigh)
		}
	case mc.StPreview:
		nowOver := update.State.Progress > c.conf.Wall.Performance.LowThreshold
		wasUnder := c.states[update.Id].Progress <= c.conf.Wall.Performance.LowThreshold
		if wasUnder && nowOver {
			c.moveInstance(update.Id, affLow)
		} else if changed {
			if c.anyActive {
				c.moveInstance(update.Id, affMid)
			} else {
				c.moveInstance(update.Id, affHigh)
			}
		}
	case mc.StIngame:
		c.moveInstance(update.Id, affActive)
	}
	c.states[update.Id].State = update.State
}

// moveInstance attempts to move the given instance to the given group. If
// doing so will cause multiple instances to be in the active group, the
// function panics. If the instance is prioritized, its group is updated
// but it remains in the high cgroup.
func (c *cpuManager) moveInstance(id int, group int) {
	if c.states[id].group == group {
		return
	}
	if group == affActive {
		if c.anyActive {
			panic("multiple active instances")
		}
		c.anyActive = true
		for id, state := range c.states {
			if state.group == affHigh {
				c.moveInstance(id, affMid)
			}
		}
	} else if c.states[id].group == affActive {
		c.anyActive = false
		for id, state := range c.states {
			if state.group == affMid {
				c.moveInstance(id, affHigh)
			}
		}
	}
	c.states[id].group = group
	c.updateAffinity(id)
}

// setPriority sets the priority of an instance. If the instance is not already
// in the high cgroup, it is moved there.
func (c *cpuManager) setPriority(id int, priority bool) {
	if priority == c.states[id].priority {
		log.Println("cpuManager (debug): pointless priority update")
	}
	c.states[id].priority = priority
	c.updateAffinity(id)
}

// updateAffinity updates the affinity cgroup an instance is part of. If the
// instance is prioritized, that takes precedence over whatever group it is
// a part of.
func (c *cpuManager) updateAffinity(id int) {
	var group int
	if c.states[id].priority {
		group = affHigh
	} else {
		group = c.states[id].group
	}
	name := baseNames[group]
	if c.conf.Wall.Performance.CcxSplit {
		if id < len(c.pids)/2 {
			name += "0"
		} else {
			name += "1"
		}
	}
	// XXX: Synchronous IO in event loop
	err := os.WriteFile(
		fmt.Sprintf("/sys/fs/cgroup/resetti/%s/cgroup.procs", name),
		[]byte(strconv.Itoa(c.pids[id])),
		0644,
	)
	if err != nil {
		log.Printf("cpuManager: updateAffinity failed: %s\n", err)
	}
}

// Run handles state updates and moves instances between affinity groups.
func (c *cpuManager) Run(ctx context.Context) {
	// TODO: Move instances to a consistent state when starting and stopping
	for {
		select {
		case <-ctx.Done():
			return
		case update := <-c.updates:
			c.handleUpdate(update)
		case prio := <-c.priority:
			c.setPriority(prio.id, prio.state)
		case id := <-c.removeBurst:
			if c.states[id].Type == mc.StIdle {
				c.moveInstance(id, affIdle)
			}
		}
	}
}
