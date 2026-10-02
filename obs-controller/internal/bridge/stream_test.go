package bridge_test

import (
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/jacobgad/obs-controller/internal/testutil"
)

func TestMJPEGStreamServesFramesAtActiveRate(t *testing.T) {
	srv := newOBSServer(t)
	broker := testutil.NewFakeMQTT()
	b := startBridge(t, srv, broker)
	waitFor(t, "availability online", func() bool {
		return lastPayload(broker, "obs/test/availability") == "online"
	})

	resp, err := http.Get("http://" + b.HTTPAddr() + "/stream/test")
	if err != nil {
		t.Fatalf("GET /stream/test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	mediaType, params, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/x-mixed-replace" {
		t.Fatalf("content type = %q (%v)", resp.Header.Get("Content-Type"), err)
	}

	reader := multipart.NewReader(resp.Body, params["boundary"])
	for i := range 3 {
		part, err := reader.NextPart()
		if err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		if ct := part.Header.Get("Content-Type"); ct != "image/jpeg" {
			t.Errorf("frame %d content type = %q", i, ct)
		}
		data, err := io.ReadAll(part)
		if err != nil || string(data) != "hello" {
			t.Fatalf("frame %d = %q (%v)", i, data, err)
		}
	}
}

func TestSnapshotServesLatestFrame(t *testing.T) {
	srv := newOBSServer(t)
	broker := testutil.NewFakeMQTT()
	b := startBridge(t, srv, broker)
	waitFor(t, "availability online", func() bool {
		return lastPayload(broker, "obs/test/availability") == "online"
	})

	var body []byte
	waitFor(t, "snapshot available", func() bool {
		resp, err := http.Get("http://" + b.HTTPAddr() + "/snapshot/test")
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/jpeg" {
			return false
		}
		body, _ = io.ReadAll(resp.Body)
		return true
	})
	if string(body) != "hello" {
		t.Errorf("body = %q", body)
	}
}

func TestStreamUnknownConnectionIs404(t *testing.T) {
	srv := newOBSServer(t)
	broker := testutil.NewFakeMQTT()
	b := startBridge(t, srv, broker)

	for _, path := range []string{"/stream/nope", "/snapshot/nope"} {
		resp, err := http.Get("http://" + b.HTTPAddr() + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s status = %d, want 404", path, resp.StatusCode)
		}
	}
}
