package mqtt

import (
	"encoding/json"
	"strings"
	"testing"
)

var (
	testInfo   = DeviceInfo{ID: "main_obs", Name: "Main OBS", OBSVersion: "31.0.0", Platform: "windows"}
	testOrigin = Origin{Version: "0.1.0", SupportURL: "https://example.test"}
)

func TestDeviceMessages(t *testing.T) {
	msgs := DeviceMessages(testInfo, []string{"Intro", "Live"}, []string{"Cut", "Fade"}, testOrigin)
	if len(msgs) == 0 {
		t.Fatal("no messages")
	}
	seenUniqueIDs := map[string]bool{}
	for _, msg := range msgs {
		if !strings.HasPrefix(msg.Topic, "homeassistant/") || !strings.HasSuffix(msg.Topic, "/config") {
			t.Errorf("topic %q is not a discovery config topic", msg.Topic)
		}
		var payload map[string]any
		if err := json.Unmarshal(msg.JSON(), &payload); err != nil {
			t.Fatalf("payload for %s is not JSON: %v", msg.Topic, err)
		}
		uid, _ := payload["unique_id"].(string)
		if !strings.HasPrefix(uid, "obs_main_obs_") {
			t.Errorf("unique_id %q lacks device prefix", uid)
		}
		if seenUniqueIDs[uid] {
			t.Errorf("duplicate unique_id %q", uid)
		}
		seenUniqueIDs[uid] = true
		device, _ := payload["device"].(map[string]any)
		if device == nil || device["name"] != "Main OBS" || device["sw_version"] != "31.0.0" {
			t.Errorf("device block for %s = %v", msg.Topic, device)
		}
		if payload["origin"] == nil {
			t.Errorf("missing origin for %s", msg.Topic)
		}
	}
}

func TestSelectOptionsAndAvailability(t *testing.T) {
	msgs := DeviceMessages(testInfo, []string{"Intro", "Live"}, []string{"Cut"}, testOrigin)
	byObject := map[string]map[string]any{}
	for _, msg := range msgs {
		uid := msg.Payload["unique_id"].(string)
		byObject[strings.TrimPrefix(uid, "obs_main_obs_")] = msg.Payload
	}

	for _, object := range []string{"program_scene", "preview_scene"} {
		options, _ := byObject[object]["options"].([]string)
		if len(options) != 2 || options[0] != "Intro" {
			t.Errorf("%s options = %v", object, options)
		}
	}
	if options, _ := byObject["transition"]["options"].([]string); len(options) != 1 || options[0] != "Cut" {
		t.Errorf("transition options = %v", options)
	}

	connected := byObject["connected"]
	if avail, _ := connected["availability"].([]map[string]any); len(avail) != 1 || avail[0]["topic"] != BridgeAvailability {
		t.Errorf("connected availability = %v", connected["availability"])
	}
	stream := byObject["stream"]
	if avail, _ := stream["availability"].([]map[string]any); len(avail) != 2 {
		t.Errorf("stream availability = %v", stream["availability"])
	}
	if stream["availability_mode"] != "all" {
		t.Errorf("stream availability_mode = %v", stream["availability_mode"])
	}

	camera := byObject["screenshot"]
	if camera["topic"] != "obs/main_obs/screenshot" {
		t.Errorf("camera topic = %v", camera["topic"])
	}
}

func TestSceneSelectMessagesCoverBothSelects(t *testing.T) {
	msgs := SceneSelectMessages(testInfo, []string{"A"}, testOrigin)
	if len(msgs) != 2 {
		t.Fatalf("len = %d, want 2", len(msgs))
	}
	topics := map[string]bool{}
	for _, msg := range msgs {
		topics[msg.Topic] = true
	}
	for _, want := range []string{
		"homeassistant/select/obs_main_obs/program_scene/config",
		"homeassistant/select/obs_main_obs/preview_scene/config",
	} {
		if !topics[want] {
			t.Errorf("missing %s", want)
		}
	}
}

func TestTransitionSelectMessage(t *testing.T) {
	msg := TransitionSelectMessage(testInfo, []string{"Cut"}, testOrigin)
	if msg.Topic != "homeassistant/select/obs_main_obs/transition/config" {
		t.Errorf("topic = %q", msg.Topic)
	}
}
