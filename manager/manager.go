// Package manager provides a "reset manager" which handles incoming events
// from various sources, manages and resets instances, and updates OBS as
// needed.
package manager

import (
	"fmt"
	"resetti/cfg"
	"resetti/mc"
	"resetti/obs"
	"resetti/x11"

	"github.com/jezek/xgb/xproto"
)

// Manager provides a reset manager implementation capable of both
// traditional multi-instance resetting and wall-style resetting.
type Manager struct {
	Active         int
	Settings       cfg.ResetSettings
	Instances      []mc.Instance
	Watchers       []Watcher
	keys           map[x11.Key]int
	lastTimestamps map[int]xproto.Timestamp
	x              *x11.Client
	obs            *obs.Client
}

// NewManager creates a new Manager instance.
func NewManager(x *x11.Client, o *obs.Client, settings cfg.Config) (*Manager, error) {
	// Setup instance map.
	instances, err := mc.GetInstances(x)
	if err != nil {
		return nil, err
	}

	if len(instances) == 0 {
		return nil, fmt.Errorf("no instances found")
	}

	// Setup hotkey map.
	keys := make(map[x11.Key]int)
	keys[settings.Keys.Reset] = cfg.KeyReset
	keys[settings.Keys.Focus] = cfg.KeyFocus

	for v := range keys {
		x.GrabKey(v)
	}

	manager := Manager{
		Active:         0,
		Settings:       settings.Reset,
		Instances:      instances,
		Watchers:       make([]Watcher, len(instances)),
		keys:           keys,
		lastTimestamps: make(map[int]xproto.Timestamp),
		x:              x,
		obs:            o,
	}

	return &manager, nil
}

// stopWatchers stops all active log watchers.
func (m *Manager) stopWatchers() {
	// Stop watchers.
	for _, w := range m.Watchers {
		w.Stop()
	}

	// Clear watchers.
	m.Watchers = []Watcher{}
}
