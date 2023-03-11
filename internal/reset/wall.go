package reset

// TODO add more logging

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/jezek/xgb/xproto"
	"github.com/pkg/errors"
	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/obs"
	"github.com/woofdoggo/resetti/internal/x11"
)

// Affinity states
const (
	// affIdle is used when the instance is finished generating.
	affIdle int = iota

	// affLow is used when the instance is past the user's `low_threshold`.
	affLow

	// affMid is used when the instance would be high priority but the user is
	// playing an instance.
	affMid

	// affHigh is used when the instance has not yet reached the `low_threshold`.
	affHigh

	// affActive is used for the currently active instance.
	affActive
)

// Wall provides an implementation of the famous "wall" resetting style, where
// all of the user's instances are shown on a grid on an OBS projector and they
// can choose which to play.
type Wall struct {
	conf    cfg.Profile
	obs     *obs.Client
	x       *x11.Client
	counter Counter

	logReaders []mc.LogReader
	instances  []mc.Instance
	states     []wallState
	current    int
	pause      chan int

	movingWall MovingWall

	wallGrab          bool
	lastMouseId       int
	projector         xproto.Window
	projectorChildren []xproto.Window
}

// wallState contains the state of an instance, as well as some auxiliary
// wall-specific details about it.
type wallState struct {
	mc.InstanceState
	WpPause  bool
	Locked   bool
	Affinity int
}

// NewWall creates a new Wall for wall resetting.
func NewWall(conf cfg.Profile, infos []mc.InstanceInfo, x *x11.Client) Wall {
	// Create a different instance of `wall` for the moving wall setting.
	wall := Wall{
		conf:        conf,
		x:           x,
		logReaders:  make([]mc.LogReader, 0, len(infos)),
		current:     -1,
		pause:       make(chan int, len(infos)*2),
		lastMouseId: -1,
	}
	wall.instances = make([]mc.Instance, 0, len(infos))
	for _, info := range infos {
		wall.instances = append(wall.instances, mc.NewInstance(info, &conf, x))
	}
	wall.states = make([]wallState, len(infos))
	return wall
}

