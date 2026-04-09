# Remove Aggregator: Direct Panel Query Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the aggregator service entirely. Three plant-manager API endpoints query `plant-panel-*` directly with on-demand aggregation.

**Architecture:** Eliminate the `plant-summary-*` index and the aggregator service. `/api/power/history` and `/api/plants/{id}/history` query raw panel data with `date_histogram + sum(watt)`. `/api/plants/summary` is replaced by `/api/plants/current` which uses `terms + top_hits` to get each panel's latest reading and reduces to per-plant summaries in Go. Both history endpoints apply a 5-second watermark to avoid showing partial Fluentd-buffered data.

**Tech Stack:** Go (plant-manager), Elasticsearch, React 19, TypeScript

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `services/plant-manager/main.go` | Modify | Rewrite 3 endpoints to query `plant-panel-*` |
| `frontend/src/hooks/usePlants.ts` | Modify | Change URL to `/api/plants/current`, parse new flat array response |
| `services/aggregator/` | Delete | Entire directory |
| `docker-compose.yml` | Modify | Remove `aggregator` service block |
| `infra/elasticsearch/init-index.sh` | Modify | Remove `plant-summary-policy` ILM and `plant-summary-template` |
| `docs/architecture.md` | Modify | Remove aggregator from data flow |
| `docs/future-improvements.md` | Modify | Mark Option B as implemented |

---

### Task 1: plant-manager — rewrite both history endpoints to query plant-panel-*

**Files:**
- Modify: `services/plant-manager/main.go`

- [ ] **Step 1: Rewrite `/api/plants/{plantId}/history` handler**

In `services/plant-manager/main.go`, find the `/api/plants/{plantId}/history` handler (currently lines 107-162). Replace the handler body with the version below.

Locate this code:

