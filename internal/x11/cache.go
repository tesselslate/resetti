package x11

import (
	"sync"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// atomCache maintains a mapping of strings to X11 atoms to avoid re-requesting
// atoms from the X server repeatedly.
type atomCache struct {
	conn *xgb.Conn
	data map[string]xproto.Atom
	mx   sync.RWMutex
}

// Get returns the atom with the associated name.
func (c *atomCache) Get(name string) (xproto.Atom, error) {
	// Try to retrieve the atom from the cache.
	c.mx.RLock()
	if atom, ok := c.data[name]; ok {
		c.mx.RUnlock()
		return atom, nil
	}
	c.mx.RUnlock()

	// Request the atom from the X server.
	reply, err := xproto.InternAtom(c.conn, false, uint16(len(name)), name).Reply()
	if err != nil {
		return 0, err
	}
	c.mx.Lock()
	defer c.mx.Unlock()
	c.data[name] = reply.Atom
	return reply.Atom, nil
}
