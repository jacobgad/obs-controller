// Package config loads add-on options from /data/options.json and the MQTT broker
// details from either the environment or the Home Assistant Supervisor.
package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Connection is one configured OBS instance. Name is the Home Assistant device name;
// ID is the stable identity every MQTT topic and unique_id derives from.
type Connection struct {
	ID       string
	Name     string
	Host     string
	Port     int
	Password string
}

// Addr is the host:port the obs-websocket client dials.
func (c Connection) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// LogValue renders the connection for slog without the password.
func (c Connection) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("id", c.ID),
		slog.String("host", c.Host),
		slog.Int("port", c.Port),
	)
}

// Options are the validated add-on options.
type Options struct {
	Connections       []Connection
	ScreenshotActive  time.Duration
	ScreenshotIdle    time.Duration
	ScreenshotWidth   int
	ScreenshotQuality int
	LogLevel          slog.Level
}

// MQTT is how to reach the broker.
type MQTT struct {
	Host     string
	Port     int
	Username string
	Password string
	TLS      bool
}

// String renders the broker address without the password, so formatting an MQTT
// value (or any struct containing one) can never leak the credential into logs.
func (m MQTT) String() string {
	return fmt.Sprintf("mqtt://%s@%s:%d tls=%v", m.Username, m.Host, m.Port, m.TLS)
}

// GoString mirrors String for %#v, which bypasses Stringer.
func (m MQTT) GoString() string { return m.String() }

// LogValue renders the broker details for slog without the password.
func (m MQTT) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("host", m.Host),
		slog.Int("port", m.Port),
		slog.String("username", m.Username),
		slog.Bool("tls", m.TLS),
	)
}

// Config is everything the binary needs to start.
type Config struct {
	Options Options
	MQTT    MQTT
}

type rawConnection struct {
	Name     string `json:"name"`
	ID       string `json:"id"`
	Host     string `json:"host"`
	Port     *int   `json:"port"`
	Password string `json:"password"`
}

type rawOptions struct {
	Connections       []rawConnection `json:"connections"`
	ShotActiveSecs    *int            `json:"screenshot_interval_active_seconds"`
	ShotIdleSecs      *int            `json:"screenshot_interval_idle_seconds"`
	ScreenshotWidth   *int            `json:"screenshot_width"`
	ScreenshotQuality *int            `json:"screenshot_quality"`
	LogLevel          *string         `json:"log_level"`
}

var idPattern = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)

// ParseOptions validates the JSON contents of options.json and applies defaults.
func ParseOptions(data []byte) (Options, error) {
	var raw rawOptions
	if err := json.Unmarshal(data, &raw); err != nil {
		return Options{}, fmt.Errorf("options are not valid JSON: %w", err)
	}
	if len(raw.Connections) == 0 {
		return Options{}, errors.New("connections: configure at least one OBS connection")
	}
	opts := Options{
		ScreenshotActive:  2 * time.Second,
		ScreenshotIdle:    10 * time.Second,
		ScreenshotWidth:   640,
		ScreenshotQuality: 60,
		LogLevel:          slog.LevelInfo,
	}
	seen := map[string]bool{}
	for i, rc := range raw.Connections {
		conn, err := parseConnection(rc)
		if err != nil {
			return Options{}, fmt.Errorf("connections[%d]: %w", i, err)
		}
		if seen[conn.ID] {
			return Options{}, fmt.Errorf("connections: duplicate id %q (ids derive from names unless set explicitly)", conn.ID)
		}
		seen[conn.ID] = true
		opts.Connections = append(opts.Connections, conn)
	}
	intervals := []struct {
		name string
		raw  *int
		dst  *time.Duration
	}{
		{"screenshot_interval_active_seconds", raw.ShotActiveSecs, &opts.ScreenshotActive},
		{"screenshot_interval_idle_seconds", raw.ShotIdleSecs, &opts.ScreenshotIdle},
	}
	for _, iv := range intervals {
		if iv.raw == nil {
			continue
		}
		if v := *iv.raw; v < 1 || v > 3600 {
			return Options{}, fmt.Errorf("%s must be between 1 and 3600", iv.name)
		}
		*iv.dst = time.Duration(*iv.raw) * time.Second
	}
	if raw.ScreenshotWidth != nil {
		if v := *raw.ScreenshotWidth; v < 8 || v > 4096 {
			return Options{}, errors.New("screenshot_width must be between 8 and 4096")
		}
		opts.ScreenshotWidth = *raw.ScreenshotWidth
	}
	if raw.ScreenshotQuality != nil {
		if v := *raw.ScreenshotQuality; v < 0 || v > 100 {
			return Options{}, errors.New("screenshot_quality must be between 0 and 100")
		}
		opts.ScreenshotQuality = *raw.ScreenshotQuality
	}
	if raw.LogLevel != nil {
		if err := opts.LogLevel.UnmarshalText([]byte(*raw.LogLevel)); err != nil {
			return Options{}, errors.New("log_level must be one of debug, info, warn, error")
		}
	}
	return opts, nil
}

