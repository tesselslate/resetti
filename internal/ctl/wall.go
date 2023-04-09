package ctl

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jezek/xgb/xproto"
	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/obs"
	"github.com/woofdoggo/resetti/internal/x11"
)

// Wall implements a standard "Wall" interface, where the user can see all of
// their instances on an OBS projector and manage them from there.
type Wall struct {
	host *Controller
	conf *cfg.Profile
	obs  *obs.Client
	x    *x11.Client

	instances []mc.InstanceInfo
	states    []mc.State
	locks     []bool
	active    int // Active instance. -1 is a sentinel for wall

	proj projectorState

	wallBinds   bool
	lastMouseId int
}

// projectorState contains information about the state of the projector.
type projectorState struct {
	// Wall size (in instances)
	wallWidth, wallHeight int

	// Instance size (in pixels)
	instWidth, instHeight int

	// Projector window size
	winWidth, winHeight int

	// OBS canvas size
	baseWidth, baseHeight int

	// Section of the projector window that contains the wall
	size     cfg.Rectangle
	window   xproto.Window
	children []xproto.Window
}

// Setup implements Frontend.
func (w *Wall) Setup(deps frontendDependencies) error {
	w.host = deps.host
	w.conf = deps.conf
	w.obs = deps.obs
	w.x = deps.x

	w.active = -1
	w.lastMouseId = -1
	w.instances = make([]mc.InstanceInfo, len(deps.instances))
	w.states = make([]mc.State, len(deps.states))
	w.locks = make([]bool, len(deps.states))
	copy(w.instances, deps.instances)
	copy(w.states, deps.states)

	err := w.obs.Batch(obs.SerialRealtime, func(b *obs.Batch) {
		for i := 1; i <= len(w.instances); i += 1 {
			settings := obs.StringMap{
				"show_cursor":    false,
				"capture_window": strconv.Itoa(int(w.instances[i-1].Wid)),
			}
			b.SetItemVisibility("Wall", fmt.Sprintf("Lock %d", i), false)
			b.SetItemVisibility("Wall", fmt.Sprintf("Wall MC %d", i), true)
			b.SetSourceSettings(fmt.Sprintf("Wall MC %d", i), settings, true)
			b.SetSourceSettings(fmt.Sprintf("MC %d", i), settings, true)
		}
	})
	if err != nil {
		return fmt.Errorf("obs setup: %w", err)
	}
	if err = w.getWallSize(); err != nil {
		return fmt.Errorf("get wall size: %w", err)
	}
	if err = w.focusProjector(); err != nil {
		return fmt.Errorf("focus projector: %w", err)
	}
	if err = w.obs.SetScene("Wall"); err != nil {
		return fmt.Errorf("set scene: %w", err)
	}

	if err = w.bindWallKeys(); err != nil {
		return fmt.Errorf("bind keys: %w", err)
	}
	w.deleteSleepbgLock(true)

	return nil
}

// FocusChange implements Frontend.
func (w *Wall) FocusChange(evt x11.FocusEvent) {
	if err := w.findProjector(); err != nil {
		log.Printf("FocusChange: Failed to find projector: %s\n", err)
		return
	}

	if evt.Window == w.proj.window && !w.wallBinds {
		if err := w.bindWallKeys(); err != nil {
			log.Printf("FocusChange: Failed to bind keys: %s\n", err)
		}
	} else if evt.Window != w.proj.window && w.wallBinds {
		w.unbindWallKeys()
	}
}

// Input implements Frontend.
func (w *Wall) Input(input Input) {
	actions := w.conf.Keybinds[input.Bind]
	if w.active != -1 {
		for _, action := range actions.IngameActions {
			switch action.Type {
			case cfg.ActionIngameReset:
				w.resetIngame()
			case cfg.ActionIngameFocus:
				w.host.FocusInstance(w.active)
			}
		}
	} else {
		for _, action := range actions.WallActions {
			switch action.Type {
			case cfg.ActionWallFocus:
				if err := w.focusProjector(); err != nil {
					log.Printf("Input: Failed to focus projector: %s\n", err)
				}
			case cfg.ActionWallResetAll:
				if !w.wallBinds || input.Held {
					continue
				}
				w.wallResetAll()
			case cfg.ActionWallPlayFirstLocked:
				if !w.wallBinds || input.Held {
					continue
				}
				w.playFirstLocked()
			case cfg.ActionWallLock, cfg.ActionWallPlay, cfg.ActionWallReset, cfg.ActionWallResetOthers:
				if !w.wallBinds {
					continue
				}
				var id int
				if action.Extra != nil {
					if input.Held {
						continue
					}
					id = *action.Extra
				} else {
					mouseId, ok := w.getInstanceId(input)
					if !ok {
						w.unbindWallKeys()
						continue
					}
					if input.Held && mouseId == w.lastMouseId {
						continue
					}
					id = mouseId
					w.lastMouseId = id
				}
				if id < 0 || id > len(w.instances)-1 {
					continue
				}
				if id == -1 {
					continue
				}
				switch action.Type {
				case cfg.ActionWallLock:
					w.wallLock(id)
				case cfg.ActionWallPlay:
					if w.states[id].Type == mc.StIdle {
						w.wallPlay(id)
					}
				case cfg.ActionWallReset:
					w.wallReset(id)
				case cfg.ActionWallResetOthers:
					if w.states[id].Type == mc.StIdle {
						w.wallResetOthers(id)
					}
				}
			}
		}
	}
}

