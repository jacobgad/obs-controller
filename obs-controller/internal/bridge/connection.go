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

	"github.com/jacobgad/obs-controller/internal/config"
	"github.com/jacobgad/obs-controller/internal/mqtt"
	"github.com/jacobgad/obs-controller/internal/obs"
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
	client *obs.Client
	st     obsState

	frameMu    sync.Mutex
	subs       map[chan []byte]struct{}
	subsClosed bool
	latest     []byte
	viewerCh   chan struct{}
}

func newConnection(cfg config.Connection, deps Deps) *connection {
	return &connection{
		cfg:      cfg,
		opts:     deps.Options,
		broker:   deps.MQTT,
		log:      deps.Log.With("connection", cfg.ID),
		origin:   deps.Origin,
		topics:   mqtt.ForDevice(cfg.ID),
		subs:     make(map[chan []byte]struct{}),
		viewerCh: make(chan struct{}, 1),
	}
}

func (c *connection) run(ctx context.Context) {
	defer c.closeSubs()
	c.publishDown(ctx)
	backoff := initialBackoff
	for ctx.Err() == nil {
		client, err := c.dial(ctx)
		if err != nil {
			if errors.Is(err, obs.ErrAuthFailed) {
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
			_ = client.Close()
			sleepCtx(ctx, jitter(backoff))
			continue
		}
		c.loop(ctx, client)
		c.onDown(ctx)
	}
}

func (c *connection) dial(ctx context.Context) (*obs.Client, error) {
	return obs.Dial(ctx, obs.Options{
		Addr:             c.cfg.Addr(),
		Password:         c.cfg.Password,
		HandshakeTimeout: handshakeTimeout,
		EventSubscriptions: obs.SubGeneral | obs.SubConfig | obs.SubScenes |
			obs.SubTransitions | obs.SubOutputs | obs.SubUI,
	})
}

func (c *connection) onUp(ctx context.Context, client *obs.Client) error {
	st := obsState{}
	version, err := client.GetVersion(ctx)
	if err != nil {
		return err
	}
	st.obsVersion = version.ObsVersion
	st.platform = version.Platform
	st.imageFormat = pickImageFormat(version.SupportedImageFormats)
	if major := wsMajorVersion(version.ObsWebSocketVersion); major > 0 && major < minWSMajor {
		c.log.Warn("obs_websocket_version_unsupported", "version", version.ObsWebSocketVersion, "minimum", "5.0.0")
	}

	video, err := client.GetVideoSettings(ctx)
	if err != nil {
		return err
	}
	st.canvasWidth, st.canvasHeight = video.BaseWidth, video.BaseHeight

	sceneList, err := client.GetSceneList(ctx)
	if err != nil {
		return err
	}
	st.scenes = sceneNames(sceneList.Scenes)
	st.programScene = sceneList.CurrentProgramSceneName
	st.previewScene = sceneList.CurrentPreviewSceneName

	transitionList, err := client.GetSceneTransitionList(ctx)
	if err != nil {
		return err
	}
	st.transitions = transitionNames(transitionList.Transitions)
	st.transition = transitionList.CurrentSceneTransitionName

	current, err := client.GetCurrentSceneTransition(ctx)
	if err != nil {
		return err
	}
	st.durationMs = int(current.TransitionDuration)

	studio, err := client.GetStudioModeEnabled(ctx)
	if err != nil {
		return err
	}
	st.studioMode = studio.StudioModeEnabled

	stream, err := client.GetStreamStatus(ctx)
	if err != nil {
		return err
	}
	st.streaming = stream.OutputActive

	record, err := client.GetRecordStatus(ctx)
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

func (c *connection) loop(ctx context.Context, client *obs.Client) {
	timer := time.NewTimer(c.captureInterval())
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = client.Close()
			return
		case ev, ok := <-client.Events():
			if !ok {
				return
			}
			c.handleEvent(ctx, ev)
			continue
		case <-c.viewerCh:
		case <-timer.C:
		}
		c.capture(ctx)
		timer.Reset(c.captureInterval())
	}
}

func (c *connection) captureInterval() time.Duration {
	c.frameMu.Lock()
	defer c.frameMu.Unlock()
	if len(c.subs) > 0 {
		return c.opts.ActivePollInterval
	}
	return c.opts.IdlePollInterval
}

