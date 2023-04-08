// Package ctl implements the main controller used for all of the available
// resetting schemes (e.g. multi, wall.)
package ctl

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"

	"github.com/jezek/xgb/xproto"
	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/obs"
	"github.com/woofdoggo/resetti/internal/x11"
)

// bufferSize is the capacity a buffered channel that processes per-instance
// state should have for each instance.
const bufferSize = 16

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
	cpu      CpuManager
	frontend Frontend

	useAffinity bool
	binds       map[cfg.Bind]cfg.ActionList
	hooks       map[int]string
	inputs      inputState

	obsErrors <-chan error
	mgrErrors <-chan error
	x11Errors <-chan error
	mgrEvents <-chan mc.Update
	x11Events <-chan x11.Event
	signals   <-chan os.Signal
}

// A Frontend handles user-facing I/O (input handling, instance actions, OBS
// output) and communicates with a Controller.
type Frontend interface {
	// FocusChange processes a single window focus change.
	FocusChange(x11.FocusEvent)

	// Input processes a single user input.
	Input(Input)

	// Setup takes in all of the potentially needed dependencies and prepares
	// the Frontend to handle user input.
	Setup(frontendDependencies) error

	// Update processes a single instance state update.
	Update(mc.Update)
}

// An Input represents a single user input.
type Input struct {
	Bind cfg.Bind
	Held bool
	X, Y int
}

// buttonMod contains a button, modifier pair.
type buttonMod struct {
	button xproto.Button
	mod    x11.Keymod
}

// frontendDependencies contains all of the dependencies that a Frontend might
// need to setup and run.
type frontendDependencies struct {
	conf      *cfg.Profile
	obs       *obs.Client
	x         *x11.Client
	states    []mc.State
	instances []mc.InstanceInfo
	host      *Controller
}

type inputState struct {
	cx, cy  int
	buttons map[buttonMod]bool
	keys    map[x11.Key]uint32
}

