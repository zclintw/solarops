package store

import (
    "sync"
    "time"

    "github.com/google/uuid"
    "github.com/solarops/shared/models"
)

type Store struct {
    alerts map[string]*models.Alert
    mu     sync.RWMutex
}

func New() *Store {
    return &Store{alerts: make(map[string]*models.Alert)}
}

func (s *Store) Create(alert models.Alert) models.Alert {
    s.mu.Lock()
    defer s.mu.Unlock()
    alert.ID = uuid.New().String()
    alert.CreatedAt = time.Now().UTC()
    alert.UpdatedAt = alert.CreatedAt
    s.alerts[alert.ID] = &alert
    return alert
}

func (s *Store) Get(id string) (models.Alert, bool) {
    s.mu.RLock()
    defer s.mu.RUnlock()
    a, ok := s.alerts[id]
    if !ok {
        return models.Alert{}, false
    }
    return *a, true
}

func (s *Store) Acknowledge(id string) bool {
    s.mu.Lock()
    defer s.mu.Unlock()
    a, ok := s.alerts[id]
    if !ok {
        return false
    }
    a.Status = models.AlertStatusAcknowledged
    a.UpdatedAt = time.Now().UTC()
    return true
}

func (s *Store) Resolve(plantID, panelID, alertType string) []models.Alert {
    s.mu.Lock()
    defer s.mu.Unlock()
    var resolved []models.Alert
    for _, a := range s.alerts {
        if a.PlantID == plantID && a.PanelID == panelID && a.Type == alertType &&
            a.Status != models.AlertStatusResolved {
            a.Status = models.AlertStatusResolved
            a.UpdatedAt = time.Now().UTC()
            resolved = append(resolved, *a)
        }
    }
    return resolved
}

func (s *Store) FindActive(plantID, panelID, alertType string) (models.Alert, bool) {
    s.mu.RLock()
    defer s.mu.RUnlock()
    for _, a := range s.alerts {
        if a.PlantID == plantID && a.PanelID == panelID && a.Type == alertType &&
            (a.Status == models.AlertStatusActive || a.Status == models.AlertStatusAcknowledged) {
            return *a, true
        }
    }
    return models.Alert{}, false
}

func (s *Store) List(statusFilter string) []models.Alert {
    s.mu.RLock()
    defer s.mu.RUnlock()
    result := make([]models.Alert, 0)
    for _, a := range s.alerts {
        if statusFilter == "" || a.Status == statusFilter {
            result = append(result, *a)
        }
    }
    return result
}
