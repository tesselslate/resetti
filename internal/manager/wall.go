package manager

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/logger"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/obs"
	"github.com/woofdoggo/resetti/internal/x11"

	"github.com/jezek/xgb/xproto"
)

type WallManager struct {
	stop   chan struct{}
	active sync.Mutex

	workers      []*Worker
	locks        []bool
	ready        []bool
	workerErrors chan WorkerError
	updates      chan mc.Instance
	current      int
	onWall       bool
	wallGrab     bool
	projector    xproto.Window
	screenWidth  uint16
	screenHeight uint16
	wallWidth    uint16
	wallHeight   uint16
	lastMouseId  int

	Errors chan error
	conf   cfg.Config
}

func (m *WallManager) Start(instances []mc.Instance, errch chan error) error {
	m.conf = cfg.GetConfig()
	if len(instances) == 0 {
		return errors.New("no instances")
	}
	if !m.active.TryLock() {
		return errors.New("already running")
	}
	if m.conf.Obs.Enabled {
		err := obs.SetupScenes(instances)
		if err != nil {
			return err
		}
	}
	m.stop = make(chan struct{})
	m.workerErrors = make(chan WorkerError, len(instances))
	m.updates = make(chan mc.Instance, len(instances)*4)
	m.Errors = errch
	if err := m.createWorkers(instances); err != nil {
		return err
	}
	m.locks = make([]bool, len(m.workers))
	for i := 0; i < len(m.locks); i++ {
		_ = obs.SetVisible("Wall", fmt.Sprintf("Lock %d", i+1), false)
	}
	m.ready = make([]bool, len(m.workers))
	err := setAffinity(instances, m.conf.General.Affinity)
	if err != nil {
		return err
	}
	m.lastMouseId = -1
	go m.run()
	return nil
}

func (m *WallManager) Stop() {
	logger.Log("Manager: sent stop signal...")
	m.stop <- struct{}{}
	<-m.stop
	logger.Log("Manager stop signal returned!")
}

func (m *WallManager) Wait() {
	// Suppress "empty critical section" warning with defer.
	defer m.active.Unlock()
	m.active.Lock()
}

func (m *WallManager) createWorkers(instances []mc.Instance) error {
	m.stopWorkers()
	m.workers = make([]*Worker, 0)
	for _, i := range instances {
		w := &Worker{}
		w.SetInstance(i)
		w.Subscribe(m.updates)
		err := w.Start(m.workerErrors)
		if err != nil {
			m.stopWorkers()
			return err
		}
		m.workers = append(m.workers, w)
	}
	return nil
}

func (m *WallManager) stopWorkers() {
	wg := sync.WaitGroup{}
	for i := 0; i < len(m.workers); i++ {
		wg.Add(1)
		go func(i int) {
			if m.workers[i] != nil {
				m.workers[i].Stop()
			}
			wg.Done()
		}(i)
	}
	wg.Wait()
}

func (m *WallManager) grabKeys() {
	x11.GrabKey(m.conf.Keys.Focus)
	x11.GrabKey(m.conf.Keys.Reset)
}

func (m *WallManager) ungrabKeys() {
	x11.UngrabKey(m.conf.Keys.Focus)
	x11.UngrabKey(m.conf.Keys.Reset)
}

func (m *WallManager) grabWallKeys() {
	if m.wallGrab {
		return
	}
	for i := 0; i < len(m.workers); i++ {
		key := x11.Key{
			Code: xproto.Keycode(i + 10),
		}
		key.Mod = m.conf.Keys.WallPlay
		x11.GrabKey(key)
		key.Mod = m.conf.Keys.WallReset
		x11.GrabKey(key)
		key.Mod = m.conf.Keys.WallResetOthers
		x11.GrabKey(key)
		key.Mod = m.conf.Keys.WallLock
		x11.GrabKey(key)
	}
	if m.conf.Wall.UseMouse {
		if err := x11.GrabPointer(); err != nil {
			logger.LogError("Failed to grab pointer: %s", err)
		}
	}
	m.wallGrab = true
}

