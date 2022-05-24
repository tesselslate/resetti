package manager

import (
	"fmt"
	"resetti/cfg"
	"resetti/mc"
	"resetti/x11"
	"time"
)

// StandardManager provides a Manager implementation for standard
// single/multi instance resetting.
type StandardManager struct {
	managerState

	active bool
	stop   chan bool
}

func (s *StandardManager) Start(instances []mc.Instance) error {
	if s.active {
		return fmt.Errorf("manager already running")
	}

	// setup channels
	s.stop = make(chan bool, 1)
	s.wErrCh = make(chan WorkerError, 32)
	s.wCmdCh = make([]chan WorkerCommand, len(instances))
	for i := 0; i < len(instances); i++ {
		s.wCmdCh[i] = make(chan WorkerCommand, 8)
	}

	// setup workers
	s.workers = make([]*Worker, len(instances))
	for i := 0; i < len(instances); i++ {
		worker, err := NewWorker(s, instances[i])
		if err != nil {
			return err
		}

		err = worker.Run(s.wCmdCh[i], s.wErrCh)
		if err != nil {
			return err
		}

		s.workers[i] = worker
	}

	s.active = true
	go s.run()

	return nil
}

func (s *StandardManager) Stop() error {
	if !s.active {
		return fmt.Errorf("manager already stopped")
	}

	s.stop <- true
	return nil
}

func (s *StandardManager) GetConfig() cfg.ResetSettings {
	return s.conf.Reset
}

func (s *StandardManager) GetX() *x11.Client {
	return s.x
}

func (s *StandardManager) cleanup() {
	for i := 0; i < len(s.workers); i++ {
		s.workers[i].Stop()
	}
}

func (s *StandardManager) run() {
	defer s.cleanup()

	for {
		select {
		case err := <-s.wErrCh:
			// Handle worker error.
			if err.Fatal {
				// If worker error is fatal, try to reboot the worker.
				time.Sleep(100 * time.Millisecond)
				err := s.workers[err.Id].Run(s.wCmdCh[err.Id], s.wErrCh)
				if err != nil {
					// TODO report error
					return
				}
			}

			// TODO report error
		case <-s.stop:
			// Stop.
			s.active = false
			return
		}
	}
}
