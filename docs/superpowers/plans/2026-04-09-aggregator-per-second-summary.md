# Per-Second Aggregation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace 10-second averaged aggregation with per-second exact summaries, and drive both power charts from ES history instead of client-side accumulation.

**Architecture:** Aggregator queries `now-15s` to `now-5s` with a 1-second `date_histogram`, writing ~10 summary docs per plant per cycle. A new `/api/power/history` endpoint serves cross-plant totals. Both Dashboard and PlantDetail fetch from ES every 10 seconds, displaying 300 points (5 minutes).

**Tech Stack:** Go (aggregator, plant-manager), Elasticsearch, React 19, Recharts, TypeScript

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `services/aggregator/main.go` | Modify | Update `buildQuery` (date_histogram + sum), update `parseBuckets` (two-level loop, bucket timestamps), update structs |
| `services/aggregator/main_test.go` | Modify | Update tests for new query structure and new parseBuckets response shape |
| `services/plant-manager/main.go` | Modify | Add `/api/power/history` endpoint, change existing history API `avg` → `sum` |
| `frontend/src/App.tsx` | Modify | Remove powerHistory accumulation logic, remove prop to Dashboard |
| `frontend/src/pages/Dashboard.tsx` | Modify | Remove powerHistory prop, add ES-driven history fetch with 10s polling |
| `frontend/src/pages/PlantDetail.tsx` | Modify | Change to `range=5m&interval=1s`, remove live-append effect, add 10s re-fetch |
| `frontend/src/components/PowerChart.tsx` | Modify | WINDOW_SIZE 60→300, Y-axis tracks historical max |

---

### Task 1: Aggregator — update buildQuery with TDD

**Files:**
- Modify: `services/aggregator/main.go`
- Modify: `services/aggregator/main_test.go`

- [ ] **Step 1: Update the test for buildQuery to match new structure**

Replace `TestBuildQuery_Structure` in `services/aggregator/main_test.go` (lines 199-240) with:

```go
func TestBuildQuery_Structure(t *testing.T) {
	query := buildQuery()

	if query["size"] != 0 {
		t.Error("expected size 0")
	}

	// Verify time range filter uses shifted window
	q, ok := query["query"].(map[string]interface{})
	if !ok {
		t.Fatal("expected query")
	}
	rangeFilter, ok := q["range"].(map[string]interface{})
	if !ok {
		t.Fatal("expected range filter")
	}
	ts, ok := rangeFilter["@timestamp"].(map[string]interface{})
	if !ok {
		t.Fatal("expected @timestamp range")
	}
	if ts["gte"] != "now-15s" {
		t.Errorf("expected gte now-15s, got %v", ts["gte"])
	}
	if ts["lt"] != "now-5s" {
		t.Errorf("expected lt now-5s, got %v", ts["lt"])
	}

	aggs, ok := query["aggs"].(map[string]interface{})
	if !ok {
		t.Fatal("expected aggs")
	}

	byPlant, ok := aggs["by_plant"].(map[string]interface{})
	if !ok {
		t.Fatal("expected by_plant aggregation")
	}

	subAggs, ok := byPlant["aggs"].(map[string]interface{})
	if !ok {
		t.Fatal("expected sub-aggregations under by_plant")
	}

	// Verify per_second date_histogram exists
	perSecond, ok := subAggs["per_second"].(map[string]interface{})
	if !ok {
		t.Fatal("expected per_second sub-aggregation")
	}
	dh, ok := perSecond["date_histogram"].(map[string]interface{})
	if !ok {
		t.Fatal("expected date_histogram in per_second")
	}
	if dh["fixed_interval"] != "1s" {
		t.Errorf("expected fixed_interval 1s, got %v", dh["fixed_interval"])
	}
	if dh["min_doc_count"] != 1 {
		t.Errorf("expected min_doc_count 1, got %v", dh["min_doc_count"])
	}

	// Verify total_watt uses sum (not avg/sum_bucket)
	perSecondAggs, ok := perSecond["aggs"].(map[string]interface{})
	if !ok {
		t.Fatal("expected aggs inside per_second")
	}
	totalWatt, ok := perSecondAggs["total_watt"].(map[string]interface{})
	if !ok {
		t.Fatal("expected total_watt in per_second aggs")
	}
	if _, ok := totalWatt["sum"]; !ok {
		t.Error("expected total_watt to use sum aggregation")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/aggregator && go test -run TestBuildQuery_Structure -v`