func (m *WallManager) ungrabWallKeys() {
	if !m.wallGrab {
		return
	}
	for i := 0; i < len(m.workers); i++ {
		key := x11.Key{
			Code: xproto.Keycode(i + 10),
		}
		key.Mod = m.conf.Keys.WallPlay
		x11.UngrabKey(key)
		key.Mod = m.conf.Keys.WallReset
		x11.UngrabKey(key)
		key.Mod = m.conf.Keys.WallResetOthers
		x11.UngrabKey(key)
		key.Mod = m.conf.Keys.WallLock
		x11.UngrabKey(key)
	}
	if m.conf.Wall.UseMouse {
		if err := x11.UngrabPointer(); err != nil {
			logger.LogError("Failed to release pointer: %s", err)
		}
	}
	m.wallGrab = false
}

func (m *WallManager) run() {
	cleanup := []func(){
		m.ungrabKeys,
		m.ungrabWallKeys,
		m.stopWorkers,
		m.active.Unlock,
	}
	defer func() {
		for _, v := range cleanup {
			v()
		}
	}()

	m.grabKeys()
	m.grabWallKeys()
	// Get screen size.
	sw, sh, err := x11.ScreenSize()
	if err != nil {
		m.Errors <- fmt.Errorf("failed to get screen size: %s", err)
		return
	}
	m.screenWidth = sw
	m.screenHeight = sh
	// Locate OBS projector.
	err = m.findProjector()
	if err != nil {
		m.Errors <- err
		return
	}
	if m.projector == 0 {
		// No projector found. Spawn one.
		err := obs.OpenProjector()
		if err != nil {
			m.Errors <- fmt.Errorf("failed to spawn projector: %s", err)
			return
		}
		time.Sleep(100 * time.Millisecond)
		err = m.findProjector()
		if err != nil {
			m.Errors <- err
			return
		}
		if m.projector == 0 {
			m.Errors <- errors.New("still unable to find OBS projector")
			return
		}
	}
	x11.FocusWindow(m.projector)
	m.onWall = true
	for i := range m.locks {
		m.setLock(i, false)
	}
	if m.conf.Wall.StretchWindows {
		for _, v := range m.workers {
			err := v.Resize(m.conf.Wall.StretchWidth, m.conf.Wall.StretchHeight)
			if err != nil {
				m.Errors <- fmt.Errorf("failed to resize instance: %s", err)
				return
			}
		}
		logger.Log("Stretched instances.")
	}
	if m.conf.Wall.UseMouse {
		ww, wh, err := obs.GetWallSize(len(m.workers))
		logger.Log("Got wall size: %dx%d, %d.", ww, wh, len(m.workers))
		if err != nil {
			m.Errors <- fmt.Errorf("failed to get wall size: %s", err)
			return
		}
		m.wallWidth = ww
		m.wallHeight = wh
	}
	xevt := make(chan any, 32)
	x11.Subscribe(nil, xevt)
	for {
		select {
		case werr := <-m.workerErrors:
			// Wait a moment and then attempt to reboot the dead worker.
			logger.LogError("Worker %d died: %s", werr.Id, werr.Err)
			time.Sleep(10 * time.Millisecond)
			err := m.workers[werr.Id].Start(m.workerErrors)
			if err != nil {
				m.Errors <- fmt.Errorf("failed to reboot worker %d: %s", werr.Id, err)
				return
			}
		case evt := <-xevt:
			switch evt := evt.(type) {
			case x11.KeyEvent:
				if evt.State == x11.KeyDown {
					switch evt.Key {
					case m.conf.Keys.Focus:
						m.focus(evt)
					case m.conf.Keys.Reset:
						m.reset(evt)
					default:
						if evt.Key.Code < 10 || evt.Key.Code > 19 {
							continue
						}
						id := int(evt.Key.Code - 10)
						if id >= len(m.workers) {
							continue
						}
						m.handleEvent(id, evt.Key.Mod, evt.Timestamp)
					}
				}
			case x11.MoveEvent:
				if evt.State&xproto.ButtonMask1 != 0 {
					iw := m.screenWidth / m.wallWidth
					ih := m.screenHeight / m.wallHeight
					x := uint16(evt.X) / iw
					y := uint16(evt.Y) / ih
					id := int((y * m.wallWidth) + x)
					if id >= len(m.workers) {
						continue
					}
					if m.lastMouseId == id {
						continue
					}
					m.lastMouseId = id
					m.handleEvent(id, x11.Keymod(evt.State)^xproto.ButtonMask1, evt.Timestamp)
				}
			case x11.ButtonEvent:
				iw := m.screenWidth / m.wallWidth
				ih := m.screenHeight / m.wallHeight
				x := uint16(evt.X) / iw
				y := uint16(evt.Y) / ih
				id := int((y * m.wallWidth) + x)
				if id >= len(m.workers) {
					continue
				}
				m.lastMouseId = id
				m.handleEvent(id, x11.Keymod(evt.State), evt.Timestamp)
			}
		case u := <-m.updates:
			switch u.State.Identifier {
			case mc.StateReady:
				m.ready[u.Id] = true
			case mc.StateGenerating, mc.StatePreview:
				m.ready[u.Id] = false
			}
		case <-m.stop:
			// Delete cleanup tasks and run them before returning.
			logger.Log("Stopping manager...")
			for i, v := range cleanup {
				v()
				logger.Log("Cleanup: Completed task %d", i)
			}
			cleanup = make([]func(), 0)
			m.stop <- struct{}{}
			logger.Log("Stopped manager!")
			return
		}
	}
}

