package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
)

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

type secondBucket struct {
	KeyAsString string                  `json:"key_as_string"`
	TotalWatt   struct{ Value float64 } `json:"total_watt"`
	PlantName   struct {
		Buckets []struct{ Key string } `json:"buckets"`
	} `json:"plant_name"`
	PanelCount   struct{ Value int } `json:"panel_count"`
	OnlinePanels struct {
		Count struct{ Value int } `json:"count"`
	} `json:"online_panels"`
	OfflinePanels struct {
		Count struct{ Value int } `json:"count"`
	} `json:"offline_panels"`
	FaultyCount struct {
		Count struct{ Value int } `json:"count"`
	} `json:"faulty_count"`
}

type plantBucket struct {
	Key       string `json:"key"`
	PerSecond struct {
		Buckets []secondBucket `json:"buckets"`
	} `json:"per_second"`
}

type plantSummary struct {
	PlantID      string  `json:"plantId"`
	PlantName    string  `json:"plantName"`
	Timestamp    string  `json:"@timestamp"`
	TimestampAlt string  `json:"timestamp"`
	TotalWatt    float64 `json:"totalWatt"`
	PanelCount   int     `json:"panelCount"`
	OnlineCount  int     `json:"onlineCount"`
	OfflineCount int     `json:"offlineCount"`
	FaultyCount  int     `json:"faultyCount"`
}

func parseBuckets(raw []json.RawMessage) []plantSummary {
	var summaries []plantSummary
	for _, r := range raw {
		var bucket plantBucket
		if err := json.Unmarshal(r, &bucket); err != nil {
			continue
		}

		for _, sb := range bucket.PerSecond.Buckets {
			plantName := ""
			if len(sb.PlantName.Buckets) > 0 {
				plantName = sb.PlantName.Buckets[0].Key
			}

			summaries = append(summaries, plantSummary{
				PlantID:      bucket.Key,
				PlantName:    plantName,
				Timestamp:    sb.KeyAsString,
				TimestampAlt: sb.KeyAsString,
				TotalWatt:    sb.TotalWatt.Value,
				PanelCount:   sb.PanelCount.Value,
				OnlineCount:  sb.OnlinePanels.Count.Value,
				OfflineCount: sb.OfflinePanels.Count.Value,
				FaultyCount:  sb.FaultyCount.Count.Value,
			})
		}
	}
	return summaries
}

func buildQuery() map[string]interface{} {
	return map[string]interface{}{
		"size": 0,
		// Rolling window: query 40s of data every 10s cycle = 30s overlap.
		// Each second is processed by 4 consecutive cycles, giving late-arriving
		// panel data (Fluentd flush delay) multiple chances to be included.
		// Combined with deterministic doc IDs below, re-aggregating the same
		// second is idempotent (UPSERT, not double-count).
		"query": map[string]interface{}{
			"range": map[string]interface{}{
				"@timestamp": map[string]interface{}{
					"gte": "now-45s",
					"lt":  "now-5s",
				},
			},
		},
		"aggs": map[string]interface{}{
			"by_plant": map[string]interface{}{
				"terms": map[string]interface{}{
					"field": "plantId",
					"size":  100,
				},
				"aggs": map[string]interface{}{
					"per_second": map[string]interface{}{
						"date_histogram": map[string]interface{}{
							"field":          "@timestamp",
							"fixed_interval": "1s",
							"min_doc_count":  1,
						},
						"aggs": map[string]interface{}{
							"total_watt": map[string]interface{}{
								"sum": map[string]interface{}{"field": "watt"},
							},
							"plant_name": map[string]interface{}{
								"terms": map[string]interface{}{
									"field": "plantName",
									"size":  1,
								},
							},
							"panel_count": map[string]interface{}{
								"cardinality": map[string]interface{}{"field": "panelId"},
							},
							"online_panels": map[string]interface{}{
								"filter": map[string]interface{}{
									"term": map[string]interface{}{"status": "online"},
								},
								"aggs": map[string]interface{}{
									"count": map[string]interface{}{
										"cardinality": map[string]interface{}{"field": "panelId"},
									},
								},
							},
							"offline_panels": map[string]interface{}{
								"filter": map[string]interface{}{
									"term": map[string]interface{}{"status": "offline"},
								},
								"aggs": map[string]interface{}{
									"count": map[string]interface{}{
										"cardinality": map[string]interface{}{"field": "panelId"},
									},
								},
							},
							// faulty_count uses cardinality of panelId to count distinct faulty
							// panels per second (not document count, which would over-count panels
							// emitting multiple readings within the 1s bucket).
							"faulty_count": map[string]interface{}{
								"filter": map[string]interface{}{
									"exists": map[string]interface{}{"field": "faultMode"},
								},
								"aggs": map[string]interface{}{
									"count": map[string]interface{}{
										"cardinality": map[string]interface{}{"field": "panelId"},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func main() {
	esURL := envOrDefault("ES_URL", "http://localhost:9200")
	interval := 10 * time.Second

	es, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{esURL},
	})
	if err != nil {
		log.Fatalf("ES connect: %v", err)
	}
	log.Printf("Aggregator started (ES: %s, interval: %s)", esURL, interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-ticker.C:
			aggregate(es)
		case <-sigCh:
			log.Println("Shutting down...")
			return
		}
	}
}

func aggregate(es *elasticsearch.Client) {
	query := buildQuery()

	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(query)

	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Second)
	defer cancel()

	res, err := es.Search(
		es.Search.WithContext(ctx),
		es.Search.WithIndex("plant-panel-*"),
		es.Search.WithBody(&buf),
	)
	if err != nil {
		log.Printf("ES query error: %v", err)
		return
	}
	defer res.Body.Close()

	if res.IsError() {
		body, _ := io.ReadAll(res.Body)
		log.Printf("ES error: %s", body)
		return
	}

	var result struct {
		Aggregations struct {
			ByPlant struct {
				Buckets []json.RawMessage `json:"buckets"`
			} `json:"by_plant"`
		} `json:"aggregations"`
	}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		log.Printf("ES decode error: %v", err)
		return
	}

	indexName := fmt.Sprintf("plant-summary-%s", time.Now().UTC().Format("2006-01-02"))
	summaries := parseBuckets(result.Aggregations.ByPlant.Buckets)

	written := 0
	for _, s := range summaries {
		var docBuf bytes.Buffer
		json.NewEncoder(&docBuf).Encode(s)

		// Deterministic doc ID per (plant, second). UPSERT semantics: re-processing
		// the same second by the next cycle replaces the previous doc instead of
		// creating a duplicate. This is the "idempotent sink" pattern.
		t, err := time.Parse(time.RFC3339, s.Timestamp)
		if err != nil {
			log.Printf("invalid timestamp %q for plant %s: %v", s.Timestamp, s.PlantID, err)
			continue
		}
		docID := fmt.Sprintf("%s_%d", s.PlantID, t.Unix())

		indexRes, err := es.Index(indexName, &docBuf,
			es.Index.WithDocumentID(docID),
			es.Index.WithContext(context.Background()),
		)
		if err != nil {
			log.Printf("ES index error for plant %s: %v", s.PlantID, err)
			continue
		}
		indexRes.Body.Close()
		written++
	}

	if written > 0 {
		log.Printf("Aggregated %d plant summaries → %s", written, indexName)
	}
}
