package models

import "time"

// Panel statuses
const (
	StatusOnline  = "online"
	StatusOffline = "offline"
)

// Fault modes
const (
	FaultNone         = ""
	FaultDead         = "DEAD"
	FaultDegraded     = "DEGRADED"
	FaultIntermittent = "INTERMITTENT"
)

// Command types
const (
	CmdOffline = "OFFLINE"
	CmdOnline  = "ONLINE"
	CmdReset   = "RESET"
	CmdFault   = "FAULT"
)

// WebSocket message types (server → client)
const (
	MsgPlantData         = "PLANT_DATA"
	MsgPlantRegistered   = "PLANT_REGISTERED"
	MsgPlantUnregistered = "PLANT_UNREGISTERED"
	MsgAlertNew          = "ALERT_NEW"
	MsgAlertResolved     = "ALERT_RESOLVED"
)

// WebSocket message types (client → server)
const (
	MsgPanelOffline = "PANEL_OFFLINE"
	MsgPanelOnline  = "PANEL_ONLINE"
	MsgPanelReset   = "PANEL_RESET"
)

// Alert types
const (
	AlertPanelFault    = "PANEL_FAULT"
	AlertPanelDegraded = "PANEL_DEGRADED"
	AlertPanelUnstable = "PANEL_UNSTABLE"
	AlertDataGap       = "DATA_GAP"
)

// Alert statuses
const (
	AlertStatusActive       = "active"
	AlertStatusAcknowledged = "acknowledged"
	AlertStatusResolved     = "resolved"
)

type PanelData struct {
	PanelID     string  `json:"panelId"`
	PanelNumber int     `json:"panelNumber"`
	Status      string  `json:"status"`
	FaultMode   string  `json:"faultMode,omitempty"`
	Watt        float64 `json:"watt"`
}

type PlantData struct {
	PlantID      string      `json:"plantId"`
	PlantName    string      `json:"plantName"`
	Timestamp    time.Time   `json:"timestamp"`
	Panels       []PanelData `json:"panels"`
	TotalWatt    float64     `json:"totalWatt"`
	OnlineCount  int         `json:"onlineCount"`
	OfflineCount int         `json:"offlineCount"`
	FaultyCount  int         `json:"faultyCount"`
}

type Command struct {
	Command   string `json:"command"`
	PanelID   string `json:"panelId"`
	FaultMode string `json:"faultMode,omitempty"`
}

type WSMessage struct {
	Type    string      `json:"type"`
	Payload any `json:"payload"`
}

type Alert struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	PlantID     string    `json:"plantId"`
	PlantName   string    `json:"plantName"`
	PanelID     string    `json:"panelId,omitempty"`
	PanelNumber int       `json:"panelNumber,omitempty"`
	Status      string    `json:"status"`
	Message     string    `json:"message"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type PlantInfo struct {
	PlantID    string  `json:"plantId"`
	PlantName  string  `json:"plantName"`
	Panels     int     `json:"panels"`
	WattPerSec float64 `json:"wattPerSec"`
}
