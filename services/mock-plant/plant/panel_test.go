package plant

import (
    "testing"

    "github.com/solarops/shared/models"
)

func TestNewPanel(t *testing.T) {
    p := NewPanel(1, 300.0)
    if p.Number != 1 {
        t.Errorf("expected number 1, got %d", p.Number)
    }
    if p.WattPerSec != 300.0 {
        t.Errorf("expected watt 300, got %f", p.WattPerSec)
    }
    if p.Status != models.StatusOnline {
        t.Errorf("expected online, got %s", p.Status)
    }
}

func TestPanelGenerate_Online(t *testing.T) {
    p := NewPanel(1, 300.0)
    data := p.Generate()
    if data.Watt != 300.0 {
        t.Errorf("expected 300W, got %f", data.Watt)
    }
    if data.Status != models.StatusOnline {
        t.Errorf("expected online, got %s", data.Status)
    }
}

func TestPanelGenerate_Offline(t *testing.T) {
    p := NewPanel(1, 300.0)
    p.SetOffline()
    data := p.Generate()
    if data.Watt != 0 {
        t.Errorf("expected 0W when offline, got %f", data.Watt)
    }
    if data.Status != models.StatusOffline {
        t.Errorf("expected offline, got %s", data.Status)
    }
}

func TestPanelFault_Dead(t *testing.T) {
    p := NewPanel(1, 300.0)
    p.SetFault(models.FaultDead)
    data := p.Generate()
    if data.Watt != 0 {
        t.Errorf("expected 0W for DEAD, got %f", data.Watt)
    }
    if data.FaultMode != models.FaultDead {
        t.Errorf("expected DEAD fault, got %s", data.FaultMode)
    }
}

func TestPanelFault_Degraded(t *testing.T) {
    p := NewPanel(1, 300.0)
    p.SetFault(models.FaultDegraded)

    prev := p.Generate().Watt
    for i := 0; i < 5; i++ {
        data := p.Generate()
        if data.Watt >= prev {
            t.Errorf("degraded panel should decrease: prev=%f, now=%f", prev, data.Watt)
        }
        prev = data.Watt
    }
}

func TestPanelFault_Intermittent(t *testing.T) {
    p := NewPanel(1, 300.0)
    p.SetFault(models.FaultIntermittent)

    zeroCount := 0
    normalCount := 0
    for i := 0; i < 100; i++ {
        data := p.Generate()
        if data.Watt == 0 {
            zeroCount++
        } else {
            normalCount++
        }
    }
    // Should have a mix of both
    if zeroCount == 0 || normalCount == 0 {
        t.Errorf("intermittent should produce both zero and normal: zero=%d, normal=%d", zeroCount, normalCount)
    }
}

func TestPanelReset(t *testing.T) {
    p := NewPanel(1, 300.0)
    p.SetFault(models.FaultDead)
    p.SetOffline()
    p.Reset()

    if p.Status != models.StatusOnline {
        t.Errorf("expected online after reset, got %s", p.Status)
    }
    data := p.Generate()
    if data.Watt != 300.0 {
        t.Errorf("expected 300W after reset, got %f", data.Watt)
    }
    if data.FaultMode != models.FaultNone {
        t.Errorf("expected no fault after reset, got %s", data.FaultMode)
    }
}
