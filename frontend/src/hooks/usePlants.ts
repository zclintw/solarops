import { useState, useCallback, useRef, useEffect } from "react";
import type { PlantData, PlantState, Alert, WSMessage } from "../types";

const STALE_THRESHOLD_MS = 10_000;
const OFFLINE_THRESHOLD_MS = 60_000;

export function usePlants() {
  const [plants, setPlants] = useState<Record<string, PlantState>>({});
  const [alerts, setAlerts] = useState<Alert[]>([]);
  const plantsRef = useRef(plants);
  plantsRef.current = plants;

  const handleMessage = useCallback((msg: WSMessage) => {
    switch (msg.type) {
      case "PLANT_DATA": {
        const data = msg.payload as PlantData;
        setPlants((prev) => ({
          ...prev,
          [data.plantId]: {
            data,
            status: data.faultyCount > 0 ? "fault" : "online",
            lastSeen: Date.now(),
          },
        }));
        break;
      }
      case "PLANT_REGISTERED": {
        const info = msg.payload as { plantId: string; plantName: string };
        setPlants((prev) => ({
          ...prev,
          [info.plantId]: {
            data: null,
            status: "online",
            lastSeen: Date.now(),
          },
        }));
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

  // Check for stale/offline plants
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
    }, 1000);

    return () => clearInterval(interval);
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

  return { plants, alerts, handleMessage, removePlant, acknowledgeAlert };
}
