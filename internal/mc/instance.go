package mc

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/jezek/xgb/xproto"
	"github.com/tesselslate/resetti/internal/cfg"
	"github.com/tesselslate/resetti/internal/log"
	"github.com/tesselslate/resetti/internal/x11"
)

// TODO: Pre 1.14 support

// An instance contains all of the relevant information for an instance, such
// as its game directory and current state.
type instance struct {
	info   InstanceInfo
	altRes bool
}

// A Manager controls several Minecraft instances. It keeps track of each
// instance's state and communicates with a frontend to operate on the
// instances for the user.
type Manager struct {
	// The mutex is only needed to guard access to the active instance ID and
	// instance states.
	mu sync.Mutex

	instance instance // Minecraft instance being managed

	conf *cfg.Profile
	x    *x11.Client
}

// NewManager attempts to create a new Manager for the given instances.
func NewManager(info InstanceInfo, conf *cfg.Profile, x *x11.Client) (*Manager, error) {
	// Create instance.
	instance := instance{info, false}

	m := Manager{
		sync.Mutex{},
		instance,
		conf,
		x,
	}
	x.Click(info.Wid)

	return &m, nil
}

// Run starts managing instances in the background. Any non-fatal errors are
// logged, any fatal errors are returned via the provided error channel.
func (m *Manager) Run(ctx context.Context) {
	instanceCheckup := time.NewTicker(time.Second)

	for {
		select {
		case <-ctx.Done():
			return
		case <-instanceCheckup.C:
			inst := m.instance
			_, err := os.Stat(fmt.Sprintf("/proc/%d/", inst.info.Pid))
			if err != nil {
				log.Warn("Instance (%s) died. Reboot it and restart resetti.", inst.info.Dir)
			}
		}
	}
}

// Focus attempts to focus the window of the given instance. Any errors will
// be logged.
func (m *Manager) Focus() {
	if err := m.x.FocusWindow(m.instance.info.Wid); err != nil {
		log.Error("Focus failed: %s", err)
	}
}

// ToggleResolution switches the given instance between the normal (play)
// resolution and the given alternate resolution. It returns whether or not
// the instance is now using the alternate resolution.
func (m *Manager) ToggleResolution(resId int) bool {
	if m.instance.altRes {
		m.setResolution(m.conf.NormalRes)
	} else {
		m.setResolution(&m.conf.AltRes[resId])
	}
	m.instance.altRes = !m.instance.altRes
	m.Focus()
	return m.instance.altRes
}

// Reset attempts to reset the given instance. The return value will indicate
// whether or not the instance was in a legal state for resetting. If an actual
// error occurs, it will be logged.
func (m *Manager) Reset() bool {
	// Check if the reset can occur.
	m.mu.Lock()
	defer m.mu.Unlock()

	// Ghost pie fix.
	m.sendKeyUp(x11.KeyShift)
	m.sendKeyPress(x11.KeyF3)
	if m.instance.altRes {
		m.setResolution(m.conf.NormalRes)
		m.instance.altRes = false
	}
	m.sendKeyPress(m.instance.info.ResetKey)
	return true
}

// sendKeyPress sends a key down and key up event to the given instance.
func (m *Manager) sendKeyPress(key xproto.Keycode) {
	m.x.SendKeyPress(key, m.instance.info.Wid)
}

// sendKeyUp sends a key up event to the given instance.
func (m *Manager) sendKeyUp(key xproto.Keycode) {
	m.x.SendKeyUp(key, m.instance.info.Wid)
}

// setResolution sets the window geometry of an instance.
func (m *Manager) setResolution(rect *cfg.Rectangle) {
	if rect == nil {
		return
	}
	m.x.MoveWindow(
		m.instance.info.Wid,
		rect.X, rect.Y, rect.W, rect.H,
	)
}
