package main

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

type Attributes struct {
	pid   uint32
	class []string
}

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

type Key struct {
	code xproto.Keycode
	mod  Keymod
}

type XClient struct {
	conn *xgb.Conn
	root xproto.Window
	keys []Key
	ch   chan Key
}

func NewXClient() (*XClient, error) {
	x, err := xgb.NewConn()
	if err != nil {
		return nil, err
	}

	root := xproto.Setup(x).DefaultScreen(x).Root
	ch := make(chan Key)

	client := XClient{
		conn: x,
		root: root,
		keys: []Key{},
		ch:   ch,
	}

	return &client, nil
}

func (c *XClient) GetProperty(win xproto.Window, atom xproto.Atom, atype xproto.Atom) ([]byte, error) {
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

func (c *XClient) GetPropertyInt(win xproto.Window, name string) (uint32, error) {
	reply, err := xproto.InternAtom(c.conn, false, uint16(len(name)), name).Reply()
	if err != nil {
		return 0, err
	}

	bytes, err := c.GetProperty(win, reply.Atom, xproto.AtomCardinal)
	if err != nil {
		return 0, err
	}

	return binary.LittleEndian.Uint32(bytes), nil
}

func (c *XClient) GetPropertyString(win xproto.Window, name string) ([]string, error) {
	reply, err := xproto.InternAtom(c.conn, false, uint16(len(name)), name).Reply()
	if err != nil {
		return nil, err
	}

	rawprop, err := c.GetProperty(win, reply.Atom, xproto.AtomString)
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

func (c *XClient) GetWindowAttributes(win xproto.Window) (*Attributes, error) {
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

func (c *XClient) GetWindowList(win xproto.Window) ([]xproto.Window, error) {
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

func (c *XClient) GrabKey(key Key) {
	xproto.GrabKey(c.conn, true, c.root, uint16(key.mod), key.code, xproto.GrabModeAsync, xproto.GrabModeAsync)
	c.keys = append(c.keys, key)
}

func (c *XClient) SendKey(press bool, win xproto.Window, key xproto.Keycode, mod Keymod, timestamp xproto.Timestamp) error {
	evt := xproto.KeyPressEvent{
		Sequence:   0,
		Detail:     key,
		Time:       timestamp,
		Root:       win,
		Event:      win,
		Child:      win,
		RootX:      0,
		RootY:      0,
		EventX:     0,
		EventY:     0,
		State:      uint16(mod),
		SameScreen: true,
	}

	if press {
		reply := xproto.SendEventChecked(c.conn, true, win, xproto.EventMaskKeyPress, string(evt.Bytes()))
		return reply.Check()
	} else {
		evt := xproto.KeyReleaseEvent(evt)
		reply := xproto.SendEventChecked(c.conn, true, win, xproto.EventMaskKeyRelease, string(evt.Bytes()))
		return reply.Check()
	}
}

func (c *XClient) UngrabKey(key Key) {
	xproto.UngrabKey(c.conn, key.code, c.root, uint16(key.mod))

	i := 0
	for _, v := range c.keys {
		if v != key {
			c.keys[i] = v
			i++
		}
	}

	c.keys = c.keys[:i]
}
