package reset

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jezek/xgb/xproto"
	go_obs "github.com/woofdoggo/go-obs"
	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/x11"
)

func ResetCycle(conf cfg.Profile, instances []Instance) error {
	// Start X connection.
	var x *x11.Client
	x, err := x11.NewClient()
	if err != nil {
		return err
	}
	xEvt, xErr, err := x.Poll()
	if err != nil {
		return err
	}
	go func() {
		for err := range xErr {
			log.Printf("X err: %s", err)
		}
	}()

	// Start OBS connection.
	var obs *go_obs.Client
	if conf.Obs.Enabled {
		obs = &go_obs.Client{}
		needsAuth, obsErr, err := obs.Connect(fmt.Sprintf("localhost:%d", conf.Obs.Port))
		if err != nil {
			return err
		}
		if needsAuth {
			err := obs.Login(conf.Obs.Password)
			if err != nil {
				return err
			}
		}
		err = setSceneCollection(obs, fmt.Sprintf("resetti - %d multi", len(instances)))
		if err != nil {
			return err
		}
		go func() {
			for err := range obsErr {
				log.Printf("OBS err: %s", err)
			}
		}()
	}

	// Grab keys.
	x.GrabKey(conf.Keys.Focus, x.RootWindow())
	x.GrabKey(conf.Keys.Reset, x.RootWindow())
	defer x.UngrabKey(conf.Keys.Focus, x.RootWindow())
	defer x.UngrabKey(conf.Keys.Reset, x.RootWindow())

	// Focus first instance and set OBS scene.
	x.FocusWindow(instances[0].Wid)
	setScene(obs, "Instance 1")

	// Start log readers.
	logUpdates := make(chan LogUpdate, UPDATE_CHANNEL_SIZE)
	logCtx, stopLogReaders := context.WithCancel(context.Background())
	defer stopLogReaders()
	for i, inst := range instances {
		ctx, cancel := context.WithCancel(logCtx)
		ch, err := readLog(inst, ctx)
		if err != nil {
			cancel()
			return err
		}
		go func(id int) {
			for {
				update, more := <-ch
				logUpdates <- LogUpdate{
					Id:    id,
					State: update,
					Done:  !more,
				}
				if !more {
					cancel()
					return
				}
			}
		}(i)
	}

	// Start main loop.
	current := 0
	states := make([]InstanceState, len(instances))
	lastTime := make([]xproto.Timestamp, len(instances))
	for {
		select {
		case update := <-logUpdates:
			// If a log reader channel was closed, something went wrong.
			if update.Done {
				log.Println("ResetCycle err: log reader closed")
				return nil
			}
			// Ignore updates which do not modify the main state.
			if update.State.State == states[update.Id].State {
				continue
			}
			states[update.Id] = update.State
			// If an instance entered preview or finished generating and is *not*
			// the active instance, press F3+Escape.
			if update.State.State == StPreview ||
				(current != update.Id && update.State.State == StIdle) {
				go func() {
					time.Sleep(time.Duration(conf.Reset.Delay) * time.Millisecond)
					v14_pause(x, instances[update.Id], &lastTime[update.Id])
				}()
			}
		case evt := <-xEvt:
			key := evt.(x11.KeyEvent)
			if key.State == x11.KeyDown {
				switch key.Key {
				case conf.Keys.Focus:
					err := x.FocusWindow(instances[current].Wid)
					if err != nil {
						log.Printf("ResetCycle err: failed to focus %d: %s", current, err)
					}
				case conf.Keys.Reset:
					next := (current + 1) % len(instances)
					err := x.FocusWindow(instances[next].Wid)
					if err != nil {
						log.Printf("ResetCycle err: failed to focus %d: %s", current, err)
						continue
					}
					if conf.Reset.UnpauseFocus && states[next].State == StIdle {
						x.SendKeyPress(
							x11.KeyEscape,
							instances[next].Wid,
							&lastTime[next],
						)
					}
					lastTime[current] = key.Time
					v14_reset(x, instances[current], &lastTime[current])
					log.Printf("ResetCycle: reset %d", current)
					current = next
					err = setScene(obs, fmt.Sprintf("Instance %d", current+1))
					if err != nil {
						log.Printf("ResetCycle err: failed to set scene: %s", err)
					}
				}
			}
		}
	}
}
