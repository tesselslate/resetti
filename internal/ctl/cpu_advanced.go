package ctl

import (
	"context"
	"fmt"
	"math"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/log"
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

// advancedCpuManager moves instances between several affinity "groups" as their
// states change. See the documentation for more information on each group.
type advancedCpuManager struct {
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

// Run implements CpuManager.
func (c *advancedCpuManager) Run(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	wg.Add(1)
	for {
		select {
		case <-ctx.Done():
			c.initGroups()
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

// SetPriority implements CpuManager.
func (c *advancedCpuManager) SetPriority(id int, state bool) {
	c.priority <- priorityUpdate{id, state}
}

// Update implements CpuManager.
func (c *advancedCpuManager) Update(update mc.Update) {
	c.updates <- update
}

// handleUpdate handles a single instance state update and moves the instance
// between groups as needed.
func (c *advancedCpuManager) handleUpdate(update mc.Update) {
	changed := c.states[update.Id].Type != update.State.Type

	switch update.State.Type {
	case mc.StIdle:
		if c.conf.Wall.Perf.Adv.BurstLength <= 0 {
			c.moveInstance(update.Id, affIdle)
			return
		}
		if changed {
			c.moveInstance(update.Id, affMid)
			time.AfterFunc(time.Millisecond*time.Duration(c.conf.Wall.Perf.Adv.BurstLength), func() {
				c.removeBurst <- update.Id
			})
		}
	case mc.StDirt:
		if c.anyActive {
			c.moveInstance(update.Id, affMid)
		} else {
			c.moveInstance(update.Id, affHigh)
		}
	case mc.StPreview:
		nowOver := update.State.Progress > c.conf.Wall.Perf.Adv.LowThreshold
		wasUnder := c.states[update.Id].Progress <= c.conf.Wall.Perf.Adv.LowThreshold
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

// initGroups moves each instance to a consistent state for starting or
// stopping the cpuManager.
func (c *advancedCpuManager) initGroups() {
	for i := range c.states {
		c.moveInstance(i, affHigh)
	}
}

// moveInstance attempts to move the given instance to the given group. If
// doing so will cause multiple instances to be in the active group, the
// function panics. If the instance is prioritized, its group is updated
// but it remains in the high cgroup.
func (c *advancedCpuManager) moveInstance(id int, group int) {
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
func (c *advancedCpuManager) setPriority(id int, priority bool) {
	c.states[id].priority = priority
	c.updateAffinity(id)
}

// updateAffinity updates the affinity cgroup an instance is part of. If the
// instance is prioritized, that takes precedence over whatever group it is
// a part of.
func (c *advancedCpuManager) updateAffinity(id int) {
	var group int
	if c.states[id].priority {
		group = affHigh
	} else {
		group = c.states[id].group
	}
	name := baseNames[group]
	perGroup := int(math.Ceil(float64(len(c.states)) / float64(c.conf.Wall.Perf.Adv.CcxSplit)))
	name += strconv.Itoa(id / perGroup)
	// These writes are usually fast (<= ~500us) but sometimes spike up to as
	// slow as 30+ ms. Do them asynchronously.
	go func() {
		err := os.WriteFile(
			fmt.Sprintf("/sys/fs/cgroup/resetti/%s/cgroup.procs", name),
			[]byte(strconv.Itoa(c.pids[id])),
			0644,
		)
		if err != nil {
			logger := log.FromName("resetti")
			logger.Error("cpuManager: updateAffinity failed: %s", err)
		}
	}()
}
