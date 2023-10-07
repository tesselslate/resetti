package ctl

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/tesselslate/resetti/internal/cfg"
	"github.com/tesselslate/resetti/internal/log"
	"github.com/tesselslate/resetti/internal/mc"
)

// sequenceCpuManager moves each instance to its own core while resetting. When
// an instance is being played, the active instance is given access to more
// cores and the background instances are given access to less.
type sequenceCpuManager struct {
	anyActive bool // Whether there is an active instance
	pids      []int
	states    []mc.State
	cores     [][]int
	cpus      []int

	conf     *cfg.Profile
	priority chan priorityUpdate
	updates  chan mc.Update
}

// Run implements CpuManager.
func (c *sequenceCpuManager) Run(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	wg.Add(1)

	// Make flattened CPU list.
	for _, core := range c.cores {
		c.cpus = append(c.cpus, core...)
	}
	activeCpus := c.conf.Wall.Perf.Seq.ActiveCpus
	bgCpus := c.conf.Wall.Perf.Seq.BackgroundCpus
	lockCpus := c.conf.Wall.Perf.Seq.LockCpus

	for {
		select {
		case <-ctx.Done():
			if err := c.initGroups(); err != nil {
				log.Error("cpuManager: Failed to move instances to consistent state: %s", err)
			}
			return
		case update := <-c.updates:
			if activeCpus <= 0 && bgCpus <= 0 {
				continue
			}

			prev := c.states[update.Id].Type
			next := update.State.Type
			changed := prev != next
			if changed && next == mc.StIngame {
				if c.anyActive {
					panic("multiple active instances")
				}
				c.anyActive = true
				for id := range c.pids {
					if id == update.Id {
						if activeCpus > 0 {
							c.writeCpus(id, c.cpus[:activeCpus])
						}
					} else {
						if bgCpus > 0 {
							c.writeCpus(id, c.cpus[:bgCpus])
						}
					}
				}
			} else if changed && prev == mc.StIngame {
				c.anyActive = false
				for id := range c.pids {
					c.writeCpus(id, c.cores[id])
				}
			}
			c.states[update.Id] = update.State
		case prio := <-c.priority:
			if lockCpus > 0 {
				if prio.state {
					c.writeCpus(prio.id, c.cpus[:lockCpus])
				} else {
					if c.states[prio.id].Type == mc.StIngame {
						c.writeCpus(prio.id, c.cpus[:activeCpus])
					} else {
						c.writeCpus(prio.id, c.cores[prio.id])
					}
				}
			}
		}
	}
}

// SetPriority implements CpuManager.
func (c *sequenceCpuManager) SetPriority(id int, prio bool) {
	c.priority <- priorityUpdate{id, prio}
}

// Update implements CpuManager.
func (c *sequenceCpuManager) Update(update mc.Update) {
	c.updates <- update
}

// initGroups moves all instances to a consistent affinity state.
func (c *sequenceCpuManager) initGroups() error {
	for id, pid := range c.pids {
		err := os.WriteFile(
			fmt.Sprintf("/sys/fs/cgroup/resetti/inst%d/cgroup.procs", id),
			[]byte(strconv.Itoa(pid)),
			0644,
		)
		if err != nil {
			return fmt.Errorf("move instance %d: %w", id, err)
		}
		c.writeCpus(id, c.cores[id])
	}
	return nil
}

// writeCpus updates the CPUs assigned to a given instance.
func (c *sequenceCpuManager) writeCpus(id int, cpus []int) {
	// Writing CPU sets can have a latency impact.
	go func() {
		if err := writeCpuSet(fmt.Sprintf("inst%d", id), cpus); err != nil {
			log.Error("sequenceCpuManager.writeCpus: Failed to write CPU set: %s", err)
		}
	}()
}
