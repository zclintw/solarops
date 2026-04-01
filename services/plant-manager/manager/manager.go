package manager

import (
	"sync"
)

type PlantEntry struct {
	PlantID    string  `json:"plantId"`
	PlantName  string  `json:"plantName"`
	Panels     int     `json:"panels"`
	WattPerSec float64 `json:"wattPerSec"`
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

func (r *Registry) Add(plantID, name string, panels int, wattPerSec float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plants[plantID] = &PlantEntry{
		PlantID:    plantID,
		PlantName:  name,
		Panels:     panels,
		WattPerSec: wattPerSec,
	}
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

func (r *Registry) List() []PlantEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]PlantEntry, 0, len(r.plants))
	for _, p := range r.plants {
		result = append(result, *p)
	}
	return result
}
