import type { PanelReading } from "../types";

interface PanelGridProps {
  panels: PanelReading[];
  onToggle: (panelId: string, currentStatus: string) => void;
  onReset: (panelId: string) => void;
}

function getPanelColor(panel: PanelReading): string {
  if (panel.status === "offline") return "#6b7280";
  if (panel.faultMode) return "#ef4444";
  return "#22c55e";
}

export function PanelGrid({ panels, onToggle, onReset }: PanelGridProps) {
  return (
    <div
      style={{
        display: "grid",
        gridTemplateColumns: "repeat(auto-fill, minmax(100px, 1fr))",
        gap: 8,
      }}
    >
      {panels.map((panel) => {
        const color = getPanelColor(panel);
        const hasFault = !!panel.faultMode && panel.status !== "offline";

        return (
          <div
            key={panel.panelId}
            style={{
              border: `2px solid ${color}`,
              borderRadius: 8,
              padding: 12,
              backgroundColor: "#1a1a1a",
              textAlign: "center",
            }}
          >
            <div style={{ fontWeight: "bold", fontSize: 16 }}>
              {panel.panelNumber}
            </div>
            <div
              style={{
                width: 10,
                height: 10,
                borderRadius: "50%",
                backgroundColor: color,
                margin: "8px auto",
              }}
            />
            <div style={{ fontSize: 14 }}>
              {panel.watt.toFixed(0)}W
            </div>
            {panel.faultMode && (
              <div style={{ fontSize: 11, color: "#ef4444", marginTop: 4 }}>
                {panel.faultMode}
              </div>
            )}
            <div style={{ marginTop: 8, display: "flex", gap: 4, justifyContent: "center" }}>
              <button
                onClick={() => onToggle(panel.panelId, panel.status)}
                style={{
                  padding: "2px 8px",
                  fontSize: 11,
                  backgroundColor: panel.status === "offline" ? "#22c55e" : "#ef4444",
                  border: "none",
                  borderRadius: 4,
                  color: "#fff",
                  cursor: "pointer",
                }}
              >
                {panel.status === "offline" ? "ON" : "OFF"}
              </button>
              {hasFault && (
                <button
                  onClick={() => onReset(panel.panelId)}
                  style={{
                    padding: "2px 8px",
                    fontSize: 11,
                    backgroundColor: "#3b82f6",
                    border: "none",
                    borderRadius: 4,
                    color: "#fff",
                    cursor: "pointer",
                  }}
                >
                  Reset
                </button>
              )}
            </div>
          </div>
        );
      })}
    </div>
  );
}
