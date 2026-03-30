# NATS/ES Separation & Daily Index Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Cleanly separate NATS (events only) from ES (all data). Add daily index rotation for `plant-panel` and `plant-summary`. Add aggregator service that reads from ES and writes summaries. Frontend polls ES via plant-manager API instead of receiving data via WebSocket.

**Architecture:** Fluentd writes per-panel readings to `plant-panel-YYYY-MM-DD`. A new Go aggregator service queries ES every 10s, computes per-plant summaries, and writes to `plant-summary-YYYY-MM-DD`. Frontend gets data by polling plant-manager API (which queries ES), and only uses WebSocket for events (alerts, plant status, commands). ws-gateway stops forwarding data subjects.

**Tech Stack:** Go 1.25, NATS, Elasticsearch 8.12, Fluentd, React + TypeScript

---

### Task 1: Update ES Index Templates for Daily Rotation

**Files:**
- Modify: `infra/elasticsearch/init-index.sh`

**Step 1: Replace the single index template with two daily-rotated templates**

Replace `infra/elasticsearch/init-index.sh` entirely:

```bash
#!/bin/sh
# Wait for ES to be ready
until curl -s http://elasticsearch:9200/_cluster/health | grep -q '"status":"green"\|"status":"yellow"'; do
  echo "Waiting for Elasticsearch..."
  sleep 2
done

# Panel-level readings (written by Fluentd): plant-panel-YYYY-MM-DD
curl -X PUT "http://elasticsearch:9200/_index_template/plant-panel-template" \
  -H "Content-Type: application/json" \
  -d '{
  "index_patterns": ["plant-panel-*"],
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

# Plant-level summaries (written by aggregator): plant-summary-YYYY-MM-DD
curl -X PUT "http://elasticsearch:9200/_index_template/plant-summary-template" \
  -H "Content-Type: application/json" \
  -d '{
  "index_patterns": ["plant-summary-*"],
  "template": {
    "settings": {
      "number_of_shards": 1,
      "number_of_replicas": 0
    },
    "mappings": {
      "properties": {
        "plantId":      { "type": "keyword" },
        "plantName":    { "type": "keyword" },
        "timestamp":    { "type": "date" },
        "totalWatt":    { "type": "float" },
        "panelCount":   { "type": "integer" },
        "onlineCount":  { "type": "integer" },
        "offlineCount": { "type": "integer" },
        "faultyCount":  { "type": "integer" }
      }
    }
  }
}'

echo ""
echo "Index templates created."
```

**Step 2: Commit**

```bash
git add infra/elasticsearch/init-index.sh
git commit -m "refactor(infra): daily-rotated ES index templates for panel and summary"
```

---

### Task 2: Update Fluentd to Write Daily Index

**Files:**
- Modify: `infra/fluentd/fluent.conf`

**Step 1: Change Fluentd to write to daily-rotated index**

Replace `infra/fluentd/fluent.conf`:

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
  index_date_pattern now/d{-YYYY-MM-dd}
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

Note: `index_date_pattern now/d{-YYYY-MM-dd}` with `index_name plant-panel` produces `plant-panel-2026-03-30`.

**Step 2: Commit**

```bash
git add infra/fluentd/fluent.conf
git commit -m "refactor(fluentd): write to daily-rotated plant-panel-YYYY-MM-DD index"
```

---

### Task 3: Create Aggregator Service

**Files:**
- Create: `services/aggregator/main.go`
- Create: `services/aggregator/go.mod`
- Create: `services/aggregator/Dockerfile`

**Step 1: Create go.mod**

```bash
cd services/aggregator
go mod init github.com/solarops/aggregator
```

Add dependencies: `github.com/elastic/go-elasticsearch/v8`.

**Step 2: Write main.go**

The aggregator runs a loop every 10 seconds:
1. Query ES `plant-panel-*` for the last 10 seconds, grouped by `plantId`
2. For each plant, compute: totalWatt (sum of watt), panelCount (cardinality of panelId), onlineCount, offlineCount, faultyCount
3. Write each plant summary as a document to `plant-summary-YYYY-MM-DD`

