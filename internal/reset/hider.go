package reset

import (
	"fmt"
	"time"

	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/obs"
)

type hider struct {
	conf   *cfg.Profile
	obs    *obs.Client
	states []mc.InstanceState
	show   chan int
}

func NewHider(conf *cfg.Profile, obs *obs.Client, states []mc.InstanceState) *hider {
	h := hider{
		conf,
		obs,
		make([]mc.InstanceState, len(states)),
		make(chan int, len(states)),
	}
	copy(h.states, states)
	go h.run()
	return &h
}

func (h *hider) Hide(id int) {
	h.obs.SetSceneItemVisibleAsync("Wall", fmt.Sprintf("Wall MC %d", id+1), false)
}

func (h *hider) Update(update mc.Update) {
	nowPreview := h.states[update.Id].State != mc.StPreview && update.State.State == mc.StPreview
	h.states[update.Id] = update.State
	if nowPreview {
		go func() {
			<-time.After(time.Duration(h.conf.Wall.ShowDelay) * time.Millisecond)
			h.show <- update.Id
		}()
	}
}

func (h *hider) run() {
	for id := range h.show {
		h.obs.SetSceneItemVisibleAsync("Wall", fmt.Sprintf("Wall MC %d", id+1), true)
	}
}
