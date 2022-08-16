package reset

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/jezek/xgb/xproto"
	go_obs "github.com/woofdoggo/go-obs"
	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/x11"
)

func ResetWall(conf cfg.Profile, instances []Instance) error {
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
	obs := &go_obs.Client{}
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

	// Set OBS sources.
	err = setSources(obs, instances)
	if err != nil {
		return err
	}

	// Find OBS projector.
	projector, err := findProjector(x)
	if err != nil {
		return err
	}

	// Get wall and screen size.
	wallWidth, wallHeight, err := getWallSize(obs, len(instances))
	if err != nil {
		return err
	}
	screenWidth, screenHeight, err := x.GetScreenSize()
	if err != nil {
		return err
	}
	instanceWidth, instanceHeight := screenWidth/wallWidth, screenHeight/wallHeight

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

	// Grab global keys.
	x.GrabKey(conf.Keys.Focus, x.RootWindow())
	x.GrabKey(conf.Keys.Reset, x.RootWindow())
	defer x.UngrabKey(conf.Keys.Focus, x.RootWindow())
	defer x.UngrabKey(conf.Keys.Reset, x.RootWindow())

	// Prepare to start main loop.
	current := 0
	onWall := true
	states := make([]InstanceState, len(instances))
	lastTime := make([]xproto.Timestamp, len(instances))
	locks := make([]bool, len(instances))
	lastMouseId := -1

	grabWallKeys := func() error {
		win := x.RootWindow()
		for i := 0; i < len(instances); i++ {
			key := x11.Key{
				Code: xproto.Keycode(i + 10),
			}
			key.Mod = conf.Keys.WallPlay
			err := x.GrabKey(key, win)
			if err != nil {
				return err
			}
			key.Mod = conf.Keys.WallReset
			err = x.GrabKey(key, win)
			if err != nil {
				return err
			}
			key.Mod = conf.Keys.WallResetOthers
			err = x.GrabKey(key, win)
			if err != nil {
				return err
			}
			key.Mod = conf.Keys.WallLock
			err = x.GrabKey(key, win)
			if err != nil {
				return err
			}
		}
		if conf.Wall.UseMouse {
			err := x.GrabPointer(x.RootWindow())
			if err != nil {
				return err
			}
		}
		return nil
	}
	ungrabWallKeys := func() error {
		win := x.RootWindow()
		for i := 0; i < len(instances); i++ {
			key := x11.Key{
				Code: xproto.Keycode(i + 10),
			}
			key.Mod = conf.Keys.WallPlay
			err := x.UngrabKey(key, win)
			if err != nil {
				return err
			}
			key.Mod = conf.Keys.WallReset
			err = x.UngrabKey(key, win)
			if err != nil {
				return err
			}
			key.Mod = conf.Keys.WallResetOthers
			err = x.UngrabKey(key, win)
			if err != nil {
				return err
			}
			key.Mod = conf.Keys.WallLock
			err = x.UngrabKey(key, win)
			if err != nil {
				return err
			}
		}
		if conf.Wall.UseMouse {
			err := x.UngrabPointer()
			if err != nil {
				return err
			}
		}
		return nil
	}
	updateLastTime := func(id int, time xproto.Timestamp) {
		if lastTime[id] < time {
			lastTime[id] = time
		}
	}
	setLock := func(id int, state bool) {
		if locks[id] == state {
			return
		}
		locks[id] = state
		err := setVisible(obs, "Wall", fmt.Sprintf("Lock %d", id+1), state)
		if err != nil {
			log.Printf("ResetWall: setLock err: %s", err)
		}
	}
	goToWall := func() {
		onWall = true
		go setScene(obs, "Wall")
		err := x.FocusWindow(projector)
		if err != nil {
			log.Printf("ResetWall err: goToWall: %s", err)
		}
		grabWallKeys()
	}
	wallPlay := func(id int, time xproto.Timestamp) {
		if states[id].State != StIdle {
			return
		}
		go setScene(obs, fmt.Sprintf("Instance %d", id+1))
		err := ungrabWallKeys()
		if err != nil {
			log.Printf("ResetWall err: failed ungrab wall keys: %s", err)
		}
		x.FocusWindow(instances[id].Wid)
		updateLastTime(id, time)
		if conf.Reset.UnpauseFocus {
			x.SendKeyPress(x11.KeyEscape, instances[id].Wid, &lastTime[id])
		}
		if conf.Wall.StretchWindows {
			err := x.MoveWindow(
				instances[id].Wid,
				0, 0,
				uint32(conf.Wall.UnstretchWidth),
				uint32(conf.Wall.UnstretchHeight),
			)
			if err != nil {
				log.Printf("ResetWall err: failed to unstretch window: %s", err)
			}
		}
		setLock(id, false)
		onWall = false
		current = id
	}
	wallReset := func(id int, time xproto.Timestamp) {
		if locks[id] || states[id].State == StGenerating {
			return
		}
		updateLastTime(id, time)
		v14_reset(x, instances[id], &lastTime[id])
		if conf.Hooks.WallReset != "" {
			go runCmd(conf.Hooks.WallReset)
		}
	}
	wallResetOthers := func(id int, time xproto.Timestamp) {
		if states[id].State != StIdle {
			return
		}
		wallPlay(id, time)
		for i := 0; i < len(instances); i++ {
			if i != id && !locks[i] && states[i].State != StGenerating {
				v14_reset(x, instances[i], &lastTime[i])
				if conf.Hooks.WallReset != "" {
					go runCmd(conf.Hooks.WallReset)
				}
			}
		}
	}
	wallLock := func(id int) {
		setLock(id, !locks[id])
		log.Printf("ResetWall: lock %d %t", id, locks[id])
		if locks[id] && conf.Hooks.Lock != "" {
			go runCmd(conf.Hooks.Lock)
		} else if !locks[id] && conf.Hooks.Unlock != "" {
			go runCmd(conf.Hooks.Unlock)
		}
	}
	handleEvent := func(id int, state x11.Keymod, time xproto.Timestamp) {
		switch state {
		case conf.Keys.WallPlay:
			wallPlay(id, time)
		case conf.Keys.WallReset:
			wallReset(id, time)
		case conf.Keys.WallResetOthers:
			wallResetOthers(id, time)
		case conf.Keys.WallLock:
			wallLock(id)
		}
	}
	handleReset := func(evt x11.KeyEvent) {
		if onWall {
			wg := sync.WaitGroup{}
			for i, v := range instances {
				if locks[i] || states[i].State == StGenerating {
					continue
				}
				wg.Add(1)
				go func(inst Instance) {
					updateLastTime(inst.Id, evt.Time)
					v14_reset(x, inst, &lastTime[inst.Id])
					wg.Done()
					if conf.Hooks.WallReset != "" {
						runCmd(conf.Hooks.WallReset)
					}
				}(v)
			}
			wg.Wait()
		} else {
			updateLastTime(current, evt.Time)
			v14_reset(x, instances[current], &lastTime[current])
			if conf.Wall.StretchWindows {
				err := x.MoveWindow(
					instances[current].Wid,
					0, 0,
					uint32(conf.Wall.StretchWidth),
					uint32(conf.Wall.StretchHeight),
				)
				if err != nil {
					log.Printf("ResetWall err: failed to unstretch window: %s", err)
				}
			}
			time.Sleep(time.Duration(conf.Reset.Delay) * time.Millisecond)
			if !conf.Wall.GoToLocked {
				goToWall()
			} else {
				for idx, locked := range locks {
					if locked {
						if states[idx].State != StIdle {
							continue
						}
						wallPlay(idx, evt.Time)
						return
					}
				}
				goToWall()
			}
		}
	}

	// Start main loop.
	grabWallKeys()
	x.FocusWindow(projector)
	for {
		select {
		case update := <-logUpdates:
			// If a log reader channel was closed, something went wrong.
			if update.Done {
				log.Println("ResetWall err: log reader closed")
				return nil
			}

			// If the instance finished generating or entered the preview
			// screen, pause it.
			prev := states[update.Id]
			if prev.State != update.State.State {
				if update.State.State == StPreview || update.State.State == StIdle {
					x.SendKeyDown(x11.KeyF3, instances[update.Id].Wid, &lastTime[update.Id])
					x.SendKeyPress(x11.KeyEscape, instances[update.Id].Wid, &lastTime[update.Id])
					x.SendKeyUp(x11.KeyF3, instances[update.Id].Wid, &lastTime[update.Id])
				}
			}

			// TODO: Update affinity.

			// Update state.
			states[update.Id] = update.State
		case evt := <-xEvt:
			switch evt := evt.(type) {
			case x11.KeyEvent:
				if evt.State == x11.KeyDown {
					switch evt.Key {
					case conf.Keys.Focus:
						if onWall {
							x.FocusWindow(projector)
						} else {
							x.FocusWindow(instances[current].Wid)
						}
					case conf.Keys.Reset:
						handleReset(evt)
					default:
						if !onWall {
							continue
						}
						id := int(evt.Key.Code - 10)
						if id < 0 || id > 8 || id > len(instances) {
							continue
						}
						handleEvent(id, evt.Key.Mod, evt.Time)
					}
				}
			case x11.MoveEvent:
				if evt.State&xproto.ButtonMask1 != 0 {
					x := uint16(evt.X) / instanceWidth
					y := uint16(evt.Y) / instanceHeight
					id := int((y * wallWidth) + x)
					if id >= len(instances) {
						continue
					}
					if lastMouseId == id {
						continue
					}
					lastMouseId = id
					handleEvent(id, x11.Keymod(evt.State)^xproto.ButtonMask1, evt.Time)
				}
			case x11.ButtonEvent:
				x := uint16(evt.X) / instanceWidth
				y := uint16(evt.Y) / instanceHeight
				id := int((y * wallWidth) + x)
				if id >= len(instances) {
					continue
				}
				lastMouseId = id
				handleEvent(id, x11.Keymod(evt.State), evt.Time)
			}
		}
	}
}
