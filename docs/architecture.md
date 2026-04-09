# SolarOps 系統架構文件

> 最後更新：2026-03-31

## 系統概覽

SolarOps 是一個太陽能電廠即時監控平台。分散各地的太陽能廠每秒產生發電資料，透過 Fluentd 寫入 Elasticsearch；後端服務負責聚合、告警與指令控制；前端透過 REST 輪詢資料、WebSocket 接收事件。

### 設計原則

- **NATS = 事件通道**：電廠狀態、告警、操控指令
- **Elasticsearch = 資料倉儲**：所有原始讀值與聚合摘要，按日分 index
- **前端輪詢**：儀表板每 3 秒拉取摘要；面板詳情每 2 秒拉取最新讀值

---

## 服務清單

| 服務 | 語言 | Port | 說明 |
|------|------|------|------|
| `mock-plant` | Go | — | 模擬太陽能廠，每秒發布資料 |
| `ws-gateway` | Go | 8080 | WebSocket 閘道，橋接 NATS ↔ 前端 |
| `alert-service` | Go | 8081 | 即時告警偵測與管理 |
| `plant-manager` | Go | 8082 | ES 查詢閘道、NATS 自動發現、面板指令轉發 |
| `aggregator` | Go | — | 每 10 秒從 ES 聚合摘要並寫回 ES |
| `frontend` | React/TS | 3000 | 監控儀表板（Nginx 反向代理） |
| `elasticsearch` | — | 9200 | 資料倉儲 |
| `nats` | — | 4222 | 訊息匯流排 |
| `fluentd` | — | — | 日誌採集，每個電廠一個 sidecar |
| `kibana` | — | 5601 | ES 資料視覺化（開發用） |

---

## 整體架構圖

```mermaid
graph TB
    subgraph Plants["太陽能廠（可動態新增）"]
        MP1["mock-plant-1\nSunrise Valley\n5 panels × 3000W"]
        MP2["mock-plant-2\nGolden Ridge\n4 panels × 2500W"]
        MP3["mock-plant-3\nBlue Horizon\n6 panels × 2800W"]
    end

    subgraph Fluentd["Fluentd sidecar（每廠一個）"]
        FD1["fluentd-1"]
        FD2["fluentd-2"]
        FD3["fluentd-3"]
    end

    subgraph Infra["基礎設施"]
        NATS["NATS\n:4222"]
        ES["Elasticsearch\n:9200\nplant-panel-YYYY-MM-DD\nplant-summary-YYYY-MM-DD"]
    end

    subgraph Backend["後端服務"]
        WS["ws-gateway\n:8080"]
        AS["alert-service\n:8081"]
        PM["plant-manager\n:8082"]
        AGG["aggregator\n每 10 秒"]
    end

    subgraph Frontend["前端"]
        FE["React frontend\n:3000\n（Nginx 反向代理）"]
    end

    subgraph Browser["瀏覽器"]
        DASH["Dashboard\n輪詢每 3s"]
        DETAIL["Plant Detail\n輪詢每 2s"]
        WS_CONN["WebSocket\n事件接收"]
    end

    MP1 -->|"plant.{id}.panel.data\n每秒每面板"| NATS
    MP1 -->|"plant.{id}.status\n啟動 + 每 30s"| NATS
    MP1 -->|"panel JSON log"| FD1
    MP2 -->|"plant.{id}.panel.data"| NATS
    MP2 -->|"plant.{id}.status"| NATS
    MP2 -->|"panel JSON log"| FD2
    MP3 -->|"plant.{id}.panel.data"| NATS
    MP3 -->|"plant.{id}.status"| NATS
    MP3 -->|"panel JSON log"| FD3

    FD1 -->|"plant-panel-YYYY-MM-DD"| ES
    FD2 -->|"plant-panel-YYYY-MM-DD"| ES
    FD3 -->|"plant-panel-YYYY-MM-DD"| ES

    AS -->|"訂閱 plant.*.panel.data"| NATS
    AS -->|"發布 alert.new / alert.resolved"| NATS

    WS -->|"訂閱 plant.*.status"| NATS
    WS -->|"訂閱 alert.>"| NATS
    WS -->|"發布 plant.{id}.command"| NATS

    AGG -->|"查詢 plant-panel-*"| ES
    AGG -->|"寫入 plant-summary-YYYY-MM-DD"| ES

    PM -->|"查詢 plant-panel-* / plant-summary-*"| ES
    PM -->|"訂閱 plant.*.status"| NATS
    PM -->|"發布 plant.{id}.command"| NATS

    FE -->|"反向代理 /ws"| WS
    FE -->|"反向代理 /api/*"| PM
    FE -->|"反向代理 /api/alerts"| AS

    DASH -->|"GET /api/plants/summary\n每 3s"| FE
    DETAIL -->|"GET /api/plants/{id}/panels\n每 2s"| FE
    DETAIL -->|"GET /api/plants/{id}/history"| FE
    WS_CONN -->|"WebSocket /ws\n事件接收 + 指令發送"| FE
```

