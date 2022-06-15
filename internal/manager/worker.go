package manager

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/logger"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/ui"
	"github.com/woofdoggo/resetti/internal/x11"

	"github.com/fsnotify/fsnotify"
	"github.com/jezek/xgb/xproto"
)

var (
	ErrCannotReset error = errors.New("invalid state for resetting")
)

type WorkerError struct {
	Err error
	Id  int
}

// Worker manages a single Minecraft instance and its state.
type Worker struct {
	sync.Mutex
	stop   chan struct{}
	active sync.Mutex

	conf cfg.Config

	reader   *bufio.Reader
	watcher  *fsnotify.Watcher
	instance mc.Instance
	lastTime xproto.Timestamp
	update   chan<- mc.Instance
}

// Start begins running the Worker's goroutine in the background.
func (w *Worker) Start(errch chan<- WorkerError) error {
	if !w.active.TryLock() {
		return errors.New("worker already running")
	}
	w.stop = make(chan struct{})
	path := w.instance.Dir + "/logs/latest.log"
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	w.reader = bufio.NewReader(file)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	w.watcher = watcher
	err = watcher.Add(path)
	if err != nil {
		watcher.Close()
		return err
	}
	state, _ := w.readState()
	w.setState(state)
	go w.run(errch)
	return nil
}

// Subscribe allows the manager of the Worker to receive any state updates
// and act upon them. This is primarily used for the set seed manager.
func (w *Worker) Subscribe(ch chan<- mc.Instance) {
	w.update = ch
}

// Stop stops the Worker's background goroutine. This function will hang
// permanently if the Worker is not running.
func (w *Worker) Stop() {
	// Send the stop signal. Since the channel is unbuffered, it cannot be
	// immediately received again here. Then, we wait for the Worker
	// goroutine to send another value back to signal that it is finished.
	w.stop <- struct{}{}
	<-w.stop
	logger.Log("Stopped worker %d!", w.instance.Id)
}

// SetConfig sets the worker's configuration.
func (w *Worker) SetConfig(c cfg.Config) {
	w.Lock()
	w.conf = c
	w.Unlock()
}

// SetInstance sets the instance the worker will manage.
func (w *Worker) SetInstance(i mc.Instance) {
	w.instance = i
}

// Focus waits for the Worker to finish its current task before focusing the
// instance's window.
func (w *Worker) Focus(time xproto.Timestamp) error {
	w.Lock()
	defer w.Unlock()
	w.lastTime = time
	err := x11.FocusWindow(w.instance.Window)
	if err != nil {
		return err
	}
	// If the instance is ready (generated, paused), then unpause it.
	if w.instance.State.Identifier == mc.StateReady {
		w.setStateId(mc.StateIngame)
		if w.conf.Reset.UnpauseFocus {
			x11.SendKeyPress(x11.KeyEscape, w.instance.Window, &w.lastTime)
		}
	}
	return nil
}

// Reset waits for the Worker to finish its current task before focusing the
// instance's window.
func (w *Worker) Reset(time xproto.Timestamp) error {
	w.Lock()
	defer w.Unlock()
	if w.instance.State.Identifier == mc.StateGenerating {
		return ErrCannotReset
	}
	if time == xproto.TimeCurrentTime {
		time = w.lastTime
	}
	time, err := w.instance.Reset(&w.conf, time)
	w.lastTime = time
	w.setStateId(mc.StateGenerating)
	return err
}

// Resize will resize the window of the Worker's instance.
func (w *Worker) Resize(width, height uint16) error {
	return x11.MoveWindow(w.instance.Window, 0, 0, uint32(width), uint32(height))
}