// Run creates a new controller with the given configuration profile and runs it.
func Run(conf *cfg.Profile) error {
	defer log.Println("Done!")
	wg := sync.WaitGroup{}
	defer wg.Wait()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := Controller{}
	c.conf = conf
	c.binds = make(map[cfg.Bind]cfg.ActionList)
	c.hooks = map[int]string{
		HookReset:     c.conf.Hooks.Reset,
		HookLock:      c.conf.Hooks.WallLock,
		HookUnlock:    c.conf.Hooks.WallUnlock,
		HookWallPlay:  c.conf.Hooks.WallPlay,
		HookWallReset: c.conf.Hooks.WallReset,
	}
	c.inputs = inputState{0, 0, make(map[buttonMod]bool), make(map[x11.Key]uint32)}

	signals := make(chan os.Signal, 8)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1)
	c.signals = signals

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

	if c.conf.Wall.Enabled {
		if c.conf.Wall.Performance.Affinity != "" {
			states := c.manager.GetStates()
			c.cpu, err = NewCpuManager(instances, states, conf)
			if err != nil {
				return fmt.Errorf("(init) create cpuManager: %w", err)
			}
		}
		if c.conf.Wall.Performance.Affinity == "advanced" {
			c.useAffinity = true
		}
	}

	if c.conf.Wall.Enabled {
		c.frontend = &Wall{}
	} else {
		c.frontend = &Multi{}
	}

	// Start various components
	err = c.frontend.Setup(frontendDependencies{
		conf:      c.conf,
		obs:       c.obs,
		x:         c.x,
		states:    c.manager.GetStates(),
		instances: instances,
		host:      &c,
	})
	if err != nil {
		return fmt.Errorf("(init) setup frontend: %w", err)
	}
	go c.counter.Run(ctx, &wg)
	if c.useAffinity {
		go c.cpu.Run(ctx, &wg)
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

// BindInstanceKeys ensures that all keybinds with instance actions are bound.
func (c *Controller) BindInstanceKeys() error {
	for bind, action := range c.conf.Keybinds {
		if len(action.IngameActions) != 0 {
			if _, ok := c.binds[bind]; !ok {
				if err := c.bind(bind); err != nil {
					return err
				}
			}
		} else {
			if _, ok := c.binds[bind]; ok {
				c.unbind(bind)
			}
		}
	}
	return nil
}

// BindWallKeys ensures that all keys with wall actions are bound.
func (c *Controller) BindWallKeys() error {
	for bind, action := range c.conf.Keybinds {
		if len(action.WallActions) != 0 {
			if _, ok := c.binds[bind]; !ok {
				if err := c.bind(bind); err != nil {
					return err
				}
			}
		} else {
			if _, ok := c.binds[bind]; ok {
				c.unbind(bind)
			}
		}
	}
	return nil
}

// FocusInstance switches focus to the given instance.
func (c *Controller) FocusInstance(id int) {
	c.manager.Focus(id)
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
	return ok
}

// RunHook runs the hook of the given type if it exists.
func (c *Controller) RunHook(hook int) {
	cmdStr := c.hooks[hook]
	if cmdStr == "" {
		return
	}
	go func() {
		bin, rawArgs, ok := strings.Cut(cmdStr, " ")
		var args []string
		if ok {
			args = strings.Split(rawArgs, " ")
		}
		cmd := exec.Command(bin, args...)
		err := cmd.Run()
		if err != nil {
			log.Printf("RunHook (%d) failed: %s\n", hook, err)
		}
	}()
}

// SetPriority sets the priority of the instance in the CPU manager.
func (c *Controller) SetPriority(id int, prio bool) {
	if c.useAffinity {
		c.cpu.SetPriority(id, prio)
	}
}

// UnbindWallKeys unbinds all wall-only keys, except for focus projector.
func (c *Controller) UnbindWallKeys() {
	// HACK: Clear button state after pointer ungrab. This needs to be
	// made better.
	c.inputs.buttons = make(map[buttonMod]bool)
	for bind := range c.binds {
		actions := c.conf.Keybinds[bind]
		if len(actions.IngameActions) == 0 {
			hasFocusProjector := false
			for _, action := range actions.WallActions {
				if action.Type == cfg.ActionWallFocus {
					hasFocusProjector = true
					break
				}
			}
			if !hasFocusProjector {
				c.unbind(bind)
			}
		}
	}
}

// bind binds the given key.
func (c *Controller) bind(bind cfg.Bind) error {
	if bind.Key != nil {
		err := c.x.GrabKey(x11.Key{
			Code: *bind.Key,
			Mod:  bind.Mod,
		}, c.x.GetRootWindow())
		if err != nil {
			return err
		}
	}
	c.binds[bind] = c.conf.Keybinds[bind]
	return nil
}

// debug prints debug information.
func (c *Controller) debug() {
	mem := runtime.MemStats{}
	runtime.ReadMemStats(&mem)
	memStats := strings.Builder{}
	memStats.WriteString(fmt.Sprintf("\nLive objects: %d\n", mem.HeapObjects))
	memStats.WriteString(fmt.Sprintf("Malloc count: %d\n", mem.Mallocs))
	memStats.WriteString(fmt.Sprintf("Total allocation: %.2f mb\n", float64(mem.TotalAlloc)/1000000))
	memStats.WriteString(fmt.Sprintf("Current allocation: %.2f mb\n", float64(mem.HeapAlloc)/1000000))
	memStats.WriteString(fmt.Sprintf("GC time: %.2f%%\n", mem.GCCPUFraction))
	memStats.WriteString(fmt.Sprintf("GC cycles: %d\n", mem.NumGC))
	memStats.WriteString(fmt.Sprintf("Total STW: %.4f ms", float64(mem.PauseTotalNs)/1000000))
	log.Printf(
		"Received SIGUSR1\n---- Debug info\nGoroutine count: %d\nMemory:%s\nInstances:\n%s",
		runtime.NumGoroutine(),
		memStats.String(),
		c.manager.Debug(),
	)
}

// matchBind attempts to match a given keybind based on the current input state.
func (c *Controller) matchBind() (bind cfg.Bind, ok bool) {
	if len(c.inputs.keys)+len(c.inputs.buttons) != 1 {
		return cfg.Bind{}, false
	}
	if len(c.inputs.keys) == 1 {
		// There's only one element.
		for key := range c.inputs.keys {
			for bind := range c.binds {
				if bind.Key != nil && *bind.Key == key.Code && bind.Mod == key.Mod {
					return bind, true
				}
			}
		}
	} else {
		// There's only one element.
		for button := range c.inputs.buttons {
			for bind := range c.binds {
				if bind.Mouse != nil && *bind.Mouse == button.button && bind.Mod == button.mod {
					return bind, true
				}
			}
		}
	}
	return cfg.Bind{}, false
}

// unbind removes the given bind.
func (c *Controller) unbind(bind cfg.Bind) {
	delete(c.binds, bind)
	if bind.Key != nil {
		err := c.x.UngrabKey(x11.Key{
			Code: *bind.Key,
			Mod:  bind.Mod,
		}, c.x.GetRootWindow())
		if err != nil {
			log.Printf("Unbind (%v) failed: %s\n", bind, err)
		}
	}
}

// run runs the main loop for the controller.
func (c *Controller) run(ctx context.Context) error {
	for {
		select {
		case sig := <-c.signals:
			switch sig {
			case syscall.SIGINT, syscall.SIGTERM:
				log.Println("Shutting down.")
				return nil
			case syscall.SIGUSR1:
				c.debug()
			}
		case err := <-c.mgrErrors:
			// All manager errors are fatal.
			return fmt.Errorf("manager: %w", err)
		case err, ok := <-c.obsErrors:
			if !ok {
				return fmt.Errorf("fatal OBS error: %w", err)
			}
			log.Printf("OBS error: %s\n", err)
		case err, ok := <-c.x11Errors:
			if !ok {
				return fmt.Errorf("fatal X error: %w", err)
			}
			log.Printf("X error: %s\n", err)
		case evt := <-c.mgrEvents:
			c.frontend.Update(evt)
			if c.useAffinity {
				c.cpu.Update(evt)
			}
		case evt := <-c.x11Events:
			switch evt := evt.(type) {
			case x11.FocusEvent:
				c.frontend.FocusChange(evt)
			case x11.KeyEvent:
				if evt.State == x11.StateDown {
					// XXX: We have to use a GLFW hackfix here ourselves (:
					// Ignore repeat keydown events.
					if last, ok := c.inputs.keys[evt.Key]; ok {
						if evt.Timestamp-last <= 20 {
							c.inputs.keys[evt.Key] = evt.Timestamp
							continue
						}
					}
					c.inputs.keys[evt.Key] = evt.Timestamp
					bind, ok := c.matchBind()
					if !ok {
						continue
					}
					c.frontend.Input(Input{
						bind, false, c.inputs.cx, c.inputs.cy,
					})
				} else {
					delete(c.inputs.keys, evt.Key)
				}
			case x11.ButtonEvent:
				// Get rid of button modmask by discarding higher bits.
				bm := buttonMod{evt.Button, evt.Mod & 255}
				c.inputs.cx, c.inputs.cy = int(evt.Point.X), int(evt.Point.Y)
				if evt.State == x11.StateDown {
					c.inputs.buttons[bm] = true
					bind, ok := c.matchBind()
					if !ok {
						continue
					}
					c.frontend.Input(Input{
						bind, false, c.inputs.cx, c.inputs.cy,
					})
				} else {
					delete(c.inputs.buttons, bm)
				}
			case x11.MoveEvent:
				c.inputs.cx, c.inputs.cy = int(evt.Point.X), int(evt.Point.Y)
				bind, ok := c.matchBind()
				if !ok {
					continue
				}
				c.frontend.Input(Input{
					bind, true, c.inputs.cx, c.inputs.cy,
				})
			}
		}
	}
}
