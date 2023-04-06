package ctl

import (
	"fmt"

	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/obs"
	"github.com/woofdoggo/resetti/internal/x11"
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

	if err := m.host.Bind(BindFocus, m.conf.Keybinds.Focus); err != nil {
		return fmt.Errorf("bind focus: %w", err)
	}
	if err := m.host.Bind(BindReset, m.conf.Keybinds.Reset); err != nil {
		return fmt.Errorf("bind reset: %w", err)
	}

	m.active = 0
	m.states = make([]mc.State, len(deps.states))
	copy(m.states, deps.states)
	return nil
}

// Input implements Frontend.
func (m *Multi) Input(raw x11.Event) {
	// Verify that this is a key down event.
	evt, ok := raw.(x11.KeyEvent)
	if !ok || evt.State != x11.StateDown {
		return
	}
	input, ok := m.host.GetBindFor(raw)
	if !ok {
		return
	}

	// Process the input.
	switch input {
	case BindFocus:
		m.host.FocusInstance(m.active)
	case BindReset:
		// TODO: Implement moving-wall style best instance picker
		// TODO: Handle reset failure
		current := m.active
		next := (m.active + 1) % len(m.states)
		_ = m.host.ResetInstance(current)
		m.host.PlayInstance(next)
		m.active = next
		m.updateObs()
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
	m.obs.BatchAsync(obs.SerialRealtime, func(b *obs.Batch) error {
		for i := 1; i <= len(m.states); i += 1 {
			b.SetItemVisibility("Instance", fmt.Sprintf("MC %d", i), i-1 == m.active)
		}
		return nil
	})
}
