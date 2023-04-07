// Package x11 provides a simple client for interacting with the X server to do
// things like sending input events.
package x11

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// Atom names
const (
	netActiveWindow   = "_NET_ACTIVE_WINDOW"
	netCurrentDesktop = "_NET_CURRENT_DESKTOP"
	netWmDesktop      = "_NET_WM_DESKTOP"
	netWmPid          = "_NET_WM_PID"
	utf8String        = "UTF8_STRING"
	wmClass           = "WM_CLASS"
	wmName            = "WM_NAME"
)

// Event masks
const (
	maskButton uint32 = xproto.EventMaskButtonPress |
		xproto.EventMaskButtonRelease

	maskEnterLeave uint32 = xproto.EventMaskEnterWindow |
		xproto.EventMaskLeaveWindow

	maskKeyPress uint32 = xproto.EventMaskKeyPress |
		xproto.EventMaskKeyRelease

	maskPointer uint16 = xproto.EventMaskPointerMotion |
		xproto.EventMaskButtonPress

	maskProperty uint32 = xproto.EventMaskPropertyChange |
		xproto.EventMaskSubstructureNotify

	maskSubstructure uint32 = xproto.EventMaskSubstructureNotify |
		xproto.EventMaskSubstructureRedirect

	maskWindow uint16 = xproto.ConfigWindowX |
		xproto.ConfigWindowY |
		xproto.ConfigWindowHeight |
		xproto.ConfigWindowWidth
)

// Error types
var (
	ErrConnectionDied = errors.New("connection with X server closed")
	errInvalidLength  = errors.New("invalid response length")
)

// Pointer grab error names
var pointerGrabErrors = []string{
	"Success",
	"Already grabbed",
	"Invalid time",
	"Not viewable",
	"Frozen",
}

// Client maintains a connection with the X server and performs tasks like
// sending fake inputs and receiving user input.
type Client struct {
	atoms atomCache     // Atom cache
	conn  *xgb.Conn     // The X server connection
	root  xproto.Window // Root window

	// The offset between the system clock and X server time, in milliseconds.
	timeOffset uint64

	// Information about the last key events sent for each window. This is used
	// to ensure that resetti's inputs don't get dropped by GLFW.
	lastKeyState map[xproto.Window]keyState
	mu           sync.Mutex
}

// keyState contains state about the last key event sent to a given window.
// This is used to ensure that resetti's inputs don't get dropped by GLFW.
type keyState struct {
	time uint32
	code xproto.Keycode
}

// rawEvent represents an event which is to be sent to another window.
type rawEvent interface {
	Bytes() []byte
}

// NewClient attempts to create a new Client.
func NewClient() (Client, error) {
	conn, err := xgb.NewConn()
	if err != nil {
		return Client{}, err
	}
	root := xproto.Setup(conn).DefaultScreen(conn).Root
	err = xproto.ChangeWindowAttributesChecked(
		conn,
		root,
		xproto.CwEventMask,
		[]uint32{maskProperty},
	).Check()
	if err != nil {
		return Client{}, err
	}
	offset, err := approximateOffset(conn)
	if err != nil {
		return Client{}, err
	}
	return Client{
		atomCache{
			conn: conn,
			data: make(map[string]xproto.Atom),
		},
		conn,
		root,
		offset,
		make(map[xproto.Window]keyState),
		sync.Mutex{},
	}, nil
}

// Click clicks the top left corner (0, 0) of the given window.
func (c *Client) Click(win xproto.Window) error {
	// Send an EnterNotify event to get GLFW to update the cursor position.
	// Then send a LeaveNotify to stop tracking cursor movement.
	// Then send a ButtonPress to click the window.
	//
	// Reference:
	// https://github.com/glfw/glfw/blob/3.3.8/src/x11_window.c#L1465
	evt := xproto.EnterNotifyEvent{
		Root:  win,
		Event: win,
		Child: win,
	}
	if err := c.sendEvent(evt, maskEnterLeave, win); err != nil {
		return err
	}
	evt2 := xproto.LeaveNotifyEvent(evt)
	if err := c.sendEvent(evt2, maskEnterLeave, win); err != nil {
		return err
	}
	evt3 := xproto.ButtonPressEvent{
		Detail: 1,
		Root:   win,
		Event:  win,
		Child:  win,
	}
	if err := c.sendEvent(evt3, maskButton, win); err != nil {
		return err
	}
	evt4 := xproto.ButtonReleaseEvent(evt3)
	return c.sendEvent(evt4, maskButton, win)
}

