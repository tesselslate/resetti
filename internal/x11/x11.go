package x11

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// NewClient attempts to create a new X client.
func NewClient() (*Client, error) {
	xc, err := xgb.NewConn()
	if err != nil {
		return nil, err
	}
	return &Client{
		conn: xc,
		atoms: atomMap{
			atoms: make(map[string]xproto.Atom),
			mu:    &sync.RWMutex{},
		},
		root:        xproto.Setup(xc).DefaultScreen(xc).Root,
		polling:     false,
		stopPolling: make(chan struct{}),
	}, nil
}

// Get returns the atom with the given name if it has already been queried,
// otherwise it asks the X server for the atom and caches it.
func (a *atomMap) Get(c *Client, name string) (xproto.Atom, error) {
	// Check to see if this atom has already been queried for.
	a.mu.RLock()
	if atom, ok := a.atoms[name]; ok {
		a.mu.RUnlock()
		return atom, nil
	}
	a.mu.RUnlock()

	// Get the atom from the X server and cache it for future queries.
	a.mu.Lock()
	defer a.mu.Unlock()
	reply, err := xproto.InternAtom(c.conn, false, uint16(len(name)), name).Reply()
	if err != nil {
		return 0, err
	}
	a.atoms[name] = reply.Atom
	return reply.Atom, nil
}

// Click fakes a mouse click on the given window.
func (c *Client) Click(win xproto.Window) error {
	send := func(evt string) error {
		return xproto.SendEventChecked(
			c.conn,
			true,
			win,
			xproto.EventMaskButtonPress|xproto.EventMaskButtonRelease,
			evt,
		).Check()
	}
	evt := xproto.ButtonPressEvent{
		Sequence:   0,
		Detail:     1,
		Time:       xproto.TimeCurrentTime,
		Root:       win,
		Event:      win,
		Child:      win,
		RootX:      0,
		RootY:      0,
		EventX:     0,
		EventY:     0,
		SameScreen: true,
	}
	raw := evt.Bytes()
	if err := send(string(raw)); err != nil {
		return err
	}
	raw[0] = 5
	return send(string(raw))
}

// FocusWindow switches input focus to the given window. It does so by sending
// a message to the root window indicating that it should update the
// _NET_ACTIVE_WINDOW property.
// See: https://specifications.freedesktop.org/wm-spec/1.3/ar01s03.html
func (c *Client) FocusWindow(win xproto.Window) error {
	active_window, err := c.atoms.Get(c, "_NET_ACTIVE_WINDOW")
	if err != nil {
		return err
	}
	data := make([]uint32, 5)
	// Source indication (1 = application)
	data[0] = 1
	evt := xproto.ClientMessageEvent{
		Format: 32,
		Window: win,
		Type:   active_window,
		Data:   xproto.ClientMessageDataUnionData32New(data),
	}
	return xproto.SendEventChecked(
		c.conn,
		true,
		c.root,
		xproto.EventMaskSubstructureNotify|xproto.EventMaskSubstructureRedirect,
		string(evt.Bytes()),
	).Check()
}

// getProperty gets a property from a window and returns it in the form of a byte
// slice. See the getPropertyX functions.
func (c *Client) getProperty(win xproto.Window, name string, typ xproto.Atom) ([]byte, error) {
	atom, err := c.atoms.Get(c, name)
	if err != nil {
		return nil, err
	}
	reply, err := xproto.GetProperty(
		c.conn,
		false,
		win,
		atom,
		typ,
		0,
		1024,
	).Reply()
	if err != nil {
		return nil, err
	}
	return reply.Value, nil
}

// getPropertyInt gets an unsigned 32-bit integer property from a window.
func (c *Client) getPropertyInt(win xproto.Window, name string, typ xproto.Atom) (uint32, error) {
	reply, err := c.getProperty(win, name, typ)
	if err != nil {
		return 0, err
	}
	if len(reply) != 4 {
		return 0, fmt.Errorf("invalid response length %d", len(reply))
	}
	return binary.LittleEndian.Uint32(reply), nil
}

// getPropertyString gets a string property from a window. The string is
// returned as is and may contain null bytes.
func (c *Client) getPropertyString(win xproto.Window, name string) (string, error) {
	reply, err := c.getProperty(win, name, xproto.AtomString)
	if err != nil {
		return "", err
	}
	return string(reply), nil
}

// GetActiveWindow returns the currently focused window.
func (c *Client) GetActiveWindow() (xproto.Window, error) {
	win, err := c.getPropertyInt(c.root, "_NET_ACTIVE_WINDOW", xproto.AtomWindow)
	if err != nil {
		return 0, err
	}
	return xproto.Window(win), nil
}

