# Changelog

## 0.1.0

- Initial release.
- Multiple OBS instances via obs-websocket 5.x, each exposed as one Home Assistant MQTT device keyed on its configured `id`.
- Switches: stream, record, recording paused, studio mode. Selects: program scene, preview scene, transition (options track OBS live). Number: transition duration. Button: trigger studio-mode transition.
- Camera entity with a still of the current program scene, scaled by OBS, on configurable active/idle intervals.
- Status sensors (stream/record duration, congestion, dropped frames, last recording path) and diagnostics (CPU, memory, FPS, render/encode lag, free disk) on configurable active/idle poll intervals.
- Per-connection availability plus a bridge Last Will; a Connected binary sensor that stays available while OBS is down.
- Infinite reconnect with jittered exponential backoff; auth failures retried slowly and logged loudly; commands while disconnected are dropped, never queued; retained command replays are ignored.
