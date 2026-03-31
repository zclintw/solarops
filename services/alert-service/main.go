package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/solarops/alert-service/detector"
	"github.com/solarops/alert-service/store"
	"github.com/solarops/shared/models"
)

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	natsURL := envOrDefault("NATS_URL", nats.DefaultURL)
	addr := envOrDefault("LISTEN_ADDR", ":8081")

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

	alertStore := store.New()
	det := detector.NewDetector(3, 30.0, 5)

	// Subscribe to real-time panel data
	nc.Subscribe("plant.*.panel.data", func(msg *nats.Msg) {
		var reading models.PanelReading
		if err := json.Unmarshal(msg.Data, &reading); err != nil {
			return
		}
		det.Feed(reading.PlantID, reading.PanelID, reading.PanelNumber, reading.PlantName, reading.Watt, reading.Timestamp)
		newAlerts := det.Check()
		for _, alert := range newAlerts {
			if _, found := alertStore.FindActive(alert.PlantID, alert.PanelID, alert.Type); found {
				continue
			}
			created := alertStore.Create(alert)
			alertJSON, _ := json.Marshal(created)
			nc.Publish("alert.new", alertJSON)
			log.Printf("New alert: %s - %s", created.Type, created.Message)
		}
	})
	log.Println("Subscribed to plant.*.panel.data")

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/alerts", func(w http.ResponseWriter, r *http.Request) {
		statusFilter := r.URL.Query().Get("status")
		alerts := alertStore.List(statusFilter)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(alerts)
	})

	mux.HandleFunc("POST /api/alerts/{id}/acknowledge", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if alertStore.Acknowledge(id) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "acknowledged"})
		} else {
			http.Error(w, "alert not found", http.StatusNotFound)
		}
	})

	mux.HandleFunc("POST /api/alerts/{id}/resolve", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if alertStore.Delete(id) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "resolved"})
		} else {
			http.Error(w, "alert not found", http.StatusNotFound)
		}
	})

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	go func() {
		log.Printf("Alert Service listening on %s", addr)
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Fatalf("HTTP server: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Println("Shutting down...")
}
