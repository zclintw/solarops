import type { Alert } from "../types";

interface AlertListProps {
  alerts: Alert[];
  onAcknowledge: (id: string) => void;
}

const TYPE_COLORS: Record<string, string> = {
  PANEL_FAULT: "#ef4444",
  PANEL_DEGRADED: "#f97316",
  PANEL_UNSTABLE: "#eab308",
  DATA_GAP: "#6b7280",
};

export function AlertList({ alerts, onAcknowledge }: AlertListProps) {
  const activeAlerts = alerts.filter((a) => a.status !== "resolved");

  if (activeAlerts.length === 0) {
    return (
      <div style={{ padding: 16, color: "#666" }}>No active alerts</div>
    );
  }

  return (
    <div>
      {activeAlerts.map((alert) => (
        <div
          key={alert.id}
          style={{
            display: "flex",
            justifyContent: "space-between",
            alignItems: "center",
            padding: "8px 16px",
            borderBottom: "1px solid #333",
          }}
        >
          <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
            <span
              style={{
                width: 8,
                height: 8,
                borderRadius: "50%",
                backgroundColor: TYPE_COLORS[alert.type] || "#888",
                display: "inline-block",
              }}
            />
            <span>
              Panel-{alert.panelNumber} @ {alert.plantName}
            </span>
            <span style={{ color: "#888", fontSize: 12 }}>
              {alert.type}
            </span>
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
            <span
              style={{
                fontSize: 12,
                color: alert.status === "acknowledged" ? "#eab308" : "#ef4444",
              }}
            >
              {alert.status}
            </span>
            {alert.status === "active" && (
              <button
                onClick={() => onAcknowledge(alert.id)}
                style={{
                  padding: "2px 8px",
                  fontSize: 12,
                  backgroundColor: "#333",
                  border: "1px solid #555",
                  borderRadius: 4,
                  color: "#fff",
                  cursor: "pointer",
                }}
              >
                ACK
              </button>
            )}
          </div>
        </div>
      ))}
    </div>
  );
}
