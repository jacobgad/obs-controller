package bridge

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/andreykaipov/goobs"
	"github.com/andreykaipov/goobs/api/closecodes"
	"github.com/andreykaipov/goobs/api/events"
	"github.com/andreykaipov/goobs/api/events/subscriptions"
	"github.com/andreykaipov/goobs/api/typedefs"
	"github.com/gorilla/websocket"
	"github.com/jacobgad/obs-controller/internal/config"
	"github.com/jacobgad/obs-controller/internal/mqtt"
)

const (
	initialBackoff   = time.Second
	maxBackoff       = 30 * time.Second
	authRetryDelay   = 60 * time.Second
	handshakeTimeout = 10 * time.Second
	publishTimeout   = 10 * time.Second
	minWSMajor       = 5
)

type obsState struct {
	obsVersion   string
	platform     string
	imageFormat  string
	canvasWidth  float64
	canvasHeight float64
	scenes       []string
	transitions  []string
	streaming    bool
	recording    bool
	recordPaused bool
	studioMode   bool
	programScene string
	previewScene string
	transition   string
	durationMs   int
}

type connection struct {
	cfg    config.Connection
	opts   config.Options
	broker mqtt.Connection
	log    *slog.Logger
	origin mqtt.Origin
	topics mqtt.DeviceTopics

	mu     sync.Mutex
	client *goobs.Client
	st     obsState
}

func newConnection(cfg config.Connection, deps Deps) *connection {
	return &connection{
		cfg:    cfg,
		opts:   deps.Options,
		broker: deps.MQTT,
		log:    deps.Log.With("connection", cfg.ID),
		origin: deps.Origin,
		topics: mqtt.ForDevice(cfg.ID),
	}
}

func (c *connection) run(ctx context.Context) {
	c.publishDown(ctx)
	backoff := initialBackoff
	for ctx.Err() == nil {
		client, err := c.dial()
		if err != nil {
			if isAuthFailure(err) {
				c.log.Error("obs_auth_failed", "host", c.cfg.Host, "port", c.cfg.Port,
					"detail", "check the obs-websocket password for this connection")
				sleepCtx(ctx, authRetryDelay)
			} else {
				c.log.Warn("obs_connect_failed", "host", c.cfg.Host, "port", c.cfg.Port, "error", err.Error())
				sleepCtx(ctx, jitter(backoff))
				backoff = min(backoff*2, maxBackoff)
			}
			continue
		}
		backoff = initialBackoff
		if err := c.onUp(ctx, client); err != nil {
			c.log.Warn("obs_init_failed", "error", err.Error())
			_ = client.Disconnect()
			sleepCtx(ctx, jitter(backoff))
			continue
		}
		c.loop(ctx, client)
		c.onDown(ctx)
	}
}

func (c *connection) dial() (*goobs.Client, error) {
	dialer := *websocket.DefaultDialer
	dialer.HandshakeTimeout = handshakeTimeout
	opts := []goobs.Option{
		goobs.WithDialer(&dialer),
		goobs.WithEventSubscriptions(subscriptions.General | subscriptions.Config |
			subscriptions.Scenes | subscriptions.Transitions | subscriptions.Outputs | subscriptions.Ui),
	}
	if c.cfg.Password != "" {
		opts = append(opts, goobs.WithPassword(c.cfg.Password))
	}
	return goobs.New(c.cfg.Addr(), opts...)
}

func isAuthFailure(err error) bool {
	var closeErr *websocket.CloseError
	return errors.As(err, &closeErr) && closeErr.Code == closecodes.AuthenticationFailed
}

