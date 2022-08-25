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

type wallState struct {
	conf        cfg.Profile
	x           *x11.Client
	obs         *go_obs.Client
	instances   []Instance
	states      []InstanceState
	lastTime    []xproto.Timestamp
	locks       []bool
	current     int
	onWall      bool
	lastMouseId int
	projector   xproto.Window
}

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
	obs, obsErr, err := connectObs(conf, len(instances))
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
	logUpdates, stopLogReaders, err := startLogReaders(instances)
	if err != nil {
		return err
	}
	defer stopLogReaders()

	// Grab global keys.
	x.GrabKey(conf.Keys.Focus, x.RootWindow())
	x.GrabKey(conf.Keys.Reset, x.RootWindow())
	defer x.UngrabKey(conf.Keys.Focus, x.RootWindow())
	defer x.UngrabKey(conf.Keys.Reset, x.RootWindow())

	// Turn off any lock indicators from the last time resetti was run.
	for i := 0; i < len(instances); i++ {
		setVisible(obs, "Wall", fmt.Sprintf("Lock %d", i+1), false)
	}

	// Prepare to start main loop.
	wall := wallState{
		conf:        conf,
		x:           x,
		obs:         obs,
		instances:   instances,
		states:      make([]InstanceState, len(instances)),
		lastTime:    make([]xproto.Timestamp, len(instances)),
		locks:       make([]bool, len(instances)),
		current:     0,
		onWall:      true,
		lastMouseId: -1,
		projector:   projector,
	}

	// Start UI.
	display := newResetDisplay(instances)
	uiStateUpdates, _, uiStopped, err := display.Init()
	if err != nil {
		return err
	}
	ctx, cancelUi := context.WithCancel(context.Background())
	display.Run(ctx, false) // TODO: Toggle affinity display
	defer display.Fini()
	defer cancelUi()

	// Start main loop.
	wallGrabKeys(&wall)
	x.FocusWindow(projector)
	for {
		select {
		case <-uiStopped:
			return nil
		case update := <-logUpdates:
			// If a log reader channel was closed, something went wrong.
			if update.Done {
				log.Println("ResetWall err: log reader closed")
				return nil
			}

			// If the instance finished generating or entered the preview
			// screen, pause it.
			prev := wall.states[update.Id]
			if prev.State != update.State.State {
				if update.State.State == StPreview || update.State.State == StIdle {
					x.SendKeyDown(x11.KeyF3, instances[update.Id].Wid, &wall.lastTime[update.Id])
					x.SendKeyPress(x11.KeyEscape, instances[update.Id].Wid, &wall.lastTime[update.Id])
					x.SendKeyUp(x11.KeyF3, instances[update.Id].Wid, &wall.lastTime[update.Id])
				}
			}

			// TODO: Update affinity.

			// Update state.
			wall.states[update.Id] = update.State
			uiStateUpdates <- update
		case evt := <-xEvt:
			switch evt := evt.(type) {
			case x11.KeyEvent:
				if evt.State == x11.KeyDown {
					switch evt.Key {
					case conf.Keys.Focus:
						if wall.onWall {
							x.FocusWindow(projector)
						} else {
							x.FocusWindow(instances[wall.current].Wid)
						}
					case conf.Keys.Reset:
						wallHandleReset(&wall, evt)
					default:
						if !wall.onWall {
							continue
						}
						id := int(evt.Key.Code - 10)
						if id < 0 || id > 8 || id > len(instances) {
							continue
						}
						wallHandleEvent(&wall, id, evt.Key.Mod, evt.Time)
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
					if wall.lastMouseId == id {
						continue
					}
					wall.lastMouseId = id
					wallHandleEvent(&wall, id, x11.Keymod(evt.State)^xproto.ButtonMask1, evt.Time)
				}
			case x11.ButtonEvent:
				x := uint16(evt.X) / instanceWidth
				y := uint16(evt.Y) / instanceHeight
				id := int((y * wallWidth) + x)
				if id >= len(instances) {
					continue
				}
				wall.lastMouseId = id
				wallHandleEvent(&wall, id, x11.Keymod(evt.State), evt.Time)
			}
		}
	}
}

func wallGrabKeys(w *wallState) error {
	win := w.x.RootWindow()
	for i := 0; i < len(w.instances); i++ {
		key := x11.Key{
			Code: xproto.Keycode(i + 10),
		}
		key.Mod = w.conf.Keys.WallPlay
		err := w.x.GrabKey(key, win)
		if err != nil {
			return err
		}
		key.Mod = w.conf.Keys.WallReset
		err = w.x.GrabKey(key, win)
		if err != nil {
			return err
		}
		key.Mod = w.conf.Keys.WallResetOthers
		err = w.x.GrabKey(key, win)
		if err != nil {
			return err
		}
		key.Mod = w.conf.Keys.WallLock
		err = w.x.GrabKey(key, win)
		if err != nil {
			return err
		}
	}
	if w.conf.Wall.UseMouse {
		err := w.x.GrabPointer(w.x.RootWindow())
		if err != nil {
			return err
		}
	}
	return nil
}

func wallUngrabKeys(w *wallState) error {
	win := w.x.RootWindow()
	for i := 0; i < len(w.instances); i++ {
		key := x11.Key{
			Code: xproto.Keycode(i + 10),
		}
		key.Mod = w.conf.Keys.WallPlay
		err := w.x.UngrabKey(key, win)
		if err != nil {
			return err
		}
		key.Mod = w.conf.Keys.WallReset
		err = w.x.UngrabKey(key, win)
		if err != nil {
			return err
		}
		key.Mod = w.conf.Keys.WallResetOthers
		err = w.x.UngrabKey(key, win)
		if err != nil {
			return err
		}
		key.Mod = w.conf.Keys.WallLock
		err = w.x.UngrabKey(key, win)
		if err != nil {
			return err
		}
	}
	if w.conf.Wall.UseMouse {
		err := w.x.UngrabPointer()
		if err != nil {
			return err
		}
	}
	return nil
}

