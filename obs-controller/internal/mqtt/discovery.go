package mqtt

import "encoding/json"

// Origin identifies this add-on in discovery payloads.
type Origin struct {
	Version    string
	SupportURL string
}

// DeviceInfo identifies one OBS connection in the HA device registry.
type DeviceInfo struct {
	ID         string
	Name       string
	OBSVersion string
	Platform   string
}

// Message is one retained discovery config.
type Message struct {
	Topic   string
	Payload map[string]any
}

// JSON renders the payload.
func (m Message) JSON() []byte {
	data, _ := json.Marshal(m.Payload)
	return data
}

const bridgeName = "OBS Controller"

type entity struct {
	component string
	object    string
	name      string
	category  string
	icon      string
	// bridgeOnly entities stay available while the OBS connection is down (the
	// Connected binary sensor must read OFF then, not unavailable).
	bridgeOnly bool
	fields     func(t DeviceTopics, options []string) map[string]any
}

func switchFields(state, set string) func(DeviceTopics, []string) map[string]any {
	return func(DeviceTopics, []string) map[string]any {
		return map[string]any{
			"state_topic":   state,
			"command_topic": set,
			"payload_on":    PayloadOn,
			"payload_off":   PayloadOff,
			"optimistic":    false,
			"retain":        false,
			"qos":           1,
		}
	}
}

func selectFields(state, set string) func(DeviceTopics, []string) map[string]any {
	return func(_ DeviceTopics, options []string) map[string]any {
		if options == nil {
			options = []string{}
		}
		return map[string]any{
			"state_topic":   state,
			"command_topic": set,
			"options":       options,
			"optimistic":    false,
			"retain":        false,
			"qos":           1,
		}
	}
}

func entities() []entity {
	return []entity{
		{component: "switch", object: "stream", name: "Stream", icon: "mdi:broadcast",
			fields: func(t DeviceTopics, _ []string) map[string]any {
				return switchFields(t.StreamState, t.StreamSet)(t, nil)
			}},
		{component: "switch", object: "record", name: "Record", icon: "mdi:record-rec",
			fields: func(t DeviceTopics, _ []string) map[string]any {
				return switchFields(t.RecordState, t.RecordSet)(t, nil)
			}},
		{component: "switch", object: "record_paused", name: "Recording paused", icon: "mdi:pause-circle-outline",
			fields: func(t DeviceTopics, _ []string) map[string]any {
				return switchFields(t.RecordPausedState, t.RecordPausedSet)(t, nil)
			}},
		{component: "switch", object: "studio_mode", name: "Studio mode", icon: "mdi:monitor-multiple",
			fields: func(t DeviceTopics, _ []string) map[string]any {
				return switchFields(t.StudioModeState, t.StudioModeSet)(t, nil)
			}},
		programSceneSelect,
		previewSceneSelect,
		transitionSelect,
		{component: "number", object: "transition_duration", name: "Transition duration", icon: "mdi:timer-cog-outline",
			fields: func(t DeviceTopics, _ []string) map[string]any {
				return map[string]any{
					"state_topic":         t.TransitionDurationState,
					"command_topic":       t.TransitionDurationSet,
					"min":                 50,
					"max":                 20000,
					"step":                50,
					"mode":                "box",
					"unit_of_measurement": "ms",
					"optimistic":          false,
					"retain":              false,
					"qos":                 1,
				}
			}},
		{component: "button", object: "trigger_transition", name: "Trigger transition", icon: "mdi:play-box-outline",
			fields: func(t DeviceTopics, _ []string) map[string]any {
				return map[string]any{
					"command_topic": t.TriggerTransitionPress,
					"payload_press": PayloadPress,
					"retain":        false,
					"qos":           1,
				}
			}},
		{component: "binary_sensor", object: "connected", name: "Connected", category: "diagnostic",
			bridgeOnly: true,
			fields: func(t DeviceTopics, _ []string) map[string]any {
				return map[string]any{
					"state_topic":  t.ConnectedState,
					"device_class": "connectivity",
					"payload_on":   PayloadOn,
					"payload_off":  PayloadOff,
				}
			}},
		{component: "sensor", object: "last_recording_path", name: "Last recording", icon: "mdi:file-video",
			fields: func(t DeviceTopics, _ []string) map[string]any {
				return map[string]any{"state_topic": t.SensorState("last_recording_path")}
			}},
	}
}

