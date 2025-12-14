package store

import (
	"sync"
)

type IngressErrorMap struct {
	mu sync.RWMutex
	m  map[string]error
}

func NewIngressErrorMap() *IngressErrorMap {
	return &IngressErrorMap{
		m: make(map[string]error),
	}
}

// Set key and value to IngressErrorMap
func (i *IngressErrorMap) Set(key string, err error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.m[key] = err
}

// Get uses key to get value
func (i *IngressErrorMap) Get(key string) (error, bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	v, ok := i.m[key]
	return v, ok
}

// Delete removes a key from the map
func (i *IngressErrorMap) Delete(key string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	delete(i.m, key)
}
