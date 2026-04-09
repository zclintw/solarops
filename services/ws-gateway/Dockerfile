FROM golang:1.25-alpine AS builder

WORKDIR /build
COPY shared/ ./shared/
COPY services/ws-gateway/ ./services/ws-gateway/
COPY go.work ./

# Copy go.mod stubs for other workspace modules (go.work resolution)
COPY services/mock-plant/go.mod ./services/mock-plant/go.mod
COPY services/alert-service/go.mod ./services/alert-service/go.mod
COPY services/plant-manager/go.mod ./services/plant-manager/go.mod

WORKDIR /build/services/ws-gateway
RUN CGO_ENABLED=0 go build -o /app/ws-gateway .

FROM alpine:3.19
COPY --from=builder /app/ws-gateway /usr/local/bin/ws-gateway
EXPOSE 8080
ENTRYPOINT ["ws-gateway"]
