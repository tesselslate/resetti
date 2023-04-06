// Package ctl implements the main controller used for all of the available
// resetting schemes (e.g. multi, wall.)
package ctl

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"

	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/obs"
	"github.com/woofdoggo/resetti/internal/x11"
)

// bufferSize is the capacity a buffered channel that processes per-instance
// state should have for each instance.
const bufferSize = 16

// Input keybindings
const (
	BindFocus int = iota
	BindReset
	BindWallLock
	BindWallPlay
	BindWallReset
	BindWallResetOthers
)

// Hook types
const (
	HookReset int = iota
	HookLock
	HookUnlock
	HookWallPlay
	HookWallReset
)

// Controller manages all of the components necessary for resetti to run and
// handles communication between them.
type Controller struct {
	conf *cfg.Profile
	obs  *obs.Client
	x    *x11.Client

	counter  counter
	manager  *mc.Manager
	cpu      cpuManager
	frontend Frontend

	useAffinity bool
	binds       map[int]cfg.Bind
	hooks       map[int]string

	obsErrors <-chan error
	mgrErrors <-chan error
	x11Errors <-chan error
	mgrEvents <-chan mc.Update
	x11Events <-chan x11.Event
}

// A Frontend handles user-facing I/O (input handling, instance actions, OBS
// output) and communicates with a Controller.
type Frontend interface {
	// Setup takes in all of the potentially needed dependencies and prepares
	// the Frontend to handle user input.
	Setup(frontendDependencies) error

	// Input processes a single user input.
	Input(x11.Event)

	// Update processes a single instance state update.
	Update(mc.Update)
}

// frontendDependencies contains all of the dependencies that a Frontend might
// need to setup and run.
type frontendDependencies struct {
	conf   *cfg.Profile
	obs    *obs.Client
	x      *x11.Client
	states []mc.State
	host   *Controller
}

// Run creates a new controller with the given configuration profile and runs it.
func Run(conf *cfg.Profile) error {
	wg := sync.WaitGroup{}
	defer wg.Wait()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := Controller{}
	c.conf = conf
	c.binds = make(map[int]cfg.Bind)
	c.hooks = map[int]string{
		HookReset:     c.conf.Hooks.Reset,
		HookLock:      c.conf.Hooks.WallLock,
		HookUnlock:    c.conf.Hooks.WallUnlock,
		HookWallPlay:  c.conf.Hooks.WallPlay,
		HookWallReset: c.conf.Hooks.WallReset,
	}

	x, err := x11.NewClient()
	if err != nil {
		return fmt.Errorf("(init) create X client: %w", err)
	}
	c.x = &x

	c.obs = &obs.Client{}
	if conf.Obs.Enabled {
		obsErrors, err := c.obs.Connect(ctx, conf.Obs.Port, conf.Obs.Password)
		if err != nil {
			return fmt.Errorf("(init) create OBS client: %w", err)
		}
		c.obsErrors = obsErrors
	}

	c.counter, err = newCounter(conf)
	if err != nil {
		return fmt.Errorf("(init) create counter: %w", err)
	}

	instances, err := mc.FindInstances(&x)
	if err != nil {
		return fmt.Errorf("(init) find instances: %w", err)
	}
	c.manager, err = mc.NewManager(instances, conf, &x)
	if err != nil {
		return fmt.Errorf("(init) create manager: %w", err)
	}

	if c.conf.Wall.Enabled && c.conf.Wall.Performance.Affinity {
		c.useAffinity = true
		states := c.manager.GetStates()
		c.cpu, err = newCpuManager(instances, states, conf)
		if err != nil {
			return fmt.Errorf("(init) create cpuManager: %w", err)
		}
	}

	if c.conf.Wall.Enabled {
		c.frontend = &Wall{}
	} else {
		c.frontend = &Multi{}
	}

	// Start various components
	err = c.frontend.Setup(frontendDependencies{
		conf:   c.conf,
		obs:    c.obs,
		x:      c.x,
		states: c.manager.GetStates(),
		host:   &c,
	})
	if err != nil {
		return fmt.Errorf("(init) setup frontend: %w", err)
	}
	go c.counter.Run(ctx, &wg)
	if c.useAffinity {
		go c.cpu.Run(ctx)
	}
	evtch := make(chan mc.Update, bufferSize*len(instances))
	errch := make(chan error, 1)
	c.mgrEvents = evtch
	c.mgrErrors = errch
	go c.manager.Run(ctx, evtch, errch)
	c.x11Events, c.x11Errors, err = c.x.Poll(ctx)
	if err != nil {
		return fmt.Errorf("(init) X poll: %w", err)
	}

	err = c.run(ctx)
	if err != nil {
		fmt.Println("Failed to run:", err)
	}
	return nil
}