---

## 資料流

### 1. 資料寫入流程（每秒）

```mermaid
sequenceDiagram
    participant MP as mock-plant
    participant NATS
    participant FD as Fluentd
    participant ES as Elasticsearch

    MP->>NATS: plant.{id}.panel.data × N panels
    MP->>FD: 寫入 /var/log/plant/data.log (JSON)
    FD->>ES: 寫入 plant-panel-YYYY-MM-DD<br/>（flush 每 1s）
```

### 2. 聚合流程（每 10 秒）

```mermaid
sequenceDiagram
    participant AGG as aggregator
    participant ES as Elasticsearch

    AGG->>ES: 查詢 plant-panel-* (now-15s to now-5s)<br/>group by plantId → date_histogram(1s) → sum(watt)<br/>cardinality(panelId), status counts
    ES-->>AGG: 各電廠每秒聚合結果（~10 buckets/plant）
    AGG->>ES: 寫入 plant-summary-YYYY-MM-DD<br/>（每秒一筆，timestamp 取自 ES bucket key，~10 筆/電廠/cycle）
```

### 3. 前端資料取得流程

```mermaid
sequenceDiagram
    participant Browser
    participant Nginx
    participant PM as plant-manager
    participant ES as Elasticsearch
    participant WS as ws-gateway
    participant NATS

    Browser->>Nginx: GET /api/plants/summary (每 3s)
    Nginx->>PM: 轉發
    PM->>ES: 查詢 plant-summary-* (top_hits per plant)
    ES-->>PM: 最新摘要
    PM-->>Browser: JSON

    Browser->>Nginx: GET /api/plants/{id}/panels (每 2s)
    Nginx->>PM: 轉發
    PM->>ES: 查詢 plant-panel-* (top_hits per panel)
    ES-->>PM: 最新面板讀值
    PM-->>Browser: JSON

    Browser->>Nginx: WebSocket /ws
    Nginx->>WS: 升級
    NATS-->>WS: alert.new / alert.resolved / plant.*.status
    WS-->>Browser: ALERT_NEW / ALERT_RESOLVED / PLANT_REGISTERED 事件
    Browser->>WS: PANEL_OFFLINE / PANEL_ONLINE / PANEL_RESET 指令
    WS->>NATS: plant.{id}.command
```

### 4. 告警偵測流程

```mermaid
sequenceDiagram
    participant MP as mock-plant
    participant NATS
    participant AS as alert-service

    MP->>NATS: plant.{id}.panel.data (每秒)
    NATS->>AS: 即時推送
    AS->>AS: Detector.Feed() → Check()
    AS->>NATS: alert.new (若偵測到異常)
    NATS->>Browser: 透過 ws-gateway 推送至前端
```

---

## NATS 主題對應

| 主題 | 方向 | 說明 |
|------|------|------|
| `plant.{id}.status` | mock-plant → NATS | 電廠上線狀態，啟動時 + 每 30s 心跳 |
| `plant.{id}.panel.data` | mock-plant → NATS | 每秒每面板讀值（PanelReading） |
| `plant.{id}.command` | NATS → mock-plant | 操控指令：OFFLINE / ONLINE / RESET / FAULT |
| `alert.new` | alert-service → NATS | 新告警觸發 |
| `alert.resolved` | alert-service → NATS | 告警解除（手動 resolve 時發布） |

---

## Elasticsearch Index 結構

### `plant-panel-YYYY-MM-DD`（Fluentd 寫入）

每秒每面板一筆，由 Fluentd logstash_format 按日建立。

