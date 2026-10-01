FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
COPY internal ./internal
ARG BUILD_VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${BUILD_VERSION}" -o /out/obs-controller .

FROM alpine:3.24

ARG BUILD_VERSION=dev
ARG BUILD_ARCH

RUN apk add --no-cache ca-certificates
COPY --from=build /out/obs-controller /usr/local/bin/obs-controller

LABEL \
  io.hass.name="OBS Controller" \
  io.hass.description="MQTT bridge for OBS Studio via obs-websocket" \
  io.hass.version="${BUILD_VERSION}" \
  io.hass.type="app" \
  io.hass.arch="aarch64|amd64"

CMD ["/usr/local/bin/obs-controller"]
