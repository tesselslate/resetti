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

const RESIZE_WIDTH = 1600
const RESIZE_HEIGHT = 300

type WallManager struct {
	stop   chan struct{}
	active sync.Mutex

	workers      []*Worker
	locks        []bool
	workerErrors chan WorkerError
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
	m.Errors = errch
	if err := m.createWorkers(instances); err != nil {
		return err
	}
	m.locks = make([]bool, len(m.workers))
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

func (m *WallManager) Restart(instances []mc.Instance) error {
	return m.createWorkers(instances)
}

func (m *WallManager) SetConfig(conf cfg.Config) {
	m.conf = conf
}

func (m *WallManager) createWorkers(instances []mc.Instance) error {
	m.stopWorkers()
	m.workers = make([]*Worker, 0)
	for _, i := range instances {
		w := &Worker{}
		w.SetConfig(m.conf)
		w.SetInstance(i)
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
			err := v.Resize(RESIZE_WIDTH, RESIZE_HEIGHT)
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
		logger.Log("Resetting all instances. Waiting...")
		wg := sync.WaitGroup{}
		for _, v := range m.workers {
			wg.Add(1)
			go func(v *Worker) {
				defer wg.Done()
				if m.locks[v.instance.Id] {
					return
				}
				logger.Log("Reset instance %d.", v.instance.Id)
				err := v.Reset(evt.Timestamp)
				if err != nil {
					logger.LogError("Failed to reset instance %d: %s", m.current, err)
				}
			}(v)
		}
		wg.Wait()
		logger.Log("Reset all instances.")
	} else {
		go obs.SetScene("Wall")
		logger.Log("Resetting instance %d; going to wall.", m.current)
		err := m.workers[m.current].Reset(evt.Timestamp)
		if err != nil {
			logger.LogError("Failed to reset instance %d: %s", m.current, err)
		}
		if m.conf.Wall.StretchWindows {
			err := m.workers[m.current].Resize(RESIZE_WIDTH, RESIZE_HEIGHT)
			if err != nil {
				logger.LogError("Failed to resize instance: %s", err)
				return
			}
		}
		time.Sleep(time.Duration(m.conf.Reset.Delay) * time.Millisecond)
		m.grabWallKeys()
		m.onWall = true
		err = x11.FocusWindow(m.projector)
		if err != nil {
			logger.LogError("Failed to focus projector: %s", err)
		}
		logger.Log("Reset instance successfully.")
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
}

func (m *WallManager) wallResetOthers(id int, t xproto.Timestamp) {
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
			logger.Log("Reset instance %d.", i)
			err := m.workers[i].Reset(t)
			if err != nil {
				logger.LogError("Failed to reset instance %d: %s", id, err)
			}
		}
	}
}

func (m *WallManager) wallPlay(id int, t xproto.Timestamp) {
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
	logger.Log("Toggled lock state of instance %d (%t)", id, m.locks[id])
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
		if strings.Contains(title, "Projector (Scene)") {
			m.projector = win
			return nil
		}
	}
	return nil
}
