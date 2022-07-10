package mc

import (
	"fmt"
	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/x11"
	"math"
	"time"

	"github.com/jezek/xgb/xproto"
)

// Multiply desired FOV by this constant to get the number of
// required right presses.
//
// Pressing the right key moves the FOV slider by 1/142 of the
// slider's width, and there are 81 possible FOV values: 30-110
// inclusive.
const FOV_RATIO float64 = 142.0 / 80.0

// Multiply desired mouse sensitivity by this constant to get the
// number of required right presses. Some sensitivties cannot be
// reached by pressing the right key.
//
// This follows the same logic as FOV_RATIO. (142.0 / 200.0)
const SENS_RATIO float64 = 71.0 / 100.0

// The amount of presses needed to reset render distance.
const RD_PRESSES int = 30

// The maximum amount of presses needed to reset a settings slider.
const SETTINGS_PRESSES int = 142

// I couldn't be bothered to figure out a formula for entity distance.
var ED_PRESSES = map[int]int{
	50:  0,
	75:  4,
	100: 12,
	125: 20,
	150: 28,
	175: 36,
	200: 44,
	225: 52,
	250: 60,
	275: 68,
	300: 75,
	325: 83,
	350: 91,
	375: 99,
	400: 107,
	425: 115,
	450: 123,
	475: 131,
	500: 139,
}

// Reset resets an instance according to the user's reset settings.
func (i Instance) Reset(settings *cfg.Config, t xproto.Timestamp) (xproto.Timestamp, error) {
	// Pick the appropriate reset method based on the instance version.
	// TODO: Implement 1.7, 1.8
	switch i.Version {
	case Version1_14, Version1_15, Version1_16:
		err := v16_reset(i, settings, &t)
		return t, err
	default:
		return 0, fmt.Errorf("unsupported version")
	}
}

