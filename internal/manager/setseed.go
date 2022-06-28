package manager

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/jezek/xgb/xproto"
	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/logger"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/obs"
	"github.com/woofdoggo/resetti/internal/x11"
)

type SSGState int

const (
	Idle SSGState = iota
	Playing
	Resetting
	Ready
	ReadySoon
)

type SetseedManager struct {
	stop   chan struct{}
	active sync.Mutex

	workers      []*Worker
	states       []SSGState
	workerErrors chan WorkerError
	updates      chan mc.Instance
	current      int
	waiting      bool
	projector    xproto.Window

	Errors chan error
	conf   cfg.Config
}

func (m *SetseedManager) Start(instances []mc.Instance, errch chan error) error {
	m.conf = cfg.GetConfig()
	if len(instances) == 0 {
		return errors.New("no instances")
	}
	if !m.active.TryLock() {
		return errors.New("already running")
	}
	err := obs.SetupScenes(instances)
	if err != nil {
		return err
	}
	m.stop = make(chan struct{})
	m.workerErrors = make(chan WorkerError, len(instances))
	m.updates = make(chan mc.Instance, 64)
	m.Errors = errch
	if err := m.createWorkers(instances); err != nil {
		return err
	}
	m.states = make([]SSGState, len(m.workers))
	for i := 0; i < len(m.states); i++ {
		m.states[i] = Idle
	}
	err = setAffinity(instances, m.conf.General.Affinity)
	if err != nil {
		return err
	}
	go m.run()
	return nil
}

func (m *SetseedManager) Stop() {
	logger.Log("Manager: Sent stop signal...")
	m.stop <- struct{}{}
	<-m.stop
	logger.Log("Manager stop signal returned!")
}

func (m *SetseedManager) Wait() {
	// Suppress "empty critical section" warning with defer.
	defer m.active.Unlock()
	m.active.Lock()
}

func (m *SetseedManager) createWorkers(instances []mc.Instance) error {
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

func (m *SetseedManager) stopWorkers() {
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

func (m *SetseedManager) grabKeys() {
	x11.GrabKey(m.conf.Keys.Focus)
	x11.GrabKey(m.conf.Keys.Reset)
}

func (m *SetseedManager) ungrabKeys() {
	x11.UngrabKey(m.conf.Keys.Focus)
	x11.UngrabKey(m.conf.Keys.Reset)
}

func (m *SetseedManager) run() {
	m.grabKeys()
	defer m.ungrabKeys()
	defer m.stopWorkers()
	defer m.active.Unlock()
	xevt := make(chan any, 32)
	x11.Subscribe(nil, xevt)
	// Locate OBS projector.
	windows, err := x11.GetAllWindows()
	if err != nil {
		m.Errors <- err
		return
	}
	for _, win := range windows {
		title, err := x11.GetWindowTitle(win)
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
		err := obs.OpenProjector()
		if err != nil {
			m.Errors <- fmt.Errorf("failed to spawn projector: %s", err)
			return
		}
	}
	x11.FocusWindow(m.projector)
	go obs.SetScene("Wall")
	m.waiting = true
	initialTime := xproto.Timestamp(0)
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
		case e := <-xevt:
			evt, ok := e.(x11.KeyEvent)
			if !ok {
				continue
			}
			initialTime = evt.Timestamp
			if evt.State == x11.KeyDown {
				if !m.waiting {
					switch evt.Key {
					case m.conf.Keys.Focus:
						err := m.workers[m.current].Focus(evt.Timestamp)
						if err != nil {
							logger.LogError("Failed to focus instance %d: %s", m.current, err)
						}
					case m.conf.Keys.Reset:
						// Reset current instance.
						err := m.workers[m.current].Reset(evt.Timestamp)
						if err != nil {
							logger.LogError("Failed to reset instance %d: %s", m.current, err)
						} else {
							logger.Log("Reset instance %d.", m.current)
						}
						// Check for new instance to play.
						needToWait := true
						m.states[m.current] = Resetting
						for i, v := range m.states {
							if v == Ready {
								err := m.workers[i].Focus(evt.Timestamp)
								if err != nil {
									logger.LogError("Failed to switch to instance %d: %s", i, err)
									continue
								}
								m.current = i
								m.states[i] = Playing
								needToWait = false
								go obs.SetScene(fmt.Sprintf("Instance %d", i+1))
								break
							}
						}
						if needToWait {
							m.waiting = true
							logger.Log("No instances ready. Waiting...")
							go obs.SetScene("Wall")
							err := x11.FocusWindow(m.projector)
							if err != nil {
								logger.LogError("Failed to focus projector: %s", err)
							}
						}
					}
				} else { // m.waiting
					switch evt.Key {
					case m.conf.Keys.Focus:
						err := x11.FocusWindow(m.projector)
						if err != nil {
							logger.LogError("Failed to focus projector: %s", err)
						}
					case m.conf.Keys.Reset:
						// Set seed if instance is on main menu.
						for _, v := range m.workers {
							v.SetSeed(evt.Timestamp)
						}
						foundReady := false
						for i, v := range m.states {
							if v == Idle || v == Resetting {
								err := m.workers[i].Reset(0)
								if err != nil {
									logger.LogError("Failed to background reset instance %d: %s", i, err)
								}
							}
							if !foundReady && v == Ready {
								m.current = i
								err := m.workers[i].Focus(evt.Timestamp)
								go obs.SetScene(fmt.Sprintf("Instance %d", i+1))
								foundReady = true
								if err != nil {
									logger.LogError("Failed to focus instance %d: %s", i, err)
								}
								logger.Log("Finished waiting! Found ready: %d", i)
								m.waiting = false
							}
						}
					}
				}
			}
		case u := <-m.updates:
			switch u.State.Identifier {
			case mc.StateReady, mc.StatePreview:
				dx := math.Abs(m.conf.SSG.SpawnX - u.State.Spawn.X)
				dz := math.Abs(m.conf.SSG.SpawnZ - u.State.Spawn.Z)
				dist := math.Sqrt(dx*dx + dz*dz)
				if dist <= m.conf.SSG.Radius {
					if u.State.Identifier == mc.StateReady {
						logger.Log("Instance %d ready! Distance: %f", u.Id, dist)
						m.states[u.Id] = Ready
					} else {
						m.states[u.Id] = ReadySoon
					}
				} else {
					m.states[u.Id] = Resetting
					logger.Log("Instance %d: Bad spawn (%f, %f, %f)", u.Id, dist, u.State.Spawn.X, u.State.Spawn.Z)
					if initialTime != 0 {
						err := m.workers[u.Id].Reset(0)
						if err != nil {
							logger.LogError("Failed to background reset instance %d: %s", u.Id, err)
						}
					}
				}
			case mc.StateIngame:
				m.states[u.Id] = Playing
			}
		case <-m.stop:
			logger.Log("Stopping manager...")
			m.stop <- struct{}{}
			return
		}
	}
}
