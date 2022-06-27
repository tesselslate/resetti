package manager

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/logger"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/obs"
	"github.com/woofdoggo/resetti/internal/x11"
)

// StandardManager provides a Manager implementation for resetting one or more
// instances by cycling between each sequentially.
type StandardManager struct {
	stop   chan struct{}
	active sync.Mutex

	workers      []*Worker
	workerErrors chan WorkerError
	current      int

	Errors chan error
	conf   cfg.Config
}

func (m *StandardManager) Start(instances []mc.Instance, errch chan error) error {
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
	err := setAffinity(instances, m.conf.General.Affinity)
	if err != nil {
		return err
	}
	go m.run()
	return nil
}

func (m *StandardManager) Stop() {
	m.stop <- struct{}{}
	<-m.stop
}

func (m *StandardManager) Wait() {
	// Suppress "empty critical section" warning with defer.
	defer m.active.Unlock()
	m.active.Lock()
}

func (m *StandardManager) Restart(instances []mc.Instance) error {
	return m.createWorkers(instances)
}

func (m *StandardManager) SetConfig(conf cfg.Config) {
	m.conf = conf
}

func (m *StandardManager) createWorkers(instances []mc.Instance) error {
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

func (m *StandardManager) stopWorkers() {
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

func (m *StandardManager) grabKeys() {
	x11.GrabKey(m.conf.Keys.Focus)
	x11.GrabKey(m.conf.Keys.Reset)
}

func (m *StandardManager) ungrabKeys() {
	x11.UngrabKey(m.conf.Keys.Focus)
	x11.UngrabKey(m.conf.Keys.Reset)
}

func (m *StandardManager) run() {
	cleanup := []func(){
		m.active.Unlock,
		m.stopWorkers,
		m.ungrabKeys,
	}
	defer func() {
		for _, v := range cleanup {
			v()
		}
	}()

	m.grabKeys()
	xevt := make(chan any, 32)
	x11.Subscribe(nil, xevt)
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
		case evt := <-xevt:
			switch evt := evt.(type) {
			case x11.KeyEvent:
				if evt.State == x11.KeyDown {
					switch evt.Key {
					case m.conf.Keys.Focus:
						err := m.workers[m.current].Focus(evt.Timestamp)
						if err != nil {
							logger.LogError("Failed to focus worker %d: %s", m.current, err)
							continue
						}
					case m.conf.Keys.Reset:
						next := (m.current + 1) % len(m.workers)
						err := m.workers[next].Focus(evt.Timestamp)
						if err != nil {
							logger.LogError("Failed to focus worker %d: %s", m.current, err)
							continue
						}
						err = m.workers[m.current].Reset(evt.Timestamp)
						if err != nil {
							logger.LogError("Failed to reset worker %d: %s", m.current, err)
							continue
						}
						m.current = next
						logger.Log("Reset instance %d.", m.current)
						if m.conf.Obs.Enabled {
							err := obs.SetScene(fmt.Sprintf("Instance %d", m.current+1))
							if err != nil {
								logger.LogError("Failed to switch OBS scene: %s", err)
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
			logger.Log("Stopped manager!")
			return
		}
	}
}
