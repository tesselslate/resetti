package reset

// TODO add more logging

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/jezek/xgb/xproto"
	"github.com/pkg/errors"
	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/obs"
	"github.com/woofdoggo/resetti/internal/x11"
	"golang.org/x/sys/unix"
)

// Affinity states
const (
	// affIdle is used when the instance is finished generating.
	affIdle int = iota

	// affLow is used when the instance is past the user's `low_threshold`.
	affLow

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

	logReaders []LogReader
	instances  []mc.Instance
	states     []wallState
	current    int

	wallGrab    bool
	lastMouseId int

	cpusIdle   unix.CPUSet
	cpusLow    unix.CPUSet
	cpusMid    unix.CPUSet
	cpusHigh   unix.CPUSet
	cpusActive unix.CPUSet
}

// wallState contains the state of an instance, as well as some auxiliary
// wall-specific details about it.
type wallState struct {
	mc.InstanceState
	Frozen   bool
	Locked   bool
	Affinity int
}

// NewWall creates a new Wall for wall resetting.
func NewWall(conf cfg.Profile, infos []mc.InstanceInfo, x *x11.Client) Wall {
	wall := Wall{
		conf:        conf,
		x:           x,
		logReaders:  make([]LogReader, 0, len(infos)),
		current:     -1,
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
		reader, err := NewLogReader(ctx, &wg, v.InstanceInfo)
		if err != nil {
			return errors.Wrap(err, "failed to setup log reader")
		}
		m.logReaders = append(m.logReaders, reader)
		if _, err = reader.readState(); err != nil {
			return errors.Wrap(err, "failed to read state")
		}
		m.states[i] = wallState{
			InstanceState: reader.state,
		}
		if err = v.Click(); err != nil {
			return errors.Wrap(err, "failed to click")
		}
	}
	updates, readerErrors := mux(m.logReaders)

	// Setup OBS.
	m.obs = &obs.Client{}
	obsError, err := m.obs.Connect(ctx, fmt.Sprintf("localhost:%d", m.conf.Obs.Port), m.conf.Obs.Password)
	if err != nil {
		return err
	}
	err = m.obs.SetSceneCollection(fmt.Sprintf("resetti - %d multi", len(m.instances)))
	if err != nil {
		return errors.Wrap(err, "failed to set scene collection")
	}
	screenWidth, screenHeight, err := m.x.GetScreenSize()
	if err != nil {
		return errors.Wrap(err, "failed to get screen size")
	}
	wallWidth, wallHeight, err := getWallSize(m.obs, len(m.instances))
	if err != nil {
		return errors.Wrap(err, "failed to get wall size")
	}
	instanceWidth, instanceHeight := screenWidth/wallWidth, screenHeight/wallHeight
	if err = setSources(m.obs, m.instances); err != nil {
		return errors.Wrap(err, "failed to set sources")
	}

	// Disable all lock icons (some may be on from the last time resetti
	// was used.)
	for i := 1; i <= len(m.instances); i += 1 {
		err = m.obs.SetSceneItemVisible("Wall", fmt.Sprintf("Lock %d", i), false)
		if err != nil {
			return errors.Wrap(err, "failed to make lock invisible")
		}
	}
	if err = m.obs.SetScene("Wall"); err != nil {
		return errors.Wrap(err, "failed to set scene")
	}

	// Setup reset counter.
	counter, err := NewCounter(ctx, &wg, m.conf)
	if err != nil {
		return err
	}
	m.counter = counter

	// Setup affinity CPU masks.
	if m.conf.AdvancedWall.Affinity {
		m.cpusIdle = makeCpuSet(m.conf.AdvancedWall.CpusIdle)
		m.cpusLow = makeCpuSet(m.conf.AdvancedWall.CpusLow)
		m.cpusMid = makeCpuSet(m.conf.AdvancedWall.CpusMid)
		m.cpusHigh = makeCpuSet(m.conf.AdvancedWall.CpusHigh)
		m.cpusActive = makeCpuSet(m.conf.AdvancedWall.CpusActive)
	}

	// Start polling for X events.
	xEvt, xErr, err := m.x.Poll(ctx, true)
	if err != nil {
		return errors.Wrap(err, "failed to start polling for X events")
	}

	// Start the main loop.
	if err = m.x.GrabKey(m.conf.Keys.Focus, m.x.RootWindow()); err != nil {
		return errors.Wrap(err, "failed to grab focus key")
	}
	if err = m.x.GrabKey(m.conf.Keys.Reset, m.x.RootWindow()); err != nil {
		return errors.Wrap(err, "failed to grab reset key")
	}
	if err = m.GrabWallKeys(); err != nil {
		return errors.Wrap(err, "failed to grab wall keys")
	}
	m.FocusProjector()
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
		case update := <-updates:
			state := update.State
			id := update.Id

			// Pause the instance if it is now idle and not focused.
			nowIdle := m.states[id].State != mc.StIdle && state.State == mc.StIdle
			nowPreview := m.states[id].State != mc.StPreview && state.State == mc.StPreview
			if (nowIdle || nowPreview) && m.current != id {
				time.Sleep(10 * time.Millisecond)
				m.instances[id].Pause(0)
			}
			m.states[id].InstanceState = state

			// Update the instance's affinity.
			if m.conf.AdvancedWall.Affinity {
				m.UpdateAffinity(id)
			}
		case evt := <-xEvt:
			switch evt := evt.(type) {
			case x11.KeyEvent:
				if evt.State == x11.KeyUp {
					continue
				}
				switch evt.Key {
				case m.conf.Keys.Focus:
					if m.current == -1 {
						m.FocusProjector()
					} else {
						m.instances[m.current].Focus()
					}
				case m.conf.Keys.Reset:
					m.HandleResetInput(evt.Time)
				default:
					if m.current != -1 {
						continue
					}
					id := int(evt.Key.Code - 10)
					if id < 0 || id > 8 || id > len(m.instances) {
						continue
					}
					m.HandleInput(id, evt.Key.Mod, evt.Time)
				}
			case x11.MoveEvent:
				// Ignored any buffered keypresses/mouse clicks if we are not
				// on the wall anymore.
				if m.current != -1 {
					continue
				}
				if evt.State&xproto.ButtonMask1 == 0 {
					continue
				}
				x := uint16(evt.X) / instanceWidth
				y := uint16(evt.Y) / instanceHeight
				id := int((y * wallWidth) + x)
				if id >= len(m.instances) || m.lastMouseId == id {
					continue
				}
				m.lastMouseId = id
				m.HandleInput(id, x11.Keymod(evt.State^xproto.ButtonMask1), evt.Time)
			case x11.ButtonEvent:
				// Ignored any buffered keypresses/mouse clicks if we are not
				// on the wall anymore.
				if m.current != -1 {
					continue
				}
				x := uint16(evt.X) / instanceWidth
				y := uint16(evt.Y) / instanceHeight
				id := int((y * wallWidth) + x)
				if id >= len(m.instances) {
					continue
				}
				m.lastMouseId = id
				m.HandleInput(id, x11.Keymod(evt.State), evt.Time)
			case x11.FocusEvent:
				if m.current != -1 {
					continue
				}
				win, err := findProjector(m.x)
				if err != nil {
					log.Printf("Failed to find projector (focus event): %s\n", err)
					continue
				}
				if evt.Win == win && !m.wallGrab {
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
func (m *Wall) FocusProjector() {
	projector, err := findProjector(m.x)
	if err != nil {
		log.Printf("Failed to find projector: %s\n", err)
		return
	}
	if err = m.x.FocusWindow(projector); err != nil {
		log.Printf("Failed to focus projector: %s\n", err)
	}
}

// GotoWall returns to the wall scene.
func (m *Wall) GotoWall() {
	m.current = -1
	if err := m.obs.SetScene("Wall"); err != nil {
		log.Printf("Failed to go to wall scene: %s\n", err)
	}
	m.FocusProjector()
	if err := m.GrabWallKeys(); err != nil {
		log.Printf("Failed to grab wall keys: %s\n", err)
	}
	m.DeleteSleepbgLock()
	m.SetAffinities()
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
	}
	m.wallGrab = true
	if m.conf.Wall.UseMouse {
		return m.x.GrabPointer(win)
	}
	return nil
}

// HandleInput handles an instance-specific input on the wall.
func (m *Wall) HandleInput(id int, mod x11.Keymod, timestamp xproto.Timestamp) {
	switch mod {
	case m.conf.Keys.WallPlay:
		m.WallPlay(id, timestamp)
	case m.conf.Keys.WallReset:
		m.WallReset(id, timestamp)
	case m.conf.Keys.WallResetOthers:
		m.WallResetOthers(id, timestamp)
	case m.conf.Keys.WallLock:
		m.WallLock(id, timestamp)
	}
}

// HandleResetInput handles the user pressing the Reset keybind.
func (m *Wall) HandleResetInput(timestamp xproto.Timestamp) {
	if m.current == -1 {
		log.Println("Resetting all")
		for idx := range m.instances {
			m.WallReset(idx, timestamp)
		}
		return
	}
	log.Printf("Resetting %d from ingame\n", m.current)
	m.reset(m.current, timestamp)
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
				m.WallPlay(idx, timestamp)
				return
			}
		}
	}
	m.GotoWall()
}