Expected: FAIL (current buildQuery uses `now-10s`, `sum_bucket`, no `per_second`)

- [ ] **Step 3: Update buildQuery in main.go**

Replace the `buildQuery` function (lines 87-161) with:

```go
func buildQuery() map[string]interface{} {
	return map[string]interface{}{
		"size": 0,
		"query": map[string]interface{}{
			"range": map[string]interface{}{
				"@timestamp": map[string]interface{}{
					"gte": "now-15s",
					"lt":  "now-5s",
				},
			},
		},
		"aggs": map[string]interface{}{
			"by_plant": map[string]interface{}{
				"terms": map[string]interface{}{
					"field": "plantId",
					"size":  100,
				},
				"aggs": map[string]interface{}{
					"per_second": map[string]interface{}{
						"date_histogram": map[string]interface{}{
							"field":          "@timestamp",
							"fixed_interval": "1s",
							"min_doc_count":  1,
						},
						"aggs": map[string]interface{}{
							"total_watt": map[string]interface{}{
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
							"online_panels": map[string]interface{}{
								"filter": map[string]interface{}{
									"term": map[string]interface{}{"status": "online"},
								},
								"aggs": map[string]interface{}{
									"count": map[string]interface{}{
										"cardinality": map[string]interface{}{"field": "panelId"},
									},
								},
							},
							"offline_panels": map[string]interface{}{
								"filter": map[string]interface{}{
									"term": map[string]interface{}{"status": "offline"},
								},
								"aggs": map[string]interface{}{
									"count": map[string]interface{}{
										"cardinality": map[string]interface{}{"field": "panelId"},
									},
								},
							},
							"faulty_count": map[string]interface{}{
								"filter": map[string]interface{}{
									"exists": map[string]interface{}{"field": "faultMode"},
								},
								"aggs": map[string]interface{}{
									"count": map[string]interface{}{
										"cardinality": map[string]interface{}{"field": "panelId"},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/aggregator && go test -run TestBuildQuery_Structure -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add services/aggregator/main.go services/aggregator/main_test.go
git commit -m "refactor(aggregator): change buildQuery to per-second date_histogram

Query window shifted to now-15s..now-5s for Fluentd delay tolerance.
Uses sum(watt) per 1s bucket instead of avg+sum_bucket over 10s."
```

---

### Task 2: Aggregator — update structs and parseBuckets with TDD

**Files:**
- Modify: `services/aggregator/main.go`
- Modify: `services/aggregator/main_test.go`

- [ ] **Step 1: Update all parseBuckets tests for new response shape**

The ES response now has a nested structure: plant bucket → `per_second.buckets[]` → sub-aggs. Each `per_second` bucket has `key_as_string` for the timestamp.

Replace all `TestParseBuckets_*` tests in `services/aggregator/main_test.go` (lines 9-197) with:

