package x11

import (
	"encoding/binary"
	"errors"
	"strings"

	"github.com/jezek/xgb/xproto"
)

func closeAndWait() {
	conn.Close()
	for {
		// WaitForEvent returns two nils when the connection is closed.
		evt, err := conn.WaitForEvent()
		if evt == nil && err == nil {
			break
		}
	}
}

func getAtom(name string) (xproto.Atom, error) {
	atomsMu.RLock() // Read lock
	if val, ok := atoms[name]; ok {
		atomsMu.RUnlock() // Read unlock
		return val, nil
	}
	atomsMu.RUnlock() // Read unlock
	res, err := xproto.InternAtom(
		conn,
		false,
		uint16(len(name)),
		name,
	).Reply()
	if err != nil {
		return 0, err
	}
	atomsMu.Lock() // Write lock
	atoms[name] = res.Atom
	atomsMu.Unlock() // Write unlock
	return res.Atom, nil
}

func getProperty(win xproto.Window, name string, typ xproto.Atom) ([]byte, error) {
	atom, err := getAtom(name)
	if err != nil {
		return nil, err
	}
	res, err := xproto.GetProperty(
		conn,
		false,
		win,
		atom,
		typ,
		0,
		(1<<32)-1,
	).Reply()
	if err != nil {
		return nil, err
	}
	if res.Format == 0 {
		return nil, errors.New("format of 0")
	}
	return res.Value, nil
}

func getPropertyInt(win xproto.Window, name string) (uint32, error) {
	bytes, err := getProperty(win, name, xproto.AtomCardinal)
	if err != nil {
		return 0, err
	}
	if len(bytes) != 4 {
		return 0, errors.New("bad response length")
	}
	return binary.LittleEndian.Uint32(bytes), nil
}

func getPropertyString(win xproto.Window, name string) ([]string, error) {
	bytes, err := getProperty(win, name, xproto.AtomString)
	if err != nil {
		return nil, err
	}
	return strings.Split(string(bytes), "\x00"), nil
}

func listenForEvents() {
	for {
		evt, err := conn.WaitForEvent()
		if evt == nil && err == nil {
			errCh <- ErrConnectionDied
			conn = nil
			done <- struct{}{}
			return
		}
		if err != nil {
			errCh <- err
			continue
		}
		switch evt := evt.(type) {

		case xproto.KeyPressEvent:
			evtCh <- KeyEvent{
				Key: Key{
					Code: evt.Detail,
					Mod:  Keymod(evt.State),
				},
				State:     KeyDown,
				Timestamp: evt.Time,
			}
		case xproto.KeyReleaseEvent:
			evtCh <- KeyEvent{
				Key: Key{
					Code: evt.Detail,
					Mod:  Keymod(evt.State),
				},
				State:     KeyUp,
				Timestamp: evt.Time,
			}
		case xproto.ButtonPressEvent:
			evtCh <- ButtonEvent{
				X:         evt.RootX,
				Y:         evt.RootY,
				State:     evt.State,
				Timestamp: evt.Time,
			}
		case xproto.MotionNotifyEvent:
			evtCh <- MoveEvent{
				X:         evt.RootX,
				Y:         evt.RootY,
				State:     evt.State,
				Timestamp: evt.Time,
			}
		}
	}
}

func sendKey(code xproto.Keycode, keydown bool, time xproto.Timestamp, win xproto.Window) error {
	evt := xproto.KeyPressEvent{
		Sequence:   0,
		Detail:     code,
		Time:       time,
		Root:       win,
		Event:      win,
		Child:      win,
		RootX:      0,
		RootY:      0,
		EventX:     0,
		EventY:     0,
		SameScreen: true,
	}
	// Get the raw bytes of this event and set the type to 3 (KeyRelease)
	// before sending if it is a KeyUp event.
	// Alternatively, `evt` could be casted to xproto.KeyReleaseEvent.
	bytes := evt.Bytes()
	if !keydown {
		bytes[0] = 3
	}
	return xproto.SendEventChecked(
		conn,
		true,
		win,
		key_mask,
		string(bytes),
	).Check()
}