| 欄位 | 類型 | 說明 |
|------|------|------|
| `@timestamp` | date | Fluentd 事件時間，供 ES/Kibana 查詢使用 |
| `timestamp` | date | PanelReading struct 原始時間，供前端 TypeScript 使用 |
| `plantId` | keyword | 電廠 UUID |
| `plantName` | keyword | 電廠名稱 |
| `panelId` | keyword | 面板 UUID |
| `panelNumber` | integer | 面板編號 |
| `status` | keyword | `online` / `offline` |
| `faultMode` | keyword | 故障模式（正常時欄位不存在） |
| `watt` | float | 當下發電量（瓦） |

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

---

## 資料生命週期（ILM）

| Index Pattern | 保留天數 | 估計資料量 |
|---------------|---------|-----------|
| `plant-panel-*` | 7 天 | ~130 萬筆/天（3 廠 × 5 面板 × 86400 秒） |
| `plant-summary-*` | 30 天 | ~26000 筆/天（3 廠 × 8640 筆/10 秒） |

ILM policy 由 `es-init` 容器在啟動時建立，新建的 index 自動套用。
修改 template 不影響既有 index；開發環境可用 `docker compose down -v` 重建使其生效。

---

## Plant Manager API

plant-manager 是前端的 BFF（Backend For Frontend），負責 ES 查詢閘道與 NATS 指令轉發。
電廠透過 NATS 心跳自動註冊，不需要 API 呼叫。

| Method | Path | 說明 |
|--------|------|------|
| `GET` | `/api/plants` | 列出已知電廠（自動發現的 registry） |
| `GET` | `/api/plants/summary` | 儀表板輪詢：查 `plant-summary-*` top_hits |
| `GET` | `/api/plants/{plantId}/panels` | 面板詳情輪詢：查 `plant-panel-*` top_hits |
| `GET` | `/api/plants/{plantId}/history` | 歷史功率曲線：date_histogram |
| `POST` | `/api/plants/{plantId}/panels/{panelId}/fault` | 觸發面板故障指令 |

---

## Alert Service API

| Method | Path | 說明 |
|--------|------|------|
| `GET` | `/api/alerts` | 列出告警，可加 `?status=active\|acknowledged` 過濾 |
| `POST` | `/api/alerts/{id}/acknowledge` | 確認告警（active → acknowledged） |
| `POST` | `/api/alerts/{id}/resolve` | 解除告警（從 store 刪除） |

### 告警偵測規則

偵測器對每個面板保留最新 20 筆讀值（滑動視窗），每次收到新讀值後執行檢查：

| 告警類型 | 觸發條件 | 參數 |
|---------|---------|------|
| `PANEL_FAULT` | 末尾連續 0W 讀值達閾值 | 3 次 |
| `PANEL_DEGRADED` | 視窗首筆→末筆發電量下降比例超閾值 | ≥ 30% |
| `PANEL_UNSTABLE` | 視窗內 0W ↔ 非 0W 翻轉次數超閾值 | ≥ 5 次 |

相同 `(plantId, panelId, type)` 組合若已有 active/acknowledged 告警，則不重複建立。

### 告警工作流程

```mermaid
stateDiagram-v2
    [*] --> active : 偵測器觸發（Detector.Check）
    active --> acknowledged : 使用者按 ACK
    acknowledged --> [*] : 使用者按 Resolved（從 store 刪除）
```

> 使用者按 Resolved 時，alert-service 從 store 刪除告警並發布 `alert.resolved` 到 NATS，ws-gateway 廣播 `ALERT_RESOLVED` 至所有連線的瀏覽器分頁。

---

## 前端狀態機

```mermaid
stateDiagram-v2
    [*] --> online : 首次出現於 summary 輪詢
    online --> fault : summary.faultyCount > 0
    fault --> online : faultyCount 回到 0
    online --> stale : 超過 30s 未收到新 summary
    stale --> online : 再次收到 summary
    stale --> offline : 超過 90s 未收到新 summary
    offline --> [*] : 使用者手動移除
```

---

## Docker Compose 服務拓撲

```mermaid
graph LR
    nats["nats\n(healthy)"]
    es["elasticsearch\n(healthy)"]
    kibana["kibana"] --> es
    es_init["es-init"] --> es

    ws["ws-gateway"] --> nats
    as["alert-service"] --> nats
    pm["plant-manager"] --> nats
    pm --> es

    agg["aggregator"] --> es

    mp1["mock-plant-1"] --> nats
    mp2["mock-plant-2"] --> nats
    mp3["mock-plant-3"] --> nats

    fd1["fluentd-1"] --> es
    fd2["fluentd-2"] --> es
    fd3["fluentd-3"] --> es

    fe["frontend\n:3000"] --> ws
    fe --> pm
    fe --> as
```

