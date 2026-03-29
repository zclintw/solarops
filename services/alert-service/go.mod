module github.com/solarops/alert-service

go 1.25.0

require (
	github.com/elastic/go-elasticsearch/v8 v8.19.3
	github.com/google/uuid v1.6.0
	github.com/nats-io/nats.go v1.50.0
	github.com/solarops/shared v0.0.0
)

require (
	github.com/elastic/elastic-transport-go/v8 v8.8.0 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/klauspost/compress v1.18.5 // indirect
	github.com/nats-io/nkeys v0.4.15 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	go.opentelemetry.io/otel v1.28.0 // indirect
	go.opentelemetry.io/otel/metric v1.28.0 // indirect
	go.opentelemetry.io/otel/trace v1.28.0 // indirect
	golang.org/x/crypto v0.49.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
)

replace github.com/solarops/shared => ../../shared