// FocusWindow activates the given window.
func (c *Client) FocusWindow(win xproto.Window) error {
	winDesktop, err := c.getPropertyInt(c.root, netWmDesktop, xproto.AtomCardinal)
	switch err {
	case errInvalidLength:
		break
	case nil:
		if err = c.setCurrentDesktop(winDesktop); err != nil {
			return fmt.Errorf("set current desktop: %w", err)
		}
	default:
		return fmt.Errorf("get window desktop: %w", err)
	}
	activeWindow, err := c.atoms.Get(netActiveWindow)
	if err != nil {
		return fmt.Errorf("get _NET_ACTIVE_WINDOW atom: %w", err)
	}
	data := make([]uint32, 5)
	data[0] = 1 // Source indicator (1 = application)
	evt := xproto.ClientMessageEvent{
		Format: 32,
		Window: win,
		Type:   activeWindow,
		Data:   xproto.ClientMessageDataUnionData32New(data),
	}
	return c.sendEvent(evt, maskSubstructure, c.root)
}

// GetActiveWindow returns the currently focused window.
func (c *Client) GetActiveWindow() (xproto.Window, error) {
	win, err := c.getPropertyInt(c.root, netActiveWindow, xproto.AtomWindow)
	if err != nil {
		// The _NET_ACTIVE_WINDOW property might not exist depending on the
		// window manager.
		if err == errInvalidLength {
			return 0, nil
		}
		return 0, err
	}
	return xproto.Window(win), nil
}

// GetCurrentTime returns the approximate current X server time.
func (c *Client) GetCurrentTime() uint32 {
	return uint32(time.Now().UnixMilli() - int64(c.timeOffset))
}

// GetRootWindow returns the ID of the root window.
func (c *Client) GetRootWindow() xproto.Window {
	return c.root
}

// GetWindowList returns a list of all open windows.
func (c *Client) GetWindowList() []xproto.Window {
	return c.GetWindowChildren(c.root)
}

// GetWindowChildren returns a list of all child windows (and their children,
// and so on) for the given window.
func (c *Client) GetWindowChildren(win xproto.Window) []xproto.Window {
	// Traverse the window tree in an iterative fashion.
	queue := []xproto.Window{win}
	for ptr := 0; ptr < len(queue); ptr += 1 {
		next := queue[ptr]
		tree, err := xproto.QueryTree(c.conn, next).Reply()

		// Windows may be closed while we traverse the tree. Ignore any errors
		// during the traversal.
		if err != nil {
			continue
		}
		queue = append(queue, tree.Children...)
	}
	return queue
}

// GetWindowClass returns the class of the given window.
func (c *Client) GetWindowClass(win xproto.Window) (string, error) {
	class, err := c.getPropertyString(win, wmClass)
	if err != nil {
		return "", err
	}
	return strings.Split(class, "\x00")[0], nil
}

// GetWindowPid returns the PID of the process that owns the given window.
func (c *Client) GetWindowPid(win xproto.Window) (uint32, error) {
	return c.getPropertyInt(win, netWmPid, xproto.AtomCardinal)
}

// GetWindowSize returns the size of the given window.
func (c *Client) GetWindowSize(win xproto.Window) (uint16, uint16, error) {
	// XXX: cache window size from poll loop?
	geo, err := xproto.GetGeometry(c.conn, xproto.Drawable(win)).Reply()
	if err != nil {
		return 0, 0, err
	}
	return geo.Width, geo.Height, nil
}

// GetWindowTitle returns the title of the given window.
func (c *Client) GetWindowTitle(win xproto.Window) (string, error) {
	return c.getPropertyString(win, wmName)
}

