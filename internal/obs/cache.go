package obs

import (
	"sync"
)

// idCache is used to keep a cache of scene item IDs.
type idCache struct {
	mx    *sync.RWMutex
	cache map[[2]string]int
}

// newIdCache creates a new empty idCache.
func newIdCache() idCache {
	c := idCache{}
	c.mx = &sync.RWMutex{}
	c.cache = make(map[[2]string]int)
	return c
}

// Get returns the ID of the given scene/source pair if it exists.
func (i *idCache) Get(scene string, name string) (int, bool) {
	i.mx.RLock()
	id, ok := i.cache[[2]string{scene, name}]
	i.mx.RUnlock()
	return id, ok
}

// Set inserts the given scene/source pair into the cache.
func (i *idCache) Set(scene string, name string, id int) {
	i.mx.Lock()
	i.cache[[2]string{scene, name}] = id
	i.mx.Unlock()
}
