package ctl

import (
	"fmt"
	"log"
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

	instances             []mc.InstanceInfo // List of instance metadata
	states                []mc.State        // List of instance states
	locks                 []bool            // Which instances are locked
	active                int               // Active instance. -1 is a sentinel for wall
	instWidth, instHeight int               // The size of each instance on the OBS scene.
	wallWidth, wallHeight int               // The size of the wall, in instances.
	lastMouseId           int               // The ID of the last instance a mouse action was done on.

	proj  ProjectorController
	hider hider
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
	if err := w.proj.Setup(deps.conf, deps.obs, deps.x); err != nil {
		return fmt.Errorf("projector setup: %w", err)
	}
	if err := prepareObs(deps.obs, deps.instances); err != nil {
		return fmt.Errorf("obs setup: %w", err)
	}
	if err := w.getWallSize(); err != nil {
		return fmt.Errorf("get wall size: %w", err)
	}
	if err := w.proj.Focus(); err != nil {
		return fmt.Errorf("focus projector: %w", err)
	}
	if err := w.obs.SetScene("Wall"); err != nil {
		return fmt.Errorf("set scene: %w", err)
	}
	if w.conf.Wall.Hiding.ShowMethod != "" {
		w.hider = newHider(deps.conf, deps.obs, deps.states)
		go w.hider.Run()
	}

	// Often, the user will not have an existing sleepbg.lock file from their
	// last session, so ignore any errors on the first deletion.
	w.host.DeleteSleepbgLock(true)

	return nil
}

// FocusChange implements Frontend.
func (w *Wall) FocusChange(win xproto.Window) {
	w.proj.FocusChange(win)
}

// Input implements Frontend.
func (w *Wall) Input(input Input) {
	actions := w.conf.Keybinds[input.Bind]
	if w.active != -1 {
		if input.Held {
			return
		}
		for _, action := range actions.IngameActions {
			switch action.Type {
			case cfg.ActionIngameReset:
				w.resetIngame()
			case cfg.ActionIngameFocus:
				w.host.FocusInstance(w.active)
			case cfg.ActionIngameThin:
				w.host.ToggleThinInstance(w.active)
			}
		}
	} else {
		for _, action := range actions.WallActions {
			// wall_focus_projector is the only wall action that can be taken
			// while the projector isn't focused.
			if action.Type == cfg.ActionWallFocus {
				if input.Held {
					continue
				}
				if err := w.proj.Focus(); err != nil {
					log.Printf("Input: Failed to focus projector: %s\n", err)
				}
			}
			if w.active != -1 || !w.proj.Active {
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
					// Only accept mouse-based inputs if the projector is focused.
					if !w.proj.Active {
						continue
					}
					// Ungrab the pointer if the user clicks outside of
					// the projector.
					if !w.proj.InBounds(input.X, input.Y) {
						w.proj.Unfocus()
					}
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
	// TODO guard
	w.hider.Update(update)
}

// getInstanceId returns the ID of the instance at the specified coordinates.
func (w *Wall) getInstanceId(input Input) (id int, ok bool) {
	x, y := w.proj.ToVideo(input.X, input.Y)
	x /= w.instWidth
	y /= w.instHeight
	if x < 0 || y < 0 || x >= w.wallWidth || y >= w.wallHeight {
		return 0, false
	}
	id = y*w.wallWidth + x
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
	w.wallWidth, w.wallHeight = len(xs), len(ys)
	width, height, err := w.obs.GetCanvasSize()
	if err != nil {
		return err
	}
	w.instWidth, w.instHeight = width/w.wallWidth, height/w.wallHeight
	return nil
}

// resetIngame resets the active instance.
func (w *Wall) resetIngame() {
	w.host.ResetInstance(w.active)
	w.active = -1
	if w.conf.Wall.GotoLocked && w.playFirstLocked() {
		return
	}
	if err := w.proj.Focus(); err != nil {
		log.Printf("resetIngame: Failed to focus projector: %s\n", err)
	}
	w.host.DeleteSleepbgLock(false)
	w.obs.SetSceneAsync("Wall")
	w.host.RunHook(HookReset)
}

// playFirstLocked plays the first idle, locked instance.
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
	w.proj.Unfocus()
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
	w.host.CreateSleepbgLock()
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
