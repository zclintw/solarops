package manager

import (
	"sync"
)

type PlantEntry struct {
	PlantID     string  `json:"plantId"`
	PlantName   string  `json:"plantName"`
	Panels      int     `json:"panels"`
	WattPerSec  float64 `json:"wattPerSec"`
	ContainerID string  `json:"containerId"`
}

type Registry struct {
	plants map[string]*PlantEntry
	mu     sync.RWMutex
}

func NewRegistry() *Registry {
	return &Registry{
		plants: make(map[string]*PlantEntry),
	}
}

func (r *Registry) Add(plantID, name string, panels int, wattPerSec float64, containerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plants[plantID] = &PlantEntry{
		PlantID:     plantID,
		PlantName:   name,
		Panels:      panels,
		WattPerSec:  wattPerSec,
		ContainerID: containerID,
	}
}

func (r *Registry) Remove(plantID string) (PlantEntry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.plants[plantID]
	if !ok {
		return PlantEntry{}, false
	}
	delete(r.plants, plantID)
	return *p, true
}

func (r *Registry) Get(plantID string) (PlantEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.plants[plantID]
	if !ok {
		return PlantEntry{}, false
	}
	return *p, true
}

func (r *Registry) NameExists(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.plants {
		if p.PlantName == name {
			return true
		}
	}
	return false
}

func (r *Registry) List() []PlantEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]PlantEntry, 0, len(r.plants))
	for _, p := range r.plants {
		result = append(result, *p)
	}
	return result
}