func (m *WallManager) goToWall() {
	go obs.SetScene("Wall")
	m.grabWallKeys()
	m.onWall = true
	err := x11.FocusWindow(m.projector)
	if err != nil {
		logger.LogError("Failed to focus projector: %s", err)
	}
}

func (m *WallManager) handleEvent(id int, state x11.Keymod, time xproto.Timestamp) {
	switch state {
	case m.conf.Keys.WallPlay:
		m.wallPlay(id, time)
	case m.conf.Keys.WallReset:
		m.wallReset(id, time)
	case m.conf.Keys.WallResetOthers:
		m.wallResetOthers(id, time)
	case m.conf.Keys.WallLock:
		m.wallLock(id, time)
	}
}

func (m *WallManager) focus(evt x11.KeyEvent) {
	if m.onWall {
		x11.FocusWindow(m.projector)
	} else {
		err := m.workers[m.current].Focus(evt.Timestamp)
		if err != nil {
			logger.LogError("Failed to focus instance %d: %s", m.current, err)
		}
	}
}

func (m *WallManager) reset(evt x11.KeyEvent) {
	if m.onWall {
		logger.Log("Resetting all instances...")
		wg := sync.WaitGroup{}
		for _, v := range m.workers {
			wg.Add(1)
			go func(v *Worker) {
				if m.locks[v.instance.Id] {
					wg.Done()
					return
				}
				logger.Log("Reset instance %d.", v.instance.Id)
				err := v.Reset(evt.Timestamp)
				if err != nil {
					logger.LogError("Failed to reset instance %d: %s", m.current, err)
				}
				wg.Done()
				if m.conf.Hooks.WallReset != "" {
					runHook(m.conf.Hooks.WallReset)
				}
			}(v)
		}
		wg.Wait()
		logger.Log("Reset all instances.")
	} else {
		err := m.workers[m.current].Reset(evt.Timestamp)
		if err != nil {
			logger.LogError("Failed to reset instance %d: %s", m.current, err)
		}
		if m.conf.Wall.StretchWindows {
			err := m.workers[m.current].Resize(m.conf.Wall.StretchWidth, m.conf.Wall.StretchHeight)
			if err != nil {
				logger.LogError("Failed to resize instance: %s", err)
				return
			}
		}
		time.Sleep(time.Duration(m.conf.Reset.Delay) * time.Millisecond)
		if !m.conf.Wall.GoToLocked {
			logger.Log("Reset %d; going to wall.", m.current)
			m.goToWall()
		} else {
			for idx, locked := range m.locks {
				if locked {
					if m.conf.Wall.NoPlayGen && !m.ready[idx] {
						continue
					}
					m.wallPlay(idx, evt.Timestamp)
					logger.Log("Reset, going to %d.", idx)
					return
				}
			}
			logger.Log("Reset %d; going to wall.", m.current)
			m.goToWall()
		}
	}
}

