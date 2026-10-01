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
  - id: main_obs
    host: 192.168.1.50
    port: 4455
    password: secret
poll_interval_active_seconds: 5
poll_interval_idle_seconds: 30
screenshot_interval_active_seconds: 2
screenshot_interval_idle_seconds: 10
screenshot_width: 640
screenshot_quality: 60
log_level: info
```

| Option | Default | Meaning |
| --- | --- | --- |
| `connections` | required | One entry per OBS instance. |
| `connections[].id` | required | Stable slug (`a-z`, `0-9`, `_`, max 32). All MQTT topics and Home Assistant entity IDs derive from it — renaming it orphans entity history. Must be unique. |
| `connections[].host` | required | Hostname or IP of the machine running OBS. |
| `connections[].port` | `4455` | obs-websocket server port. |
| `connections[].password` | none | obs-websocket password, if authentication is enabled. |
| `poll_interval_active_seconds` | `5` | Sensor poll interval while streaming or recording. |
| `poll_interval_idle_seconds` | `30` | Sensor poll interval otherwise. |
| `screenshot_interval_active_seconds` | `2` | Program preview refresh while streaming or recording. |
| `screenshot_interval_idle_seconds` | `10` | Program preview refresh otherwise. |
| `screenshot_width` | `640` | Preview width in pixels; height follows the OBS canvas aspect ratio. OBS does the scaling. |
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
| Program | camera | Still of the current program scene, refreshed on the screenshot interval. |
| Connected | binary sensor | OBS reachable from the add-on; stays available while OBS is down so automations can trigger on it. |
| Stream duration / congestion / dropped frames | sensors | |
| Record duration / Last recording | sensors | |
| CPU, Memory, FPS, Render lag, Encode lag, Free disk | diagnostic sensors | From OBS's own stats. |

## Behaviour

- State changes arrive as obs-websocket events and are published immediately; numeric sensors are polled on the active/idle intervals.
- When an OBS instance is unreachable its entities go unavailable in Home Assistant, and the add-on reconnects forever with exponential backoff (1–30 s). Commands received while disconnected are dropped and logged — never queued.
- A wrong password is retried once a minute and logged as `obs_auth_failed`; fix the password in OBS's WebSocket Server Settings and the add-on recovers on its own. Config changes on the add-on side require a restart.
- Retained MQTT messages on command topics are never acted on.

## Support

Issues and source: <https://github.com/jacobgad/obs-controller>
