package reset

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/fsnotify/fsnotify"
)

type threadUpdate struct {
	Id    int
	Added bool
	Tid   int
}

func watchProcThreads(ctx context.Context, inst Instance, ch chan<- threadUpdate) (map[int]struct{}, error) {
	// Begin watching for any thread creations/deletions.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	dir := fmt.Sprintf("/proc/%d/task", inst.Pid)
	err = watcher.Add(dir)
	if err != nil {
		watcher.Close()
		return nil, err
	}
	go func() {
		defer watcher.Close()
		for {
			select {
			case err, ok := <-watcher.Errors:
				if !ok {
					log.Printf("Thread watcher %d fatal error: %s\n", inst.Id, err)
					return
				}
				log.Printf("Thread watcher %d error: %s\n", inst.Id, err)
			case evt, ok := <-watcher.Events:
				if !ok {
					log.Printf("Thread watcher %d: no more events", inst.Id)
					return
				}
				tid, err := strconv.Atoi(evt.Name)
				if err != nil {
					log.Printf("Thread watcher %d failed atoi: %s", inst.Id, err)
				}
				ch <- threadUpdate{
					Id:    inst.Id,
					Added: evt.Op == fsnotify.Create,
					Tid:   tid,
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Get the list of currently existing threads.
	threadList, err := getProcThreads(inst)
	if err != nil {
		return nil, err
	}
	threads := make(map[int]struct{})
	for _, v := range threadList {
		threads[v] = struct{}{}
	}
	return threads, nil
}

func getProcThreads(inst Instance) ([]int, error) {
	threads := make([]int, 0)
	dirEntries, err := os.ReadDir(fmt.Sprintf("/proc/%d/task", inst.Pid))
	if err != nil {
		return nil, err
	}
	for _, entry := range dirEntries {
		tid, err := strconv.Atoi(entry.Name())
		if err != nil {
			return nil, err
		}
		threads = append(threads, tid)
	}
	return threads, nil
}