```go
func TestParseBuckets_NormalPlant(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage(`{
			"key": "plant-001",
			"per_second": {
				"buckets": [
					{
						"key_as_string": "2026-04-01T12:00:05.000Z",
						"total_watt": {"value": 15000},
						"plant_name": {"buckets": [{"key": "Sunrise Valley"}]},
						"panel_count": {"value": 5},
						"online_panels": {"doc_count": 10, "count": {"value": 5}},
						"offline_panels": {"doc_count": 0, "count": {"value": 0}},
						"faulty_count": {"doc_count": 0, "count": {"value": 0}}
					}
				]
			}
		}`),
	}

	summaries := parseBuckets(raw)

	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}

	s := summaries[0]
	if s.PlantID != "plant-001" {
		t.Errorf("expected plant-001, got %s", s.PlantID)
	}
	if s.PlantName != "Sunrise Valley" {
		t.Errorf("expected Sunrise Valley, got %s", s.PlantName)
	}
	if s.TotalWatt != 15000 {
		t.Errorf("expected totalWatt 15000, got %f", s.TotalWatt)
	}
	if s.PanelCount != 5 {
		t.Errorf("expected panelCount 5, got %d", s.PanelCount)
	}
	if s.OnlineCount != 5 {
		t.Errorf("expected onlineCount 5, got %d", s.OnlineCount)
	}
	if s.Timestamp != "2026-04-01T12:00:05.000Z" {
		t.Errorf("expected timestamp from bucket key, got %s", s.Timestamp)
	}
}

func TestParseBuckets_MultipleSeconds(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage(`{
			"key": "plant-001",
			"per_second": {
				"buckets": [
					{
						"key_as_string": "2026-04-01T12:00:05.000Z",
						"total_watt": {"value": 15000},
						"plant_name": {"buckets": [{"key": "Sunrise Valley"}]},
						"panel_count": {"value": 5},
						"online_panels": {"doc_count": 10, "count": {"value": 5}},
						"offline_panels": {"doc_count": 0, "count": {"value": 0}},
						"faulty_count": {"doc_count": 0, "count": {"value": 0}}
					},
					{
						"key_as_string": "2026-04-01T12:00:06.000Z",
						"total_watt": {"value": 14500},
						"plant_name": {"buckets": [{"key": "Sunrise Valley"}]},
						"panel_count": {"value": 5},
						"online_panels": {"doc_count": 10, "count": {"value": 5}},
						"offline_panels": {"doc_count": 0, "count": {"value": 0}},
						"faulty_count": {"doc_count": 0, "count": {"value": 0}}
					}
				]
			}
		}`),
	}

	summaries := parseBuckets(raw)

	if len(summaries) != 2 {
		t.Fatalf("expected 2 summaries (one per second), got %d", len(summaries))
	}
	if summaries[0].TotalWatt != 15000 {
		t.Errorf("expected first second 15000, got %f", summaries[0].TotalWatt)
	}
	if summaries[1].TotalWatt != 14500 {
		t.Errorf("expected second second 14500, got %f", summaries[1].TotalWatt)
	}
	if summaries[0].Timestamp != "2026-04-01T12:00:05.000Z" {
		t.Errorf("expected bucket timestamp, got %s", summaries[0].Timestamp)
	}
	if summaries[1].Timestamp != "2026-04-01T12:00:06.000Z" {
		t.Errorf("expected bucket timestamp, got %s", summaries[1].Timestamp)
	}
}

func TestParseBuckets_MultiplePlants(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage(`{
			"key": "plant-001",
			"per_second": {
				"buckets": [{
					"key_as_string": "2026-04-01T12:00:05.000Z",
					"total_watt": {"value": 15000},
					"plant_name": {"buckets": [{"key": "Sunrise Valley"}]},
					"panel_count": {"value": 5},
					"online_panels": {"doc_count": 10, "count": {"value": 5}},
					"offline_panels": {"doc_count": 0, "count": {"value": 0}},
					"faulty_count": {"doc_count": 0, "count": {"value": 0}}
				}]
			}
		}`),
		json.RawMessage(`{
			"key": "plant-002",
			"per_second": {
				"buckets": [{
					"key_as_string": "2026-04-01T12:00:05.000Z",
					"total_watt": {"value": 16800},
					"plant_name": {"buckets": [{"key": "Blue Horizon"}]},
					"panel_count": {"value": 6},
					"online_panels": {"doc_count": 12, "count": {"value": 6}},
					"offline_panels": {"doc_count": 0, "count": {"value": 0}},
					"faulty_count": {"doc_count": 3, "count": {"value": 1}}
				}]
			}
		}`),
	}

	summaries := parseBuckets(raw)

	if len(summaries) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(summaries))
	}

	total := summaries[0].TotalWatt + summaries[1].TotalWatt
	if total != 31800 {
		t.Errorf("expected combined totalWatt 31800, got %f", total)
	}
	if summaries[1].FaultyCount != 1 {
		t.Errorf("expected faultyCount 1 for plant-002, got %d", summaries[1].FaultyCount)
	}
}

func TestParseBuckets_WithOfflinePanels(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage(`{
			"key": "plant-003",
			"per_second": {
				"buckets": [{
					"key_as_string": "2026-04-01T12:00:05.000Z",
					"total_watt": {"value": 6000},
					"plant_name": {"buckets": [{"key": "Golden Ridge"}]},
					"panel_count": {"value": 4},
					"online_panels": {"doc_count": 6, "count": {"value": 2}},
					"offline_panels": {"doc_count": 4, "count": {"value": 2}},
					"faulty_count": {"doc_count": 0, "count": {"value": 0}}
				}]
			}
		}`),
	}

	summaries := parseBuckets(raw)
	s := summaries[0]

	if s.OnlineCount != 2 {
		t.Errorf("expected onlineCount 2, got %d", s.OnlineCount)
	}
	if s.OfflineCount != 2 {
		t.Errorf("expected offlineCount 2, got %d", s.OfflineCount)
	}
	if s.TotalWatt != 6000 {
		t.Errorf("expected totalWatt 6000, got %f", s.TotalWatt)
	}
}

func TestParseBuckets_MissingPlantName(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage(`{
			"key": "plant-orphan",
			"per_second": {
				"buckets": [{
					"key_as_string": "2026-04-01T12:00:05.000Z",
					"total_watt": {"value": 1000},
					"plant_name": {"buckets": []},
					"panel_count": {"value": 1},
					"online_panels": {"doc_count": 1, "count": {"value": 1}},
					"offline_panels": {"doc_count": 0, "count": {"value": 0}},
					"faulty_count": {"doc_count": 0, "count": {"value": 0}}
				}]
			}
		}`),
	}

	summaries := parseBuckets(raw)
	if summaries[0].PlantName != "" {
		t.Errorf("expected empty plantName, got %s", summaries[0].PlantName)
	}
}

func TestParseBuckets_EmptyInput(t *testing.T) {
	summaries := parseBuckets(nil)
	if len(summaries) != 0 {
		t.Errorf("expected 0 summaries for nil input, got %d", len(summaries))
	}

	summaries = parseBuckets([]json.RawMessage{})
	if len(summaries) != 0 {
		t.Errorf("expected 0 summaries for empty input, got %d", len(summaries))
	}
}

func TestParseBuckets_InvalidJSON(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage(`{invalid json}`),
		json.RawMessage(`{
			"key": "plant-ok",
			"per_second": {
				"buckets": [{
					"key_as_string": "2026-04-01T12:00:05.000Z",
					"total_watt": {"value": 5000},
					"plant_name": {"buckets": [{"key": "Valid Plant"}]},
					"panel_count": {"value": 2},
					"online_panels": {"doc_count": 2, "count": {"value": 2}},
					"offline_panels": {"doc_count": 0, "count": {"value": 0}},
					"faulty_count": {"doc_count": 0, "count": {"value": 0}}
				}]
			}
		}`),
	}

	summaries := parseBuckets(raw)

	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary (skip invalid), got %d", len(summaries))
	}
	if summaries[0].PlantID != "plant-ok" {
		t.Errorf("expected plant-ok, got %s", summaries[0].PlantID)
	}
}

func TestParseBuckets_EmptyPerSecondBuckets(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage(`{
			"key": "plant-empty",
			"per_second": {
				"buckets": []
			}
		}`),
	}

	summaries := parseBuckets(raw)
	if len(summaries) != 0 {
		t.Errorf("expected 0 summaries for plant with no per_second buckets, got %d", len(summaries))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd services/aggregator && go test -run TestParseBuckets -v`
