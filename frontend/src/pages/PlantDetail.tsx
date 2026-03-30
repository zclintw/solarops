import { useParams, Link } from "react-router-dom";
import { useEffect, useState } from "react";
import type { PlantState } from "../types";
import { PanelGrid } from "../components/PanelGrid";
import { PowerChart } from "../components/PowerChart";

interface PlantDetailProps {
  plants: Record<string, PlantState>;
  send: (type: string, payload: unknown) => void;
}

const STATUS_COLORS: Record<string, string> = {
  online: "#22c55e",
  fault: "#ef4444",
  stale: "#eab308",
  offline: "#6b7280",
};

export function PlantDetail({ plants, send }: PlantDetailProps) {
  const { plantId } = useParams<{ plantId: string }>();
  const state = plantId ? plants[plantId] : undefined;
  const [history, setHistory] = useState<{ time: string; watt: number }[]>([]);

  useEffect(() => {
    if (!plantId) return;
    fetch(`/api/plants/${plantId}/history?range=1h&interval=10s`)
      .then((res) => res.json())
      .then((data) => {
        const buckets = data?.aggregations?.over_time?.buckets || [];
        setHistory(
          buckets.map((b: { key_as_string: string; total_watt: { value: number } }) => ({
            time: new Date(b.key_as_string).toLocaleTimeString(),
            watt: Math.round(b.total_watt?.value || 0),
          }))
        );
      })
      .catch(console.error);
  }, [plantId]);

  useEffect(() => {
    if (!state?.summary) return;
    const now = Math.floor(Date.now() / 10000) * 10000;
    setHistory((prev) => [
      ...prev.slice(-59),
      {
        time: new Date(now).toLocaleTimeString(),
        watt: Math.round(state.summary!.totalWatt),
      },
    ]);
  }, [state?.summary?.timestamp]);

  if (!state?.summary) {
    return (
      <div style={{ padding: 24 }}>
        <Link to="/" style={{ color: "#888" }}>
          Back to Dashboard
        </Link>
        <div style={{ marginTop: 16 }}>Loading plant data...</div>
      </div>
    );
  }

  const { summary, panels, status } = state;
  const panelList = Object.values(panels);
  const color = STATUS_COLORS[status] || "#6b7280";

  const handleToggle = (panelId: string, currentStatus: string) => {
    const type = currentStatus === "offline" ? "PANEL_ONLINE" : "PANEL_OFFLINE";
    send(type, { plantId, panelId });
  };

  const handleReset = (panelId: string) => {
    send("PANEL_RESET", { plantId, panelId });
  };

  return (
    <div style={{ padding: 24, maxWidth: 1200, margin: "0 auto" }}>
      <Link to="/" style={{ color: "#888", textDecoration: "none" }}>
        ← Back to Dashboard
      </Link>

      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: 16,
          marginTop: 16,
          marginBottom: 24,
        }}
      >
        <h1 style={{ margin: 0 }}>{summary.plantName}</h1>
        <span
          style={{
            width: 12,
            height: 12,
            borderRadius: "50%",
            backgroundColor: color,
            display: "inline-block",
          }}
        />
        <span style={{ fontSize: 28, fontWeight: "bold", color: "#22c55e" }}>
          {(summary.totalWatt / 1000).toFixed(1)} kW
        </span>
      </div>

      <div
        style={{
          marginBottom: 24,
          padding: 20,
          backgroundColor: "#1a1a1a",
          borderRadius: 8,
          border: "1px solid #333",
        }}
      >
        <h2 style={{ margin: "0 0 16px", fontSize: 16 }}>Power History</h2>
        <PowerChart data={history} height={250} />
      </div>

      <div
        style={{
          padding: 20,
          backgroundColor: "#1a1a1a",
          borderRadius: 8,
          border: "1px solid #333",
        }}
      >
        <h2 style={{ margin: "0 0 16px", fontSize: 16 }}>
          Solar Panels ({summary.panelCount})
          <span style={{ color: "#888", fontWeight: "normal", marginLeft: 8 }}>
            Online: {summary.onlineCount} | Faulty: {summary.faultyCount} | Offline: {summary.offlineCount}
          </span>
        </h2>
        <PanelGrid
          panels={panelList}
          onToggle={handleToggle}
          onReset={handleReset}
        />
      </div>
    </div>
  );
}