// Run attempts to run the wall resetter. If an error occurs during
// setup, it will be returned.
func (m *Wall) Run() error {
	// If necessary, run the cgroup script and set up the cpusets for each
	// cgroup.
	if m.conf.AdvancedWall.Affinity {
		if err := runCgroupScript(&m.conf); err != nil {
			return errors.Wrap(err, "run cgroup script")
		}
		if err := writeCgroups(&m.conf); err != nil {
			return errors.Wrap(err, "write cgroups")
		}
	}

	// Ensure that the user's window manager supports the necessary EWMH
	// properties.
	wm_supported, err := m.x.GetWmSupported()
	if err != nil {
		return errors.Wrap(err, "wm supported")
	}
	found := false
	for _, v := range wm_supported {
		if v == "_NET_ACTIVE_WINDOW" {
			found = true
			break
		}
	}
	if !found {
		log.Println("WARNING: Window manager does not support _NET_ACTIVE_WINDOW: wall key grabs might not work")
	}

	// Setup synchronization primitives.
	sigs := make(chan os.Signal, 16)
	signal.Notify(sigs, syscall.SIGINT)
	ctx, cancel := context.WithCancel(context.Background())
	wg := sync.WaitGroup{}
	defer func() {
		cancel()
		log.Println("Waiting for services to stop...")
		wg.Wait()
		log.Println("Stopping!")
	}()

	// Start log readers and click instances to fix the Atum bug.
	for i, v := range m.instances {
		reader, state, err := mc.NewLogReader(ctx, v.InstanceInfo)
		if err != nil {
			return errors.Wrap(err, "failed to setup log reader")
		}
		m.logReaders = append(m.logReaders, reader)
		m.states[i] = wallState{InstanceState: state}
		if err = v.Click(); err != nil {
			return errors.Wrap(err, "failed to click")
		}
	}
	updates, readerErrors := Mux(m.logReaders)

	// Setup OBS.
	m.obs = &obs.Client{}
	obsError, err := m.obs.Connect(ctx, fmt.Sprintf("localhost:%d", m.conf.Obs.Port), m.conf.Obs.Password)
	if err != nil {
		return err
	}
	if !m.conf.MovingWall.UseMovingWall {
		err = m.obs.SetSceneCollection(fmt.Sprintf("resetti - %d multi", len(m.instances)))
		if err != nil {
			return errors.Wrap(err, "failed to set scene collection")
		}
	} else {
		err = m.obs.SetSceneCollection(fmt.Sprintf("resetti - %d moving_multi", len(m.instances)))
		if err != nil {
			return errors.Wrap(err, "failed to set scene collection")
		}
	}
	wallWidth, wallHeight, err := getWallSize(m.obs, len(m.instances))
	if err != nil {
		return errors.Wrap(err, "failed to get wall size")
	}
	if err = setSources(m.obs, m.instances, m.conf.MovingWall.UseMovingWall); err != nil {
		return errors.Wrap(err, "failed to set sources")
	}

	// Disable all lock icons (some may be on from the last time resetti
	// was used.)
	if !m.conf.MovingWall.UseMovingWall {
		for i := 1; i <= len(m.instances); i += 1 {
			err = m.obs.SetSceneItemVisible("Wall", fmt.Sprintf("Lock %d", i), false)
			if err != nil {
				return errors.Wrap(err, "failed to make lock invisible")
			}
		}
	}
	if err = m.obs.SetScene("Wall"); err != nil {
		return errors.Wrap(err, "failed to set scene")
	}

	// Disable capture cursor on all instances.
	if err = m.hideCursors(); err != nil {
		return errors.Wrap(err, "failed to hide cursors")
	}

	// Setup moving wall with defaults.
	if m.conf.MovingWall.UseMovingWall {
		m.movingWall = DefaultMovingWall(m.obs, m.conf)
		err = m.movingWall.SetupWallScene(m.conf, m.instances)
		if err != nil {
			return errors.Wrap(err, "failed to setup wall scene for moving wall.")
		}
	}

	// Setup reset counter.
	counter, err := NewCounter(ctx, &wg, m.conf)
	if err != nil {
		return err
	}
	m.counter = counter

	// Start polling for X events.
	xEvt, xErr, err := m.x.Poll(ctx, true)
	if err != nil {
		return errors.Wrap(err, "failed to start polling for X events")
	}

	// Start the main loop.
	if err = m.FocusProjector(); err != nil {
		return errors.Wrap(err, "failed to switch to projector")
	}
	if err = m.x.GrabKey(m.conf.Keys.Focus, m.x.RootWindow()); err != nil {
		return errors.Wrap(err, "failed to grab focus key")
	}
	if err = m.x.GrabKey(m.conf.Keys.Reset, m.x.RootWindow()); err != nil {
		return errors.Wrap(err, "failed to grab reset key")
	}
	if err = m.GrabWallKeys(); err != nil {
		return errors.Wrap(err, "failed to grab wall keys")
	}
	printDebugInfo(m.x, m.instances)
	for {
		select {
		case <-sigs:
			log.Println("Received SIGINT.")
			return nil
		case err := <-obsError:
			log.Printf("Critical OBS error: %s\n", err)
			return nil
		case err := <-xErr:
			if err == x11.ErrDied {
				log.Println("X connection closed")
				return nil
			}
			log.Printf("Unhandled X error: %s\n", err)
		case err := <-readerErrors:
			log.Printf("Fatal reader error: %s\n", err)
			return nil
		case id := <-m.pause:
			if m.states[id].State == mc.StIdle || m.states[id].State == mc.StPreview {
				m.instances[id].PressF3Esc(0)
			}
		case update := <-updates:
			state := update.State
			id := update.Id

			// Pause the instance if it is now idle and not focused.
			nowIdle := m.states[id].State != mc.StIdle && state.State == mc.StIdle
			nowPreview := m.states[id].State != mc.StPreview && state.State == mc.StPreview
			if (nowIdle || nowPreview) && m.current != id {
				go func(i int) {
					<-time.After(time.Millisecond * time.Duration(m.conf.Reset.PauseDelay))
					m.pause <- i
				}(id)
			}
			m.states[id].InstanceState = state

			// Update the instance's affinity.
			if m.conf.AdvancedWall.Affinity {
				m.UpdateAffinity(id)
			}

			// Update the moving wall state.
			if !m.conf.MovingWall.UseMovingWall {
				continue
			}
			if nowPreview {
				err := m.movingWall.loadingView.renderInstance(m.instances[id])
				if err != nil {
					return err
				}
			}
		case evt := <-xEvt:
			switch evt := evt.(type) {
			case x11.KeyEvent:
				if evt.State == x11.KeyUp {
					continue
				}
				switch evt.Key {
				case m.conf.Keys.Reset:
					m.HandleResetInput(evt.Time)
				case m.conf.MovingWall.ResetFirstLoaded:
					if m.conf.MovingWall.UseMovingWall {
						if err = m.HandleResetLoading(evt.Time); err != nil {
							log.Panicf("Error Resetting the loading view: %s\n", err)
						}
					}
				case m.conf.MovingWall.LockFirstLoaded:
					if m.conf.MovingWall.UseMovingWall {
						if err = m.HandleLockLoaded(evt.Time); err != nil {
							log.Panicf("Error locking from loading view: %s\n", err)
						}
					}
				case m.conf.MovingWall.UnlockFirstLocked:
					if m.conf.MovingWall.UseMovingWall {
						if err = m.HandleUnlockLocked(evt.Time); err != nil {
							log.Panicf("Error unlocking from loading view: %s\n", err)
						}
					}
				case m.conf.MovingWall.PlayFirstLocked:
					if m.conf.MovingWall.UseMovingWall {
						if err = m.HandlePlayLocked(evt.Time); err != nil {
							log.Panicf("Error playing from the locked view: %s\n", err)
						}
					}
				case m.conf.MovingWall.PlayFirstLoaded:
					if m.conf.MovingWall.UseMovingWall {
						if err = m.HandlePlayLoaded(evt.Time); err != nil {
							log.Panicf("Error playing from the loading view: %s\n", err)
						}
					}
				case m.conf.Keys.Focus:
					if m.current == -1 {
						if err = m.FocusProjector(); err != nil {
							log.Printf("Failed to focus projector: %s\n", err)
						}
					} else {
						m.instances[m.current].Focus()
					}
				default:
					if m.current != -1 || m.conf.MovingWall.UseMovingWall {
						continue
					}
					id := int(evt.Key.Code - 10)
					if id < 0 || id > 8 || id > len(m.instances) {
						continue
					}
					if err := m.HandleInput(id, evt.Key.Mod, evt.Time); err != nil {
						log.Printf("Failed to handle input: %s\n", err)
					}
				}
			case x11.MoveEvent:
				// Ignored any buffered keypresses/mouse clicks if we are not
				// on the wall anymore.
				if m.current != -1 {
					continue
				}
				// Ignore presses not on the wall projector.
				found := false
				for _, child := range m.projectorChildren {
					if child == evt.Win {
						found = true
						break
					}
				}
				if !found {
					continue
				}
				if evt.State&xproto.ButtonMask1 == 0 {
					continue
				}
				w, h, err := m.x.GetWindowSize(m.projector)
				if err != nil {
					log.Printf("Failed to get projector size: %s\n", err)
					continue
				}
				x := uint16(evt.X) / (w / wallWidth)
				y := uint16(evt.Y) / (h / wallHeight)
				id := int((y * wallWidth) + x)
				if id >= len(m.instances) || m.lastMouseId == id {
					continue
				}
				m.lastMouseId = id
				if err := m.HandleInput(id, x11.Keymod(evt.State^xproto.ButtonMask1), evt.Time); err != nil {
					log.Printf("Failed to handle input: %s\n", err)
				}
			case x11.ButtonEvent:
				// Ignored any buffered keypresses/mouse clicks if we are not
				// on the wall anymore.
				if m.current != -1 {
					continue
				}

				// If the user clicked off of the projector, release the grab.
				found := false
				for _, child := range m.projectorChildren {
					if child == evt.Win {
						found = true
						break
					}
				}
				if !found {
					if err = m.UngrabWallKeys(); err != nil {
						log.Printf("Failed to ungrab wall keys: %s\n", err)
					}
					continue
				}

				// Handle the mouse click as normal.
				w, h, err := m.x.GetWindowSize(m.projector)
				if err != nil {
					log.Printf("Failed to get projector size: %s\n", err)
					continue
				}
				x := uint16(evt.X) / (w / wallWidth)
				y := uint16(evt.Y) / (h / wallHeight)
				id := int((y * wallWidth) + x)
				if id >= len(m.instances) {
					continue
				}
				m.lastMouseId = id
				if err := m.HandleInput(id, x11.Keymod(evt.State), evt.Time); err != nil {
					log.Printf("Failed to handle input: %s\n", err)
				}
			case x11.FocusEvent:
				win, err := findProjector(m.x)
				if err != nil {
					log.Printf("Failed to find projector (focus event): %s\n", err)
					continue
				}
				m.projector = win
				if err != nil {
					log.Printf("Failed to find projector children: %s\n", err)
					continue
				}
				if evt.Win == m.projector && !m.wallGrab && m.current == -1 {
					if err = m.GrabWallKeys(); err != nil {
						log.Printf("Failed to grab wall keys (focus event): %s\n", err)
					}
				} else if evt.Win != win && m.wallGrab {
					if err = m.UngrabWallKeys(); err != nil {
						log.Printf("Failed to ungrab wall keys (focus event): %s\n", err)
					}
				}
			}
		}
	}
}

