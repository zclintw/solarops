#!/bin/sh
# Wait for ES to be ready
until curl -s http://elasticsearch:9200/_cluster/health | grep -q '"status":"green"\|"status":"yellow"'; do
  echo "Waiting for Elasticsearch..."
  sleep 2
done

# Create index template
curl -X PUT "http://elasticsearch:9200/_index_template/plant-data-template" \
  -H "Content-Type: application/json" \
  -d '{
  "index_patterns": ["plant-data*"],
  "template": {
    "settings": {
      "number_of_shards": 1,
      "number_of_replicas": 0
    },
    "mappings": {
      "properties": {
        "plantId": { "type": "keyword" },
        "plantName": { "type": "keyword" },
        "timestamp": { "type": "date" },
        "totalWatt": { "type": "float" },
        "onlineCount": { "type": "integer" },
        "offlineCount": { "type": "integer" },
        "faultyCount": { "type": "integer" },
        "panels": {
          "type": "nested",
          "properties": {
            "panelId": { "type": "keyword" },
            "panelNumber": { "type": "integer" },
            "status": { "type": "keyword" },
            "faultMode": { "type": "keyword" },
            "watt": { "type": "float" }
          }
        }
      }
    }
  }
}'

echo ""
echo "Index template created."
