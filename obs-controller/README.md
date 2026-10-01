# OBS Controller

Home Assistant add-on: MQTT bridge exposing OBS Studio instances as Home Assistant devices via obs-websocket.

See [DOCS.md](./DOCS.md) for installation, configuration and the entity list.

## Development

```sh
go build ./...
go test ./...
golangci-lint run ./...
```

Run outside the Supervisor by setting the broker and options path explicitly:

```sh
MQTT_HOST=localhost OBS_CONTROLLER_OPTIONS_PATH=./options.json go run .
```
