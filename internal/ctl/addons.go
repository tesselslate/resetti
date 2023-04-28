package ctl

import (
	"fmt"
	"log"

	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/obs"
)

// freezer freezes and unfreezes instances as their states change.
type freezer struct {
	conf      *cfg.Profile
	obs       *obs.Client
	canFreeze []bool
	states    []mc.State
}

// hider processes instance state updates and hides and shows instances as they
// change states.
type hider struct {
	conf   *cfg.Profile
	obs    *obs.Client
	states []mc.State
}

// newFreezer creates a new freezer with the given config.
func newFreezer(conf *cfg.Profile, obs *obs.Client, states []mc.State) *freezer {
	canFreeze := make([]bool, len(states))
	newStates := make([]mc.State, len(states))
	copy(newStates, states)
	f := freezer{
		conf,
		obs,
		canFreeze,
		newStates,
	}
	f.unfreezeAll()
	return &f
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

// SetCanFreeze sets whether or not the given instance can be frozen.
func (f *freezer) SetCanFreeze(id int, canFreeze bool) {
	f.canFreeze[id] = canFreeze
	if !canFreeze {
		f.setFrozen(id, false)
	} else {
		f.Update(mc.Update{State: f.states[id], Id: id})
	}
}

// Unfreeze unfreezes the given instance and marks it as freezable.
func (f *freezer) Unfreeze(id int) {
	f.canFreeze[id] = true
	f.setFrozen(id, false)
}

// Update updates the state of an instance.
func (f *freezer) Update(update mc.Update) {
	prev := f.states[update.Id]
	next := update.State
	threshold := f.conf.Wall.FreezeAt

	nowPreview := prev.Type != mc.StPreview && next.Type == mc.StPreview
	wasUnder := prev.Progress < threshold
	nowOver := next.Progress >= threshold
	if (next.Type == mc.StPreview && wasUnder && nowOver) || (nowPreview && nowOver) {
		f.setFrozen(update.Id, true)
	}
	if next.Type == mc.StDirt {
		f.setFrozen(update.Id, false)
	}

	f.states[update.Id] = update.State
}

// setFrozen freezes or unfreezes the given instance.
func (f *freezer) setFrozen(id int, frozen bool) {
	if frozen && !f.canFreeze[id] {
		return
	}
	f.obs.SetSourceFilterEnabled(
		fmt.Sprintf("Wall MC %d", id+1),
		fmt.Sprintf("Freeze %d", id+1),
		frozen,
	)
}

// unfreezeAll unfreezes all instances.
func (f *freezer) unfreezeAll() {
	err := f.obs.Batch(obs.SerialFrame, func(b *obs.Batch) {
		for i := 1; i <= len(f.states); i += 1 {
			b.SetSourceFilterEnabled(
				fmt.Sprintf("Wall MC %d", i),
				fmt.Sprintf("Freeze %d", i),
				false,
			)
		}
	})
	if err != nil {
		log.Printf("freezer.unfreezeAll: Batch failed: %s\n", err)
	}
}

// ShouldShow processes a single state update and determines whether or not the
// instance should now be shown.
func (h *hider) ShouldShow(update mc.Update) bool {
	prev := h.states[update.Id]
	next := update.State
	threshold := h.conf.Wall.ShowAt

	nowPreview := prev.Type != mc.StPreview && next.Type == mc.StPreview
	wasUnder := prev.Progress < threshold
	nowOver := next.Progress >= threshold
	h.states[update.Id] = update.State
	return (next.Type == mc.StPreview && wasUnder && nowOver) || (nowPreview && nowOver)
}

// showAll shows all instances.
func (h *hider) showAll() {
	err := h.obs.Batch(obs.SerialFrame, func(b *obs.Batch) {
		for i := 1; i <= len(h.states); i += 1 {
			b.SetItemVisibility("Wall", fmt.Sprintf("Wall MC %d", i), true)
		}
	})
	if err != nil {
		log.Printf("hider.showAll: Batch failed: %s\n", err)
	}
}
