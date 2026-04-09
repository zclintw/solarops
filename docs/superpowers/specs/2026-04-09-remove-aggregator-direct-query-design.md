# Remove Aggregator: Direct Panel Query Design

## Problem

The current architecture uses an `aggregator` service that pre-computes per-second `plant-summary` documents from raw `plant-panel` data, with rolling window + UPSERT semantics to handle late-arriving data. This has produced two recurring bugs:

1. **False low dips**: Late-arriving panel data missed the aggregator's query window, causing summaries to undercount.
2. **Spikes on fault reset**: Duplicate `plant-summary` docs (suspected cause: ES UPSERT race during refresh window) cause `/api/power/history` to double-count, producing 2x spikes (e.g., 73600W instead of 36800W).

The root cause is **pre-aggregation itself**: once a summary is written, it has its own consistency requirements (no duplicates, idempotent writes, watermark for late data). For our demo-scale data volume, pre-aggregation is unnecessary complexity.

## Requirements

- Remove the aggregator service entirely
- Frontend behavior unchanged: Dashboard total power chart, PlantDetail per-plant chart, Dashboard plant cards, all continue working
- Eliminate the spike-on-fault-reset bug
- Eliminate the false-low-dip bug
- Apply a 5-second watermark to history queries to avoid showing partial Fluentd-buffered data
- Reduce architectural complexity (fewer services, less code)

## Chosen Approach: Direct Query of `plant-panel-*`

Three plant-manager API endpoints are rewritten to query `plant-panel-*` directly with on-demand aggregation. The `plant-summary-*` index and the `aggregator` service are removed entirely.

### Why this fixes both bugs

- **No spike**: There's no `plant-summary-*` index, so no duplicate-summary problem. Each query computes from raw panel data, where each panel emits exactly one reading per second (Fluentd `pos_file` prevents log re-reading).
- **No false dips**: Each query is fresh. Late-arriving panel data is automatically included in the next query (10s later). Combined with the 5-second watermark, queries only show "stable" data.

### Failure mode tradeoff

| Scenario | Old (with aggregator) | New (direct query) |
|----------|----------------------|---------------------|
| Late Fluentd flush | Cached in summary | 5s watermark hides unstable region |
| Doc duplicate | **2x spike** (current bug) | Impossible (no summary index) |
| Network blip | Aggregator may miss cycle | Self-correcting on next refresh |

## Design

### Endpoint 1: `/api/plants/current` (renamed from `/api/plants/summary`)

**Purpose:** Dashboard plant cards — display each plant's current state (totalWatt, panelCount, statuses).

**ES query** on `plant-panel-*`:

```
filter: @timestamp gte now-30s   (excludes long-offline plants)
aggs:
  by_plant (terms: plantId, size: 100):
    by_panel (terms: panelId, size: 200):
      latest (top_hits: size=1, sort=@timestamp desc)
```

**Go-side reduction:**

For each plant bucket, iterate its panel buckets. For each panel, take `latest.hits.hits[0]._source` (the latest `PanelReading`). Reduce to a `PlantSummary`:

- `plantId`: from plant bucket key
- `plantName`: from first panel's reading
- `timestamp`: latest panel reading's timestamp
- `totalWatt`: sum of all panels' `watt`
- `panelCount`: number of panel buckets
- `onlineCount`: count where `status == "online"`
- `offlineCount`: count where `status == "offline"`
- `faultyCount`: count where `faultMode != ""`

**Response format:** `{ "plants": PlantSummary[] }` (array, not raw ES aggregation structure).

### Endpoint 2: `/api/power/history`

**Purpose:** Dashboard's total power chart — 5 minutes of per-second total power across all plants.

**ES query** on `plant-panel-*`:

```
filter: @timestamp in [now-5m-5s, now-5s)
aggs:
  over_time (date_histogram: @timestamp, fixed_interval=interval, min_doc_count=1):
    total_watt (sum: watt)
```

Query parameters: `range` (default `5m`), `interval` (default `1s`).