// reset performs all of the common tasks for resetting an instance (those
// which would be done both for resetting from ingame and on the wall.)
func (m *Wall) reset(id int, timestamp xproto.Timestamp) {
	m.instances[id].Reset(timestamp)
	m.counter.Increment()
	m.states[id].State = mc.StDirt
	if m.conf.AdvancedWall.Affinity {
		m.SetAffinity(id, affHigh)
	}
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
	var set unix.CPUSet
	switch affinity {
	case affIdle:
		set = m.cpusIdle
	case affLow:
		set = m.cpusLow
	case affHigh:
		if m.current == -1 {
			// on wall
			set = m.cpusHigh
		} else {
			// on instance
			set = m.cpusMid
		}
	case affActive:
		set = m.cpusActive
	}
	if err := m.instances[id].SetAffinity(&set); err != nil {
		log.Printf("SetAffinity failed: %s\n", err)
	}
}

// SetLocked sets the lock state of the given instance.
func (m *Wall) SetLocked(id int, locked bool) {
	// Do nothing if the state is unchanged.
	if m.states[id].Locked == locked {
		return
	}
	m.states[id].Locked = locked
	err := m.obs.SetSceneItemVisible("Wall", fmt.Sprintf("Lock %d", id+1), locked)
	if err != nil {
		log.Printf("SetLocked error: %s\n", err)
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
func (m *Wall) WallPlay(id int, timestamp xproto.Timestamp) {
	if m.states[id].State != mc.StIdle {
		return
	}
	m.SetAffinity(id, affActive)
	m.states[id].State = mc.StIngame
	if err := m.obs.SetScene(fmt.Sprintf("Instance %d", id+1)); err != nil {
		log.Printf("Failed to set scene: %s\n", err)
	}
	if err := m.UngrabWallKeys(); err != nil {
		log.Printf("Failed to ungrab wall keys: %s\n", err)
	}
	m.instances[id].FocusAndUnpause(timestamp, true)
	if m.conf.Wall.StretchWindows {
		if err := m.instances[id].Unstretch(m.conf); err != nil {
			log.Printf("Failed to unstretch instance: %s\n", err)
		}
	}
	m.SetLocked(id, false)
	m.current = id
	m.CreateSleepbgLock()
	if m.conf.AdvancedWall.Affinity {
		m.SetAffinities()
	}
}

// WallReset resets the given instance.
func (m *Wall) WallReset(id int, timestamp xproto.Timestamp) {
	// Don't reset if the instance is locked, frozen, or on the dirt screen.
	state := m.states[id]
	if state.Locked || state.Frozen || state.State == mc.StDirt {
		return
	}
	m.reset(id, timestamp)
	go runHook(m.conf.Hooks.WallReset)
}

// WallResetOthers plays the given instance and resets all other unlocked
// instances.
func (m *Wall) WallResetOthers(id int, timestamp xproto.Timestamp) {
	if m.states[id].State != mc.StIdle {
		return
	}
	m.WallPlay(id, timestamp)
	for i := 0; i < len(m.instances); i++ {
		if i != id {
			m.WallReset(i, timestamp)
		}
	}
}

// WallLock locks the given instance.
func (m *Wall) WallLock(id int, timestamp xproto.Timestamp) {
	m.SetLocked(id, !m.states[id].Locked)
	if m.states[id].Locked {
		go runHook(m.conf.Hooks.Lock)
		if m.states[id].State == mc.StPreview {
			m.SetAffinity(id, affHigh)
		}
	} else {
		go runHook(m.conf.Hooks.Unlock)
	}
}