// CreateSleepbgLock creates the sleepbg.lock file if the user has enabled it
// in their configuration profile.
func (m *Wall) CreateSleepbgLock() {
	if !m.conf.Wall.SleepBgLock {
		return
	}
	file, err := os.Create(m.conf.Wall.SleepBgLockPath)
	if err != nil {
		log.Printf("Failed to create sleepbg.lock: %s\n", err)
	}
	_ = file.Close()
}

// DeleteSleepbgLock deletes the sleepbg.lock file if the user has disabled it
// in their configuration profile.
func (m *Wall) DeleteSleepbgLock() {
	if !m.conf.Wall.SleepBgLock {
		return
	}
	if err := os.Remove(m.conf.Wall.SleepBgLockPath); err != nil {
		log.Printf("Failed to delete sleepbg.lock: %s\n", err)
	}
}

// FocusProjector focuses the OBS projector.
func (m *Wall) FocusProjector() error {
	projector, err := findProjector(m.x)
	if err != nil {
		return err
	}
	if err = m.x.FocusWindow(projector); err != nil {
		return errors.Wrap(err, "focus")
	}
	m.projector = projector
	children, err := m.x.GetChildWindows(projector)
	if err != nil {
		return errors.Wrap(err, "children")
	}
	m.projectorChildren = children
	return nil
}

