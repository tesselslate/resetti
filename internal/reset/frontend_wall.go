package reset

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jezek/xgb/xproto"
	"github.com/pkg/errors"
	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/obs"
	"github.com/woofdoggo/resetti/internal/reset/wall"
	"github.com/woofdoggo/resetti/internal/x11"
)

// FrontendWall implements a generic "wall" frontend which delegates much of
// the input/output handling to an OBS controller.
type FrontendWall struct {
	conf *cfg.Profile
	host *Controller
	obs  *obs.Client
	x    *x11.Client

	obsController wall.ObsController
	projector     xproto.Window
	subprojector  []xproto.Window
	lastMouseId   int
	grabbed       bool

	active    int
	instances []mc.Instance
	states    []mc.InstanceState
	locks     []bool
}

func (f *FrontendWall) HandleInput(event x11.Event) error {
	switch event := event.(type) {
	case x11.KeyEvent:
		if event.State == x11.StateUp {
			break
		}
		switch event.Key {
		case f.conf.Keys.Focus:
			if f.active == -1 {
				return f.focusProjector()
			} else {
				f.instances[f.active].Focus()
			}
		case f.conf.Keys.Reset:
			if f.active == -1 {
				toReset := f.obsController.GetResetAllInstances()
				for _, id := range toReset {
					f.wallReset(id)
				}
			} else {
				if err := f.instances[f.active].Stretch(f.conf); err != nil {
					return err
				}
				go runHook(f.conf.Hooks.Reset)
				f.host.ResetInstance(f.active, f.x.GetCurrentTime())
				time.Sleep(time.Millisecond * time.Duration(f.conf.Reset.Delay))
				if f.conf.Wall.GoToLocked {
					for idx, state := range f.states {
						if f.locks[idx] && state.State == mc.StIdle {
							return f.wallPlay(idx)
						}
					}
				}
				return f.gotoWall()
			}
		default:
			if f.active != -1 {
				break
			}
			list := f.obsController.GetResetAllInstances()
			id := int(event.Key.Code - 10)
			if id < 0 || id > len(list) {
				break
			}
			return f.handleInput(id, event.Key.Mod)
		}
	case x11.MoveEvent:
		if f.active != -1 {
			break
		}
		for _, win := range f.subprojector {
			if win == event.Window {
				if event.Mod&xproto.ButtonMask1 == 0 {
					break
				}
				id := f.obsController.GetInstanceId(int(event.Point.X), int(event.Point.Y))
				if f.lastMouseId == id {
					break
				}
				f.lastMouseId = id
				return f.handleInput(id, event.Mod^xproto.ButtonMask1)
			}
		}
	case x11.ButtonEvent:
		if f.active != -1 {
			break
		}
		found := false
		for _, win := range f.subprojector {
			if win == event.Window {
				found = true
				break
			}
		}
		if !found {
			return f.ungrabKeys()
		}
		id := f.obsController.GetInstanceId(int(event.Point.X), int(event.Point.Y))
		f.lastMouseId = id
		return f.handleInput(id, event.Mod)
	case x11.FocusEvent:
		if err := f.findProjector(); err != nil {
			return err
		}
		if event.Window == f.projector && !f.grabbed && f.active == -1 {
			return f.grabKeys()
		} else if event.Window != f.projector && f.grabbed {
			return f.ungrabKeys()
		}
	}
	return nil
}

func (f *FrontendWall) HandleUpdate(update mc.Update) error {
	f.states[update.Id] = update.State
	return f.obsController.Update(update)
}

