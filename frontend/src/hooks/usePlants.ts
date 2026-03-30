import { useState, useCallback, useEffect } from "react";
import type { PanelReading, PlantSummary, PlantState, Alert, WSMessage } from "../types";

const STALE_THRESHOLD_MS = 30_000;
const OFFLINE_THRESHOLD_MS = 90_000;
const POLL_INTERVAL_MS = 3_000;

export function usePlants() {
  const [plants, setPlants] = useState<Record<string, PlantState>>({});
  const [alerts, setAlerts] = useState<Alert[]>([]);

  // Poll plant summaries from ES via plant-manager API
  useEffect(() => {
    const poll = async () => {
      try {
        const res = await fetch("/api/plants/summary");
        const data = await res.json();
        const buckets: Array<{
          key: string;
          latest: { hits: { hits: Array<{ _source: PlantSummary }> } };
        }> = data?.aggregations?.by_plant?.buckets || [];

        setPlants((prev) => {
          const next = { ...prev };
          for (const bucket of buckets) {
            const summary = bucket.latest?.hits?.hits?.[0]?._source;
            if (!summary) continue;
            next[summary.plantId] = {
              summary,
              panels: prev[summary.plantId]?.panels || {},
              status: summary.faultyCount > 0 ? "fault" : "online",
              lastSeen: Date.now(),
            };
          }
          return next;
        });
      } catch {}
    };

    poll();
    const interval = setInterval(poll, POLL_INTERVAL_MS);
    return () => clearInterval(interval);
  }, []);

  // WebSocket events only: plant registration and alerts
  const handleMessage = useCallback((msg: WSMessage) => {
    switch (msg.type) {
      case "PLANT_REGISTERED": {
        const info = msg.payload as { plantId: string; plantName: string };
        setPlants((prev) => {
          if (prev[info.plantId]) return prev; // already known from polling
          return {
            ...prev,
            [info.plantId]: {
              summary: null,
              panels: {},
              status: "online",
              lastSeen: Date.now(),
            },
          };
        });
        break;
      }
      case "ALERT_NEW": {
        const alert = msg.payload as Alert;
        setAlerts((prev) => [alert, ...prev]);
        break;
      }
      case "ALERT_RESOLVED": {
        const alert = msg.payload as Alert;
        setAlerts((prev) =>
          prev.map((a) =>
            a.id === alert.id ? { ...a, status: "resolved" } : a
          )
        );
        break;
      }
    }
  }, []);

  // Check for stale/offline plants (no data from ES in a while)
  useEffect(() => {
    const interval = setInterval(() => {
      const now = Date.now();
      setPlants((prev) => {
        const next = { ...prev };
        let changed = false;
        for (const [id, state] of Object.entries(next)) {
          const elapsed = now - state.lastSeen;
          let newStatus = state.status;

          if (elapsed > OFFLINE_THRESHOLD_MS && state.status !== "offline") {
            newStatus = "offline";
          } else if (
            elapsed > STALE_THRESHOLD_MS &&
            state.status !== "offline" &&
            state.status !== "stale"
          ) {
            newStatus = "stale";
          }

          if (newStatus !== state.status) {
            next[id] = { ...state, status: newStatus };
            changed = true;
          }
        }
        return changed ? next : prev;
      });
    }, 5000);

    return () => clearInterval(interval);
  }, []);

  // Fetch existing alerts on mount
  useEffect(() => {
    fetch("/api/alerts")
      .then((r) => r.json())
      .then((data: Alert[]) => setAlerts(data))
      .catch(() => {});
  }, []);

  const removePlant = useCallback((plantId: string) => {
    setPlants((prev) => {
      const next = { ...prev };
      delete next[plantId];
      return next;
    });
  }, []);

  const acknowledgeAlert = useCallback(async (alertId: string) => {
    await fetch(`/api/alerts/${alertId}/acknowledge`, { method: "POST" });
    setAlerts((prev) =>
      prev.map((a) =>
        a.id === alertId ? { ...a, status: "acknowledged" } : a
      )
    );
  }, []);

  // Update panels for a specific plant (called by PlantDetail)
  const updatePanels = useCallback((plantId: string, panels: Record<string, PanelReading>) => {
    setPlants((prev) => {
      const existing = prev[plantId];
      if (!existing) return prev;
      return { ...prev, [plantId]: { ...existing, panels } };
    });
  }, []);

  return { plants, alerts, handleMessage, removePlant, acknowledgeAlert, updatePanels };
}