func v16_reset(i Instance, settings *cfg.Config, t *xproto.Timestamp) error {
	delay := time.Duration(settings.Reset.MenuDelay) * time.Millisecond
	// Act based on the instance's state.
	switch i.State.Identifier {
	case StateUnknown:
		// If the state is unknown, assume the instance is on the title screen.
		x11.SendKeyDown(x11.KeyShift, i.Window, t)
		x11.SendKeyPress(x11.KeyTab, i.Window, t)
		x11.SendKeyUp(x11.KeyShift, i.Window, t)
		x11.SendKeyPress(x11.KeyEnter, i.Window, t)
		return nil
	case StateReady:
		// Press Escape twice to reach the normal menu after F3+Escape.
		x11.SendKeyPressAlt(x11.KeyEscape, i.Window, t)
		time.Sleep(delay)
		x11.SendKeyPressAlt(x11.KeyEscape, i.Window, t)
		time.Sleep(delay)

		x11.SendKeyDown(x11.KeyShift, i.Window, t)
		x11.SendKeyPress(x11.KeyTab, i.Window, t)
		x11.SendKeyUp(x11.KeyShift, i.Window, t)

		x11.SendKeyPress(x11.KeyEnter, i.Window, t)
		return nil
	case StateIngame:
		// If the instance is ingame, break out of the switch and run the main
		// reset action.
		break
	case StatePreview:
		x11.SendKeyPress(x11.KeyH, i.Window, t)
		return nil

	default:
		return fmt.Errorf("bad state; cannot reset")
	}
	// If the user does not want their settings reset, we can just
	// press menu.quitWorld immediately.
	if !settings.Reset.SetSettings {
		x11.SendKeyPress(x11.KeyEscape, i.Window, t)
		time.Sleep(delay)

		x11.SendKeyDown(x11.KeyShift, i.Window, t)
		x11.SendKeyPress(x11.KeyTab, i.Window, t)
		x11.SendKeyUp(x11.KeyShift, i.Window, t)
		time.Sleep(delay)

		x11.SendKeyPress(x11.KeyEnter, i.Window, t)
		return nil
	}
	setRd := settings.Mc.Rd != 0
	setEd := settings.Mc.Ed != 0
	setFov := settings.Mc.Fov != 0
	setSens := settings.Mc.Sens != 0
	if setRd {
		// Set the user's render distance.
		// We will press F3+Shift+F 30 times to ensure that it is set to 2.
		x11.SendKeyDown(x11.KeyF3, i.Window, t)
		x11.SendKeyDown(x11.KeyShift, i.Window, t)
		for j := 0; j < RD_PRESSES; j++ {
			x11.SendKeyPressAlt(x11.KeyF, i.Window, t)
		}
		x11.SendKeyUp(x11.KeyShift, i.Window, t)
		// Then, press F3+F the required amount of times to set it.
		for j := int(0); j < settings.Mc.Rd-2; j++ {
			x11.SendKeyPressAlt(x11.KeyF, i.Window, t)
		}
		// Release F3 once done adjusting render distance.
		x11.SendKeyUp(x11.KeyF3, i.Window, t)
		time.Sleep(delay)
	}
	x11.SendKeyPress(x11.KeyEscape, i.Window, t)
	time.Sleep(delay)
	for j := 0; j < 6; j++ {
		x11.SendKeyPressAlt(x11.KeyTab, i.Window, t)
	}
	x11.SendKeyPress(x11.KeyEnter, i.Window, t)
	x11.SendKeyPress(x11.KeyTab, i.Window, t)

	// Adjust the FOV. First set it to 30, then set it to the user's value.
	for j := 0; j < SETTINGS_PRESSES; j++ {
		x11.SendKeyPressAlt(x11.KeyLeft, i.Window, t)
	}

	presses := int(math.Ceil(float64(settings.Mc.Fov-30) * FOV_RATIO))
	for j := 0; j < presses; j++ {
		x11.SendKeyPressAlt(x11.KeyRight, i.Window, t)
	}

	// Tab 6 times to reach Controls. Press Enter.
	// Tab once to reach Mouse Settings. Press Enter.
	// Tab once to reach Sensitivity.
	for j := 0; j < 6; j++ {
		x11.SendKeyPressAlt(x11.KeyTab, i.Window, t)
	}

	x11.SendKeyPress(x11.KeyEnter, i.Window, t)
	time.Sleep(delay)
	x11.SendKeyPress(x11.KeyTab, i.Window, t)
	x11.SendKeyPress(x11.KeyEnter, i.Window, t)
	time.Sleep(delay)
	x11.SendKeyPress(x11.KeyTab, i.Window, t)

	// Reset and adjust mouse sensitivity.
	for j := 0; j < SETTINGS_PRESSES; j++ {
		x11.SendKeyPressAlt(x11.KeyLeft, i.Window, t)
	}

	presses = int(math.Ceil(float64(settings.Mc.Sens) * SENS_RATIO))
	if settings.Mc.Sens == 200 {
		// 142 presses only brings the sensitivity bar to 199%, not hyperspeed.
		presses += 1
	}
	for j := 0; j < presses; j++ {
		x11.SendKeyPressAlt(x11.KeyRight, i.Window, t)
	}

	// Press Escape 3 times to ex11.t the menu, and once more to reenter.
	for j := 0; j < 4; j++ {
		if setEd || setFov || setSens {
			time.Sleep(delay)
			for j := 0; j < 6; j++ {
				x11.SendKeyPressAlt(x11.KeyTab, i.Window, t)
			}
			x11.SendKeyPress(x11.KeyEnter, i.Window, t)
			x11.SendKeyPress(x11.KeyTab, i.Window, t)
			if setFov {
				// Adjust the FOV. First set it to 30, then set it to the user's value.
				for j := 0; j < SETTINGS_PRESSES; j++ {
					x11.SendKeyPressAlt(x11.KeyLeft, i.Window, t)
				}
				presses := int(math.Ceil(float64(settings.Mc.Fov-30) * FOV_RATIO))
				for j := 0; j < presses; j++ {
					x11.SendKeyPressAlt(x11.KeyRight, i.Window, t)
				}
			}
			if setEd || setSens {
				if setSens {
					for j := 0; j < 6; j++ {
						x11.SendKeyPressAlt(x11.KeyTab, i.Window, t)
					}
					x11.SendKeyPress(x11.KeyEnter, i.Window, t)
					time.Sleep(delay)
					x11.SendKeyPress(x11.KeyTab, i.Window, t)
					x11.SendKeyPress(x11.KeyEnter, i.Window, t)
					time.Sleep(delay)
					x11.SendKeyPress(x11.KeyTab, i.Window, t)
					// Reset and adjust mouse sensitivity.
					for j := 0; j < SETTINGS_PRESSES; j++ {
						x11.SendKeyPressAlt(x11.KeyLeft, i.Window, t)
					}
					presses := int(math.Ceil(float64(settings.Mc.Sens) * SENS_RATIO))
					for j := 0; j < presses; j++ {
						x11.SendKeyPressAlt(x11.KeyRight, i.Window, t)
					}
					// Press Escape 2 times to get back to Settings.
					for j := 0; j < 2; j++ {
						x11.SendKeyPressAlt(x11.KeyEscape, i.Window, t)
						time.Sleep(delay)
					}
					// Send an extra tab key press to get back to FOV for setting
					// entity distance.
					x11.SendKeyPressAlt(x11.KeyTab, i.Window, t)
				}
				if setEd {
					for j := 0; j < 5; j++ {
						x11.SendKeyPressAlt(x11.KeyTab, i.Window, t)
					}
					x11.SendKeyPress(x11.KeyEnter, i.Window, t)
					time.Sleep(delay)
					x11.SendKeyDown(x11.KeyShift, i.Window, t)
					for j := 0; j < 3; j++ {
						x11.SendKeyPressAlt(x11.KeyTab, i.Window, t)
					}
					x11.SendKeyUp(x11.KeyShift, i.Window, t)
					for j := 0; j < SETTINGS_PRESSES; j++ {
						x11.SendKeyPressAlt(x11.KeyLeft, i.Window, t)
					}
					for j := 0; j < ED_PRESSES[settings.Mc.Ed]; j++ {
						x11.SendKeyPressAlt(x11.KeyRight, i.Window, t)
					}
					x11.SendKeyPressAlt(x11.KeyEscape, i.Window, t)
					time.Sleep(delay)
				}
			}
			x11.SendKeyPressAlt(x11.KeyEscape, i.Window, t)
			time.Sleep(delay)
			x11.SendKeyPressAlt(x11.KeyEscape, i.Window, t)
			time.Sleep(delay)
		}
	}
	// Reset.
	x11.SendKeyDown(x11.KeyShift, i.Window, t)
	x11.SendKeyPress(x11.KeyTab, i.Window, t)
	x11.SendKeyUp(x11.KeyShift, i.Window, t)
	x11.SendKeyPress(x11.KeyEnter, i.Window, t)
	return nil
}