func (f *FrontendWall) Setup(opts FrontendOptions) error {
	f.conf = opts.Conf
	f.host = opts.Controller
	f.obs = opts.Obs
	f.x = opts.X
	f.active = -1
	f.lastMouseId = -1
	f.instances = make([]mc.Instance, len(opts.Instances))
	f.states = make([]mc.InstanceState, len(opts.Instances))
	f.locks = make([]bool, len(opts.Instances))
	copy(f.instances, opts.Instances)
	copy(f.states, opts.States)
	err := opts.X.GrabKey(f.conf.Keys.Reset, opts.X.GetRootWindow())
	if err != nil {
		return err
	}
	err = opts.X.GrabKey(f.conf.Keys.Focus, opts.X.GetRootWindow())
	if err != nil {
		return err
	}

	// OBS setup.
	err = f.obs.Batch(obs.SerialRealtime, func(b *obs.Batch) error {
		for i := 1; i <= len(f.instances); i += 1 {
			b.SetItemVisibility("Wall", fmt.Sprintf("Lock %d", i), false)
			b.SetItemVisibility("Wall", fmt.Sprintf("Wall MC %d", i), true)
			crops := strings.Split(f.conf.AdvancedWall.CropInstances, "x")
			var cLeft, cRight, cTop, cBot int
			if len(crops) == 4 {
				if cLeft, err = strconv.Atoi(crops[0]); err != nil {
					return err
				}
				if cRight, err = strconv.Atoi(crops[1]); err != nil {
					return err
				}
				if cTop, err = strconv.Atoi(crops[2]); err != nil {
					return err
				}
				if cBot, err = strconv.Atoi(crops[3]); err != nil {
					return err
				}
			}
			settings := obs.StringMap{
				"show_cursor":    false,
				"capture_window": strconv.Itoa(int(f.instances[i-1].Wid)),
				"cut_left":       cLeft,
				"cut_right":      cRight,
				"cut_top":        cTop,
				"cut_bot":        cBot,
			}
			b.SetSourceSettings(fmt.Sprintf("MC %d", i), settings, true)
			b.SetSourceSettings(fmt.Sprintf("Wall MC %d", i), settings, true)
			b.SetItemVisibility("Wall", fmt.Sprintf("Wall MC %d", i), true)
			if f.conf.AdvancedWall.PreviewFreezing {
				b.SetSourceFilterEnabled(
					fmt.Sprintf("Wall MC %d", i),
					fmt.Sprintf("Freeze %d", i),
					false,
				)
			}
		}
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "obs setup")
	}
	if err = f.obs.SetScene("Wall"); err != nil {
		return err
	}
	// TODO: Different OBS controllers
	f.obsController = &wall.StandardController{}
	if err = f.obsController.Setup(f.obs, f.conf, f.states); err != nil {
		return err
	}
	if err = f.focusProjector(); err != nil {
		return err
	}
	if err = f.grabKeys(); err != nil {
		return err
	}

	// Delete sleepbg.lock.
	return f.toggleSleepbg(false)
}

func (f *FrontendWall) ShouldPause(id int) bool {
	return f.active != id
}

// findProjector finds the OBS projector.
func (f *FrontendWall) findProjector() error {
	windows, err := f.x.GetWindowList()
	if err != nil {
		return err
	}
	for _, win := range windows {
		title, err := f.x.GetWindowTitle(win)
		if err != nil {
			continue
		}
		if strings.Contains(title, "Projector (Scene) - Wall") {
			f.projector = win
			width, height, err := f.x.GetWindowSize(f.projector)
			if err != nil {
				return err
			}
			f.obsController.UpdateProjector(int(width), int(height))
			return nil
		}
	}
	return errors.New("no projector found")
}

// focusProjector finds the OBS projector and switches focus to it.
func (f *FrontendWall) focusProjector() error {
	if err := f.findProjector(); err != nil {
		return err
	}
	subwindows, err := f.x.GetWindowChildren(f.projector)
	if err != nil {
		return err
	}
	f.subprojector = subwindows
	return f.x.FocusWindow(f.projector)
}

// gotoWall switches focus back to the wall projector and forms all other
// necessary tasks to go back to the wall.
func (f *FrontendWall) gotoWall() error {
	f.active = -1
	f.obs.SetSceneAsync("Wall")
	f.obs.BatchAsync(obs.SerialRealtime, func(b *obs.Batch) error {
		for i := 1; i <= len(f.instances); i += 1 {
			b.SetSourceSettings(fmt.Sprintf("MC %d", i), obs.StringMap{"show_cursor": false}, true)
		}
		return nil
	})
	if err := f.focusProjector(); err != nil {
		return err
	}
	return f.toggleSleepbg(false)
}

// grabKeys grabs keys that are only used on the wall projector.
func (f *FrontendWall) grabKeys() error {
	// Grab keys.
	win := f.x.GetRootWindow()
	grabCount := len(f.instances)
	if grabCount > 10 {
		grabCount = 10
	}
	for i := 0; i < grabCount; i += 1 {
		wallMods := []x11.Keymod{
			f.conf.Keys.WallPlay,
			f.conf.Keys.WallReset,
			f.conf.Keys.WallResetOthers,
			f.conf.Keys.WallLock,
		}
		key := x11.Key{Code: xproto.Keycode(i + 10)}
		for _, v := range wallMods {
			key.Mod = v
			if err := f.x.GrabKey(key, win); err != nil {
				return err
			}
		}
	}
	f.grabbed = true

	// Grab pointer.
	if f.conf.Wall.UseMouse {
		timeout := time.Millisecond
		for tries := 0; tries < 5; tries += 1 {
			err := f.x.GrabPointer(f.projector)
			if err == nil {
				return nil
			}
			log.Printf("Pointer grab failed (attempt %d): %s\n", tries, err)
			time.Sleep(timeout)
			timeout *= 5
		}
		return errors.New("failed to grab pointer")
	}
	return nil
}

