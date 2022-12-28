package x11

import (
	"context"
	"errors"
	"log"

	"github.com/jezek/xgb/xproto"
)

var ErrDied = errors.New("connection closed")

// Poll starts a separate goroutine which will listen for and forward
// various input events (keypresses, mouse movements, button presses).
func (c *Client) Poll(ctx context.Context, substructures bool) (<-chan XEvent, <-chan error, error) {
	ch := make(chan XEvent, CHANNEL_SIZE)
	errCh := make(chan error, ERROR_CHANNEL_SIZE)
	if substructures {
		err := xproto.ChangeWindowAttributesChecked(
			c.conn,
			c.root,
			xproto.CwEventMask,
			[]uint32{xproto.EventMaskPropertyChange | xproto.EventMaskSubstructureNotify},
		).Check()
		if err != nil {
			return nil, nil, err
		}
	}
	go c.poll(ctx, substructures, ch, errCh)
	return ch, errCh, nil
}

func (c *Client) poll(ctx context.Context, substructures bool, ch chan<- XEvent, errCh chan<- error) {
	defer log.Println("Service: X11 poller stopped")
	defer close(ch)
	defer close(errCh)

	log.Println("Service: X11 poller started")
	for {
		// Check if event polling should stop.
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Wait for the next X event/error and process it.
		evt, err := c.conn.WaitForEvent()
		if evt == nil && err == nil {
			errCh <- ErrDied
			return
		}
		if err != nil {
			errCh <- err
			continue
		}

		// If the event is a user input, then forward it.
		switch evt := evt.(type) {
		case xproto.KeyPressEvent:
			ch <- KeyEvent{
				Key: Key{
					Code: evt.Detail,
					Mod:  Keymod(evt.State),
				},
				State: KeyDown,
				Time:  evt.Time,
			}
		case xproto.KeyReleaseEvent:
			ch <- KeyEvent{
				Key: Key{
					Code: evt.Detail,
					Mod:  Keymod(evt.State),
				},
				State: KeyUp,
				Time:  evt.Time,
			}
		case xproto.ButtonPressEvent:
			ch <- ButtonEvent{
				X:     evt.EventX,
				Y:     evt.EventY,
				Win:   evt.Child,
				State: evt.State,
				Time:  evt.Time,
			}
		case xproto.MotionNotifyEvent:
			ch <- MoveEvent{
				X:     evt.EventX,
				Y:     evt.EventY,
				Win:   evt.Child,
				State: evt.State,
				Time:  evt.Time,
			}
		case xproto.PropertyNotifyEvent:
			atom, err := c.atoms.Get(c, "_NET_ACTIVE_WINDOW")
			if err != nil {
				errCh <- err
				continue
			}
			if atom != evt.Atom {
				continue
			}
			win, err := c.GetActiveWindow()
			if err != nil {
				errCh <- err
				continue
			}
			ch <- FocusEvent{win, evt.Time}
		}
	}
}
