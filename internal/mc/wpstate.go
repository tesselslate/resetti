package mc

import (
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
)

// wpstateReader reads an instance's wpstateout.txt and provides state updates.
type wpstateReader struct {
	state State
	path  string
	file  *os.File
}

// newWpstateReader creates a new wpstateReader for the given instance.
func newWpstateReader(inst InstanceInfo) (wpstateReader, State, error) {
	path := inst.Dir + "/wpstateout.txt"
	file, err := os.Open(path)
	if err != nil {
		return wpstateReader{}, State{}, errors.Wrap(err, "open log")
	}
	reader := wpstateReader{State{}, path, file}
	state, _, err := reader.Process()
	if err != nil {
		return wpstateReader{}, State{}, errors.Wrap(err, "read state")
	}
	return reader, state, nil
}

// Path implements stateReader.
func (r *wpstateReader) Path() string {
	return r.path
}

// Process implements stateReader.
func (r *wpstateReader) Process() (State, bool, error) {
	buf := make([]byte, 32)
	n, err := r.file.Read(buf)
	if err != nil && err != io.EOF {
		return r.state, false, err
	}
	if n == 0 {
		return r.state, false, nil
	}

	buf = buf[:n]
	a, b, split := strings.Cut(string(buf), ",")
	switch a {
	case "title":
		r.state.Type = StMenu
		r.state.Progress = 0
		r.state.Menu = false
	case "waiting":
		r.state.Type = StDirt
		r.state.Progress = 0
		r.state.Menu = false
	case "generating":
		r.state.Type = StDirt
		if !split {
			return r.state, false, errors.New("no generating split")
		}
		progress, err := strconv.Atoi(b)
		if err != nil {
			return r.state, false, err
		}
		r.state.Progress = progress
		r.state.Menu = false
	case "previewing":
		r.state.Type = StPreview
		if !split {
			return r.state, false, errors.New("no previewing split")
		}
		progress, err := strconv.Atoi(b)
		if err != nil {
			return r.state, false, err
		}
		r.state.Progress = progress
		r.state.Menu = false
	case "inworld":
		if r.state.Type != StIngame {
			r.state.Type = StIdle
		}
		if !split {
			return r.state, false, errors.New("no inworld split")
		}
		r.state.Menu = b != "unpaused"
	default:
		return r.state, false, errors.Errorf("unrecognized log type: %s", a)
	}
	return r.state, true, nil
}

// ProcessEvent implements stateReader.
func (r *wpstateReader) ProcessEvent(op fsnotify.Op) error {
	// TODO: Recovery
	return nil
}