The `[now-5m-5s, now-5s)` range applies a 5-second watermark — the most recent 5 seconds are excluded to avoid showing partial Fluentd-buffered data. The chart appears to lag by 5 seconds, which is acceptable for the demo.

**Response:** Pass-through ES response body (same format as today; frontend already parses `aggregations.over_time.buckets`).

### Endpoint 3: `/api/plants/{plantId}/history`

**Purpose:** PlantDetail's per-plant chart.

**ES query** on `plant-panel-*`:

```
filter:
  - term: { plantId: <plantId> }
  - @timestamp in [now-5m-5s, now-5s)
aggs:
  over_time (date_histogram: @timestamp, fixed_interval=interval, min_doc_count=1):
    total_watt (sum: watt)
```

Same structure as `/api/power/history`, with an added `plantId` term filter. Same 5-second watermark.

**Response:** Pass-through ES response body.

### Frontend Changes

Only `frontend/src/hooks/usePlants.ts` changes:

1. Fetch URL: `/api/plants/summary` → `/api/plants/current`
2. Response parsing: change from ES aggregation structure (`data.aggregations.by_plant.buckets[].latest.hits.hits[0]._source`) to flat array (`data.plants`)

The `PlantSummary` type in `frontend/src/types.ts` is unchanged — only the source/wrapping changes, the inner shape stays identical.

All other frontend files unchanged: `Dashboard.tsx`, `PlantDetail.tsx`, `PowerChart.tsx`, `App.tsx`, `types.ts`.

### Removals

- Delete `services/aggregator/` (entire directory: main.go, main_test.go, Dockerfile, go.mod, go.sum)
- Remove `aggregator` service from `docker-compose.yml`
- Remove `plant-summary-*` index template from `infra/elasticsearch/init-index.sh`
- Update `docs/architecture.md` to remove aggregator from data flow diagram
- Update `docs/future-improvements.md` to mark Option B as implemented

### What stays unchanged

- `shared/models/models.go` — `PlantSummary` struct preserved as the API contract
- `frontend/src/types.ts` — `PlantSummary` type preserved
- All other frontend files
- Mock-plant, Fluentd, ws-gateway, alert-service — none touched
- `plant-panel-*` index — unchanged, still the source of truth

## Data Flow (After)

```
mock-plant → log file → Fluentd (1s flush) → ES plant-panel-*
                                                    ↓
                                            plant-manager (3 endpoints)
                                                    ↓
                                                  frontend
```

No aggregator. No plant-summary index. Three endpoints, each querying `plant-panel-*` directly.

## Deployment

Deployment requires three steps in order:

```bash
# 1. Rebuild and deploy plant-manager + frontend
docker compose up -d --build plant-manager frontend

# 2. Stop and remove the aggregator service
docker compose stop aggregator
docker compose rm -f aggregator

# 3. Clean up the abandoned plant-summary index
curl -X DELETE 'http://localhost:9200/plant-summary-*'
```

## Verification

After deployment:
- Dashboard plant cards display each plant's totalWatt, panelCount, and status correctly
- Total Power chart shows ~41800W stable (with all panels healthy)
- PlantDetail chart shows per-plant power
- Trigger panel fault → reset: **no spike** in either chart
- Trigger panel offline: chart reflects the drop within ~10 seconds (next refresh) plus the 5s watermark
- Both charts have a constant ~5 second visual lag from "now" (acceptable for demo)

## Scope

| Action | Files |
|--------|-------|
| **Delete** | `services/aggregator/` (entire dir), `docker-compose.yml` aggregator service, `infra/elasticsearch/init-index.sh` plant-summary template |
| **Modify** | `services/plant-manager/main.go` (3 handlers), `frontend/src/hooks/usePlants.ts` (URL + parsing), `docs/architecture.md`, `docs/future-improvements.md` |
| **Unchanged** | `shared/models/`, all other frontend files |

**Net change:** approximately −240 lines (aggregator removal dominates).
