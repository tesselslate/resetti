package manager

import (
	"bufio"
	"fmt"
	"os"
	"resetti/cfg"
	"resetti/mc"
	"resetti/x11"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/jezek/xgb/xproto"
)

// WorkerCommand represents a single command issued to a worker.
type WorkerCommand struct {
	Op   int
	Time xproto.Timestamp
}

const (
	CmdFocus int = iota
	CmdReset
)

// WorkerError contains an error reported by a worker.
type WorkerError struct {
	Err   error
	Fatal bool
	Id    int
}

// Worker manages a single instance.
type Worker struct {
	conf     cfg.ResetSettings
	instance mc.Instance
	time     xproto.Timestamp

	reader  *bufio.Reader
	logfile *os.File
	watcher Watcher
	manager Manager

	mx      sync.Mutex
	active  bool
	stop    chan bool
	errch   chan WorkerError
	statech chan mc.Instance
}

// NewWorker creates a new Worker.
func NewWorker(mgr Manager, instance mc.Instance) (*Worker, error) {
	// Open log file.
	logfile := instance.Dir + "/logs/latest.log"
	file, err := os.Open(logfile)
	if err != nil {
		return nil, err
	}
	reader := bufio.NewReader(file)

	return &Worker{
		conf:     mgr.GetConfig(),
		instance: instance,
		time:     0,
		reader:   reader,
		logfile:  file,
		watcher:  NewWatcher(logfile),
		manager:  mgr,
		mx:       sync.Mutex{},
		active:   false,
		stop:     make(chan bool, 1),
	}, nil
}

// Run begins running the worker.
func (w *Worker) Run(cmdch chan WorkerCommand, errch chan WorkerError, statech chan mc.Instance) error {
	// Check if worker is already running.
	if w.active {
		return fmt.Errorf("worker already running")
	}

	// Perform initial state update.
	state, updated := w.readState()
	if updated {
		w.instance.State = state
		statech <- w.instance
	}

	// Start log watcher.
	w.active = true
	if err := w.watcher.Watch(); err != nil {
		return err
	}

	// Start worker loop.
	w.errch = errch
	w.statech = statech
	go w.loop(cmdch)
	return nil
}

// Stop stops the worker.
func (w *Worker) Stop() error {
	// Check if worker is already stopped.
	if !w.active {
		return fmt.Errorf("worker already stopped")
	}

	// Emit stop signal.
	w.stop <- true
	return nil
}

// GetState gets the Worker's state.
func (w *Worker) GetState() mc.InstanceState {
	w.mx.Lock()
	defer w.mx.Unlock()
	return w.instance.State
}

// SetState sets the Worker's state.
func (w *Worker) SetState(state mc.InstanceState) {
	w.mx.Lock()
	defer w.mx.Unlock()
	w.instance.State = state
	w.statech <- w.instance
}

// cleanup cleans up the Worker's resources.
func (w *Worker) cleanup() {
	w.watcher.Stop()
	w.logfile.Close()
}

// loop runs the main Worker loop.
func (w *Worker) loop(cmdch chan WorkerCommand) {
	defer w.cleanup()

	for {
		select {
		case error := <-w.watcher.Errors:
			// Handle error from log watcher.

			// If the error is not fatal, continue.
			if !error.Fatal {
				continue
			}

			// If the error is fatal, try to reboot the log watcher 10
			// times. If all attempts fail, consider the worker dead and
			// notify the manager.
			if err := w.rebootWatcher(); err != nil {
				w.errch <- WorkerError{
					Err:   err,
					Fatal: true,
					Id:    w.instance.Id,
				}

				w.active = false
				return
			}
		case event := <-w.watcher.Updates:
			// Handle event from log watcher.
			switch event.Op {
			case fsnotify.Remove, fsnotify.Rename:
				// If the log file was removed or renamed, try to first reboot
				// the watcher.
				if err := w.rebootWatcher(); err != nil {
					w.errch <- WorkerError{
						Err:   err,
						Fatal: true,
						Id:    w.instance.Id,
					}

					w.active = false
					return
				}
			case fsnotify.Write:
				// Process any state updates from the log.
				w.updateState()
			}
		case cmd := <-cmdch:
			// Handle command from manager.
			switch cmd.Op {
			case CmdFocus:
				// Focus the instance's window.
				x := w.manager.GetX()
				x.FocusWindow(w.instance.Window)

				if w.instance.State == mc.StatePaused {
					x.SendKeyPress(x11.KeyEscape, w.instance.Window, &cmd.Time)
				}

				w.mx.Lock()
				w.time = cmd.Time
				w.instance.State = mc.StateIngame
				w.statech <- w.instance
				w.mx.Unlock()
			case CmdReset:
				// Reset the instance.
				w.time = cmd.Time
				go w.reset()
			}
		case <-w.stop:
			// Clean up resources.
			w.active = false
			return
		}
	}
}

