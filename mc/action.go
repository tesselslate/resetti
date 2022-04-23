package mc

import (
	"fmt"
	"math"
	"resetti/cfg"
	"resetti/x11"
	"time"

	"github.com/jezek/xgb/xproto"
)

// Multiply desired FOV by this constant to get the number of
// required right presses.
//
// Pressing the right key moves the FOV slider by 1/142 of the
// slider's width, and there are 81 possible FOV values: 30-110
// inclusive.
const FOV_RATIO float64 = 142.0 / 81.0

// The maximum amount of presses needed to reset FOV.
const FOV_PRESSES int = 142

// Multiply desired mouse sensitivity by this constant to get the
// number of required right presses. Some sensitivties cannot be
// reached by pressing the right key.
//
// This follows the same logic as FOV_RATIO. (142.0 / 200.0)
const SENS_RATIO float64 = 71.0 / 100.0

// The maximum amount of presses needed to reset sensitivity.
const SENS_PRESSES int = 142

// The amount of presses needed to reset render distance.
const RD_PRESSES int = 30

// Reset resets an instance according to the user's reset settings.
func (i *Instance) Reset(settings *cfg.ResetSettings, x *x11.Client, t xproto.Timestamp) error {
	// Pick the appropriate reset method based on the instance version.
	// TODO: Implement 1.7, 1.8, 1.14, 1.15
	// TODO: Implement F3+Esc pause on load/preview in log reader loop
	switch i.Version {
	case Version1_16:
		return v16_reset(i, settings, x, t)
	default:
		return fmt.Errorf("unsupported version")
	}
}

