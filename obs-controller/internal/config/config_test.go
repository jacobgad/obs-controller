package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

const minimalOptions = `{"connections": [{"name": "Main OBS", "host": "192.168.1.50"}]}`

func TestParseOptionsDefaults(t *testing.T) {
	opts, err := ParseOptions([]byte(minimalOptions))
	if err != nil {
		t.Fatalf("ParseOptions: %v", err)
	}
	if len(opts.Connections) != 1 {
		t.Fatalf("connections = %d, want 1", len(opts.Connections))
	}
	conn := opts.Connections[0]
	if conn.ID != "main_obs" || conn.Name != "Main OBS" || conn.Host != "192.168.1.50" || conn.Port != 4455 || conn.Password != "" {
		t.Errorf("connection = %+v", conn)
	}
	if conn.Addr() != "192.168.1.50:4455" {
		t.Errorf("Addr() = %q", conn.Addr())
	}
	if opts.ScreenshotActive != 2*time.Second || opts.ScreenshotIdle != 10*time.Second {
		t.Errorf("screenshot intervals = %v/%v", opts.ScreenshotActive, opts.ScreenshotIdle)
	}
	if opts.ScreenshotWidth != 640 || opts.ScreenshotQuality != 60 {
		t.Errorf("screenshot params = %d/%d", opts.ScreenshotWidth, opts.ScreenshotQuality)
	}
	if opts.LogLevel != slog.LevelInfo {
		t.Errorf("log level = %v", opts.LogLevel)
	}
}

func TestParseOptionsExplicitValues(t *testing.T) {
	data := `{
		"connections": [{"name": "Studio", "id": "studio_rig", "host": "obs.local", "port": 4466, "password": "secret"}],
		"screenshot_interval_active_seconds": 1,
		"screenshot_interval_idle_seconds": 20,
		"screenshot_width": 1280,
		"screenshot_quality": 80,
		"log_level": "debug"
	}`
	opts, err := ParseOptions([]byte(data))
	if err != nil {
		t.Fatalf("ParseOptions: %v", err)
	}
	conn := opts.Connections[0]
	if conn.ID != "studio_rig" || conn.Port != 4466 || conn.Password != "secret" {
		t.Errorf("connection = %+v", conn)
	}
	if opts.ScreenshotActive != time.Second {
		t.Errorf("screenshot active = %v", opts.ScreenshotActive)
	}
	if opts.ScreenshotWidth != 1280 || opts.ScreenshotQuality != 80 {
		t.Errorf("screenshot params = %d/%d", opts.ScreenshotWidth, opts.ScreenshotQuality)
	}
	if opts.LogLevel != slog.LevelDebug {
		t.Errorf("log level = %v", opts.LogLevel)
	}
}

func TestParseOptionsRejections(t *testing.T) {
	cases := map[string]string{
		"no connections":    `{"connections": []}`,
		"missing name":      `{"connections": [{"host": "h"}]}`,
		"unsluggable name":  `{"connections": [{"name": "測試", "host": "h"}]}`,
		"bad explicit id":   `{"connections": [{"name": "A", "id": "Main OBS", "host": "h"}]}`,
		"missing host":      `{"connections": [{"name": "A"}]}`,
		"bad port":          `{"connections": [{"name": "A", "host": "h", "port": 70000}]}`,
		"duplicate ids":     `{"connections": [{"name": "Main OBS", "host": "h"}, {"name": "main-obs", "host": "i"}]}`,
		"bad poll interval": minimalish(`"screenshot_interval_active_seconds": 0`),
		"bad width":         minimalish(`"screenshot_width": 5000`),
		"bad quality":       minimalish(`"screenshot_quality": 101`),
		"bad log level":     minimalish(`"log_level": "verbose"`),
		"not json":          `nope`,
	}
	for name, data := range cases {
		if _, err := ParseOptions([]byte(data)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func minimalish(extra string) string {
	return strings.TrimSuffix(minimalOptions, "}") + ", " + extra + "}"
}

func TestMQTTStringNeverLeaksPassword(t *testing.T) {
	m := MQTT{Host: "broker", Port: 1883, Username: "user", Password: "hunter2"}
	for _, rendered := range []string{m.String(), m.GoString()} {
		if strings.Contains(rendered, "hunter2") {
			t.Errorf("rendered %q leaks password", rendered)
		}
	}
}

func TestConnectionLogValueNeverLeaksPassword(t *testing.T) {
	c := Connection{ID: "a", Host: "h", Port: 4455, Password: "hunter2"}
	if strings.Contains(c.LogValue().String(), "hunter2") {
		t.Error("LogValue leaks password")
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Main OBS":              "main_obs",
		"  Studio -- 2  ":       "studio_2",
		"café corner":           "caf_corner",
		"ALLCAPS":               "allcaps",
		"測試":                    "",
		strings.Repeat("a", 40): strings.Repeat("a", 32),
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}