```go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
)

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

type PlantBucket struct {
	Key       string `json:"key"`
	DocCount  int    `json:"doc_count"`
	SumWatt   struct{ Value float64 } `json:"sum_watt"`
	PlantName struct {
		Buckets []struct{ Key string } `json:"buckets"`
	} `json:"plant_name"`
	StatusCounts struct {
		Buckets []struct {
			Key      string `json:"key"`
			DocCount int    `json:"doc_count"`
		} `json:"buckets"`
	} `json:"status_counts"`
	FaultyCounts struct {
		DocCount int `json:"doc_count"`
	} `json:"faulty_count"`
	PanelCount struct{ Value int } `json:"panel_count"`
}

func main() {
	esURL := envOrDefault("ES_URL", "http://localhost:9200")
	interval := 10 * time.Second

	es, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{esURL},
	})
	if err != nil {
		log.Fatalf("ES connect: %v", err)
	}

	log.Printf("Aggregator started (ES: %s, interval: %s)", esURL, interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Wait a bit for initial data before first aggregation
	for {
		select {
		case <-ticker.C:
			aggregate(es)
		case <-sigCh:
			log.Println("Shutting down...")
			return
		}
	}
}

func aggregate(es *elasticsearch.Client) {
	// Query last 10 seconds of panel data, grouped by plantId
	query := map[string]interface{}{
		"size": 0,
		"query": map[string]interface{}{
			"range": map[string]interface{}{
				"timestamp": map[string]interface{}{"gte": "now-10s"},
			},
		},
		"aggs": map[string]interface{}{
			"by_plant": map[string]interface{}{
				"terms": map[string]interface{}{
					"field": "plantId",
					"size":  100,
				},
				"aggs": map[string]interface{}{
					"sum_watt": map[string]interface{}{
						"sum": map[string]interface{}{"field": "watt"},
					},
					"plant_name": map[string]interface{}{
						"terms": map[string]interface{}{
							"field": "plantName",
							"size":  1,
						},
					},
					"panel_count": map[string]interface{}{
						"cardinality": map[string]interface{}{"field": "panelId"},
					},
					"status_counts": map[string]interface{}{
						"terms": map[string]interface{}{
							"field": "status",
							"size":  10,
						},
					},
					"faulty_count": map[string]interface{}{
						"filter": map[string]interface{}{
							"exists": map[string]interface{}{"field": "faultMode"},
						},
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(query)

	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Second)
	defer cancel()

	res, err := es.Search(
		es.Search.WithContext(ctx),
		es.Search.WithIndex("plant-panel-*"),
		es.Search.WithBody(&buf),
	)
	if err != nil {
		log.Printf("ES query error: %v", err)
		return
	}
	defer res.Body.Close()

	if res.IsError() {
		body, _ := io.ReadAll(res.Body)
		log.Printf("ES error: %s", body)
		return
	}

	var result struct {
		Aggregations struct {
			ByPlant struct {
				Buckets []json.RawMessage `json:"buckets"`
			} `json:"by_plant"`
		} `json:"aggregations"`
	}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		log.Printf("ES decode error: %v", err)
		return
	}

	now := time.Now().UTC()
	indexName := fmt.Sprintf("plant-summary-%s", now.Format("2006-01-02"))

	for _, raw := range result.Aggregations.ByPlant.Buckets {
		var bucket PlantBucket
		if err := json.Unmarshal(raw, &bucket); err != nil {
			continue
		}

		plantName := ""
		if len(bucket.PlantName.Buckets) > 0 {
			plantName = bucket.PlantName.Buckets[0].Key
		}

		online, offline := 0, 0
		for _, s := range bucket.StatusCounts.Buckets {
			switch s.Key {
			case "online":
				online = s.DocCount
			case "offline":
				offline = s.DocCount
			}
		}

		summary := map[string]interface{}{
			"plantId":      bucket.Key,
			"plantName":    plantName,
			"timestamp":    now.Format(time.RFC3339),
			"totalWatt":    bucket.SumWatt.Value,
			"panelCount":   bucket.PanelCount.Value,
			"onlineCount":  online,
			"offlineCount": offline,
			"faultyCount":  bucket.FaultyCounts.DocCount,
		}

		var docBuf bytes.Buffer
		json.NewEncoder(&docBuf).Encode(summary)

		_, err := es.Index(indexName, &docBuf,
			es.Index.WithContext(context.Background()),
		)
		if err != nil {
			log.Printf("ES index error for plant %s: %v", bucket.Key, err)
		}
	}
}
```

**Step 3: Create Dockerfile**

Create `services/aggregator/Dockerfile`:

```dockerfile
FROM golang:1.25-alpine AS build
WORKDIR /app
COPY go.work go.work.sum ./
COPY shared/ shared/
COPY services/aggregator/ services/aggregator/
RUN cd services/aggregator && go build -o /aggregator .

FROM alpine:3.20
COPY --from=build /aggregator /aggregator
CMD ["/aggregator"]
```

**Step 4: Verify compilation**

```bash
cd services/aggregator && go build ./...
```

**Step 5: Commit**

