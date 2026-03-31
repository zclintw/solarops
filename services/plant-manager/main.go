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
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	dockerclient "github.com/moby/moby/client"
	"github.com/moby/moby/api/types/container"
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
	mockPlantImage := envOrDefault("MOCK_PLANT_IMAGE", "solarops-mock-plant")
	networkName := envOrDefault("DOCKER_NETWORK", "solarops_default")

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

	// Connect to Docker
	docker, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalf("Docker connect: %v", err)
	}
	defer docker.Close()

	registry := manager.NewRegistry()

	// Track plants from NATS status messages
	nc.Subscribe("plant.*.status", func(msg *nats.Msg) {
		var info models.PlantInfo
		if err := json.Unmarshal(msg.Data, &info); err != nil {
			return
		}
		if info.PlantID != "" && info.PlantName != "" {
			registry.Add(info.PlantID, info.PlantName, info.Panels, info.WattPerSec, "")
			log.Printf("Plant registered via NATS: %s (%s)", info.PlantName, info.PlantID)
		}
	})

	mux := http.NewServeMux()

	// List plants
	mux.HandleFunc("GET /api/plants", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(registry.List())
	})

	// Create plant
	mux.HandleFunc("POST /api/plants", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name       string  `json:"name"`
			Panels     int     `json:"panels"`
			WattPerSec float64 `json:"wattPerSec"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		if registry.NameExists(req.Name) {
			http.Error(w, "plant name already exists", http.StatusConflict)
			return
		}

		// Start new container
		ctx := context.Background()
		result, err := docker.ContainerCreate(ctx, dockerclient.ContainerCreateOptions{
			Image: mockPlantImage,
			Config: &container.Config{
				Env: []string{
					"PLANT_NAME=" + req.Name,
					"PLANT_PANELS=" + strconv.Itoa(req.Panels),
					"WATT_PER_SEC=" + fmt.Sprintf("%.0f", req.WattPerSec),
					"NATS_URL=" + natsURL,
					"LOG_PATH=/var/log/plant/data.log",
				},
			},
			HostConfig: &container.HostConfig{},
			Name:       "solarops-plant-" + req.Name,
		})
		if err != nil {
			log.Printf("Container create error: %v", err)
			http.Error(w, "failed to create plant container", http.StatusInternalServerError)
			return
		}

		// Connect to network
		docker.NetworkConnect(ctx, networkName, dockerclient.NetworkConnectOptions{
			Container: result.ID,
		})

		if _, err := docker.ContainerStart(ctx, result.ID, dockerclient.ContainerStartOptions{}); err != nil {
			log.Printf("Container start error: %v", err)
			http.Error(w, "failed to start plant container", http.StatusInternalServerError)
			return
		}

		log.Printf("Started new plant container: %s (%s)", req.Name, result.ID[:12])

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"containerId": result.ID[:12],
			"name":        req.Name,
			"status":      "starting",
		})
	})

	// Delete plant
	mux.HandleFunc("DELETE /api/plants/{plantId}", func(w http.ResponseWriter, r *http.Request) {
		plantID := r.PathValue("plantId")
		entry, ok := registry.Remove(plantID)
		if !ok {
			http.Error(w, "plant not found", http.StatusNotFound)
			return
		}

		if entry.ContainerID != "" {
			ctx := context.Background()
			timeout := 10
			docker.ContainerStop(ctx, entry.ContainerID, dockerclient.ContainerStopOptions{Timeout: &timeout})
			docker.ContainerRemove(ctx, entry.ContainerID, dockerclient.ContainerRemoveOptions{})
			log.Printf("Removed plant container: %s", entry.PlantName)
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "removed"})
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
			rangeParam = "1h"
		}
		interval := r.URL.Query().Get("interval")
		if interval == "" {
			interval = "10s"
		}

		query := map[string]interface{}{
			"size": 0,
			"query": map[string]interface{}{
				"bool": map[string]interface{}{
					"filter": []map[string]interface{}{
						{"term": map[string]interface{}{"plantId": plantID}},
						{"range": map[string]interface{}{
							"@timestamp": map[string]interface{}{"gte": "now-" + rangeParam},
						}},
					},
				},
			},
			"aggs": map[string]interface{}{
				"over_time": map[string]interface{}{
					"date_histogram": map[string]interface{}{
						"field":          "@timestamp",
						"fixed_interval": interval,
					},
					"aggs": map[string]interface{}{
						"total_watt": map[string]interface{}{
							"avg": map[string]interface{}{"field": "totalWatt"},
						},
					},
				},
			},
		}

		var buf bytes.Buffer
		json.NewEncoder(&buf).Encode(query)

		res, err := es.Search(
			es.Search.WithContext(context.Background()),
			es.Search.WithIndex("plant-summary-*"),
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

	// Latest summary per plant (for dashboard polling)
	mux.HandleFunc("GET /api/plants/summary", func(w http.ResponseWriter, r *http.Request) {
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
			es.Search.WithIndex("plant-summary-*"),
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