Expected: FAIL (parseBuckets still expects old flat structure and takes `now` param)

- [ ] **Step 3: Update structs and parseBuckets in main.go**

Replace the `plantBucket` struct (lines 25-44) with:

```go
type secondBucket struct {
	KeyAsString  string `json:"key_as_string"`
	TotalWatt    struct{ Value float64 } `json:"total_watt"`
	PlantName    struct {
		Buckets []struct{ Key string } `json:"buckets"`
	} `json:"plant_name"`
	PanelCount   struct{ Value int } `json:"panel_count"`
	OnlinePanels struct {
		DocCount int `json:"doc_count"`
		Count    struct{ Value int } `json:"count"`
	} `json:"online_panels"`
	OfflinePanels struct {
		DocCount int `json:"doc_count"`
		Count    struct{ Value int } `json:"count"`
	} `json:"offline_panels"`
	FaultyCount struct {
		DocCount int `json:"doc_count"`
		Count    struct{ Value int } `json:"count"`
	} `json:"faulty_count"`
}

type plantBucket struct {
	Key       string `json:"key"`
	PerSecond struct {
		Buckets []secondBucket `json:"buckets"`
	} `json:"per_second"`
}
```

Replace the `parseBuckets` function (lines 58-85) with:

```go
func parseBuckets(raw []json.RawMessage) []plantSummary {
	var summaries []plantSummary
	for _, r := range raw {
		var bucket plantBucket
		if err := json.Unmarshal(r, &bucket); err != nil {
			continue
		}

		for _, sb := range bucket.PerSecond.Buckets {
			plantName := ""
			if len(sb.PlantName.Buckets) > 0 {
				plantName = sb.PlantName.Buckets[0].Key
			}

			summaries = append(summaries, plantSummary{
				PlantID:      bucket.Key,
				PlantName:    plantName,
				Timestamp:    sb.KeyAsString,
				TimestampAlt: sb.KeyAsString,
				TotalWatt:    sb.TotalWatt.Value,
				PanelCount:   sb.PanelCount.Value,
				OnlineCount:  sb.OnlinePanels.Count.Value,
				OfflineCount: sb.OfflinePanels.Count.Value,
				FaultyCount:  sb.FaultyCount.Count.Value,
			})
		}
	}
	return summaries
}
```

