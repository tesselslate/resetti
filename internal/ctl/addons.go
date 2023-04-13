package ctl

import (
	"fmt"
	"log"

	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/obs"
)

// hider processes instance state updates and hides and shows instances as they
// change states.
type hider struct {
	conf   *cfg.Profile
	obs    *obs.Client
	states []mc.State
}

// newHider creates a new hider with the given config.
func newHider(conf *cfg.Profile, obs *obs.Client, states []mc.State) *hider {
	newStates := make([]mc.State, len(states))
	copy(newStates, states)
	h := hider{
		conf,
		obs,
		newStates,
	}
	h.showAll()
	return &h
}

// Reset signals that the given instance has been reset.
func (n *hider) Reset(id int) {
	n.obs.SetSceneItemVisibleAsync("Wall", fmt.Sprintf("Wall MC %d", id+1), false)
}

// Update updates the state of an instance and returns whether or not it is now
// shown.
func (h *hider) Update(update mc.Update) (show bool) {
	prev := h.states[update.Id]
	next := update.State
	threshold := h.conf.Wall.ShowAt

	nowPreview := prev.Type != mc.StPreview && next.Type == mc.StPreview
	wasUnder := prev.Progress < threshold
	nowOver := next.Progress >= threshold
	if (next.Type == mc.StPreview && wasUnder && nowOver) || (nowPreview && nowOver) {
		h.obs.SetSceneItemVisibleAsync("Wall", fmt.Sprintf("Wall MC %d", update.Id+1), true)
        show = true
	}

	h.states[update.Id] = update.State
    return show
}

// showAll shows all instances.
func (h *hider) showAll() {
	err := h.obs.Batch(obs.SerialFrame, func(b *obs.Batch) {
		for i := 1; i <= len(h.states); i += 1 {
			b.SetItemVisibility("Wall", fmt.Sprintf("Wall MC %d", i), true)
		}
	})
	if err != nil {
		log.Printf("notifier.showAll: Batch failed: %s\n", err)
	}
}