```go
	mux.HandleFunc("GET /api/plants/{plantId}/history", func(w http.ResponseWriter, r *http.Request) {
		plantID := r.PathValue("plantId")
		rangeParam := r.URL.Query().Get("range")
		if rangeParam == "" {
			rangeParam = "1h"
		}
		interval := r.URL.Query().Get("interval")
		if interval == "" {
			interval = "1s"
		}

		query := map[string]interface{}{
			"size": 0,
			"query": map[string]interface{}{
				"bool": map[string]interface{}{
					"filter": []map[string]interface{}{
						{"term": map[string]interface{}{"plantId": plantID}},
						{"range": map[string]interface{}{
							"@timestamp": map[string]interface{}{"gte": "now-" + rangeParam},
						}},
					},
				},
			},
			"aggs": map[string]interface{}{
				"over_time": map[string]interface{}{
					"date_histogram": map[string]interface{}{
						"field":          "@timestamp",
						"fixed_interval": interval,
						"min_doc_count":  1,
					},
					"aggs": map[string]interface{}{
						"total_watt": map[string]interface{}{
							"sum": map[string]interface{}{"field": "totalWatt"},
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

Replace with:

```go
	mux.HandleFunc("GET /api/plants/{plantId}/history", func(w http.ResponseWriter, r *http.Request) {
		plantID := r.PathValue("plantId")
		rangeParam := r.URL.Query().Get("range")
		if rangeParam == "" {
			rangeParam = "5m"
		}
		interval := r.URL.Query().Get("interval")
		if interval == "" {
			interval = "1s"
		}

		// Query plant-panel-* directly with sum(watt). 5-second watermark
		// (lt: now-5s) hides partial Fluentd-buffered data near "now".
		query := map[string]interface{}{
			"size": 0,
			"query": map[string]interface{}{
				"bool": map[string]interface{}{
					"filter": []map[string]interface{}{
						{"term": map[string]interface{}{"plantId": plantID}},
						{"range": map[string]interface{}{
							"@timestamp": map[string]interface{}{
								"gte": "now-" + rangeParam + "-5s",
								"lt":  "now-5s",
							},
						}},
					},
				},
			},
			"aggs": map[string]interface{}{
				"over_time": map[string]interface{}{
					"date_histogram": map[string]interface{}{
						"field":          "@timestamp",
						"fixed_interval": interval,
						"min_doc_count":  1,
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

Key changes vs the previous version:
- Default `range` changed from `"1h"` to `"5m"` (matches frontend expectations)
- Time filter now uses both `gte` (`now-{range}-5s`) and `lt` (`now-5s`) for the 5s watermark
- `sum(totalWatt)` → `sum(watt)` (panel-level field)
- Index changed from `plant-summary-*` to `plant-panel-*`

- [ ] **Step 2: Rewrite `/api/power/history` handler**

Locate this code (currently lines 165-214):

```go
	// Total power history across all plants (for Dashboard)
	mux.HandleFunc("GET /api/power/history", func(w http.ResponseWriter, r *http.Request) {
		rangeParam := r.URL.Query().Get("range")
		if rangeParam == "" {
			rangeParam = "5m"
		}
		interval := r.URL.Query().Get("interval")
		if interval == "" {
			interval = "1s"
		}

		query := map[string]interface{}{
			"size": 0,
			"query": map[string]interface{}{
				"range": map[string]interface{}{
					"@timestamp": map[string]interface{}{"gte": "now-" + rangeParam},
				},
			},
			"aggs": map[string]interface{}{
				"over_time": map[string]interface{}{
					"date_histogram": map[string]interface{}{
						"field":          "@timestamp",
						"fixed_interval": interval,
						"min_doc_count":  1,
					},
					"aggs": map[string]interface{}{
						"total_watt": map[string]interface{}{
							"sum": map[string]interface{}{"field": "totalWatt"},
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

Replace with:

```go
	// Total power history across all plants (for Dashboard)
	mux.HandleFunc("GET /api/power/history", func(w http.ResponseWriter, r *http.Request) {
		rangeParam := r.URL.Query().Get("range")
		if rangeParam == "" {
			rangeParam = "5m"
		}
		interval := r.URL.Query().Get("interval")
		if interval == "" {
			interval = "1s"
		}

		// Query plant-panel-* directly with sum(watt). 5-second watermark
		// (lt: now-5s) hides partial Fluentd-buffered data near "now".
		query := map[string]interface{}{
			"size": 0,
			"query": map[string]interface{}{
				"range": map[string]interface{}{
					"@timestamp": map[string]interface{}{
						"gte": "now-" + rangeParam + "-5s",
						"lt":  "now-5s",
					},
				},
			},
			"aggs": map[string]interface{}{
				"over_time": map[string]interface{}{
					"date_histogram": map[string]interface{}{
						"field":          "@timestamp",
						"fixed_interval": interval,
						"min_doc_count":  1,
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

- [ ] **Step 3: Verify plant-manager compiles**

Run: `cd services/plant-manager && go build -o /dev/null .`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add services/plant-manager/main.go
git commit -m "refactor(plant-manager): query plant-panel-* directly for history endpoints

Both /api/power/history and /api/plants/{id}/history now query raw
panel data with sum(watt) instead of pre-aggregated plant-summary-*.
Adds 5-second watermark to hide partial Fluentd-buffered data."
```

---

### Task 2: plant-manager — replace `/api/plants/summary` with `/api/plants/current`

**Files:**
- Modify: `services/plant-manager/main.go`

- [ ] **Step 1: Replace the handler**

Locate this code (currently lines 216-261) — the `/api/plants/summary` handler:

```go
	// Latest summary per plant (for dashboard polling)
	mux.HandleFunc("GET /api/plants/summary", func(w http.ResponseWriter, r *http.Request) {
		query := map[string]interface{}{
			"size": 0,
			"query": map[string]interface{}{
				"range": map[string]interface{}{
					"@timestamp": map[string]interface{}{"gte": "now-30s"},
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
									{"@timestamp": "desc"},
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

Replace with the new `/api/plants/current` handler. This queries `plant-panel-*` to get each panel's latest reading, then reduces to per-plant summaries in Go:

```go
	// Current state per plant (for dashboard polling).
	// Queries plant-panel-* with terms+top_hits to get each panel's latest
	// reading, then reduces to per-plant summaries in Go.
	mux.HandleFunc("GET /api/plants/current", func(w http.ResponseWriter, r *http.Request) {
		query := map[string]interface{}{
			"size": 0,
			"query": map[string]interface{}{
				"range": map[string]interface{}{
					"@timestamp": map[string]interface{}{"gte": "now-30s"},
				},
			},
			"aggs": map[string]interface{}{
				"by_plant": map[string]interface{}{
					"terms": map[string]interface{}{
						"field": "plantId",
						"size":  100,
					},
					"aggs": map[string]interface{}{
						"by_panel": map[string]interface{}{
							"terms": map[string]interface{}{
								"field": "panelId",
								"size":  200,
							},
							"aggs": map[string]interface{}{
								"latest": map[string]interface{}{
									"top_hits": map[string]interface{}{
										"size": 1,
										"sort": []map[string]interface{}{
											{"@timestamp": "desc"},
										},
									},
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

		// Parse the ES response and reduce to per-plant summaries.
		var esResp struct {
			Aggregations struct {
				ByPlant struct {
					Buckets []struct {
						Key     string `json:"key"`
						ByPanel struct {
							Buckets []struct {
								Latest struct {
									Hits struct {
										Hits []struct {
											Source models.PanelReading `json:"_source"`
										} `json:"hits"`
									} `json:"hits"`
								} `json:"latest"`
							} `json:"buckets"`
						} `json:"by_panel"`
					} `json:"buckets"`
				} `json:"by_plant"`
			} `json:"aggregations"`
		}
		if err := json.NewDecoder(res.Body).Decode(&esResp); err != nil {
			http.Error(w, "ES decode failed", http.StatusInternalServerError)
			return
		}

		summaries := make([]models.PlantSummary, 0, len(esResp.Aggregations.ByPlant.Buckets))
		for _, plantBucket := range esResp.Aggregations.ByPlant.Buckets {
			summary := models.PlantSummary{
				PlantID:    plantBucket.Key,
				PanelCount: len(plantBucket.ByPanel.Buckets),
			}
			var latestTimestamp time.Time
			for _, panelBucket := range plantBucket.ByPanel.Buckets {
				if len(panelBucket.Latest.Hits.Hits) == 0 {
					continue
				}
				reading := panelBucket.Latest.Hits.Hits[0].Source
				if summary.PlantName == "" {
					summary.PlantName = reading.PlantName
				}
				if reading.Timestamp.After(latestTimestamp) {
					latestTimestamp = reading.Timestamp
				}
				summary.TotalWatt += reading.Watt
				switch reading.Status {
				case models.StatusOnline:
					summary.OnlineCount++
				case models.StatusOffline:
					summary.OfflineCount++
				}
				if reading.FaultMode != models.FaultNone {
					summary.FaultyCount++
				}
			}
			summary.Timestamp = latestTimestamp
			summaries = append(summaries, summary)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"plants": summaries})
	})
```

- [ ] **Step 2: Verify plant-manager compiles**

Run: `cd services/plant-manager && go build -o /dev/null .`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add services/plant-manager/main.go
git commit -m "refactor(plant-manager): replace /api/plants/summary with /api/plants/current

New endpoint queries plant-panel-* with terms+top_hits and reduces
to per-plant PlantSummary in Go. Returns flat array {plants: []}
instead of raw ES aggregation structure."
```

---

### Task 3: Frontend — update usePlants hook

**Files:**
- Modify: `frontend/src/hooks/usePlants.ts`

- [ ] **Step 1: Update fetch URL and response parsing**

In `frontend/src/hooks/usePlants.ts`, find the polling effect (currently around lines 13-43). Replace the fetch and `setPlants` block.

Find this code:

```typescript
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
```

Replace with:

```typescript
  // Poll plant current state from plant-manager API
  useEffect(() => {
    const poll = async () => {
      try {
        const res = await fetch("/api/plants/current");
        const data: { plants: PlantSummary[] } = await res.json();

        setPlants((prev) => {
          const next = { ...prev };
          for (const summary of data.plants || []) {
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
```

- [ ] **Step 2: Verify frontend compiles**

Run: `cd frontend && npx tsc -p tsconfig.app.json --noEmit`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add frontend/src/hooks/usePlants.ts
git commit -m "refactor(frontend): usePlants fetches /api/plants/current

Replaces ES aggregation parsing with simple {plants: []} array shape.
Backend now returns pre-reduced PlantSummary objects."
```

---

### Task 4: Cleanup — remove aggregator service

**Files:**
- Delete: `services/aggregator/` (entire directory)
- Modify: `docker-compose.yml`
- Modify: `infra/elasticsearch/init-index.sh`

- [ ] **Step 1: Delete the aggregator directory**

Run:

```bash
rm -rf /home/zc/projects/solarops/services/aggregator
```

- [ ] **Step 2: Remove the aggregator service from docker-compose.yml**

In `docker-compose.yml`, find this block:

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

Delete the entire block (including the blank line before the next service, but keep the section comments intact).

- [ ] **Step 3: Remove plant-summary ILM and template from init-index.sh**

In `infra/elasticsearch/init-index.sh`, delete two blocks.

First block (the ILM policy, currently lines 23-34):

```sh
# Summary data: low volume (~8640 docs/day per plant), keep 30 days
curl -X PUT "http://elasticsearch:9200/_ilm/policy/plant-summary-policy" \
  -H "Content-Type: application/json" \
  -d '{
    "policy": {
      "phases": {
        "hot":    { "min_age": "0ms", "actions": {} },
        "delete": { "min_age": "30d", "actions": { "delete": {} } }
      }
    }
  }'
echo ""
```

Second block (the index template, currently lines 68-96):

```sh
# Plant-level summaries (written by aggregator): plant-summary-YYYY-MM-DD
# @timestamp: written by aggregator alongside timestamp for ES/Kibana consistency
# timestamp:  written by aggregator (used by frontend TypeScript and backend queries)
curl -X PUT "http://elasticsearch:9200/_index_template/plant-summary-template" \
  -H "Content-Type: application/json" \
  -d '{
  "index_patterns": ["plant-summary-*"],
  "template": {
    "settings": {
      "number_of_shards": 1,
      "number_of_replicas": 0,
      "index.lifecycle.name": "plant-summary-policy"
    },
    "mappings": {
      "properties": {
        "@timestamp":   { "type": "date" },
        "timestamp":    { "type": "date" },
        "plantId":      { "type": "keyword" },
        "plantName":    { "type": "keyword" },
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
```

After deletion, the file should still have the panel ILM policy and panel index template intact, plus the final `echo "ILM policies and index templates created."` line.

- [ ] **Step 4: Verify docker-compose still parses**

Run: `docker compose config > /dev/null`
Expected: No errors

- [ ] **Step 5: Commit**

```bash
git add -A services/aggregator docker-compose.yml infra/elasticsearch/init-index.sh
git commit -m "chore: remove aggregator service and plant-summary index template

The aggregator service is no longer needed — plant-manager now
queries plant-panel-* directly for all aggregations. Removes the
service from docker-compose, deletes the aggregator source tree,
and removes the plant-summary ILM policy and index template from
init-index.sh."
```

---

### Task 5: Documentation update

**Files:**
- Modify: `docs/architecture.md`
- Modify: `docs/future-improvements.md`

- [ ] **Step 1: Remove aggregator from service list in architecture.md**

In `docs/architecture.md`, find this row in the 服務清單 table:

```markdown
| `aggregator` | Go | — | 每 10 秒從 ES 聚合摘要並寫回 ES |
```

Delete this entire line.

- [ ] **Step 2: Update ES description in service list**

In the same table, find:

```markdown
| `elasticsearch` | — | 9200 | 資料倉儲 |
```

Leave this row as-is — the ES service itself doesn't change.

- [ ] **Step 3: Remove aggregator from main architecture diagram**

In `docs/architecture.md`, find the `Backend["後端服務"]` subgraph block in the main mermaid graph:

```
    subgraph Backend["後端服務"]
        WS["ws-gateway\n:8080"]
        AS["alert-service\n:8081"]
        PM["plant-manager\n:8082"]
        AGG["aggregator\n每 10 秒"]
    end
```

Replace with (remove the AGG line):

```
    subgraph Backend["後端服務"]
        WS["ws-gateway\n:8080"]
        AS["alert-service\n:8081"]
        PM["plant-manager\n:8082"]
    end
```

- [ ] **Step 4: Remove aggregator arrows from main architecture diagram**

In the same mermaid graph, find these two lines:

```
    AGG -->|"查詢 plant-panel-*"| ES
    AGG -->|"寫入 plant-summary-YYYY-MM-DD"| ES
```

Delete both lines entirely.

- [ ] **Step 5: Update plant-manager arrow in main diagram**

Find:

```
    PM -->|"查詢 plant-panel-* / plant-summary-*"| ES
```

Replace with:

```
    PM -->|"查詢 plant-panel-*"| ES
```

- [ ] **Step 6: Update ES index description in main diagram**

Find:

```
        ES["Elasticsearch\n:9200\nplant-panel-YYYY-MM-DD\nplant-summary-YYYY-MM-DD"]
```

Replace with:

```
        ES["Elasticsearch\n:9200\nplant-panel-YYYY-MM-DD"]
```

- [ ] **Step 7: Update Dashboard endpoint in main diagram**

Find:

```
    DASH -->|"GET /api/plants/summary\n每 3s"| FE
```

Replace with:

```
    DASH -->|"GET /api/plants/current\n每 3s"| FE
```

- [ ] **Step 8: Delete the "聚合流程" section**

Find the entire "### 2. 聚合流程（每 10 秒）" section, including its mermaid block:

```markdown
### 2. 聚合流程（每 10 秒）

\`\`\`mermaid
sequenceDiagram
    participant AGG as aggregator
    participant ES as Elasticsearch

    AGG->>ES: 查詢 plant-panel-* (now-15s to now-5s)<br/>group by plantId → date_histogram(1s) → sum(watt)<br/>cardinality(panelId), status counts
    ES-->>AGG: 各電廠每秒聚合結果（~10 buckets/plant）
    AGG->>ES: 寫入 plant-summary-YYYY-MM-DD<br/>（每秒一筆，timestamp 取自 ES bucket key，~10 筆/電廠/cycle）
\`\`\`
```

Delete this entire section. Renumber the following section "### 3. 前端資料取得流程" to "### 2. 前端資料取得流程", and "### 4. 告警偵測流程" to "### 3. 告警偵測流程".

- [ ] **Step 9: Update the "前端資料取得流程" sequence diagram**

In what is now "### 2. 前端資料取得流程", find:

```
    Browser->>Nginx: GET /api/plants/summary (每 3s)
    Nginx->>PM: 轉發
    PM->>ES: 查詢 plant-summary-* (top_hits per plant)
    ES-->>PM: 最新摘要
    PM-->>Browser: JSON
```

Replace with:

```
    Browser->>Nginx: GET /api/plants/current (每 3s)
    Nginx->>PM: 轉發
    PM->>ES: 查詢 plant-panel-* (terms+top_hits per panel)
    ES-->>PM: 各 panel 最新讀值
    PM->>PM: Reduce 為 PlantSummary[]
    PM-->>Browser: { plants: [...] }
```

- [ ] **Step 10: Delete plant-summary index documentation**

Find the entire "### `plant-summary-YYYY-MM-DD`（aggregator 寫入）" section including its table and the "### 時間欄位說明" paragraph that follows. Delete from `### \`plant-summary-YYYY-MM-DD\`` to immediately before `## 資料生命週期（ILM）`.

The section to delete:

```markdown
### `plant-summary-YYYY-MM-DD`（aggregator 寫入）

每個 cycle（10 秒）每電廠寫入約 10 筆（每秒一筆，timestamp 取自 ES bucket key）。

| 欄位 | 類型 | 說明 |
|------|------|------|
| `@timestamp` | date | 聚合時間，供 ES/Kibana 查詢使用 |
| `timestamp` | date | 聚合時間，供前端 TypeScript 使用（與 @timestamp 同值） |
| `plantId` | keyword | 電廠 UUID |
| `plantName` | keyword | 電廠名稱 |
| `totalWatt` | float | 瞬間總發電量（avg_watt × panelCount） |
| `panelCount` | integer | 面板總數（cardinality） |
| `onlineCount` | integer | 線上面板數 |
| `offlineCount` | integer | 離線面板數 |
| `faultyCount` | integer | 故障面板數 |

### 時間欄位說明

兩個 index 都同時保有 `@timestamp` 和 `timestamp`：
- **`@timestamp`**：ES 生態系標準欄位，Kibana、ILM、跨 index 查詢預設使用
- **`timestamp`**：前端 TypeScript 友善名稱（`summary.timestamp` vs `summary["@timestamp"]`）
```

After deletion, "### `plant-panel-YYYY-MM-DD`" stays, followed directly by "## 資料生命週期（ILM）".

- [ ] **Step 11: Update ILM table**

Find:

```markdown
| Index Pattern | 保留天數 | 估計資料量 |
|---------------|---------|-----------|
| `plant-panel-*` | 7 天 | ~130 萬筆/天（3 廠 × 5 面板 × 86400 秒） |
| `plant-summary-*` | 30 天 | ~26000 筆/天（3 廠 × 8640 筆/10 秒） |
```

Replace with:

```markdown
| Index Pattern | 保留天數 | 估計資料量 |
|---------------|---------|-----------|
| `plant-panel-*` | 7 天 | ~130 萬筆/天（3 廠 × 5 面板 × 86400 秒） |
```

- [ ] **Step 12: Update Plant Manager API table**

Find:

```markdown
| `GET` | `/api/plants/summary` | 儀表板輪詢：查 `plant-summary-*` top_hits |
```

Replace with:

```markdown
| `GET` | `/api/plants/current` | 儀表板輪詢：查 `plant-panel-*` terms+top_hits 後在 Go 端 reduce |
| `GET` | `/api/power/history` | 全電廠功率歷史：查 `plant-panel-*` date_histogram + sum(watt) |
```

Then find:

```markdown
| `GET` | `/api/plants/{plantId}/history` | 歷史功率曲線：date_histogram |
```

Replace with:

```markdown
| `GET` | `/api/plants/{plantId}/history` | 單電廠歷史功率：查 `plant-panel-*` date_histogram + sum(watt) |
```

- [ ] **Step 13: Remove aggregator from Docker Compose 服務拓撲 diagram**

Find:

```
    agg["aggregator"] --> es
```

Delete this line entirely.

- [ ] **Step 14: Remove aggregator from 模組結構**

Find:

```
│   ├── plant-manager/           # 電廠 registry（NATS 自動發現）+ ES 查詢閘道
│   └── aggregator/              # ES 讀取聚合 → 摘要寫回
```

Replace with:

```
│   └── plant-manager/           # 電廠 registry（NATS 自動發現）+ ES 查詢閘道
```

- [ ] **Step 15: Update PlantSummary description in shared/**

Find:

```
├── shared/                      # 共用 models (PlantInfo, PanelReading, PlantSummary, Command...)
```

Leave unchanged — `PlantSummary` is still in shared/models, just now produced by plant-manager's reduce instead of by aggregator.

- [ ] **Step 16: Update last-updated date and add note**

Find:

```markdown
> 最後更新：2026-03-31
```

Replace with:

```markdown
> 最後更新：2026-04-09
```

- [ ] **Step 17: Update future-improvements.md — mark Option A as implemented**

In `docs/future-improvements.md`, find the "方案 A：移除 aggregator" section and add a marker at the top of that section indicating it has been implemented:

Find:
```markdown
### 方案 A：移除 aggregator，API 直接查 raw panel data
```

Replace with:
```markdown
### 方案 A：移除 aggregator，API 直接查 raw panel data ✅ 已實作（2026-04-09）
```

Also at the top of the document (after the title), add a note:

Find:
```markdown
# 後續修改計畫參考

這份文件記錄已知的架構改善方向，作為未來修改的參考依據。
```

Replace with:
```markdown
# 後續修改計畫參考

這份文件記錄已知的架構改善方向，作為未來修改的參考依據。

> **更新（2026-04-09）：** 方案 A 已實作。aggregator service 已移除，plant-manager 現在直接查詢 `plant-panel-*`。詳見 `docs/superpowers/specs/2026-04-09-remove-aggregator-direct-query-design.md`。
```

- [ ] **Step 18: Commit**

```bash
git add docs/architecture.md docs/future-improvements.md
git commit -m "docs: update architecture and future-improvements after aggregator removal

Reflects the new direct-query architecture in architecture.md.
Marks Option A as implemented in future-improvements.md."
```

---

### Task 6: Manual deployment + verification

- [ ] **Step 1: Rebuild and deploy plant-manager and frontend**

Run:

```bash
cd /home/zc/projects/solarops
docker compose up -d --build plant-manager frontend
```

- [ ] **Step 2: Stop and remove the aggregator container**

Run:

```bash
docker compose stop aggregator
docker compose rm -f aggregator
```

- [ ] **Step 3: Delete the abandoned plant-summary index from ES**

Run:

```bash
curl -X DELETE 'http://localhost:9200/plant-summary-*'
```

Expected: `{"acknowledged":true}` or similar success response.

- [ ] **Step 4: Verify Dashboard plant cards work**

1. Open http://localhost:3000 in browser
2. Wait ~10 seconds for first poll
3. Confirm each plant card shows correct values:
   - Plant name
   - totalWatt (should match Sunrise 15.0 / Golden 10.0 / Blue 16.8 kW when healthy)
   - Panels count, Normal count, Faulty count
   - Status indicator color (green/yellow/red)

- [ ] **Step 5: Verify Dashboard total power chart**

1. Wait ~30 seconds for chart to populate
2. Confirm chart shows ~41.8kW stable line
3. Verify the chart's right edge lags ~5 seconds behind "now" (this is the watermark)

- [ ] **Step 6: Verify PlantDetail chart**

1. Click any plant card to navigate to PlantDetail
2. Confirm the per-plant power chart populates
3. Verify per-second granularity

- [ ] **Step 7: Verify the spike-on-fault-reset bug is gone**

1. On PlantDetail, set a panel to DEAD fault mode
2. Wait ~10 seconds, observe the chart drops to reflect the fault
3. Click reset on the same panel
4. **Expected:** chart returns smoothly to the previous level — NO spike to ~2x value
5. Repeat 2-3 times to confirm no spikes

- [ ] **Step 8: Verify no aggregator-related errors in logs**

Run:

```bash
docker compose logs plant-manager --tail 50
```

Expected: No errors. Should not contain references to `plant-summary-*`.
