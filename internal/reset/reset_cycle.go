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
		client, obsErr, err := connectObs(conf, len(instances))
		if err != nil {
			return err
		}
		err = setSources(client, instances)
		if err != nil {
			return err
		}
		obs = client
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
	logUpdates, stopLogReaders, err := startLogReaders(instances)
	if err != nil {
		return err
	}
	defer stopLogReaders()

	// Start UI.
	display := newResetDisplay(instances)
	uiStateUpdates, _, uiStopped, err := display.Init()
	if err != nil {
		return err
	}
	ctx, cancelUi := context.WithCancel(context.Background())
	display.Run(ctx, false)
	defer display.Fini()
	defer cancelUi()

	// Start main loop.
	current := 0
	states := make([]InstanceState, len(instances))
	lastTime := make([]xproto.Timestamp, len(instances))
	for {
		select {
		case <-uiStopped:
			return nil
		case update := <-logUpdates:
			// If a log reader channel was closed, something went wrong.
			if update.Done {
				log.Println("ResetCycle err: log reader closed")
				return nil
			}
			uiStateUpdates <- update
			// Ignore updates which do not modify the main state.
			if update.State.State == states[update.Id].State {
				continue
			}
			states[update.Id] = update.State
			// If an instance entered preview or finished generating and is *not*
			// the active instance, press F3+Escape.
			active, err := x.GetActiveWindow()
			if err != nil {
				// If we can't get the current focused window, just assume the
				// active instance is not focused.
				active = 0
			}
			unpause := conf.Reset.UnpauseFocus
			idle := update.State.State == StIdle
			preview := update.State.State == StPreview
			focused := active == instances[update.Id].Wid
			if preview || (idle && !focused) || (idle && !unpause) {
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
					lastTime[current] = key.Time
					lastTime[next] = key.Time
					if conf.Reset.UnpauseFocus && states[next].State == StIdle {
						x.SendKeyPress(
							x11.KeyEscape,
							instances[next].Wid,
							&lastTime[next],
						)
					}
					if conf.Reset.ClickFocus {
						time.Sleep(time.Millisecond * time.Duration(conf.Reset.Delay))
						x.Click(instances[next].Wid)
					}
					v14_reset(x, instances[current], &lastTime[current])
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
