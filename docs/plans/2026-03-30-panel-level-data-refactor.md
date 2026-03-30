# Panel-Level Data Refactor Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Split the data model from "1 message = entire plant with embedded panels array" to "1 message per panel + 1 summary message per plant" for better panel-level analysis and ES query performance.

**Architecture:** Each mock-plant publishes per-panel NATS messages (`plant.{id}.panel.data`) alongside a plant summary (`plant.{id}.summary`). Log files write one line per panel instead of one giant line per plant. ES uses a flat `plant-panel` index instead of nested. Frontend state is restructured to hold panels as a map keyed by panelId.

**Tech Stack:** Go 1.25, NATS, Elasticsearch 8.12, Fluentd, React + TypeScript

---

### Task 1: Add New Models to Shared Package

**Files:**
- Modify: `shared/models/models.go`

**Step 1: Add PanelReading and PlantSummary structs and new message constants**

Add these new types and constants to `shared/models/models.go`. Keep the existing `PlantData` and `PanelData` — we'll remove them after all consumers migrate.

```go
// New: standalone panel reading (includes plant context for ES/NATS independence)
type PanelReading struct {
	PlantID     string    `json:"plantId"`
	PlantName   string    `json:"plantName"`
	PanelID     string    `json:"panelId"`
	PanelNumber int       `json:"panelNumber"`
	Status      string    `json:"status"`
	FaultMode   string    `json:"faultMode,omitempty"`
	Watt        float64   `json:"watt"`
	Timestamp   time.Time `json:"timestamp"`
}

// New: plant-level summary without embedded panels array
type PlantSummary struct {
	PlantID      string    `json:"plantId"`
	PlantName    string    `json:"plantName"`
	Timestamp    time.Time `json:"timestamp"`
	TotalWatt    float64   `json:"totalWatt"`
	PanelCount   int       `json:"panelCount"`
	OnlineCount  int       `json:"onlineCount"`
	OfflineCount int       `json:"offlineCount"`
	FaultyCount  int       `json:"faultyCount"`
}
```

Add new WebSocket message type constants:

```go
// WebSocket message types (server → client)
const (
	MsgPlantData         = "PLANT_DATA"          // deprecated, keep for transition
	MsgPlantSummary      = "PLANT_SUMMARY"       // NEW
	MsgPanelReading      = "PANEL_READING"       // NEW
	MsgPlantRegistered   = "PLANT_REGISTERED"
	MsgPlantUnregistered = "PLANT_UNREGISTERED"
	MsgAlertNew          = "ALERT_NEW"
	MsgAlertResolved     = "ALERT_RESOLVED"
)
```

**Step 2: Verify compilation**

Run: `cd services/alert-service && go build ./... && cd ../ws-gateway && go build ./... && cd ../mock-plant && go build ./... && cd ../plant-manager && go build ./...`
Expected: All compile successfully (no consumers use new types yet)

**Step 3: Commit**

```bash
git add shared/models/models.go
git commit -m "feat(shared): add PanelReading and PlantSummary models for panel-level data"
```

---

### Task 2: Refactor Mock Plant Data Generation

**Files:**
- Modify: `services/mock-plant/plant/plant.go`
- Test: `services/mock-plant/plant/plant_test.go`

**Step 1: Write tests for the new GeneratePanelReadings and GenerateSummary methods**

Add to `services/mock-plant/plant/plant_test.go`:

