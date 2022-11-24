package reset

import (
	"bufio"
	"context"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
	"github.com/woofdoggo/resetti/internal/mc"
)

const update_channel_size = 32

type LogReader struct {
	Errors <-chan error
	Events <-chan mc.InstanceState

	id     int // instance id
	state  mc.InstanceState
	file   *os.File
	reader *bufio.Reader
	watch  *fsnotify.Watcher
}

func NewLogReader(ctx context.Context, wg *sync.WaitGroup, info mc.InstanceInfo) (LogReader, error) {
	logFile := info.Dir + "/logs/latest.log"
	fh, err := os.Open(logFile)
	if err != nil {
		return LogReader{}, errors.Wrap(err, "failed to open log")
	}
	reader := bufio.NewReader(fh)
	watch, err := fsnotify.NewWatcher()
	if err != nil {
		fh.Close()
		return LogReader{}, errors.Wrap(err, "failed to open watcher")
	}
	if err = watch.Add(logFile); err != nil {
		watch.Close()
		fh.Close()
		return LogReader{}, errors.Wrap(err, "failed to watch log")
	}
	errch := make(chan error, 1)
	evtch := make(chan mc.InstanceState, update_channel_size)
	lr := LogReader{
		errch,
		evtch,
		info.Id,
		mc.InstanceState{},
		fh,
		reader,
		watch,
	}
	_, err = lr.readState()
	if err != nil {
		watch.Close()
		fh.Close()
		return LogReader{}, errors.Wrap(err, "failed to read log")
	}
	go lr.run(ctx, wg, errch, evtch)
	return lr, nil
}

func (r *LogReader) run(ctx context.Context, wg *sync.WaitGroup, errch chan<- error, evtch chan<- mc.InstanceState) {
	wg.Add(1)
	defer func() {
		r.watch.Close()
		r.file.Close()
		close(evtch)
		close(errch)
		log.Printf("Service: Log reader %d stopped\n", r.id)
		wg.Done()
	}()

	log.Printf("Service: Log reader %d started\n", r.id)
	for {
		select {
		case <-ctx.Done():
			return
		case err, ok := <-r.watch.Errors:
			if !ok {
				errch <- err
				return
			}
			log.Printf("Error in log reader %d: %s\n", r.id, err)
		case evt, ok := <-r.watch.Events:
			if !ok {
				errch <- errors.New("no more events")
				return
			}
			switch evt.Op {
			case fsnotify.Remove, fsnotify.Rename:
				errch <- errors.New("log file removed")
				return
			case fsnotify.Write:
				// TODO: return and close channel if Stopped message is read?
				updated, err := r.readState()
				if err != nil {
					errch <- errors.Wrap(err, "log read failed")
					return
				}
				if updated {
					evtch <- r.state
				}
			}
		}
	}
}

func (r *LogReader) readState() (updated bool, err error) {
	// It is possible (likely, even) that we will manage to read a partially
	// written log line. As such, we will attempt to read new file contents
	// multiple times until a line break is encountered. If no line break is
	// found, then we can return an error.
	for {
		// Read the next line.
		buf, err := r.reader.ReadBytes('\n')
		if err == io.EOF {
			// If there is no content, we've actually reached the end of the log
			// file (until the next line is written.)
			if len(buf) == 0 {
				return updated, nil
			}

			// If there *is* content in the buffer, we read a partially written
			// log line. We'll try to read the remainder of the line a few times.
			d := time.Millisecond
			for tries := 1; tries <= 5; tries += 1 {
				time.Sleep(d)
				d *= 2
				remainder, err := r.reader.ReadBytes('\n')
				buf = append(buf, remainder...)

				if err == io.EOF {
					if tries == 5 {
						return false, errors.New("failed to read a full line")
					}
					continue
				} else if err != nil {
					return false, err
				}
				break
			}
		} else if err != nil {
			return false, err
		}
		line := string(buf)

		// Process the log line.
		if strings.Contains(line, "CHAT") {
			// Skip chat messages.
			continue
		} else if strings.Contains(line, "Resetting a random seed") ||
			strings.Contains(line, "Resetting the set seed") ||
			strings.Contains(line, "Leaving world generation") {
			// We got to the dirt screen.
			r.state.State = mc.StDirt
			r.state.Progress = 0
			updated = true
		} else if strings.Contains(line, "Starting Preview at") {
			r.state.State = mc.StPreview
			updated = true
		} else if strings.Contains(line, "Preparing spawn area") {
			// Update the world generation progress.
			words := strings.Split(line, " ")
			if len(words) != 7 {
				// This log line should have 6 spaces.
				log.Printf("Instance %d: Preparing spawn area had %d spaces\n", r.id, len(words))
				continue
			}
			progress, err := strconv.Atoi(strings.Trim(words[6], "%\n"))
			if err != nil {
				log.Printf("Instance %d: Failed to read generation percentage: %s\n", r.id, err)
				continue
			}
			r.state.Progress = progress
			updated = true
		} else if strings.Contains(line, "Loaded 0 advancements") {
			// World generation has finished.
			r.state.State = mc.StIdle
			updated = true
		}
	}
}
