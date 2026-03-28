package plant

import (
    "testing"

    "github.com/solarops/shared/models"
)

func TestNewPlant(t *testing.T) {
    p := NewPlant("Test Plant", 5, 300.0)
    if p.Name != "Test Plant" {
        t.Errorf("expected name 'Test Plant', got %s", p.Name)
    }
    if len(p.Panels) != 5 {
        t.Errorf("expected 5 panels, got %d", len(p.Panels))
    }
}

func TestPlantGenerateData(t *testing.T) {
    p := NewPlant("Test Plant", 3, 300.0)
    data := p.GenerateData()

    if data.PlantName != "Test Plant" {
        t.Errorf("expected plant name, got %s", data.PlantName)
    }
    if len(data.Panels) != 3 {
        t.Errorf("expected 3 panels, got %d", len(data.Panels))
    }
    if data.TotalWatt != 900.0 {
        t.Errorf("expected 900W total, got %f", data.TotalWatt)
    }
    if data.OnlineCount != 3 {
        t.Errorf("expected 3 online, got %d", data.OnlineCount)
    }
}

func TestPlantHandleCommand_Offline(t *testing.T) {
    p := NewPlant("Test", 3, 300.0)
    panelID := p.Panels[0].ID

    p.HandleCommand(models.Command{
        Command: models.CmdOffline,
        PanelID: panelID,
    })

    data := p.GenerateData()
    if data.OfflineCount != 1 {
        t.Errorf("expected 1 offline, got %d", data.OfflineCount)
    }
}

func TestPlantHandleCommand_Fault(t *testing.T) {
    p := NewPlant("Test", 3, 300.0)
    panelID := p.Panels[0].ID

    p.HandleCommand(models.Command{
        Command:   models.CmdFault,
        PanelID:   panelID,
        FaultMode: models.FaultDead,
    })

    data := p.GenerateData()
    if data.FaultyCount != 1 {
        t.Errorf("expected 1 faulty, got %d", data.FaultyCount)
    }
}

func TestPlantHandleCommand_Reset(t *testing.T) {
    p := NewPlant("Test", 3, 300.0)
    panelID := p.Panels[0].ID

    p.HandleCommand(models.Command{
        Command:   models.CmdFault,
        PanelID:   panelID,
        FaultMode: models.FaultDead,
    })
    p.HandleCommand(models.Command{
        Command: models.CmdReset,
        PanelID: panelID,
    })

    data := p.GenerateData()
    if data.FaultyCount != 0 {
        t.Errorf("expected 0 faulty after reset, got %d", data.FaultyCount)
    }
    if data.TotalWatt != 900.0 {
        t.Errorf("expected full power after reset, got %f", data.TotalWatt)
    }
}
