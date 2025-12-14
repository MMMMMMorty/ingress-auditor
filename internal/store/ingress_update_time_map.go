package store

import (
	"sync"
	"time"
)

type IngressUpdateTimeMap struct {
	mu sync.RWMutex
	m  map[string]time.Time
}

func NewIngressUpdateTimeMap() *IngressUpdateTimeMap {
	return &IngressUpdateTimeMap{
		m: make(map[string]time.Time),
	}
}

// Set key and value to IngressUpdateTimeMap
func (i *IngressUpdateTimeMap) Set(key string, updateTime time.Time) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.m[key] = updateTime
}

// Get uses key to get value
func (i *IngressUpdateTimeMap) Get(key string) (time.Time, bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	v, ok := i.m[key]
	return v, ok
}
