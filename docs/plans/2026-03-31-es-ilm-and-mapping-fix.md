# ES ILM Policy、@timestamp Mapping 修正、架構文件更新

> **Goal:** 修正 index template mapping，加入 ILM 自動清理，aggregator 同時寫 `@timestamp` + `timestamp` 實現雙用途，同步更新架構文件。

---

## 背景問題

1. **`@timestamp` 未明確 mapping**：Fluentd `logstash_format true` 產生 `@timestamp`，aggregator 和 plant-manager 查詢都用 `@timestamp`，但 template 沒有顯式定義它，靠 ES 動態推斷。

2. **`plant-summary-*` 只有 `timestamp` 缺少 `@timestamp`**：`plant-panel-*` 有兩個時間欄位（`@timestamp` 給 ES/Kibana，`timestamp` 給前端 TypeScript），但 `plant-summary-*` 只有 `timestamp`。為了跨 index 一致性，aggregator 應同時寫入 `@timestamp` 和 `timestamp`（相同值）。

3. **無資料生命週期管理**：`plant-panel-*` 每秒每面板一筆，3 廠 × 5 面板 ≈ 130 萬筆/天，長期運行會吃滿磁碟。

4. **架構文件過時**：`plant.{id}.summary` NATS 主題已移除但文件未更新。

---

## 修正計劃

### Step 1：修正 `init-index.sh` — ILM + template mapping

**File:** `infra/elasticsearch/init-index.sh`

變更內容：
- 先建 ILM policy，再建 index template
- `plant-panel-*` template：加上 `"@timestamp": { "type": "date" }`（保留 `timestamp`）
- `plant-summary-*` template：加上 `"@timestamp": { "type": "date" }`（保留 `timestamp`）
- 兩個 template settings 都加上 `index.lifecycle.name`

### Step 2：aggregator 同時寫入 `@timestamp` 和 `timestamp`

**File:** `services/aggregator/main.go`（~L185）

```go
// 現況
"timestamp": now.Format(time.RFC3339Nano),

// 改為
"@timestamp": now.Format(time.RFC3339Nano),
"timestamp":  now.Format(time.RFC3339Nano),
```

### Step 3：plant-manager 查詢改用 `@timestamp`

**File:** `services/plant-manager/main.go`

summary 相關查詢目前用 `"timestamp"` 做 range filter 和 sort，改用 `"@timestamp"` 讓跨 index 查詢一致：
- `GET /api/plants/summary`（~L263）：`"timestamp": {"gte": "now-30s"}` → `"@timestamp"`
- `GET /api/plants/summary`（~L277）：`sort: [{"timestamp": "desc"}]` → `"@timestamp"`
- `GET /api/plants/{plantId}/history`（~L219）：`"timestamp": {"gte": ...}` → `"@timestamp"`
- `GET /api/plants/{plantId}/history`（~L227）：`date_histogram.field: "timestamp"` → `"@timestamp"`

前端仍透過 `_source.timestamp` 取值，不受影響。

### Step 4：更新 `docs/architecture.md`

**File:** `docs/architecture.md`

1. **NATS 主題表（~L192）**：刪除 `plant.{id}.summary` 整行
2. **ES index 結構**：兩個 index 都標注同時有 `@timestamp` 和 `timestamp`，說明用途差異
3. **新增「資料生命週期」章節**：說明 ILM 7/30 天保留策略

---

## 驗證方式

```sh
# 重建環境（需要清 volume 讓新 template 生效）
docker compose down -v && docker compose up -d

# 等 30 秒後確認
# 1. ILM policy 存在
curl -s http://localhost:9200/_ilm/policy/plant-panel-policy | python3 -m json.tool

# 2. Template 有 @timestamp 和 ILM 設定
curl -s http://localhost:9200/_index_template/plant-panel-template | python3 -m json.tool
curl -s http://localhost:9200/_index_template/plant-summary-template | python3 -m json.tool

# 3. Summary index 有 @timestamp 欄位
curl -s "http://localhost:9200/plant-summary-*/_mapping" | python3 -m json.tool

# 4. 現有 index 已套用 ILM
curl -s "http://localhost:9200/plant-panel-*/_ilm/explain" | python3 -m json.tool

# 5. 前端 summary 輪詢正常（資料有回來）
curl -s http://localhost:3000/api/plants/summary | python3 -m json.tool
```