// GetAllWindows returns a list of all windows.
func (c *Client) GetAllWindows() ([]xproto.Window, error) {
	// Traverse the window tree starting from the root window in an iterative fashion.
	queue := []xproto.Window{c.root}
	windows := make([]xproto.Window, 0)
	for len(queue) > 0 {
		win := queue[0]
		queue = queue[1:]
		windows = append(windows, win)
		reply, err := xproto.QueryTree(c.conn, win).Reply()
		// It's possible that a window is closed while we traverse the window
		// tree, so just continue querying the window tree incase of an error.
		if err != nil {
			continue
		}
		queue = append(queue, reply.Children...)
	}
	return windows, nil
}

// GetScreenSize returns the size of the monitor.
// TODO: Implement multi-monitor logic (xinerama?). For the time being,
// resetti does not work particularly well with more than one monitor.
func (c *Client) GetScreenSize() (uint16, uint16, error) {
	reply, err := xproto.GetGeometry(c.conn, xproto.Drawable(c.root)).Reply()
	if err != nil {
		return 0, 0, err
	}
	return reply.Width, reply.Height, nil
}

// GetWindowClass returns the WM_CLASS property of the given window.
func (c *Client) GetWindowClass(win xproto.Window) (string, error) {
	class, err := c.getPropertyString(win, "WM_CLASS")
	if err != nil {
		return "", err
	}
	// The WM_CLASS property consists of two null-separated values.
	// We take the first.
	return strings.Split(class, "\x00")[0], nil
}

// GetWindowPid returns the process ID of the given window.
func (c *Client) GetWindowPid(win xproto.Window) (uint32, error) {
	pid, err := c.getPropertyInt(win, "_NET_WM_PID", xproto.AtomCardinal)
	if err != nil {
		return 0, err
	}
	return pid, nil
}

// GetWindowTitle returns the title of the given window.
func (c *Client) GetWindowTitle(win xproto.Window) (string, error) {
	title, err := c.getPropertyString(win, "WM_NAME")
	if err != nil {
		return "", err
	}
	return title, nil
}

// GetWmName returns the name of the window manager, if available.
func (c *Client) GetWmName() string {
	supporting, err := c.getPropertyInt(c.root, "_NET_SUPPORTING_WM_CHECK", xproto.AtomWindow)
	if err != nil {
		supporting, err = c.getPropertyInt(c.root, "_NET_SUPPORTING_WM_CHECK", xproto.AtomCardinal)
		if err != nil {
			return "failed _NET_SUPPORTING_WM_CHECK"
		}
	}
	var nameUtf8 string
	utf8, err := c.atoms.Get(c, "UTF8_STRING")
	if err == nil {
		rawName, err := c.getProperty(
			xproto.Window(supporting),
			"_NET_WM_NAME",
			utf8,
		)
		if err == nil {
			nameUtf8 = string(rawName)
		}
	}
	rawName, _ := c.getProperty(
		xproto.Window(supporting),
		"_NET_WM_NAME",
		xproto.AtomString,
	)
	return string(rawName) + " | " + nameUtf8
}

// GetWmSupported returns a prettified list of the window manager's
// _NET_SUPPORTED variable.
func (c *Client) GetWmSupported() string {
	raw, err := c.getProperty(c.root, "_NET_SUPPORTED", xproto.AtomAtom)
	if err != nil {
		return "failed _NET_SUPPORTED"
	}
	supported := make([]string, 0)
	for i := 0; i < len(raw); i += 4 {
		reply, err := xproto.GetAtomName(
			c.conn,
			xproto.Atom(binary.LittleEndian.Uint32(raw[i:i+4])),
		).Reply()
		if err != nil {
			continue
		}
		supported = append(supported, reply.Name)
	}
	return strings.Join(supported, ", ")
}

// GrabKey grabs a keyboard key from a window, diverting keypress events
// to resetti.
func (c *Client) GrabKey(key Key, win xproto.Window) error {
	return xproto.GrabKeyChecked(
		c.conn,
		true,
		win,
		uint16(key.Mod),
		key.Code,
		xproto.GrabModeAsync,
		xproto.GrabModeAsync,
	).Check()
}

// GrabPointer grabs the mouse pointer from the given window.
func (c *Client) GrabPointer(win xproto.Window) error {
	_, err := xproto.GrabPointer(
		c.conn,
		true,
		win,
		xproto.EventMaskPointerMotion|xproto.EventMaskButtonPress,
		xproto.GrabModeAsync,
		xproto.GrabModeAsync,
		c.root,
		xproto.CursorNone,
		xproto.TimeCurrentTime,
	).Reply()
	return err
}

// MoveWindow moves and resizes the given window.
func (c *Client) MoveWindow(win xproto.Window, x, y, w, h uint32) error {
	return xproto.ConfigureWindowChecked(
		c.conn,
		win,
		xproto.ConfigWindowX|
			xproto.ConfigWindowY|
			xproto.ConfigWindowWidth|
			xproto.ConfigWindowHeight,
		[]uint32{x, y, w, h},
	).Check()
}

