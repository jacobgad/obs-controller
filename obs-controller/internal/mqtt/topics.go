package mqtt

import "regexp"

// Topic layout and payload constants shared with Home Assistant.
const (
	Prefix            = "obs"
	HADiscoveryPrefix = "homeassistant"

	PayloadOnline  = "online"
	PayloadOffline = "offline"
	PayloadOn      = "ON"
	PayloadOff     = "OFF"
	PayloadPress   = "PRESS"

	BridgeAvailability = Prefix + "/bridge/availability"

	CommandSetWildcard   = Prefix + "/+/+/set"
	CommandPressWildcard = Prefix + "/+/+/press"
)

// DeviceTopics are the per-connection topics; the configured id is the only identity
// in the path.
type DeviceTopics struct {
	Availability            string
	ConnectedState          string
	StreamState             string
	StreamSet               string
	RecordState             string
	RecordSet               string
	RecordPausedState       string
	RecordPausedSet         string
	StudioModeState         string
	StudioModeSet           string
	ProgramSceneState       string
	ProgramSceneSet         string
	PreviewSceneState       string
	PreviewSceneSet         string
	TransitionState         string
	TransitionSet           string
	TransitionDurationState string
	TransitionDurationSet   string
	TriggerTransitionPress  string
	Screenshot              string
	base                    string
}

// ForDevice derives the topic set for a connection id.
func ForDevice(id string) DeviceTopics {
	base := Prefix + "/" + id
	return DeviceTopics{
		Availability:            base + "/availability",
		ConnectedState:          base + "/connected/state",
		StreamState:             base + "/stream/state",
		StreamSet:               base + "/stream/set",
		RecordState:             base + "/record/state",
		RecordSet:               base + "/record/set",
		RecordPausedState:       base + "/record_paused/state",
		RecordPausedSet:         base + "/record_paused/set",
		StudioModeState:         base + "/studio_mode/state",
		StudioModeSet:           base + "/studio_mode/set",
		ProgramSceneState:       base + "/program_scene/state",
		ProgramSceneSet:         base + "/program_scene/set",
		PreviewSceneState:       base + "/preview_scene/state",
		PreviewSceneSet:         base + "/preview_scene/set",
		TransitionState:         base + "/transition/state",
		TransitionSet:           base + "/transition/set",
		TransitionDurationState: base + "/transition_duration/state",
		TransitionDurationSet:   base + "/transition_duration/set",
		TriggerTransitionPress:  base + "/trigger_transition/press",
		Screenshot:              base + "/screenshot",
		base:                    base,
	}
}

// SensorState is the state topic for a named sensor.
func (t DeviceTopics) SensorState(object string) string {
	return t.base + "/sensor/" + object + "/state"
}

// DeviceNodeID is the discovery node_id for a connection id.
func DeviceNodeID(id string) string { return "obs_" + id }

// DeviceIdentifier is the Home Assistant device registry identifier for a connection id.
func DeviceIdentifier(id string) string { return "obs:" + id }

// HADiscoveryTopic builds homeassistant/<component>/<node>/<object>/config.
func HADiscoveryTopic(component, nodeID, objectID string) string {
	return HADiscoveryPrefix + "/" + component + "/" + nodeID + "/" + objectID + "/config"
}

var commandPattern = regexp.MustCompile(`^` + Prefix + `/([a-z0-9_]{1,32})/([a-z_]+)/(set|press)$`)

// Command is a parsed obs/<id>/<object>/(set|press) topic.
type Command struct {
	ConnectionID string
	Object       string
}

// ParseCommand recognises command topics.
func ParseCommand(topic string) (Command, bool) {
	m := commandPattern.FindStringSubmatch(topic)
	if m == nil {
		return Command{}, false
	}
	return Command{ConnectionID: m[1], Object: m[2]}, true
}
