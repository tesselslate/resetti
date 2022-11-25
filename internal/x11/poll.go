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
func (c *Client) Poll(ctx context.Context) (<-chan XEvent, <-chan error, error) {
	ch := make(chan XEvent, CHANNEL_SIZE)
	errCh := make(chan error, ERROR_CHANNEL_SIZE)
	go c.poll(ctx, ch, errCh)
	return ch, errCh, nil
}

func (c *Client) poll(ctx context.Context, ch chan<- XEvent, errCh chan<- error) {
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
				X:     evt.RootX,
				Y:     evt.RootY,
				State: evt.State,
				Time:  evt.Time,
			}
		case xproto.MotionNotifyEvent:
			ch <- MoveEvent{
				X:     evt.RootX,
				Y:     evt.RootY,
				State: evt.State,
				Time:  evt.Time,
			}
		}
	}
}
