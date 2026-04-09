package main

import (
	"encoding/json"
	"testing"
)

func TestParseBuckets_NormalPlant(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage(`{
			"key": "plant-001",
			"per_second": {
				"buckets": [
					{
						"key_as_string": "2026-04-01T12:00:05.000Z",
						"total_watt": {"value": 15000},
						"plant_name": {"buckets": [{"key": "Sunrise Valley"}]},
						"panel_count": {"value": 5},
						"online_panels": {"doc_count": 10, "count": {"value": 5}},
						"offline_panels": {"doc_count": 0, "count": {"value": 0}},
						"faulty_count": {"doc_count": 0, "count": {"value": 0}}
					}
				]
			}
		}`),
	}

	summaries := parseBuckets(raw)

	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}

	s := summaries[0]
	if s.PlantID != "plant-001" {
		t.Errorf("expected plant-001, got %s", s.PlantID)
	}
	if s.PlantName != "Sunrise Valley" {
		t.Errorf("expected Sunrise Valley, got %s", s.PlantName)
	}
	if s.TotalWatt != 15000 {
		t.Errorf("expected totalWatt 15000, got %f", s.TotalWatt)
	}
	if s.PanelCount != 5 {
		t.Errorf("expected panelCount 5, got %d", s.PanelCount)
	}
	if s.OnlineCount != 5 {
		t.Errorf("expected onlineCount 5, got %d", s.OnlineCount)
	}
	if s.Timestamp != "2026-04-01T12:00:05.000Z" {
		t.Errorf("expected timestamp from bucket key, got %s", s.Timestamp)
	}
}

func TestParseBuckets_MultipleSeconds(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage(`{
			"key": "plant-001",
			"per_second": {
				"buckets": [
					{
						"key_as_string": "2026-04-01T12:00:05.000Z",
						"total_watt": {"value": 15000},
						"plant_name": {"buckets": [{"key": "Sunrise Valley"}]},
						"panel_count": {"value": 5},
						"online_panels": {"doc_count": 10, "count": {"value": 5}},
						"offline_panels": {"doc_count": 0, "count": {"value": 0}},
						"faulty_count": {"doc_count": 0, "count": {"value": 0}}
					},
					{
						"key_as_string": "2026-04-01T12:00:06.000Z",
						"total_watt": {"value": 14500},
						"plant_name": {"buckets": [{"key": "Sunrise Valley"}]},
						"panel_count": {"value": 5},
						"online_panels": {"doc_count": 10, "count": {"value": 5}},
						"offline_panels": {"doc_count": 0, "count": {"value": 0}},
						"faulty_count": {"doc_count": 0, "count": {"value": 0}}
					}
				]
			}
		}`),
	}

	summaries := parseBuckets(raw)

	if len(summaries) != 2 {
		t.Fatalf("expected 2 summaries (one per second), got %d", len(summaries))
	}
	if summaries[0].TotalWatt != 15000 {
		t.Errorf("expected first second 15000, got %f", summaries[0].TotalWatt)
	}
	if summaries[1].TotalWatt != 14500 {
		t.Errorf("expected second second 14500, got %f", summaries[1].TotalWatt)
	}
	if summaries[0].Timestamp != "2026-04-01T12:00:05.000Z" {
		t.Errorf("expected bucket timestamp, got %s", summaries[0].Timestamp)
	}
	if summaries[1].Timestamp != "2026-04-01T12:00:06.000Z" {
		t.Errorf("expected bucket timestamp, got %s", summaries[1].Timestamp)
	}
}

func TestParseBuckets_MultiplePlants(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage(`{
			"key": "plant-001",
			"per_second": {
				"buckets": [{
					"key_as_string": "2026-04-01T12:00:05.000Z",
					"total_watt": {"value": 15000},
					"plant_name": {"buckets": [{"key": "Sunrise Valley"}]},
					"panel_count": {"value": 5},
					"online_panels": {"doc_count": 10, "count": {"value": 5}},
					"offline_panels": {"doc_count": 0, "count": {"value": 0}},
					"faulty_count": {"doc_count": 0, "count": {"value": 0}}
				}]
			}
		}`),
		json.RawMessage(`{
			"key": "plant-002",
			"per_second": {
				"buckets": [{
					"key_as_string": "2026-04-01T12:00:05.000Z",
					"total_watt": {"value": 16800},
					"plant_name": {"buckets": [{"key": "Blue Horizon"}]},
					"panel_count": {"value": 6},
					"online_panels": {"doc_count": 12, "count": {"value": 6}},
					"offline_panels": {"doc_count": 0, "count": {"value": 0}},
					"faulty_count": {"doc_count": 3, "count": {"value": 1}}
				}]
			}
		}`),
	}

	summaries := parseBuckets(raw)

	if len(summaries) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(summaries))
	}

	total := summaries[0].TotalWatt + summaries[1].TotalWatt
	if total != 31800 {
		t.Errorf("expected combined totalWatt 31800, got %f", total)
	}
	if summaries[1].FaultyCount != 1 {
		t.Errorf("expected faultyCount 1 for plant-002, got %d", summaries[1].FaultyCount)
	}
}

