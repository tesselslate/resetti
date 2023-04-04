package ctl

import (
	"context"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/woofdoggo/resetti/internal/cfg"
)

// counter keeps track of the number of resets performed and writes them to a
// file on disk.
type counter struct {
	file      *os.File
	lastWrite time.Time
	count     int
	inc       chan bool
}

// newCounter creates a new counter with the given configuration profile. If
// the user has count_resets disabled, the counter will do nothing.
func newCounter(conf *cfg.Profile) (counter, error) {
	if !conf.General.CountResets {
		return counter{}, nil
	}

	file, err := os.OpenFile(conf.General.CountPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return counter{}, errors.Wrap(err, "open file")
	}
	buf := make([]byte, 32)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		_ = file.Close()
		return counter{}, errors.Wrap(err, "read file")
	}
	resets := 0
	if n != 0 {
		buf = buf[:n]
		resets, err = strconv.Atoi(strings.TrimSpace(string(buf)))
		if err != nil {
			_ = file.Close()
			return counter{}, errors.Wrap(err, "parse reset count")
		}
	}

	return counter{file, time.Now(), resets, make(chan bool, bufferSize)}, nil
}

// Increment increments the reset counter.
func (c *counter) Increment() {
	c.lastWrite = time.Now()
	if c.inc != nil {
		c.inc <- true
	}
}

// increment adds 1 to the reset count and writes it to the count file.
func (c *counter) increment() {
	c.count += 1
	// TODO: Use lastWrite to batch writes.
	buf := []byte(strconv.Itoa(c.count))
	_, err := c.file.Seek(0, 0)
	if err != nil {
		log.Printf("Reset counter: seek failed: %s\n", err)
		return
	}
	n, err := c.file.Write(buf)
	if err != nil {
		log.Printf("Reset counter: write failed: %s\n", err)
	} else if n != len(buf) {
		log.Printf("Reset counter: write failed: not a full write (%d/%d)\n", n, len(buf))
	}
}

// Run starts processing resets in the background.
func (c *counter) Run(ctx context.Context, wg *sync.WaitGroup) {
	wg.Add(1)
	defer func() {
		if err := c.file.Close(); err != nil {
			log.Printf("Reset counter: close failed: %s\n", err)
			log.Printf("Here's your reset count! Back it up: %d\n", c.count)
		}
		wg.Done()
	}()
	for {
		select {
		case <-ctx.Done():
			// Drain the channel of any more reset increments.
			time.Sleep(10 * time.Millisecond)
			for range c.inc {
				c.increment()
			}
		case <-c.inc:
			c.increment()
		}
	}
}
