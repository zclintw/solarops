FROM golang:1.25-alpine AS build
WORKDIR /app
# Copy go.mod stubs for all workspace modules so go.work resolves
COPY services/alert-service/go.mod ./services/alert-service/go.mod
COPY services/mock-plant/go.mod ./services/mock-plant/go.mod
COPY services/plant-manager/go.mod ./services/plant-manager/go.mod
COPY services/ws-gateway/go.mod ./services/ws-gateway/go.mod
COPY go.work go.work.sum ./
COPY shared/ shared/
COPY services/aggregator/ services/aggregator/
RUN cd services/aggregator && go build -o /aggregator .

FROM alpine:3.20
COPY --from=build /aggregator /aggregator
CMD ["/aggregator"]