```go
func TestGeneratePanelReadings(t *testing.T) {
	p := NewPlant("Test", 3, 500)
	readings := p.GeneratePanelReadings()

	if len(readings) != 3 {
		t.Fatalf("expected 3 readings, got %d", len(readings))
	}
	for i, r := range readings {
		if r.PlantID != p.ID {
			t.Errorf("reading %d: wrong plantId", i)
		}
		if r.PlantName != "Test" {
			t.Errorf("reading %d: wrong plantName", i)
		}
		if r.PanelNumber != i+1 {
			t.Errorf("reading %d: expected panelNumber %d, got %d", i, i+1, r.PanelNumber)
		}
		if r.Watt != 500 {
			t.Errorf("reading %d: expected 500 watt, got %.0f", i, r.Watt)
		}
		if r.Timestamp.IsZero() {
			t.Errorf("reading %d: timestamp is zero", i)
		}
	}
}

func TestGenerateSummary(t *testing.T) {
	p := NewPlant("Test", 3, 500)
	summary := p.GenerateSummary()

	if summary.PlantID != p.ID {
		t.Error("wrong plantId")
	}
	if summary.TotalWatt != 1500 {
		t.Errorf("expected 1500 totalWatt, got %.0f", summary.TotalWatt)
	}
	if summary.PanelCount != 3 {
		t.Errorf("expected panelCount 3, got %d", summary.PanelCount)
	}
	if summary.OnlineCount != 3 {
		t.Errorf("expected onlineCount 3, got %d", summary.OnlineCount)
	}
	if summary.FaultyCount != 0 {
		t.Errorf("expected faultyCount 0, got %d", summary.FaultyCount)
	}
}

func TestGenerateSummaryWithFault(t *testing.T) {
	p := NewPlant("Test", 3, 500)
	p.HandleCommand(models.Command{
		Command:   models.CmdFault,
		PanelID:   p.Panels[0].ID,
		FaultMode: models.FaultDead,
	})
	summary := p.GenerateSummary()

	if summary.TotalWatt != 1000 {
		t.Errorf("expected 1000 totalWatt (one dead), got %.0f", summary.TotalWatt)
	}
	if summary.FaultyCount != 1 {
		t.Errorf("expected faultyCount 1, got %d", summary.FaultyCount)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `cd services/mock-plant && go test ./plant/ -run "TestGeneratePanelReadings|TestGenerateSummary" -v`
Expected: FAIL — methods don't exist yet

**Step 3: Implement GeneratePanelReadings and GenerateSummary in plant.go**

Add two new methods to `Plant` in `services/mock-plant/plant/plant.go`:

```go
func (p *Plant) GeneratePanelReadings() []models.PanelReading {
	p.mu.RLock()
	defer p.mu.RUnlock()

	now := time.Now().UTC()
	readings := make([]models.PanelReading, len(p.Panels))
	for i, panel := range p.Panels {
		pd := panel.Generate()
		readings[i] = models.PanelReading{
			PlantID:     p.ID,
			PlantName:   p.Name,
			PanelID:     pd.PanelID,
			PanelNumber: pd.PanelNumber,
			Status:      pd.Status,
			FaultMode:   pd.FaultMode,
			Watt:        pd.Watt,
			Timestamp:   now,
		}
	}
	return readings
}

