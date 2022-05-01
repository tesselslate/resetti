package manager

import (
	"fmt"

	"github.com/fsnotify/fsnotify"
)

// WatchError represents an error encountered by a file watcher.
type WatchError struct {
	Err   error
	Fatal bool
}

// Watcher sends notifications whenever a file is updated.
type Watcher struct {
	Errors  chan WatchError
	Updates chan fsnotify.Event

	active  bool
	file    string
	stopch  chan bool
	watcher *fsnotify.Watcher
}

// NewWatcher creates a new Watcher.
func NewWatcher(file string) Watcher {
	return Watcher{
		Errors:  make(chan WatchError, 32),
		Updates: make(chan fsnotify.Event, 32),

		active:  false,
		file:    file,
		stopch:  make(chan bool, 1),
		watcher: nil,
	}
}

// Watch spawns a goroutine which will send a notification whenever the
// file it is watching is updated.
func (w *Watcher) Watch() error {
	if w.active {
		return fmt.Errorf("watcher is already running")
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	w.watcher = watcher

	err = w.watcher.Add(w.file)
	if err != nil {
		return err
	}

	// Begin watching the file and sending notifications.
	go func() {
		w.active = true
		defer w.cleanup()

		for {
			select {
			case event, ok := <-w.watcher.Events:
				if !ok {
					w.Errors <- WatchError{
						Err:   fmt.Errorf("watcher closed"),
						Fatal: true,
					}
					return
				}

				w.Updates <- event
			case err, ok := <-w.watcher.Errors:
				w.Errors <- WatchError{
					Err:   err,
					Fatal: !ok,
				}

				if !ok {
					return
				}
			case <-w.stopch:
				return
			}
		}
	}()

	return err
}

// Stop stops the watcher.
func (w *Watcher) Stop() {
	w.stopch <- true
}

func (w *Watcher) cleanup() {
	w.watcher.Close()
	w.active = false
}