// SetSeed sets the seed of the instance if it is on the main menu.
func (w *Worker) SetSeed(timestamp xproto.Timestamp) {
	w.lastTime = timestamp
	w.Lock()
	defer w.Unlock()
	if w.instance.State.Identifier != mc.StateUnknown {
		return
	}
	x11.SendKeyDown(x11.KeyShift, w.instance.Window, &w.lastTime)
	x11.SendKeyPress(x11.KeyTab, w.instance.Window, &w.lastTime)
	x11.SendKeyPress(x11.KeyEnter, w.instance.Window, &w.lastTime)
	x11.SendKeyUp(x11.KeyShift, w.instance.Window, &w.lastTime)
	time.Sleep(time.Duration(w.conf.Reset.Delay) * time.Millisecond)
	x11.SendKeyDown(x11.KeyCtrl, w.instance.Window, &w.lastTime)
	x11.SendKeyPress(x11.KeyA, w.instance.Window, &w.lastTime)
	x11.SendKeyPress(x11.KeyBackspace, w.instance.Window, &w.lastTime)
	x11.SendKeyUp(x11.KeyCtrl, w.instance.Window, &w.lastTime)
	time.Sleep(time.Duration(w.conf.Reset.Delay) * time.Millisecond)
	for _, c := range w.conf.SSG.Seed {
		if c == '-' {
			x11.SendKeyPressAlt(x11.KeyMinus, w.instance.Window, &w.lastTime)
		} else if c >= '1' && c <= '9' {
			x11.SendKeyPressAlt(xproto.Keycode(10+c-'1'), w.instance.Window, &w.lastTime)
		} else if c == '0' {
			x11.SendKeyPressAlt(x11.Key0, w.instance.Window, &w.lastTime)
		}
	}
	time.Sleep(time.Duration(w.conf.Reset.Delay) * time.Millisecond)
	x11.SendKeyPressAlt(x11.KeyTab, w.instance.Window, &w.lastTime)
	x11.SendKeyPressAlt(x11.KeyTab, w.instance.Window, &w.lastTime)
	x11.SendKeyPress(x11.KeyEnter, w.instance.Window, &w.lastTime)
	time.Sleep(time.Duration(w.conf.Reset.Delay) * time.Millisecond)
}

func (w *Worker) run(errch chan<- WorkerError) {
	for {
		select {
		case err, ok := <-w.watcher.Errors:
			if !ok {
				errch <- WorkerError{
					err,
					w.instance.Id,
				}
				w.active.Unlock()
				return
			}
			logger.LogError("File watcher error: %s", err)
		case evt, ok := <-w.watcher.Events:
			if !ok {
				errch <- WorkerError{
					errors.New("log watcher closed"),
					w.instance.Id,
				}
				w.active.Unlock()
				return
			}
			switch evt.Op {
			case fsnotify.Write:
				w.updateState()
			case fsnotify.Remove, fsnotify.Rename:
				errch <- WorkerError{
					errors.New("log file no longer available"),
					w.instance.Id,
				}
				w.watcher.Close()
				w.active.Unlock()
				return
			}
		case <-w.stop:
			// Signal to the sender that this goroutine is finished.
			w.stop <- struct{}{}
			w.watcher.Close()
			w.active.Unlock()
			return
		}
	}
}

