package plant

import (
    "sync"
    "time"

    "github.com/google/uuid"
    "github.com/solarops/shared/models"
)

type Plant struct {
    ID     string
    Name   string
    Panels []*Panel
    mu     sync.RWMutex
}

func NewPlant(name string, panelCount int, wattPerSec float64) *Plant {
    panels := make([]*Panel, panelCount)
    for i := 0; i < panelCount; i++ {
        panels[i] = NewPanel(i+1, wattPerSec)
    }
    return &Plant{
        ID:     uuid.New().String(),
        Name:   name,
        Panels: panels,
    }
}

func (p *Plant) GenerateData() models.PlantData {
    p.mu.RLock()
    defer p.mu.RUnlock()

    panelData := make([]models.PanelData, len(p.Panels))
    totalWatt := 0.0
    online, offline, faulty := 0, 0, 0

    for i, panel := range p.Panels {
        pd := panel.Generate()
        panelData[i] = pd
        totalWatt += pd.Watt

        switch {
        case pd.Status == models.StatusOffline:
            offline++
        case pd.FaultMode != models.FaultNone:
            faulty++
            online++ // faulty panels are still "online" in status
        default:
            online++
        }
    }

    return models.PlantData{
        PlantID:      p.ID,
        PlantName:    p.Name,
        Timestamp:    time.Now().UTC(),
        Panels:       panelData,
        TotalWatt:    totalWatt,
        OnlineCount:  online,
        OfflineCount: offline,
        FaultyCount:  faulty,
    }
}

func (p *Plant) HandleCommand(cmd models.Command) {
    p.mu.Lock()
    defer p.mu.Unlock()

    for _, panel := range p.Panels {
        if panel.ID == cmd.PanelID {
            switch cmd.Command {
            case models.CmdOffline:
                panel.SetOffline()
            case models.CmdOnline:
                panel.SetOnline()
            case models.CmdReset:
                panel.Reset()
            case models.CmdFault:
                panel.SetFault(cmd.FaultMode)
            }
            return
        }
    }
}
