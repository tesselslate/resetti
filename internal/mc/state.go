package mc

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"golang.org/x/exp/slices"
)

// Log lines
const (
	logResetRs  = "Resetting a random seed"
	logResetSs  = "Resetting the set seed"
	logPreview  = "Starting Preview at"
	logProgress = "Preparing spawn area"
	logLoaded   = "Loaded 0 advancements"
)

// Instance state types
const (
	// Main menu
	StMenu int = iota

	// Dirt world generation screen
	StDirt

	// World preview screen
	StPreview

	// In the world, paused and ready to be played
	StIdle

	// In the world, currently being played
	StIngame

	// In the world.
	//
	// This is an internal state. It can be returned by StateReader.Process(),
	// but should be converted to the appropriate state (either StIdle or
	// StIngame) by the Manager.
	stWorld
)

// State reader choice
var (
	forceLog     = slices.Contains(os.Args, "--force-log")
	forceWpstate = slices.Contains(os.Args, "--force-wpstate")
)

// State names
var stateNames = [...]string{
	"menu",
	"dirt",
	"preview",
	"idle",
	"ingame",
	"world",
}

// State contains information about the current state of an instance.
type State struct {
	// Current main state (e.g. dirt, preview)
	Type int

	// World generation progress (0 to 100)
	Progress int

	// Whether or not the instance is in a menu (e.g. pause, inventory).
	// Requires WorldPreview state reader to detect.
	Menu bool

	// The last time the instance reached the world preview screen.
	LastPreview time.Time

	// The last time the instance was reset.
	LastReset time.Time
}

// The StateReader interface provides a method for obtaining the state of an
// instance (e.g. generating, previewing, ingame.)
//
// There are currently two implementations: the traditional log reader, and the
// newer wpstateout.txt reader. The wpstateout.txt reader is preferred and
// should be used whenever possible, as it is simpler, faster, and more
// featureful.
type StateReader interface {
	// Path returns the path of the file being read.
	Path() string

	// Process reads any changes to the file and returns any state updates.
	Process() (state State, updated bool, err error)

	// ProcessEvent handles a non-modification change to the file, such as it
	// being deleted or moved. A non-nil error return signals an irrecoverable
	// failure.
	ProcessEvent(fsnotify.Op) error
}

// Update contains a change to the state of a specific instance.
type Update struct {
	State State
	Id    int
}

// logReader reads an instance's standard log file and provides state
// updates.
type logReader struct {
	state  State
	path   string
	file   *os.File
	reader *bufio.Reader
}

// wpstateReader reads an instance's wpstateout.txt and provides state updates.
type wpstateReader struct {
	state State
	path  string
	file  *os.File
}

// createStateReader attempts to create the best StateReader for the given
// instance.
func createStateReader(inst InstanceInfo) (StateReader, State, error) {
	// TODO: Better state detection heuristic (WorldPreview jar version?)

	// Decide which state reader to create.
	_, err := os.Stat(inst.Dir + "/wpstateout.txt")
	if err != nil && !os.IsNotExist(err) {
		return nil, State{}, fmt.Errorf("stat %d/wpstateout.txt: %w", inst.Id, err)
	}
	if forceLog && forceWpstate {
		return nil, State{}, errors.New("can only force one state reader type")
	}
	useLogReader := forceLog || (!forceWpstate && os.IsNotExist(err))

	// Create the state reader.
	if useLogReader {
		reader, state, err := newLogReader(inst)
		if err != nil {
			return nil, State{}, fmt.Errorf("create logReader %d: %w", inst.Id, err)
		}
		return &reader, state, nil
	} else {
		if os.IsNotExist(err) {
			return nil, State{}, errors.New("cannot force wpstate reader without wpstateout.txt")
		}
		reader, state, err := newWpstateReader(inst)
		if err != nil {
			return nil, State{}, fmt.Errorf("create wpstateReader %d: %w", inst.Id, err)
		}
		return &reader, state, nil
	}
}

// newLogReader creates a new logReader for the given instance.
func newLogReader(inst InstanceInfo) (logReader, State, error) {
	path := inst.Dir + "/logs/latest.log"
	file, err := os.Open(path)
	if err != nil {
		return logReader{}, State{}, fmt.Errorf("open log: %w", err)
	}
	r := bufio.NewReader(file)
	reader := logReader{State{}, path, file, r}
	state, _, err := reader.Process()
	if err != nil {
		_ = file.Close()
		return logReader{}, State{}, fmt.Errorf("read state: %w", err)
	}
	return reader, state, nil
}