// Bind adds the given keybind.
func (c *Controller) Bind(kind int, bind cfg.Bind) error {
	if bind.Key != nil {
		err := c.x.GrabKey(x11.Key{
			Code: *bind.Key,
			Mod:  bind.Mod,
		}, c.x.GetRootWindow())
		if err != nil {
			return err
		}
	}
	c.binds[kind] = bind
	return nil
}

// FocusInstance switches focus to the given instance.
func (c *Controller) FocusInstance(id int) {
	c.manager.Focus(id)
}

// GetBindFor returns which keybind the given input is (if any.)
func (c *Controller) GetBindFor(evt x11.Event) (bind int, ok bool) {
	switch evt := evt.(type) {
	case x11.KeyEvent:
		for idx, bind := range c.binds {
			if bind.MatchKey(evt) {
				return idx, true
			}
		}
	case x11.MoveEvent:
		for idx, bind := range c.binds {
			if bind.MatchMove(evt) {
				return idx, true
			}
		}
	case x11.ButtonEvent:
		for idx, bind := range c.binds {
			if bind.MatchButton(evt) {
				return idx, true
			}
		}
	}
	return 0, false
}

// PlayInstance switches focus to the given instance, marks it as the active
// instance, and starts playing it.
func (c *Controller) PlayInstance(id int) {
	c.manager.Play(id)
	if c.useAffinity {
		c.cpu.Update(mc.Update{
			State: mc.State{Type: mc.StIngame},
			Id:    id,
		})
	}
}

// ResetInstance attempts to reset the given instance and returns whether or
// not the reset was successful.
func (c *Controller) ResetInstance(id int) bool {
	ok := c.manager.Reset(id)
	if ok {
		c.counter.Increment()
	}
	go c.RunHook(HookReset)
	return ok
}

// RunHook runs the hook of the given type if it exists.
func (c *Controller) RunHook(hook int) {
	bin, rawArgs, ok := strings.Cut(c.hooks[hook], " ")
	var args []string
	if ok {
		args = strings.Split(rawArgs, " ")
	}
	cmd := exec.Command(bin, args...)
	err := cmd.Run()
	if err != nil {
		log.Printf("RunHook (%d) failed: %s\n", hook, err)
	}
}

// SetPriority sets the priority of the instance in the CPU manager.
func (c *Controller) SetPriority(id int, prio bool) {
	if c.useAffinity {
		c.cpu.SetPriority(id, prio)
	}
}

// Unbind removes the given bind.
func (c *Controller) Unbind(kind int) {
	bind, ok := c.binds[kind]
	if !ok {
		panic(fmt.Sprintf("tried to unbind unbound key: %d", kind))
	}
	if bind.Key != nil {
		err := c.x.UngrabKey(x11.Key{
			Code: *bind.Key,
			Mod:  bind.Mod,
		}, c.x.GetRootWindow())
		if err != nil {
			log.Printf("Unbind (%d) failed: %s\n", kind, err)
		}
	}
}

// run runs the main loop for the controller.
func (c *Controller) run(ctx context.Context) error {
	for {
		select {
		case err := <-c.mgrErrors:
			// All manager errors are fatal.
			return fmt.Errorf("manager: %w", err)
		case err := <-c.obsErrors:
			// TODO: Make OBS differentiate fatal/non fatal errors.
			log.Printf("OBS error: %s\n", err)
		case err := <-c.x11Errors:
			// TODO: Make X differentiate fatal/non fatal errors.
			log.Printf("X error: %s\n", err)
		case evt := <-c.mgrEvents:
			c.frontend.Update(evt)
			if c.useAffinity {
				c.cpu.Update(evt)
			}
		case evt := <-c.x11Events:
			// TODO: Proper input handling that allows for holding key with
			// mousemove, just like mouse button with mouse move right now.
			c.frontend.Input(evt)
		}
	}
}
