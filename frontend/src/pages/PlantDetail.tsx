import { useParams, Link } from "react-router-dom";
import { useEffect, useState } from "react";
import type { PlantState, PanelReading } from "../types";
import { PanelGrid } from "../components/PanelGrid";
import { PowerChart } from "../components/PowerChart";

interface PlantDetailProps {
  plants: Record<string, PlantState>;
  send: (type: string, payload: unknown) => void;
  updatePanels: (plantId: string, panels: Record<string, PanelReading>) => void;
}

const STATUS_COLORS: Record<string, string> = {
  online: "#22c55e",
  fault: "#ef4444",
  stale: "#eab308",
  offline: "#6b7280",
};

export function PlantDetail({ plants, send, updatePanels }: PlantDetailProps) {
  const { plantId } = useParams<{ plantId: string }>();
  const state = plantId ? plants[plantId] : undefined;
  const [history, setHistory] = useState<{ time: string; watt: number | null }[]>([]);

  // Load historical chart data once on mount
  useEffect(() => {
    if (!plantId) return;
    fetch(`/api/plants/${plantId}/history?range=1h&interval=10s`)
      .then((res) => res.json())
      .then((data) => {
        const buckets = data?.aggregations?.over_time?.buckets || [];
        setHistory(
          buckets.map((b: { key_as_string: string; total_watt: { value: number | null } }) => ({
            time: new Date(b.key_as_string).toLocaleTimeString(),
            watt: b.total_watt?.value != null ? Math.round(b.total_watt.value) : null,
          }))
        );
      })
      .catch(console.error);
  }, [plantId]);

  // Append a live data point whenever the summary updates (polled every 3s)
  useEffect(() => {
    if (!state?.summary) return;
    setHistory((prev) => [
      ...prev.slice(-59),
      {
        time: new Date().toLocaleTimeString(),
        watt: Math.round(state.summary!.totalWatt),
      },
    ]);
  }, [state?.summary?.timestamp]);

  // Poll panel readings every 2s
  useEffect(() => {
    if (!plantId) return;
    const poll = async () => {
      try {
        const res = await fetch(`/api/plants/${plantId}/panels`);
        const data = await res.json();
        const buckets: Array<{
          key: string;
          latest: { hits: { hits: Array<{ _source: PanelReading }> } };
        }> = data?.aggregations?.by_panel?.buckets || [];
        const panels: Record<string, PanelReading> = {};
        for (const bucket of buckets) {
          const reading = bucket.latest?.hits?.hits?.[0]?._source;
          if (reading) panels[reading.panelId] = reading;
        }
        updatePanels(plantId, panels);
      } catch {}
    };
    poll();
    const interval = setInterval(poll, 2_000);
    return () => clearInterval(interval);
  }, [plantId, updatePanels]);

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

  // NOTE: Fault uses REST directly (not WebSocket→NATS like ON/OFF/Reset).
  // This is intentional for dev/testing convenience only — in production,
  // fault injection should go through the same command channel as other panel ops.
  const handleFault = (panelId: string, mode: string) => {
    fetch(`/api/plants/${plantId}/panels/${panelId}/fault`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ mode }),
    }).catch(console.error);
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
          onFault={handleFault}
        />
      </div>
    </div>
  );
}