func wallUpdateLastTime(w *wallState, id int, timestamp xproto.Timestamp) {
	if w.lastTime[id] < timestamp {
		w.lastTime[id] = timestamp
	}
}

func wallSetLock(w *wallState, id int, state bool) {
	if w.locks[id] == state {
		return
	}
	w.locks[id] = state
	err := setVisible(w.obs, "Wall", fmt.Sprintf("Lock %d", id+1), state)
	if err != nil {
		log.Printf("ResetWall: setLock err: %s", err)
	}
}

func wallGotoWall(w *wallState) {
	w.onWall = true
	go setScene(w.obs, "Wall")
	err := w.x.FocusWindow(w.projector)
	if err != nil {
		log.Printf("ResetWall err: goToWall: %s", err)
	}
	wallGrabKeys(w)
}

func wallPlay(w *wallState, id int, timestamp xproto.Timestamp) {
	if w.states[id].State != StIdle {
		return
	}
	go setScene(w.obs, fmt.Sprintf("Instance %d", id+1))
	err := wallUngrabKeys(w)
	if err != nil {
		log.Printf("ResetWall err: failed ungrab wall keys: %s", err)
	}
	w.x.FocusWindow(w.instances[id].Wid)
	wallUpdateLastTime(w, id, timestamp)
	if w.conf.Reset.UnpauseFocus {
		w.x.SendKeyPress(x11.KeyEscape, w.instances[id].Wid, &w.lastTime[id])
	}
	if w.conf.Reset.ClickFocus {
		time.Sleep(time.Millisecond * time.Duration(w.conf.Reset.Delay))
		w.x.Click(w.instances[id].Wid)
	}
	if w.conf.Wall.StretchWindows {
		err := w.x.MoveWindow(
			w.instances[id].Wid,
			0, 0,
			uint32(w.conf.Wall.UnstretchWidth),
			uint32(w.conf.Wall.UnstretchHeight),
		)
		if err != nil {
			log.Printf("ResetWall err: failed to unstretch window: %s", err)
		}
	}
	wallSetLock(w, id, false)
	w.onWall = false
	w.current = id
}

func wallReset(w *wallState, id int, timestamp xproto.Timestamp) {
	if w.locks[id] || w.states[id].State == StGenerating {
		return
	}
	wallUpdateLastTime(w, id, timestamp)
	v14_reset(w.x, w.instances[id], &w.lastTime[id])
	go runHook(w.conf.Hooks.WallReset)
}

func wallResetOthers(w *wallState, id int, timestamp xproto.Timestamp) {
	if w.states[id].State != StIdle {
		return
	}
	wallPlay(w, id, timestamp)
	for i := 0; i < len(w.instances); i++ {
		if i != id && !w.locks[i] && w.states[i].State != StGenerating {
			v14_reset(w.x, w.instances[i], &w.lastTime[i])
			go runHook(w.conf.Hooks.WallReset)
		}
	}
}

func wallLock(w *wallState, id int) {
	wallSetLock(w, id, !w.locks[id])
	if w.locks[id] {
		go runHook(w.conf.Hooks.Lock)
	} else {
		go runHook(w.conf.Hooks.Unlock)
	}
}

func wallHandleEvent(w *wallState, id int, state x11.Keymod, timestamp xproto.Timestamp) {
	switch state {
	case w.conf.Keys.WallPlay:
		wallPlay(w, id, timestamp)
	case w.conf.Keys.WallReset:
		wallReset(w, id, timestamp)
	case w.conf.Keys.WallResetOthers:
		wallResetOthers(w, id, timestamp)
	case w.conf.Keys.WallLock:
		wallLock(w, id)
	}
}

func wallHandleReset(w *wallState, evt x11.KeyEvent) {
	if w.onWall {
		wg := sync.WaitGroup{}
		for i, v := range w.instances {
			if w.locks[i] || w.states[i].State == StGenerating {
				continue
			}
			wg.Add(1)
			go func(inst Instance) {
				wallUpdateLastTime(w, inst.Id, evt.Time)
				v14_reset(w.x, inst, &w.lastTime[inst.Id])
				wg.Done()
				runHook(w.conf.Hooks.WallReset)
			}(v)
		}
		wg.Wait()
	} else {
		wallUpdateLastTime(w, w.current, evt.Time)
		v14_reset(w.x, w.instances[w.current], &w.lastTime[w.current])
		if w.conf.Wall.StretchWindows {
			err := w.x.MoveWindow(
				w.instances[w.current].Wid,
				0, 0,
				uint32(w.conf.Wall.StretchWidth),
				uint32(w.conf.Wall.StretchHeight),
			)
			if err != nil {
				log.Printf("ResetWall err: failed to unstretch window: %s", err)
			}
		}
		time.Sleep(time.Duration(w.conf.Reset.Delay) * time.Millisecond)
		if !w.conf.Wall.GoToLocked {
			wallGotoWall(w)
		} else {
			for idx, locked := range w.locks {
				if locked {
					if w.states[idx].State != StIdle {
						continue
					}
					wallPlay(w, idx, evt.Time)
					return
				}
			}
			wallGotoWall(w)
		}
	}
}
