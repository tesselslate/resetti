package ctl

import (
	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/obs"
	"github.com/woofdoggo/resetti/internal/x11"
)

// Wall implements a standard "Wall" interface, where the user can see all of
// their instances on an OBS projector and manage them from there.
type Wall struct {
	host *Controller
	conf *cfg.Profile
	o    *obs.Client

	states []mc.State
	active int
}

// Setup implements Frontend.
func (w *Wall) Setup(deps frontendDependencies) error {
	w.host = deps.host
	w.conf = deps.conf
	w.o = deps.obs

	w.active = 0
	w.states = make([]mc.State, len(deps.states))
	copy(w.states, deps.states)
	return nil
}

// Input implements Frontend.
func (w *Wall) Input(evt x11.Event) {

}

// Update implements Frontend.
func (w *Wall) Update(update mc.Update) {

}