// GotoWall returns to the wall scene.
func (m *Wall) GotoWall() {
	m.current = -1
	if err := m.obs.SetScene("Wall"); err != nil {
		log.Printf("Failed to go to wall scene: %s\n", err)
	}
	if err := m.hideCursors(); err != nil {
		log.Printf("Failed to hide cursors: %s\n", err)
	}
	err := m.FocusProjector()
	if err != nil {
		log.Printf("Failed to focus projector: %s\n", err)
	}
	if err != nil {
		log.Printf("Failed to get projector children: %s\n", err)
	}
	m.DeleteSleepbgLock()
	if m.conf.AdvancedWall.Affinity {
		m.SetAffinities()
	}
}

// GrabWallKeys attempts to grab wall-only keys from the X server.
func (m *Wall) GrabWallKeys() error {
	log.Println("Grabbing wall keys")
	win := m.x.RootWindow()
	for i := 0; i < len(m.instances); i++ {
		wallMods := []x11.Keymod{
			m.conf.Keys.WallPlay,
			m.conf.Keys.WallReset,
			m.conf.Keys.WallResetOthers,
			m.conf.Keys.WallLock,
		}
		key := x11.Key{
			Code: xproto.Keycode(i + 10),
		}
		for _, v := range wallMods {
			key.Mod = v
			if err := m.x.GrabKey(key, win); err != nil {
				return err
			}
		}

		if m.conf.MovingWall.UseMovingWall {
			movingWallKeys := []x11.Key{
				m.conf.MovingWall.LockFirstLoaded,
				m.conf.MovingWall.UnlockFirstLocked,
				m.conf.MovingWall.PlayFirstLocked,
				m.conf.MovingWall.PlayFirstLoaded,
				m.conf.MovingWall.ResetFirstLoaded,
			}
			for _, key := range movingWallKeys {
				if err := m.x.GrabKey(key, win); err != nil {
					return err
				}
			}
		}
	}
	m.wallGrab = true
	if m.conf.Wall.UseMouse {
		wait := 1
		for tries := 0; tries < 5; tries += 1 {
			err := m.x.GrabPointer(m.projector)
			if err == nil {
				return nil
			}
			log.Printf("GrabWallKeys: pointer grab failed (try %d): %s\n", tries, err)
			time.Sleep(time.Millisecond * time.Duration(wait))
			wait *= 4
		}
		log.Println("Pointer grab failed (5 tries)")
		return errors.New("failed to grab pointer")
	}
	return nil
}