func (p *Plant) GenerateSummary() models.PlantSummary {
	p.mu.RLock()
	defer p.mu.RUnlock()

	totalWatt := 0.0
	online, offline, faulty := 0, 0, 0
	for _, panel := range p.Panels {
		pd := panel.Generate()
		totalWatt += pd.Watt
		switch {
		case pd.Status == models.StatusOffline:
			offline++
		case pd.FaultMode != models.FaultNone:
			faulty++
			online++
		default:
			online++
		}
	}
	return models.PlantSummary{
		PlantID:      p.ID,
		PlantName:    p.Name,
		Timestamp:    time.Now().UTC(),
		TotalWatt:    totalWatt,
		PanelCount:   len(p.Panels),
		OnlineCount:  online,
		OfflineCount: offline,
		FaultyCount:  faulty,
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `cd services/mock-plant && go test ./plant/ -run "TestGeneratePanelReadings|TestGenerateSummary" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add services/mock-plant/plant/plant.go services/mock-plant/plant/plant_test.go
git commit -m "feat(mock-plant): add GeneratePanelReadings and GenerateSummary methods"
```

---

### Task 3: Refactor Mock Plant NATS Publishing and Logging

**Files:**
- Modify: `services/mock-plant/main.go`
- Modify: `services/mock-plant/logger/logger.go`

**Step 1: Change logger to accept any JSON-serializable value**

Replace the `Write` method signature in `services/mock-plant/logger/logger.go`:

```go
func (l *FileLogger) Write(data any) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	bytes, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = l.file.Write(append(bytes, '\n'))
	return err
}
```

**Step 2: Change main.go publishing loop**

In `services/mock-plant/main.go`, replace the data publishing block inside the `case <-ticker.C:` with:

```go
		case <-ticker.C:
			readings := p.GeneratePanelReadings()
			summary := p.GenerateSummary()

			// Publish each panel reading individually
			panelSubject := fmt.Sprintf("plant.%s.panel.data", p.ID)
			for _, reading := range readings {
				bytes, _ := json.Marshal(reading)
				nc.Publish(panelSubject, bytes)
				// Write each panel as separate log line
				if fileLog != nil {
					fileLog.Write(reading)
				}
			}

			// Publish plant summary
			summarySubject := fmt.Sprintf("plant.%s.summary", p.ID)
			summaryBytes, _ := json.Marshal(summary)
			nc.Publish(summarySubject, summaryBytes)
```

Also remove the old `dataSubject` variable and `GenerateData()` call. Remove the import of `"github.com/solarops/shared/models"` only if it becomes unused (it is still used for `models.Command` in the command handler, so keep it).

**Step 3: Verify compilation**

Run: `cd services/mock-plant && go build ./...`
Expected: Compiles successfully

**Step 4: Commit**

```bash
git add services/mock-plant/main.go services/mock-plant/logger/logger.go
git commit -m "feat(mock-plant): publish per-panel NATS messages and flat log lines"
```

---

### Task 4: Update Alert Service Subscription

**Files:**
- Modify: `services/alert-service/main.go`

**Step 1: Change NATS subscription from plant.*.data to plant.*.panel.data**

In `services/alert-service/main.go`, replace the subscription block:

```go
	// Subscribe to real-time panel data
	nc.Subscribe("plant.*.panel.data", func(msg *nats.Msg) {
		var reading models.PanelReading
		if err := json.Unmarshal(msg.Data, &reading); err != nil {
			return
		}
		det.Feed(reading.PlantID, reading.PanelID, reading.PanelNumber, reading.PlantName, reading.Watt, reading.Timestamp)
		newAlerts := det.Check()
		for _, alert := range newAlerts {
			if _, found := alertStore.FindActive(alert.PlantID, alert.PanelID, alert.Type); found {
				continue
			}
			created := alertStore.Create(alert)
			alertJSON, _ := json.Marshal(created)
			nc.Publish("alert.new", alertJSON)
			log.Printf("New alert: %s - %s", created.Type, created.Message)
		}
	})
	log.Println("Subscribed to plant.*.panel.data")
```

**Step 2: Verify compilation**

Run: `cd services/alert-service && go build ./...`
Expected: Compiles successfully

**Step 3: Commit**

```bash
git add services/alert-service/main.go
git commit -m "refactor(alert-service): subscribe to per-panel NATS messages"
```

---

### Task 5: Update WebSocket Gateway

**Files:**
- Modify: `services/ws-gateway/main.go`

**Step 1: Replace plant.*.data subscription with plant.*.summary and plant.*.panel.data**

In `services/ws-gateway/main.go`, replace the `plant.*.data` subscription with two subscriptions:

```go
	// Subscribe to plant summaries (for dashboard overview)
	nc.Subscribe("plant.*.summary", func(msg *nats.Msg) {
		wsMsg := models.WSMessage{Type: models.MsgPlantSummary, Payload: json.RawMessage(msg.Data)}
		data, _ := json.Marshal(wsMsg)
		h.Broadcast(data)
	})

	// Subscribe to individual panel readings (for detail view)
	nc.Subscribe("plant.*.panel.data", func(msg *nats.Msg) {
		wsMsg := models.WSMessage{Type: models.MsgPanelReading, Payload: json.RawMessage(msg.Data)}
		data, _ := json.Marshal(wsMsg)
		h.Broadcast(data)
	})
```

Remove the old `plant.*.data` subscription.

**Step 2: Verify compilation**

Run: `cd services/ws-gateway && go build ./...`
Expected: Compiles successfully

**Step 3: Commit**

```bash
git add services/ws-gateway/main.go
git commit -m "refactor(ws-gateway): forward panel readings and plant summaries separately"
```

---

### Task 6: Update Elasticsearch Index Template

**Files:**
- Modify: `infra/elasticsearch/init-index.sh`

**Step 1: Replace the nested mapping with a flat panel-reading mapping**

Replace the entire `init-index.sh` with:

```bash
#!/bin/sh
# Wait for ES to be ready
until curl -s http://elasticsearch:9200/_cluster/health | grep -q '"status":"green"\|"status":"yellow"'; do
  echo "Waiting for Elasticsearch..."
  sleep 2
done

# Create flat panel-reading index template
curl -X PUT "http://elasticsearch:9200/_index_template/plant-panel-template" \
  -H "Content-Type: application/json" \
  -d '{
  "index_patterns": ["plant-panel*"],
  "template": {
    "settings": {
      "number_of_shards": 1,
      "number_of_replicas": 0
    },
    "mappings": {
      "properties": {
        "plantId":     { "type": "keyword" },
        "plantName":   { "type": "keyword" },
        "panelId":     { "type": "keyword" },
        "panelNumber": { "type": "integer" },
        "status":      { "type": "keyword" },
        "faultMode":   { "type": "keyword" },
        "watt":        { "type": "float" },
        "timestamp":   { "type": "date" }
      }
    }
  }
}'

echo ""
echo "Index template created."
```

**Step 2: Commit**

```bash
git add infra/elasticsearch/init-index.sh
git commit -m "refactor(infra): flat panel-reading ES index template (no nested)"
```

---

### Task 7: Update Fluentd Config

**Files:**
- Modify: `infra/fluentd/fluent.conf`

**Step 1: Change the ES index name from plant-data to plant-panel**

Replace the entire `fluent.conf`:

```conf
<source>
  @type tail
  path /var/log/plant/data.log
  pos_file /var/log/fluentd-buffers/data.log.pos
  tag plant.panel
  <parse>
    @type json
    time_key timestamp
    time_type string
    time_format %Y-%m-%dT%H:%M:%S.%N%z
  </parse>
  read_from_head true
  refresh_interval 1
</source>

<match plant.panel>
  @type elasticsearch
  host elasticsearch
  port 9200
  index_name plant-panel
  suppress_type_name true
  include_timestamp true

  <buffer>
    @type file
    path /var/log/fluentd-buffers/plant-panel
    flush_interval 1s
    retry_max_interval 30s
    retry_forever true
    chunk_limit_size 2M
    queue_limit_length 32
  </buffer>
</match>
```

**Step 2: Commit**

```bash
git add infra/fluentd/fluent.conf
git commit -m "refactor(fluentd): write to flat plant-panel index"
```

---

### Task 8: Update Plant Manager History Query

**Files:**
- Modify: `services/plant-manager/main.go`

**Step 1: Update the history endpoint to query the new flat index**

In the `GET /api/plants/{plantId}/history` handler in `services/plant-manager/main.go`, change the ES query:

1. Change the index from `"plant-data"` to `"plant-panel"`
2. Change the aggregation field from `"totalWatt"` to `"watt"` and add a sum aggregation across panels

Replace the entire history handler's query and ES call (lines ~206-248) with:

```go
	mux.HandleFunc("GET /api/plants/{plantId}/history", func(w http.ResponseWriter, r *http.Request) {
		plantID := r.PathValue("plantId")
		rangeParam := r.URL.Query().Get("range")
		if rangeParam == "" {
			rangeParam = "1h"
		}
		interval := r.URL.Query().Get("interval")
		if interval == "" {
			interval = "10s"
		}

		query := map[string]interface{}{
			"size": 0,
			"query": map[string]interface{}{
				"bool": map[string]interface{}{
					"filter": []map[string]interface{}{
						{"term": map[string]interface{}{"plantId": plantID}},
						{"range": map[string]interface{}{
							"timestamp": map[string]interface{}{"gte": "now-" + rangeParam},
						}},
					},
				},
			},
			"aggs": map[string]interface{}{
				"over_time": map[string]interface{}{
					"date_histogram": map[string]interface{}{
						"field":          "timestamp",
						"fixed_interval": interval,
					},
					"aggs": map[string]interface{}{
						"total_watt": map[string]interface{}{
							"sum": map[string]interface{}{"field": "watt"},
						},
					},
				},
			},
		}

		var buf bytes.Buffer
		json.NewEncoder(&buf).Encode(query)

		res, err := es.Search(
			es.Search.WithContext(context.Background()),
			es.Search.WithIndex("plant-panel"),
			es.Search.WithBody(&buf),
		)
		if err != nil {
			http.Error(w, "ES query failed", http.StatusInternalServerError)
			return
		}
		defer res.Body.Close()

		w.Header().Set("Content-Type", "application/json")
		io.Copy(w, res.Body)
	})
```

**Step 2: Verify compilation**

Run: `cd services/plant-manager && go build ./...`
Expected: Compiles successfully

**Step 3: Commit**

```bash
git add services/plant-manager/main.go
git commit -m "refactor(plant-manager): query flat plant-panel index for history"
```

---

### Task 9: Update Frontend Types and State

**Files:**
- Modify: `frontend/src/types.ts`
- Modify: `frontend/src/hooks/usePlants.ts`

**Step 1: Update TypeScript types**

Replace `frontend/src/types.ts`:

```typescript
export interface PanelReading {
  plantId: string;
  plantName: string;
  panelId: string;
  panelNumber: number;
  status: "online" | "offline";
  faultMode: string | null;
  watt: number;
  timestamp: string;
}

export interface PlantSummary {
  plantId: string;
  plantName: string;
  timestamp: string;
  totalWatt: number;
  panelCount: number;
  onlineCount: number;
  offlineCount: number;
  faultyCount: number;
}

export interface Alert {
  id: string;
  type: string;
  plantId: string;
  plantName: string;
  panelId?: string;
  panelNumber?: number;
  status: "active" | "acknowledged" | "resolved";
  message: string;
  createdAt: string;
  updatedAt: string;
}

export interface WSMessage {
  type: string;
  payload: unknown;
}

export type PlantStatus = "online" | "fault" | "stale" | "offline";

export interface PlantState {
  summary: PlantSummary | null;
  panels: Record<string, PanelReading>;
  status: PlantStatus;
  lastSeen: number;
}
```

**Step 2: Update usePlants.ts state management**

Replace `frontend/src/hooks/usePlants.ts` `handleMessage` cases with:

```typescript
  const handleMessage = useCallback((msg: WSMessage) => {
    switch (msg.type) {
      case "PLANT_SUMMARY": {
        const summary = msg.payload as PlantSummary;
        setPlants((prev) => ({
          ...prev,
          [summary.plantId]: {
            ...prev[summary.plantId],
            summary,
            panels: prev[summary.plantId]?.panels || {},
            status: summary.faultyCount > 0 ? "fault" : "online",
            lastSeen: Date.now(),
          },
        }));
        break;
      }
      case "PANEL_READING": {
        const reading = msg.payload as PanelReading;
        setPlants((prev) => {
          const existing = prev[reading.plantId];
          return {
            ...prev,
            [reading.plantId]: {
              summary: existing?.summary || null,
              panels: {
                ...(existing?.panels || {}),
                [reading.panelId]: reading,
              },
              status: existing?.status || "online",
              lastSeen: Date.now(),
            },
          };
        });
        break;
      }
      case "PLANT_REGISTERED": {
        const info = msg.payload as { plantId: string; plantName: string };
        setPlants((prev) => ({
          ...prev,
          [info.plantId]: {
            summary: null,
            panels: {},
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
```

Also update the stale/offline check `useEffect` to work with the new shape — replace `state.status` references (they remain the same, no change needed there).

**Step 3: Commit**

```bash
git add frontend/src/types.ts frontend/src/hooks/usePlants.ts
git commit -m "refactor(frontend): update types and state for panel-level data"
```

---

### Task 10: Update Frontend Components

**Files:**
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/components/PlantCard.tsx`
- Modify: `frontend/src/components/PanelGrid.tsx`
- Modify: `frontend/src/pages/Dashboard.tsx`
- Modify: `frontend/src/pages/PlantDetail.tsx`

**Step 1: Update App.tsx**

In `App.tsx`, change the `useEffect` interval that samples total watt from reading `s.data?.totalWatt` to `s.summary?.totalWatt`:

```typescript
const totalWatt = Object.values(plantsRef.current).reduce(
  (sum, s) => sum + (s.summary?.totalWatt || 0),
  0
);
```

**Step 2: Update PlantCard.tsx**

Read the current `PlantCard.tsx` and update it to use `PlantState` with `summary` and `panels` instead of `data`:

- Replace `state.data.totalWatt` → `state.summary?.totalWatt ?? 0`
- Replace `state.data.panels.length` → `state.summary?.panelCount ?? 0`
- Replace `state.data.onlineCount` → `state.summary?.onlineCount ?? 0`
- Replace `state.data.faultyCount` → `state.summary?.faultyCount ?? 0`
- Replace `state.data.plantName` → `state.summary?.plantName ?? ""`

**Step 3: Update PanelGrid.tsx**

Change the `panels` prop type from `PanelData[]` to `PanelReading[]`. Update internal references:
- The shape is almost the same — `panelId`, `panelNumber`, `status`, `faultMode`, `watt` all exist on `PanelReading`

**Step 4: Update Dashboard.tsx**

Update the Dashboard props and rendering to use the new `PlantState` shape. The `Dashboard` receives `plants: Record<string, PlantState>` — update accesses from `state.data.*` to `state.summary.*`.

**Step 5: Update PlantDetail.tsx**

Change how panels are accessed:

```typescript
// OLD:
const panels = state?.data?.panels || [];

// NEW:
const panels = state ? Object.values(state.panels) : [];
```

And update any `state.data.plantName` → `state.summary?.plantName`.

**Step 6: Verify frontend builds**

Run: `cd frontend && npm run build`
Expected: Build succeeds with no TypeScript errors

**Step 7: Commit**

```bash
git add frontend/src/App.tsx frontend/src/components/PlantCard.tsx frontend/src/components/PanelGrid.tsx frontend/src/pages/Dashboard.tsx frontend/src/pages/PlantDetail.tsx
git commit -m "refactor(frontend): adapt components to panel-level data model"
```

---

### Task 11: Clean Up Old Models

**Files:**
- Modify: `shared/models/models.go`

**Step 1: Remove deprecated types and constants**

Remove `PlantData` struct and `MsgPlantData` constant from `shared/models/models.go`. Keep `PanelData` only if still used by mock-plant's `Panel.Generate()` internally — if so, add a comment marking it as internal.

**Step 2: Verify all services compile**

Run: `cd services/mock-plant && go build ./... && cd ../alert-service && go build ./... && cd ../ws-gateway && go build ./... && cd ../plant-manager && go build ./...`
Expected: All compile

**Step 3: Commit**

```bash
git add shared/models/models.go
git commit -m "refactor(shared): remove deprecated PlantData type"
```

---

### Task 12: Rebuild, Deploy, and Verify

**Step 1: Delete old ES index and volumes to start clean**

```bash
docker compose down -v
```

**Step 2: Rebuild all services**

```bash
docker compose build
```

**Step 3: Start all services**

```bash
docker compose up -d
```

**Step 4: Wait for startup and verify**

```bash
sleep 30

# Check all services running
docker compose ps

# Check plants registered
curl -s http://localhost:8082/api/plants | python3 -m json.tool

# Check ES uses flat index
curl -s 'http://localhost:9200/plant-panel/_count'

# Check a single panel document is flat
curl -s 'http://localhost:9200/plant-panel/_search?size=1' | python3 -m json.tool

# Check frontend loads
curl -s -o /dev/null -w "%{http_code}" http://localhost:3000

# Trigger a fault and verify alert
PLANT_ID=$(curl -s http://localhost:8082/api/plants | python3 -c "import json,sys; print(json.load(sys.stdin)[0]['plantId'])")
PANEL_ID=$(curl -s "http://localhost:9200/plant-panel/_search?size=1&q=plantId:$PLANT_ID" | python3 -c "import json,sys; print(json.load(sys.stdin)['hits']['hits'][0]['_source']['panelId'])")
curl -s -X POST "http://localhost:8082/api/plants/$PLANT_ID/panels/$PANEL_ID/fault" -H "Content-Type: application/json" -d '{"mode":"dead"}'
sleep 5
curl -s http://localhost:8081/api/alerts | python3 -m json.tool
```

Expected: All services running, flat ES documents, alerts triggered on fault.

**Step 5: Commit**

No code changes — just verification.
