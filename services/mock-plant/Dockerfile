FROM golang:1.25-alpine AS builder

WORKDIR /build

# Copy shared module first (for caching)
COPY shared/ ./shared/
COPY services/mock-plant/ ./services/mock-plant/
# Copy go.mod stubs for other workspace modules so go.work resolves
COPY services/alert-service/go.mod ./services/alert-service/go.mod
COPY services/plant-manager/go.mod ./services/plant-manager/go.mod
COPY services/ws-gateway/go.mod ./services/ws-gateway/go.mod
COPY services/aggregator/go.mod ./services/aggregator/go.mod
COPY go.work ./

WORKDIR /build/services/mock-plant
RUN CGO_ENABLED=0 go build -o /app/mock-plant .

FROM alpine:3.19

RUN mkdir -p /var/log/plant
COPY --from=builder /app/mock-plant /usr/local/bin/mock-plant

ENTRYPOINT ["mock-plant"]