// RootWindow returns the ID of the root window.
func (c *Client) RootWindow() xproto.Window {
	return c.root
}

// sendKey sends a key event with the given parameters.
func (c *Client) sendKey(state bool, code xproto.Keycode, win xproto.Window, timestamp xproto.Timestamp) error {
	evt := xproto.KeyPressEvent{
		Sequence:   0,
		Detail:     code,
		Time:       timestamp,
		Root:       win,
		Event:      win,
		Child:      win,
		RootX:      0,
		RootY:      0,
		EventX:     0,
		EventY:     0,
		SameScreen: true,
	}
	// To send it as an event, convert the event to its byte form first.
	// Additionally, if it is a key release, set the first byte (signifying the
	// event type) to 3.
	raw := evt.Bytes()
	if !state {
		raw[0] = 3
	}
	return xproto.SendEventChecked(
		c.conn,
		true,
		win,
		xproto.EventMaskKeyPress|xproto.EventMaskKeyRelease,
		string(raw),
	).Check()
}

// SendKeyDown sends a key-down event with the given parameters. It increments
// the timestamp parameter by one to deal with a quirk of how GLFW handles key
// events.
//
// See:
// https://github.com/glfw/glfw/blob/c18851f52ec9704eb06464058a600845ec1eada1/src/x11_window.c#L1250
func (c *Client) SendKeyDown(code xproto.Keycode, win xproto.Window, timestamp *xproto.Timestamp) error {
	err := c.sendKey(true, code, win, *timestamp)
	*timestamp += 1
	return err
}

// SendKeyUp sends a key-up event with the given parameters.
//
// Unlike SendKeyDown, it does *not* increment the timestamp parameter as
// GLFW's input handling does not require the timestamp to increase on key
// release events.
func (c *Client) SendKeyUp(code xproto.Keycode, win xproto.Window, timestamp *xproto.Timestamp) error {
	err := c.sendKey(false, code, win, *timestamp)
	return err
}

// SendKeyPress sends a key-down and key-up event with the given parameters.
// It increments the timestamp parameter by one to deal with a quirk of how
// GLFW handles key events (as described in SendKeyDown.)
//
// In addition to the timestamp difference check, repeated presses of the same
// key must circumvent another check - the timestamp must have at least a 20ms
// difference. In cases where this is necessary, send an alternating sequence
// of the key you want to press and a dummy key (such as Control) that will
// trick GLFW.
//
// See:
// https://github.com/glfw/glfw/blob/c18851f52ec9704eb06464058a600845ec1eada1/src/x11_window.c#L1321
func (c *Client) SendKeyPress(code xproto.Keycode, win xproto.Window, timestamp *xproto.Timestamp) error {
	err := c.sendKey(true, code, win, *timestamp)
	*timestamp += 1
	if err != nil {
		return err
	}
	return c.sendKey(false, code, win, *timestamp)
}

// UngrabKey releases a grabbed key and returns it back to the X server.
func (c *Client) UngrabKey(key Key, win xproto.Window) error {
	return xproto.UngrabKeyChecked(
		c.conn,
		key.Code,
		win,
		uint16(key.Mod),
	).Check()
}

// UngrabPointer returns the mouse pointer to the X server.
func (c *Client) UngrabPointer() error {
	return xproto.UngrabPointerChecked(c.conn, xproto.TimeCurrentTime).Check()
}

func (k *Key) UnmarshalTOML(value interface{}) error {
	str, ok := value.(string)
	if !ok {
		return errors.New("not a string")
	}
	substrs := strings.Split(str, "-")
	for _, s := range substrs {
		if val, ok := keys[strings.ToLower(s)]; ok {
			k.Code = val
		} else if val, ok := mods[strings.ToLower(s)]; ok {
			k.Mod |= val
		} else if strings.HasPrefix(strings.ToLower(s), "code") {
			num, err := strconv.Atoi(s[4:])
			if err != nil {
				return fmt.Errorf("invalid key component: %s", s)
			}
			if num > 255 || num < 0 {
				return fmt.Errorf("invalid key code: %d", num)
			}
			k.Code = xproto.Keycode(num)
		} else {
			return fmt.Errorf("invalid key component: %s", s)
		}
	}
	return nil
}

func (m *Keymod) UnmarshalTOML(value interface{}) error {
	str, ok := value.(string)
	if !ok {
		return errors.New("not a string")
	}
	substrs := strings.Split(str, "-")
	for _, s := range substrs {
		if s == "" {
			return nil
		}
		if val, ok := mods[strings.ToLower(s)]; ok {
			*m |= val
		} else {
			return fmt.Errorf("invalid key component: %s", s)
		}
	}
	return nil
}

func (e ButtonEvent) Timestamp() xproto.Timestamp {
	return e.Time
}

func (e KeyEvent) Timestamp() xproto.Timestamp {
	return e.Time
}

func (e MoveEvent) Timestamp() xproto.Timestamp {
	return e.Time
}