// newWpstateReader creates a new wpstateReader for the given instance.
func newWpstateReader(inst InstanceInfo) (wpstateReader, State, error) {
	path := inst.Dir + "/wpstateout.txt"
	file, err := os.Open(path)
	if err != nil {
		return wpstateReader{}, State{}, fmt.Errorf("open log: %w", err)
	}
	reader := wpstateReader{State{}, path, file}
	state, _, err := reader.Process()
	if err != nil {
		return wpstateReader{}, State{}, fmt.Errorf("read state: %w", err)
	}
	return reader, state, nil
}

// Path implements StateReader.
func (r *logReader) Path() string {
	return r.path
}

// Process implements StateReader.
func (r *logReader) Process() (State, bool, error) {
	if r.file == nil {
		return State{}, false, nil
	}

	updated := false
	for {
		line, err := r.readLine()
		if err != nil {
			return r.state, updated, err
		}
		if line == "" {
			return r.state, updated, nil
		}

		// Process the log line.
		if strings.Contains(line, "CHAT") {
			continue
		} else if strings.Contains(line, logResetRs) || strings.Contains(line, logResetSs) {
			r.state.Type = StDirt
			r.state.Progress = 0
			updated = true
		} else if strings.Contains(line, logPreview) {
			r.state.Type = StPreview
			updated = true
		} else if strings.Contains(line, logProgress) {
			// [XX:XX:XX] [Render thread/INFO]: Preparing spawn area: X%
			words := strings.Split(line, " ")
			if len(words) != 7 {
				log.Printf("logReader.process: Progress line had %d words\n", len(words))
				continue
			}
			progress, err := strconv.Atoi(strings.Trim(words[6], "%\n"))
			if err != nil {
				log.Printf("logReader.process: Failed to parse progress (%s)\n", err)
			}
			r.state.Progress = progress
			updated = true
		} else if strings.Contains(line, logLoaded) {
			r.state.Type = stWorld
			updated = true
		}
	}
}

// ProcessEvent implements StateReader.
func (r *logReader) ProcessEvent(op fsnotify.Op) error {
	switch op {
	case fsnotify.Remove:
		log.Println("logReader.ProcessEvent: Log file is gone.")
		if r.file == nil {
			return nil
		}
		if err := r.file.Close(); err != nil {
			log.Printf("Failed to close log: %s\n", err)
		}
		log.Println("logReader.ProcessEvent: Closed log. Waiting for return.")
		r.file = nil
		r.reader = nil
	case fsnotify.Create:
		log.Println("logReader.ProcessEvent: Log file is back!")
		file, err := os.Open(r.path)
		if err != nil {
			return fmt.Errorf("reopen log: %w", err)
		}
		r.file = file
		r.reader = bufio.NewReader(r.file)
	}
	return nil
}

func (r *logReader) readLine() (string, error) {
	// Attempt to read the log file until the next newline is reached.
	// Possible outcomes:
	//   err == nil                     We read a line.
	//   err == io.EOF && len(buf) == 0 There is not another line to read.
	//   err == io.EOF && len(buf) != 0 There is a partial line, keep reading.
	buf, err := r.reader.ReadBytes('\n')
	switch err {
	case nil:
		return string(buf), nil
	case io.EOF:
		if len(buf) == 0 {
			return "", nil
		}

		timeout := time.Millisecond
		for tries := 0; tries < 5; tries += 1 {
			time.Sleep(timeout)
			timeout *= 2

			remainder, err := r.reader.ReadBytes('\n')
			buf = append(buf, remainder...)
			switch err {
			case io.EOF:
				continue
			case nil:
				log.Printf("logReader.readLine: succeeded after %d tries\n", tries+1)
				return string(buf), nil
			default:
				return "", err
			}
		}
		return "", errors.New("read failed (5 tries)")
	default:
		return "", err
	}
}

// Path implements StateReader.
func (r *wpstateReader) Path() string {
	return r.path
}

// Process implements StateReader.
func (r *wpstateReader) Process() (State, bool, error) {
	buf := make([]byte, 32)
	_, err := r.file.Seek(0, 0)
	if err != nil {
		return r.state, false, err
	}
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
		r.state.Type = stWorld
		if !split {
			return r.state, false, errors.New("no inworld split")
		}
		r.state.Menu = b != "unpaused"
	default:
		return r.state, false, fmt.Errorf("unrecognized log type: %s", a)
	}
	return r.state, true, nil
}

// ProcessEvent implements StateReader.
func (r *wpstateReader) ProcessEvent(op fsnotify.Op) error {
	switch op {
	case fsnotify.Remove, fsnotify.Rename:
		return errors.New("wpstateout.txt gone")
	default:
		return nil
	}
}
