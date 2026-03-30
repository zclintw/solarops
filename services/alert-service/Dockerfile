FROM golang:1.25-alpine AS builder

WORKDIR /build
COPY shared/ ./shared/
COPY services/alert-service/ ./services/alert-service/
COPY services/aggregator/go.mod ./services/aggregator/go.mod
COPY go.work ./
COPY services/mock-plant/go.mod ./services/mock-plant/go.mod
COPY services/ws-gateway/go.mod ./services/ws-gateway/go.mod
COPY services/plant-manager/go.mod ./services/plant-manager/go.mod

WORKDIR /build/services/alert-service
RUN CGO_ENABLED=0 go build -o /app/alert-service .

FROM alpine:3.19
COPY --from=builder /app/alert-service /usr/local/bin/alert-service
EXPOSE 8081
ENTRYPOINT ["alert-service"]