// Update implements Frontend.
func (w *Wall) Update(update mc.Update) {
	w.states[update.Id] = update.State
}

// bindWallKeys binds the keys that are only used on the wall projector.
func (w *Wall) bindWallKeys() error {
	if w.wallBinds {
		return nil
	}
	log.Println("Binding wall keys")
	w.wallBinds = true
	if err := w.host.BindWallKeys(); err != nil {
		return fmt.Errorf("host bind keys: %w", err)
	}

	// The pointer grab can fail in some scenarios. Retry with exponential
	// backoff.
	// TODO: Can this be made better? Listen for pointer grab release?
	// TODO: Only grab pointer if mouse-dependent keys are in config
	timeout := time.Millisecond
	for tries := 1; tries <= 5; tries += 1 {
		if err := w.x.GrabPointer(w.proj.window); err != nil {
			log.Printf("Pointer grab failed (%d/5): %s\n", tries, err)
		} else {
			return nil
		}
		time.Sleep(timeout)
		timeout *= 4
	}
	return errors.New("pointer grab failed after 5 tries")
}

// createSleepbgLock creates the sleepbg.lock file.
func (w *Wall) createSleepbgLock() {
	file, err := os.Create(w.conf.Wall.Perf.SleepbgPath)
	if err != nil {
		log.Printf("Failed to create sleepbg.lock: %s\n", err)
	} else {
		_ = file.Close()
	}
}

// deleteSleepbgLock deletes the sleepbg.lock file.
func (w *Wall) deleteSleepbgLock(ignoreErrors bool) {
	err := os.Remove(w.conf.Wall.Perf.SleepbgPath)
	if err != nil && !ignoreErrors {
		log.Printf("Failed to delete sleepbg.lock: %s\n", err)
	}
}

// findProjector finds the wall projector.
func (w *Wall) findProjector() error {
	windows := w.x.GetWindowList()
	for _, win := range windows {
		title, err := w.x.GetWindowTitle(win)
		if err != nil {
			continue
		}
		if strings.Contains(title, "Projector (Scene) - Wall") {
			w.proj.window = win
			width, height, err := w.x.GetWindowSize(win)
			if err != nil {
				return fmt.Errorf("get projector size: %w", err)
			}
			w.proj.children = w.x.GetWindowChildren(win)

			// Calculate projector letterboxing. Reference:
			// https://github.com/obsproject/obs-studio/blob/1b708b312e00595277dbf871f8488820cba4540a/UI/display-helpers.hpp#L23
			// https://github.com/obsproject/obs-studio/blob/1b708b312e00595277dbf871f8488820cba4540a/UI/window-projector.cpp#L180
			w.proj.winWidth, w.proj.winHeight = int(width), int(height)
			baseRatio := float64(w.proj.baseWidth) / float64(w.proj.baseHeight)
			projRatio := float64(w.proj.winWidth) / float64(w.proj.winHeight)
			var scale float64
			if projRatio > baseRatio {
				scale = float64(w.proj.winHeight) / float64(w.proj.baseHeight)
			} else {
				scale = float64(w.proj.winWidth) / float64(w.proj.baseWidth)
			}
			w.proj.size.X = uint32(w.proj.winWidth/2) - (w.proj.size.W / 2)
			w.proj.size.Y = uint32(w.proj.winHeight/2) - (w.proj.size.H / 2)
			w.proj.size.W = uint32(scale * float64(w.proj.baseWidth))
			w.proj.size.H = uint32(scale * float64(w.proj.baseHeight))
			w.proj.instWidth, w.proj.instHeight = int(w.proj.size.W)/w.proj.wallWidth, int(w.proj.size.H)/w.proj.wallHeight
			return nil
		}
	}
	return errors.New("no projector found")
}

// focusProjector finds the wall projector and focuses it.
func (w *Wall) focusProjector() error {
	if err := w.findProjector(); err != nil {
		return fmt.Errorf("find projector: %w", err)
	}
	if err := w.x.FocusWindow(w.proj.window); err != nil {
		return fmt.Errorf("focus projector: %w", err)
	}
	return nil
}

