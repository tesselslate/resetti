// Package ctl implements the main controller used for all of the available
// resetting schemes (e.g. multi, wall.)
package ctl

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jezek/xgb/xproto"
	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/obs"
	"github.com/woofdoggo/resetti/internal/x11"
	"golang.org/x/exp/slices"
)

// bufferSize is the capacity a buffered channel that processes per-instance
// state should have for each instance.
const bufferSize = 16

// Hook types
const (
	HookReset int = iota
	HookAltRes
	HookNormalRes
	HookLock
	HookUnlock
	HookWallPlay
	HookWallReset
)

// Controller manages all of the components necessary for resetti to run and
// handles communication between them.
type Controller struct {
	conf *cfg.Profile
	dbg  *debugLogger
	obs  *obs.Client
	x    *x11.Client

	counter  counter
	manager  *mc.Manager
	cpu      CpuManager
	frontend Frontend

	binds    map[cfg.Bind]cfg.ActionList
	inputMgr inputManager
	inputs   <-chan Input
	hooks    map[int]string

	obsErrors <-chan error
	mgrErrors <-chan error
	x11Errors <-chan error
	mgrEvents <-chan mc.Update
	signals   <-chan os.Signal
	x11Events <-chan x11.Event
}

// A Frontend handles user-facing I/O (input handling, instance actions, OBS
// output) and communicates with a Controller.
type Frontend interface {
	// Input processes a single user input.
	Input(Input)

	// ProcessEvent processes a miscellanous event from the X server.
	ProcessEvent(x11.Event)

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

// inputManager checks the state of the user's input devices to determine if
// they are pressing any hotkeys.
type inputManager struct {
	conf *cfg.Profile
	x    *x11.Client

	lastBinds      []cfg.Bind    // The keybinds pressed during the last query.
	lastFailWindow xproto.Window // The last window QueryPointer failed on.
}

// Run creates a new controller with the given configuration profile and runs it.
func Run(conf *cfg.Profile) error {
	log.Println("Starting up.")
	defer log.Println("Done!")
	wg := sync.WaitGroup{}
	defer wg.Wait()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := Controller{}
	c.dbg = &debugLogger{&c}
	c.conf = conf
	c.binds = make(map[cfg.Bind]cfg.ActionList)
	c.hooks = map[int]string{
		HookReset:     c.conf.Hooks.Reset,
		HookAltRes:    c.conf.Hooks.AltRes,
		HookNormalRes: c.conf.Hooks.NormalRes,
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
	modernWpCount := 0
	for _, inst := range instances {
		if inst.ModernWp {
			modernWpCount += 1
		}
	}
	log.Printf("Found %d/%d instances with modern WorldPreview\n", modernWpCount, len(instances))
	c.manager, err = mc.NewManager(instances, conf, &x)
	if err != nil {
		return fmt.Errorf("(init) create manager: %w", err)
	}

	if c.conf.Wall.Enabled {
		if c.conf.Wall.Perf.Affinity != "" {
			states := c.manager.GetStates()
			c.cpu, err = NewCpuManager(instances, states, conf)
			if err != nil {
				return fmt.Errorf("(init) create cpuManager: %w", err)
			}
		}
	}

	if c.conf.Wall.Enabled {
		if c.conf.Wall.Moving.Enabled {
			c.frontend = &MovingWall{}
		} else {
			c.frontend = &Wall{}
		}
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
	if c.cpu != nil {
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
	inputs := make(chan Input, 256)
	c.inputMgr = inputManager{c.conf, c.x, nil, 0}
	c.inputs = inputs
	go c.inputMgr.Run(inputs)

	signals := make(chan os.Signal, 8)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1)
	c.signals = signals

	log.Printf("Ready.")
	go c.dbg.Run()
	err = c.run(ctx)
	if err != nil {
		fmt.Println("Failed to run:", err)
	}
	return nil
}

// prepareObs hides all lock sources and sets the settings for each instance
// capture.
func prepareObs(o *obs.Client, instances []mc.InstanceInfo) error {
	return o.Batch(obs.SerialRealtime, func(b *obs.Batch) {
		for i := 1; i <= len(instances); i += 1 {
			settings := obs.StringMap{
				"show_cursor":    true,
				"capture_window": strconv.Itoa(int(instances[i-1].Wid)),
			}
			wallSettings := obs.StringMap{
				"show_cursor":    false,
				"capture_window": strconv.Itoa(int(instances[i-1].Wid)),
			}
			b.SetItemVisibility("Wall", fmt.Sprintf("Lock %d", i), false)
			b.SetItemVisibility("Wall", fmt.Sprintf("Wall MC %d", i), true)
			b.SetSourceSettings(fmt.Sprintf("Wall MC %d", i), wallSettings, true)
			b.SetSourceSettings(fmt.Sprintf("MC %d", i), settings, true)
		}
	})
}

// CreateSleepbgLock creates the sleepbg.lock file.
func (c *Controller) CreateSleepbgLock() {
	file, err := os.Create(c.conf.Wall.Perf.SleepbgPath)
	if err != nil {
		log.Printf("Failed to create sleepbg.lock: %s\n", err)
	} else {
		_ = file.Close()
	}
}

// DeleteSleepbgLock deletes the sleepbg.lock file.
func (c *Controller) DeleteSleepbgLock(ignoreErrors bool) {
	err := os.Remove(c.conf.Wall.Perf.SleepbgPath)
	if err != nil && !ignoreErrors {
		log.Printf("Failed to delete sleepbg.lock: %s\n", err)
	}
}

// FocusInstance switches focus to the given instance.
func (c *Controller) FocusInstance(id int) {
	c.manager.Focus(id)
}

// ToggleResolution switches the given instance between the normal and alternate
// resolution.
func (c *Controller) ToggleResolution(id int) {
	if c.manager.ToggleResolution(id) {
		c.RunHook(HookAltRes)
	} else {
		c.RunHook(HookNormalRes)
	}
}

// PlayInstance switches focus to the given instance, marks it as the active
// instance, and starts playing it.
func (c *Controller) PlayInstance(id int) {
	c.manager.Play(id)
	if c.cpu != nil {
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
		if c.cpu != nil {
			c.cpu.Update(mc.Update{
				State: mc.State{Type: mc.StDirt},
				Id:    id,
			})
		}
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
	if c.cpu != nil {
		c.cpu.SetPriority(id, prio)
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
				c.dbg.printAll()
			}
		case err := <-c.mgrErrors:
			// All manager errors are fatal.
			if errors.Is(err, mc.ErrInstanceClosed) {
				// Don't log the error twice.
				return nil
			} else {
				return fmt.Errorf("manager: %w", err)
			}
		case err, ok := <-c.obsErrors:
			if !ok {
				if err != nil {
					return fmt.Errorf("fatal OBS error: %w", err)
				} else {
					log.Println("OBS closed. Stopping.")
					return nil
				}
			}
			log.Printf("OBS error: %s\n", err)
		case err, ok := <-c.x11Errors:
			if !ok {
				return fmt.Errorf("fatal X error: %w", err)
			}
			log.Printf("X error: %s\n", err)
		case evt := <-c.mgrEvents:
			c.frontend.Update(evt)
			if c.cpu != nil {
				c.cpu.Update(evt)
			}
		case evt := <-c.x11Events:
			c.frontend.ProcessEvent(evt)
		case input := <-c.inputs:
			c.frontend.Input(input)
		}
	}
}

func (i *inputManager) Run(inputs chan<- Input) {
	for {
		// Sleep for this polling iteration and query the input devices' state.
		time.Sleep(time.Second / time.Duration(i.conf.PollRate))
		keymap, err := i.x.QueryKeymap()
		if err != nil {
			log.Printf("inputManager: Query keymap failed: %s\n", err)
			continue
		}

		var pointer x11.Pointer

		window := i.x.GetActiveWindow()
		if window != i.lastFailWindow {
			pointer, err = i.x.QueryPointer(window)
			if err != nil {
				log.Printf("inputManager: Query pointer failed: %s\n", err)
				i.lastFailWindow = window
				continue
			}
		}

		// PERF: This is kind of bad and can probably be optimized
		var pressed []cfg.Bind
		for bind := range i.conf.Keybinds {
			var mask [32]byte
			if bind.Key != nil {
				key := *bind.Key
				mask[key/8] |= (1 << (key % 8))
			}
			for _, key := range bind.Mods[:bind.ModCount] {
				mask[key/8] |= (1 << (key % 8))
			}
			if keymap.HasPressed(mask) {
				if bind.Button == nil || pointer.HasPressed(*bind.Button) {
					pressed = append(pressed, bind)
				}
			}
		}
		if len(pressed) == 0 {
			i.lastBinds = pressed
			continue
		}

		// Sort so that the most specific keybind (the one with the most
		// modifiers) is the one picked.
		slices.SortFunc(pressed, func(a, b cfg.Bind) bool {
			return b.ModCount < a.ModCount
		})
		bind := pressed[0]
		inputs <- Input{
			bind,
			slices.Contains(i.lastBinds, bind),
			pointer.EventX, pointer.EventY,
		}
		i.lastBinds = pressed
	}
}
