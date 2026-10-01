package obs_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jacobgad/obs-controller/internal/obs"
	"github.com/jacobgad/obs-controller/internal/obs/obstest"
)

func dial(t *testing.T, srv *obstest.Server, password string) *obs.Client {
	t.Helper()
	client, err := obs.Dial(context.Background(), obs.Options{
		Addr:               srv.Addr(),
		Password:           password,
		EventSubscriptions: obs.SubScenes | obs.SubOutputs,
	})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

func TestDialWithoutAuth(t *testing.T) {
	srv := obstest.New("")
	defer srv.Close()
	client := dial(t, srv, "")
	if err := client.Call(context.Background(), "GetVersion", nil, nil); err != nil {
		t.Fatalf("Call: %v", err)
	}
}

func TestDialWithAuth(t *testing.T) {
	srv := obstest.New("hunter2")
	defer srv.Close()
	client := dial(t, srv, "hunter2")
	if err := client.Call(context.Background(), "GetVersion", nil, nil); err != nil {
		t.Fatalf("Call: %v", err)
	}
}

func TestDialWrongPassword(t *testing.T) {
	srv := obstest.New("hunter2")
	defer srv.Close()
	_, err := obs.Dial(context.Background(), obs.Options{Addr: srv.Addr(), Password: "wrong"})
	if !errors.Is(err, obs.ErrAuthFailed) {
		t.Fatalf("err = %v, want ErrAuthFailed", err)
	}
}

func TestDialMissingPassword(t *testing.T) {
	srv := obstest.New("hunter2")
	defer srv.Close()
	_, err := obs.Dial(context.Background(), obs.Options{Addr: srv.Addr()})
	if !errors.Is(err, obs.ErrAuthFailed) {
		t.Fatalf("err = %v, want ErrAuthFailed", err)
	}
}

func TestCallDecodesResponse(t *testing.T) {
	srv := obstest.New("")
	defer srv.Close()
	srv.Respond("GetVersion", map[string]any{
		"obsVersion":            "31.0.0",
		"obsWebSocketVersion":   "5.5.2",
		"platform":              "windows",
		"supportedImageFormats": []string{"jpg", "png"},
	})
	client := dial(t, srv, "")
	version, err := client.GetVersion(context.Background())
	if err != nil {
		t.Fatalf("GetVersion: %v", err)
	}
	if version.ObsVersion != "31.0.0" || version.ObsWebSocketVersion != "5.5.2" || len(version.SupportedImageFormats) != 2 {
		t.Errorf("version = %+v", version)
	}
}

func TestCallSendsParams(t *testing.T) {
	srv := obstest.New("")
	defer srv.Close()
	client := dial(t, srv, "")
	if err := client.SetCurrentProgramScene(context.Background(), "Live"); err != nil {
		t.Fatalf("SetCurrentProgramScene: %v", err)
	}
	received := srv.Received()
	last := received[len(received)-1]
	if last.Type != "SetCurrentProgramScene" {
		t.Fatalf("request type = %q", last.Type)
	}
	var params struct {
		SceneName string `json:"sceneName"`
	}
	if err := json.Unmarshal(last.Data, &params); err != nil || params.SceneName != "Live" {
		t.Errorf("params = %s (err %v)", last.Data, err)
	}
}

func TestCallSurfacesRequestErrors(t *testing.T) {
	srv := obstest.New("")
	defer srv.Close()
	srv.Handle("StartStream", func(json.RawMessage) (any, error) {
		return nil, errors.New("output already active")
	})
	client := dial(t, srv, "")
	err := client.StartStream(context.Background())
	var reqErr *obs.RequestError
	if !errors.As(err, &reqErr) || reqErr.RequestType != "StartStream" {
		t.Fatalf("err = %v, want RequestError for StartStream", err)
	}
}

func TestEventsAreDecodedAndDelivered(t *testing.T) {
	srv := obstest.New("")
	defer srv.Close()
	client := dial(t, srv, "")
	if err := srv.SendEvent("CurrentProgramSceneChanged", map[string]any{"sceneName": "Intro"}); err != nil {
		t.Fatalf("SendEvent: %v", err)
	}
	select {
	case ev := <-client.Events():
		changed, ok := ev.(*obs.CurrentProgramSceneChanged)
		if !ok || changed.SceneName != "Intro" {
			t.Fatalf("event = %#v", ev)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no event delivered")
	}
}

func TestUnknownEventsAreDropped(t *testing.T) {
	srv := obstest.New("")
	defer srv.Close()
	client := dial(t, srv, "")
	if err := srv.SendEvent("InputVolumeMeters", map[string]any{}); err != nil {
		t.Fatalf("SendEvent: %v", err)
	}
	if err := srv.SendEvent("StudioModeStateChanged", map[string]any{"studioModeEnabled": true}); err != nil {
		t.Fatalf("SendEvent: %v", err)
	}
	select {
	case ev := <-client.Events():
		if _, ok := ev.(*obs.StudioModeStateChanged); !ok {
			t.Fatalf("event = %#v, want StudioModeStateChanged (unknown event should be dropped)", ev)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no event delivered")
	}
}

func TestServerCloseClosesDoneAndEvents(t *testing.T) {
	srv := obstest.New("")
	client := dial(t, srv, "")
	srv.Close()
	select {
	case <-client.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("Done not closed")
	}
	if _, ok := <-client.Events(); ok {
		t.Fatal("events channel should be closed")
	}
	if err := client.Call(context.Background(), "GetVersion", nil, nil); err == nil {
		t.Fatal("Call after close should fail")
	}
}
