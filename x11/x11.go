// Package x11 implements an X11 client which is used for sending
// synthetic key events and managing Minecraft instances.
package x11

import (
	"encoding/binary"
	"errors"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

var atoms map[string]xproto.Atom
var conn *xgb.Conn
var grabbedKeys map[Key]xproto.Window
var rootWindow xproto.Window
var errCh chan<- error
var evtCh chan<- any
var done chan struct{}

const (
	key_mask     = xproto.EventMaskKeyPress | xproto.EventMaskKeyRelease
	pointer_mask = xproto.EventMaskPointerMotion | xproto.EventMaskButtonPress
	window_mask  = xproto.ConfigWindowX | xproto.ConfigWindowY |
		xproto.ConfigWindowWidth | xproto.ConfigWindowHeight
)

func Initialize() error {
	if conn != nil {
		return errors.New("already initialized")
	}
	xc, err := xgb.NewConn()
	if err != nil {
		return err
	}
	atoms = make(map[string]xproto.Atom)
	conn = xc
	grabbedKeys = make(map[Key]xproto.Window)
	rootWindow = xproto.Setup(conn).DefaultScreen(conn).Root
	done = make(chan struct{})
	go listenForEvents()
	return nil
}

func Close() {
	conn.Close()
	<-done
}

func Subscribe(err chan<- error, evt chan<- any) {
	if err != nil {
		errCh = err
	}
	if evt != nil {
		evtCh = evt
	}
}

func FocusWindow(win xproto.Window) error {
	atom, err := getAtom("_NET_ACTIVE_WINDOW")
	if err != nil {
		return err
	}
	geo, err := xproto.GetGeometry(conn, xproto.Drawable(win)).Reply()
	if err != nil {
		return err
	}
	data := make([]uint32, 5)
	data[0] = 2
	data[1] = 0
	evt := xproto.ClientMessageEvent{
		Format: 32,
		Window: win,
		Type:   atom,
		Data:   xproto.ClientMessageDataUnionData32New(data),
	}
	err = xproto.SendEventChecked(
		conn,
		true,
		geo.Root,
		xproto.EventMaskSubstructureNotify|xproto.EventMaskSubstructureRedirect,
		string(evt.Bytes()),
	).Check()
	return err
}

func GetActiveWindow() (xproto.Window, error) {
	winBytes, err := getProperty(rootWindow, "_NET_ACTIVE_WINDOW", xproto.AtomWindow)
	if err != nil {
		return 0, err
	}
	if len(winBytes) == 0 {
		return 0, errors.New("no response")
	}
	return xproto.Window(binary.LittleEndian.Uint32(winBytes)), nil
}

func GetAllWindows() ([]xproto.Window, error) {
	queue := []xproto.Window{rootWindow}
	windows := make([]xproto.Window, 0)
	for len(queue) > 0 {
		win := queue[0]
		queue = queue[1:]
		res, err := xproto.QueryTree(conn, win).Reply()
		if err != nil {
			return nil, err
		}
		windows = append(windows, win)
		queue = append(queue, res.Children...)
	}
	return windows, nil
}

func GetWindowClass(win xproto.Window) (string, error) {
	class, err := getPropertyString(win, "WM_CLASS")
	if err != nil {
		return "", err
	}
	return class[0], nil
}

func GetWindowPid(win xproto.Window) (uint32, error) {
	return getPropertyInt(win, "_NET_WM_PID")
}

func GetWindowTitle(win xproto.Window) (string, error) {
	title, err := getPropertyString(win, "WM_NAME")
	if err != nil {
		return "", err
	}
	return title[0], nil
}

func GrabKey(key Key, win xproto.Window) error {
	if _, ok := grabbedKeys[key]; ok {
		return errors.New("already grabbed key")
	}
	if win == 0 {
		win = rootWindow
	}
	grabbedKeys[key] = win
	return xproto.GrabKeyChecked(
		conn,
		true,
		win,
		uint16(key.Mod),
		key.Code,
		xproto.GrabModeAsync,
		xproto.GrabModeAsync,
	).Check()
}

func GrabKeyboard(win xproto.Window) error {
	if win == 0 {
		win = rootWindow
	}
	_, err := xproto.GrabKeyboard(
		conn,
		true,
		win,
		xproto.TimeCurrentTime,
		xproto.GrabModeAsync,
		xproto.GrabModeAsync,
	).Reply()
	return err
}

func GrabPointer(win xproto.Window) error {
	if win == 0 {
		win = rootWindow
	}
	_, err := xproto.GrabPointer(
		conn,
		true,
		win,
		pointer_mask,
		xproto.GrabModeAsync,
		xproto.GrabModeAsync,
		win,
		xproto.CursorNone,
		xproto.TimeCurrentTime,
	).Reply()
	return err
}

func MoveWindow(win xproto.Window, x, y, w, h uint32) error {
	return xproto.ConfigureWindowChecked(
		conn,
		win,
		window_mask,
		[]uint32{x, y, w, h},
	).Check()
}

func ScreenSize() (uint16, uint16, error) {
	res, err := xproto.GetGeometry(conn, xproto.Drawable(rootWindow)).Reply()
	if err != nil {
		return 0, 0, err
	}
	return res.Width, res.Height, nil
}

func SendKeyDown(code xproto.Keycode, win xproto.Window, timestamp *xproto.Timestamp) error {
	// We accept a pointer to the timestamp and modify it here. We could return
	// the modified timestamp instead, but this is easier when sending lots of keys.
	//
	// Adjusting the timestamp is necessary due to GLFW's checks to deal with
	// X's weird behavior for sending "key hold" events.
	//
	// See:
	// https://github.com/glfw/glfw/blob/master/src/x11_window.c#L1218
	err := sendKey(code, true, *timestamp, win)
	*timestamp += 1
	return err
}

func SendKeyPress(code xproto.Keycode, win xproto.Window, timestamp *xproto.Timestamp) error {
	// See SendKeyDown for the reason why `timestamp` is incremented.
	err := sendKey(code, true, *timestamp, win)
	*timestamp += 1
	if err != nil {
		return err
	}
	err = sendKey(code, false, *timestamp, win)
	*timestamp += 1
	return err
}

func SendKeyPressAlt(code xproto.Keycode, win xproto.Window, timestamp *xproto.Timestamp) error {
	// This method exists for cases where resetti may need to send the same key
	// multiple times in a row. GLFW will reject such repeated key events when
	// they occur too quickly, so here we send an extraneous Control press to
	// ensure that all key events are processed properly.
	//
	// See:
	// https://github.com/glfw/glfw/blob/master/src/x11_window.c#L1295
	err := SendKeyPress(code, win, timestamp)
	if err != nil {
		return err
	}
	return SendKeyPress(KeyCtrl, win, timestamp)
}

func SendKeyUp(code xproto.Keycode, win xproto.Window, timestamp *xproto.Timestamp) error {
	// See SendKeyDown for the reason why `timestamp` is incremented.
	err := sendKey(code, false, *timestamp, win)
	*timestamp += 1
	return err
}

func UngrabKey(key Key) error {
	win, ok := grabbedKeys[key]
	if !ok {
		return errors.New("key not grabbed")
	}
	delete(grabbedKeys, key)
	return xproto.UngrabKeyChecked(
		conn,
		key.Code,
		win,
		uint16(key.Mod),
	).Check()
}

func UngrabKeyboard() error {
	return xproto.UngrabKeyboardChecked(conn, xproto.TimeCurrentTime).Check()
}

func UngrabPointer() error {
	return xproto.UngrabPointerChecked(conn, xproto.TimeCurrentTime).Check()
}
