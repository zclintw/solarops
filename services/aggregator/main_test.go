package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseBuckets_NormalPlant(t *testing.T) {
	// Simulates ES response for Sunrise Valley: 5 panels × 3000W = 15000W
	raw := []json.RawMessage{
		json.RawMessage(`{
			"key": "plant-001",
			"total_watt": {"value": 15000},
			"plant_name": {"buckets": [{"key": "Sunrise Valley"}]},
			"panel_count": {"value": 5},
			"online_panels": {"doc_count": 10, "count": {"value": 5}},
			"offline_panels": {"doc_count": 0, "count": {"value": 0}},
			"faulty_count": {"doc_count": 0}
		}`),
	}

	now := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	summaries := parseBuckets(raw, now)

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
	if s.OfflineCount != 0 {
		t.Errorf("expected offlineCount 0, got %d", s.OfflineCount)
	}
	if s.FaultyCount != 0 {
		t.Errorf("expected faultyCount 0, got %d", s.FaultyCount)
	}
}

func TestParseBuckets_MultiplePlants(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage(`{
			"key": "plant-001",
			"total_watt": {"value": 15000},
			"plant_name": {"buckets": [{"key": "Sunrise Valley"}]},
			"panel_count": {"value": 5},
			"online_panels": {"doc_count": 10, "count": {"value": 5}},
			"offline_panels": {"doc_count": 0, "count": {"value": 0}},
			"faulty_count": {"doc_count": 0}
		}`),
		json.RawMessage(`{
			"key": "plant-002",
			"total_watt": {"value": 16800},
			"plant_name": {"buckets": [{"key": "Blue Horizon"}]},
			"panel_count": {"value": 6},
			"online_panels": {"doc_count": 12, "count": {"value": 6}},
			"offline_panels": {"doc_count": 0, "count": {"value": 0}},
			"faulty_count": {"doc_count": 1}
		}`),
	}

	summaries := parseBuckets(raw, time.Now().UTC())

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
			"total_watt": {"value": 6000},
			"plant_name": {"buckets": [{"key": "Golden Ridge"}]},
			"panel_count": {"value": 4},
			"online_panels": {"doc_count": 6, "count": {"value": 2}},
			"offline_panels": {"doc_count": 4, "count": {"value": 2}},
			"faulty_count": {"doc_count": 0}
		}`),
	}

	summaries := parseBuckets(raw, time.Now().UTC())
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
			"total_watt": {"value": 1000},
			"plant_name": {"buckets": []},
			"panel_count": {"value": 1},
			"online_panels": {"doc_count": 1, "count": {"value": 1}},
			"offline_panels": {"doc_count": 0, "count": {"value": 0}},
			"faulty_count": {"doc_count": 0}
		}`),
	}

	summaries := parseBuckets(raw, time.Now().UTC())

	if summaries[0].PlantName != "" {
		t.Errorf("expected empty plantName, got %s", summaries[0].PlantName)
	}
}

func TestParseBuckets_EmptyInput(t *testing.T) {
	summaries := parseBuckets(nil, time.Now().UTC())
	if len(summaries) != 0 {
		t.Errorf("expected 0 summaries for nil input, got %d", len(summaries))
	}

	summaries = parseBuckets([]json.RawMessage{}, time.Now().UTC())
	if len(summaries) != 0 {
		t.Errorf("expected 0 summaries for empty input, got %d", len(summaries))
	}
}

func TestParseBuckets_InvalidJSON(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage(`{invalid json}`),
		json.RawMessage(`{
			"key": "plant-ok",
			"total_watt": {"value": 5000},
			"plant_name": {"buckets": [{"key": "Valid Plant"}]},
			"panel_count": {"value": 2},
			"online_panels": {"doc_count": 2, "count": {"value": 2}},
			"offline_panels": {"doc_count": 0, "count": {"value": 0}},
			"faulty_count": {"doc_count": 0}
		}`),
	}

	summaries := parseBuckets(raw, time.Now().UTC())

	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary (skip invalid), got %d", len(summaries))
	}
	if summaries[0].PlantID != "plant-ok" {
		t.Errorf("expected plant-ok, got %s", summaries[0].PlantID)
	}
}

func TestParseBuckets_TimestampFormat(t *testing.T) {
	now := time.Date(2026, 4, 1, 12, 30, 45, 0, time.UTC)
	raw := []json.RawMessage{
		json.RawMessage(`{
			"key": "plant-ts",
			"total_watt": {"value": 1000},
			"plant_name": {"buckets": [{"key": "Test"}]},
			"panel_count": {"value": 1},
			"online_panels": {"doc_count": 1, "count": {"value": 1}},
			"offline_panels": {"doc_count": 0, "count": {"value": 0}},
			"faulty_count": {"doc_count": 0}
		}`),
	}

	summaries := parseBuckets(raw, now)

	if summaries[0].Timestamp != summaries[0].TimestampAlt {
		t.Errorf("@timestamp and timestamp should be equal")
	}
	if summaries[0].Timestamp == "" {
		t.Error("timestamp should not be empty")
	}
}

func TestBuildQuery_Structure(t *testing.T) {
	query := buildQuery()

	if query["size"] != 0 {
		t.Error("expected size 0")
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

	// Verify the fix: should have by_panel > avg_watt > sum_bucket pipeline, not direct sum
	if _, ok := subAggs["by_panel"]; !ok {
		t.Error("expected by_panel sub-aggregation (panels should be grouped before averaging)")
	}
	if _, ok := subAggs["total_watt"]; !ok {
		t.Error("expected total_watt pipeline aggregation")
	}

	totalWatt, ok := subAggs["total_watt"].(map[string]interface{})
	if !ok {
		t.Fatal("expected total_watt to be a map")
	}
	sumBucket, ok := totalWatt["sum_bucket"].(map[string]interface{})
	if !ok {
		t.Fatal("expected sum_bucket in total_watt")
	}
	if sumBucket["buckets_path"] != "by_panel>avg_watt" {
		t.Errorf("expected buckets_path 'by_panel>avg_watt', got %v", sumBucket["buckets_path"])
	}
}
