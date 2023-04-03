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

const (
	log_reset_rs = "Resetting a random seed"
	log_reset_ss = "Resetting the set seed"
	log_preview  = "Starting Preview at"
	log_progress = "Preparing spawn area"
	log_loaded   = "Loaded 0 advancements"
)

// LogReader reads the log files of multiple instances and provides a stream
// of state updates.
type LogReader struct {
	readers []instanceReader
	watcher *fsnotify.Watcher
}

type instanceReader struct {
	state  State
	path   string
	file   *os.File
	reader *bufio.Reader
}

// NewLogReader creates a new LogReader for the given instances.
func NewLogReader(info []InstanceInfo) (LogReader, []State, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return LogReader{}, nil, errors.Wrap(err, "create watcher")
	}

	readers := make([]instanceReader, 0)
	states := make([]State, 0)
	for idx, info := range info {
		logFile := info.Dir + "/logs/latest.log"
		file, err := os.Open(logFile)
		if err != nil {
			return LogReader{}, nil, errors.Wrapf(err, "open log file %d", idx)
		}
		if err = watcher.Add(logFile); err != nil {
			return LogReader{}, nil, errors.Wrapf(err, "watch log file %d", idx)
		}
		r := bufio.NewReader(file)
		reader := instanceReader{
			State{},
			logFile,
			file,
			r,
		}
		readers = append(readers, reader)
		if _, err = reader.process(); err != nil {
			return LogReader{}, nil, errors.Wrapf(err, "read state %d", idx)
		}
		states = append(states, reader.state)
	}

	reader := LogReader{readers, watcher}
	return reader, states, nil
}

// Run starts reading all instance log files and processing updates.
func (r *LogReader) Run(errch chan<- error, evtch chan<- Update) {
	defer func() {
		if err := r.watcher.Close(); err != nil {
			log.Printf("LogReader: failed to close watcher: %s\n", err)
		}
		for idx, reader := range r.readers {
			if err := reader.file.Close(); err != nil {
				log.Printf("LogReader: failed to close file %d: %s\n", idx, err)
			}
		}
	}()

	for {
		select {
		case err, more := <-r.watcher.Errors:
			if !more {
				errch <- err
				return
			}
			log.Printf("LogReader: watcher errored: %s\n", err)
		case evt, more := <-r.watcher.Events:
			if !more {
				// TODO: Handle closing properly (e.g. midnight bug?)
				log.Println("LogReader: watcher stopped")
				return
			}
			switch evt.Op {
			case fsnotify.Write:
				for idx, reader := range r.readers {
					if reader.path == evt.Name {
						updated, err := reader.process()
						if err != nil {
							// TODO: Handle error
							errch <- err
							return
						}
						if updated {
							evtch <- Update{reader.state, idx}
						}
					}
				}
			}
		}
	}
}

// process reads any new log lines.
func (r *instanceReader) process() (bool, error) {
	updated := false
	for {
		line, err := r.readLine()
		if err != nil {
			return updated, err
		}
		if line == "" {
			return updated, nil
		}

		// Process the log line.
		if strings.Contains(line, "CHAT") {
			continue
		} else if strings.Contains(line, log_reset_rs) || strings.Contains(line, log_reset_ss) {
			r.state.Type = StDirt
			r.state.Progress = 0
			updated = true
		} else if strings.Contains(line, log_preview) {
			r.state.Type = StPreview
			updated = true
		} else if strings.Contains(line, log_progress) {
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
		} else if strings.Contains(line, log_loaded) {
			r.state.Type = StIdle
			updated = true
		}
	}
}

// readLine reads a single log line.
func (r *instanceReader) readLine() (string, error) {
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
