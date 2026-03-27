# SolarOps MVP — 太陽能電廠監控平台設計

## 1. 概述

分散各處的太陽能電廠即時回報產電數據，後端接收並推送至前端 dashboard，操作員可即時監控各電廠狀態、管理告警、遠端控制太陽能板。系統支援運行中動態新增/移除電廠，無需重啟。

## 2. 技術選型

| 層面 | 技術 | 理由 |
|------|------|------|
| 後端微服務 | Go | 高效能、goroutine 天然適合並發、單一 binary 部署 |
| 前端 | React | 指定需求 |
| 即時通道 | WebSocket | 雙向通訊：推數據 + 接收操控指令 |
| 訊息匯流排 | NATS | 輕量、單一 binary、天然 pub/sub、動態擴充友善 |
| 日誌收集 | Fluentd (sidecar) | 每電廠一個，本地 buffer 避免丟失 |
| 資料存儲/分析 | Elasticsearch | 歷史查詢、聚合分析、趨勢偵測 |
| 容器化 | Docker Compose | 統一編排所有服務 |

## 3. 系統架構

```
┌──────────────────────────────────────────────────────────────────┐
│                         Docker Compose                           │
│                                                                  │
│  ┌─────────────────────┐  ┌─────────────────────┐                │
│  │  Mock Plant pod     │  │  Mock Plant pod     │  ... (×N)      │
│  │ ┌───────────┐       │  │ ┌───────────┐       │                │
│  │ │Mock Plant │→ log  │  │ │Mock Plant │→ log  │                │
│  │ └─────┬─────┘  file │  │ └─────┬─────┘  file │                │
│  │       │         ↓   │  │       │         ↓   │                │
│  │       │  ┌────────┐ │  │       │  ┌────────┐ │                │
│  │       │  │Fluentd │ │  │       │  │Fluentd │ │                │
│  │       │  │sidecar │─┼──┼───────┼──│sidecar │─┼──┐             │
│  │       │  └────────┘ │  │       │  └────────┘ │  │             │
│  └───────┼─────────────┘  └───────┼─────────────┘  │             │
│          │ NATS                    │                 ▼             │
│          └────────┬────────────────┘          ┌──────────┐        │
│                   ▼                           │    ES    │        │
│            ┌─────────────┐                    └────┬─────┘        │
│            │    NATS     │                         │              │
│            └──────┬──────┘                         │              │
│        ┌──────────┼───────────┐                    │              │
│        ▼          ▼           ▼                    │              │
│  ┌──────────┐ ┌───────────┐ ┌────────────┐         │              │
│  │   WS     │ │  Alert    ├─┘  Plant     │         │              │
│  │ Gateway  │ │  Service  │ │  Manager   │         │              │
│  └────┬─────┘ └───────────┘ └─────┬──────┘         │              │
│       ▼                           │                              │
│  ┌───────────┐                    │                              │
│  │  React    │◄── REST API ───────┘                              │
│  │ Frontend  │                                                   │
│  └───────────┘                                                   │
└──────────────────────────────────────────────────────────────────┘
```

### 兩條獨立資料路徑

- **即時路徑**：Mock Plant → NATS → WS Gateway → WebSocket → React Frontend
- **持久路徑**：Mock Plant → 本地 log file → Fluentd sidecar → Elasticsearch

### 控制指令統一走 NATS

- 前端操作員：React → WebSocket → WS Gateway → NATS → Mock Plant
- Plant Manager API：HTTP → Plant Manager → NATS → Mock Plant

## 4. 微服務設計

### 4.1 Mock Plant（模擬太陽能電廠）

**職責**：模擬一座太陽能電廠，每秒產生產電數據，支援故障模擬和遠端控制。

**配置方式**：
- 初始電廠：docker-compose.yml 環境變數指定
- 動態新增：Plant Manager 透過 Docker API 啟動新 container

**環境變數**：