// handleInput handles a wall action input (reset, lock, etc.)
func (f *FrontendWall) handleInput(id int, mod x11.Keymod) error {
	switch mod {
	case f.conf.Keys.WallPlay:
		return f.wallPlay(id)
	case f.conf.Keys.WallReset:
		f.wallReset(id)
		return nil
	case f.conf.Keys.WallResetOthers:
		return f.wallResetOthers(id)
	case f.conf.Keys.WallLock:
		return f.setLocked(id, !f.locks[id])
	}
	panic("unreachable")
}

// setLocked sets the lock state of an instance.
func (f *FrontendWall) setLocked(id int, lock bool) error {
	if f.locks[id] == lock {
		return nil
	}
	f.host.SetInstancePriority(id, lock)
	f.locks[id] = lock
	if lock {
		go runHook(f.conf.Hooks.Lock)
		return f.obsController.Lock(id)
	} else {
		go runHook(f.conf.Hooks.Unlock)
		return f.obsController.Unlock(id)
	}
}

// toggleSleepbg creates or deletes the sleepbg.lock file.
func (f *FrontendWall) toggleSleepbg(state bool) error {
	if !f.conf.Wall.SleepBgLock {
		return nil
	}
	if state {
		file, err := os.Create(f.conf.Wall.SleepBgLockPath)
		defer func() {
			_ = file.Close()
		}()
		return err
	} else {
		err := os.Remove(f.conf.Wall.SleepBgLockPath)
		if err != nil {
			log.Printf("sleepbg.lock removal failed: %s\n", err)
		}
		return nil
	}
}

// ungrabKeys ungrabs keys that are only used on the wall projector.
func (f *FrontendWall) ungrabKeys() error {
	// Ungrab keys.
	win := f.x.GetRootWindow()
	grabCount := len(f.instances)
	if grabCount > 10 {
		grabCount = 10
	}
	for i := 0; i < grabCount; i += 1 {
		wallMods := []x11.Keymod{
			f.conf.Keys.WallPlay,
			f.conf.Keys.WallReset,
			f.conf.Keys.WallResetOthers,
			f.conf.Keys.WallLock,
		}
		key := x11.Key{Code: xproto.Keycode(i + 10)}
		for _, v := range wallMods {
			key.Mod = v
			if err := f.x.UngrabKey(key, win); err != nil {
				return err
			}
		}
	}
	f.grabbed = false

	// Ungrab pointer.
	if f.conf.Wall.UseMouse {
		return f.x.UngrabPointer()
	}
	return nil
}

// wallPlay plays a single instance.
func (f *FrontendWall) wallPlay(id int) error {
	if f.states[id].State != mc.StIdle {
		return nil
	}
	// NOTE: Even though the window focus change will cause the wall grabs to
	// be released, they aren't released in time for Minecraft to grab the
	// pointer. Release them here explicitly.
	if err := f.ungrabKeys(); err != nil {
		return err
	}
	if err := f.host.PlayInstance(id); err != nil {
		return err
	}
	f.states[id].State = mc.StIngame
	f.active = id
	f.obs.SetSceneAsync("Instance")
	f.obs.BatchAsync(obs.SerialRealtime, func(b *obs.Batch) error {
		for i := 1; i <= len(f.instances); i += 1 {
			b.SetItemVisibility("Instance", fmt.Sprintf("MC %d", i), i-1 == id)
		}
		return nil
	})
	f.instances[id].FocusAndUnpause(f.x.GetCurrentTime()+10, true)
	if err := f.instances[id].Unstretch(f.conf); err != nil {
		return err
	}

	// Pause and unpause after resizing to fix the desynced menu cursor and/or
	// failed cursor grab. Block user inputs for ~20ms while this happens.
	time.Sleep(time.Millisecond * time.Duration(f.conf.Reset.Delay))
	f.instances[id].PressEsc(f.x.GetCurrentTime() + 15)
	f.instances[id].PressEsc(f.x.GetCurrentTime() + 20)
	if err := f.setLocked(id, false); err != nil {
		return err
	}
	f.obs.SetSourceSettingsAsync(
		fmt.Sprintf("MC %d", id+1),
		obs.StringMap{"show_cursor": true},
		true,
	)
	return f.toggleSleepbg(true)
}

// wallReset resets a single instance.
func (f *FrontendWall) wallReset(id int) {
	state := f.states[id].State
	if f.locks[id] || state == mc.StDirt {
		return
	}
	f.host.ResetInstance(id, f.x.GetCurrentTime())
	go runHook(f.conf.Hooks.WallReset)
}

// wallResetOthers attempts to play one instance and reset all others.
func (f *FrontendWall) wallResetOthers(id int) error {
	if f.states[id].State != mc.StIdle {
		return nil
	}
	if err := f.wallPlay(id); err != nil {
		return err
	}
	for idx := range f.instances {
		if idx != id {
			f.wallReset(idx)
		}
	}
	return nil
}