```bash
git add services/aggregator/
git commit -m "feat(aggregator): new service reads ES panel data, writes plant summaries"
```

---

### Task 4: Add Aggregator to Docker Compose and Go Workspace

**Files:**
- Modify: `docker-compose.yml`
- Modify: `go.work`

**Step 1: Add aggregator to go.work**

Add `./services/aggregator` to the `use` block in `go.work`.

**Step 2: Add aggregator service to docker-compose.yml**

Add after `plant-manager` service:

```yaml
  aggregator:
    build:
      context: .
      dockerfile: services/aggregator/Dockerfile
    environment:
      - ES_URL=http://elasticsearch:9200
    depends_on:
      elasticsearch:
        condition: service_healthy
```

**Step 3: Commit**

```bash
git add docker-compose.yml go.work
git commit -m "feat(compose): add aggregator service to orchestration"
```

---

### Task 5: Add Plant-Manager API Endpoints for Frontend Polling

**Files:**
- Modify: `services/plant-manager/main.go`

**Step 1: Add endpoint to get latest plant summaries**

Add a new handler `GET /api/plants/summary` that queries ES `plant-summary-*` for the latest summary per plant:

```go
	// Latest summary per plant (for dashboard polling)
	mux.HandleFunc("GET /api/plants/summary", func(w http.ResponseWriter, r *http.Request) {
		query := map[string]interface{}{
			"size": 0,
			"query": map[string]interface{}{
				"range": map[string]interface{}{
					"timestamp": map[string]interface{}{"gte": "now-30s"},
				},
			},
			"aggs": map[string]interface{}{
				"by_plant": map[string]interface{}{
					"terms": map[string]interface{}{
						"field": "plantId",
						"size":  100,
					},
					"aggs": map[string]interface{}{
						"latest": map[string]interface{}{
							"top_hits": map[string]interface{}{
								"size": 1,
								"sort": []map[string]interface{}{
									{"timestamp": "desc"},
								},
							},
						},
					},
				},
			},
		}

		var buf bytes.Buffer
		json.NewEncoder(&buf).Encode(query)

		res, err := es.Search(
			es.Search.WithContext(context.Background()),
			es.Search.WithIndex("plant-summary-*"),
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

**Step 2: Add endpoint to get latest panel readings for a plant**

Add `GET /api/plants/{plantId}/panels`:

```go
	// Latest panel readings for a plant (for detail view polling)
	mux.HandleFunc("GET /api/plants/{plantId}/panels", func(w http.ResponseWriter, r *http.Request) {
		plantID := r.PathValue("plantId")

		query := map[string]interface{}{
			"size": 0,
			"query": map[string]interface{}{
				"bool": map[string]interface{}{
					"filter": []map[string]interface{}{
						{"term": map[string]interface{}{"plantId": plantID}},
						{"range": map[string]interface{}{
							"timestamp": map[string]interface{}{"gte": "now-10s"},
						}},
					},
				},
			},
			"aggs": map[string]interface{}{
				"by_panel": map[string]interface{}{
					"terms": map[string]interface{}{
						"field": "panelId",
						"size":  100,
					},
					"aggs": map[string]interface{}{
						"latest": map[string]interface{}{
							"top_hits": map[string]interface{}{
								"size": 1,
								"sort": []map[string]interface{}{
									{"timestamp": "desc"},
								},
							},
						},
					},
				},
			},
		}

		var buf bytes.Buffer
		json.NewEncoder(&buf).Encode(query)

		res, err := es.Search(
			es.Search.WithContext(context.Background()),
			es.Search.WithIndex("plant-panel-*"),
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

**Step 3: Update history endpoint to use wildcard index**

Change `es.Search.WithIndex("plant-panel")` to `es.Search.WithIndex("plant-panel-*")` in the existing history handler.

**Step 4: Verify compilation**

```bash
cd services/plant-manager && go build ./...
```

**Step 5: Commit**

```bash
git add services/plant-manager/main.go
git commit -m "feat(plant-manager): add summary and panels polling endpoints, use wildcard index"
```

---

### Task 6: Slim Down ws-gateway (Events Only)

**Files:**
- Modify: `services/ws-gateway/main.go`

**Step 1: Remove data subscriptions**

Remove these two NATS subscriptions from ws-gateway:
- `plant.*.summary` → `PLANT_SUMMARY`
- `plant.*.panel.data` → `PANEL_READING`

Keep:
- `plant.*.status` → `PLANT_REGISTERED` / `PLANT_UNREGISTERED`
- `alert.>` → `ALERT_NEW` / `ALERT_RESOLVED`
- WebSocket → NATS command forwarding (PANEL_OFFLINE, PANEL_ONLINE, PANEL_RESET)

**Step 2: Verify compilation**

```bash
cd services/ws-gateway && go build ./...
```

**Step 3: Commit**

```bash
git add services/ws-gateway/main.go
git commit -m "refactor(ws-gateway): remove data forwarding, keep events only"
```

---

### Task 7: Update Frontend Types and State for Polling

**Files:**
- Modify: `frontend/src/types.ts`
- Modify: `frontend/src/hooks/usePlants.ts`
- Modify: `frontend/src/App.tsx`

**Step 1: Update usePlants.ts**

Remove `PLANT_SUMMARY` and `PANEL_READING` cases from `handleMessage`. They are no longer pushed via WebSocket.

Add a polling `useEffect` that fetches `/api/plants/summary` every 3 seconds and updates plant state:

```typescript
  // Poll plant summaries from ES via plant-manager API
  useEffect(() => {
    const poll = async () => {
      try {
        const res = await fetch("/api/plants/summary");
        const data = await res.json();
        const buckets = data?.aggregations?.by_plant?.buckets || [];
        setPlants((prev) => {
          const next = { ...prev };
          for (const bucket of buckets) {
            const hit = bucket.latest?.hits?.hits?.[0]?._source;
            if (!hit) continue;
            next[hit.plantId] = {
              summary: hit,
              panels: prev[hit.plantId]?.panels || {},
              status: hit.faultyCount > 0 ? "fault" : "online",
              lastSeen: Date.now(),
            };
          }
          return next;
        });
      } catch {}
    };

    poll(); // initial fetch
    const interval = setInterval(poll, 3000);
    return () => clearInterval(interval);
  }, []);
```

**Step 2: Update App.tsx**

Remove the `powerHistoryRef` / `setInterval(10_000)` for dashboard chart. The dashboard chart will now also poll from ES (handled in Dashboard component).

Simplify `onMessage` — it only handles alerts and plant registration now.

Remove `powerHistory` prop from `<Dashboard>`.

**Step 3: Commit**

```bash
git add frontend/src/types.ts frontend/src/hooks/usePlants.ts frontend/src/App.tsx
git commit -m "refactor(frontend): poll ES for data, WebSocket for events only"
```

---

### Task 8: Update Frontend Components for Polling

**Files:**
- Modify: `frontend/src/pages/Dashboard.tsx`
- Modify: `frontend/src/pages/PlantDetail.tsx`

**Step 1: Update Dashboard.tsx**

Remove `powerHistory` prop. Add a polling `useEffect` to fetch total power history from ES via `/api/plants/summary/history` (or compute from existing `/api/plants/{id}/history`).

Alternatively, use the existing plant summary data to build the chart from polled snapshots.

**Step 2: Update PlantDetail.tsx**

Add polling for panel data: fetch `/api/plants/{plantId}/panels` every 2-3 seconds and update panel display.

Remove the `useEffect` that relied on `state?.summary?.timestamp` WebSocket push for chart updates — instead fetch from ES history.

**Step 3: Update nginx.conf if needed**

Ensure `/api/plants` proxy covers the new endpoints (it already does since they're under `/api/plants`).

**Step 4: Verify frontend builds**

```bash
cd frontend && npx tsc --noEmit
```

**Step 5: Commit**

```bash
git add frontend/src/pages/Dashboard.tsx frontend/src/pages/PlantDetail.tsx
git commit -m "refactor(frontend): Dashboard and PlantDetail poll ES for data"
```

---

### Task 9: Update Smoke Test

**Files:**
- Modify: `scripts/smoke-test.sh`

**Step 1: Update ES checks**

Change `plant-panel/_count` to `plant-panel-*/_count`. Add check for `plant-summary-*/_count`.

**Step 2: Commit**

```bash
git add scripts/smoke-test.sh
git commit -m "fix(scripts): update smoke test for daily index pattern"
```

---

### Task 10: Rebuild, Deploy, and Verify

**Step 1: Clean old data**

```bash
docker compose down -v
```

**Step 2: Rebuild**

```bash
docker compose build
```

**Step 3: Start**

```bash
docker compose up -d
```

**Step 4: Verify**

```bash
sleep 30
bash scripts/smoke-test.sh

# Verify daily index exists
curl -s "http://localhost:9200/_cat/indices/plant-*?v"

# Verify aggregator produced summaries
curl -s "http://localhost:9200/plant-summary-*/_count"

# Verify frontend polling works
curl -s "http://localhost:8082/api/plants/summary" | python3 -m json.tool
```