func (w *Worker) readState() (mc.InstanceState, bool) {
	state := w.instance.State
	updated := false
	for {
		lineBytes, _, err := w.reader.ReadLine()
		if err != nil {
			break
		}
		line := string(lineBytes)
		if strings.Contains(line, "CHAT") {
			continue
		} else if strings.Contains(line, "Resetting a random seed") || strings.Contains(line, "Resetting the set seed") {
			state.Identifier = mc.StateGenerating
			updated = true
		} else if strings.Contains(line, "Saving and pausing game...") {
			state.Identifier = mc.StateReady
			updated = true
		} else if strings.Contains(line, "Starting Preview at") {
			state.Identifier = mc.StatePreview
			updated = true
			pos, err := readPos(line)
			if err != nil {
				logger.LogError("Failed to read position in 'preview' message: %s", err)
				continue
			}
			state.Spawn = pos
		} else if strings.Contains(line, "Leaving world generation") {
			state.Identifier = mc.StateGenerating
			updated = true
		} else if strings.Contains(line, "Saving chunks for level") {
			start := strings.Index(line, "ServerLevel")
			if start == -1 {
				logger.LogError("Failed to find world name in 'saving chunks' message")
				continue
			}
			line = line[start:]
			open := strings.IndexRune(line, '[')
			end := strings.IndexRune(line, ']')
			if open == -1 || end == -1 {
				logger.LogError("Could not find index of brackets in 'saving chunks' message")
				continue
			}
			state.World = line[open+1 : end]
			updated = true
		} else if strings.Contains(line, "logged in with entity id") {
			pos, err := readPos(line)
			if err != nil {
				logger.LogError("Failed to read position in 'logged in' message: %s", err)
				continue
			}
			state.Spawn = pos
			state.Identifier = mc.StateReady
			updated = true
		}
	}
	return state, updated
}

func (w *Worker) updateState() {
	w.Lock()
	defer w.Unlock()
	state, updated := w.readState()
	// Preliminary checks:
	// If no state updates were logged, then no action is needed.
	if !updated {
		return
	}
	// If the state did not change, then no action is needed.
	if w.instance.State.Identifier == state.Identifier {
		return
	}
	// If the instance is already being played on, it cannot switch
	// directly to the Ready state. This condition should only be met
	// when playing on a LAN world.
	if w.instance.State.Identifier == mc.StateIngame && state.Identifier == mc.StateReady {
		return
	}
	w.setState(state)
	activeWin, err := x11.GetActiveWindow()
	if err != nil {
		logger.LogError("Failed to get active window: %s", err)
		return
	}
	isPreview := w.instance.State.Identifier == mc.StatePreview
	isReady := w.instance.State.Identifier == mc.StateReady
	isActive := activeWin == w.instance.Window
	// If the window is currently focused and enters the Ready state, then the
	// player wants to play it and it can be switched to Ingame.
	if isActive && isReady {
		w.setStateId(mc.StateIngame)
		return
	}
	// If the instance is not currently focused and it has either switched
	// to the WorldPreview menu or finished generating, press F3+Esc
	// to get the transparent pause menu.
	if (!isActive || !w.conf.Reset.UnpauseFocus) && (isPreview || isReady) {
		time.Sleep(time.Duration(w.conf.Reset.Delay) * time.Millisecond)
		x11.SendKeyDown(x11.KeyF3, w.instance.Window, &w.lastTime)
		x11.SendKeyPress(x11.KeyEscape, w.instance.Window, &w.lastTime)
		x11.SendKeyUp(x11.KeyF3, w.instance.Window, &w.lastTime)
	}
}

func (w *Worker) setState(s mc.InstanceState) {
	w.instance.State = s
	ui.UpdateInstance(w.instance)
	if w.update != nil {
		w.update <- w.instance
	}
}

func (w *Worker) setStateId(s int) {
	w.instance.State.Identifier = s
	ui.UpdateInstance(w.instance)
	if w.update != nil {
		w.update <- w.instance
	}
}

func readPos(s string) (mc.Position, error) {
	pos := mc.Position{}
	open := strings.IndexRune(s, '(')
	end := strings.IndexRune(s, ')')
	if open == -1 || end == -1 {
		return pos, errors.New("could not find parentheses")
	}
	splits := strings.Split(s[open+1:end], ",")
	x, err := strconv.ParseFloat(splits[0], 64)
	if err != nil {
		return pos, fmt.Errorf("could not parse X of 'logged in' message: %s", err)
	}
	z, err := strconv.ParseFloat(strings.TrimSpace(splits[2]), 64)
	if err != nil {
		return pos, fmt.Errorf("could not parse Z of 'logged in' message: %s", err)
	}
	return mc.Position{X: x, Z: z}, nil
}
