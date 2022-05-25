package manager

import (
	"errors"
	"fmt"
	"resetti/cfg"
	"resetti/mc"
	"resetti/x11"
	"time"

	obs "github.com/woofdoggo/go-obs"
)

// StandardManager provides a Manager implementation for standard
// single/multi instance resetting.
type StandardManager struct {
	managerState

	active bool
	stop   chan bool

	current int
}

func (s *StandardManager) Setup(x *x11.Client, o *obs.Client, c cfg.Config) {
	s.x = x
	s.o = o
	s.conf = c
}

func (s *StandardManager) Start(instances []mc.Instance, stateCh chan mc.Instance) error {
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

	s.mErrCh = make(chan error)
	s.mStateCh = stateCh

	// setup workers
	s.workers = make([]*Worker, len(instances))
	for i := 0; i < len(instances); i++ {
		worker, err := NewWorker(s, instances[i])
		if err != nil {
			return err
		}

		err = worker.Run(s.wCmdCh[i], s.wErrCh, s.mStateCh)
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
	s.x.UngrabKey(s.conf.Keys.Focus)
	s.x.UngrabKey(s.conf.Keys.Reset)
	s.x.LoopStop()
}

func (s *StandardManager) run() {
	defer s.cleanup()
	s.x.GrabKey(s.conf.Keys.Focus)
	s.x.GrabKey(s.conf.Keys.Reset)
	xerr, xevt := s.x.Loop()

	for i := 0; i < len(s.workers); i++ {
		win := s.workers[i].instance.Window
		s.x.SetTitle(win, fmt.Sprintf("Minecraft | Instance %d", i+1))
	}

	for {
		select {
		case err := <-s.wErrCh:
			// Handle worker error.
			if err.Fatal {
				// If worker error is fatal, try to reboot the worker.
				time.Sleep(100 * time.Millisecond)
				err := s.workers[err.Id].Run(s.wCmdCh[err.Id], s.wErrCh, s.mStateCh)
				if err != nil {
					s.mErrCh <- err
					return
				}
			}
		case err := <-xerr:
			if err.Error() == "connection died" {
				s.mErrCh <- errors.New("x connection died")
			}
		case evt := <-xevt:
			if evt.State == x11.KeyDown {
				switch evt.Key {
				case s.conf.Keys.Focus:
					s.wCmdCh[s.current] <- WorkerCommand{
						Op:   CmdFocus,
						Time: evt.Timestamp,
					}
				case s.conf.Keys.Reset:
					s.wCmdCh[s.current] <- WorkerCommand{
						Op:   CmdReset,
						Time: evt.Timestamp,
					}
					next := (s.current + 1) % len(s.workers)
					if next != s.current {
						s.current = next
						s.x.FocusWindow(s.workers[s.current].instance.Window)
						if s.workers[s.current].GetState() == mc.StatePaused {
							s.x.SendKeyPress(
								x11.KeyEscape,
								s.workers[s.current].instance.Window,
								&evt.Timestamp,
							)
						}
						s.workers[s.current].SetState(mc.StateIngame)
						if s.o != nil {
							obs.NewSetCurrentSceneRequest(s.o, fmt.Sprintf("Instance %d", s.current))
						}
					}
				}
			}
		case <-s.stop:
			// Stop.
			s.active = false
			return
		}
	}
}