// retiredEntities were published by earlier releases; their retained discovery
// configs are cleared so stale entities never linger in Home Assistant.
var retiredEntities = []struct{ component, object string }{
	{"sensor", "stream_duration"}, {"sensor", "stream_congestion"}, {"sensor", "stream_dropped_pct"},
	{"sensor", "record_duration"}, {"sensor", "cpu_usage"}, {"sensor", "memory_usage"},
	{"sensor", "active_fps"}, {"sensor", "render_lag_pct"}, {"sensor", "encode_lag_pct"},
	{"sensor", "free_disk_space"},
	{"camera", "screenshot"},
}

// RetiredDiscoveryTopics lists config topics to clear for one connection id.
func RetiredDiscoveryTopics(id string) []string {
	topics := make([]string, 0, len(retiredEntities))
	for _, e := range retiredEntities {
		topics = append(topics, HADiscoveryTopic(e.component, DeviceNodeID(id), e.object))
	}
	return topics
}

var (
	programSceneSelect = entity{component: "select", object: "program_scene", name: "Program scene", icon: "mdi:movie-open",
		fields: func(t DeviceTopics, options []string) map[string]any {
			return selectFields(t.ProgramSceneState, t.ProgramSceneSet)(t, options)
		}}
	previewSceneSelect = entity{component: "select", object: "preview_scene", name: "Preview scene", icon: "mdi:movie-open-outline",
		fields: func(t DeviceTopics, options []string) map[string]any {
			return selectFields(t.PreviewSceneState, t.PreviewSceneSet)(t, options)
		}}
	transitionSelect = entity{component: "select", object: "transition", name: "Transition", icon: "mdi:transition",
		fields: func(t DeviceTopics, options []string) map[string]any {
			return selectFields(t.TransitionState, t.TransitionSet)(t, options)
		}}
)

func build(info DeviceInfo, e entity, options []string, o Origin) Message {
	topics := ForDevice(info.ID)
	payload := e.fields(topics, options)
	payload["name"] = e.name
	payload["unique_id"] = DeviceNodeID(info.ID) + "_" + e.object
	payload["object_id"] = DeviceNodeID(info.ID) + "_" + e.object
	if e.icon != "" {
		payload["icon"] = e.icon
	}
	payload["device"] = device(info)
	payload["origin"] = origin(o)
	if e.category != "" {
		payload["entity_category"] = e.category
	}
	if e.bridgeOnly {
		payload["availability"] = []map[string]any{bridgeAvailability()}
	} else {
		payload["availability"] = []map[string]any{
			bridgeAvailability(),
			{"topic": topics.Availability, "payload_available": PayloadOnline, "payload_not_available": PayloadOffline},
		}
		payload["availability_mode"] = "all"
	}
	return Message{Topic: HADiscoveryTopic(e.component, DeviceNodeID(info.ID), e.object), Payload: payload}
}

// DeviceMessages lists every entity published for one OBS connection.
func DeviceMessages(info DeviceInfo, scenes, transitions []string, o Origin) []Message {
	specs := entities()
	out := make([]Message, 0, len(specs))
	for _, e := range specs {
		out = append(out, build(info, e, optionsFor(e, scenes, transitions), o))
	}
	return out
}

// SceneSelectMessages rebuilds the two scene selects, republished whenever the
// scene list changes.
func SceneSelectMessages(info DeviceInfo, scenes []string, o Origin) []Message {
	return []Message{
		build(info, programSceneSelect, scenes, o),
		build(info, previewSceneSelect, scenes, o),
	}
}

// TransitionSelectMessage rebuilds the transition select, republished whenever the
// transition list changes.
func TransitionSelectMessage(info DeviceInfo, transitions []string, o Origin) Message {
	return build(info, transitionSelect, transitions, o)
}

func optionsFor(e entity, scenes, transitions []string) []string {
	switch e.object {
	case "program_scene", "preview_scene":
		return scenes
	case "transition":
		return transitions
	default:
		return nil
	}
}

func origin(o Origin) map[string]any {
	return map[string]any{"name": bridgeName, "sw_version": o.Version, "support_url": o.SupportURL}
}

func bridgeAvailability() map[string]any {
	return map[string]any{"topic": BridgeAvailability, "payload_available": PayloadOnline, "payload_not_available": PayloadOffline}
}

func device(info DeviceInfo) map[string]any {
	name := info.Name
	if name == "" {
		name = "OBS " + info.ID
	}
	d := map[string]any{
		"identifiers":  []string{DeviceIdentifier(info.ID)},
		"name":         name,
		"manufacturer": "OBS Project",
		"model":        "OBS Studio",
	}
	if info.OBSVersion != "" {
		d["sw_version"] = info.OBSVersion
	}
	if info.Platform != "" {
		d["model_id"] = info.Platform
	}
	return d
}
