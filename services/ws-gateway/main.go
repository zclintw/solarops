package main

import (
    "encoding/json"
    "log"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/gorilla/websocket"
    "github.com/nats-io/nats.go"
    "github.com/solarops/shared/models"
    "github.com/solarops/ws-gateway/hub"
)

var upgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool { return true },
}

func envOrDefault(key, def string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return def
}

func main() {
    natsURL := envOrDefault("NATS_URL", nats.DefaultURL)
    addr := envOrDefault("LISTEN_ADDR", ":8080")

    // Connect to NATS
    nc, err := nats.Connect(natsURL,
        nats.RetryOnFailedConnect(true),
        nats.MaxReconnects(-1),
        nats.ReconnectWait(2*time.Second),
    )
    if err != nil {
        log.Fatalf("NATS connect: %v", err)
    }
    defer nc.Close()
    log.Printf("Connected to NATS: %s", natsURL)

    h := hub.New()

    // Subscribe to plant data — forward to all WebSocket clients
    nc.Subscribe("plant.*.data", func(msg *nats.Msg) {
        wsMsg := models.WSMessage{Type: models.MsgPlantData, Payload: json.RawMessage(msg.Data)}
        data, _ := json.Marshal(wsMsg)
        h.Broadcast(data)
    })

    // Subscribe to plant status — forward registered/unregistered events
    nc.Subscribe("plant.*.status", func(msg *nats.Msg) {
        var info map[string]interface{}
        json.Unmarshal(msg.Data, &info)

        msgType := models.MsgPlantRegistered
        if status, ok := info["status"].(string); ok && status == "offline" {
            msgType = models.MsgPlantUnregistered
        }

        wsMsg := models.WSMessage{Type: msgType, Payload: json.RawMessage(msg.Data)}
        data, _ := json.Marshal(wsMsg)
        h.Broadcast(data)
    })

    // Subscribe to alerts
    nc.Subscribe("alert.>", func(msg *nats.Msg) {
        var msgType string
        switch msg.Subject {
        case "alert.new":
            msgType = models.MsgAlertNew
        case "alert.resolved":
            msgType = models.MsgAlertResolved
        default:
            return
        }
        wsMsg := models.WSMessage{Type: msgType, Payload: json.RawMessage(msg.Data)}
        data, _ := json.Marshal(wsMsg)
        h.Broadcast(data)
    })

    // WebSocket handler
    http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
        conn, err := upgrader.Upgrade(w, r, nil)
        if err != nil {
            log.Printf("WS upgrade error: %v", err)
            return
        }

        ch := make(chan []byte, 256)
        h.Register(ch)
        log.Printf("Client connected (total: %d)", h.ClientCount())

        // Writer goroutine: send NATS messages to WebSocket client
        go func() {
            defer conn.Close()
            for msg := range ch {
                if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
                    break
                }
            }
        }()

        // Reader goroutine: receive control commands from WebSocket client, publish to NATS
        for {
            _, message, err := conn.ReadMessage()
            if err != nil {
                break
            }

            var wsMsg models.WSMessage
            if err := json.Unmarshal(message, &wsMsg); err != nil {
                continue
            }

            // Extract plantId and panelId from payload
            payloadBytes, _ := json.Marshal(wsMsg.Payload)
            var cmdPayload struct {
                PlantID string `json:"plantId"`
                PanelID string `json:"panelId"`
            }
            json.Unmarshal(payloadBytes, &cmdPayload)

            var cmd models.Command
            switch wsMsg.Type {
            case models.MsgPanelOffline:
                cmd = models.Command{Command: models.CmdOffline, PanelID: cmdPayload.PanelID}
            case models.MsgPanelOnline:
                cmd = models.Command{Command: models.CmdOnline, PanelID: cmdPayload.PanelID}
            case models.MsgPanelReset:
                cmd = models.Command{Command: models.CmdReset, PanelID: cmdPayload.PanelID}
            default:
                continue
            }

            cmdBytes, _ := json.Marshal(cmd)
            subject := "plant." + cmdPayload.PlantID + ".command"
            nc.Publish(subject, cmdBytes)
        }

        h.Unregister(ch)
        log.Printf("Client disconnected (total: %d)", h.ClientCount())
    })

    // Health check endpoint
    http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("ok"))
    })

    // Start HTTP server
    go func() {
        log.Printf("WS Gateway listening on %s", addr)
        if err := http.ListenAndServe(addr, nil); err != nil {
            log.Fatalf("HTTP server: %v", err)
        }
    }()

    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    <-sigCh
    log.Println("Shutting down...")
}
