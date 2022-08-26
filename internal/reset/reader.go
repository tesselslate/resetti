package reset

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/fsnotify/fsnotify"
)

const UPDATE_CHANNEL_SIZE = 32

type LogUpdate struct {
	Id    int
	State InstanceState
	Done  bool
}

// readLog reads the log file of an instance for any state updates. The update
// channel will be closed once an error occurs or the provided context is
// cancelled.
func readLog(instance Instance, ctx context.Context) (<-chan InstanceState, error) {
	// Prepare to read the log file:
	// - Open a buffered reader
	// - Start watching the file for updates
	logPath := instance.Dir + "/logs/latest.log"
	file, err := os.Open(logPath)
	if err != nil {
		return nil, err
	}
	reader := bufio.NewReader(file)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	err = watcher.Add(logPath)
	if err != nil {
		watcher.Close()
		return nil, err
	}
	updates := make(chan InstanceState, UPDATE_CHANNEL_SIZE)
	// Extract the spawn position from a log line.
	// ... (X, Y, Z) ...
	readPos := func(line string) ([2]float64, error) {
		var pos [2]float64
		open := strings.IndexRune(line, '(')
		end := strings.IndexRune(line, ')')
		if open == -1 || end == -1 {
			return pos, errors.New("could not find parentheses")
		}
		coords := strings.Split(
			strings.ReplaceAll(line[open+1:end], " ", ""),
			",",
		)
		if len(coords) < 3 {
			log.Printf("ReadLog %d: invalid readPos: %s", instance.Id, line)
			return pos, nil
		}
		x, err := strconv.ParseFloat(coords[0], 64)
		if err != nil {
			return pos, fmt.Errorf("could not parse X position: %s", err)
		}
		z, err := strconv.ParseFloat(coords[2], 64)
		if err != nil {
			return pos, fmt.Errorf("could not parse Z position: %s", err)
		}
		pos[0] = x
		pos[1] = z
		return pos, nil
	}
	// Read any new log lines and check for state updates.
	readState := func(current InstanceState) (InstanceState, bool) {
		updated := false
		for {
			// Read the next line in the log.
			bytes, prefix, err := reader.ReadLine()
			if err != nil {
				break
			}

			// If the line is too long to be read all at once, we don't need it.
			if prefix {
				for prefix {
					_, prefix, _ = reader.ReadLine()
				}
				continue
			}

			// Process the log line.
			line := string(bytes)
			if strings.Contains(line, "CHAT") {
				continue
			} else if strings.Contains(line, "Resetting a random seed") ||
				strings.Contains(line, "Resetting the set seed") {
				current.State = StGenerating
				current.Progress = 0
				current.Spawn[0] = 0
				current.Spawn[1] = 0
				updated = true
			} else if strings.Contains(line, "Leaving world generation") {
				current.State = StGenerating
				current.Progress = 0
				current.Spawn[0] = 0
				current.Spawn[1] = 0
				updated = true
			} else if strings.Contains(line, "Preparing spawn area") {
				splits := strings.Split(line, " ")
				// HACK: This shouldn't ever be triggered, but it is seemingly possible
				// for a partial log line to get written (perhaps related to instance
				// freezing?).
				if len(splits) != 7 {
					log.Printf("ReadLog %d: invalid spawn area line: %s\n", instance.Id, line)
					continue
				}
				progress, err := strconv.Atoi(strings.TrimSuffix(splits[6], "%"))
				if err != nil {
					log.Printf("ReadLog %d: failed to read progress: %s\n", instance.Id, err)
					log.Println(line)
					continue
				}
				current.Progress = progress
				updated = true
			} else if strings.Contains(line, "Loaded 0 advancements") {
				current.State = StIdle
				updated = true
			} else if strings.Contains(line, "Starting Preview at") {
				current.State = StPreview
				updated = true
				pos, err := readPos(line)
				if err != nil {
					log.Printf("ReadLog %d: failed to read pos: %s\n", instance.Id, err)
					log.Println(line)
					continue
				}
				current.Spawn = pos
			} else if strings.Contains(line, "logged in with entity id") {
				pos, err := readPos(line)
				updated = true
				if err != nil {
					log.Printf("ReadLog %d: failed to read pos: %s\n", instance.Id, err)
					log.Println(line)
				}
				current.Spawn = pos
			}
		}
		return current, updated
	}

	// Read the log file for updates.
	go func() {
		defer file.Close()
		defer watcher.Close()
		defer close(updates)
		state := InstanceState{}
		{
			// Read the log file once when starting to check for any previous
			// updates and to reach the end of the file.
			newState, updated := readState(state)
			if updated {
				updates <- newState
				state = newState
			}
		}
		for {
			select {
			case <-ctx.Done():
				log.Printf("ReadLog %d: cancelled\n", instance.Id)
				return
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("ReadLog %d err: %s\n", instance.Id, err)
			case evt, ok := <-watcher.Events:
				if !ok {
					return
				}
				switch evt.Op {
				case fsnotify.Remove:
					log.Printf("ReadLog %d: log file removed\n", instance.Id)
					return
				case fsnotify.Rename:
					log.Printf("ReadLog %d: log file renamed\n", instance.Id)
					return
				case fsnotify.Write:
					newState, updated := readState(state)
					if updated {
						updates <- newState
						state = newState
					}
				}
			}
		}
	}()
	return updates, nil
}
