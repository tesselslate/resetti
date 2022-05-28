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

// Key represents the contents of a keypress.
type Key struct {
	Code xproto.Keycode
	Mod  Keymod
}

// KeyEvent represents a single key event.
type KeyEvent struct {
	Key       Key
	State     KeyState
	Timestamp xproto.Timestamp
}

// KeyState represents the state of a keypress.
type KeyState int

// Client managaes an active X connection.
type Client struct {
	Root xproto.Window
	conn *xgb.Conn
	keys []Key
	loop bool

	Errors chan error
	Keys   chan KeyEvent
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
		loop: false,
	}

	return &client, nil
}

// FocusWindow sets the given window as the active window.
func (c *Client) FocusWindow(win xproto.Window) error {
	const ACTIVE_WINDOW string = "_NET_ACTIVE_WINDOW"

	// Get root window of target window.
	geo, err := xproto.GetGeometry(c.conn, xproto.Drawable(win)).Reply()
	if err != nil {
		return err
	}

	// Activate target activeWin.
	activeWin, err := xproto.InternAtom(c.conn, false, uint16(len(ACTIVE_WINDOW)), ACTIVE_WINDOW).Reply()
	if err != nil {
		return err
	}

	// Create X request.
	data := make([]uint32, 5)
	data[0] = 2
	data[1] = 0

	evt := xproto.ClientMessageEvent{
		Format: 32,
		Window: win,
		Type:   activeWin.Atom,
		Data:   xproto.ClientMessageDataUnionData32New(data),
	}

	// Send request.
	err = xproto.SendEventChecked(
		c.conn,
		true,
		geo.Root,
		xproto.EventMaskSubstructureNotify|xproto.EventMaskSubstructureRedirect,
		string(evt.Bytes()),
	).Check()

	return err
}

// GetActiveWindow returns the currently activated window.
func (c *Client) GetActiveWindow() (xproto.Window, error) {
	const ACTIVE_WINDOW string = "_NET_ACTIVE_WINDOW"
	activeWin, err := xproto.InternAtom(c.conn, false, uint16(len(ACTIVE_WINDOW)), ACTIVE_WINDOW).Reply()
	if err != nil {
		return 0, err
	}

	winBytes, err := c.getProperty(c.Root, activeWin.Atom, xproto.AtomWindow)
	if err != nil {
		return 0, err
	}

	if len(winBytes) == 0 {
		return 0, fmt.Errorf("no response")
	}

	return xproto.Window(binary.LittleEndian.Uint32(winBytes)), nil
}

