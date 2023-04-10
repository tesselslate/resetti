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
	"golang.org/x/exp/slices"
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

	proj  projectorState
	hider hider

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
	active   bool
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
				"show_cursor":    true,
				"capture_window": strconv.Itoa(int(w.instances[i-1].Wid)),
			}
			wallSettings := obs.StringMap{
				"show_cursor":    false,
				"capture_window": strconv.Itoa(int(w.instances[i-1].Wid)),
			}
			b.SetItemVisibility("Wall", fmt.Sprintf("Lock %d", i), false)
			b.SetItemVisibility("Wall", fmt.Sprintf("Wall MC %d", i), true)
			b.SetSourceSettings(fmt.Sprintf("Wall MC %d", i), wallSettings, true)
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
	if w.conf.Wall.Hiding.ShowMethod != "" {
		w.hider = newHider(deps.conf, deps.obs, deps.states)
		go w.hider.Run()
	}

	w.deleteSleepbgLock(true)

	return nil
}

// FocusChange implements Frontend.
func (w *Wall) FocusChange(win xproto.Window) {
	if err := w.findProjector(); err != nil {
		log.Printf("FocusChange: Failed to find projector: %s\n", err)
		return
	}
	w.proj.active = slices.Contains(w.proj.children, win)
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
			// wall_focus_projector is the only wall action that can be taken
			// while the projector isn't focused.
			if action.Type == cfg.ActionWallFocus {
				if err := w.focusProjector(); err != nil {
					log.Printf("Input: Failed to focus projector: %s\n", err)
				}
			}
			if w.active != -1 || !w.proj.active {
				continue
			}

			switch action.Type {
			case cfg.ActionWallResetAll:
				if input.Held {
					continue
				}
				w.wallResetAll()
			case cfg.ActionWallPlayFirstLocked:
				if input.Held {
					continue
				}
				w.playFirstLocked()
			case cfg.ActionWallLock, cfg.ActionWallPlay, cfg.ActionWallReset, cfg.ActionWallResetOthers:
				var id int
				if action.Extra != nil {
					if input.Held {
						continue
					}
					id = *action.Extra
				} else {
					mouseId, ok := w.getInstanceId(input)
					if !ok {
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
	w.hider.Update(update)
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
	w.host.PlayInstance(id)

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