func (c *connection) onUp(ctx context.Context, client *goobs.Client) error {
	st := obsState{}
	version, err := client.General.GetVersion()
	if err != nil {
		return err
	}
	st.obsVersion = version.ObsVersion
	st.platform = version.Platform
	st.imageFormat = pickImageFormat(version.SupportedImageFormats)
	if major := wsMajorVersion(version.ObsWebSocketVersion); major > 0 && major < minWSMajor {
		c.log.Warn("obs_websocket_version_unsupported", "version", version.ObsWebSocketVersion, "minimum", "5.0.0")
	}

	video, err := client.Config.GetVideoSettings()
	if err != nil {
		return err
	}
	st.canvasWidth, st.canvasHeight = video.BaseWidth, video.BaseHeight

	sceneList, err := client.Scenes.GetSceneList()
	if err != nil {
		return err
	}
	st.scenes = sceneNames(sceneList.Scenes)
	st.programScene = sceneList.CurrentProgramSceneName
	st.previewScene = sceneList.CurrentPreviewSceneName

	transitionList, err := client.Transitions.GetSceneTransitionList()
	if err != nil {
		return err
	}
	st.transitions = transitionNames(transitionList.Transitions)
	st.transition = transitionList.CurrentSceneTransitionName

	current, err := client.Transitions.GetCurrentSceneTransition()
	if err != nil {
		return err
	}
	st.durationMs = int(current.TransitionDuration)

	studio, err := client.Ui.GetStudioModeEnabled()
	if err != nil {
		return err
	}
	st.studioMode = studio.StudioModeEnabled

	stream, err := client.Stream.GetStreamStatus()
	if err != nil {
		return err
	}
	st.streaming = stream.OutputActive

	record, err := client.Record.GetRecordStatus()
	if err != nil {
		return err
	}
	st.recording = record.OutputActive
	st.recordPaused = record.OutputPaused

	c.mu.Lock()
	c.client = client
	c.st = st
	c.mu.Unlock()

	c.log.Info("obs_connected", "host", c.cfg.Host, "port", c.cfg.Port,
		"obs_version", version.ObsVersion, "websocket_version", version.ObsWebSocketVersion)
	c.publishUp(ctx)
	return nil
}

func (c *connection) loop(ctx context.Context, client *goobs.Client) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	lastPoll, lastShot := time.Now(), time.Now()
	for {
		select {
		case <-ctx.Done():
			_ = client.Disconnect()
			return
		case ev, ok := <-client.IncomingEvents:
			if !ok {
				return
			}
			c.handleEvent(ctx, ev)
		case now := <-ticker.C:
			active := c.active()
			if now.Sub(lastPoll) >= interval(active, c.opts.PollActive, c.opts.PollIdle) {
				lastPoll = now
				c.pollSensors(ctx)
			}
			if now.Sub(lastShot) >= interval(active, c.opts.ScreenshotActive, c.opts.ScreenshotIdle) {
				lastShot = now
				c.publishScreenshot(ctx)
			}
		}
	}
}

func (c *connection) onDown(ctx context.Context) {
	c.mu.Lock()
	c.client = nil
	c.mu.Unlock()
	c.log.Warn("obs_disconnected", "host", c.cfg.Host, "port", c.cfg.Port)
	c.publishDown(context.WithoutCancel(ctx))
}

func (c *connection) handleEvent(ctx context.Context, ev any) {
	switch e := ev.(type) {
	case *events.StreamStateChanged:
		c.update(func(st *obsState) { st.streaming = e.OutputActive })
		c.pub(ctx, c.topics.StreamState, onOff(e.OutputActive))
		c.pollSensors(ctx)
	case *events.RecordStateChanged:
		c.handleRecordState(ctx, e)
	case *events.CurrentProgramSceneChanged:
		c.update(func(st *obsState) { st.programScene = e.SceneName })
		c.pub(ctx, c.topics.ProgramSceneState, e.SceneName)
		c.publishScreenshot(ctx)
	case *events.CurrentPreviewSceneChanged:
		c.update(func(st *obsState) { st.previewScene = e.SceneName })
		c.pub(ctx, c.topics.PreviewSceneState, e.SceneName)
	case *events.SceneListChanged:
		c.update(func(st *obsState) { st.scenes = sceneNames(e.Scenes) })
		c.publishSceneSelects(ctx)
	case *events.SceneNameChanged:
		c.handleSceneRenamed(ctx, e)
	case *events.StudioModeStateChanged:
		c.handleStudioMode(ctx, e)
	case *events.CurrentSceneTransitionChanged:
		c.handleTransitionChanged(ctx, e)
	case *events.CurrentSceneTransitionDurationChanged:
		c.update(func(st *obsState) { st.durationMs = int(e.TransitionDuration) })
		c.pub(ctx, c.topics.TransitionDurationState, strconv.Itoa(int(e.TransitionDuration)))
	case *events.ExitStarted:
		c.log.Info("obs_exiting")
	}
}

