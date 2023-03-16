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
	"github.com/woofdoggo/resetti/internal/x11"
	"golang.org/x/exp/slices"
)

// FrontendMoving implements a "moving wall" frontend.
type FrontendMoving struct {
	conf  *cfg.Profile
	host  *Controller
	obs   *obs.Client
	x     *x11.Client
	hider *hider

	projector    xproto.Window
	subprojector []xproto.Window
	projWidth    int
	projHeight   int
	lastMouseId  int
	grabbed      bool

	focusWidth     int
	focusHeight    int
	videoWidth     int
	videoHeight    int
	lockAreaHeight int
	lockAreaCount  int
	focus          []int
	lockArea       []int

	active      int
	instances   []mc.Instance
	states      []mc.InstanceState
	locks       []bool
	lastPreview []time.Time
}

func (f *FrontendMoving) HandleInput(event x11.Event) error {
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
				for _, id := range f.focus {
					if id != -1 {
						f.wallReset(id)
					}
				}
				f.fillFocusGrid(true)
				f.rerender()
			} else {
				f.instances[f.active].PressF3(f.x.GetCurrentTime())
				f.host.ResetInstance(f.active, f.x.GetCurrentTime()+5)
				if f.hider != nil {
					f.hider.Hide(f.active)
				}
				if err := f.instances[f.active].Stretch(f.conf); err != nil {
					return err
				}
				go runHook(f.conf.Hooks.Reset)
				time.Sleep(time.Millisecond * time.Duration(f.conf.Reset.Delay))
				if f.conf.Wall.GoToLocked {
					for idx, state := range f.states {
						if f.locks[idx] && state.State == mc.StIdle {
							err := f.wallPlay(idx)
							f.rerender()
							return err
						}
					}
				}
				return f.gotoWall()
			}
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
				id := f.getId(int(event.Point.X), int(event.Point.Y))
				if f.lastMouseId == id || id == -1 {
					break
				}
				// The lock area changes immediately, so allowing for dragging
				// is counterintuitive.
				if slices.Contains(f.lockArea, id) {
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
		id := f.getId(int(event.Point.X), int(event.Point.Y))
		if id == -1 {
			return nil
		}
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

func (f *FrontendMoving) HandleUpdate(update mc.Update) error {
	prev := f.states[update.Id]
	next := update.State
	if prev.State != mc.StPreview && next.State == mc.StPreview {
		f.lastPreview[update.Id] = time.Now()
		// Replace any dirt focus instances.
		for idx, id := range f.focus {
			if id == -1 {
				continue
			}
			free := !slices.Contains(f.focus, update.Id) && !slices.Contains(f.lockArea, update.Id)
			if f.states[id].State == mc.StDirt && free {
				f.focus[idx] = update.Id
			}
		}
	}
	f.states[update.Id] = next
	if f.hider != nil {
		f.hider.Update(update)
	}
	return nil
}

func (f *FrontendMoving) Setup(opts FrontendOptions) error {
	f.conf = opts.Conf
	f.host = opts.Controller
	f.obs = opts.Obs
	f.x = opts.X
	f.active = -1
	f.lastMouseId = -1
	f.instances = make([]mc.Instance, len(opts.Instances))
	f.states = make([]mc.InstanceState, len(opts.Instances))
	f.lockArea = make([]int, 0)
	f.locks = make([]bool, len(opts.Instances))
	f.lastPreview = make([]time.Time, len(opts.Instances))
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
	if f.conf.Wall.InstanceHiding {
		f.hider = NewHider(f.conf, f.obs, f.states)
	}

	// Moving wall config setup.
	n, err := fmt.Sscanf(f.conf.Moving.FocusSize, "%dx%d", &f.focusWidth, &f.focusHeight)
	if err != nil {
		return err
	}
	if n != 2 {
		return errors.New("invalid focus size")
	}
	f.focus = make([]int, f.focusWidth*f.focusHeight)
	for i := 0; i < len(f.focus); i += 1 {
		f.focus[i] = i
	}
	f.lockAreaCount = f.conf.Moving.LockAreaCount
	f.lockAreaHeight = f.conf.Moving.LockAreaHeight

	// OBS setup.
	err = f.obs.Batch(obs.SerialRealtime, func(b *obs.Batch) error {
		for i := 1; i <= len(f.instances); i += 1 {
			b.SetItemVisibility("Wall", fmt.Sprintf("Lock %d", i), false)
			settings := obs.StringMap{
				"show_cursor":    false,
				"capture_window": strconv.Itoa(int(f.instances[i-1].Wid)),
			}
			b.SetSourceSettings(fmt.Sprintf("MC %d", i), settings, true)
			b.SetSourceSettings(fmt.Sprintf("Wall MC %d", i), settings, true)
			b.SetItemVisibility("Wall", fmt.Sprintf("Wall MC %d", i), true)
		}
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "obs setup")
	}
	if err = f.focusProjector(); err != nil {
		return err
	}
	if err = f.obs.SetScene("Wall"); err != nil {
		return err
	}
	f.videoWidth, f.videoHeight, err = f.obs.GetCanvasSize()
	if err != nil {
		return err
	}
	f.rerender()
	if err = f.grabKeys(); err != nil {
		return err
	}

	// Delete sleepbg.lock.
	return f.toggleSleepbg(false)
}

func (f *FrontendMoving) ShouldPause(id int) bool {
	return f.active != id
}

// findProjector finds the OBS projector.
func (f *FrontendMoving) findProjector() error {
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
			f.projWidth, f.projHeight = int(width), int(height)
			return nil
		}
	}
	return errors.New("no projector found")
}

