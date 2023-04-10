package ctl

import (
	"fmt"

	"github.com/jezek/xgb/xproto"
	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/obs"
)

// Multi implements a traditional Multi-instance interface, where the user
// plays and resets one instance at a time.
type Multi struct {
	host *Controller
	conf *cfg.Profile
	obs  *obs.Client

	states []mc.State
	active int
}

// Setup implements Frontend.
func (m *Multi) Setup(deps frontendDependencies) error {
	m.host = deps.host
	m.conf = deps.conf
	m.obs = deps.obs

	m.active = 0
	m.states = make([]mc.State, len(deps.states))
	copy(m.states, deps.states)
	return nil
}

// FocusChange implements Frontend.
func (m *Multi) FocusChange(win xproto.Window) {
	// Do nothing.
}

// Input implements Frontend.
func (m *Multi) Input(input Input) {
	actions := m.conf.Keybinds[input.Bind]
	for _, action := range actions.IngameActions {
		switch action.Type {
		case cfg.ActionIngameFocus:
			m.host.FocusInstance(m.active)
		case cfg.ActionIngameReset:
			// TODO: Implement moving wall style best instance picker
			// TODO: Handle reset failure
			next := (m.active + 1) % len(m.states)
			current := m.active
			_ = m.host.ResetInstance(current)
			m.host.PlayInstance(next)
			m.active = next
			m.updateObs()
			m.host.RunHook(HookReset)
		}
	}
}

// Update implements Frontend.
func (m *Multi) Update(update mc.Update) {
	m.states[update.Id] = update.State
}

// updateObs changes which instance is visible on the OBS scene.
func (m *Multi) updateObs() {
	if !m.conf.Obs.Enabled {
		return
	}
	m.obs.BatchAsync(obs.SerialRealtime, func(b *obs.Batch) {
		for i := 1; i <= len(m.states); i += 1 {
			b.SetItemVisibility("Instance", fmt.Sprintf("MC %d", i), i-1 == m.active)
		}
	})
}
