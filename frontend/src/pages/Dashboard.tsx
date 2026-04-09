import { PlantCard } from "../components/PlantCard";
import { AlertList } from "../components/AlertList";
import { PowerChart } from "../components/PowerChart";
import type { PlantState, Alert } from "../types";

interface DashboardProps {
  plants: Record<string, PlantState>;
  alerts: Alert[];
  powerHistory: { time: string; watt: number }[];
  onRemovePlant: (id: string) => void;
  onAcknowledgeAlert: (id: string) => void;
  onResolveAlert: (id: string) => void;
}

export function Dashboard({
  plants,
  alerts,
  powerHistory,
  onRemovePlant,
  onAcknowledgeAlert,
  onResolveAlert,
}: DashboardProps) {
  const plantEntries = Object.entries(plants);

  const totalWatt = plantEntries.reduce(
    (sum, [, state]) => sum + (state.summary?.totalWatt || 0),
    0
  );

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
        <PowerChart data={powerHistory} height={250} />
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