func TestParseBuckets_WithOfflinePanels(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage(`{
			"key": "plant-003",
			"per_second": {
				"buckets": [{
					"key_as_string": "2026-04-01T12:00:05.000Z",
					"total_watt": {"value": 6000},
					"plant_name": {"buckets": [{"key": "Golden Ridge"}]},
					"panel_count": {"value": 4},
					"online_panels": {"doc_count": 6, "count": {"value": 2}},
					"offline_panels": {"doc_count": 4, "count": {"value": 2}},
					"faulty_count": {"doc_count": 0, "count": {"value": 0}}
				}]
			}
		}`),
	}

	summaries := parseBuckets(raw)
	s := summaries[0]

	if s.OnlineCount != 2 {
		t.Errorf("expected onlineCount 2, got %d", s.OnlineCount)
	}
	if s.OfflineCount != 2 {
		t.Errorf("expected offlineCount 2, got %d", s.OfflineCount)
	}
	if s.TotalWatt != 6000 {
		t.Errorf("expected totalWatt 6000, got %f", s.TotalWatt)
	}
}

func TestParseBuckets_MissingPlantName(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage(`{
			"key": "plant-orphan",
			"per_second": {
				"buckets": [{
					"key_as_string": "2026-04-01T12:00:05.000Z",
					"total_watt": {"value": 1000},
					"plant_name": {"buckets": []},
					"panel_count": {"value": 1},
					"online_panels": {"doc_count": 1, "count": {"value": 1}},
					"offline_panels": {"doc_count": 0, "count": {"value": 0}},
					"faulty_count": {"doc_count": 0, "count": {"value": 0}}
				}]
			}
		}`),
	}

	summaries := parseBuckets(raw)
	if summaries[0].PlantName != "" {
		t.Errorf("expected empty plantName, got %s", summaries[0].PlantName)
	}
}

func TestParseBuckets_EmptyInput(t *testing.T) {
	summaries := parseBuckets(nil)
	if len(summaries) != 0 {
		t.Errorf("expected 0 summaries for nil input, got %d", len(summaries))
	}

	summaries = parseBuckets([]json.RawMessage{})
	if len(summaries) != 0 {
		t.Errorf("expected 0 summaries for empty input, got %d", len(summaries))
	}
}

func TestParseBuckets_InvalidJSON(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage(`{invalid json}`),
		json.RawMessage(`{
			"key": "plant-ok",
			"per_second": {
				"buckets": [{
					"key_as_string": "2026-04-01T12:00:05.000Z",
					"total_watt": {"value": 5000},
					"plant_name": {"buckets": [{"key": "Valid Plant"}]},
					"panel_count": {"value": 2},
					"online_panels": {"doc_count": 2, "count": {"value": 2}},
					"offline_panels": {"doc_count": 0, "count": {"value": 0}},
					"faulty_count": {"doc_count": 0, "count": {"value": 0}}
				}]
			}
		}`),
	}

	summaries := parseBuckets(raw)

	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary (skip invalid), got %d", len(summaries))
	}
	if summaries[0].PlantID != "plant-ok" {
		t.Errorf("expected plant-ok, got %s", summaries[0].PlantID)
	}
}

func TestParseBuckets_EmptyPerSecondBuckets(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage(`{
			"key": "plant-empty",
			"per_second": {
				"buckets": []
			}
		}`),
	}

	summaries := parseBuckets(raw)
	if len(summaries) != 0 {
		t.Errorf("expected 0 summaries for plant with no per_second buckets, got %d", len(summaries))
	}
}

func TestBuildQuery_Structure(t *testing.T) {
	query := buildQuery()

	if query["size"] != 0 {
		t.Error("expected size 0")
	}

	q, ok := query["query"].(map[string]interface{})
	if !ok {
		t.Fatal("expected query")
	}
	rangeFilter, ok := q["range"].(map[string]interface{})
	if !ok {
		t.Fatal("expected range filter")
	}
	ts, ok := rangeFilter["@timestamp"].(map[string]interface{})
	if !ok {
		t.Fatal("expected @timestamp range")
	}
	if ts["gte"] != "now-15s" {
		t.Errorf("expected gte now-15s, got %v", ts["gte"])
	}
	if ts["lt"] != "now-5s" {
		t.Errorf("expected lt now-5s, got %v", ts["lt"])
	}

	aggs, ok := query["aggs"].(map[string]interface{})
	if !ok {
		t.Fatal("expected aggs")
	}

	byPlant, ok := aggs["by_plant"].(map[string]interface{})
	if !ok {
		t.Fatal("expected by_plant aggregation")
	}

	subAggs, ok := byPlant["aggs"].(map[string]interface{})
	if !ok {
		t.Fatal("expected sub-aggregations under by_plant")
	}

	perSecond, ok := subAggs["per_second"].(map[string]interface{})
	if !ok {
		t.Fatal("expected per_second sub-aggregation")
	}
	dh, ok := perSecond["date_histogram"].(map[string]interface{})
	if !ok {
		t.Fatal("expected date_histogram in per_second")
	}
	if dh["fixed_interval"] != "1s" {
		t.Errorf("expected fixed_interval 1s, got %v", dh["fixed_interval"])
	}
	if dh["min_doc_count"] != 1 {
		t.Errorf("expected min_doc_count 1, got %v", dh["min_doc_count"])
	}

	perSecondAggs, ok := perSecond["aggs"].(map[string]interface{})
	if !ok {
		t.Fatal("expected aggs inside per_second")
	}
	totalWatt, ok := perSecondAggs["total_watt"].(map[string]interface{})
	if !ok {
		t.Fatal("expected total_watt in per_second aggs")
	}
	if _, ok := totalWatt["sum"]; !ok {
		t.Error("expected total_watt to use sum aggregation")
	}
}
