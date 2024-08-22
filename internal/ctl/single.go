package ctl

import (
	"github.com/jezek/xgb/xproto"
	"github.com/tesselslate/resetti/internal/cfg"
	"github.com/tesselslate/resetti/internal/mc"
	"github.com/tesselslate/resetti/internal/x11"
)

// Single implements a traditional Single-instance interface with extra support
// for resolution binds and hooks.
type Single struct {
	host *Controller
	conf *cfg.Profile
	x    *x11.Client

	instance mc.InstanceInfo
}

// Setup implements Frontend.
func (m *Single) Setup(deps frontendDependencies) error {
	m.host = deps.host
	m.conf = deps.conf
	m.x = deps.x

	m.instance = deps.instance

	m.host.FocusInstance()
	return nil
}

// Input implements Frontend.
func (m *Single) Input(input Input) {
	actions := m.conf.Keybinds[input.Bind]
	if input.Held {
		return
	}
	for _, action := range actions.IngameActions {
		switch action.Type {
		case cfg.ActionIngameFocus:
			m.host.FocusInstance()
		case cfg.ActionIngameRes:
			if m.x.GetActiveWindow() != m.instance.Wid {
				continue
			}
			if action.Extra != nil {
				resId := *action.Extra
				if resId < 0 || resId > len(m.conf.AltRes)-1 {
					continue
				}
				m.host.ToggleResolution(resId)
			} else {
				m.host.ToggleResolution(0)
			}
		case cfg.ActionIngameReset:
			if m.x.GetActiveWindow() != m.instance.Wid {
				continue
			}
			if m.host.ResetInstance() {
				m.host.RunHook(HookReset, 0)
			}
		}
	}
}

// ProcessEvent implements Frontend.
func (m *Single) ProcessEvent(evt x11.Event) {
	switch evt := evt.(type) {
	case x11.FocusEvent:
		if m.instance.Wid == xproto.Window(evt) {
			m.host.RunHook(HookFocusGained, 0)
		} else {
			m.host.RunHook(HookFocusLost, 0)
		}
	}
}
