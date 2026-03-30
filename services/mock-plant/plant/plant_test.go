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

func TestPlantHandleCommand_Offline(t *testing.T) {
    p := NewPlant("Test", 3, 300.0)
    panelID := p.Panels[0].ID

    p.HandleCommand(models.Command{
        Command: models.CmdOffline,
        PanelID: panelID,
    })

    summary := p.GenerateSummary()
    if summary.OfflineCount != 1 {
        t.Errorf("expected 1 offline, got %d", summary.OfflineCount)
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

    summary := p.GenerateSummary()
    if summary.FaultyCount != 1 {
        t.Errorf("expected 1 faulty, got %d", summary.FaultyCount)
    }
}

func TestGeneratePanelReadings(t *testing.T) {
	p := NewPlant("Test", 3, 500)
	readings := p.GeneratePanelReadings()

	if len(readings) != 3 {
		t.Fatalf("expected 3 readings, got %d", len(readings))
	}
	for i, r := range readings {
		if r.PlantID != p.ID {
			t.Errorf("reading %d: wrong plantId", i)
		}
		if r.PlantName != "Test" {
			t.Errorf("reading %d: wrong plantName", i)
		}
		if r.PanelNumber != i+1 {
			t.Errorf("reading %d: expected panelNumber %d, got %d", i, i+1, r.PanelNumber)
		}
		if r.Watt != 500 {
			t.Errorf("reading %d: expected 500 watt, got %.0f", i, r.Watt)
		}
		if r.Timestamp.IsZero() {
			t.Errorf("reading %d: timestamp is zero", i)
		}
	}
}

func TestGenerateSummary(t *testing.T) {
	p := NewPlant("Test", 3, 500)
	summary := p.GenerateSummary()

	if summary.PlantID != p.ID {
		t.Error("wrong plantId")
	}
	if summary.TotalWatt != 1500 {
		t.Errorf("expected 1500 totalWatt, got %.0f", summary.TotalWatt)
	}
	if summary.PanelCount != 3 {
		t.Errorf("expected panelCount 3, got %d", summary.PanelCount)
	}
	if summary.OnlineCount != 3 {
		t.Errorf("expected onlineCount 3, got %d", summary.OnlineCount)
	}
	if summary.FaultyCount != 0 {
		t.Errorf("expected faultyCount 0, got %d", summary.FaultyCount)
	}
}

func TestGenerateSummaryWithFault(t *testing.T) {
	p := NewPlant("Test", 3, 500)
	p.HandleCommand(models.Command{
		Command:   models.CmdFault,
		PanelID:   p.Panels[0].ID,
		FaultMode: models.FaultDead,
	})
	summary := p.GenerateSummary()

	if summary.TotalWatt != 1000 {
		t.Errorf("expected 1000 totalWatt (one dead), got %.0f", summary.TotalWatt)
	}
	if summary.FaultyCount != 1 {
		t.Errorf("expected faultyCount 1, got %d", summary.FaultyCount)
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

    summary := p.GenerateSummary()
    if summary.FaultyCount != 0 {
        t.Errorf("expected 0 faulty after reset, got %d", summary.FaultyCount)
    }
    if summary.TotalWatt != 900.0 {
        t.Errorf("expected full power after reset, got %f", summary.TotalWatt)
    }
}