// HandleInput handles an instance-specific input on the wall.
func (m *Wall) HandleInput(id int, mod x11.Keymod, timestamp xproto.Timestamp) error {
	switch mod {
	case m.conf.Keys.WallPlay:
		err := m.WallPlay(id, timestamp)
		if err != nil {
			return err
		}
	case m.conf.Keys.WallReset:
		err := m.WallReset(id, timestamp)
		if err != nil {
			return err
		}
	case m.conf.Keys.WallResetOthers:
		m.WallResetOthers(id, timestamp)
	case m.conf.Keys.WallLock:
		err := m.WallLock(id, timestamp)
		if err != nil {
			return err
		}
	}
	return nil
}

// Handles resetting the first instance of the loading view.
func (m *Wall) HandleResetLoading(timestamp xproto.Timestamp) error {
	if m.movingWall.loadingView.renderedInstances > 0 && m.current == -1 {
		err := m.reset(m.movingWall.loadingView.instances[0].Id, timestamp)
		if err != nil {
			return err
		}
		go runHook(m.conf.Hooks.Reset)
	}
	return nil
}

// Handles the locking of the first loaded instance.
func (m *Wall) HandleLockLoaded(timestamp xproto.Timestamp) error {
	if m.movingWall.loadingView.renderedInstances > 0 && m.current == -1 {
		err := m.WallLock(m.movingWall.loadingView.instances[0].Id, timestamp)
		if err != nil {
			return err
		}
	}
	return nil
}

// Handles the unlocking of the first locked instance.
func (m *Wall) HandleUnlockLocked(timestamp xproto.Timestamp) error {
	if m.movingWall.lockedView.renderedInstances > 0 && m.current == -1 {
		err := m.WallLock(m.movingWall.lockedView.instances[0].Id, timestamp)
		if err != nil {
			return err
		}
	}
	return nil
}