func (m *WallManager) wallReset(id int, t xproto.Timestamp) {
	if m.locks[id] {
		return
	}
	logger.Log("Reset instance %d.", id)
	err := m.workers[id].Reset(t)
	if err != nil {
		logger.LogError("Failed to reset instance %d: %s", id, err)
	}
	if m.conf.Hooks.WallReset != "" {
		go runHook(m.conf.Hooks.WallReset)
	}
}

func (m *WallManager) wallResetOthers(id int, t xproto.Timestamp) {
	if m.conf.Wall.NoPlayGen && !m.ready[id] {
		return
	}
	go obs.SetScene(fmt.Sprintf("Instance %d", id+1))
	m.ungrabWallKeys()
	m.onWall = false
	m.current = id
	err := m.workers[id].Focus(t)
	if err != nil {
		logger.LogError("Failed to focus instance %d: %s", id, err)
		return
	}
	if m.conf.Wall.StretchWindows {
		err := m.workers[m.current].Resize(m.screenWidth, m.screenHeight)
		if err != nil {
			logger.LogError("Failed to resize instance: %s", err)
			return
		}
	}
	m.setLock(id, false)
	for i := 0; i < len(m.workers); i++ {
		if i != id && !m.locks[i] {
			logger.Log("Reset %d.", i)
			err := m.workers[i].Reset(t)
			if err != nil {
				logger.LogError("Failed to reset instance %d: %s", id, err)
			}
			if m.conf.Hooks.WallReset != "" {
				go runHook(m.conf.Hooks.WallReset)
			}
		}
	}
}

func (m *WallManager) wallPlay(id int, t xproto.Timestamp) {
	if m.conf.Wall.NoPlayGen && !m.ready[id] {
		return
	}
	go obs.SetScene(fmt.Sprintf("Instance %d", id+1))
	m.ungrabWallKeys()
	m.onWall = false
	m.current = id
	err := m.workers[id].Focus(t + 10)
	if err != nil {
		logger.LogError("Failed to focus instance %d: %s", id, err)
		return
	}
	if m.conf.Wall.StretchWindows {
		err := m.workers[m.current].Resize(m.screenWidth, m.screenHeight)
		if err != nil {
			logger.LogError("Failed to resize instance: %s", err)
			return
		}
	}
	m.setLock(id, false)
}

func (m *WallManager) wallLock(id int, t xproto.Timestamp) {
	m.setLock(id, !m.locks[id])
	logger.Log("Lock %d (%t)", id, m.locks[id])
	if m.locks[id] && m.conf.Hooks.Lock != "" {
		go runHook(m.conf.Hooks.Lock)
	} else if !m.locks[id] && m.conf.Hooks.Unlock != "" {
		go runHook(m.conf.Hooks.Unlock)
	}
}

func (m *WallManager) setLock(i int, state bool) {
	if m.locks[i] == state {
		return
	}
	m.locks[i] = state
	err := obs.SetVisible("Wall", fmt.Sprintf("Lock %d", i+1), state)
	if err != nil {
		logger.LogError("Failed to set lock (%d, %t).", i, state)
	}
}

func (m *WallManager) findProjector() error {
	windows, err := x11.GetAllWindows()
	if err != nil {
		return err
	}
	for _, win := range windows {
		title, err := x11.GetWindowTitle(win)
		if err != nil {
			continue
		}
		if strings.Contains(title, "Projector (Scene) - Wall") {
			m.projector = win
			return nil
		}
	}
	return nil
}
