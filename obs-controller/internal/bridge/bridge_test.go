package bridge_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/jacobgad/obs-controller/internal/bridge"
	"github.com/jacobgad/obs-controller/internal/config"
	"github.com/jacobgad/obs-controller/internal/mqtt"
	"github.com/jacobgad/obs-controller/internal/obs/obstest"
	"github.com/jacobgad/obs-controller/internal/testutil"
)

const testTimeout = 5 * time.Second

func newOBSServer(t *testing.T) *obstest.Server {
	t.Helper()
	srv := obstest.New("pw")
	t.Cleanup(srv.Close)
	srv.Respond("GetVersion", map[string]any{
		"obsVersion":            "31.0.0",
		"obsWebSocketVersion":   "5.5.2",
		"platform":              "linux",
		"supportedImageFormats": []string{"jpg", "png"},
	})
	srv.Respond("GetVideoSettings", map[string]any{"baseWidth": 1920, "baseHeight": 1080})
	srv.Respond("GetSceneList", map[string]any{
		"currentProgramSceneName": "Live",
		"scenes":                  []map[string]any{{"sceneName": "Intro"}, {"sceneName": "Live"}},
	})
	srv.Respond("GetSceneTransitionList", map[string]any{
		"currentSceneTransitionName": "Fade",
		"transitions":                []map[string]any{{"transitionName": "Fade"}},
	})
	srv.Respond("GetCurrentSceneTransition", map[string]any{"transitionName": "Fade", "transitionDuration": 300})
	srv.Respond("GetStudioModeEnabled", map[string]any{"studioModeEnabled": false})
	srv.Respond("GetStreamStatus", map[string]any{"outputActive": false})
	srv.Respond("GetRecordStatus", map[string]any{"outputActive": false})
	srv.Respond("GetSourceScreenshot", map[string]any{"imageData": "data:image/jpg;base64,aGVsbG8="})
	return srv
}

func startBridge(t *testing.T, srv *obstest.Server, broker *testutil.FakeMQTT) *bridge.Bridge {
	t.Helper()
	host, portText, err := net.SplitHostPort(srv.Addr())
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	port, _ := strconv.Atoi(portText)
	opts := config.Options{
		Connections:        []config.Connection{{ID: "test", Name: "Test OBS", Host: host, Port: port, Password: "pw"}},
		ActivePollInterval: 50 * time.Millisecond,
		IdlePollInterval:   time.Hour,
		ScreenshotWidth:    640, ScreenshotQuality: 60,
	}
	b := bridge.New(bridge.Deps{
		MQTT:     broker,
		Options:  opts,
		Log:      slog.New(slog.DiscardHandler),
		Origin:   mqtt.Origin{Version: "test", SupportURL: "https://example.test"},
		HTTPAddr: "127.0.0.1:0",
	})
	ctx, cancel := context.WithCancel(context.Background())
	if err := b.Start(ctx); err != nil {
		cancel()
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		cancel()
		stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
		defer stopCancel()
		b.Stop(stopCtx)
	})
	return b
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(testTimeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func lastPayload(broker *testutil.FakeMQTT, topic string) string {
	payload, _ := broker.Last(topic)
	return string(payload)
}

func TestBridgePublishesDiscoveryAndState(t *testing.T) {
	srv := newOBSServer(t)
	broker := testutil.NewFakeMQTT()
	startBridge(t, srv, broker)

	waitFor(t, "availability online", func() bool {
		return lastPayload(broker, "obs/test/availability") == "online"
	})
	if got := lastPayload(broker, "obs/test/connected/state"); got != "ON" {
		t.Errorf("connected = %q", got)
	}
	if got := lastPayload(broker, "obs/test/program_scene/state"); got != "Live" {
		t.Errorf("program scene = %q", got)
	}
	if got := lastPayload(broker, "obs/test/transition_duration/state"); got != "300" {
		t.Errorf("transition duration = %q", got)
	}
	for _, retired := range []string{
		"homeassistant/sensor/obs_test/cpu_usage/config",
		"homeassistant/camera/obs_test/screenshot/config",
	} {
		if payload, ok := broker.Last(retired); !ok || len(payload) != 0 {
			t.Errorf("retired config %s = %q, %v; want cleared", retired, payload, ok)
		}
	}

	var discovery map[string]any
	payload, ok := broker.Last("homeassistant/select/obs_test/program_scene/config")
	if !ok {
		t.Fatal("missing program scene discovery config")
	}
	if err := json.Unmarshal(payload, &discovery); err != nil {
		t.Fatalf("discovery payload: %v", err)
	}
	options, _ := discovery["options"].([]any)
	if len(options) != 2 || options[0] != "Live" {
		t.Errorf("scene options = %v (OBS bottom-up order should be reversed)", options)
	}
}

func TestBridgeRoutesCommandsToOBS(t *testing.T) {
	srv := newOBSServer(t)
	broker := testutil.NewFakeMQTT()
	startBridge(t, srv, broker)
	waitFor(t, "availability online", func() bool {
		return lastPayload(broker, "obs/test/availability") == "online"
	})

	broker.Deliver("obs/test/program_scene/set", []byte("Intro"))
	waitFor(t, "SetCurrentProgramScene request", func() bool {
		for _, req := range srv.Received() {
			if req.Type == "SetCurrentProgramScene" {
				var params struct {
					SceneName string `json:"sceneName"`
				}
				return json.Unmarshal(req.Data, &params) == nil && params.SceneName == "Intro"
			}
		}
		return false
	})

	broker.Deliver("obs/test/stream/set", []byte("ON"))
	waitFor(t, "StartStream request", func() bool {
		for _, req := range srv.Received() {
			if req.Type == "StartStream" {
				return true
			}
		}
		return false
	})
}

func TestBridgePublishesEventDrivenState(t *testing.T) {
	srv := newOBSServer(t)
	broker := testutil.NewFakeMQTT()
	startBridge(t, srv, broker)
	waitFor(t, "availability online", func() bool {
		return lastPayload(broker, "obs/test/availability") == "online"
	})

	if err := srv.SendEvent("CurrentProgramSceneChanged", map[string]any{"sceneName": "Intro"}); err != nil {
		t.Fatalf("SendEvent: %v", err)
	}
	waitFor(t, "program scene state", func() bool {
		return lastPayload(broker, "obs/test/program_scene/state") == "Intro"
	})

	if err := srv.SendEvent("RecordStateChanged", map[string]any{
		"outputActive": true, "outputState": "OBS_WEBSOCKET_OUTPUT_STARTED",
	}); err != nil {
		t.Fatalf("SendEvent: %v", err)
	}
	waitFor(t, "record state", func() bool {
		return lastPayload(broker, "obs/test/record/state") == "ON"
	})
}

func TestBridgeMarksDeviceOfflineWhenOBSDies(t *testing.T) {
	srv := newOBSServer(t)
	broker := testutil.NewFakeMQTT()
	startBridge(t, srv, broker)
	waitFor(t, "availability online", func() bool {
		return lastPayload(broker, "obs/test/availability") == "online"
	})

	srv.Close()
	waitFor(t, "availability offline", func() bool {
		return lastPayload(broker, "obs/test/availability") == "offline"
	})
	if got := lastPayload(broker, "obs/test/connected/state"); got != "OFF" {
		t.Errorf("connected = %q", got)
	}
}