| 變數 | 說明 | 範例 |
|------|------|------|
| `PLANT_NAME` | 電廠顯示名稱 | `Sunrise Valley` |
| `PLANT_PANELS` | 太陽能板數量 | `50` |
| `WATT_PER_SEC` | 每板每秒產電量 (W) | `300` |
| `NATS_URL` | NATS 連線位址 | `nats://nats:4222` |

**命名規則**：
- 電廠：有意義的名稱（docker-compose.yml 或 API 指定）+ UUID v4
- 太陽能板：連續編號 `Panel-1`, `Panel-2`, ... + UUID v4

**NATS 互動**：
- Publish：`plant.{plant-id}.data` — 每秒產電數據
- Subscribe：`plant.{plant-id}.command` — 接收控制指令

**數據格式（publish 到 NATS）**：
```json
{
  "plantId": "uuid",
  "plantName": "Sunrise Valley",
  "timestamp": "2026-03-28T10:00:00Z",
  "panels": [
    {
      "panelId": "uuid",
      "panelNumber": 1,
      "status": "online",
      "faultMode": null,
      "watt": 300.0
    },
    {
      "panelId": "uuid",
      "panelNumber": 2,
      "status": "online",
      "faultMode": "DEAD",
      "watt": 0.0
    }
  ],
  "totalWatt": 14700.0,
  "onlineCount": 49,
  "offlineCount": 0,
  "faultyCount": 1
}
```

**控制指令格式（subscribe from NATS）**：
```json
{
  "command": "OFFLINE" | "ONLINE" | "RESET" | "FAULT",
  "panelId": "uuid",
  "faultMode": "DEAD" | "DEGRADED" | "INTERMITTENT"
}
```

**故障模式**：

| 模式 | 行為 |
|------|------|
| `DEAD` | 產電量歸零 |
| `DEGRADED` | 產電量每秒衰減 X% |
| `INTERMITTENT` | 隨機在正常/歸零之間跳動 |

**板子操作**：

| 操作 | 效果 |
|------|------|
| `OFFLINE` | 停止產電、停止回報 |
| `ONLINE` | 恢復產電 |
| `RESET` | 清除故障狀態 + 自動恢復 Online |
| `FAULT` | 設定故障模式，開始模擬 |

**Log 輸出**：同時寫入本地 log file（JSON 格式），供 Fluentd sidecar 收集。

### 4.2 WebSocket Gateway

**職責**：橋接 NATS 與前端 WebSocket 連線。

**功能**：
- 訂閱 `plant.*.data`：即時電廠數據推送給前端
- 訂閱 `alert.>`：告警推送給前端
- 接收前端 WebSocket 訊息：解析控制指令，publish 到對應 `plant.{plant-id}.command`

**WebSocket 訊息類型（server → client）**：
- `PLANT_DATA`：電廠即時數據
- `PLANT_REGISTERED`：新電廠上線
- `PLANT_UNREGISTERED`：電廠離線
- `ALERT_NEW`：新告警
- `ALERT_RESOLVED`：告警自動解除

**WebSocket 訊息類型（client → server）**：
- `PANEL_OFFLINE`：關閉太陽能板
- `PANEL_ONLINE`：開啟太陽能板
- `PANEL_RESET`：重置故障板

### 4.3 Alert Service

**職責**：從 ES 查詢數據做異常偵測，產生告警，管理告警生命週期。

**偵測邏輯**：

| 告警類型 | 偵測方式 | 條件 |
|----------|----------|------|
| `PANEL_FAULT` | 閾值偵測 | 板子產電量 = 0 持續 N 秒 |
| `PANEL_DEGRADED` | 趨勢偵測 | 板子產電量連續 N 秒下降超過 X% |
| `PANEL_UNSTABLE` | 頻率偵測 | 板子在 N 秒內產電量在正常/零之間跳動超過 M 次 |
| `DATA_GAP` | 數據中斷 | 某電廠在 ES 中超過 N 秒無新數據 |

