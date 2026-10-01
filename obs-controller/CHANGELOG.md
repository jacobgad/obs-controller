# Changelog

## 0.2.0

- Connections are now configured with a required free-form `name` (used as the Home Assistant device name); `id` is optional and defaults to the slugified name. Port and password remain optional (port defaults to 4455).

- Replaced the third-party obs-websocket library with a minimal internal client: context-aware requests, slog-integrated, and only two runtime dependencies remain (`paho.golang`, `gorilla/websocket`). The whole bridge is now covered by end-to-end tests against a fake OBS server.
- Removed the numeric status sensors (durations, congestion, dropped frames, CPU, memory, FPS, lag, disk) and their poll loop along with the `poll_interval_*` options. All remaining state is event-driven; the program preview screenshot is the only thing polled. The Last recording sensor stays. Retained discovery configs for the removed sensors are cleared automatically.

## 0.1.0

- Initial release.
- Multiple OBS instances via obs-websocket 5.x, each exposed as one Home Assistant MQTT device keyed on its configured `id`.
- Switches: stream, record, recording paused, studio mode. Selects: program scene, preview scene, transition (options track OBS live). Number: transition duration. Button: trigger studio-mode transition.
- Camera entity with a still of the current program scene, scaled by OBS, on configurable active/idle intervals.
- Status sensors (stream/record duration, congestion, dropped frames, last recording path) and diagnostics (CPU, memory, FPS, render/encode lag, free disk) on configurable active/idle poll intervals.
- Per-connection availability plus a bridge Last Will; a Connected binary sensor that stays available while OBS is down.
- Infinite reconnect with jittered exponential backoff; auth failures retried slowly and logged loudly; commands while disconnected are dropped, never queued; retained command replays are ignored.