// focusProjector finds the OBS projector and switches focus to it.
func (f *FrontendMoving) focusProjector() error {
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

// getId returns the ID of the instance at the specified coordinates, or -1 if
// it does not exist.
func (f *FrontendMoving) getId(x, y int) int {
	if x < 0 || y < 0 || x > f.projWidth || y > f.projHeight {
		return -1
	}
	if y >= f.projHeight-f.lockAreaHeight {
		id := x / (f.projWidth / f.lockAreaCount)
		if id >= len(f.lockArea) {
			return -1
		} else {
			return f.lockArea[id]
		}
	} else {
		x /= (f.projWidth / f.focusWidth)
		y /= ((f.projHeight - f.lockAreaHeight) / f.focusHeight)
		id := y*f.focusWidth + x
		if id >= len(f.instances) {
			return -1
		} else {
			return f.focus[id]
		}
	}
}

// gotoWall switches focus back to the wall projector and forms all other
// necessary tasks to go back to the wall.
func (f *FrontendMoving) gotoWall() error {
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
func (f *FrontendMoving) grabKeys() error {
	f.grabbed = true
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
func (f *FrontendMoving) handleInput(id int, mod x11.Keymod) error {
	switch mod {
	case f.conf.Keys.WallPlay:
		err := f.wallPlay(id)
		f.rerender()
		return err
	case f.conf.Keys.WallReset:
		f.wallReset(id)
		if idx := slices.Index(f.focus, id); idx != -1 {
			f.focus[slices.Index(f.focus, id)] = -1
		}
		f.rerender()
		return nil
	case f.conf.Keys.WallResetOthers:
		return f.wallResetOthers(id)
	case f.conf.Keys.WallLock:
		if !f.locks[id] {
			f.lockArea = append(f.lockArea, id)
			f.focus[slices.Index(f.focus, id)] = -1
		} else {
			idx := slices.Index(f.lockArea, id)
			f.lockArea = slices.Delete(f.lockArea, idx, idx+1)
		}
		f.rerender()
		return f.setLocked(id, !f.locks[id])
	}
	return nil
}

// setLocked sets the lock state of an instance.
func (f *FrontendMoving) setLocked(id int, lock bool) error {
	if f.locks[id] == lock {
		return nil
	}
	f.host.SetInstancePriority(id, lock)
	f.locks[id] = lock
	if lock {
		go runHook(f.conf.Hooks.Lock)
	} else {
		go runHook(f.conf.Hooks.Unlock)
	}
	return nil
}

// toggleSleepbg creates or deletes the sleepbg.lock file.
func (f *FrontendMoving) toggleSleepbg(state bool) error {
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
func (f *FrontendMoving) ungrabKeys() error {
	f.grabbed = false
	if f.conf.Wall.UseMouse {
		return f.x.UngrabPointer()
	}
	return nil
}

// wallPlay plays a single instance.
func (f *FrontendMoving) wallPlay(id int) error {
	if f.states[id].State != mc.StIdle {
		return nil
	}
	if idx := slices.Index(f.focus, id); idx != -1 {
		f.focus[idx] = -1
		f.fillFocusGrid(false)
	} else if idx = slices.Index(f.lockArea, id); idx != -1 {
		f.lockArea = slices.Delete(f.lockArea, idx, idx+1)
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
func (f *FrontendMoving) wallReset(id int) {
	state := f.states[id].State
	f.states[id].State = mc.StDirt
	inGrace := (time.Now().UnixMilli() - f.lastPreview[id].UnixMilli()) <= int64(f.conf.Wall.GracePeriod)
	if f.locks[id] || state == mc.StDirt || inGrace {
		return
	}
	f.host.ResetInstance(id, f.x.GetCurrentTime())
	if f.hider != nil {
		f.hider.Hide(id)
	}
	go runHook(f.conf.Hooks.WallReset)
}

// wallResetOthers attempts to play one instance and reset all others.
func (f *FrontendMoving) wallResetOthers(id int) error {
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
	f.fillFocusGrid(true)
	f.rerender()
	return nil
}

// Moving wall layout code

// fillFocusGrid refills the focus grid with new instances.
func (f *FrontendMoving) fillFocusGrid(replaceAll bool) {
	nextPicks := f.pickBestInstances()
	for idx, id := range f.focus {
		if !replaceAll && id != -1 {
			continue
		}
		if len(nextPicks) == 0 {
			return
		}
		f.focus[idx] = nextPicks[0]
		nextPicks = nextPicks[1:]
	}
}

// pickBestInstances returns a list of the best instances to add to the
// focus grid.
func (f *FrontendMoving) pickBestInstances() []int {
	instances := make([]int, 0)
	for idx := 0; idx < len(f.instances); idx += 1 {
		if slices.Contains(f.focus, idx) || slices.Contains(f.lockArea, idx) {
			continue
		}
		instances = append(instances, idx)
	}
	slices.SortFunc(instances, func(a, b int) bool {
		if f.states[a].State < f.states[b].State {
			return true
		}
		if f.states[a].Progress < f.states[b].Progress {
			return true
		}
		if f.lastPreview[a].UnixMilli() > f.lastPreview[b].UnixMilli() {
			return true
		}
		return false
	})
	return instances
}

// rerender updates the OBS layout.
func (f *FrontendMoving) rerender() {
	f.obs.BatchAsync(obs.SerialRealtime, func(b *obs.Batch) error {
		visible := make([]bool, len(f.instances))
		instWidth := f.videoWidth / f.focusWidth
		instHeight := (f.videoHeight - f.lockAreaHeight) / f.focusHeight
		for idx := 0; idx < len(f.focus); idx += 1 {
			if f.focus[idx] == -1 {
				continue
			}
			y := idx / f.focusWidth
			x := idx % f.focusWidth
			b.SetItemBounds(
				"Wall",
				fmt.Sprintf("Wall MC %d", f.focus[idx]+1),
				float64(x*instWidth),
				float64(y*instHeight),
				float64(instWidth),
				float64(instHeight),
			)
			visible[f.focus[idx]] = true
		}
		instWidth = f.videoWidth / f.lockAreaCount
		for idx := 0; idx < len(f.lockArea); idx += 1 {
			b.SetItemBounds(
				"Wall",
				fmt.Sprintf("Wall MC %d", f.lockArea[idx]+1),
				float64(idx*instWidth),
				float64(f.videoHeight-f.lockAreaHeight),
				float64(instWidth),
				float64(f.lockAreaHeight),
			)
			visible[f.lockArea[idx]] = true
		}

		for idx, visible := range visible {
			if !visible {
				b.SetItemBounds(
					"Wall",
					fmt.Sprintf("Wall MC %d", idx+1),
					float64(-f.projWidth),
					float64(-f.projHeight),
					float64(instWidth),
					float64(instHeight),
				)
			}
		}
		return nil
	})
}