func (c *connection) handleRecordState(ctx context.Context, e *events.RecordStateChanged) {
	switch e.OutputState {
	case "OBS_WEBSOCKET_OUTPUT_STARTED":
		c.update(func(st *obsState) { st.recording, st.recordPaused = true, false })
	case "OBS_WEBSOCKET_OUTPUT_STOPPED":
		c.update(func(st *obsState) { st.recording, st.recordPaused = false, false })
		if e.OutputPath != "" {
			c.pub(ctx, c.topics.SensorState("last_recording_path"), e.OutputPath)
		}
	case "OBS_WEBSOCKET_OUTPUT_PAUSED":
		c.update(func(st *obsState) { st.recordPaused = true })
	case "OBS_WEBSOCKET_OUTPUT_RESUMED":
		c.update(func(st *obsState) { st.recordPaused = false })
	default:
		return
	}
	st := c.snapshotState()
	c.pub(ctx, c.topics.RecordState, onOff(st.recording))
	c.pub(ctx, c.topics.RecordPausedState, onOff(st.recordPaused))
}

func (c *connection) handleSceneRenamed(ctx context.Context, e *events.SceneNameChanged) {
	c.update(func(st *obsState) {
		if st.programScene == e.OldSceneName {
			st.programScene = e.SceneName
		}
		if st.previewScene == e.OldSceneName {
			st.previewScene = e.SceneName
		}
	})
	st := c.snapshotState()
	c.pub(ctx, c.topics.ProgramSceneState, st.programScene)
	c.pub(ctx, c.topics.PreviewSceneState, st.previewScene)
}

func (c *connection) handleStudioMode(ctx context.Context, e *events.StudioModeStateChanged) {
	preview := ""
	if e.StudioModeEnabled {
		if client := c.snapshotClient(); client != nil {
			if resp, err := client.Scenes.GetCurrentPreviewScene(); err == nil {
				preview = resp.CurrentPreviewSceneName
			}
		}
	}
	c.update(func(st *obsState) {
		st.studioMode = e.StudioModeEnabled
		st.previewScene = preview
	})
	c.pub(ctx, c.topics.StudioModeState, onOff(e.StudioModeEnabled))
	c.pub(ctx, c.topics.PreviewSceneState, preview)
}

func (c *connection) handleTransitionChanged(ctx context.Context, e *events.CurrentSceneTransitionChanged) {
	c.update(func(st *obsState) { st.transition = e.TransitionName })
	st := c.snapshotState()
	if !contains(st.transitions, e.TransitionName) {
		if client := c.snapshotClient(); client != nil {
			if resp, err := client.Transitions.GetSceneTransitionList(); err == nil {
				c.update(func(st *obsState) { st.transitions = transitionNames(resp.Transitions) })
				c.publishTransitionSelect(ctx)
			}
		}
	}
	c.pub(ctx, c.topics.TransitionState, e.TransitionName)
}

func (c *connection) update(fn func(*obsState)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	fn(&c.st)
}

func (c *connection) snapshotState() obsState {
	c.mu.Lock()
	defer c.mu.Unlock()
	st := c.st
	st.scenes = append([]string{}, c.st.scenes...)
	st.transitions = append([]string{}, c.st.transitions...)
	return st
}

func (c *connection) snapshotClient() *goobs.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.client
}

func (c *connection) active() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.st.streaming || c.st.recording
}

func interval(active bool, activeInterval, idleInterval time.Duration) time.Duration {
	if active {
		return activeInterval
	}
	return idleInterval
}

func sceneNames(scenes []*typedefs.Scene) []string {
	names := make([]string, 0, len(scenes))
	// OBS lists scenes bottom-up; reversing matches the order shown in the OBS UI.
	for i := len(scenes) - 1; i >= 0; i-- {
		names = append(names, scenes[i].SceneName)
	}
	return names
}

func transitionNames(transitions []*typedefs.Transition) []string {
	names := make([]string, 0, len(transitions))
	for _, t := range transitions {
		names = append(names, t.TransitionName)
	}
	return names
}

func pickImageFormat(supported []string) string {
	for _, want := range []string{"jpg", "jpeg", "png"} {
		if contains(supported, want) {
			return want
		}
	}
	return "png"
}

func wsMajorVersion(version string) int {
	head, _, _ := strings.Cut(version, ".")
	major, err := strconv.Atoi(head)
	if err != nil {
		return 0
	}
	return major
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

func onOff(v bool) string {
	if v {
		return mqtt.PayloadOn
	}
	return mqtt.PayloadOff
}

func jitter(d time.Duration) time.Duration {
	return time.Duration(float64(d) * (0.8 + 0.4*rand.Float64())) //nolint:gosec // backoff jitter needs no cryptographic randomness
}

func sleepCtx(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
