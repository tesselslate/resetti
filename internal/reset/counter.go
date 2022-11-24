package reset

import (
	"context"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/woofdoggo/resetti/internal/cfg"
)

// Counter keeps track of the number of resets performed and writes them to
// a file on disk.
type Counter struct {
	file  *os.File
	count int
	ch    chan bool
}

// NewCounter creates a new reset counter from the user's configuration profile.
// If CountResets is disabled, then the Counter will do nothing.
func NewCounter(ctx context.Context, wg *sync.WaitGroup, conf cfg.Profile) (Counter, error) {
	if !conf.General.CountResets {
		return Counter{}, nil
	}
	file, err := os.OpenFile(conf.General.CountPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return Counter{}, err
	}
	content := make([]byte, 32)
	read, err := file.Read(content)
	if err != nil && err != io.EOF {
		file.Close()
		return Counter{}, err
	}
	var count int
	if read != 0 {
		content = content[:read]
		count, err = strconv.Atoi(strings.TrimSpace(string(content)))
		if err != nil {
			file.Close()
			return Counter{}, err
		}
	}
	c := Counter{file, count, make(chan bool, 128)}
	go c.run(ctx, wg)
	return c, nil
}

// Increment increments the reset count if CountResets is enabled in the user's
// configuration, otherwise it is a no-op.
func (c *Counter) Increment() {
	if c.ch != nil {
		c.ch <- true
	}
}

// run starts listening for resets in the background and increments the reset
// count. In order to minimize pointless I/O,
//
// This function is only ever called if CountResets is true, in which case
// c.file and c.ch must not be nil.
func (c *Counter) run(ctx context.Context, wg *sync.WaitGroup) {
	wg.Add(1)
	defer func() {
		c.file.Close()
		log.Println("Service: Reset counter stopped")
		wg.Done()
	}()

	log.Println("Service: Reset counter started")
	for {
		select {
		case <-c.ch:
			c.count += 1
			b := []byte(strconv.Itoa(c.count))

			// Go to the start of the file.
			_, err := c.file.Seek(0, 0)
			if err != nil {
				log.Printf("Counter seek failed: %s\n", err)
				continue
			}
			if _, err = c.file.Write(b); err != nil {
				log.Printf("Counter write failed: %s\n", err)
			}
		case <-ctx.Done():
			// Unfortunately, this make it possible for resetti to drop
			// some resets, as we can hit this branch before emptying ch. We
			// can't nest this in a default case, though, because it would cause
			// this goroutine to enter a tight loop.
			return
		}
	}
}
