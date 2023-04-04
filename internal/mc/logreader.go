package mc

import (
	"bufio"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
)

// Log lines
const (
	logResetRs  = "Resetting a random seed"
	logResetSs  = "Resetting the set seed"
	logPreview  = "Starting Preview at"
	logProgress = "Preparing spawn area"
	logLoaded   = "Loaded 0 advancements"
)

// logReader reads an instance's standard log file and provides state
// updates.
type logReader struct {
	state  State
	path   string
	file   *os.File
	reader *bufio.Reader
}

// newLogReader creates a new logReader for the given instance.
func newLogReader(inst InstanceInfo) (logReader, State, error) {
	path := inst.Dir + "/logs/latest.log"
	file, err := os.Open(path)
	if err != nil {
		return logReader{}, State{}, errors.Wrap(err, "open log")
	}
	r := bufio.NewReader(file)
	reader := logReader{State{}, path, file, r}
	state, _, err := reader.Process()
	if err != nil {
		_ = file.Close()
		return logReader{}, State{}, errors.Wrap(err, "read state")
	}
	return reader, state, nil
}

// Path implements stateReader.
func (r *logReader) Path() string {
	return r.path
}

// Process implements stateReader.
func (r *logReader) Process() (State, bool, error) {
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
				log.Printf("process: Progress line had %d words\n", len(words))
				continue
			}
			progress, err := strconv.Atoi(strings.Trim(words[6], "%\n"))
			if err != nil {
				log.Printf("process: Failed to parse progress (%s)\n", err)
			}
			r.state.Progress = progress
			updated = true
		} else if strings.Contains(line, logLoaded) {
			r.state.Type = StIdle
			updated = true
		}
	}
}

// ProcessEvent implements stateReader.
func (r *logReader) ProcessEvent(op fsnotify.Op) error {
	// TODO: Recovery (e.g. midnight bug)
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
				log.Printf("succeeded after %d tries\n", tries+1)
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