func v16_reset(i *Instance, settings *cfg.ResetSettings, x *x11.Client, t xproto.Timestamp) error {
	delay := time.Duration(settings.Delay) * time.Millisecond

	// Act based on the instance's state.
	switch i.State {
	case StateUnknown:
		return fmt.Errorf("cannot reset with unknown state")
	case StateIdle:
		// If F3+Esc, press Escape twice to reach the normal menu.
		if settings.HideMenu {
			x.SendKeyPress(x11.KeyEscape, i.Window, &t)
			time.Sleep(delay)
			x.SendKeyPress(x11.KeyEscape, i.Window, &t)
			time.Sleep(delay)
		}

		x.SendKeyDown(x11.KeyShift, i.Window, &t)
		x.SendKeyPress(x11.KeyTab, i.Window, &t)
		x.SendKeyUp(x11.KeyShift, i.Window, &t)

		x.SendKeyPress(x11.KeyEnter, i.Window, &t)
		return nil
	case StateIngame:
		// If the instance is ingame, break out of the switch and run the main
		// reset action.
		break
	case StateGenerating:
		// If the instance is currently generating a world, it can be reset
		// only if WorldPreview is active.
		if !i.HasWp {
			return fmt.Errorf("cannot reset during worldgen without worldpreview")
		}

		// If F3+Esc, press Escape to reach the normal menu.
		if settings.HideMenu {
			x.SendKeyPress(x11.KeyEscape, i.Window, &t)
			time.Sleep(delay)
		}

		x.SendKeyDown(x11.KeyShift, i.Window, &t)
		x.SendKeyPress(x11.KeyTab, i.Window, &t)
		x.SendKeyUp(x11.KeyShift, i.Window, &t)

		x.SendKeyPress(x11.KeyEnter, i.Window, &t)
		return nil
	}

	// If the user has LowRd enabled, set their render distance to 5.
	if settings.LowRd {
		x.SendKeyDown(x11.KeyF3, i.Window, &t)

		x.SendKeyDown(x11.KeyShift, i.Window, &t)
		for j := 0; j < RD_PRESSES; j++ {
			x.SendKeyPress(x11.KeyF, i.Window, &t)
		}
		x.SendKeyUp(x11.KeyShift, i.Window, &t)

		for j := uint8(0); j < 3; j++ {
			x.SendKeyPress(x11.KeyF, i.Window, &t)
		}

		x.SendKeyUp(x11.KeyF3, i.Window, &t)
	}

	// If the user does not want their settings reset, we can just
	// press menu.quitWorld immediately.
	if !settings.SetSettings {
		x.SendKeyPress(x11.KeyEscape, i.Window, &t)
		time.Sleep(delay)

		x.SendKeyDown(x11.KeyShift, i.Window, &t)
		x.SendKeyPress(x11.KeyTab, i.Window, &t)
		x.SendKeyUp(x11.KeyShift, i.Window, &t)
		time.Sleep(delay)

		x.SendKeyPress(x11.KeyEnter, i.Window, &t)
		return nil
	}

	// Set RD if the user doesn't have LowRd enabled.
	if !settings.LowRd {
		// We will press F3+Shift+F 30 times to ensure that it is set to 2.
		x.SendKeyDown(x11.KeyF3, i.Window, &t)

		x.SendKeyDown(x11.KeyShift, i.Window, &t)
		for j := 0; j < RD_PRESSES; j++ {
			x.SendKeyPress(x11.KeyF, i.Window, &t)
		}
		x.SendKeyUp(x11.KeyShift, i.Window, &t)

		// Then, press F3+F the required amount of times to set it.
		for j := uint8(0); j < settings.Mc.Render-2; j++ {
			x.SendKeyPress(x11.KeyF, i.Window, &t)
		}

		// Release F3 once done adjusting render distance.
		x.SendKeyUp(x11.KeyF3, i.Window, &t)
	}

	// Then, pause the game, enter the Options menu, and select FOV.
	// Escape -> Tab x6 -> Enter -> Tab
	x.SendKeyPress(x11.KeyEscape, i.Window, &t)
	time.Sleep(delay)
	for j := 0; j < 6; j++ {
		x.SendKeyPress(x11.KeyTab, i.Window, &t)
	}
	x.SendKeyPress(x11.KeyEnter, i.Window, &t)
	x.SendKeyPress(x11.KeyTab, i.Window, &t)

	// Adjust the FOV. First set it to 30, then set it to the user's value.
	for j := 0; j < FOV_PRESSES; j++ {
		x.SendKeyPress(x11.KeyLeft, i.Window, &t)
	}

	presses := int(math.Ceil(float64(settings.Mc.Fov-30) * FOV_RATIO))
	for j := 0; j < presses; j++ {
		x.SendKeyPress(x11.KeyRight, i.Window, &t)
	}

	// Tab 6 times to reach Controls. Press Enter.
	// Tab once to reach Mouse Settings. Press Enter.
	// Tab once to reach Sensitivity.
	for j := 0; j < 6; j++ {
		x.SendKeyPress(x11.KeyTab, i.Window, &t)
	}

	x.SendKeyPress(x11.KeyEnter, i.Window, &t)
	time.Sleep(delay)
	x.SendKeyPress(x11.KeyTab, i.Window, &t)
	x.SendKeyPress(x11.KeyEnter, i.Window, &t)
	time.Sleep(delay)
	x.SendKeyPress(x11.KeyTab, i.Window, &t)

	// Reset and adjust mouse sensitivity.
	for j := 0; j < SENS_PRESSES; j++ {
		x.SendKeyPress(x11.KeyLeft, i.Window, &t)
	}

	presses = int(math.Ceil(float64(settings.Mc.Sensitivity) * SENS_RATIO))
	for j := 0; j < presses; j++ {
		x.SendKeyPress(x11.KeyRight, i.Window, &t)
	}

	// Press Escape 3 times to exit the menu, and once more to reenter.
	for j := 0; j < 4; j++ {
		x.SendKeyPress(x11.KeyEscape, i.Window, &t)
		time.Sleep(delay)
	}

	// Reset.
	x.SendKeyDown(x11.KeyShift, i.Window, &t)
	x.SendKeyPress(x11.KeyTab, i.Window, &t)
	x.SendKeyUp(x11.KeyShift, i.Window, &t)

	x.SendKeyPress(x11.KeyEnter, i.Window, &t)

	return nil
}
