package reset

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/obs"
	"github.com/woofdoggo/resetti/internal/x11"
)

// Controller receives user input and connects various components (input handling,
// affinity management, OBS output, etc) together.
type Controller struct {
	conf      cfg.Profile
	instances []mc.Instance
	states    []mc.InstanceState
	readers   []mc.LogReader

	obs *obs.Client
	x   *x11.Client

	affinity CpuManager
	counter  Counter
	frontend Frontend

	pause     chan int
	logErrors <-chan error
	obsErrors <-chan error
	xErrors   <-chan error
	inputs    <-chan x11.Event
	updates   <-chan mc.Update
}

// Run creates a new Controller with the given configuration profile and runs
// its main loop.
func Run(conf cfg.Profile) error {
	ctx, cancel := context.WithCancel(context.Background())
	// XXX: This gets called twice if (*Controller).Run() is called, but the
	// 2nd call is a no-op and doesn't break anything. We need to call it here
	// to make `go vet` happy.
	defer cancel()

	wg := sync.WaitGroup{}
	c := Controller{
		conf: conf,
	}

	// Setup X client.
	x, err := x11.NewClient()
	if err != nil {
		return errors.Wrap(err, "start X client")
	}
	c.x = &x
	c.inputs, c.xErrors, err = x.Poll(ctx)
	if err != nil {
		return errors.Wrap(err, "start X polling")
	}

	// Setup OBS client.
	if conf.Obs.Enabled {
		c.obs = &obs.Client{}
		c.obsErrors, err = c.obs.Connect(ctx, conf.Obs.Port, conf.Obs.Password)
		if err != nil {
			return errors.Wrap(err, "start OBS client")
		}
	}

	// Find instances.
	infos, err := mc.FindInstances(c.x)
	if err != nil {
		return errors.Wrap(err, "find instances")
	}
	c.instances = make([]mc.Instance, 0)
	c.states = make([]mc.InstanceState, 0)
	c.readers = make([]mc.LogReader, 0)
	updatech := make(chan mc.Update, 16*len(infos))
	errch := make(chan error, len(infos))
	c.pause = make(chan int, len(infos))
	c.updates = updatech
	c.logErrors = errch
	for _, info := range infos {
		instance := mc.NewInstance(info, &c.conf, c.x)
		if err = instance.Click(); err != nil {
			return errors.Wrap(err, "click instance")
		}
		c.instances = append(c.instances, instance)
		reader, state, err := mc.NewLogReader(info)
		if err != nil {
			return errors.Wrap(err, "create log reader")
		}
		c.states = append(c.states, state)
		c.readers = append(c.readers, reader)
		go reader.Run(ctx, errch, updatech)
	}

	// Setup miscellaneous resources (reset counter, affinity manager.)
	c.counter, err = NewCounter(ctx, &wg, conf)
	if err != nil {
		return errors.Wrap(err, "start counter")
	}
	if c.conf.AdvancedWall.Affinity {
		c.affinity, err = NewCpuManager(conf, infos)
		if err != nil {
			return errors.Wrap(err, "start affinity manager")
		}
	}

	// Setup frontend.
	switch c.conf.General.ResetType {
	case "standard":
		c.frontend = &FrontendMulti{}
	case "wall":
		c.frontend = &FrontendWall{}
	}
	err = c.frontend.Setup(FrontendOptions{
		&conf,
		&c,
		c.obs,
		c.x,
		c.states,
		c.instances,
	})
	if err != nil {
		return errors.Wrap(err, "start frontend")
	}

	// Run main loop.
	log.Println("Ready")
	c.run(ctx, cancel, &wg)
	return nil
}

// PlayInstance sets the given instance as active.
func (c *Controller) PlayInstance(id int) error {
	c.states[id].State = mc.StIngame
	if c.conf.AdvancedWall.Affinity {
		return c.affinity.Update(id, c.states[id])
	}
	return nil
}

// SetInstancePriority sets the priority of the instance for CPU affinity.
func (c *Controller) SetInstancePriority(id int, priority bool) {
	if !c.conf.AdvancedWall.Affinity {
		return
	}
	if err := c.affinity.SetPriority(id, priority); err != nil {
		log.Printf("Failed to set instance priority: %s\n", err)
	}
}

// ResetInstance resets the given instance.
func (c *Controller) ResetInstance(id int, time uint32) {
	if c.conf.AdvancedWall.Affinity {
		err := c.affinity.Update(id, mc.InstanceState{State: mc.StDirt})
		if err != nil {
			log.Printf("Failed to set affinity on reset: %s\n", err)
		}
	}
	if c.states[id].State == mc.StPreview {
		c.instances[id].LeavePreview(time)
	} else {
		c.instances[id].Reset(time)
	}
	c.counter.Increment()
}

// run runs the main loop.
func (c *Controller) run(ctx context.Context, cancel context.CancelFunc, wg *sync.WaitGroup) {
	defer wg.Wait()
	defer cancel()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT)
	for {
		select {
		case <-signals:
			log.Println("Shutting down.")
			return
		case err := <-c.obsErrors:
			log.Printf("Fatal OBS error: %s\n", err)
			return
		case err := <-c.logErrors:
			log.Printf("Fatal reader error: %s\n", err)
			return
		case err := <-c.xErrors:
			if err == x11.ErrConnectionDied {
				log.Printf("X connection closed. Shutting down.")
				return
			}
			log.Printf("Unhandled X error: %s\n", err)
		case id := <-c.pause:
			if c.states[id].State == mc.StIdle || c.states[id].State == mc.StPreview {
				c.instances[id].PressF3Esc(c.x.GetCurrentTime())
			}
		case update := <-c.updates:
			if err := c.frontend.HandleUpdate(update); err != nil {
				log.Printf("Error handling update: %s\n", err)
			}
			id := update.Id
			state := update.State

			nowIdle := c.states[id].State != mc.StIdle && state.State == mc.StIdle
			nowPreview := c.states[id].State != mc.StPreview && state.State == mc.StPreview
			if (nowIdle || nowPreview) && c.frontend.ShouldPause(id) {
				go func(id int) {
					<-time.After(time.Millisecond * time.Duration(c.conf.Reset.PauseDelay))
					c.pause <- id
				}(id)
			}

			if c.conf.AdvancedWall.Affinity {
				if err := c.affinity.Update(id, state); err != nil {
					log.Printf("Error updating affinity: %s\n", err)
				}
			}
			c.states[id] = state
		case input := <-c.inputs:
			if err := c.frontend.HandleInput(input); err != nil {
				log.Printf("Error handling input: %s\n", err)
			}
		}
	}
}
