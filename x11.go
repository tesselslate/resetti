package main

import (
	"encoding/binary"
	"fmt"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

type Attributes struct {
	pid   uint32
	class string
}

type Key struct {
	code xproto.Keycode
	mod  uint16
}

type XClient struct {
	x    *xgb.Conn
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
		x:    x,
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
		reply, err := xproto.GetProperty(c.x, false, win, atom, atype, offset, 8).Reply()
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
	reply, err := xproto.InternAtom(c.x, false, uint16(len(name)), name).Reply()
	if err != nil {
		return 0, err
	}

	bytes, err := c.GetProperty(win, reply.Atom, xproto.AtomCardinal)
	if err != nil {
		return 0, err
	}

	return binary.LittleEndian.Uint32(bytes), nil
}

func (c *XClient) GetPropertyString(win xproto.Window, name string) (string, error) {
	reply, err := xproto.InternAtom(c.x, false, uint16(len(name)), name).Reply()
	if err != nil {
		return "", err
	}

	bytes, err := c.GetProperty(win, reply.Atom, xproto.AtomString)
	if err != nil {
		return "", err
	}

	return string(bytes), nil
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
	reply, err := xproto.QueryTree(c.x, win).Reply()
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
	xproto.GrabKey(c.x, true, c.root, key.mod, key.code, xproto.GrabModeAsync, xproto.GrabModeAsync)
	c.keys = append(c.keys, key)
}

func (c *XClient) SendKey(press bool, win xproto.Window, key xproto.Keycode, mod uint16, timestamp xproto.Timestamp) error {
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
		State:      mod,
		SameScreen: true,
	}

	if press {
		reply := xproto.SendEventChecked(c.x, true, win, xproto.EventMaskKeyPress, evt.String())
		return reply.Check()
	} else {
		evt := xproto.KeyReleaseEvent(evt)
		reply := xproto.SendEventChecked(c.x, true, win, xproto.EventMaskKeyRelease, evt.String())
		return reply.Check()
	}
}

func (c *XClient) UngrabKey(key Key) {
	xproto.UngrabKey(c.x, key.code, c.root, key.mod)

	i := 0
	for _, v := range c.keys {
		if v != key {
			c.keys[i] = v
			i++
		}
	}

	c.keys = c.keys[:i]
}
