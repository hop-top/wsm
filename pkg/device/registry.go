package device

import (
	"errors"
	"sort"
	"sync"
)

// Registry tracks known devices in memory.
type Registry struct {
	mu      sync.RWMutex
	devices map[string]*Device
}

func NewRegistry() *Registry {
	return &Registry{devices: make(map[string]*Device)}
}

func (r *Registry) Register(d Device) error {
	if d.ID == "" {
		return errors.New("device id is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.devices[d.ID]; ok {
		first := existing.FirstSeen
		*existing = d
		existing.FirstSeen = first
		return nil
	}
	copy := d
	r.devices[d.ID] = &copy
	return nil
}

func (r *Registry) Get(id string) (*Device, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.devices[id]
	if !ok {
		return nil, errors.New("device not found")
	}
	copy := *d
	return &copy, nil
}

func (r *Registry) List() []Device {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Device, 0, len(r.devices))
	for _, d := range r.devices {
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