// getProperty returns the raw bytes of a window property, if it exists.
func (c *Client) getProperty(win xproto.Window, atom xproto.Atom, atype xproto.Atom) ([]byte, error) {
	reply, err := xproto.GetProperty(c.conn, false, win, atom, atype, 0, (1<<32)-1).Reply()
	if err != nil {
		return nil, err
	}

	if reply.Format == 0 {
		return nil, fmt.Errorf("format of 0")
	}

	return reply.Value, nil
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

	if len(bytes) != 4 {
		return 0, fmt.Errorf("no response")
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

// GetWindowTitle gets the title of a window.
func (c *Client) GetWindowTitle(win xproto.Window) (string, error) {
	res, err := c.GetPropertyString(win, "WM_NAME")
	if err != nil {
		return "", err
	}
	return res[0], nil
}

// GrabKey "grabs" a key from the X server so that all instances of that key
// being pressed are routed to resetti.
func (c *Client) GrabKey(key Key) {
	xproto.GrabKey(c.conn, true, c.Root, uint16(key.Mod), key.Code, xproto.GrabModeAsync, xproto.GrabModeAsync)
	c.keys = append(c.keys, key)
}

// GrabKeyboard "grabs" the entire keyboard from the X server.
func (c *Client) GrabKeyboard() error {
	_, err := xproto.GrabKeyboard(
		c.conn,
		true,
		c.Root,
		xproto.TimeCurrentTime,
		xproto.GrabModeAsync,
		xproto.GrabModeAsync,
	).Reply()
	return err
}

// Loop starts a goroutine which will listen for keypress events from
// the X server.
func (c *Client) Loop() {
	c.Errors = make(chan error, 16)
	c.Keys = make(chan KeyEvent, 16)
	c.loop = true

	go func() {
		for c.loop {
			evt, err := c.conn.WaitForEvent()
			if err == nil && evt == nil {
				c.Errors <- fmt.Errorf("connection died")
				return
			}

			if err != nil {
				c.Errors <- err
				continue
			}

			switch evt := evt.(type) {
			case xproto.KeyPressEvent:
				c.Keys <- KeyEvent{
					Key: Key{
						Code: evt.Detail,
						Mod:  Keymod(evt.State),
					},
					State:     KeyDown,
					Timestamp: evt.Time,
				}
			case xproto.KeyReleaseEvent:
				c.Keys <- KeyEvent{
					Key: Key{
						Code: evt.Detail,
						Mod:  Keymod(evt.State),
					},
					State:     KeyUp,
					Timestamp: evt.Time,
				}
			}
		}
	}()
}

// LoopStop stops any active loop goroutine.
func (c *Client) LoopStop() {
	c.loop = false
}

// sendKey sends a synthetic keypress to the given window.
func (c *Client) sendKey(press KeyEvent, win xproto.Window) error {
	if press.State == KeyPress {
		newPress := press
		newPress.State = KeyDown

		if err := c.sendKey(newPress, win); err != nil {
			return err
		}

		newPress.State = KeyUp
		newPress.Timestamp += 1
		if err := c.sendKey(newPress, win); err != nil {
			return err
		}

		return nil
	}

	evt := xproto.KeyPressEvent{
		Sequence:   0,
		Detail:     press.Key.Code,
		Time:       press.Timestamp,
		Root:       win,
		Event:      win,
		Child:      win,
		RootX:      0,
		RootY:      0,
		EventX:     0,
		EventY:     0,
		SameScreen: true,
	}

	bytes := evt.Bytes()
	if press.State == KeyUp {
		bytes[0] = 3
	}

	reply := xproto.SendEventChecked(c.conn, true, win, xproto.EventMaskKeyPress, string(bytes))
	return reply.Check()
}

// SendKeyDown sends a keydown event with the given parameters.
func (c *Client) SendKeyDown(code xproto.Keycode, win xproto.Window, timestamp *xproto.Timestamp) error {
	evt := KeyEvent{
		Key: Key{
			Code: code,
		},
		State:     KeyDown,
		Timestamp: xproto.Timestamp(*timestamp),
	}

	// We only adjust the timestamp to deal with GLFW's timestamp checks (which
	// exist to work around buggy X behavior. What a surprise.)
	//
	// See:
	// https://github.com/glfw/glfw/blob/master/src/x11_window.c#L1218

	err := c.sendKey(evt, win)
	*timestamp += 1
	return err
}

// SendKeyUp sends a keyup event with the given parameters.
func (c *Client) SendKeyUp(code xproto.Keycode, win xproto.Window, timestamp *xproto.Timestamp) error {
	evt := KeyEvent{
		Key: Key{
			Code: code,
		},
		State:     KeyUp,
		Timestamp: xproto.Timestamp(*timestamp),
	}

	err := c.sendKey(evt, win)
	*timestamp += 1
	return err
}

// SendKeyPress sends a keydown and keyup event with the given parameters.
func (c *Client) SendKeyPress(code xproto.Keycode, win xproto.Window, timestamp *xproto.Timestamp) error {
	evt := KeyEvent{
		Key: Key{
			Code: code,
		},
		State:     KeyPress,
		Timestamp: xproto.Timestamp(*timestamp),
	}

	// These shenanigans warrant a bit of explaining for anyone who reads this
	// and wonders why an extraneous backslash is sent.
	//
	// GLFW will reject any key-up events which have the same keycode as the very
	// next X event and are within 20ms of that next event. We are sending key
	// events with 1ms timestamp differences, so sending lots of keypresses
	// quickly will trigger the check and drop inputs.
	//
	// Sending the backslash key event makes sure that the next event in the queue
	// has a different keycode, thus the check will not be triggered and key events
	// can be sent at a stupid rate.
	//
	// See:
	// https://github.com/glfw/glfw/blob/master/src/x11_window.c#L1295

	err := c.sendKey(evt, win)
	*timestamp += 2
	evt.Timestamp += 2

	evt.Key.Code = KeyBackslash
	_ = c.sendKey(evt, win)
	*timestamp += 2

	return err
}

// SetTitle sets the title for the given window.
func (c *Client) SetTitle(win xproto.Window, title string) error {
	const WM_NAME = "_NET_WM_NAME"
	wmName, err := xproto.InternAtom(c.conn, false, uint16(len(WM_NAME)), WM_NAME).Reply()
	if err != nil {
		return err
	}

	return xproto.ChangePropertyChecked(
		c.conn,
		xproto.PropertyNewValue,
		win,
		wmName.Atom,
		xproto.AtomString,
		8,
		uint32(len(title)),
		[]byte(title),
	).Check()
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

// UngrabKeyboard returns the keyboard to other X clients.
func (c *Client) UngrabKeyboard() {
	xproto.UngrabKeyboard(c.conn, xproto.TimeCurrentTime)
}
