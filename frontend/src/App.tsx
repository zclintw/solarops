import { BrowserRouter, Routes, Route } from "react-router-dom";
import { useCallback, useRef } from "react";
import { Dashboard } from "./pages/Dashboard";
import { PlantDetail } from "./pages/PlantDetail";
import { useWebSocket } from "./hooks/useWebSocket";
import { usePlants } from "./hooks/usePlants";

function App() {
  const { plants, alerts, handleMessage, removePlant, acknowledgeAlert } =
    usePlants();

  const powerHistoryRef = useRef<{ time: string; watt: number }[]>([]);
  const lastAggTime = useRef(0);

  const onMessage = useCallback(
    (msg: { type: string; payload: unknown }) => {
      handleMessage(msg);

      if (msg.type === "PLANT_DATA") {
        const now = Math.floor(Date.now() / 10000) * 10000;
        if (now > lastAggTime.current) {
          lastAggTime.current = now;
          const totalWatt = Object.values(plants).reduce(
            (sum, s) => sum + (s.data?.totalWatt || 0),
            0
          );
          powerHistoryRef.current = [
            ...powerHistoryRef.current.slice(-59),
            {
              time: new Date(now).toLocaleTimeString(),
              watt: Math.round(totalWatt),
            },
          ];
        }
      }
    },
    [handleMessage, plants]
  );

  const { send } = useWebSocket(onMessage);

  return (
    <BrowserRouter>
      <div
        style={{
          minHeight: "100vh",
          backgroundColor: "#0a0a0a",
          color: "#fff",
          fontFamily: "system-ui, sans-serif",
        }}
      >
        <header
          style={{
            padding: "12px 24px",
            borderBottom: "1px solid #333",
            display: "flex",
            alignItems: "center",
            gap: 12,
          }}
        >
          <h1 style={{ margin: 0, fontSize: 20 }}>SolarOps</h1>
          <span style={{ color: "#888", fontSize: 14 }}>
            Solar Plant Monitoring
          </span>
        </header>

        <Routes>
          <Route
            path="/"
            element={
              <Dashboard
                plants={plants}
                alerts={alerts}
                onRemovePlant={removePlant}
                onAcknowledgeAlert={acknowledgeAlert}
                powerHistory={powerHistoryRef.current}
              />
            }
          />
          <Route
            path="/plants/:plantId"
            element={<PlantDetail plants={plants} send={send} />}
          />
        </Routes>
      </div>
    </BrowserRouter>
  );
}

export default App;
