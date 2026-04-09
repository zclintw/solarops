package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/nats-io/nats.go"
	"github.com/solarops/plant-manager/manager"
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
	addr := envOrDefault("LISTEN_ADDR", ":8082")

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

	// Connect to ES
	es, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{esURL},
	})
	if err != nil {
		log.Fatalf("ES connect: %v", err)
	}

	registry := manager.NewRegistry()

	// Track plants from NATS status messages
	nc.Subscribe("plant.*.status", func(msg *nats.Msg) {
		var info models.PlantInfo
		if err := json.Unmarshal(msg.Data, &info); err != nil {
			return
		}
		if info.PlantID != "" && info.PlantName != "" {
			registry.Add(info.PlantID, info.PlantName, info.Panels, info.WattPerSec)
			log.Printf("Plant registered via NATS: %s (%s)", info.PlantName, info.PlantID)
		}
	})

	mux := http.NewServeMux()

	// List plants
	mux.HandleFunc("GET /api/plants", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(registry.List())
	})

	// Trigger fault via NATS
	mux.HandleFunc("POST /api/plants/{plantId}/panels/{panelId}/fault", func(w http.ResponseWriter, r *http.Request) {
		plantID := r.PathValue("plantId")
		panelID := r.PathValue("panelId")

		var req struct {
			Mode string `json:"mode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		mode := strings.ToUpper(req.Mode)
		cmdType := models.CmdFault
		if mode == "RESET" {
			cmdType = models.CmdReset
		}
		cmd := models.Command{
			Command:   cmdType,
			PanelID:   panelID,
			FaultMode: mode,
		}
		cmdBytes, _ := json.Marshal(cmd)
		nc.Publish(fmt.Sprintf("plant.%s.command", plantID), cmdBytes)

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "command sent"})
	})

	// History from ES
	mux.HandleFunc("GET /api/plants/{plantId}/history", func(w http.ResponseWriter, r *http.Request) {
		plantID := r.PathValue("plantId")
		rangeParam := r.URL.Query().Get("range")
		if rangeParam == "" {
			rangeParam = "5m"
		}
		interval := r.URL.Query().Get("interval")
		if interval == "" {
			interval = "1s"
		}

		// Query plant-panel-* directly with sum(watt). 5-second watermark
		// (lt: now-5s) hides partial Fluentd-buffered data near "now".
		query := map[string]interface{}{
			"size": 0,
			"query": map[string]interface{}{
				"bool": map[string]interface{}{
					"filter": []map[string]interface{}{
						{"term": map[string]interface{}{"plantId": plantID}},
						{"range": map[string]interface{}{
							"@timestamp": map[string]interface{}{
								"gte": "now-" + rangeParam + "-5s",
								"lt":  "now-5s",
							},
						}},
					},
				},
			},
			"aggs": map[string]interface{}{
				"over_time": map[string]interface{}{
					"date_histogram": map[string]interface{}{
						"field":          "@timestamp",
						"fixed_interval": interval,
						"min_doc_count":  1,
					},
					"aggs": map[string]interface{}{
						"total_watt": map[string]interface{}{
							"sum": map[string]interface{}{"field": "watt"},
						},
					},
				},
			},
		}

		var buf bytes.Buffer
		json.NewEncoder(&buf).Encode(query)

		res, err := es.Search(
			es.Search.WithContext(context.Background()),
			es.Search.WithIndex("plant-panel-*"),
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

		// Query plant-panel-* directly with sum(watt). 5-second watermark
		// (lt: now-5s) hides partial Fluentd-buffered data near "now".
		query := map[string]interface{}{
			"size": 0,
			"query": map[string]interface{}{
				"range": map[string]interface{}{
					"@timestamp": map[string]interface{}{
						"gte": "now-" + rangeParam + "-5s",
						"lt":  "now-5s",
					},
				},
			},
			"aggs": map[string]interface{}{
				"over_time": map[string]interface{}{
					"date_histogram": map[string]interface{}{
						"field":          "@timestamp",
						"fixed_interval": interval,
						"min_doc_count":  1,
					},
					"aggs": map[string]interface{}{
						"total_watt": map[string]interface{}{
							"sum": map[string]interface{}{"field": "watt"},
						},
					},
				},
			},
		}

		var buf bytes.Buffer
		json.NewEncoder(&buf).Encode(query)

		res, err := es.Search(
			es.Search.WithContext(context.Background()),
			es.Search.WithIndex("plant-panel-*"),
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

	// Current state per plant (for dashboard polling).
	// Queries plant-panel-* with terms+top_hits to get each panel's latest
	// reading, then reduces to per-plant summaries in Go.
	mux.HandleFunc("GET /api/plants/current", func(w http.ResponseWriter, r *http.Request) {
		query := map[string]interface{}{
			"size": 0,
			"query": map[string]interface{}{
				"range": map[string]interface{}{
					"@timestamp": map[string]interface{}{"gte": "now-30s"},
				},
			},
			"aggs": map[string]interface{}{
				"by_plant": map[string]interface{}{
					"terms": map[string]interface{}{
						"field": "plantId",
						"size":  100,
					},
					"aggs": map[string]interface{}{
						"by_panel": map[string]interface{}{
							"terms": map[string]interface{}{
								"field": "panelId",
								"size":  200,
							},
							"aggs": map[string]interface{}{
								"latest": map[string]interface{}{
									"top_hits": map[string]interface{}{
										"size": 1,
										"sort": []map[string]interface{}{
											{"@timestamp": "desc"},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		var buf bytes.Buffer
		json.NewEncoder(&buf).Encode(query)

		res, err := es.Search(
			es.Search.WithContext(context.Background()),
			es.Search.WithIndex("plant-panel-*"),
			es.Search.WithBody(&buf),
		)
		if err != nil {
			http.Error(w, "ES query failed", http.StatusInternalServerError)
			return
		}
		defer res.Body.Close()

		// Parse the ES response and reduce to per-plant summaries.
		var esResp struct {
			Aggregations struct {
				ByPlant struct {
					Buckets []struct {
						Key     string `json:"key"`
						ByPanel struct {
							Buckets []struct {
								Latest struct {
									Hits struct {
										Hits []struct {
											Source models.PanelReading `json:"_source"`
										} `json:"hits"`
									} `json:"hits"`
								} `json:"latest"`
							} `json:"buckets"`
						} `json:"by_panel"`
					} `json:"buckets"`
				} `json:"by_plant"`
			} `json:"aggregations"`
		}
		if err := json.NewDecoder(res.Body).Decode(&esResp); err != nil {
			http.Error(w, "ES decode failed", http.StatusInternalServerError)
			return
		}

		summaries := make([]models.PlantSummary, 0, len(esResp.Aggregations.ByPlant.Buckets))
		for _, plantBucket := range esResp.Aggregations.ByPlant.Buckets {
			summary := models.PlantSummary{
				PlantID:    plantBucket.Key,
				PanelCount: len(plantBucket.ByPanel.Buckets),
			}
			var latestTimestamp time.Time
			for _, panelBucket := range plantBucket.ByPanel.Buckets {
				if len(panelBucket.Latest.Hits.Hits) == 0 {
					continue
				}
				reading := panelBucket.Latest.Hits.Hits[0].Source
				if summary.PlantName == "" {
					summary.PlantName = reading.PlantName
				}
				if reading.Timestamp.After(latestTimestamp) {
					latestTimestamp = reading.Timestamp
				}
				summary.TotalWatt += reading.Watt
				switch reading.Status {
				case models.StatusOnline:
					summary.OnlineCount++
				case models.StatusOffline:
					summary.OfflineCount++
				}
				if reading.FaultMode != models.FaultNone {
					summary.FaultyCount++
				}
			}
			summary.Timestamp = latestTimestamp
			summaries = append(summaries, summary)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"plants": summaries})
	})

	// Latest panel readings for a plant (for detail view polling)
	mux.HandleFunc("GET /api/plants/{plantId}/panels", func(w http.ResponseWriter, r *http.Request) {
		plantID := r.PathValue("plantId")

		query := map[string]interface{}{
			"size": 0,
			"query": map[string]interface{}{
				"bool": map[string]interface{}{
					"filter": []map[string]interface{}{
						{"term": map[string]interface{}{"plantId": plantID}},
						{"range": map[string]interface{}{
							"@timestamp": map[string]interface{}{"gte": "now-10s"},
						}},
					},
				},
			},
			"aggs": map[string]interface{}{
				"by_panel": map[string]interface{}{
					"terms": map[string]interface{}{
						"field": "panelId",
						"size":  100,
					},
					"aggs": map[string]interface{}{
						"latest": map[string]interface{}{
							"top_hits": map[string]interface{}{
								"size": 1,
								"sort": []map[string]interface{}{
									{"@timestamp": "desc"},
								},
							},
						},
					},
				},
			},
		}

		var buf bytes.Buffer
		json.NewEncoder(&buf).Encode(query)

		res, err := es.Search(
			es.Search.WithContext(context.Background()),
			es.Search.WithIndex("plant-panel-*"),
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

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	go func() {
		log.Printf("Plant Manager listening on %s", addr)
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Fatalf("HTTP server: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Println("Shutting down...")
}
