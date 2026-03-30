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

type plantBucket struct {
	Key      string `json:"key"`
	SumWatt  struct{ Value float64 } `json:"sum_watt"`
	PlantName struct {
		Buckets []struct{ Key string } `json:"buckets"`
	} `json:"plant_name"`
	PanelCount struct{ Value int } `json:"panel_count"`
	StatusCounts struct {
		Buckets []struct {
			Key      string `json:"key"`
			DocCount int    `json:"doc_count"`
		} `json:"buckets"`
	} `json:"status_counts"`
	FaultyCount struct{ DocCount int } `json:"faulty_count"`
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
	query := map[string]interface{}{
		"size": 0,
		"query": map[string]interface{}{
			"range": map[string]interface{}{
				"timestamp": map[string]interface{}{"gte": "now-10s"},
			},
		},
		"aggs": map[string]interface{}{
			"by_plant": map[string]interface{}{
				"terms": map[string]interface{}{
					"field": "plantId",
					"size":  100,
				},
				"aggs": map[string]interface{}{
					"sum_watt": map[string]interface{}{
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
					"status_counts": map[string]interface{}{
						"terms": map[string]interface{}{
							"field": "status",
							"size":  10,
						},
					},
					"faulty_count": map[string]interface{}{
						"filter": map[string]interface{}{
							"exists": map[string]interface{}{"field": "faultMode"},
						},
					},
				},
			},
		},
	}

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

	now := time.Now().UTC()
	indexName := fmt.Sprintf("plant-summary-%s", now.Format("2006-01-02"))
	written := 0

	for _, raw := range result.Aggregations.ByPlant.Buckets {
		var bucket plantBucket
		if err := json.Unmarshal(raw, &bucket); err != nil {
			continue
		}

		plantName := ""
		if len(bucket.PlantName.Buckets) > 0 {
			plantName = bucket.PlantName.Buckets[0].Key
		}

		online, offline := 0, 0
		for _, s := range bucket.StatusCounts.Buckets {
			switch s.Key {
			case "online":
				online = s.DocCount
			case "offline":
				offline = s.DocCount
			}
		}

		summary := map[string]interface{}{
			"plantId":      bucket.Key,
			"plantName":    plantName,
			"timestamp":    now.Format(time.RFC3339Nano),
			"totalWatt":    bucket.SumWatt.Value,
			"panelCount":   bucket.PanelCount.Value,
			"onlineCount":  online,
			"offlineCount": offline,
			"faultyCount":  bucket.FaultyCount.DocCount,
		}

		var docBuf bytes.Buffer
		json.NewEncoder(&docBuf).Encode(summary)

		indexRes, err := es.Index(indexName, &docBuf,
			es.Index.WithContext(context.Background()),
		)
		if err != nil {
			log.Printf("ES index error for plant %s: %v", bucket.Key, err)
			continue
		}
		indexRes.Body.Close()
		written++
	}

	if written > 0 {
		log.Printf("Aggregated %d plant summaries → %s", written, indexName)
	}
}
