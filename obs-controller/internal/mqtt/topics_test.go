package mqtt

import "testing"

func TestForDevice(t *testing.T) {
	topics := ForDevice("main_obs")
	cases := map[string]string{
		topics.Availability:                       "obs/main_obs/availability",
		topics.ConnectedState:                     "obs/main_obs/connected/state",
		topics.StreamState:                        "obs/main_obs/stream/state",
		topics.StreamSet:                          "obs/main_obs/stream/set",
		topics.RecordPausedSet:                    "obs/main_obs/record_paused/set",
		topics.ProgramSceneState:                  "obs/main_obs/program_scene/state",
		topics.TransitionDurationSet:              "obs/main_obs/transition_duration/set",
		topics.TriggerTransitionPress:             "obs/main_obs/trigger_transition/press",
		topics.SensorState("last_recording_path"): "obs/main_obs/sensor/last_recording_path/state",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("topic = %q, want %q", got, want)
		}
	}
}

func TestParseCommand(t *testing.T) {
	valid := map[string]Command{
		"obs/main_obs/stream/set":               {ConnectionID: "main_obs", Object: "stream"},
		"obs/a1/transition_duration/set":        {ConnectionID: "a1", Object: "transition_duration"},
		"obs/main_obs/trigger_transition/press": {ConnectionID: "main_obs", Object: "trigger_transition"},
	}
	for topic, want := range valid {
		got, ok := ParseCommand(topic)
		if !ok || got != want {
			t.Errorf("ParseCommand(%q) = %+v, %v; want %+v", topic, got, ok, want)
		}
	}
	invalid := []string{
		"obs/main_obs/stream/state",
		"obs/Main/stream/set",
		"homeassistant/status",
		"obs/main_obs/stream/set/extra",
	}
	for _, topic := range invalid {
		if _, ok := ParseCommand(topic); ok {
			t.Errorf("ParseCommand(%q) unexpectedly matched", topic)
		}
	}
}
