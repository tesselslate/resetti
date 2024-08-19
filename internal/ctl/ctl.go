// Package ctl implements the main controller used for all of the available
// resetting schemes (e.g. multi, wall.)
package ctl

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jezek/xgb/xproto"
	"github.com/tesselslate/resetti/internal/cfg"
	"github.com/tesselslate/resetti/internal/log"
	"github.com/tesselslate/resetti/internal/mc"
	"github.com/tesselslate/resetti/internal/x11"
	"golang.org/x/exp/slices"
)

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
	x    *x11.Client

	manager  *mc.Manager
	frontend Frontend

	binds    map[cfg.Bind]cfg.ActionList
	inputMgr inputManager
	inputs   <-chan Input
	hooks    map[int][]string

	x11Errors <-chan error
	signals   <-chan os.Signal
	x11Events <-chan x11.Event
}

// A Frontend handles user-facing I/O (input handling, instance actions, OBS
// output) and communicates with a Controller.
type Frontend interface {
	// Input processes a single user input.
	Input(Input)

	// Setup takes in all of the potentially needed dependencies and prepares
	// the Frontend to handle user input.
	Setup(frontendDependencies) error
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
	conf     *cfg.Profile
	x        *x11.Client
	instance mc.InstanceInfo
	host     *Controller
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
	defer log.Info("Done")
	wg := sync.WaitGroup{}
	defer wg.Wait()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := Controller{}
	c.dbg = &debugLogger{&c}
	c.conf = conf
	c.binds = make(map[cfg.Bind]cfg.ActionList)
	c.hooks = map[int][]string{
		HookReset:     {c.conf.Hooks.Reset},
		HookAltRes:    c.conf.Hooks.AltRes,
		HookNormalRes: c.conf.Hooks.NormalRes,
	}

	x, err := x11.NewClient()
	if err != nil {
		return fmt.Errorf("(init) create X client: %w", err)
	}
	c.x = &x

	instance, err := mc.FindInstance(&x)
	if err != nil {
		return fmt.Errorf("(init) find instance: %w", err)
	}
	if instance.ModernWp {
		log.Info("Instance detected has modern WorldPreview")
	} else {
		log.Info("Instance detected does not have modern WorldPreview")
	}

	c.manager, err = mc.NewManager(instance, conf, &x)
	if err != nil {
		return fmt.Errorf("(init) create manager: %w", err)
	}

	c.frontend = &Single{}

	// Start various components
	err = c.frontend.Setup(frontendDependencies{
		conf:     c.conf,
		x:        c.x,
		instance: instance,
		host:     &c,
	})
	if err != nil {
		return fmt.Errorf("(init) setup frontend: %w", err)
	}

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

	log.Info("Ready.")
	go c.dbg.Run()
	err = c.run()
	if err != nil {
		fmt.Println("Failed to run:", err)
	}
	return nil
}

// FocusInstance switches focus to the given instance.
func (c *Controller) FocusInstance() {
	c.manager.Focus()
}

// ToggleResolution switches the given instance between the normal (play)
// resolution and the given alternate resolution.
func (c *Controller) ToggleResolution(resId int) {
	if c.manager.ToggleResolution(resId) {
		c.RunHook(HookAltRes, resId)
	} else {
		c.RunHook(HookNormalRes, resId)
	}
}

// ResetInstance attempts to reset the given instance and returns whether or
// not the reset was successful.
func (c *Controller) ResetInstance() bool {
	return c.manager.Reset()
}

// RunHook runs the hook of the given type if it exists.
func (c *Controller) RunHook(hook int, hookId int) {
	if hookId >= len(c.hooks[hook]) {
		log.Error("RunHook: hook id %d out of bounds", hookId)
		return
	}
	cmdStr := c.hooks[hook][hookId]
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
			log.Error("RunHook (%d) failed: %s", hook, err)
		}
	}()
}

// run runs the main loop for the controller.
func (c *Controller) run() error {
	for {
		select {
		case sig := <-c.signals:
			switch sig {
			case syscall.SIGINT, syscall.SIGTERM:
				log.Info("Shutting down.")
				return nil
			case syscall.SIGUSR1:
				c.dbg.printAll()
			}
		case err, ok := <-c.x11Errors:
			if !ok {
				return fmt.Errorf("fatal X error: %w", err)
			}
			log.Error("X error: %s", err)
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
			log.Error("inputManager: Query keymap failed: %s", err)
			continue
		}

		var pointer x11.Pointer

		window := i.x.GetActiveWindow()
		if window != i.lastFailWindow {
			pointer, err = i.x.QueryPointer(window)
			if err != nil {
				log.Error("inputManager: Query pointer failed: %s", err)
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