// getInstanceId returns the ID of the instance at the specified coordinates.
func (w *Wall) getInstanceId(input Input) (id int, ok bool) {
	x := (input.X - int(w.proj.size.X)) / w.proj.instWidth
	y := (input.Y - int(w.proj.size.Y)) / w.proj.instHeight
	if x < 0 || y < 0 || x >= w.proj.wallWidth || y >= w.proj.wallHeight {
		return 0, false
	}
	id = y*w.proj.wallWidth + x
	return id, id < len(w.instances)
}

// getWallSize finds the size of the wall and stores it in the Wall object.
func (w *Wall) getWallSize() error {
	appendUnique := func(slice []float64, item float64) []float64 {
		for _, v := range slice {
			if item == v {
				return slice
			}
		}
		return append(slice, item)
	}
	var xs, ys []float64
	for i := 1; i <= len(w.instances); i += 1 {
		x, y, _, _, err := w.obs.GetSceneItemTransform(
			"Wall",
			fmt.Sprintf("Wall MC %d", i),
		)
		if err != nil {
			return err
		}
		xs = appendUnique(xs, x)
		ys = appendUnique(ys, y)
	}
	w.proj.wallWidth, w.proj.wallHeight = len(xs), len(ys)
	width, height, err := w.obs.GetCanvasSize()
	if err != nil {
		return err
	}
	w.proj.baseWidth, w.proj.baseHeight = width, height
	return nil
}

// resetIngame resets the active instance.
func (w *Wall) resetIngame() {
	w.host.ResetInstance(w.active)
	w.active = -1
	if w.conf.Wall.GotoLocked && w.playFirstLocked() {
		return
	}
	if err := w.focusProjector(); err != nil {
		log.Printf("resetIngame: Failed to focus projector: %s\n", err)
	}
	w.deleteSleepbgLock(false)
	w.obs.SetSceneAsync("Wall")
	w.host.RunHook(HookReset)
}

// playFirstLocked plays the first instance that is locked
func (w *Wall) playFirstLocked() bool {
	for id, state := range w.states {
		if state.Type == mc.StIdle && w.locks[id] {
			w.wallPlay(id)
			return true
		}
	}
	return false
}

// setLocked sets the lock state of the given instance.
func (w *Wall) setLocked(id int, lock bool) {
	if w.locks[id] == lock {
		return
	}
	w.locks[id] = lock
	w.host.SetPriority(id, lock)
	w.obs.SetSceneItemVisibleAsync("Wall", fmt.Sprintf("Lock %d", id+1), lock)
}

// unbindWallKeys unbinds the keys that are only used on the wall projector.
func (w *Wall) unbindWallKeys() {
	if !w.wallBinds {
		return
	}
	log.Println("Unbinding wall keys")
	w.wallBinds = false
	w.host.UnbindWallKeys()
	if err := w.x.UngrabPointer(); err != nil {
		log.Printf("unbindWallKeys: Failed to ungrab pointer: %s\n", err)
	}
}

// wallLock toggles the lock state of the given instance.
func (w *Wall) wallLock(id int) {
	lock := !w.locks[id]
	w.setLocked(id, lock)
	if lock {
		w.host.RunHook(HookLock)
	} else {
		w.host.RunHook(HookUnlock)
	}
}

// wallPlay plays the given instance. It is the caller's responsibility to check
// if the instance is in a valid state for playing.
func (w *Wall) wallPlay(id int) {
	w.active = id
	w.unbindWallKeys()
	w.host.PlayInstance(id)
	if err := w.host.BindInstanceKeys(); err != nil {
		log.Printf("wallPlay: Failed to bind instance keys: %s\n", err)
	}

	w.host.RunHook(HookWallPlay)
	w.obs.BatchAsync(obs.SerialRealtime, func(b *obs.Batch) {
		for i := 1; i <= len(w.instances); i += 1 {
			b.SetItemVisibility("Instance", fmt.Sprintf("MC %d", i), i-1 == id)
		}
	})
	w.obs.SetSceneAsync("Instance")
	if w.locks[id] {
		w.setLocked(id, false)
	}
	w.createSleepbgLock()
}

// wallReset resets the given instance.
func (w *Wall) wallReset(id int) {
	if w.locks[id] {
		return
	}
	if w.states[id].Type != mc.StIngame && w.host.ResetInstance(id) {
		w.host.RunHook(HookWallReset)
	}
}

// wallResetAll resets all unlocked instances.
func (w *Wall) wallResetAll() {
	start := time.Now()
	for i := 0; i < len(w.instances); i += 1 {
		w.wallReset(i)
	}
	log.Printf("Reset all in %.2f ms\n", float64(time.Since(start).Microseconds())/1000)
}

// wallResetOthers plays an instance and resets all others. It is the caller's
// responsibility to check that the instance is in a valid state for playing.
func (w *Wall) wallResetOthers(id int) {
	w.wallPlay(id)
	for i := 0; i < len(w.instances); i += 1 {
		if i != id {
			w.wallReset(i)
		}
	}
}
