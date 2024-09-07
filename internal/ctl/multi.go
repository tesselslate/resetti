package ctl

import (
	"fmt"

	"github.com/jezek/xgb/xproto"
	"github.com/tesselslate/resetti/internal/cfg"
	"github.com/tesselslate/resetti/internal/mc"
	"github.com/tesselslate/resetti/internal/obs"
	"github.com/tesselslate/resetti/internal/x11"
)

// Multi implements a traditional Multi-instance interface, where the user
// plays and resets one instance at a time.
type Multi struct {
	host *Controller
	conf *cfg.Profile
	obs  *obs.Client
	x    *x11.Client

	instances []mc.InstanceInfo
	states    []mc.State
	active    int
}

// Setup implements Frontend.
func (m *Multi) Setup(deps frontendDependencies) error {
	m.host = deps.host
	m.conf = deps.conf
	m.obs = deps.obs
	m.x = deps.x

	m.active = 0
	m.instances = make([]mc.InstanceInfo, len(deps.instances))
	m.states = make([]mc.State, len(deps.states))
	copy(m.instances, deps.instances)
	copy(m.states, deps.states)

	m.host.FocusInstance(0)
	return nil
}

// Input implements Frontend.
func (m *Multi) Input(input Input) {
	actions := m.conf.Keybinds[input.Bind]
	if input.Held {
		return
	}
	if m.active != -1 {
		for _, action := range actions.IngameActions {
			switch action.Type {
			case cfg.ActionIngameFocus:
				if m.conf.UtilityMode {
					continue
				}
				m.host.FocusInstance(m.active)
			case cfg.ActionIngameReset:
				if m.conf.UtilityMode || m.x.GetActiveWindow() != m.instances[m.active].Wid {
					continue
				}
				next := (m.active + 1) % len(m.states)
				current := m.active
				if m.host.ResetInstance(current) {
					m.host.PlayInstance(next)
					m.active = next
					m.updateObs()
					m.host.RunHook(HookReset, 0)
				}
			case cfg.ActionIngameRes:
				if m.x.GetActiveWindow() != m.instances[m.active].Wid {
					continue
				}
				if action.Extra != nil {
					resId := *action.Extra
					if resId < 0 || resId > len(m.conf.AltRes)-1 {
						continue
					}
					m.host.ToggleResolution(m.active, resId)
				} else {
					m.host.ToggleResolution(m.active, 0)
				}
			}
		}
	}
}

// ProcessEvent implements Frontend.
func (m *Multi) ProcessEvent(evt x11.Event) {
	switch evnt := evt.(type) {
	case x11.FocusEvent:
		if m.active != -1 && m.instances[m.active].Wid == xproto.Window(evnt) {
			m.host.RunHook(HookFocusGained, 0)
		} else {
			m.host.RunHook(HookFocusLost, 0)
		}
	}
}

// Update implements Frontend.
func (m *Multi) Update(update mc.Update) {
	if m.conf.UtilityMode {
		return
	}
	m.states[update.Id] = update.State
}

// updateObs changes which instance is visible on the OBS scene.
func (m *Multi) updateObs() {
	if m.conf.UtilityMode || !m.conf.Obs.Enabled {
		return
	}
	m.obs.BatchAsync(obs.SerialRealtime, func(b *obs.Batch) {
		for i := 1; i <= len(m.states); i += 1 {
			b.SetItemVisibility("Instance", fmt.Sprintf("MC %d", i), i-1 == m.active)
		}
	})
}