// Handles playing the first locked instance.
func (m *Wall) HandlePlayLocked(timestamp xproto.Timestamp) error {
	if m.movingWall.lockedView.renderedInstances > 0 && m.current == -1 {
		err := m.WallPlay(m.movingWall.lockedView.instances[0].Id, timestamp)
		if err != nil {
			return err
		}
	}
	return nil
}

// Handles playing the first loaded instance.
func (m *Wall) HandlePlayLoaded(timestamp xproto.Timestamp) error {
	if m.movingWall.loadingView.renderedInstances > 0 && m.current == -1 {
		err := m.WallPlay(m.movingWall.loadingView.instances[0].Id, timestamp)
		if err != nil {
			return err
		}
	}
	return nil
}

// HandleResetInput handles the user pressing the Reset keybind.
func (m *Wall) HandleResetInput(timestamp xproto.Timestamp) {
	if m.current == -1 {
		if !m.conf.MovingWall.UseMovingWall {
			log.Println("Resetting all")
			for idx := range m.instances {
				if err := m.WallReset(idx, timestamp); err != nil {
					log.Printf("Failed to reset %d: %s\n", idx, err)
				}
			}
		}
		return
	}
	log.Printf("Resetting %d from ingame\n", m.current)

	// Press F3 before resetting to fix ghost pie.
	m.instances[m.current].PressF3(timestamp)
	time.Sleep(time.Duration(m.conf.Reset.Delay) * time.Millisecond)
	if err := m.reset(m.current, timestamp); err != nil {
		log.Printf("Failed to reset %d: %s\n", m.current, err)
	}
	if m.conf.Wall.StretchWindows {
		if err := m.instances[m.current].Stretch(m.conf); err != nil {
			log.Printf("Failed to stretch instance: %s\n", err)
		}
	}
	go runHook(m.conf.Hooks.Reset)
	time.Sleep(time.Duration(m.conf.Reset.Delay) * time.Millisecond)
	if m.conf.Wall.GoToLocked {
		for idx, state := range m.states {
			if state.Locked && state.State == mc.StIdle {
				if err := m.WallPlay(idx, timestamp); err != nil {
					log.Printf("Failed to play instance %d: %s\n", idx, err)
				}
			}
		}
	}
	m.GotoWall()
}

// hideCursors hides the cursor from all instance sources.
func (m *Wall) hideCursors() error {
	return m.obs.Batch(obs.SerialRealtime, func(b *obs.Batch) error {
		for i := 1; i <= len(m.instances); i += 1 {
			b.SetSourceSettings(fmt.Sprintf("MC %d", i), obs.StringMap{"show_cursor": false}, true)
			if m.conf.MovingWall.UseMovingWall {
				b.SetSourceSettings(fmt.Sprintf("MC %d LockedView", i), obs.StringMap{"show_cursor": false}, true)
				b.SetSourceSettings(fmt.Sprintf("MC %d LoadingView", i), obs.StringMap{"show_cursor": false}, true)
				b.SetSourceSettings(fmt.Sprintf("MC %d FullView", i), obs.StringMap{"show_cursor": false}, true)
			}
		}
		return nil
	})
}

// reset performs all of the common tasks for resetting an instance (those
// which would be done both for resetting from ingame and on the wall.)
func (m *Wall) reset(id int, timestamp xproto.Timestamp) error {
	m.instances[id].Reset(timestamp)
	m.counter.Increment()
	m.states[id].WpPause = false
	if m.conf.AdvancedWall.Affinity {
		m.SetAffinity(id, affHigh)
	}
	if m.conf.MovingWall.UseMovingWall {
		err := m.movingWall.loadingView.unrenderInstance(m.instances[id])
		if err != nil {
			return err
		}
	}
	return nil
}

// SetAffinities sets the CPU affinity of each instance to the correct value.
// This should be called whenever the user switches to or from the wall scene
// to switch instances between mid/high affinities.
func (m *Wall) SetAffinities() {
	for i, v := range m.states {
		m.SetAffinity(i, v.Affinity)
	}
}