Also update the `aggregate` function call site (line 232). Change:

```go
now := time.Now().UTC()
indexName := fmt.Sprintf("plant-summary-%s", now.Format("2006-01-02"))
summaries := parseBuckets(result.Aggregations.ByPlant.Buckets, now)
```

to:

```go
indexName := fmt.Sprintf("plant-summary-%s", time.Now().UTC().Format("2006-01-02"))
summaries := parseBuckets(result.Aggregations.ByPlant.Buckets)
```

Remove the `"time"` import if it is no longer used elsewhere. Check: `time.NewTicker` and `time.Second` are still used in `main()`, so keep the import.

- [ ] **Step 4: Run all aggregator tests**

Run: `cd services/aggregator && go test -v`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add services/aggregator/main.go services/aggregator/main_test.go
git commit -m "refactor(aggregator): per-second parseBuckets with bucket timestamps

parseBuckets now iterates plant → per_second buckets, producing one
summary per plant per second. Timestamps come from ES bucket keys
instead of time.Now()."
```

---

### Task 3: Plant-manager — new API endpoint + existing API change

**Files:**
- Modify: `services/plant-manager/main.go`

- [ ] **Step 1: Add `/api/power/history` endpoint**

After the existing `/api/plants/{plantId}/history` handler (after line 161), add:

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

- [ ] **Step 2: Change existing history API from avg to sum**

In the existing `/api/plants/{plantId}/history` handler, change line 138:

From:
```go
							"avg": map[string]interface{}{"field": "totalWatt"},
```

To:
```go
							"sum": map[string]interface{}{"field": "totalWatt"},
```

Also change the default interval on line 115:

From:
```go
			interval = "10s"
```

To:
```go
			interval = "1s"
```

- [ ] **Step 3: Verify plant-manager compiles**

Run: `cd services/plant-manager && go build -o /dev/null .`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add services/plant-manager/main.go
git commit -m "feat(plant-manager): add /api/power/history, change history avg to sum

New endpoint returns per-second total power across all plants for
Dashboard chart. Existing per-plant history changed from avg to sum
and default interval from 10s to 1s."
```

---

### Task 4: Frontend — PowerChart WINDOW_SIZE and Y-axis

**Files:**
- Modify: `frontend/src/components/PowerChart.tsx`

- [ ] **Step 1: Update WINDOW_SIZE and add Y-axis max tracking**

Replace the entire `frontend/src/components/PowerChart.tsx` with:

```tsx
import { useState } from "react";
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from "recharts";

interface DataPoint {
  time: string;
  watt: number | null;
}

type ChartPoint = { time: string; watt?: number | null };

const WINDOW_SIZE = 300;

interface PowerChartProps {
  data: DataPoint[];
  height?: number;
}

export function PowerChart({ data, height = 200 }: PowerChartProps) {
  const [yMax, setYMax] = useState(0);

  const dataMax = Math.max(0, ...data.map((d) => d.watt ?? 0));
  if (dataMax > yMax) {
    setYMax(dataMax);
  }

  const chartData: ChartPoint[] =
    data.length >= WINDOW_SIZE
      ? data
      : [
          ...Array.from({ length: WINDOW_SIZE - data.length }, (): ChartPoint => ({
            time: "",
          })),
          ...data,
        ];

  return (
    <ResponsiveContainer width="100%" height={height}>
      <LineChart data={chartData}>
        <CartesianGrid strokeDasharray="3 3" stroke="#333" />
        <XAxis dataKey="time" stroke="#888" fontSize={12} />
        <YAxis stroke="#888" fontSize={12} domain={[0, yMax || "auto"]} />
        <Tooltip
          contentStyle={{ backgroundColor: "#1a1a1a", border: "1px solid #333" }}
        />
        <Line
          type="monotone"
          dataKey="watt"
          stroke="#22c55e"
          strokeWidth={2}
          dot={false}
        />
      </LineChart>
    </ResponsiveContainer>
  );
}
```

- [ ] **Step 2: Verify frontend compiles**

