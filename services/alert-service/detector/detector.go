package detector

import (
    "fmt"
    "time"

    "github.com/solarops/shared/models"
)

type panelKey struct {
    PlantID string
    PanelID string
}

type reading struct {
    Watt      float64
    Timestamp time.Time
}

type panelState struct {
    PlantName   string
    PanelNumber int
    Readings    []reading
}

type Detector struct {
    states            map[panelKey]*panelState
    deadThreshold     int     // consecutive zero readings to trigger DEAD alert
    degradedPercent   float64 // percent drop to trigger DEGRADED
    unstableFlipCount int     // flips to trigger UNSTABLE
}

func NewDetector(deadThreshold int, degradedPercent float64, unstableFlipCount int) *Detector {
    return &Detector{
        states:            make(map[panelKey]*panelState),
        deadThreshold:     deadThreshold,
        degradedPercent:   degradedPercent,
        unstableFlipCount: unstableFlipCount,
    }
}

func (d *Detector) Feed(plantID, panelID string, panelNumber int, plantName string, watt float64, ts time.Time) {
    key := panelKey{PlantID: plantID, PanelID: panelID}
    state, ok := d.states[key]
    if !ok {
        state = &panelState{PlantName: plantName, PanelNumber: panelNumber}
        d.states[key] = state
    }
    state.Readings = append(state.Readings, reading{Watt: watt, Timestamp: ts})
    if len(state.Readings) > 20 {
        state.Readings = state.Readings[len(state.Readings)-20:]
    }
}

func (d *Detector) Check() []models.Alert {
    var alerts []models.Alert

    for key, state := range d.states {
        if len(state.Readings) < 2 {
            continue
        }

        // DEAD: consecutive zeros from the end
        zeroCount := 0
        for i := len(state.Readings) - 1; i >= 0; i-- {
            if state.Readings[i].Watt == 0 {
                zeroCount++
            } else {
                break
            }
        }
        if zeroCount >= d.deadThreshold {
            alerts = append(alerts, models.Alert{
                Type:        models.AlertPanelFault,
                PlantID:     key.PlantID,
                PlantName:   state.PlantName,
                PanelID:     key.PanelID,
                PanelNumber: state.PanelNumber,
                Status:      models.AlertStatusActive,
                Message:     fmt.Sprintf("Panel-%d has zero output for %d readings", state.PanelNumber, zeroCount),
            })
            continue
        }

        // DEGRADED: sustained decline from first to last reading
        if len(state.Readings) >= 3 {
            first := state.Readings[0].Watt
            last := state.Readings[len(state.Readings)-1].Watt
            if first > 0 && last < first {
                dropPercent := ((first - last) / first) * 100
                if dropPercent >= d.degradedPercent {
                    alerts = append(alerts, models.Alert{
                        Type:        models.AlertPanelDegraded,
                        PlantID:     key.PlantID,
                        PlantName:   state.PlantName,
                        PanelID:     key.PanelID,
                        PanelNumber: state.PanelNumber,
                        Status:      models.AlertStatusActive,
                        Message:     fmt.Sprintf("Panel-%d output dropped %.0f%%", state.PanelNumber, dropPercent),
                    })
                    continue
                }
            }
        }

        // UNSTABLE: count zero/nonzero flips
        flipCount := 0
        for i := 1; i < len(state.Readings); i++ {
            prev := state.Readings[i-1].Watt
            curr := state.Readings[i].Watt
            if (prev == 0 && curr > 0) || (prev > 0 && curr == 0) {
                flipCount++
            }
        }
        if flipCount >= d.unstableFlipCount {
            alerts = append(alerts, models.Alert{
                Type:        models.AlertPanelUnstable,
                PlantID:     key.PlantID,
                PlantName:   state.PlantName,
                PanelID:     key.PanelID,
                PanelNumber: state.PanelNumber,
                Status:      models.AlertStatusActive,
                Message:     fmt.Sprintf("Panel-%d output unstable: %d flips detected", state.PanelNumber, flipCount),
            })
        }
    }

    return alerts
}

func (d *Detector) ClearPanel(plantID, panelID string) {
    delete(d.states, panelKey{PlantID: plantID, PanelID: panelID})
}
