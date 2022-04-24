package manager

import (
	"bufio"
	"fmt"
	"os"
	"resetti/mc"
	"strings"

	"github.com/fsnotify/fsnotify"
)

type WatchError struct {
	Err   error
	Fatal bool
	Id    int
}

type WatchUpdate struct {
	Id    int
	State mc.InstanceState
}

type Watcher struct {
	ch chan bool
}

// Watch spawns a goroutine to watch the log file of an instance and
// notify of any necessary state updates. State updates should be discarded
// while the instance is currently being played.
func Watch(i mc.Instance, errch chan WatchError, updatech chan WatchUpdate) (*Watcher, error) {
	// Setup the log watcher.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	logfile := i.Dir + "/logs/latest.log"

	err = watcher.Add(logfile)
	if err != nil {
		return nil, err
	}

	// Open the log file.
	file, err := os.Open(logfile)
	if err != nil {
		return nil, err
	}

	reader := bufio.NewReader(file)

	// Read the log file to the end to get the current instance state.
	state, updated := readState(reader)
	if updated {
		updatech <- WatchUpdate{
			Id:    i.Id,
			State: state,
		}
	}

	stopch := make(chan bool, 1)

	// Begin watching the log file and sending any new state updates.
	go func() {
		defer watcher.Close()
		defer file.Close()
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					errch <- WatchError{
						Err:   fmt.Errorf("watcher closed"),
						Fatal: true,
						Id:    i.Id,
					}
					return
				}

				switch event.Op {
				case fsnotify.Remove, fsnotify.Rename:
					errch <- WatchError{
						Err:   fmt.Errorf("log file gone"),
						Fatal: true,
						Id:    i.Id,
					}
					return
				case fsnotify.Write:
					state, updated = readState(reader)
					if updated {
						updatech <- WatchUpdate{
							Id:    i.Id,
							State: state,
						}
					}
				}
			case err, ok := <-watcher.Errors:
				errch <- WatchError{
					Err:   err,
					Fatal: !ok,
					Id:    i.Id,
				}

				if !ok {
					return
				}
			case <-stopch:
				return
			}
		}
	}()

	watch := Watcher{
		ch: stopch,
	}

	return &watch, err
}

// Stop stops the watcher.
func (w *Watcher) Stop() {
	w.ch <- true
}

func readState(reader *bufio.Reader) (mc.InstanceState, bool) {
	lastState := mc.StateUnknown
	updated := false

	for {
		lineBytes, _, err := reader.ReadLine()
		if err != nil {
			break
		}

		line := string(lineBytes)

		if !strings.Contains(line, "CHAT") {
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
	}

	return lastState, updated
}
