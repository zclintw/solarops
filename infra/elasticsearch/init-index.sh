#!/bin/sh
# Wait for ES to be ready
until curl -s http://elasticsearch:9200/_cluster/health | grep -q '"status":"green"\|"status":"yellow"'; do
  echo "Waiting for Elasticsearch..."
  sleep 2
done

# Panel-level readings (written by Fluentd): plant-panel-YYYY-MM-DD
curl -X PUT "http://elasticsearch:9200/_index_template/plant-panel-template" \
  -H "Content-Type: application/json" \
  -d '{
  "index_patterns": ["plant-panel-*"],
  "template": {
    "settings": {
      "number_of_shards": 1,
      "number_of_replicas": 0
    },
    "mappings": {
      "properties": {
        "plantId":     { "type": "keyword" },
        "plantName":   { "type": "keyword" },
        "panelId":     { "type": "keyword" },
        "panelNumber": { "type": "integer" },
        "status":      { "type": "keyword" },
        "faultMode":   { "type": "keyword" },
        "watt":        { "type": "float" },
        "timestamp":   { "type": "date" }
      }
    }
  }
}'

echo ""

# Plant-level summaries (written by aggregator): plant-summary-YYYY-MM-DD
curl -X PUT "http://elasticsearch:9200/_index_template/plant-summary-template" \
  -H "Content-Type: application/json" \
  -d '{
  "index_patterns": ["plant-summary-*"],
  "template": {
    "settings": {
      "number_of_shards": 1,
      "number_of_replicas": 0
    },
    "mappings": {
      "properties": {
        "plantId":      { "type": "keyword" },
        "plantName":    { "type": "keyword" },
        "timestamp":    { "type": "date" },
        "totalWatt":    { "type": "float" },
        "panelCount":   { "type": "integer" },
        "onlineCount":  { "type": "integer" },
        "offlineCount": { "type": "integer" },
        "faultyCount":  { "type": "integer" }
      }
    }
  }
}'

echo ""
echo "Index templates created."
