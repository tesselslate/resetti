package mc

import (
	"bufio"
	"context"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
)

const (
	log_reset_rs = "Resetting a random seed"
	log_reset_ss = "Resetting the set seed"
	log_preview  = "Starting Preview at"
	log_progress = "Preparing spawn area"
	log_loaded   = "Loaded 0 advancements"
)

// LogReader reads the log file of an instance and provides updates to its
// state.
type LogReader struct {
	Errors <-chan error
	Events <-chan InstanceState
	state  InstanceState

	id      int
	path    string
	file    *os.File
	reader  *bufio.Reader
	watcher *fsnotify.Watcher
}

// NewLogReader creates a new logReader for the given instance.
func NewLogReader(ctx context.Context, info InstanceInfo) (LogReader, InstanceState, error) {
	logFile := info.Dir + "/logs/latest.log"
	file, err := os.Open(logFile)
	if err != nil {
		return LogReader{}, InstanceState{}, errors.Wrap(err, "open log file")
	}
	reader := bufio.NewReader(file)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		_ = file.Close()
		return LogReader{}, InstanceState{}, errors.Wrap(err, "create watcher")
	}
	if err = watcher.Add(logFile); err != nil {
		_ = watcher.Close()
		_ = file.Close()
		return LogReader{}, InstanceState{}, errors.Wrap(err, "watch log file")
	}
	errch := make(chan error, 1)
	evtch := make(chan InstanceState, 32)
	r := LogReader{
		errch,
		evtch,
		InstanceState{},
		info.Id,
		logFile,
		file,
		reader,
		watcher,
	}
	_, err = r.readUpdates()
	if err != nil {
		_ = watcher.Close()
		_ = file.Close()
		return LogReader{}, InstanceState{}, errors.Wrap(err, "read state")
	}
	go r.run(ctx, errch, evtch)
	return r, r.state, nil
}

// run starts reading the log file and processing updates in the background.
func (r *LogReader) run(ctx context.Context, errch chan<- error, evtch chan<- InstanceState) {
	defer func() {
		_ = r.file.Close()
		_ = r.watcher.Close()
		close(evtch)
		close(errch)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case err, more := <-r.watcher.Errors:
			if !more {
				errch <- err
				return
			}
			log.Printf("Log reader %d errored: %s\n", r.id, r.path)
		case evt, more := <-r.watcher.Events:
			if !more {
				// TODO: Handle log file reopen, midnight bug(?)
				log.Printf("Log reader %d stopped\n", r.id)
				return
			}
			switch evt.Op {
			case fsnotify.Write:
				updated, err := r.readUpdates()
				if err != nil {
					// TODO: Handle error
					errch <- err
					return
				}
				if updated {
					evtch <- r.state
				}
			}
		}
	}
}

// readNextLine attempts to read the next log line.
func (r *LogReader) readNextLine() (string, error) {
	// Attempt to read the next log line. If we're too early after the inotify
	// event, the log line may not be fully written yet.
	buf, err := r.reader.ReadBytes('\n')
	switch err {
	case io.EOF:
		// An empty buffer means there is actually no more content.
		if len(buf) == 0 {
			return "", nil
		}

		// If the log line hasn't been fully written yet, retry the read.
		timeout := time.Millisecond
		for tries := 0; tries < 5; tries += 1 {
			time.Sleep(timeout)
			timeout *= 2

			remainder, err := r.reader.ReadBytes('\n')
			switch err {
			case io.EOF:
				if tries == 5 {
					return "", errors.New("read line (5 tries)")
				}
			case nil:
				buf = append(buf, remainder...)
				return string(buf), nil
			default:
				return "", err
			}
		}
	case nil:
		return string(buf), nil
	default:
		return "", err
	}
	panic("unreachable")
}

// readUpdates reads any updates to the log file and updates the instance state
// accordingly.
func (r *LogReader) readUpdates() (bool, error) {
	updated := false

	for {
		// Read next line. Return if there is no content (no lines left.)
		line, err := r.readNextLine()
		if err != nil {
			return updated, err
		}
		if line == "" {
			return updated, nil
		}

		// Process the line.
		if strings.Contains(line, "CHAT") {
			continue
		} else if strings.Contains(line, log_reset_rs) || strings.Contains(line, log_reset_ss) {
			r.state.State = StDirt
			r.state.Progress = 0
			updated = true
		} else if strings.Contains(line, log_preview) {
			r.state.State = StPreview
			updated = true
		} else if strings.Contains(line, log_progress) {
			// Line format:
			// [XX:XX:XX] [Render thread/INFO]: Preparing spawn area: X%
			words := strings.Split(line, " ")
			if len(words) != 7 {
				log.Printf("Log reader %d: Progress line had %d spaces\n", r.id, len(words))
				continue
			}
			progress, err := strconv.Atoi(strings.Trim(words[6], "%\n"))
			if err != nil {
				log.Printf("Log reader %d: Failed to parse progress (%s)\n", r.id, err)
				continue
			}
			r.state.Progress = progress
			updated = true
		} else if strings.Contains(line, log_loaded) {
			r.state.State = StIdle
			updated = true
		}
	}
}
