// Package x11 implements an X11 client which is used for sending
// synthetic key events and managing Minecraft instances.
package x11

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// Attributes contains various window attributes.
type Attributes struct {
	Pid   uint32
	Class []string
}

// Keymod represents modifiers held down for a keypress.
type Keymod uint16

const (
	ModShift Keymod = 1 << 0
	ModLock         = 1 << 1
	ModCtrl         = 1 << 2
	Mod1            = 1 << 3
	Mod2            = 1 << 4
	Mod3            = 1 << 5
	Mod4            = 1 << 6
	Mod5            = 1 << 7
	ModNone         = 0
)

// Key represents the contents of a keypress.
type Key struct {
	Code xproto.Keycode
	Mod  Keymod
}

// KeyEvent represents a single key event.
type KeyEvent struct {
	Key   Key
	State KeyState
}

// KeyState represents the state of a keypress.
type KeyState int

const (
	KeyUp    KeyState = 0
	KeyDown  KeyState = 1
	KeyPress KeyState = 2
)

// Client managaes an active X connection.
type Client struct {
	Root xproto.Window
	conn *xgb.Conn
	keys []Key
}

// NewClient creates a new Client instance.
func NewClient() (*Client, error) {
	x, err := xgb.NewConn()
	if err != nil {
		return nil, err
	}

	root := xproto.Setup(x).DefaultScreen(x).Root
	client := Client{
		Root: root,
		conn: x,
		keys: []Key{},
	}

	return &client, nil
}

// getProperty returns the raw bytes of a window property, if it exists.
func (c *Client) getProperty(win xproto.Window, atom xproto.Atom, atype xproto.Atom) ([]byte, error) {
	var offset uint32 = 0
	buf := []byte{}

	for {
		reply, err := xproto.GetProperty(c.conn, false, win, atom, atype, offset, 8).Reply()
		if err != nil {
			return nil, err
		}

		if reply.Format == 0 {
			return nil, fmt.Errorf("format of 0")
		}

		buf = append(buf, reply.Value...)
		if err != nil {
			return nil, err
		}

		if reply.BytesAfter == 0 {
			break
		}

		offset += 8
	}

	return buf, nil
}

// GetPropertyInt gets a window property and returns it as an integer.
func (c *Client) GetPropertyInt(win xproto.Window, name string) (uint32, error) {
	reply, err := xproto.InternAtom(c.conn, false, uint16(len(name)), name).Reply()
	if err != nil {
		return 0, err
	}

	bytes, err := c.getProperty(win, reply.Atom, xproto.AtomCardinal)
	if err != nil {
		return 0, err
	}

	return binary.LittleEndian.Uint32(bytes), nil
}

// GetPropertyString gets a window property and returns it as a string.
func (c *Client) GetPropertyString(win xproto.Window, name string) ([]string, error) {
	reply, err := xproto.InternAtom(c.conn, false, uint16(len(name)), name).Reply()
	if err != nil {
		return nil, err
	}

	rawprop, err := c.getProperty(win, reply.Atom, xproto.AtomString)
	if err != nil {
		return nil, err
	}

	substrings := bytes.Split(rawprop, []byte{0})
	strings := []string{}

	for _, substr := range substrings {
		strings = append(strings, string(substr))
	}

	return strings, nil
}

// GetWindowAttributes returns the attributes (PID, class) of the given window.
func (c *Client) GetWindowAttributes(win xproto.Window) (*Attributes, error) {
	pid, err := c.GetPropertyInt(win, "_NET_WM_PID")
	if err != nil {
		return nil, err
	}

	class, err := c.GetPropertyString(win, "WM_CLASS")
	if err != nil {
		return nil, err
	}

	attrs := Attributes{
		pid,
		class,
	}

	return &attrs, nil
}

// GetWindowList gets a list of all windows which are beneath the given window
// by recursively searching the window tree.
func (c *Client) GetWindowList(win xproto.Window) ([]xproto.Window, error) {
	reply, err := xproto.QueryTree(c.conn, win).Reply()
	if err != nil {
		return nil, err
	}

	var children []xproto.Window
	for _, child := range reply.Children {
		children = append(children, child)
		others, err := c.GetWindowList(child)
		if err != nil {
			return nil, err
		}

		children = append(children, others...)
	}

	return children, nil
}

// GrabKey "grabs" a key from the X server so that all instances of that key
// being pressed are routed to resetti.
func (c *Client) GrabKey(key Key) {
	xproto.GrabKey(c.conn, true, c.Root, uint16(key.Mod), key.Code, xproto.GrabModeAsync, xproto.GrabModeAsync)
	c.keys = append(c.keys, key)
}

// SendKey sends a synthetic keypress to the given window.
func (c *Client) SendKey(press KeyEvent, win xproto.Window, timestamp xproto.Timestamp) error {
	if press.State == KeyPress {
		newPress := press
		newPress.State = KeyDown

		if err := c.SendKey(newPress, win, timestamp); err != nil {
			return err
		}

		newPress.State = KeyUp
		if err := c.SendKey(newPress, win, timestamp+1); err != nil {
			return err
		}

		return nil
	}

	evt := xproto.KeyPressEvent{
		Sequence:   0,
		Detail:     press.Key.Code,
		Time:       timestamp,
		Root:       win,
		Event:      win,
		Child:      win,
		RootX:      0,
		RootY:      0,
		EventX:     0,
		EventY:     0,
		State:      uint16(press.Key.Mod),
		SameScreen: true,
	}

	// TODO: See how fast SendEventChecked is. Events *need* to be sent sequentially for
	// resetti to work properly, but it would be nice to see if they can all be sent at
	// once and still arrive to GLFW in the correct order.
	switch press.State {
	case KeyDown:
		reply := xproto.SendEventChecked(c.conn, true, win, xproto.EventMaskKeyPress, string(evt.Bytes()))
		return reply.Check()
	case KeyUp:
		evt := xproto.KeyReleaseEvent(evt)
		reply := xproto.SendEventChecked(c.conn, true, win, xproto.EventMaskKeyRelease, string(evt.Bytes()))
		return reply.Check()
	default:
		panic("invalid keypress state")
	}
}

// UngrabKey returns a key to the X server after previously grabbing it.
func (c *Client) UngrabKey(key Key) {
	xproto.UngrabKey(c.conn, key.Code, c.Root, uint16(key.Mod))

	i := 0
	for _, v := range c.keys {
		if v != key {
			c.keys[i] = v
			i++
		}
	}

	c.keys = c.keys[:i]
}
