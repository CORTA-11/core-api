# syntax=docker/dockerfile:1

FROM golang:1.26.6-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/core-api ./cmd/api \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/provisioner ./cmd/provisioner

FROM alpine:3.23

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 10001 core-api \
    && adduser -S -D -H -u 10001 -G core-api core-api

COPY --from=build /out/core-api /usr/local/bin/core-api
COPY --from=build /out/provisioner /usr/local/bin/provisioner

USER 10001:10001
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/core-api"]
