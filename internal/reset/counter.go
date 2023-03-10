package reset

import (
	"context"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

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
	// Return a no-op Counter if `count_resets` is disabled.
	if !conf.General.CountResets {
		return Counter{}, nil
	}

	// Read the current reset count from the file.
	file, err := os.OpenFile(conf.General.CountPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return Counter{}, err
	}
	content := make([]byte, 32)
	read, err := file.Read(content)
	if err != nil && err != io.EOF {
		_ = file.Close()
		return Counter{}, err
	}
	count := 0
	if read != 0 {
		content = content[:read]
		count, err = strconv.Atoi(strings.TrimSpace(string(content)))
		if err != nil {
			_ = file.Close()
			return Counter{}, err
		}
	}

	// Create the Counter and start running it.
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
// count. This function is only ever called if `count_resets` is enabled in the
// user's configuration, in which case c.file and c.ch will not be nil.
func (c *Counter) run(ctx context.Context, wg *sync.WaitGroup) {
	// Synchronization and teardown setup.
	wg.Add(1)
	defer func() {
		if err := c.file.Close(); err != nil {
			log.Printf("Reset counter failed to close: %s\n", err)
			log.Printf("Here's your reset count! Back it up: %d\n", c.count)
		}
		log.Println("Reset counter stopped")
		wg.Done()
	}()

	inc := func() {
		c.count += 1
		b := []byte(strconv.Itoa(c.count))

		// Rewrite the file.
		_, err := c.file.Seek(0, 0)
		if err != nil {
			log.Printf("Counter seek failed: %s\n", err)
			return
		}
		if _, err = c.file.Write(b); err != nil {
			log.Printf("Counter write failed: %s\n", err)
		}
	}

	// Main loop.
	log.Println("Reset counter started")
	for {
		select {
		case <-c.ch:
			inc()
		case <-ctx.Done():
			// It is possible that this branch is taken while some more resets
			// are queued, so we have to wait a bit and check again before
			// cleaning up.
			time.Sleep(10 * time.Millisecond)
		drain:
			for {
				select {
				case <-c.ch:
					inc()
				default:
					break drain
				}
			}
			return
		}
	}
}
