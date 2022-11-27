package reset

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/pkg/errors"
	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/obs"
	"github.com/woofdoggo/resetti/internal/x11"
)

// Multi implements a traditional multi-instance resetting setup, where
// the user plays one instance before resetting and switching to the next
// instance.
type Multi struct {
	conf cfg.Profile
	obs  *obs.Client
	x    *x11.Client

	logReaders []LogReader
	states     []mc.InstanceState
	instances  []mc.Instance
	current    int // the index of the current instance
}

// NewMulti creates a new Multi for multi-instance resetting.
func NewMulti(conf cfg.Profile, infos []mc.InstanceInfo, x *x11.Client) Multi {
	multi := Multi{
		conf:       conf,
		x:          x,
		logReaders: make([]LogReader, 0, len(infos)),
	}
	multi.instances = make([]mc.Instance, 0, len(infos))
	for _, info := range infos {
		multi.instances = append(multi.instances, mc.NewInstance(info, &conf, x))
	}
	multi.states = make([]mc.InstanceState, len(infos))
	return multi
}

// Run attempts to run the multi-instance resetter. If an error occurs during
// setup, it will be returned.
func (m *Multi) Run() error {
	// Setup synchronization primitives.
	sigs := make(chan os.Signal, 16)
	signal.Notify(sigs, syscall.SIGINT)
	ctx, cancel := context.WithCancel(context.Background())
	wg := sync.WaitGroup{}
	defer func() {
		cancel()
		log.Println("Waiting for services to stop...")
		wg.Wait()
		log.Println("Stopping!")
	}()

	// Start log readers and click instances to fix the Atum bug.
	for i, v := range m.instances {
		reader, err := NewLogReader(ctx, &wg, v.InstanceInfo)
		if err != nil {
			return errors.Wrap(err, "failed to setup log reader")
		}
		m.logReaders = append(m.logReaders, reader)
		if _, err = reader.readState(); err != nil {
			return errors.Wrap(err, "failed to read state")
		}
		m.states[i] = reader.state
		if err = v.Click(); err != nil {
			return errors.Wrap(err, "failed to click")
		}
	}
	updates, readerErrors := mux(m.logReaders)

	// Setup OBS and the reset counter.
	var obsError <-chan error
	if m.conf.Obs.Enabled {
		m.obs = &obs.Client{}
		errch, err := m.obs.Connect(ctx, fmt.Sprintf("localhost:%d", m.conf.Obs.Port), m.conf.Obs.Password)
		obsError = errch
		if err != nil {
			return err
		}
		err = m.obs.SetSceneCollection(fmt.Sprintf("resetti - %d multi", len(m.instances)))
		if err != nil {
			return errors.Wrap(err, "failed to set scene collection")
		}
	}
	counter, err := NewCounter(ctx, &wg, m.conf)
	if err != nil {
		return err
	}

	// Perform miscellaneous tasks.
	if err = m.x.GrabKey(m.conf.Keys.Focus, m.x.RootWindow()); err != nil {
		return errors.Wrap(err, "failed to grab focus key")
	}
	if err = m.x.GrabKey(m.conf.Keys.Reset, m.x.RootWindow()); err != nil {
		return errors.Wrap(err, "failed to grab reset key")
	}
	defer func() {
		// It doesn't matter if these return an error because the program is
		// exiting and the X server will remove any grabs then anyway.
		_ = m.x.UngrabKey(m.conf.Keys.Focus, m.x.RootWindow())
		_ = m.x.UngrabKey(m.conf.Keys.Reset, m.x.RootWindow())
	}()
	m.instances[0].Focus()
	if m.obs != nil {
		if err = setSources(m.obs, m.instances); err != nil {
			return errors.Wrap(err, "failed to set OBS sources")
		}
		if err = m.obs.SetScene("Instance 1"); err != nil {
			return errors.Wrap(err, "failed to set scene")
		}
	}
	printDebugInfo(m.x, m.instances)
	xEvt, xErr, err := m.x.Poll(ctx, false)
	if err != nil {
		return err
	}

	// Start the main loop.
	for {
		select {
		case <-sigs:
			log.Println("Received SIGINT.")
			return nil
		case err := <-obsError:
			log.Printf("Critical OBS error: %s\n", err)
			return nil
		case err := <-xErr:
			if err == x11.ErrDied {
				log.Println("X connection closed")
				return nil
			}
			log.Printf("Unhandled X error: %s\n", err)
		case err := <-readerErrors:
			log.Printf("Fatal reader error: %s\n", err)
			return nil
		case update := <-updates:
			state := update.State
			id := update.Id

			// Pause the instance if it is now idle and not focused.
			nowIdle := m.states[id].State != mc.StIdle && state.State == mc.StIdle
			if nowIdle && m.current != id {
				m.instances[id].Pause(0)
			}
			m.states[id] = state
		case evt := <-xEvt:
			// This cast should be infallible, as we only ever grab keys from
			// the X server, not the mouse pointer. If it ever fails, something
			// *very* weird is going on.
			key := evt.(x11.KeyEvent)
			if key.State != x11.KeyDown {
				continue
			}
			switch key.Key {
			case m.conf.Keys.Focus:
				m.instances[m.current].Focus()
			case m.conf.Keys.Reset:
				next := (m.current + 1) % len(m.instances)
				m.instances[next].FocusAndUnpause(key.Time, m.states[next].State == mc.StIdle)
				m.instances[m.current].Reset(key.Time)
				if m.obs != nil {
					err = m.obs.SetScene(fmt.Sprintf("Instance %d", next+1))
					if err != nil {
						log.Printf("Failed to set scene: %s\n", err)
					}
				}
				go runHook(m.conf.Hooks.Reset)
				counter.Increment()
				log.Printf("Reset instance %d\n", m.current)
				m.current = next
			}
		}
	}
}
