package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
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
	esURL := envOrDefault("ES_URL", "http://localhost:9200")
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

	es, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{esURL},
	})
	if err != nil {
		log.Fatalf("ES connect: %v", err)
	}

	alertStore := store.New()
	det := detector.NewDetector(3, 30.0, 5)

	// Periodic detection loop every 10 seconds
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			queryAndDetect(es, det, alertStore, nc)
		}
	}()

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

func queryAndDetect(es *elasticsearch.Client, det *detector.Detector, alertStore *store.Store, nc *nats.Conn) {
	query := map[string]interface{}{
		"size": 1000,
		"query": map[string]interface{}{
			"range": map[string]interface{}{
				"timestamp": map[string]interface{}{"gte": "now-30s"},
			},
		},
		"sort": []map[string]interface{}{{"timestamp": "asc"}},
	}

	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(query)

	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Second)
	defer cancel()

	res, err := es.Search(
		es.Search.WithContext(ctx),
		es.Search.WithIndex("plant-data"),
		es.Search.WithBody(&buf),
	)
	if err != nil {
		log.Printf("ES query error: %v", err)
		return
	}
	defer res.Body.Close()

	if res.IsError() {
		body, _ := io.ReadAll(res.Body)
		log.Printf("ES error response: %s", body)
		return
	}

	var esResult struct {
		Hits struct {
			Hits []struct {
				Source models.PlantData `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(res.Body).Decode(&esResult); err != nil {
		log.Printf("ES decode error: %v", err)
		return
	}

	for _, hit := range esResult.Hits.Hits {
		data := hit.Source
		for _, panel := range data.Panels {
			det.Feed(data.PlantID, panel.PanelID, panel.PanelNumber, data.PlantName, panel.Watt, data.Timestamp)
		}
	}

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

	// Check DATA_GAP: plants that haven't reported
	checkDataGaps(es, alertStore, nc)
}

func checkDataGaps(es *elasticsearch.Client, alertStore *store.Store, nc *nats.Conn) {
	// Query distinct plants in last 60 seconds vs last 10 seconds
	// If a plant exists in 60s window but not in 10s window, it's a data gap
	// Simplified: just check if any known plants have no recent data
	// This is handled by the detector via absence of Feed calls
}
