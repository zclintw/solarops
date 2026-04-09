# Per-Second Aggregation and ES-Driven Power History

## Problem

The aggregator produces one `plant-summary` document per plant every 10 seconds, using `avg` + `sum_bucket` across all panels. This causes two issues:

1. **Inaccurate totals:** Averaging over 10 seconds masks real per-second fluctuations. If a panel faults at T=3s, the 10-second average softens the drop instead of showing a sharp step.
2. **False zeros from timing gaps:** The aggregator queries `now-10s` but Fluentd has ~2-3s write delay. Panel data near the window edge may not have arrived in ES yet, causing partial or empty aggregation results that appear as 0 watt dips.

Additionally, the Dashboard's total power chart was accumulated client-side from 3-second polling, which cannot achieve 1-second granularity and loses history on page refresh.

## Requirements

- Aggregator writes one `plant-summary` per plant **per second** (10 docs per cycle instead of 1)
- Each summary's `totalWatt` is the **sum** of all panel watts for that exact second (not an average)
- Each summary's `@timestamp` reflects the actual data second, not the aggregator's execution time
- Query window shifted back 5 seconds (`now-15s` to `now-5s`) to accommodate Fluentd write delay
- New API `/api/power/history` for cross-plant total power (Dashboard use)
- Both Dashboard and PlantDetail charts driven by ES history, not client-side accumulation
- Chart X-axis: 5-minute window (300 points at 1s interval)
- Chart Y-axis: fixed lower bound at 0, upper bound tracks historical max (only grows, never shrinks)
- Time buckets with no data produce no summary doc (`min_doc_count: 1`), shown as line breaks in chart

## Design

### Aggregator Changes (`services/aggregator/main.go`)

#### Query Change

Current flat aggregation replaced with nested `date_histogram`:

```
by_plant (terms: plantId, size: 100)
  └── per_second (date_histogram: @timestamp, fixed_interval: 1s, min_doc_count: 1)
        ├── total_watt (sum: watt)
        ├── plant_name (terms: plantName, size: 1)
        ├── panel_count (cardinality: panelId)
        ├── online_panels (filter: status=online → cardinality: panelId)
        ├── offline_panels (filter: status=offline → cardinality: panelId)
        └── faulty_count (filter: exists faultMode → cardinality: panelId)
```

Time range filter: `@timestamp gte now-15s, lt now-5s`

#### Summary Document Timestamp

Each `per_second` bucket's `key_as_string` is used as the summary's `@timestamp`, not `time.Now()`. This ensures the summary is placed at the correct point on the timeline when queried by the history API.

#### Write Logic

`parseBuckets` becomes a two-level loop: outer iterates plant buckets, inner iterates `per_second` buckets within each plant. Each inner bucket produces one `plant-summary` document.

#### Unchanged

- Aggregator interval: still 10 seconds
- `plant-summary` index field schema: unchanged (`plantId`, `plantName`, `@timestamp`, `timestamp`, `totalWatt`, `panelCount`, `onlineCount`, `offlineCount`, `faultyCount`)
- ES connection config: unchanged

### New API: `/api/power/history` (`services/plant-manager/main.go`)

Returns per-second total power across all plants for a given time range.

```
GET /api/power/history?range=5m&interval=1s
```

Query parameters:
- `range`: time range (default `5m`)
- `interval`: bucket interval (default `1s`)

ES query on `plant-summary-*`:

```
filter: @timestamp gte now-{range}
aggs:
  over_time (date_histogram: @timestamp, fixed_interval: {interval})
    └── total_watt (sum: totalWatt)
```

Response: raw ES response body (same pattern as existing history API).

### Existing API Change: `/api/plants/{plantId}/history`

Change the `total_watt` aggregation from `avg` to `sum`. With per-second summaries, each second has at most one document per plant, so `sum` and `avg` yield the same result — but `sum` is semantically correct.

### Frontend Changes

#### App.tsx

Remove all powerHistory accumulation logic added by the previous feature:
- Remove `plantEntries`, `totalWatt`, `lastSeen` calculations
- Remove `powerHistory` state and `useEffect`
- Remove `powerHistory` prop from `<Dashboard>`

#### Dashboard.tsx

- Remove `powerHistory` from `DashboardProps`
- Add local state for `history: { time: string; watt: number | null }[]`
- On mount: fetch `/api/power/history?range=5m&interval=1s`
- Every 10 seconds: re-fetch (full replace, not append)
- Parse response same as PlantDetail: `buckets.map(b => ({ time, watt: b.total_watt?.value != null ? Math.round(b.total_watt.value) : null }))`
- Pass `history` to `<PowerChart>`

#### PlantDetail.tsx

- Change history fetch from `range=1h&interval=10s` to `range=5m&interval=1s`
- Remove the live-append `useEffect` (lines 43-52) — replaced by periodic re-fetch
- Add 10-second re-fetch interval (same pattern as Dashboard)

#### PowerChart.tsx

- `WINDOW_SIZE`: 60 → 300 (5 minutes × 1 point/sec)
- Y-axis: track historical max with `useState`, only update upward
  ```
  domain={[0, yMax || 'auto']}
  ```
- Null watt handling: already in place (line breaks for data gaps)

### Data Flow (After)

```
Mock Plant (1s) → log → Fluentd (1s flush) → ES plant-panel-*

Aggregator (every 10s):
  query: ES plant-panel-* where @timestamp in [now-15s, now-5s)
  produce: ~10 plant-summary docs per plant (1 per second, real timestamp)
  write: ES plant-summary-*

Dashboard (mount + every 10s):
  GET /api/power/history?range=5m&interval=1s
  → PowerChart (300 points, all plants summed)

PlantDetail (mount + every 10s):
  GET /api/plants/{id}/history?range=5m&interval=1s
  → PowerChart (300 points, single plant)
```

## Scope

- 3 files modified: `services/aggregator/main.go`, `services/plant-manager/main.go`, `frontend/src/pages/PlantDetail.tsx`
- 2 files modified (revert previous feature): `frontend/src/App.tsx`, `frontend/src/pages/Dashboard.tsx`
- 1 file modified: `frontend/src/components/PowerChart.tsx`
- 0 new files, 0 new dependencies
