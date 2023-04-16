package mc

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/jezek/xgb/xproto"
	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/x11"
)

// TODO: Pre 1.14 support

// An instance contains all of the relevant information for an instance, such
// as its game directory and current state.
type instance struct {
	info   InstanceInfo
	reader StateReader
	state  State
	altRes bool
}

// A Manager controls several Minecraft instances. It keeps track of each
// instance's state and communicates with a frontend to operate on the
// instances for the user.
type Manager struct {
	// The mutex is only needed to guard access to the active instance ID and
	// instance states.
	mu sync.Mutex

	active    int               // Active instance ID. -1 signals no active instance.
	instances []instance        // List of instances
	paths     map[string]int    // State file -> instance ID mapping
	watcher   *fsnotify.Watcher // State file watcher
	pause     chan int          // Instances to pause

	conf *cfg.Profile
	x    *x11.Client
}

// NewManager attempts to create a new Manager for the given instances.
func NewManager(infos []InstanceInfo, conf *cfg.Profile, x *x11.Client) (*Manager, error) {
	// Create instances.
	instances := make([]instance, 0, len(infos))
	for _, info := range infos {
		reader, state, err := createStateReader(info)
		if err != nil {
			return nil, err
		}
		if state.Type == stWorld {
			state.Type = StIdle
		}
		instance := instance{info, reader, state, false}
		instances = append(instances, instance)
	}

	// Setup state watcher.
	watcher, err := fsnotify.NewWatcher()
	paths := make(map[string]int)
	if err != nil {
		return nil, fmt.Errorf("open watcher: %w", err)
	}
	for idx, inst := range instances {
		path := inst.reader.Path()
		paths[path] = idx
		if err := watcher.Add(path); err != nil {
			_ = watcher.Close()
			return nil, fmt.Errorf("watch instance %d: %w", idx, err)
		}
	}

	m := Manager{
		sync.Mutex{},
		-1,
		instances,
		paths,
		watcher,
		make(chan int, len(instances)*2),
		conf,
		x,
	}

	// Warmup instances.
	for _, inst := range infos {
		x.Click(inst.Wid)
	}

	return &m, nil
}

// GetStates returns a list of all instance states.
func (m *Manager) GetStates() []State {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []State
	for _, inst := range m.instances {
		out = append(out, inst.state)
	}
	return out
}

// Run starts managing instances in the background. Any non-fatal errors are
// logged, any fatal errors are returned via the provided error channel.
func (m *Manager) Run(ctx context.Context, evtch chan<- Update, errch chan<- error) {
	instanceCheckup := time.NewTicker(time.Second)
	defer func() {
		instanceCheckup.Stop()
		_ = m.watcher.Close()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-instanceCheckup.C:
			for id, inst := range m.instances {
				_, err := os.Stat(fmt.Sprintf("/proc/%d/", inst.info.Pid))
				if err != nil {
					errch <- fmt.Errorf("instance %d (%s) died. reboot it", id, inst.info.Dir)
					return
				}
			}
		case id := <-m.pause:
			state := m.instances[id].state.Type
			if state == StPreview || state == StIdle {
				m.sendKeyDown(id, x11.KeyF3)
				m.sendKeyPress(id, x11.KeyEsc)
				m.sendKeyUp(id, x11.KeyF3)
			}
		case evt, ok := <-m.watcher.Events:
			if !ok {
				errch <- errors.New("watcher events closed")
				return
			}
			id := m.paths[evt.Name]
			switch evt.Op {
			case fsnotify.Write:
				// Process any updates to the state file.
				state, updated, err := m.instances[id].reader.Process()
				if err != nil {
					log.Printf("process log (%d) failed: %s", id, err)
					continue
				}
				if !updated {
					continue
				}

				// Only modify the fields that the state reader knows about.
				m.mu.Lock()
				lastType := m.instances[id].state.Type
				m.instances[id].state.Type = state.Type
				m.instances[id].state.Progress = state.Progress
				m.instances[id].state.Menu = state.Menu

				// The stWorld state should only ever be handled internally.
				// Update it to the appropriate public state before notifying
				// the frontend.
				switch state.Type {
				case stWorld:
					if m.active == id {
						m.instances[id].state.Type = StIngame
					} else {
						m.instances[id].state.Type = StIdle
						if lastType != StIdle {
							if m.conf.Delay.IdlePause > 0 {
								time.AfterFunc(time.Millisecond*time.Duration(m.conf.Delay.IdlePause), func() {
									m.pause <- id
								})
							} else {
								m.sendKeyDown(id, x11.KeyF3)
								m.sendKeyPress(id, x11.KeyEsc)
								m.sendKeyUp(id, x11.KeyF3)
							}
						}
					}
				case StPreview:
					if lastType != StPreview {
						m.instances[id].state.LastPreview = time.Now()
						if m.conf.Delay.WpPause > 0 {
							time.AfterFunc(time.Millisecond*time.Duration(m.conf.Delay.WpPause), func() {
								m.pause <- id
							})
						} else {
							m.sendKeyDown(id, x11.KeyF3)
							m.sendKeyPress(id, x11.KeyEsc)
							m.sendKeyUp(id, x11.KeyF3)
						}
					}
				}
				evtch <- Update{m.instances[id].state, id}
				m.mu.Unlock()
			default:
				err := m.instances[id].reader.ProcessEvent(evt.Op)
				if err != nil {
					errch <- fmt.Errorf("process event (%d) failed: %w", id, err)
					return
				}
			}
		case err, ok := <-m.watcher.Errors:
			if !ok {
				errch <- fmt.Errorf("watcher died: %w", err)
			}
			log.Printf("Manager: watcher error: %s\n", err)
		}
	}
}

