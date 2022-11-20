package obs

import (
	"sync"
)

type idCache struct {
	mx    *sync.RWMutex
	cache map[[2]string]int
}

func newIdCache() idCache {
	c := idCache{}
	c.mx = &sync.RWMutex{}
	c.cache = make(map[[2]string]int)
	return c
}

func (i *idCache) Get(scene string, name string) (int, bool) {
	i.mx.RLock()
	id, ok := i.cache[[2]string{scene, name}]
	i.mx.RUnlock()
	return id, ok
}

func (i *idCache) Set(scene string, name string, id int) {
	i.mx.Lock()
	i.cache[[2]string{scene, name}] = id
	i.mx.Unlock()
}
