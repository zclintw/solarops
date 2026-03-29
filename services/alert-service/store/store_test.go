package store

import (
    "testing"
    "github.com/solarops/shared/models"
)

func TestStoreCreateAndGet(t *testing.T) {
    s := New()
    alert := models.Alert{
        Type:      models.AlertPanelFault,
        PlantID:   "plant-1",
        PlantName: "Test Plant",
        PanelID:   "panel-1",
        Status:    models.AlertStatusActive,
        Message:   "Panel dead",
    }
    created := s.Create(alert)
    if created.ID == "" {
        t.Error("expected ID to be set")
    }
    got, ok := s.Get(created.ID)
    if !ok {
        t.Error("expected to find alert")
    }
    if got.PlantID != "plant-1" {
        t.Errorf("expected plant-1, got %s", got.PlantID)
    }
}

func TestStoreAcknowledge(t *testing.T) {
    s := New()
    alert := s.Create(models.Alert{
        Type:    models.AlertPanelFault,
        PlantID: "p1",
        Status:  models.AlertStatusActive,
    })
    s.Acknowledge(alert.ID)
    got, _ := s.Get(alert.ID)
    if got.Status != models.AlertStatusAcknowledged {
        t.Errorf("expected acknowledged, got %s", got.Status)
    }
}

func TestStoreResolve(t *testing.T) {
    s := New()
    alert := s.Create(models.Alert{
        Type:    models.AlertPanelFault,
        PlantID: "p1",
        PanelID: "panel-1",
        Status:  models.AlertStatusActive,
    })
    resolved := s.Resolve("p1", "panel-1", models.AlertPanelFault)
    if len(resolved) != 1 {
        t.Errorf("expected 1 resolved, got %d", len(resolved))
    }
    got, _ := s.Get(alert.ID)
    if got.Status != models.AlertStatusResolved {
        t.Errorf("expected resolved, got %s", got.Status)
    }
}

func TestStoreList(t *testing.T) {
    s := New()
    s.Create(models.Alert{Type: models.AlertPanelFault, PlantID: "p1", Status: models.AlertStatusActive})
    s.Create(models.Alert{Type: models.AlertDataGap, PlantID: "p2", Status: models.AlertStatusActive})
    all := s.List("")
    if len(all) != 2 {
        t.Errorf("expected 2, got %d", len(all))
    }
    active := s.List(models.AlertStatusActive)
    if len(active) != 2 {
        t.Errorf("expected 2 active, got %d", len(active))
    }
}

func TestStoreFindActive(t *testing.T) {
    s := New()
    s.Create(models.Alert{
        Type:    models.AlertPanelFault,
        PlantID: "p1",
        PanelID: "panel-1",
        Status:  models.AlertStatusActive,
    })
    found := s.FindActive("p1", "panel-1", models.AlertPanelFault)
    if found == nil {
        t.Error("expected to find active alert")
    }
    notFound := s.FindActive("p1", "panel-2", models.AlertPanelFault)
    if notFound != nil {
        t.Error("expected nil for non-existent alert")
    }
}
