export interface PanelReading {
  plantId: string;
  plantName: string;
  panelId: string;
  panelNumber: number;
  status: "online" | "offline";
  faultMode: string | null;
  watt: number;
  timestamp: string;
}

export interface PlantSummary {
  plantId: string;
  plantName: string;
  timestamp: string;
  totalWatt: number;
  panelCount: number;
  onlineCount: number;
  offlineCount: number;
  faultyCount: number;
}

export interface Alert {
  id: string;
  type: string;
  plantId: string;
  plantName: string;
  panelId?: string;
  panelNumber?: number;
  status: "active" | "acknowledged" | "resolved";
  message: string;
  createdAt: string;
  updatedAt: string;
}

export interface WSMessage {
  type: string;
  payload: unknown;
}

export type PlantStatus = "online" | "fault" | "stale" | "offline";

export interface PlantState {
  summary: PlantSummary | null;
  panels: Record<string, PanelReading>;
  status: PlantStatus;
  lastSeen: number;
}