// A viewer's presence is what switches capture to the active rate.
func (c *connection) subscribe() chan []byte {
	ch := make(chan []byte, 1)
	c.frameMu.Lock()
	if c.subsClosed {
		c.frameMu.Unlock()
		close(ch)
		return ch
	}
	c.subs[ch] = struct{}{}
	viewers := len(c.subs)
	if c.latest != nil {
		ch <- c.latest
	}
	c.frameMu.Unlock()
	select {
	case c.viewerCh <- struct{}{}:
	default:
	}
	c.log.Info("mjpeg_viewer_connected", "viewers", viewers)
	return ch
}

func (c *connection) unsubscribe(ch chan []byte) {
	c.frameMu.Lock()
	if _, ok := c.subs[ch]; !ok {
		c.frameMu.Unlock()
		return
	}
	delete(c.subs, ch)
	viewers := len(c.subs)
	c.frameMu.Unlock()
	c.log.Info("mjpeg_viewer_disconnected", "viewers", viewers)
}

// closeSubs ends every open MJPEG stream at shutdown, so HTTP Shutdown does not sit
// on its full deadline waiting for streams that never finish on their own.
func (c *connection) closeSubs() {
	c.frameMu.Lock()
	defer c.frameMu.Unlock()
	c.subsClosed = true
	for ch := range c.subs {
		close(ch)
	}
	c.subs = map[chan []byte]struct{}{}
}

func (c *connection) latestFrame() []byte {
	c.frameMu.Lock()
	defer c.frameMu.Unlock()
	return c.latest
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
	case *obs.StreamStateChanged:
		c.update(func(st *obsState) { st.streaming = e.OutputActive })
		c.pub(ctx, c.topics.StreamState, onOff(e.OutputActive))
	case *obs.RecordStateChanged:
		c.handleRecordState(ctx, e)
	case *obs.CurrentProgramSceneChanged:
		c.update(func(st *obsState) { st.programScene = e.SceneName })
		c.pub(ctx, c.topics.ProgramSceneState, e.SceneName)
		c.capture(ctx)
	case *obs.CurrentPreviewSceneChanged:
		c.update(func(st *obsState) { st.previewScene = e.SceneName })
		c.pub(ctx, c.topics.PreviewSceneState, e.SceneName)
	case *obs.SceneListChanged:
		c.update(func(st *obsState) { st.scenes = sceneNames(e.Scenes) })
		c.publishSceneSelects(ctx)
	case *obs.SceneNameChanged:
		c.handleSceneRenamed(ctx, e)
	case *obs.StudioModeStateChanged:
		c.handleStudioMode(ctx, e)
	case *obs.CurrentSceneTransitionChanged:
		c.handleTransitionChanged(ctx, e)
	case *obs.CurrentSceneTransitionDurationChanged:
		c.update(func(st *obsState) { st.durationMs = int(e.TransitionDuration) })
		c.pub(ctx, c.topics.TransitionDurationState, strconv.Itoa(int(e.TransitionDuration)))
	case *obs.ExitStarted:
		c.log.Info("obs_exiting")
	}
}

func (c *connection) handleRecordState(ctx context.Context, e *obs.RecordStateChanged) {
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

func (c *connection) handleSceneRenamed(ctx context.Context, e *obs.SceneNameChanged) {
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

func (c *connection) handleStudioMode(ctx context.Context, e *obs.StudioModeStateChanged) {
	preview := ""
	if e.StudioModeEnabled {
		if client := c.snapshotClient(); client != nil {
			if resp, err := client.GetCurrentPreviewScene(ctx); err == nil {
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

func (c *connection) handleTransitionChanged(ctx context.Context, e *obs.CurrentSceneTransitionChanged) {
	c.update(func(st *obsState) { st.transition = e.TransitionName })
	st := c.snapshotState()
	if !contains(st.transitions, e.TransitionName) {
		if client := c.snapshotClient(); client != nil {
			if resp, err := client.GetSceneTransitionList(ctx); err == nil {
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

func (c *connection) snapshotClient() *obs.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.client
}

func sceneNames(scenes []obs.Scene) []string {
	names := make([]string, 0, len(scenes))
	// OBS lists scenes bottom-up; reversing matches the order shown in the OBS UI.
	for i := len(scenes) - 1; i >= 0; i-- {
		names = append(names, scenes[i].SceneName)
	}
	return names
}

func transitionNames(transitions []obs.Transition) []string {
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
