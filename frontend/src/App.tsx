import { BrowserRouter, Routes, Route } from "react-router-dom";
import { useCallback } from "react";
import { Dashboard } from "./pages/Dashboard";
import { PlantDetail } from "./pages/PlantDetail";
import { useWebSocket } from "./hooks/useWebSocket";
import { usePlants } from "./hooks/usePlants";

function App() {
  const { plants, alerts, handleMessage, removePlant, acknowledgeAlert, resolveAlert, updatePanels } =
    usePlants();

  const onMessage = useCallback(
    (msg: { type: string; payload: unknown }) => {
      handleMessage(msg);
    },
    [handleMessage]
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
                onResolveAlert={resolveAlert}
              />
            }
          />
          <Route
            path="/plants/:plantId"
            element={<PlantDetail plants={plants} send={send} updatePanels={updatePanels} />}
          />
        </Routes>
      </div>
    </BrowserRouter>
  );
}

export default App;
