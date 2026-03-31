#!/bin/sh
# Wait for ES to be ready
until curl -s http://elasticsearch:9200/_cluster/health | grep -q '"status":"green"\|"status":"yellow"'; do
  echo "Waiting for Elasticsearch..."
  sleep 2
done

# --- ILM Policies ---

# Panel data: high volume (~1.3M docs/day for 3 plants), keep 7 days
curl -X PUT "http://elasticsearch:9200/_ilm/policy/plant-panel-policy" \
  -H "Content-Type: application/json" \
  -d '{
    "policy": {
      "phases": {
        "hot":    { "min_age": "0ms", "actions": {} },
        "delete": { "min_age": "7d",  "actions": { "delete": {} } }
      }
    }
  }'
echo ""

# Summary data: low volume (~8640 docs/day per plant), keep 30 days
curl -X PUT "http://elasticsearch:9200/_ilm/policy/plant-summary-policy" \
  -H "Content-Type: application/json" \
  -d '{
    "policy": {
      "phases": {
        "hot":    { "min_age": "0ms", "actions": {} },
        "delete": { "min_age": "30d", "actions": { "delete": {} } }
      }
    }
  }'
echo ""

# --- Index Templates ---

# Panel-level readings (written by Fluentd): plant-panel-YYYY-MM-DD
# @timestamp: produced by Fluentd logstash_format (used for ES/Kibana queries)
# timestamp:  produced by PanelReading struct JSON (used by frontend TypeScript)
curl -X PUT "http://elasticsearch:9200/_index_template/plant-panel-template" \
  -H "Content-Type: application/json" \
  -d '{
  "index_patterns": ["plant-panel-*"],
  "template": {
    "settings": {
      "number_of_shards": 1,
      "number_of_replicas": 0,
      "index.lifecycle.name": "plant-panel-policy"
    },
    "mappings": {
      "properties": {
        "@timestamp":  { "type": "date" },
        "timestamp":   { "type": "date" },
        "plantId":     { "type": "keyword" },
        "plantName":   { "type": "keyword" },
        "panelId":     { "type": "keyword" },
        "panelNumber": { "type": "integer" },
        "status":      { "type": "keyword" },
        "faultMode":   { "type": "keyword" },
        "watt":        { "type": "float" }
      }
    }
  }
}'
echo ""

# Plant-level summaries (written by aggregator): plant-summary-YYYY-MM-DD
# @timestamp: written by aggregator alongside timestamp for ES/Kibana consistency
# timestamp:  written by aggregator (used by frontend TypeScript and backend queries)
curl -X PUT "http://elasticsearch:9200/_index_template/plant-summary-template" \
  -H "Content-Type: application/json" \
  -d '{
  "index_patterns": ["plant-summary-*"],
  "template": {
    "settings": {
      "number_of_shards": 1,
      "number_of_replicas": 0,
      "index.lifecycle.name": "plant-summary-policy"
    },
    "mappings": {
      "properties": {
        "@timestamp":   { "type": "date" },
        "timestamp":    { "type": "date" },
        "plantId":      { "type": "keyword" },
        "plantName":    { "type": "keyword" },
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
echo "ILM policies and index templates created."
