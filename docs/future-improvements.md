# 後續修改計畫參考

這份文件記錄已知的架構改善方向，作為未來修改的參考依據。

> **更新（2026-04-09）：** 方案 A 已實作。aggregator service 已移除，plant-manager 現在直接查詢 `plant-panel-*`。詳見 `docs/superpowers/specs/2026-04-09-remove-aggregator-direct-query-design.md`。

## Time-Series 資料管線：late-arriving data 與 pre-aggregation

### 目前狀態（2026-04-09）

```
mock-plant → log file → Fluentd → ES (plant-panel-*)
                                       ↓
                                   aggregator (10s tick)
                                       ↓
                                   ES (plant-summary-*) ← UPSERT by {plantId}_{epoch_second}
                                       ↓
                                   plant-manager API
                                       ↓
                                   frontend (5m / 1s)
```

Aggregator 採用 **rolling window + idempotent UPSERT** 模式（業界 stream processing 標準的 watermark + idempotent sink 組合）：

- 查詢窗口 `now-45s..now-5s`（40 秒寬），每 10 秒執行一次
- 每個秒會被 4 個連續 cycle 處理，給遲到的 panel data 多次機會被聚合
- Doc ID = `{plantId}_{epoch_second}`，確保 UPSERT 不會重複計算

### 為什麼需要這個機制

Mock plant 每秒產生 panel readings → Fluentd buffer 有 ~1-2 秒 flush 延遲 → ES indexing 又有延遲。aggregator 如果用「每 10 秒一個固定窗口」會錯過邊界區的資料，導致 chart 上出現 false drops（例如 41800W 突然掉到 10000W）。

### 已知限制

1. **邏輯複雜度高**：rolling window + UPSERT 需要小心維護 doc ID 設計和窗口時序
2. **高量資料下仍會撞牆**：watermark 是物理性問題，無法靠變大窗口無限解決
3. **預聚合（pre-aggregation）是 late data 問題的根源**：summary 一旦寫入就「定下來」了

---

## 未來可能的改善方向

### 方案 A：移除 aggregator，API 直接查 raw panel data ✅ 已實作（2026-04-09）

**改動範圍：**
- 重寫 `plant-manager` 的 `/api/plants/{id}/history` 和 `/api/power/history`，改為直接對 `plant-panel-*` 做 `date_histogram + sum(watt)`
- 刪除整個 `services/aggregator/` 服務
- 從 docker-compose 移除 aggregator
- 移除 ES 的 `plant-summary-*` index

**為什麼這能解決 late data：**
- 不再有預聚合的「凍結時刻」
- 每次 API 查詢都重新計算
- 新到的 panel data 下一次查詢時自然會被包含
- Late data 問題自動消失

**適用條件：**
- 資料量在 ES 直接查 raw data 的能力範圍內
- 對 demo 量級（3 plants × 15 panels × 5 分鐘 ≈ 13500 docs）來說毫秒級
- 量級擴大到每秒上萬筆 panels 才需要重新評估

**權衡：**
- ✅ 架構大幅簡化（移除一整個服務）
- ✅ Late data 問題從根本消失
- ✅ 不用維護 watermark / UPSERT / rolling window 邏輯
- ❌ 失去 pre-aggregation 的查詢加速（在 demo 量級下無感）
- ❌ 失去「完整資料管線」的教學展示價值

### 方案 B：換成 InfluxDB

**討論結論：不推薦**。

原因：
1. **不會解決根本問題**：InfluxDB 的 `tasks`（continuous queries）跟我們的 aggregator 有完全一樣的時序問題，watermark 是物理性難題
2. **InfluxDB 的優勢被當前 demo 量級掩蓋**：
   - `fill(previous)` 內建前向填充
   - `aggregateWindow(createEmpty: true)` 空桶顯式表示
   - Tags 模型對 group-by 友善
   - 連續寫入吞吐量優化
   - 內建 downsampling pipeline
   - 這些對每秒幾萬筆寫入、長期歷史保留的場景才有意義
3. **遷移成本高**：
   - Fluentd → Telegraf
   - 重寫 plant-manager 所有 ES query
   - 重寫 init scripts
   - 對 demo 而言不成比例

**適用條件：** 如果未來規模擴大到每秒上萬筆寫入、需要超長歷史資料保留、需要正式的 downsampling pipeline，再評估。

### 方案 C：維持現狀（rolling window + UPSERT）

**已實作。** 適合保留作為「展示完整資料管線」的教學範例，包含 ingestion → buffering → aggregation → storage → API → UI 的所有環節。

---

## 決策原則

當 chart 上又出現 false drops 或 late data 相關問題時，按優先順序考慮：

1. **先檢查窗口大小是否夠寬**：如果 Fluentd flush 延遲超過 30 秒，那已經是 Fluentd 或 ES 的問題，不是 aggregator
2. **檢查是否有 doc ID 衝突或重複**：用 ES 的 `GET plant-summary-*/_search?q=plantId:xxx` 看同一個 (plant, second) 是否有多份
3. **如果這個架構成為維護負擔**：認真考慮方案 A（移除 aggregator）
4. **不要輕易換儲存引擎**：除非有明確的量級壓力

---

## 相關 commits 與 specs

- 設計：`docs/superpowers/specs/2026-04-09-aggregator-per-second-summary-design.md`
- 計畫：`docs/superpowers/plans/2026-04-09-aggregator-per-second-summary.md`
- Rolling window + UPSERT 實作：`8114016` (20s 窗口) → `49384ad` (擴大到 40s)
