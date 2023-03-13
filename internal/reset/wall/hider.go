package wall

import (
	"fmt"
	"time"

	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/obs"
)

// hider hides instances while they are on the dirt screen.
type hider struct {
	conf   *cfg.Profile
	obs    *obs.Client
	states []mc.InstanceState
	shown  []bool
}

func (h *hider) Setup(conf *cfg.Profile, o *obs.Client, states []mc.InstanceState) error {
	h.conf = conf
	h.obs = o
	h.states = make([]mc.InstanceState, len(states))
	h.shown = make([]bool, len(states))
	for i := range h.shown {
		h.shown[i] = true
	}
	copy(h.states, states)
	return nil
}

func (h *hider) Update(update mc.Update) {
	nowDirt := h.states[update.Id].State != mc.StDirt && update.State.State == mc.StDirt
	h.states[update.Id] = update.State
	if h.shown[update.Id] && nowDirt {
		h.obs.SetSceneItemVisibleAsync("Wall", fmt.Sprintf("Wall MC %d", update.Id+1), false)
		h.shown[update.Id] = false
	} else if !h.shown[update.Id] && update.State.State == mc.StPreview {
		h.shown[update.Id] = true
		go func() {
			<-time.After(time.Millisecond * time.Duration(h.conf.AdvancedWall.ShowDelay))
			h.obs.SetSceneItemVisibleAsync("Wall", fmt.Sprintf("Wall MC %d", update.Id+1), true)
		}()
	}
}
