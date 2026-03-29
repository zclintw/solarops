export interface PanelData {
  panelId: string;
  panelNumber: number;
  status: "online" | "offline";
  faultMode: string | null;
  watt: number;
}

export interface PlantData {
  plantId: string;
  plantName: string;
  timestamp: string;
  panels: PanelData[];
  totalWatt: number;
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
  data: PlantData | null;
  status: PlantStatus;
  lastSeen: number;
}