**告警生命週期**：
1. Alert Service 偵測到異常 → 建立告警 → publish 到 NATS `alert.new`
2. 前端收到告警 → 顯示在告警列表
3. 操作員 acknowledge → REST API → Alert Service 更新狀態
4. 異常恢復 → Alert Service 自動標記 resolved → publish `alert.resolved`

**REST API**：
- `GET /api/alerts` — 告警列表（支援過濾：未處理/已確認/已解決）
- `POST /api/alerts/{alert-id}/acknowledge` — 操作員確認告警

**告警儲存**：Alert Service 內部用記憶體管理活躍告警。歷史告警記錄在 ES。

### 4.4 Plant Manager

**職責**：管理電廠生命週期，提供 REST API。

**REST API**：
- `GET /api/plants` — 列出所有電廠
- `POST /api/plants` — 新增電廠（啟動新 container）
- `DELETE /api/plants/{plant-id}` — 移除電廠（停止 container）
- `POST /api/plants/{plant-id}/panels/{panel-id}/fault` — 觸發故障（透過 NATS）
- `GET /api/plants/{plant-id}/history?range=1h&interval=10s` — 歷史數據（查 ES）

**新增電廠流程**：
1. 收到 POST 請求，驗證名稱不重複
2. 透過 Docker API 啟動新 Mock Plant container
3. Mock Plant 啟動後自動連 NATS 開始 publish 數據
4. WS Gateway 自動收到新電廠數據，推送 `PLANT_REGISTERED` 給前端

**移除電廠流程**：
1. 收到 DELETE 請求
2. 透過 Docker API 停止並移除 container
3. WS Gateway 偵測到該電廠不再 publish → 推送 `PLANT_UNREGISTERED`
4. 前端將電廠標示為灰色，等操作員手動移除

### 4.5 React Frontend

**頁面結構**：

#### 總覽頁 `/`
```
┌──────────────────────────────────────────────────┐
│  電廠總數: 5    總產電量: 73,500 W               │
│  ┌────────────────────────────────────────────┐  │
│  │         歷史總產電量折線圖 (10s 聚合)       │  │
│  └────────────────────────────────────────────┘  │
├──────────────────────────────────────────────────┤
│  ┌─────────┐ ┌─────────┐ ┌─────────┐            │
│  │Sunrise  │ │Golden   │ │Blue     │ ...        │
│  │Valley   │ │Ridge    │ │Horizon  │            │
│  │🟢 15kW  │ │🔴 8kW   │ │⚫ 離線  │            │
│  │板:50    │ │板:30    │ │板:40    │            │
│  │正常:50  │ │正常:28  │ │---      │            │
│  │異常:0   │ │異常:2   │ │         │            │
│  └─────────┘ └─────────┘ └─────────┘            │
├──────────────────────────────────────────────────┤
│  告警列表                                        │
│  🔴 Panel-12 @ Golden Ridge - DEAD (未處理)      │
│  🟡 Panel-7 @ Golden Ridge - DEGRADED (已確認)   │
└──────────────────────────────────────────────────┘
```

#### 單廠詳情頁 `/plants/{plant-id}`
```
┌──────────────────────────────────────────────────┐
│  Sunrise Valley    🟢 Online    15,000 W         │
│  ┌────────────────────────────────────────────┐  │
│  │         該廠歷史產電量折線圖                 │  │
│  └────────────────────────────────────────────┘  │
├──────────────────────────────────────────────────┤
│  太陽能板列表                                    │
│  ┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌────┐             │
│  │ 1  │ │ 2  │ │ 3  │ │ 4  │ │ 5  │ ...        │
│  │🟢  │ │🟢  │ │🔴  │ │🟢  │ │🟢  │            │
│  │300W│ │300W│ │0W  │ │300W│ │300W│             │
│  │    │ │    │ │[R] │ │    │ │    │             │
│  └────┘ └────┘ └────┘ └────┘ └────┘             │
│                                                  │
│  點擊板子 → Offline/Online 切換                   │
│  [R] = Reset 按鈕（僅故障板顯示）                 │
└──────────────────────────────────────────────────┘
```

