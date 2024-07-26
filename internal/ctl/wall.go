package ctl

import (
	"fmt"
	"math"
	"time"

	"github.com/tesselslate/resetti/internal/cfg"
	"github.com/tesselslate/resetti/internal/log"
	"github.com/tesselslate/resetti/internal/mc"
	"github.com/tesselslate/resetti/internal/obs"
	"github.com/tesselslate/resetti/internal/x11"
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

	proj    ProjectorController
	freezer *freezer
	hider   *hider
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
	w.obs.SetScene("Wall")
	if w.conf.Wall.FreezeAt > 0 {
		w.freezer = newFreezer(deps.conf, deps.obs, deps.states)
	}
	if w.conf.Wall.ShowAt >= 0 {
		w.hider = newHider(deps.conf, deps.obs, deps.states)
	}

	// Often, the user will not have an existing sleepbg.lock file from their
	// last session, so ignore any errors on the first deletion.
	w.host.DeleteSleepbgLock(true)

	return nil
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
				if w.x.GetActiveWindow() == w.instances[w.active].Wid {
					w.resetIngame()
				}
			case cfg.ActionIngameFocus:
				w.host.FocusInstance(w.active)
			case cfg.ActionIngameRes:
				if action.Extra != nil {
					resId := *action.Extra
					if resId < 0 || resId > len(w.conf.AltRes)-1 {
						continue
					}
					w.host.ToggleResolution(w.active, resId)
				} else {
					w.host.ToggleResolution(w.active, 0)
				}
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
					log.Error("Input: Failed to focus projector: %s", err)
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

// ProcessEvent implements Frontend.
func (w *Wall) ProcessEvent(evt x11.Event) {
	w.proj.ProcessEvent(evt)
}

// Update implements Frontend.
func (w *Wall) Update(update mc.Update) {
	w.states[update.Id] = update.State
	if w.freezer != nil {
		w.freezer.Update(update)
	}
	if w.hider != nil {
		if w.hider.ShouldShow(update) {
			x := update.Id % w.wallWidth
			y := update.Id / w.wallWidth
			w.obs.SetSceneItemBounds(
				"Wall",
				fmt.Sprintf("Wall MC %d", update.Id+1),
				float64(x*w.instWidth),
				float64(y*w.instHeight),
				float64(w.instWidth),
				float64(w.instHeight),
			)
		}
	}
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

// getWallSize checks that the layout of the wall is valid and finds the size
// of the wall in instances. If it is not valid, it prompts the user to update
// the wall layout.
func (w *Wall) getWallSize() error {
	// Get the wall size (assuming the layout is correct.)
	var xs []float64
	var ys []float64
	var width, height int
	validLayout := true
	for i := 1; i <= len(w.instances); i += 1 {
		x, y, w, h, err := w.obs.GetSceneItemTransform("Wall", fmt.Sprintf("Wall MC %d", i))
		if err != nil {
			return err
		}
		xs = append(xs, x)
		ys = append(ys, y)
		if (width != 0 && int(w) != width) || (height != 0 && int(h) != height) {
			validLayout = false
			break
		}
		width, height = int(w), int(h)
	}

	// Check that the layout is correct.
	if validLayout {
		w.wallWidth = w.proj.BaseWidth / width
		w.wallHeight = w.proj.BaseHeight / height
		const epsilon = 0.01

		for i := 0; i < len(w.instances); i += 1 {
			x := i % w.wallWidth
			y := i / w.wallWidth
			cx := math.Abs(xs[i]-float64(width*x)) <= epsilon
			cy := math.Abs(ys[i]-float64(height*y)) <= epsilon
			if !cx || !cy {
				validLayout = false
				break
			}
		}
	}

	if validLayout {
		w.instWidth = w.proj.BaseWidth / w.wallWidth
		w.instHeight = w.proj.BaseHeight / w.wallHeight
		log.Info("Found wall size: %dx%d", w.wallWidth, w.wallHeight)
		return nil
	} else {
		return w.promptWallSize()
	}
}

// promptWallSize prompts the user to enter a wall size and readjusts the
// layout automatically.
func (w *Wall) promptWallSize() error {
	fmt.Println("It seems like your wall layout is incorrect. Please enter the new wall size you would like to adjust to. (WxH)")
	var width, height int
	for {
		n, err := fmt.Scanf("%dx%d", &width, &height)
		if n == 2 {
			if width*height >= len(w.instances) {
				break
			}
			fmt.Printf("That wall size is not big enough. You need to fit %d instances.\n", len(w.instances))
		}
		if err != nil {
			fmt.Println("There was an error reading your input:", err)
		}
		fmt.Println("Your input did not follow the correct format. (WxH)")
	}
	w.wallWidth, w.wallHeight = width, height
	w.instWidth = w.proj.BaseWidth / w.wallWidth
	w.instHeight = w.proj.BaseHeight / w.wallHeight
	err := w.obs.Batch(obs.SerialFrame, func(b *obs.Batch) {
		for i := range w.instances {
			x := i % width
			y := i / width
			b.SetItemBounds(
				"Wall",
				fmt.Sprintf("Wall MC %d", i+1),
				float64(x*w.instWidth),
				float64(y*w.instHeight),
				float64(w.instWidth),
				float64(w.instHeight),
			)
		}
	})
	return err
}

// resetIngame resets the active instance.
func (w *Wall) resetIngame() {
	w.host.ResetInstance(w.active)
	w.host.RunHook(HookReset, 0)
	if w.freezer != nil {
		w.freezer.Unfreeze(w.active)
	}
	if w.hider != nil {
		w.obs.SetSceneItemBounds(
			"Wall",
			fmt.Sprintf("Wall MC %d", w.active+1),
			float64(w.proj.BaseWidth),
			float64(w.proj.BaseHeight),
			1,
			1,
		)
	}
	w.active = -1
	if w.conf.Wall.GotoLocked && w.playFirstLocked() {
		return
	}
	if err := w.proj.Focus(); err != nil {
		log.Error("resetIngame: Failed to focus projector: %s", err)
	}
	w.host.DeleteSleepbgLock(false)
	w.obs.SetScene("Wall")
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
	w.obs.SetSceneItemVisible("Wall", fmt.Sprintf("Lock %d", id+1), lock)
}

// wallLock toggles the lock state of the given instance.
func (w *Wall) wallLock(id int) {
	lock := !w.locks[id]
	if w.freezer != nil {
		w.freezer.SetCanFreeze(id, !lock)
	}
	w.setLocked(id, lock)
	if lock {
		w.host.RunHook(HookLock, 0)
	} else {
		w.host.RunHook(HookUnlock, 0)
		if w.conf.Wall.ResetUnlock {
			w.wallReset(id)
		}
	}
}

// wallPlay plays the given instance. It is the caller's responsibility to check
// if the instance is in a valid state for playing.
func (w *Wall) wallPlay(id int) {
	w.active = id
	w.proj.Unfocus()
	w.host.PlayInstance(id)

	w.host.RunHook(HookWallPlay, 0)
	w.obs.BatchAsync(obs.SerialRealtime, func(b *obs.Batch) {
		for i := 1; i <= len(w.instances); i += 1 {
			b.SetItemVisibility("Instance", fmt.Sprintf("MC %d", i), i-1 == id)
		}
	})
	w.obs.SetScene("Instance")
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
		if w.freezer != nil {
			w.freezer.Unfreeze(id)
		}
		if w.hider != nil {
			w.obs.SetSceneItemBounds(
				"Wall",
				fmt.Sprintf("Wall MC %d", id+1),
				float64(w.proj.BaseWidth),
				float64(w.proj.BaseHeight),
				1,
				1,
			)
		}
		w.host.RunHook(HookWallReset, 0)
	}
}

// wallResetAll resets all unlocked instances.
func (w *Wall) wallResetAll() {
	start := time.Now()
	for i := 0; i < len(w.instances); i += 1 {
		w.wallReset(i)
	}

	log.Info("Reset all in %.2f ms", float64(time.Since(start).Microseconds())/1000)
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
