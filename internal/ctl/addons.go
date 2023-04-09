package ctl

import (
	"fmt"
	"log"
	"time"

	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/obs"
)

// hider hides instances while they are on the dirt screen.
type hider struct {
	conf *cfg.Profile
	obs  *obs.Client

	updates chan mc.Update
	show    chan int
	states  []mc.State
}

// newHider creates a new instance hider with the given config.
func newHider(conf *cfg.Profile, obs *obs.Client, states []mc.State) hider {
	newStates := make([]mc.State, len(states))
	copy(newStates, states)
	h := hider{
		conf,
		obs,
		make(chan mc.Update, bufferSize*len(states)),
		make(chan int, len(states)),
		newStates,
	}
	h.showAll()
	return h
}

// Run hides and unhides instances in the background.
func (h *hider) Run() {
	for {
		select {
		case update := <-h.updates:
			prev := h.states[update.Id]
			next := update.State
			h.states[update.Id] = next
			showAt := h.conf.Wall.Hiding.ShowAt
			switch h.conf.Wall.Hiding.ShowMethod {
			case "delay":
				if prev.Type != mc.StPreview && next.Type == mc.StPreview {
					go func() {
						<-time.After(time.Millisecond * time.Duration(showAt))
						h.show <- update.Id
					}()
				}
			case "percentage":
				wasUnder := prev.Progress <= showAt
				nowOver := next.Progress > showAt
				wasDirt := prev.Type == mc.StDirt
				nowPreview := next.Type == mc.StPreview
				if (nowPreview && wasUnder && nowOver) || (wasDirt && nowPreview && nowOver) {
					h.obs.SetSceneItemVisibleAsync("Wall", fmt.Sprintf("Wall MC %d", update.Id+1), true)
				}
			}
			if prev.Type != mc.StDirt && next.Type == mc.StDirt {
				h.obs.SetSceneItemVisibleAsync("Wall", fmt.Sprintf("Wall MC %d", update.Id+1), false)
			}
		case id := <-h.show:
			if h.states[id].Type != mc.StDirt {
				h.obs.SetSceneItemVisibleAsync("Wall", fmt.Sprintf("Wall MC %d", id+1), true)
			}
		}
	}
}

// Update updates the state of an instance in the hider.
func (h *hider) Update(update mc.Update) {
	h.updates <- update
}

// showAll shows all instances.
func (h *hider) showAll() {
	err := h.obs.Batch(obs.SerialRealtime, func(b *obs.Batch) {
		for i := 1; i <= len(h.states); i += 1 {
			b.SetItemVisibility("Wall", fmt.Sprintf("Wall MC %d", i), true)
		}
	})
	if err != nil {
		log.Printf("hider.showAll: Batch failed: %s\n", err)
	}
}