// Focus attempts to focus the window of the given instance. Any errors will
// be logged.
func (m *Manager) Focus(id int) {
	if err := m.x.FocusWindow(m.instances[id].info.Wid); err != nil {
		log.Printf("Focus %d failed: %s\n", id, err)
	}
}

// ToggleResolution switches the given instance between the normal and alternate
// resolution and returns whether or not it is now on the alternate resolution.
func (m *Manager) ToggleResolution(id int) bool {
	if m.instances[id].altRes {
		m.setResolution(id, m.conf.Wall.UnstretchRes)
	} else {
		m.setResolution(id, m.conf.Wall.AltRes)
	}
	m.instances[id].altRes = !m.instances[id].altRes
	m.Focus(id)
	return m.instances[id].altRes
}

// Play attempts to play the given instance.
//
// If there is a currently active instance, the given instance will supersede it.
// Any additional actions which should happen before playing (e.g. stretching,
// unpausing, F1) will be handled by this function. Any errors will be logged.
func (m *Manager) Play(id int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Focus(id)
	m.active = id
	m.instances[id].state.Type = StIngame

	if m.conf.UnpauseFocus {
		m.sendKeyPress(id, x11.KeyEsc)
	}
	if m.conf.Wall.Enabled {
		if m.conf.Delay.Stretch > 0 {
			time.Sleep(time.Millisecond * time.Duration(m.conf.Delay.Stretch))
		}
		m.setResolution(id, m.conf.Wall.UnstretchRes)
		if m.conf.UnpauseFocus && m.conf.Wall.UseF1 {
			m.sendKeyPress(id, x11.KeyF1)
		}
	}

	if m.conf.UnpauseFocus {
		// Pause and unpause again to let the cursor position update for the next
		// time a menu is opened.
		if m.conf.Delay.Unpause > 0 {
			time.Sleep(time.Millisecond * time.Duration(m.conf.Delay.Unpause))
		}
		m.sendKeyPress(id, x11.KeyEsc)
		m.sendKeyPress(id, x11.KeyEsc)
	}
}

// Reset attempts to reset the given instance. The return value will indicate
// whether or not the instance was in a legal state for resetting. If an actual
// error occurs, it will be logged.
func (m *Manager) Reset(id int) bool {
	// Check if the reset can occur.
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.instances[id].state
	if state.Type == StDirt {
		return false
	}
	if m.conf.Wall.Enabled {
		if time.Since(state.LastPreview) < time.Duration(m.conf.Wall.GracePeriod) {
			return false
		}
	}

	// Reset.
	if m.active == id {
		// Ghost pie fix.
		m.sendKeyUp(id, x11.KeyShift)
		m.sendKeyPress(id, x11.KeyF3)
		if m.conf.Delay.GhostPie > 0 {
			time.Sleep(time.Millisecond * time.Duration(m.conf.Delay.GhostPie))
		}

		// Unstretch.
		m.instances[id].altRes = false
		m.active = -1
		m.setResolution(id, m.conf.Wall.StretchRes)
		if m.conf.Delay.Stretch > 0 {
			time.Sleep(time.Millisecond * time.Duration(m.conf.Delay.Stretch))
		}
	}
	var key xproto.Keycode
	if state.Type == StPreview && state.Progress < 80 {
		key = m.instances[id].info.PreviewKey
	} else {
		key = m.instances[id].info.ResetKey
	}
	m.sendKeyPress(id, key)
	return true
}

// sendKeyDown sends a key down event to the given instance.
func (m *Manager) sendKeyDown(id int, key xproto.Keycode) {
	m.x.SendKeyDown(key, m.instances[id].info.Wid)
}

// sendKeyPress sends a key down and key up event to the given instance.
func (m *Manager) sendKeyPress(id int, key xproto.Keycode) {
	m.x.SendKeyPress(key, m.instances[id].info.Wid)
}

// sendKeyUp sends a key up event to the given instance.
func (m *Manager) sendKeyUp(id int, key xproto.Keycode) {
	m.x.SendKeyUp(key, m.instances[id].info.Wid)
}

// setResolution sets the window geometry of an instance.
func (m *Manager) setResolution(id int, rect *cfg.Rectangle) {
	if rect == nil || !m.conf.Wall.Enabled {
		return
	}
	m.x.MoveWindow(
		m.instances[id].info.Wid,
		rect.X, rect.Y, rect.W, rect.H,
	)
}
