FROM golang:1.25-alpine AS build
WORKDIR /app
COPY go.work go.work.sum ./
COPY shared/ shared/
COPY services/aggregator/ services/aggregator/
RUN cd services/aggregator && go build -o /aggregator .

FROM alpine:3.20
COPY --from=build /aggregator /aggregator
CMD ["/aggregator"]