**電廠卡片狀態顏色**：

| 狀態 | 顏色 | 觸發條件 | 自動回綠 |
|------|------|----------|----------|
| 線上 | 🟢 綠色 | 持續收到正常數據 | — |
| 異常 | 🔴 紅色 | 有故障板 | 是，所有板恢復正常時 |
| 數據中斷 | 🟡 黃色 | 超過 N 秒未收到數據 | 是，重新收到數據時 |
| 離線 | ⚫ 灰色 | 長時間無數據 / 電廠移除 | 否，先回黃色再回綠色；操作員可手動移除 |

## 5. NATS Subject 設計

| Subject | Publisher | Subscriber | 用途 |
|---------|-----------|------------|------|
| `plant.{plant-id}.data` | Mock Plant | WS Gateway | 即時產電數據 |
| `plant.{plant-id}.command` | WS Gateway, Plant Manager | Mock Plant | 控制指令 |
| `plant.{plant-id}.status` | Mock Plant | WS Gateway | 電廠上線/下線通知 |
| `alert.new` | Alert Service | WS Gateway | 新告警通知 |
| `alert.resolved` | Alert Service | WS Gateway | 告警解除通知 |

## 6. 時間粒度

| 場景 | 頻率 |
|------|------|
| Mock Plant 產生數據 | 每秒 |
| WebSocket 推送到前端 | 每秒 |
| Fluentd 寫入 ES | 每秒（raw data） |
| 前端折線圖 | 10 秒聚合 |
| Alert Service 偵測週期 | 每 10 秒查一次 ES |

## 7. Docker Compose 服務清單

| 服務 | Image | Port | 依賴 |
|------|-------|------|------|
| `nats` | `nats:latest` | 4222 | — |
| `elasticsearch` | `elasticsearch:8` | 9200 | — |
| `fluentd` | 自建（含 ES plugin） | 24224 | elasticsearch |
| `ws-gateway` | 自建 Go | 8080 | nats |
| `alert-service` | 自建 Go | 8081 | nats, elasticsearch |
| `plant-manager` | 自建 Go | 8082 | nats, elasticsearch, docker.sock |
| `frontend` | 自建 React (nginx) | 3000 | ws-gateway, plant-manager, alert-service |
| `mock-plant-1` | 自建 Go | — | nats |
| `mock-plant-2` | 自建 Go | — | nats |
| `mock-plant-3` | 自建 Go | — | nats |

**注意**：每個 mock-plant 配一個 fluentd sidecar，在 docker-compose 中以 `mock-plant-1-fluentd` 方式定義，tail 對應 plant 的 log volume。

## 8. 專案目錄結構

```
solarops/
├── docker-compose.yml
├── services/
│   ├── mock-plant/          # Go - 模擬太陽能電廠
│   │   ├── main.go
│   │   ├── Dockerfile
│   │   └── ...
│   ├── ws-gateway/          # Go - WebSocket 閘道
│   │   ├── main.go
│   │   ├── Dockerfile
│   │   └── ...
│   ├── alert-service/       # Go - 告警偵測與管理
│   │   ├── main.go
│   │   ├── Dockerfile
│   │   └── ...
│   └── plant-manager/       # Go - 電廠生命週期管理
│       ├── main.go
│       ├── Dockerfile
│       └── ...
├── frontend/                # React - Dashboard
│   ├── src/
│   ├── Dockerfile
│   └── ...
├── infra/
│   └── fluentd/
│       ├── Dockerfile       # 含 ES output plugin
│       └── fluent.conf
└── docs/
    └── plans/
```
