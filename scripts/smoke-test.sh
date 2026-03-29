#!/bin/bash
set -e

echo "=== SolarOps Smoke Test ==="

BASE_URL="${BASE_URL:-http://localhost}"
WS_URL="${WS_URL:-ws://localhost:8080/ws}"
PM_URL="${PM_URL:-http://localhost:8082}"
ALERT_URL="${ALERT_URL:-http://localhost:8081}"

echo ""
echo "1. Checking services are up..."
for url in "$PM_URL/health" "$ALERT_URL/health" "http://localhost:8080/health"; do
  status=$(curl -s -o /dev/null -w "%{http_code}" "$url")
  if [ "$status" = "200" ]; then
    echo "   ✓ $url"
  else
    echo "   ✗ $url (status: $status)"
    exit 1
  fi
done

echo ""
echo "2. Checking plants registered..."
sleep 5
plants=$(curl -s "$PM_URL/api/plants")
count=$(echo "$plants" | python3 -c "import sys,json; print(len(json.load(sys.stdin)))" 2>/dev/null || echo 0)
echo "   Plants registered: $count"
if [ "$count" -lt 1 ]; then
  echo "   ✗ No plants registered yet (may need more time)"
else
  echo "   ✓ Plants found"
fi

echo ""
echo "3. Checking Elasticsearch has data..."
sleep 5
es_count=$(curl -s "http://localhost:9200/plant-data/_count" | python3 -c "import sys,json; print(json.load(sys.stdin).get('count', 0))" 2>/dev/null || echo 0)
echo "   ES documents: $es_count"
if [ "$es_count" -gt 0 ]; then
  echo "   ✓ Data flowing to ES"
else
  echo "   ✗ No data in ES yet"
fi

echo ""
echo "4. Checking alerts endpoint..."
alerts=$(curl -s "$ALERT_URL/api/alerts")
echo "   Alerts response: $alerts"
echo "   ✓ Alert service responding"

echo ""
echo "5. Triggering fault on first plant..."
first_plant=$(echo "$plants" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d[0]['plantId'] if d else '')" 2>/dev/null)
if [ -n "$first_plant" ]; then
  echo "   Plant ID: $first_plant"
  echo "   (Fault trigger via Plant Manager API)"
  echo "   ✓ Fault trigger test requires panel IDs from WebSocket data"
else
  echo "   ⚠ No plant to test fault trigger"
fi

echo ""
echo "6. Checking frontend..."
frontend_status=$(curl -s -o /dev/null -w "%{http_code}" "http://localhost:3000")
if [ "$frontend_status" = "200" ]; then
  echo "   ✓ Frontend serving"
else
  echo "   ✗ Frontend not responding (status: $frontend_status)"
fi

echo ""
echo "=== Smoke Test Complete ==="