Run: `cd frontend && npx tsc -p tsconfig.app.json --noEmit`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/PowerChart.tsx
git commit -m "feat(frontend): PowerChart 300-point window with fixed Y-axis

WINDOW_SIZE 60→300 (5 minutes at 1s interval). Y-axis lower bound
fixed at 0, upper bound tracks historical max (only grows)."
```

---

### Task 5: Frontend — App.tsx cleanup

**Files:**
- Modify: `frontend/src/App.tsx`

- [ ] **Step 1: Remove powerHistory accumulation logic**

Replace the entire `frontend/src/App.tsx` with:

```tsx
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
```

- [ ] **Step 2: Verify frontend compiles**

Run: `cd frontend && npx tsc -p tsconfig.app.json --noEmit`
Expected: Type error about `powerHistory` in Dashboard (fixed in Task 6)

- [ ] **Step 3: Commit**

```bash
git add frontend/src/App.tsx
git commit -m "refactor(frontend): remove powerHistory accumulation from App.tsx

Dashboard chart will be driven by ES history API instead of
client-side accumulation. Reverts the lift-state approach."
```

---

### Task 6: Frontend — Dashboard ES-driven history

**Files:**
- Modify: `frontend/src/pages/Dashboard.tsx`

- [ ] **Step 1: Replace Dashboard with ES-driven history**

Replace the entire `frontend/src/pages/Dashboard.tsx` with:

```tsx
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
```

- [ ] **Step 2: Verify frontend compiles**

Run: `cd frontend && npx tsc -p tsconfig.app.json --noEmit`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add frontend/src/pages/Dashboard.tsx
git commit -m "feat(frontend): Dashboard chart driven by ES /api/power/history

Fetches total power history from ES on mount and every 10s.
Replaces client-side accumulation with server-side aggregation."
```

---

### Task 7: Frontend — PlantDetail ES-driven history

**Files:**
- Modify: `frontend/src/pages/PlantDetail.tsx`

- [ ] **Step 1: Change history fetch and add periodic re-fetch**

In `frontend/src/pages/PlantDetail.tsx`, replace the two history-related useEffects (lines 25-52) with a single effect:

```tsx
  // Fetch power history from ES, re-fetch every 10s
  useEffect(() => {
    if (!plantId) return;
    const fetchHistory = () => {
      fetch(`/api/plants/${plantId}/history?range=5m&interval=1s`)
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
    };
    fetchHistory();
    const interval = setInterval(fetchHistory, 10_000);
    return () => clearInterval(interval);
  }, [plantId]);
```

- [ ] **Step 2: Verify frontend compiles**

Run: `cd frontend && npx tsc -p tsconfig.app.json --noEmit`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add frontend/src/pages/PlantDetail.tsx
git commit -m "feat(frontend): PlantDetail chart uses 5m/1s history with 10s re-fetch

Replaces 1h/10s one-time fetch + live append with periodic 5m/1s
re-fetch from ES. Consistent with Dashboard approach."
```

---

### Task 8: Manual verification

- [ ] **Step 1: Rebuild and restart aggregator**

Run: `cd services/aggregator && go build -o aggregator . && ./aggregator`
Verify logs show: `Aggregated N plant summaries → plant-summary-YYYY-MM-DD` with N being ~10× the number of plants (instead of 1× as before)

- [ ] **Step 2: Rebuild and restart plant-manager**

Run: `cd services/plant-manager && go build -o plant-manager . && ./plant-manager`

- [ ] **Step 3: Start frontend dev server**

Run: `cd frontend && npm run dev`

- [ ] **Step 4: Verify Dashboard total power chart**

1. Open browser to Dashboard
2. Wait ~20 seconds for aggregator to produce data
3. Chart should show data points at 1-second intervals
4. X-axis should cover 5 minutes
5. Y-axis should start at 0 and not shrink when power drops

- [ ] **Step 5: Verify PlantDetail chart**

1. Click a plant card to navigate to PlantDetail
2. Power History chart should show 1-second interval data
3. Wait 10 seconds — chart should update with new data
4. Navigate back to Dashboard — Dashboard chart should still have data (fetched from ES)

- [ ] **Step 6: Verify no false zero dips**

1. With all panels online (no faults), chart should show stable power with no drops to 0
2. If a panel is set to fault, chart should show the real power drop at the correct timestamp

- [ ] **Step 7: Verify data gap display**

1. Stop a mock-plant container
2. After ~15 seconds, the chart for that plant should show a break in the line (not a drop to 0)