---

## 新增電廠

系統透過 NATS 心跳自動發現電廠，不需要呼叫 API。新增步驟：

1. 在 `docker-compose.yml` 新增一組 mock-plant + fluentd sidecar
2. 執行 `docker compose up -d`（只啟動新增的 container，不影響已運行的服務）
3. 新 mock-plant 連接 NATS，發布 `plant.{id}.status`
4. plant-manager 與 ws-gateway 自動訂閱到狀態，前端收到 `PLANT_REGISTERED` 事件
5. Fluentd sidecar 將資料寫入 ES，前端 3s 後的下一次 summary 輪詢即可看到新電廠

---

## 模組結構

```
solarops/
├── go.work                      # Go workspace (go 1.25.0)
├── shared/                      # 共用 models (PlantInfo, PanelReading, PlantSummary, Command...)
├── services/
│   ├── mock-plant/              # 模擬電廠
│   │   ├── plant/               # Plant struct, GeneratePanelReadings(), HandleCommand()
│   │   └── logger/              # 寫入 JSON log 供 Fluentd 採集
│   ├── ws-gateway/
│   │   └── hub/                 # WebSocket client 管理（broadcast channel）
│   ├── alert-service/
│   │   ├── detector/            # 異常偵測邏輯
│   │   └── store/               # 告警記憶體儲存
│   ├── plant-manager/           # 電廠 registry（NATS 自動發現）+ ES 查詢閘道
│   └── aggregator/              # ES 讀取聚合 → 摘要寫回
├── frontend/
│   └── src/
│       ├── hooks/usePlants.ts   # 輪詢 + 告警狀態管理
│       ├── hooks/useWebSocket.ts
│       ├── pages/Dashboard.tsx  # 儀表板（電廠卡、告警、功率圖）
│       └── pages/PlantDetail.tsx # 面板詳情 + 歷史功率圖
└── infra/
    ├── elasticsearch/           # Index template 初始化腳本
    └── fluentd/                 # fluent.conf + Dockerfile
```

---

## MVP 已知限制與設計取捨

本專案定位為 MVP（Minimum Viable Product），以下項目是刻意的設計取捨，非遺漏。
後續迭代可依優先順序逐步改善。

### 無持久化（Alert Store / Plant Registry）

- Alert-service 使用 in-memory `map[string]*Alert`，重啟後告警消失。
- Plant-manager 的 registry 同為 in-memory，重啟後需等待各電廠心跳重新註冊（≤ 30s）。
- **MVP 理由**：單人開發環境重啟頻率低，持久化引入 Redis/ES 寫入會增加複雜度。
- **未來方向**：告警寫入 ES（`alert-YYYY-MM-DD` index），registry 寫入 Redis。

### 雙資料路徑（NATS + Fluentd）

- 面板資料同時經由兩條路徑：NATS（即時，供 alert-service）與 log → Fluentd → ES（buffered，供查詢）。
- 兩條路徑有 1~2 秒時間差，若 Fluentd 崩潰會靜默丟失 ES 資料。
- **MVP 理由**：職責分離明確——NATS 負責即時事件、Fluentd 負責日誌採集與 ES 寫入。合併為單一路徑需重新設計 ingestion 層。
- **未來方向**：評估由 Go 服務直接訂閱 NATS 寫入 ES，移除 Fluentd 依賴。

### 無 API 認證

- 所有 REST endpoint 和 WebSocket 無認證/授權。
- **MVP 理由**：靠 Docker network 隔離，前端 Nginx 僅代理必要路徑。
- **未來方向**：加入 JWT 或 API key 認證。

### 前端 Staleness 判斷

- 電廠上下線狀態由前端 `Date.now()` 計時器判斷（30s → stale、90s → offline）。
- 瀏覽器 Tab 進入背景時 `setInterval` 頻率降低，計時不精確。
- **MVP 理由**：單人使用場景，Tab 切換後回來會自動校正。
- **未來方向**：Server-side 電廠狀態管理，前端僅展示。

### NATS JetStream 啟用但未使用

- Docker Compose 的 NATS 以 `-js` 啟動，但所有服務使用 basic pub/sub。
- **MVP 理由**：預留擴展空間，JetStream 額外記憶體消耗極低（數 MB）。
- **未來方向**：告警持久化、訊息回放等功能可改用 JetStream stream。
