package manager

import (
	"errors"
	"fmt"
	"resetti/cfg"
	"resetti/mc"
	"resetti/ui"
	"resetti/x11"
	"strings"
	"sync"
	"time"

	"github.com/jezek/xgb/xproto"
	obs "github.com/woofdoggo/go-obs"
)

type WallManager struct {
	stop   chan struct{}
	active sync.Mutex

	workers      []*Worker
	workerErrors chan WorkerError
	current      int
	onWall       bool
	wallGrab     bool

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
	go m.run()
	return nil
}

func (m *WallManager) Stop() {
	m.stop <- struct{}{}
	<-m.stop
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
	}
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
	}
}

func (m *WallManager) run() {
	cleanup := []func(){
		m.active.Unlock,
		m.stopWorkers,
		m.ungrabKeys,
		m.ungrabWallKeys,
	}
	defer func() {
		for _, v := range cleanup {
			v()
		}
	}()

	m.grabKeys()
	m.grabWallKeys()
	// Locate OBS projector.
	var projector xproto.Window
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
			projector = win
			break
		}
	}
	if projector == 0 {
		// No projector found. Spawn one.
		_, err := m.o.OpenProjector("Scene", nil, "", "Wall")
		if err != nil {
			m.Errors <- fmt.Errorf("failed to spawn projector: %s", err)
			return
		}
	}
	m.x.FocusWindow(projector)
	m.onWall = true
	for {
		select {
		case werr := <-m.workerErrors:
			// Wait a moment and then attempt to reboot the dead worker.
			time.Sleep(10 * time.Millisecond)
			err := m.workers[werr.Id].Start(m.workerErrors)
			if err != nil {
				m.Errors <- fmt.Errorf("failed to reboot worker %d: %s", werr.Id, err)
				return
			}
		case evt := <-m.x.Keys:
			if evt.State == x11.KeyDown {
				switch evt.Key {
				case m.conf.Keys.Focus:
					if m.onWall {
						m.x.FocusWindow(projector)
					} else {
						err := m.workers[m.current].Focus(evt.Timestamp)
						if err != nil {
							ui.LogError("failed to focus instance %d: %s", err)
						}
					}
				case m.conf.Keys.Reset:
					if m.onWall {
						for _, v := range m.workers {
							go func(v *Worker) {
								err := v.Reset(evt.Timestamp)
								if err != nil {
									ui.LogError("failed to reset instance %d: %s", err)
								}
							}(v)
						}
					} else {
						go m.o.SetCurrentScene("Wall")
						m.x.FocusWindow(projector)
						m.grabWallKeys()
						m.onWall = true
						go func() {
							err := m.workers[m.current].Reset(evt.Timestamp)
							if err != nil {
								ui.LogError("failed to reset instance %d: %s", err)
							}
						}()
					}
				default:
					if evt.Key.Code < 10 || evt.Key.Code > 19 {
						continue
					}
					id := int(evt.Key.Code - 10)
					switch evt.Key.Mod {
					case m.conf.Wall.Play:
						go m.o.SetCurrentScene(fmt.Sprintf("Instance %d", id+1))
						m.ungrabWallKeys()
						m.onWall = false
						err := m.workers[id].Focus(evt.Timestamp)
						if err != nil {
							ui.LogError("failed to focus instance %d: %s", err)
							continue
						}
					case m.conf.Wall.Reset:
						err := m.workers[id].Reset(evt.Timestamp)
						if err != nil {
							ui.LogError("failed to reset instance %d: %s", err)
						}
					case m.conf.Wall.ResetOthers:
						go m.o.SetCurrentScene(fmt.Sprintf("Instance %d", id+1))
						m.ungrabWallKeys()
						m.onWall = false
						err := m.workers[id].Focus(evt.Timestamp)
						if err != nil {
							ui.LogError("failed to focus instance %d: %s", err)
							continue
						}
						for i := 0; i < len(m.workers); i++ {
							if i != id {
								err := m.workers[id].Reset(evt.Timestamp)
								if err != nil {
									ui.LogError("failed to reset instance %d: %s", err)
								}
							}
						}
					}
				}
			}
		case <-m.stop:
			// Delete cleanup tasks and run them before returning.
			for _, v := range cleanup {
				v()
			}
			cleanup = make([]func(), 0)
			m.stop <- struct{}{}
			return
		}
	}
}
