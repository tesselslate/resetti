package reset

import (
	"fmt"
	"log"

	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/obs"
	"github.com/woofdoggo/resetti/internal/x11"
)

type FrontendMulti struct {
	conf *cfg.Profile
	host *Controller
	obs  *obs.Client
	x    *x11.Client

	active    int
	instances []mc.Instance
	states    []mc.InstanceState
}

func (f *FrontendMulti) HandleInput(event x11.Event) error {
	evt, ok := event.(x11.KeyEvent)
	if !ok {
		return nil
	}
	if evt.State != x11.StateDown {
		return nil
	}
	switch evt.Key {
	case f.conf.Keys.Focus:
		f.instances[f.active].Focus()
	case f.conf.Keys.Reset:
		next := (f.active + 1) % len(f.instances)
		f.instances[next].FocusAndUnpause(f.x.GetCurrentTime(), f.states[next].State == mc.StIdle)
		f.host.ResetInstance(f.active, f.x.GetCurrentTime())
		if f.obs != nil {
			err := f.obs.SetScene(fmt.Sprintf("Instance %d", next+1))
			if err != nil {
				log.Printf("Failed to set scene: %s\n", err)
			}
		}
		go runHook(f.conf.Hooks.Reset)
		f.active = next
	}
	return nil
}

func (f *FrontendMulti) HandleUpdate(update mc.Update) error {
	f.states[update.Id] = update.State
	return nil
}

func (f *FrontendMulti) Setup(opts FrontendOptions) error {
	f.conf = opts.Conf
	f.host = opts.Controller
	f.obs = opts.Obs
	f.x = opts.X
	f.instances = make([]mc.Instance, len(opts.Instances))
	f.states = make([]mc.InstanceState, len(opts.Instances))
	copy(f.instances, opts.Instances)
	copy(f.states, opts.States)
	err := opts.X.GrabKey(f.conf.Keys.Reset, opts.X.GetRootWindow())
	if err != nil {
		return err
	}
	err = opts.X.GrabKey(f.conf.Keys.Focus, opts.X.GetRootWindow())
	if err != nil {
		return err
	}
	f.instances[0].Focus()
	return nil
}

func (f *FrontendMulti) ShouldPause(id int) bool {
	return id != f.active
}
