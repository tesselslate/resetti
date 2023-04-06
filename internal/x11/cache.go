package x11

import (
	"sync"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// atomCache maintains a mapping of strings to X11 atoms to avoid re-requesting
// atoms from the X server repeatedly.
type atomCache struct {
	mu sync.RWMutex

	conn *xgb.Conn
	data map[string]xproto.Atom
}

// Get returns the atom with the associated name.
func (c *atomCache) Get(name string) (xproto.Atom, error) {
	// Try to retrieve the atom from the cache.
	c.mu.RLock()
	if atom, ok := c.data[name]; ok {
		c.mu.RUnlock()
		return atom, nil
	}
	c.mu.RUnlock()

	// Request the atom from the X server.
	reply, err := xproto.InternAtom(c.conn, false, uint16(len(name)), name).Reply()
	if err != nil {
		return 0, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[name] = reply.Atom
	return reply.Atom, nil
}
