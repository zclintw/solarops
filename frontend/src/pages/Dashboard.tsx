import { useState, useEffect } from "react";
import { PlantCard } from "../components/PlantCard";
import { AlertList } from "../components/AlertList";
import { PowerChart } from "../components/PowerChart";
import type { PlantState, Alert } from "../types";

interface DashboardProps {
  plants: Record<string, PlantState>;
  alerts: Alert[];
  onRemovePlant: (id: string) => void;
  onAcknowledgeAlert: (id: string) => void;
  onResolveAlert: (id: string) => void;
}

function fetchPowerHistory() {
  return fetch("/api/power/history?range=5m&interval=1s")
    .then((res) => res.json())
    .then((data) => {
      const buckets = data?.aggregations?.over_time?.buckets || [];
      return buckets.map(
        (b: { key_as_string: string; total_watt: { value: number | null } }) => ({
          time: new Date(b.key_as_string).toLocaleTimeString(),
          watt: b.total_watt?.value != null ? Math.round(b.total_watt.value) : null,
        })
      );
    });
}

export function Dashboard({
  plants,
  alerts,
  onRemovePlant,
  onAcknowledgeAlert,
  onResolveAlert,
}: DashboardProps) {
  const plantEntries = Object.entries(plants);

  const totalWatt = plantEntries.reduce(
    (sum, [, state]) => sum + (state.summary?.totalWatt || 0),
    0
  );

  const [history, setHistory] = useState<{ time: string; watt: number | null }[]>([]);

  useEffect(() => {
    fetchPowerHistory().then(setHistory).catch(console.error);
    const interval = setInterval(() => {
      fetchPowerHistory().then(setHistory).catch(console.error);
    }, 10_000);
    return () => clearInterval(interval);
  }, []);

  return (
    <div style={{ padding: 24, maxWidth: 1200, margin: "0 auto" }}>
      {/* Summary bar */}
      <div
        style={{
          display: "flex",
          gap: 32,
          marginBottom: 24,
          padding: 20,
          backgroundColor: "#1a1a1a",
          borderRadius: 8,
          border: "1px solid #333",
        }}
      >
        <div>
          <div style={{ color: "#888", fontSize: 14 }}>Plants</div>
          <div style={{ fontSize: 32, fontWeight: "bold" }}>
            {plantEntries.length}
          </div>
        </div>
        <div>
          <div style={{ color: "#888", fontSize: 14 }}>Total Power</div>
          <div style={{ fontSize: 32, fontWeight: "bold", color: "#22c55e" }}>
            {(totalWatt / 1000).toFixed(1)} kW
          </div>
        </div>
      </div>

      {/* Power history chart */}
      <div
        style={{
          marginBottom: 24,
          padding: 20,
          backgroundColor: "#1a1a1a",
          borderRadius: 8,
          border: "1px solid #333",
        }}
      >
        <h2 style={{ margin: "0 0 16px", fontSize: 16 }}>
          Total Power Output
        </h2>
        <PowerChart data={history} height={250} />
      </div>

      {/* Plant cards grid */}
      <div
        style={{
          display: "grid",
          gridTemplateColumns: "repeat(auto-fill, minmax(280px, 1fr))",
          gap: 16,
          marginBottom: 24,
        }}
      >
        {plantEntries.map(([id, state]) => (
          <PlantCard
            key={id}
            plantId={id}
            state={state}
            onRemove={
              state.status === "offline" ? () => onRemovePlant(id) : undefined
            }
          />
        ))}
      </div>

      {/* Alerts */}
      <div
        style={{
          backgroundColor: "#1a1a1a",
          borderRadius: 8,
          border: "1px solid #333",
        }}
      >
        <h2 style={{ margin: 0, padding: 16, fontSize: 16, borderBottom: "1px solid #333" }}>
          Alerts
        </h2>
        <AlertList alerts={alerts} onAcknowledge={onAcknowledgeAlert} onResolve={onResolveAlert} />
      </div>
    </div>
  );
}
