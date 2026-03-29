package detector

import (
    "testing"
    "time"
)

func TestThresholdDetector_Dead(t *testing.T) {
    d := NewDetector(3, 30.0, 5)
    for i := 0; i < 3; i++ {
        d.Feed("plant-1", "panel-1", 1, "Plant 1", 0.0, time.Now())
    }
    alerts := d.Check()
    found := false
    for _, a := range alerts {
        if a.Type == "PANEL_FAULT" && a.PanelID == "panel-1" {
            found = true
        }
    }
    if !found {
        t.Error("expected PANEL_FAULT alert for dead panel")
    }
}

func TestThresholdDetector_NormalNoAlert(t *testing.T) {
    d := NewDetector(3, 30.0, 5)
    for i := 0; i < 5; i++ {
        d.Feed("plant-1", "panel-1", 1, "Plant 1", 300.0, time.Now())
    }
    alerts := d.Check()
    for _, a := range alerts {
        if a.PanelID == "panel-1" {
            t.Errorf("expected no alert for normal panel, got %s", a.Type)
        }
    }
}

func TestDegradedDetector(t *testing.T) {
    d := NewDetector(3, 30.0, 5)
    watts := []float64{300, 250, 200, 150}
    for _, w := range watts {
        d.Feed("plant-1", "panel-1", 1, "Plant 1", w, time.Now())
    }
    alerts := d.Check()
    found := false
    for _, a := range alerts {
        if a.Type == "PANEL_DEGRADED" {
            found = true
        }
    }
    if !found {
        t.Error("expected PANEL_DEGRADED alert")
    }
}

func TestIntermittentDetector(t *testing.T) {
    d := NewDetector(3, 30.0, 5)
    for i := 0; i < 10; i++ {
        w := 300.0
        if i%2 == 0 {
            w = 0
        }
        d.Feed("plant-1", "panel-1", 1, "Plant 1", w, time.Now())
    }
    alerts := d.Check()
    found := false
    for _, a := range alerts {
        if a.Type == "PANEL_UNSTABLE" {
            found = true
        }
    }
    if !found {
        t.Error("expected PANEL_UNSTABLE alert")
    }
}