func parseConnection(rc rawConnection) (Connection, error) {
	name := strings.TrimSpace(rc.Name)
	if name == "" {
		return Connection{}, errors.New("name is required")
	}
	id := rc.ID
	if id == "" {
		id = Slugify(name)
		if id == "" {
			return Connection{}, fmt.Errorf("name %q yields an empty id; set id explicitly", name)
		}
	}
	if !idPattern.MatchString(id) {
		return Connection{}, fmt.Errorf("id %q must match %s", id, idPattern)
	}
	if strings.TrimSpace(rc.Host) == "" {
		return Connection{}, errors.New("host is required")
	}
	conn := Connection{ID: id, Name: name, Host: strings.TrimSpace(rc.Host), Port: 4455, Password: rc.Password}
	if rc.Port != nil {
		if v := *rc.Port; v < 1 || v > 65535 {
			return Connection{}, fmt.Errorf("port %d is not a valid port", v)
		}
		conn.Port = *rc.Port
	}
	return conn, nil
}

const maxIDLength = 32

// Slugify derives a connection id from a display name: lowercased, runs of anything
// outside [a-z0-9] become single underscores, trimmed to the id length limit.
func Slugify(name string) string {
	var b strings.Builder
	underscore := false
	for _, r := range strings.ToLower(name) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			if underscore && b.Len() > 0 {
				b.WriteByte('_')
			}
			underscore = false
			b.WriteRune(r)
		default:
			underscore = true
		}
	}
	slug := b.String()
	if len(slug) > maxIDLength {
		slug = strings.Trim(slug[:maxIDLength], "_")
	}
	return slug
}

// MQTTFromEnv reads MQTT_HOST, MQTT_PORT, MQTT_USERNAME, MQTT_PASSWORD and MQTT_SSL.
func MQTTFromEnv(getenv func(string) string) (MQTT, error) {
	host := getenv("MQTT_HOST")
	if host == "" {
		return MQTT{}, errors.New("MQTT_HOST is required")
	}
	m := MQTT{Host: host, Port: 1883, Username: getenv("MQTT_USERNAME"), Password: getenv("MQTT_PASSWORD")}
	if p := getenv("MQTT_PORT"); p != "" {
		port, err := strconv.Atoi(p)
		if err != nil || port < 1 || port > 65535 {
			return MQTT{}, fmt.Errorf("MQTT_PORT %q is not a valid port", p)
		}
		m.Port = port
	}
	switch strings.ToLower(getenv("MQTT_SSL")) {
	case "true", "1", "yes":
		m.TLS = true
	}
	return m, nil
}

const supervisorServicesURL = "http://supervisor/services/mqtt"

// MQTTFromSupervisor fetches the broker registered with the Supervisor services API,
// which is reachable without hassio_api once the add-on declares the mqtt service.
func MQTTFromSupervisor(ctx context.Context, token string, client *http.Client) (MQTT, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, supervisorServicesURL, nil)
	if err != nil {
		return MQTT{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return MQTT{}, fmt.Errorf("supervisor request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusOK {
		return MQTT{}, fmt.Errorf("supervisor returned HTTP %d for the MQTT service", resp.StatusCode)
	}
	var payload struct {
		Result string `json:"result"`
		Data   struct {
			Host     string `json:"host"`
			Port     int    `json:"port"`
			Username string `json:"username"`
			Password string `json:"password"`
			SSL      bool   `json:"ssl"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.Result != "ok" || payload.Data.Host == "" {
		return MQTT{}, errors.New("supervisor did not return a usable MQTT service; is the Mosquitto broker add-on running?")
	}
	return MQTT{Host: payload.Data.Host, Port: payload.Data.Port, Username: payload.Data.Username, Password: payload.Data.Password, TLS: payload.Data.SSL}, nil
}

// Load reads options.json and resolves MQTT settings, preferring MQTT_HOST when set.
func Load(ctx context.Context) (Config, error) {
	optionsPath := envOr("OBS_CONTROLLER_OPTIONS_PATH", "/data/options.json")
	data, err := os.ReadFile(optionsPath) //nolint:gosec // path is fixed by the add-on or set by the operator's own environment
	if err != nil {
		return Config{}, fmt.Errorf("read %s: %w", optionsPath, err)
	}
	opts, err := ParseOptions(data)
	if err != nil {
		return Config{}, err
	}
	var mqtt MQTT
	if os.Getenv("MQTT_HOST") != "" {
		mqtt, err = MQTTFromEnv(os.Getenv)
	} else if token := os.Getenv("SUPERVISOR_TOKEN"); token != "" {
		mqtt, err = MQTTFromSupervisor(ctx, token, &http.Client{Timeout: 10 * time.Second})
	} else {
		err = errors.New("no MQTT configuration: set MQTT_HOST or run under the Home Assistant Supervisor")
	}
	if err != nil {
		return Config{}, err
	}
	return Config{Options: opts, MQTT: mqtt}, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