// readState attempts to read the latest instance state from its log file.
func (w *Worker) readState() (mc.InstanceState, bool) {
	lastState := mc.StateUnknown
	updated := false

	for {
		lineBytes, _, err := w.reader.ReadLine()
		if err != nil {
			break
		}

		line := string(lineBytes)
		if strings.Contains(line, "CHAT") {
			continue
		}

		if strings.Contains(line, "Resetting a random seed") {
			lastState = mc.StateGenerating
			updated = true
		} else if strings.Contains(line, "Saving chunks for level") && strings.Contains(line, "the_end") {
			lastState = mc.StatePaused
			updated = true
		} else if strings.Contains(line, "Starting Preview at") {
			lastState = mc.StatePreview
			updated = true
		}
	}

	return lastState, updated
}

// reset resets the Worker's instance.
func (w *Worker) reset() {
	w.mx.Lock()
	defer w.mx.Unlock()

	time, err := w.instance.Reset(&w.conf, w.manager.GetX(), w.time)
	if err != nil {
		w.errch <- WorkerError{
			Err:   err,
			Fatal: false,
			Id:    w.instance.Id,
		}
		return
	}

	w.time = time
}

// updateState reads the instance's log and performs any necessary actions
// when the instance's state is updated.
func (w *Worker) updateState() {
	x := w.manager.GetX()

	state, updated := w.readState()
	if !updated {
		return
	}

	// Check if the state update should be discarded.
	if w.instance.State == state {
		return
	}

	if w.instance.State == mc.StateIngame && state == mc.StatePaused {
		return
	}

	// Update the instance's state.
	w.mx.Lock()
	w.instance.State = state
	w.statech <- w.instance
	w.mx.Unlock()

	// Check if any action needs to be taken.
	activeWin, err := x.GetActiveWindow()
	if err != nil {
		w.errch <- WorkerError{
			Err:   err,
			Fatal: false,
			Id:    w.instance.Id,
		}
		return
	}

	isPreview := w.instance.State == mc.StatePreview
	isPaused := w.instance.State == mc.StatePaused
	isActive := activeWin == w.instance.Window

	// If the instance is both active and paused, then the
	// user is currently playing.
	if isActive && isPaused {
		w.mx.Lock()
		w.instance.State = mc.StateIngame
		w.statech <- w.instance
		w.mx.Unlock()
		return
	}

	// If the instance is entering WorldPreview or the world itself,
	// then press F3+Escape to get the transparent pause menu.
	if isPreview || isPaused {
		time.Sleep(time.Duration(w.conf.Delay) * time.Millisecond)
		x.SendKeyDown(x11.KeyF3, w.instance.Window, &w.time)
		x.SendKeyPress(x11.KeyEscape, w.instance.Window, &w.time)
		x.SendKeyUp(x11.KeyF3, w.instance.Window, &w.time)
	}
}

// rebootWatcher attempts to reboot the log watcher upon failure. It will try
// a total of 10 times with increasing delay between each attempt.
func (w *Worker) rebootWatcher() error {
	// Try to reboot the log watcher 10 times with exponential backoff.
	delay := time.Duration(1 * time.Millisecond)
	for j := 0; j < 10; j++ {
		err := w.watcher.Watch()
		if err != nil {
			time.Sleep(delay)
			delay *= 2
			continue
		}

		// If the reboot attempt succeeds, return.
		return nil
	}

	// If all 10 attempts fail, return an error.
	return fmt.Errorf("could not restart watcher")
}