// SetAffinity sets the CPU affinity of a specific instance.
func (m *Wall) SetAffinity(id int, affinity int) {
	m.states[id].Affinity = affinity
	if affinity == affHigh && m.current != -1 {
		affinity = affMid
	}
	cgroup := []string{
		"idle",
		"low",
		"mid",
		"high",
		"active",
	}[affinity]
	if m.conf.AdvancedWall.CcxSplit {
		if id < len(m.instances)/2 {
			cgroup += "0"
		} else {
			cgroup += "1"
		}
	}
	path := fmt.Sprintf("/sys/fs/cgroup/resetti/%s/cgroup.procs", cgroup)
	fh, err := os.OpenFile(path, os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("SetAffinity OpenFile failed: %s\n", err)
		return
	}
	defer func() {
		if err := fh.Close(); err != nil {
			log.Printf("SetAffinity CloseFile failed: %s\n", err)
		}
	}()
	_, err = fh.WriteString(strconv.Itoa(int(m.instances[id].Pid)))
	if err != nil {
		log.Printf("SetAffinity WriteString failed: %s\n", err)
	}
}

// SetLocked sets the lock state of the given instance.
func (m *Wall) SetLocked(id int, locked bool) {
	// Do nothing if the state is unchanged.
	if m.states[id].Locked == locked {
		return
	}
	if m.states[id].State != mc.StDirt {
		m.states[id].Locked = locked
		if !m.conf.MovingWall.UseMovingWall {
			err := m.obs.SetSceneItemVisible("Wall", fmt.Sprintf("Lock %d", id+1), locked)
			if err != nil {
				log.Printf("SetLocked error: %s\n", err)
			}
		}
	}
}

// UngrabWallKeys releases wall-only key grabs.
func (m *Wall) UngrabWallKeys() error {
	log.Println("Ungrabbing wall keys")
	win := m.x.RootWindow()
	for i := 0; i < len(m.instances); i++ {
		wallMods := []x11.Keymod{
			m.conf.Keys.WallPlay,
			m.conf.Keys.WallReset,
			m.conf.Keys.WallResetOthers,
			m.conf.Keys.WallLock,
		}
		key := x11.Key{
			Code: xproto.Keycode(i + 10),
		}
		for _, v := range wallMods {
			key.Mod = v
			if err := m.x.UngrabKey(key, win); err != nil {
				return err
			}
		}
		if m.conf.MovingWall.UseMovingWall {
			movingWallKeys := []x11.Key{
				m.conf.MovingWall.LockFirstLoaded,
				m.conf.MovingWall.UnlockFirstLocked,
				m.conf.MovingWall.PlayFirstLocked,
				m.conf.MovingWall.PlayFirstLoaded,
				m.conf.MovingWall.ResetFirstLoaded,
			}
			for _, key := range movingWallKeys {
				if err := m.x.UngrabKey(key, win); err != nil {
					return err
				}
			}
		}
	}
	m.wallGrab = false
	if m.conf.Wall.UseMouse {
		return m.x.UngrabPointer()
	}
	return nil
}

// UpdateAffinity updates the affinity of an instance based on its state.
func (m *Wall) UpdateAffinity(id int) {
	wallState := m.states[id]
	state := wallState.InstanceState

	switch state.State {
	case mc.StDirt:
		m.SetAffinity(id, affHigh)
	case mc.StPreview:
		if state.Progress >= m.conf.AdvancedWall.LowThreshold && !wallState.Locked {
			m.SetAffinity(id, affLow)
		} else {
			m.SetAffinity(id, affHigh)
		}
	case mc.StIdle:
		m.SetAffinity(id, affIdle)
	case mc.StIngame:
		m.SetAffinity(id, affActive)
	}
}

