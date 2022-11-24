package x11

import (
	"errors"

	"github.com/jezek/xgb/xproto"
)

var ErrDied = errors.New("connection closed")

// Poll starts a separate goroutine which will listen for and forward
// various input events (keypresses, mouse movements, button presses).
// TODO: use context/wg instead of stoppoll
func (c *Client) Poll() (<-chan XEvent, <-chan error, error) {
	if c.polling {
		return nil, nil, errors.New("already polling")
	}
	c.polling = true
	ch := make(chan XEvent, CHANNEL_SIZE)
	errCh := make(chan error, ERROR_CHANNEL_SIZE)
	go c.poll(ch, errCh)
	return ch, errCh, nil
}

// StopPoll stops polling for events.
func (c *Client) StopPoll() {
	c.stopPolling <- struct{}{}
}

func (c *Client) poll(ch chan<- XEvent, errCh chan<- error) {
	defer close(ch)
	defer close(errCh)
	for {
		// Check if event polling should stop.
		select {
		case <-c.stopPolling:
			c.polling = false
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
