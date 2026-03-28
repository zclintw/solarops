package plant

import (
    "math/rand"
    "sync"

    "github.com/google/uuid"
    "github.com/solarops/shared/models"
)

type Panel struct {
    ID          string
    Number      int
    WattPerSec  float64
    Status      string
    FaultMode   string
    currentWatt float64
    mu          sync.RWMutex
}

func NewPanel(number int, wattPerSec float64) *Panel {
    return &Panel{
        ID:          uuid.New().String(),
        Number:      number,
        WattPerSec:  wattPerSec,
        Status:      models.StatusOnline,
        FaultMode:   models.FaultNone,
        currentWatt: wattPerSec,
    }
}

func (p *Panel) Generate() models.PanelData {
    p.mu.Lock()
    defer p.mu.Unlock()

    watt := 0.0
    if p.Status == models.StatusOnline {
        switch p.FaultMode {
        case models.FaultDead:
            watt = 0
        case models.FaultDegraded:
            p.currentWatt *= 0.95 // 5% decay per tick
            watt = p.currentWatt
        case models.FaultIntermittent:
            if rand.Float64() < 0.5 {
                watt = 0
            } else {
                watt = p.WattPerSec
            }
        default:
            watt = p.WattPerSec
        }
    }

    return models.PanelData{
        PanelID:     p.ID,
        PanelNumber: p.Number,
        Status:      p.Status,
        FaultMode:   p.FaultMode,
        Watt:        watt,
    }
}

func (p *Panel) SetOffline() {
    p.mu.Lock()
    defer p.mu.Unlock()
    p.Status = models.StatusOffline
}

func (p *Panel) SetOnline() {
    p.mu.Lock()
    defer p.mu.Unlock()
    p.Status = models.StatusOnline
}

func (p *Panel) SetFault(mode string) {
    p.mu.Lock()
    defer p.mu.Unlock()
    p.FaultMode = mode
    p.currentWatt = p.WattPerSec
}

func (p *Panel) Reset() {
    p.mu.Lock()
    defer p.mu.Unlock()
    p.FaultMode = models.FaultNone
    p.Status = models.StatusOnline
    p.currentWatt = p.WattPerSec
}