// WallPlay plays the given instance.
func (m *Wall) WallPlay(id int, timestamp xproto.Timestamp) error {
	if m.states[id].State != mc.StIdle {
		return nil
	}
	if m.conf.AdvancedWall.Affinity {
		m.SetAffinity(id, affActive)
	}
	m.states[id].State = mc.StIngame
	if err := m.obs.SetScene(fmt.Sprintf("Instance %d", id+1)); err != nil {
		log.Printf("Failed to set scene: %s\n", err)
	}
	if err := m.UngrabWallKeys(); err != nil {
		log.Printf("Failed to ungrab wall keys: %s\n", err)
	}

	// Focus and unpause.
	m.instances[id].FocusAndUnpause(timestamp, true)
	if m.conf.Wall.StretchWindows {
		if err := m.instances[id].Unstretch(m.conf); err != nil {
			log.Printf("Failed to unstretch instance: %s\n", err)
		}

		// If using stretch windows, pause and unpause to fix the cursor
		// position the next time the pause menu or inventory is opened.
		time.Sleep(time.Millisecond * time.Duration(m.conf.Reset.Delay))
		m.instances[id].PressEsc(0)
		m.instances[id].PressEsc(0)
	}
	m.SetLocked(id, false)
	if m.conf.MovingWall.UseMovingWall {
		err := m.movingWall.loadingView.unrenderInstance(m.instances[id])
		if err != nil {
			return err
		}
		err = m.movingWall.lockedView.unrenderInstance(m.instances[id])
		if err != nil {
			return err
		}
	}
	m.current = id
	m.CreateSleepbgLock()
	if m.conf.AdvancedWall.Affinity {
		m.SetAffinities()
	}
	err := m.obs.SetSourceSettings(fmt.Sprintf("MC %d", id+1), obs.StringMap{"show_cursor": true}, true)
	if err != nil {
		return errors.Wrap(err, "show cursor")
	}
	return nil
}

// WallReset resets the given instance.
func (m *Wall) WallReset(id int, timestamp xproto.Timestamp) error {
	// Don't reset if the instance is locked or on the dirt screen.
	state := m.states[id]
	if state.Locked || state.State == mc.StDirt {
		return nil
	}
	err := m.reset(id, timestamp)
	if err != nil {
		return err
	}
	go runHook(m.conf.Hooks.WallReset)
	return nil
}

// WallResetOthers plays the given instance and resets all other unlocked
// instances.
func (m *Wall) WallResetOthers(id int, timestamp xproto.Timestamp) {
	if m.states[id].State != mc.StIdle {
		return
	}
	if err := m.WallPlay(id, timestamp); err != nil {
		log.Printf("Failed to play instance %d: %s\n", id, err)
	}
	for i := 0; i < len(m.instances); i++ {
		if i != id {
			if err := m.WallReset(i, timestamp); err != nil {
				log.Printf("Failed to reset instance %d: %s\n", i, err)
			}
		}
	}
}

// WallLock locks the given instance.
func (m *Wall) WallLock(id int, timestamp xproto.Timestamp) error {
	m.SetLocked(id, !m.states[id].Locked)
	if m.states[id].Locked {
		go runHook(m.conf.Hooks.Lock)
		if m.states[id].State == mc.StPreview {
			if m.conf.AdvancedWall.Affinity {
				m.SetAffinity(id, affHigh)
			}
		}
	} else {
		go runHook(m.conf.Hooks.Unlock)
	}
	if !m.conf.MovingWall.UseMovingWall {
		return nil
	}
	if m.states[id].Locked {
		err := m.movingWall.lockedView.renderInstance(m.instances[id])
		if err != nil {
			return err
		}
		err = m.movingWall.loadingView.unrenderInstance(m.instances[id])
		if err != nil {
			return err
		}
	} else {
		err := m.movingWall.lockedView.unrenderInstance(m.instances[id])
		if err != nil {
			return err
		}
		err = m.movingWall.loadingView.renderInstance(m.instances[id])
		if err != nil {
			return err
		}
	}
	return nil
}
