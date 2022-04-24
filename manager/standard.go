package manager

import (
	"fmt"
	"resetti/cfg"
	"resetti/mc"
	"resetti/x11"
	"time"
)

// RunStandard runs the Manager in standard multi mode.
func (m *Manager) RunStandard() (chan bool, chan error) {
	// Create channels.
	stopch := make(chan bool, 1)
	errch := make(chan error, 16)
	xerr, xevt := m.x.Loop()

	werr := make(chan WatchError, 16)
	wevt := make(chan WatchUpdate, 16)

	// Setup log watchers and zero all timestamps.
	for j, i := range m.Instances {
		watcher, err := Watch(i, werr, wevt)
		if err != nil {
			errch <- err
			return stopch, errch
		}

		m.Watchers[j] = *watcher
		m.lastTimestamps[j] = 0
	}

	// Start the main goroutine.
	go func() {
		defer m.stopWatchers()
		defer m.x.LoopStop()

		for {
			select {
			case <-stopch:
				// Handle stop event.
				return
			case evt := <-xevt:
				// Handle X hotkey.
				key := x11.Key{
					Code: evt.Detail,
					Mod:  x11.Keymod(evt.State),
				}

				action, ok := m.keys[key]
				if !ok {
					continue
				}

				switch action {
				case cfg.KeyFocus:
					err := m.x.FocusWindowSync(m.Instances[m.Active].Window)
					if err != nil {
						errch <- err
					}
				case cfg.KeyReset:
					go func() {
						// Calculate next instance.
						next := (m.Active + 1) % len(m.Instances)

						// Reset current instance.
						time, err := m.Instances[m.Active].Reset(&m.Settings, m.x, evt.Time)
						if err != nil {
							errch <- err
							return
						}

						// Lock mutex.
						m.mx.Lock()
						defer m.mx.Unlock()

						// Update timestamp.
						m.lastTimestamps[m.Active] = time

						// If we have more than one instance:
						if m.Active != next {
							// Switch to next instance.
							m.Active = next

							// Spawn an additional goroutine to allow the mutex
							// to unlock while waiting for the window to focus.
							go func() {
								// Focus the next instance's window.
								err = m.x.FocusWindowSync(m.Instances[m.Active].Window)
								if err != nil {
									errch <- err
									return
								}

								// If OBS is enabled, switch scenes.
								if m.obs != nil {
									err := m.obs.SetCurrentScene(fmt.Sprintf("Instance %v", m.Active))
									if err != nil {
										errch <- err
									}
								}
							}()

							// Update new instance's state to ingame.
							if m.Instances[m.Active].State == mc.StatePaused {
								m.Instances[m.Active].State = mc.StateIngame
								m.x.SendKeyPress(x11.KeyEscape, m.Instances[m.Active].Window, &evt.Time)
							}
						}
					}()
				}
			case err := <-xerr:
				// Handle X connection error.
				errch <- err
				if err.Error() == "connection died" {
					return
				}
			case evt := <-wevt:
				// Handle watcher state update.
				go func() {
					// Lock mutex.
					m.mx.Lock()
					defer m.mx.Unlock()

					inst := m.Instances[evt.Id]

					// Discard the state update if the instance is ingame and just paused.
					if inst.State == mc.StateIngame && evt.State == mc.StatePaused {
						return
					}

					// If the state didn't actually update, then discard the update.
					if inst.State == evt.State {
						return
					}

					// Get the last timestamp for the updated instance.
					timestamp, ok := m.lastTimestamps[evt.Id]
					if !ok {
						errch <- fmt.Errorf("invalid watch update (no last timestamp)")
						return
					}

					// Update instance state.
					inst.State = evt.State
					m.Instances[evt.Id] = inst

					// If HideMenu is enabled, press F3+Escape.
					// If the instance is active and "paused", don't pause.
					if !m.Settings.HideMenu {
						return
					}

					activeWin, err := m.x.GetActiveWindow()
					if err != nil {
						errch <- err
					}

					isPreview := inst.State == mc.StatePreview
					isPaused := inst.State == mc.StatePaused
					isActive := activeWin == inst.Window
					if isActive && isPaused {
						inst.State = mc.StateIngame
						m.Instances[evt.Id] = inst
						return
					}

					if isPreview || isPaused {
						// Spawn an additional goroutine to allow the mutex to
						// unlock without waiting unnecessarily long.
						go func() {
							time.Sleep(time.Duration(m.Settings.Delay) * time.Millisecond * 2)
							m.x.SendKeyDown(x11.KeyF3, inst.Window, &timestamp)
							m.x.SendKeyPress(x11.KeyEscape, inst.Window, &timestamp)
							m.x.SendKeyUp(x11.KeyF3, inst.Window, &timestamp)
						}()
					}
				}()
			case err := <-werr:
				// Handle watcher error.
				errch <- err.Err

				// If the error is fatal, try to restart the watcher.
				// If it fails to restart after 10 tries, consider it
				// dead and stop the manager.
				if err.Fatal {
					inst := m.Instances[err.Id]

					success := false
					for j := 0; j < 10; j++ {
						watcher, err2 := Watch(inst, werr, wevt)
						if err2 != nil {
							time.Sleep(10 * time.Millisecond)
							continue
						}

						m.Watchers[err.Id] = *watcher
						success = true
						break
					}

					if !success {
						errch <- fmt.Errorf("tried to reboot watcher, failed")
						return
					}
				}
			}
		}
	}()

	return stopch, errch
}
