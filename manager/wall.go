package manager

import (
	"errors"
	"fmt"
	"resetti/cfg"
	"resetti/mc"
	"resetti/x11"
	"strings"
	"time"

	"github.com/jezek/xgb/xproto"
	obs "github.com/woofdoggo/go-obs"
)

// WallManager provides a Manager implementation for
// wall-style resetting.
type WallManager struct {
	managerState

	active bool
	stop   chan bool

	current int
	onWall  bool
}

func (w *WallManager) Setup(x *x11.Client, o *obs.Client, c cfg.Config) {
	w.x = x
	w.o = o
	w.conf = c
}

func (w *WallManager) Start(instances []mc.Instance, stateCh chan mc.Instance) error {
	if w.active {
		return fmt.Errorf("manager already running")
	}

	// setup channels
	w.stop = make(chan bool, 1)
	w.wErrCh = make(chan WorkerError, 32)
	w.wCmdCh = make([]chan WorkerCommand, len(instances))
	for i := 0; i < len(instances); i++ {
		w.wCmdCh[i] = make(chan WorkerCommand, 8)
	}

	w.mErrCh = make(chan error)
	w.mStateCh = stateCh

	// setup workers
	w.workers = make([]*Worker, len(instances))
	for i := 0; i < len(instances); i++ {
		worker, err := NewWorker(w, instances[i])
		if err != nil {
			return err
		}

		err = worker.Run(w.wCmdCh[i], w.wErrCh, w.mStateCh)
		if err != nil {
			return err
		}

		w.workers[i] = worker
	}

	w.active = true
	go w.run()

	return nil
}

func (w *WallManager) Stop() error {
	if !w.active {
		return fmt.Errorf("manager already stopped")
	}

	w.stop <- true
	return nil
}

func (w *WallManager) GetConfig() cfg.ResetSettings {
	return w.conf.Reset
}

func (w *WallManager) GetX() *x11.Client {
	return w.x
}

func (w *WallManager) cleanup() {
	for i := 0; i < len(w.workers); i++ {
		w.workers[i].Stop()
	}
	w.x.UngrabKey(w.conf.Keys.Focus)
	w.x.UngrabKey(w.conf.Keys.Reset)
	w.unbindKeys()
	w.x.LoopStop()
}

func (w *WallManager) bindKeys() {
	for i := 0; i < len(w.workers); i++ {
		w.x.GrabKey(x11.Key{
			Code: xproto.Keycode(10 + i),
			Mod:  x11.ModNone,
		})
		w.x.GrabKey(x11.Key{
			Code: xproto.Keycode(10 + i),
			Mod:  x11.ModShift,
		})
	}
}

func (w *WallManager) unbindKeys() {
	for i := 0; i < len(w.workers); i++ {
		w.x.UngrabKey(x11.Key{
			Code: xproto.Keycode(10 + i),
			Mod:  x11.ModNone,
		})
		w.x.UngrabKey(x11.Key{
			Code: xproto.Keycode(10 + i),
			Mod:  x11.ModShift,
		})
	}
}

func (w *WallManager) run() {
	defer w.cleanup()
	w.x.GrabKey(w.conf.Keys.Focus)
	w.x.GrabKey(w.conf.Keys.Reset)
	xerr, xevt := w.x.Loop()
	w.bindKeys()

	for i := 0; i < len(w.workers); i++ {
		win := w.workers[i].instance.Window
		w.x.SetTitle(win, fmt.Sprintf("Minecraft | Instance %d", i+1))
	}

	w.onWall = true
	windows, err := w.x.GetWindowList(w.x.Root)
	if err != nil {
		w.mErrCh <- err
		return
	}

	var obsWindow xproto.Window
	for _, v := range windows {
		title, err := w.x.GetWindowTitle(v)
		if err != nil {
			continue
		}
		if strings.Contains(title, "Projector (Scene)") {
			obsWindow = v
			break
		}
	}

	if obsWindow == 0 {
		w.mErrCh <- errors.New("could not locate OBS projector")
		return
	}

	w.x.FocusWindow(obsWindow)
	for {
		select {
		case err := <-w.wErrCh:
			// Handle worker error.
			if err.Fatal {
				// If worker error is fatal, try to reboot the worker.
				time.Sleep(100 * time.Millisecond)
				err := w.workers[err.Id].Run(w.wCmdCh[err.Id], w.wErrCh, w.mStateCh)
				if err != nil {
					w.mErrCh <- err
					return
				}
			}
		case err := <-xerr:
			if err.Error() == "connection died" {
				w.mErrCh <- errors.New("x connection died")
			}
		case evt := <-xevt:
			if evt.State == x11.KeyDown {
				switch evt.Key {
				case w.conf.Keys.Focus:
					if w.onWall {
						w.x.FocusWindow(obsWindow)
					} else {
						w.wCmdCh[w.current] <- WorkerCommand{
							Op:   CmdFocus,
							Time: evt.Timestamp,
						}
						for i := 0; i < len(w.workers); i++ {
							w.unbindKeys()
						}
					}
				case w.conf.Keys.Reset:
					if !w.onWall {
						go obs.NewSetCurrentSceneRequest(w.o, "Wall")
						w.x.FocusWindow(obsWindow)
						w.wCmdCh[w.current] <- WorkerCommand{
							Op:   CmdReset,
							Time: evt.Timestamp,
						}
						for i := 0; i < len(w.workers); i++ {
							w.bindKeys()
						}
						w.onWall = true
					} else {
						for i := 0; i < len(w.workers); i++ {
							w.wCmdCh[i] <- WorkerCommand{
								Op:   CmdReset,
								Time: evt.Timestamp,
							}
						}
					}
				default:
					num := int(evt.Key.Code) - 10
					if evt.Key.Mod != x11.ModNone {
						for i := 0; i < len(w.workers); i++ {
							w.unbindKeys()
						}
						w.current = num
						w.wCmdCh[w.current] <- WorkerCommand{
							Op:   CmdFocus,
							Time: evt.Timestamp,
						}
						if w.workers[w.current].GetState() == mc.StatePaused {
							w.x.SendKeyPress(
								x11.KeyEscape,
								w.workers[w.current].instance.Window,
								&evt.Timestamp,
							)
						}
						w.workers[w.current].SetState(mc.StateIngame)
						go obs.NewSetCurrentSceneRequest(w.o, fmt.Sprintf("Instance %d", w.current+1))
						w.onWall = false
					} else {
						w.wCmdCh[num] <- WorkerCommand{
							Op:   CmdReset,
							Time: evt.Timestamp,
						}
					}
				}
			}
		case <-w.stop:
			// Stop.
			w.active = false
			return
		}
	}
}