// GrabKey grabs a keyboard key from the given window, diverting all instances
// of that keypress for that window to resetti.
func (c *Client) GrabKey(key Key, win xproto.Window) error {
	// TODO: investigate grab window behavior
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

// GrabPointer grabs the mouse pointer, diverting all mouse events to resetti.
func (c *Client) GrabPointer(win xproto.Window) error {
	reply, err := xproto.GrabPointer(
		c.conn,
		true,
		win,
		maskPointer,
		xproto.GrabModeAsync,
		xproto.GrabModeAsync,
		c.root,
		xproto.CursorNone,
		xproto.TimeCurrentTime,
	).Reply()
	if err != nil {
		return err
	}
	if reply.Status == xproto.GrabStatusSuccess {
		return nil
	} else {
		return errors.New(pointerGrabErrors[reply.Status])
	}
}

// MoveWindow moves and resizes the given window.
func (c *Client) MoveWindow(win xproto.Window, x, y, w, h uint32) error {
	return xproto.ConfigureWindowChecked(
		c.conn,
		win,
		maskWindow,
		[]uint32{x, y, w, h},
	).Check()
}

// Poll starts listening for user input events in the background.
func (c *Client) Poll(ctx context.Context) (<-chan Event, <-chan error, error) {
	ch := make(chan Event, 256)
	errch := make(chan error, 8)
	go c.poll(ctx, ch, errch)
	return ch, errch, nil
}

// SendKeyDown sends a key down event to the given window with the given key.
func (c *Client) SendKeyDown(code xproto.Keycode, win xproto.Window) {
	c.sendKeyEvent(code, StateDown, win)
}

// SendKeyPress sends a key press (key down and key up event) to the given
// window with the given key.
func (c *Client) SendKeyPress(code xproto.Keycode, win xproto.Window) {
	c.sendKeyEvent(code, StateDown, win)
	c.sendKeyEvent(code, StateUp, win)
}

// SendKeyUp sends a key up event to the given window with the given key.
func (c *Client) SendKeyUp(code xproto.Keycode, win xproto.Window) {
	c.sendKeyEvent(code, StateUp, win)
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

// UngrabPointer releases any pointer grabs.
func (c *Client) UngrabPointer() error {
	return xproto.UngrabPointerChecked(c.conn, xproto.TimeCurrentTime).Check()
}

// getProperty retrieves a raw window property.
func (c *Client) getProperty(win xproto.Window, name string, typ xproto.Atom) ([]byte, error) {
	atom, err := c.atoms.Get(name)
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

// getPropertyInt retrieves a 32-bit window property.
func (c *Client) getPropertyInt(win xproto.Window, name string, typ xproto.Atom) (uint32, error) {
	reply, err := c.getProperty(win, name, typ)
	if err != nil {
		return 0, err
	}
	if len(reply) != 4 {
		return 0, errInvalidLength
	}
	return binary.LittleEndian.Uint32(reply), nil
}

// getPropertyString retrieves a string window property. The returned string
// may conatin null bytes.
func (c *Client) getPropertyString(win xproto.Window, name string) (string, error) {
	reply, err := c.getProperty(win, name, xproto.AtomString)
	if err != nil {
		return "", err
	}
	return string(reply), nil
}

// sendEvent sends an event to another window.
func (c *Client) sendEvent(evt rawEvent, mask uint32, win xproto.Window) error {
	return xproto.SendEventChecked(
		c.conn,
		true,
		win,
		mask,
		string(evt.Bytes()),
	).Check()
}

// sendKeyEvent sends a key event to the given window.
func (c *Client) sendKeyEvent(key xproto.Keycode, state InputState, win xproto.Window) {
	// Here, we have to deal with two hackfixes in GLFW.
	// The first is that key events must always have a timestamp greater than
	// the last event with the same keycode. So, we always increment, regardless
	// of keycode, just to keep things simpler.
	// The second is that a key release and key press event with the same code
	// can not occur directly after each other unless they have a timestamp
	// difference of >=20ms.
	//
	// So, we always ensure the timestamp we are sending will not cause the
	// event to get dropped by GLFW. Additionally, we always ensure that the
	// timestamp is a few (15, this is arbitrary) milliseconds ahead of the
	// *actual* X server time, so that inputs from the user never cause
	// resetti's inputs to get dropped.
	//
	// Reference:
	// https://github.com/glfw/glfw/blob/3.3.8/src/x11_window.c#L1260
	// https://github.com/glfw/glfw/blob/3.3.8/src/x11_window.c#L1359

	// XXX: Can lock contention be a problem here? Come back to this after
	// ctl package is implemented and either remove the mutex or figure out if
	// contention is a performance issue.
	c.mu.Lock()
	lastState, ok := c.lastKeyState[win]
	time := c.GetCurrentTime() + 15
	if ok {
		if lastState.time >= time {
			time = lastState.time + 1
		}
		if lastState.code == key {
			time = lastState.time + 20
		}
	}
	c.lastKeyState[win] = keyState{time, key}
	c.mu.Unlock()

	evt := xproto.KeyPressEvent{
		Detail:     key,
		Time:       xproto.Timestamp(time),
		Root:       win,
		Event:      win,
		Child:      win,
		SameScreen: true,
	}
	var err error
	if state == StateDown {
		err = c.sendEvent(evt, maskKeyPress, win)
	} else {
		err = c.sendEvent(xproto.KeyReleaseEvent(evt), maskKeyPress, win)
	}
	if err != nil {
		log.Printf("Failed to send key event: %s\n", err)
	}
}

// setCurrentDesktop attempts to upadte the current desktop by setting the
// _NET_CURRENT_DESKTOP property of the root window to the given desktop.
func (c *Client) setCurrentDesktop(desktop uint32) error {
	// Get the _NET_CURRENT_DESKTOP atom.
	currentDesktop, err := c.atoms.Get(netCurrentDesktop)
	if err != nil {
		return fmt.Errorf("get _NET_CURRENT_DESKTOP atom: %w", err)
	}

	// Send the property change event.
	data := make([]uint32, 5)
	data[0] = desktop
	evt := xproto.ClientMessageEvent{
		Format: 32,
		Window: c.root,
		Type:   currentDesktop,
		Data:   xproto.ClientMessageDataUnionData32New(data),
	}
	return c.sendEvent(evt, maskSubstructure, c.root)
}

// poll listens for user inputs in the background.
func (c *Client) poll(ctx context.Context, ch chan<- Event, errch chan<- error) {
	defer close(ch)
	defer close(errch)
	activeWindow, err := c.atoms.Get(netActiveWindow)
	if err != nil {
		errch <- err
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		evt, err := c.conn.WaitForEvent()
		if evt == nil && err == nil {
			errch <- ErrConnectionDied
			return
		}
		if err != nil {
			errch <- err
			continue
		}
		switch evt := evt.(type) {
		case xproto.KeyPressEvent:
			ch <- KeyEvent{
				Key:       Key{Code: evt.Detail, Mod: Keymod(evt.State)},
				State:     StateDown,
				Timestamp: uint32(evt.Time),
			}
		case xproto.KeyReleaseEvent:
			ch <- KeyEvent{
				Key:       Key{Code: evt.Detail, Mod: Keymod(evt.State)},
				State:     StateUp,
				Timestamp: uint32(evt.Time),
			}
		case xproto.ButtonPressEvent:
			ch <- ButtonEvent{
				Button:    evt.Detail,
				Mod:       Keymod(evt.State),
				Point:     Point{evt.EventX, evt.EventY},
				Timestamp: uint32(evt.Time),
				Window:    evt.Child,
			}
		case xproto.MotionNotifyEvent:
			ch <- MoveEvent{
				Mod:       Keymod(evt.State),
				Point:     Point{evt.EventX, evt.EventY},
				Timestamp: uint32(evt.Time),
				Window:    evt.Child,
			}
		case xproto.PropertyNotifyEvent:
			if activeWindow != evt.Atom {
				continue
			}
			win, err := c.GetActiveWindow()
			if err != nil {
				errch <- err
				continue
			}
			ch <- FocusEvent{uint32(evt.Time), win}
		}
	}
}

// approximateOffset attempts to find the offset between the system clock and
// the X server time.
func approximateOffset(c *xgb.Conn) (uint64, error) {
	reply, err := xproto.InternAtom(c, false, uint16(len(wmName)), wmName).Reply()
	if err != nil {
		return 0, fmt.Errorf("get WM_NAME atom: %w", err)
	}
	atom := reply.Atom

	// Try to get the time offset 10 times and take the average.
	offsetSum := uint64(0)
	root := xproto.Setup(c).DefaultScreen(c).Root
	for i := 0; i < 10; i += 1 {
		// Send a no-op property change request and take note of the timestamp
		// sent back by the X server. This method is recommended by the ICCCM
		// spec:
		// https://x.org/releases/X11R7.6/doc/xorg-docs/specs/ICCCM/icccm.html#acquiring_selection_ownership
		send := time.Now().UnixMilli()
		xproto.ChangeProperty(
			c,
			xproto.PropModeAppend,
			root,
			atom,
			xproto.AtomString,
			8,
			0,
			[]byte{},
		)
		rawEvt, err := c.WaitForEvent()
		if rawEvt == nil && err == nil {
			return 0, ErrConnectionDied
		} else if err != nil {
			return 0, fmt.Errorf("receive response: %w", err)
		}
		evt, ok := rawEvt.(xproto.PropertyNotifyEvent)
		if !ok {
			return 0, fmt.Errorf("invalid event type (%T)", rawEvt)
		}
		offsetSum += uint64(send - int64(evt.Time))
	}
	return offsetSum / 10, nil
}
