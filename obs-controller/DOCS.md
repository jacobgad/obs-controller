# OBS Controller

Makes OBS Studio instances appear as MQTT devices in Home Assistant. Each configured obs-websocket connection becomes one Home Assistant device with switches, scene selects, status sensors and a live program preview.

## Installation

1. **Settings → Add-ons → Add-on Store → ⋮ → Repositories**, add this repository's URL.
2. Install **OBS Controller**.
3. In OBS: **Tools → WebSocket Server Settings**, enable the server and note the port and password.
4. On the add-on's **Configuration** tab add your `connections`, then **Start**.
5. Devices appear under **Settings → Devices & services → MQTT** once each OBS is reachable.

Requires the **Mosquitto broker** add-on and the **MQTT integration**. Broker credentials are read from the Supervisor; there is nothing to enter. OBS 28 or newer (obs-websocket 5.x) is required.

## Configuration

```yaml
connections:
  - name: Main OBS
    host: 192.168.1.50
    password: secret
active_screenshot_polling_interval_ms: 125
idle_screenshot_polling_interval_ms: 10000
screenshot_width: 640
screenshot_quality: 60
log_level: info
```

| Option | Default | Meaning |
| --- | --- | --- |
| `connections` | required | One entry per OBS instance. |
| `connections[].name` | required | Home Assistant device name, free-form. |
| `connections[].id` | slugified name | Stable slug (`a-z`, `0-9`, `_`, max 32) that all MQTT topics and Home Assistant entity IDs derive from. Changing the effective id orphans entity history — pin `id` explicitly before renaming a connection if history matters. Must be unique. |
| `connections[].host` | required | Hostname or IP of the machine running OBS. |
| `connections[].port` | `4455` | obs-websocket server port. |
| `connections[].password` | none | obs-websocket password, if authentication is enabled. |
| `active_screenshot_polling_interval_ms` | `125` | Capture rate while at least one MJPEG viewer is connected — this is the stream's frame rate (125 ≈ 8 fps, 100 = 10 fps). |
| `idle_screenshot_polling_interval_ms` | `10000` | Capture rate with no viewers; keeps a fresh frame ready for `/snapshot` and an instant first frame. |
| `screenshot_width` | `640` | Frame width in pixels; height follows the OBS canvas aspect ratio. OBS does the scaling. |
| `screenshot_quality` | `60` | JPEG compression quality, 0–100. |
| `log_level` | `info` | `debug` / `info` / `warn` / `error` |

## Entities

Each connection is one device named **OBS `<id>`**:

| Entity | Type | Notes |
| --- | --- | --- |
| Stream | switch | Start/stop streaming. |
| Record | switch | Start/stop recording. |
| Recording paused | switch | Pause/resume; only meaningful while recording. |
| Studio mode | switch | |
| Program scene | select | Options track the OBS scene list live. |
| Preview scene | select | Empty unless studio mode is on. |
| Transition | select | |
| Transition duration | number | Milliseconds; fixed transitions (Cut) ignore it. |
| Trigger transition | button | Studio-mode transition (preview → program). |
| Connected | binary sensor | OBS reachable from the add-on; stays available while OBS is down so automations can trigger on it. |
| Last recording | sensor | File path of the most recently finished recording. |

## Program preview (MJPEG)

The add-on serves a live program preview over HTTP on port 9981:

- `http://<ha-host>:9981/stream/<id>` — MJPEG stream (motion)
- `http://<ha-host>:9981/snapshot/<id>` — single JPEG still

Add it to Home Assistant once via **Settings → Devices & services → Add integration → MJPEG IP Camera** with the stream URL (MQTT discovery cannot create MJPEG cameras).

The add-on only asks OBS for frames at the fast rate while someone is actually watching — an open stream connection is the signal. With no viewers it captures one frame every `idle_screenshot_polling_interval_ms`, kept warm for `/snapshot` and instant stream starts. OBS renders, scales and JPEG-encodes every frame; the add-on never touches pixels.

## Behaviour

- State changes arrive as obs-websocket events and are published immediately; the only polling is the program preview capture described above.
- A program scene switch pushes a frame to every viewer immediately, regardless of the polling interval.
- When an OBS instance is unreachable its entities go unavailable in Home Assistant, and the add-on reconnects forever with exponential backoff (1–30 s). Commands received while disconnected are dropped and logged — never queued.
- A wrong password is retried once a minute and logged as `obs_auth_failed`; fix the password in OBS's WebSocket Server Settings and the add-on recovers on its own. Config changes on the add-on side require a restart.
- Retained MQTT messages on command topics are never acted on.

## Support

Issues and source: <https://github.com/jacobgad/obs-controller>
