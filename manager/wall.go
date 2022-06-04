package manager

import (
	"errors"
	"fmt"
	"github.com/woofdoggo/resetti/cfg"
	"github.com/woofdoggo/resetti/mc"
	"github.com/woofdoggo/resetti/ui"
	"github.com/woofdoggo/resetti/x11"
	"strings"
	"sync"
	"time"

	"github.com/jezek/xgb/xproto"
	obs "github.com/woofdoggo/go-obs"
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
	screenWidth  uint32
	screenHeight uint32
	wallWidth    uint32
	wallHeight   uint32
	lastMouseId  int

	Errors chan error
	conf   cfg.Config
	x      *x11.Client
	o      *obs.Client
}

func (m *WallManager) Start(instances []mc.Instance, errch chan error) error {
	if len(instances) == 0 {
		return errors.New("no instances")
	}
	if !m.active.TryLock() {
		return errors.New("already running")
	}
	if m.o != nil {
		err := setupObs(m.o, instances)
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
	err := setAffinity(instances, m.conf.Affinity)
	if err != nil {
		return err
	}
	m.lastMouseId = -1
	go m.run()
	return nil
}

func (m *WallManager) Stop() {
	ui.Log("Manager: sent stop signal...")
	m.stop <- struct{}{}
	<-m.stop
	ui.Log("Manager stop signal returned!")
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

func (m *WallManager) SetDeps(x *x11.Client, o *obs.Client) {
	m.x = x
	m.o = o
}

func (m *WallManager) createWorkers(instances []mc.Instance) error {
	m.stopWorkers()
	m.workers = make([]*Worker, 0)
	for _, i := range instances {
		w := &Worker{}
		w.SetConfig(m.conf.Reset)
		w.SetDeps(i, m.x, m.o)
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
			m.workers[i].Stop()
			wg.Done()
		}(i)
	}
	wg.Wait()
}

func (m *WallManager) grabKeys() {
	m.x.GrabKey(m.conf.Keys.Focus)
	m.x.GrabKey(m.conf.Keys.Reset)
}

func (m *WallManager) ungrabKeys() {
	m.x.UngrabKey(m.conf.Keys.Focus)
	m.x.UngrabKey(m.conf.Keys.Reset)
}

func (m *WallManager) grabWallKeys() {
	if m.wallGrab {
		return
	}
	for i := 0; i < len(m.workers); i++ {
		key := x11.Key{
			Code: xproto.Keycode(i + 10),
		}
		key.Mod = m.conf.Wall.Play
		m.x.GrabKey(key)
		key.Mod = m.conf.Wall.Reset
		m.x.GrabKey(key)
		key.Mod = m.conf.Wall.ResetOthers
		m.x.GrabKey(key)
		key.Mod = m.conf.Wall.Lock
		m.x.GrabKey(key)
	}
	if m.conf.Wall.Mouse {
		if err := m.x.GrabPointer(); err != nil {
			ui.LogError("Failed to grab pointer: %s", err)
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
		key.Mod = m.conf.Wall.Play
		m.x.UngrabKey(key)
		key.Mod = m.conf.Wall.Reset
		m.x.UngrabKey(key)
		key.Mod = m.conf.Wall.ResetOthers
		m.x.UngrabKey(key)
		key.Mod = m.conf.Wall.Lock
		m.x.UngrabKey(key)
	}
	if m.conf.Wall.Mouse {
		if err := m.x.UngrabPointer(); err != nil {
			ui.LogError("Failed to release pointer: %s", err)
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
	// Locate OBS projector.
	windows, err := m.x.GetWindowList(m.x.Root)
	if err != nil {
		m.Errors <- err
		return
	}
	for _, win := range windows {
		title, err := m.x.GetWindowTitle(win)
		if err != nil {
			continue
		}
		if strings.Contains(title, "Projector (Scene)") {
			m.projector = win
			break
		}
	}
	if m.projector == 0 {
		// No projector found. Spawn one.
		_, err := m.o.OpenProjector("Scene", nil, "", "Wall")
		if err != nil {
			m.Errors <- fmt.Errorf("failed to spawn projector: %s", err)
			return
		}
	}
	m.x.FocusWindow(m.projector)
	m.onWall = true
	for i := range m.locks {
		m.setLock(i, false)
	}
	if m.conf.Reset.Stretch {
		sw, sh, err := m.x.ScreenSize()
		if err != nil {
			m.Errors <- fmt.Errorf("failed to get screen size: %s", err)
			return
		}
		ui.Log("Got screen size: %dx%d.", sw, sh)
		m.screenWidth = sw
		m.screenHeight = sh
		for _, v := range m.workers {
			err := v.Resize(RESIZE_WIDTH, RESIZE_HEIGHT)
			if err != nil {
				m.Errors <- fmt.Errorf("failed to resize instance: %s", err)
				return
			}
		}
		ui.Log("Stretched instances.")
	}
	if m.conf.Wall.Mouse {
		ww, wh, err := getWallSize(m.o, len(m.workers))
		ui.Log("Got wall size: %dx%d, %d.", ww, wh, len(m.workers))
		if err != nil {
			m.Errors <- fmt.Errorf("failed to get wall size: %s", err)
			return
		}
		m.wallWidth = ww
		m.wallHeight = wh
	}
	for {
		select {
		case werr := <-m.workerErrors:
			// Wait a moment and then attempt to reboot the dead worker.
			ui.LogError("Worker %d died: %s", werr.Id, werr.Err)
			time.Sleep(10 * time.Millisecond)
			err := m.workers[werr.Id].Start(m.workerErrors)
			if err != nil {
				m.Errors <- fmt.Errorf("failed to reboot worker %d: %s", werr.Id, err)
				return
			}
		case evt := <-m.x.Events:
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
					x := uint32(evt.X) / iw
					y := uint32(evt.Y) / ih
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
				x := uint32(evt.X) / iw
				y := uint32(evt.Y) / ih
				id := int((y * m.wallWidth) + x)
				if id >= len(m.workers) {
					continue
				}
				m.handleEvent(id, x11.Keymod(evt.State), evt.Timestamp)
			}
		case <-m.stop:
			// Delete cleanup tasks and run them before returning.
			ui.Log("Stopping manager...")
			for i, v := range cleanup {
				v()
				ui.Log("Cleanup: Completed task %d", i)
			}
			cleanup = make([]func(), 0)
			m.stop <- struct{}{}
			ui.Log("Stopped manager!")
			return
		}
	}
}

func (m *WallManager) handleEvent(id int, state x11.Keymod, time xproto.Timestamp) {
	switch state {
	case m.conf.Wall.Play:
		m.wallPlay(id, time)
	case m.conf.Wall.Reset:
		m.wallReset(id, time)
	case m.conf.Wall.ResetOthers:
		m.wallResetOthers(id, time)
	case m.conf.Wall.Lock:
		m.wallLock(id, time)
	}
}

func (m *WallManager) focus(evt x11.KeyEvent) {
	if m.onWall {
		m.x.FocusWindow(m.projector)
	} else {
		err := m.workers[m.current].Focus(evt.Timestamp)
		if err != nil {
			ui.LogError("Failed to focus instance %d: %s", m.current, err)
		}
	}
}

func (m *WallManager) reset(evt x11.KeyEvent) {
	if m.onWall {
		ui.Log("Resetting all instances. Waiting...")
		wg := sync.WaitGroup{}
		for _, v := range m.workers {
			wg.Add(1)
			go func(v *Worker) {
				defer wg.Done()
				if m.locks[v.instance.Id] {
					return
				}
				ui.Log("Reset instance %d.", v.instance.Id)
				err := v.Reset(evt.Timestamp)
				if err != nil {
					ui.LogError("Failed to reset instance %d: %s", m.current, err)
				}
			}(v)
		}
		wg.Wait()
		ui.Log("Reset all instances.")
	} else {
		go m.o.SetCurrentScene("Wall")
		ui.Log("Resetting instance %d; going to wall.", m.current)
		err := m.workers[m.current].Reset(evt.Timestamp)
		if err != nil {
			ui.LogError("Failed to reset instance %d: %s", m.current, err)
		}
		if m.conf.Reset.Stretch {
			err := m.workers[m.current].Resize(RESIZE_WIDTH, RESIZE_HEIGHT)
			if err != nil {
				ui.LogError("Failed to resize instance: %s", err)
				return
			}
		}
		time.Sleep(time.Duration(m.conf.Reset.Delay) * time.Millisecond)
		m.grabWallKeys()
		m.onWall = true
		err = m.x.FocusWindow(m.projector)
		if err != nil {
			ui.LogError("Failed to focus projector: %s", err)
		}
		ui.Log("Reset instance successfully.")
	}
}

func (m *WallManager) wallReset(id int, t xproto.Timestamp) {
	if m.locks[id] {
		return
	}
	ui.Log("Reset instance %d.", id)
	err := m.workers[id].Reset(t)
	if err != nil {
		ui.LogError("Failed to reset instance %d: %s", id, err)
	}
}

func (m *WallManager) wallResetOthers(id int, t xproto.Timestamp) {
	go m.o.SetCurrentScene(fmt.Sprintf("Instance %d", id+1))
	m.ungrabWallKeys()
	m.onWall = false
	m.current = id
	err := m.workers[id].Focus(t)
	if err != nil {
		ui.LogError("Failed to focus instance %d: %s", id, err)
		return
	}
	if m.conf.Reset.Stretch {
		err := m.workers[m.current].Resize(m.screenWidth, m.screenHeight)
		if err != nil {
			ui.LogError("Failed to resize instance: %s", err)
			return
		}
	}
	m.setLock(id, false)
	for i := 0; i < len(m.workers); i++ {
		if i != id && !m.locks[i] {
			ui.Log("Reset instance %d.", i)
			err := m.workers[i].Reset(t)
			if err != nil {
				ui.LogError("Failed to reset instance %d: %s", id, err)
			}
		}
	}
}

func (m *WallManager) wallPlay(id int, t xproto.Timestamp) {
	go m.o.SetCurrentScene(fmt.Sprintf("Instance %d", id+1))
	m.ungrabWallKeys()
	m.onWall = false
	m.current = id
	err := m.workers[id].Focus(t + 10)
	if err != nil {
		ui.LogError("Failed to focus instance %d: %s", id, err)
		return
	}
	if m.conf.Reset.Stretch {
		err := m.workers[m.current].Resize(m.screenWidth, m.screenHeight)
		if err != nil {
			ui.LogError("Failed to resize instance: %s", err)
			return
		}
	}
	m.setLock(id, false)
}

func (m *WallManager) wallLock(id int, t xproto.Timestamp) {
	m.setLock(id, !m.locks[id])
	ui.Log("Toggled lock state of instance %d (%t)", id, m.locks[id])
}

func (m *WallManager) setLock(i int, state bool) {
	if m.locks[i] == state {
		return
	}
	m.locks[i] = state
	res, err := m.o.GetSceneItemProperties(
		"Wall",
		obs.GetSceneItemPropertiesItem{
			Name: fmt.Sprintf("Lock %d", i+1),
		},
	)
	if err != nil {
		ui.LogError("Failed to lock instance %d: %s", i, err)
		return
	}
	b := true
	_, err = m.o.SetSceneItemProperties(
		"Wall",
		obs.SetSceneItemPropertiesItem{
			Name: fmt.Sprintf("Lock %d", i+1),
		},
		obs.SetSceneItemPropertiesPosition{
			X:         &res.Position.X,
			Y:         &res.Position.Y,
			Alignment: &res.Position.Alignment,
		},
		&res.Rotation,
		obs.SetSceneItemPropertiesScale{
			X:      &res.Scale.X,
			Y:      &res.Scale.Y,
			Filter: res.Scale.Filter,
		},
		obs.SetSceneItemPropertiesCrop{
			Top:    &res.Crop.Top,
			Right:  &res.Crop.Right,
			Bottom: &res.Crop.Bottom,
			Left:   &res.Crop.Left,
		},
		&state,
		&b,
		obs.SetSceneItemPropertiesBounds{
			Type:      res.Bounds.Type,
			Alignment: &res.Bounds.Alignment,
			X:         &res.Bounds.X,
			Y:         &res.Bounds.Y,
		},
	)
	if err != nil {
		ui.LogError("Failed to lock instance %d: %s", i, err)
		return
	}
}
