package mc

import (
	"fmt"
	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/x11"
	"time"

	"github.com/jezek/xgb/xproto"
)

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
	delay := time.Duration(settings.Reset.Delay) * time.Millisecond
	switch i.State.Identifier {
	case StateUnknown:
		// If the state is unknown, assume the instance is on the title screen.
		x11.SendKeyDown(x11.KeyShift, i.Window, t)
		x11.SendKeyPress(x11.KeyTab, i.Window, t)
		x11.SendKeyUp(x11.KeyShift, i.Window, t)
		x11.SendKeyPress(x11.KeyEnter, i.Window, t)
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
	case StateIngame:
		x11.SendKeyPressAlt(x11.KeyEscape, i.Window, t)
		time.Sleep(delay)

		x11.SendKeyDown(x11.KeyShift, i.Window, t)
		x11.SendKeyPress(x11.KeyTab, i.Window, t)
		x11.SendKeyUp(x11.KeyShift, i.Window, t)
		x11.SendKeyPress(x11.KeyEnter, i.Window, t)
	case StatePreview:
		x11.SendKeyPress(x11.KeyH, i.Window, t)
	default:
		return fmt.Errorf("bad state; cannot reset")
	}
	return nil
}
