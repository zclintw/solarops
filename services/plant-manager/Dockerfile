FROM golang:1.25-alpine AS builder

WORKDIR /build
COPY shared/ ./shared/
COPY services/plant-manager/ ./services/plant-manager/
# go.work stubs for sibling modules
COPY services/mock-plant/go.mod ./services/mock-plant/go.mod
COPY services/ws-gateway/go.mod ./services/ws-gateway/go.mod
COPY services/alert-service/go.mod ./services/alert-service/go.mod
COPY services/aggregator/go.mod ./services/aggregator/go.mod
COPY go.work ./

WORKDIR /build/services/plant-manager
RUN CGO_ENABLED=0 go build -o /app/plant-manager .

FROM alpine:3.19
COPY --from=builder /app/plant-manager /usr/local/bin/plant-manager
EXPOSE 8082
ENTRYPOINT ["plant-manager"]
