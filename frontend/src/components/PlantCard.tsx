import { Link } from "react-router-dom";
import type { PlantState } from "../types";

interface PlantCardProps {
  plantId: string;
  state: PlantState;
  onRemove?: () => void;
}

const STATUS_COLORS: Record<string, string> = {
  online: "#22c55e",
  fault: "#ef4444",
  stale: "#eab308",
  offline: "#6b7280",
};

const STATUS_LABELS: Record<string, string> = {
  online: "Online",
  fault: "Fault",
  stale: "Data Stale",
  offline: "Offline",
};

export function PlantCard({ plantId, state, onRemove }: PlantCardProps) {
  const { summary, status } = state;
  const color = STATUS_COLORS[status] || "#6b7280";

  return (
    <Link
      to={`/plants/${plantId}`}
      style={{
        display: "block",
        border: `2px solid ${color}`,
        borderRadius: 8,
        padding: 16,
        backgroundColor: "#1a1a1a",
        textDecoration: "none",
        color: "inherit",
      }}
    >
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <h3 style={{ margin: 0 }}>{summary?.plantName || "Loading..."}</h3>
        <span
          style={{
            width: 12,
            height: 12,
            borderRadius: "50%",
            backgroundColor: color,
            display: "inline-block",
          }}
        />
      </div>
      <div style={{ color: "#888", fontSize: 14, marginTop: 4 }}>
        {STATUS_LABELS[status]}
      </div>
      {summary && (
        <div style={{ marginTop: 12, fontSize: 14 }}>
          <div style={{ fontSize: 24, fontWeight: "bold" }}>
            {(summary.totalWatt / 1000).toFixed(1)} kW
          </div>
          <div style={{ marginTop: 8, color: "#aaa" }}>
            Panels: {summary.panelCount} | Normal: {summary.onlineCount - summary.faultyCount} | Faulty: {summary.faultyCount}
          </div>
        </div>
      )}
      {status === "offline" && onRemove && (
        <button
          onClick={(e) => {
            e.preventDefault();
            onRemove();
          }}
          style={{
            marginTop: 8,
            padding: "4px 12px",
            backgroundColor: "#333",
            border: "1px solid #555",
            borderRadius: 4,
            color: "#fff",
            cursor: "pointer",
          }}
        >
          Remove
        </button>
      )}
    </Link>
  );
}
