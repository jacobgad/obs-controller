package bridge

import (
	"context"
	"encoding/base64"
	"errors"
	"math"
	"strconv"
	"strings"

	"github.com/jacobgad/obs-controller/internal/mqtt"
	"github.com/jacobgad/obs-controller/internal/obs"
)

func (c *connection) pub(ctx context.Context, topic, payload string) {
	c.pubBytes(ctx, topic, []byte(payload), true)
}

func (c *connection) pubBytes(ctx context.Context, topic string, payload []byte, retain bool) {
	ctx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()
	err := c.broker.Publish(ctx, topic, payload, retain)
	if err != nil && !errors.Is(err, mqtt.ErrNotConnected) {
		c.log.Warn("mqtt_publish_failed", "topic", topic, "error", err.Error())
	}
}

func (c *connection) deviceInfo() mqtt.DeviceInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	return mqtt.DeviceInfo{ID: c.cfg.ID, Name: c.cfg.Name, OBSVersion: c.st.obsVersion, Platform: c.st.platform}
}

// republish refreshes retained topics after an MQTT reconnect.
func (c *connection) republish(ctx context.Context) {
	if c.snapshotClient() != nil {
		c.publishUp(ctx)
	} else {
		c.publishDown(ctx)
	}
}

func (c *connection) publishUp(ctx context.Context) {
	st := c.snapshotState()
	info := c.deviceInfo()
	for _, msg := range mqtt.DeviceMessages(info, st.scenes, st.transitions, c.origin) {
		c.pubBytes(ctx, msg.Topic, msg.JSON(), true)
	}
	c.pub(ctx, c.topics.Availability, mqtt.PayloadOnline)
	c.pub(ctx, c.topics.ConnectedState, mqtt.PayloadOn)
	c.pub(ctx, c.topics.StreamState, onOff(st.streaming))
	c.pub(ctx, c.topics.RecordState, onOff(st.recording))
	c.pub(ctx, c.topics.RecordPausedState, onOff(st.recordPaused))
	c.pub(ctx, c.topics.StudioModeState, onOff(st.studioMode))
	c.pub(ctx, c.topics.ProgramSceneState, st.programScene)
	c.pub(ctx, c.topics.PreviewSceneState, st.previewScene)
	c.pub(ctx, c.topics.TransitionState, st.transition)
	c.pub(ctx, c.topics.TransitionDurationState, strconv.Itoa(st.durationMs))
	for _, topic := range mqtt.RetiredDiscoveryTopics(c.cfg.ID) {
		c.pubBytes(ctx, topic, nil, true)
	}
	c.publishScreenshot(ctx)
}

func (c *connection) publishDown(ctx context.Context) {
	c.pub(ctx, c.topics.Availability, mqtt.PayloadOffline)
	c.pub(ctx, c.topics.ConnectedState, mqtt.PayloadOff)
}

func (c *connection) publishSceneSelects(ctx context.Context) {
	st := c.snapshotState()
	for _, msg := range mqtt.SceneSelectMessages(c.deviceInfo(), st.scenes, c.origin) {
		c.pubBytes(ctx, msg.Topic, msg.JSON(), true)
	}
}

func (c *connection) publishTransitionSelect(ctx context.Context) {
	st := c.snapshotState()
	msg := mqtt.TransitionSelectMessage(c.deviceInfo(), st.transitions, c.origin)
	c.pubBytes(ctx, msg.Topic, msg.JSON(), true)
}

func (c *connection) publishScreenshot(ctx context.Context) {
	client := c.snapshotClient()
	st := c.snapshotState()
	if client == nil || st.programScene == "" || st.canvasWidth <= 0 || st.canvasHeight <= 0 {
		return
	}
	width := float64(c.opts.ScreenshotWidth)
	height := clamp(math.Round(width*st.canvasHeight/st.canvasWidth), 8, 4096)
	resp, err := client.GetSourceScreenshot(ctx, obs.ScreenshotParams{
		SourceName:              st.programScene,
		ImageFormat:             st.imageFormat,
		ImageWidth:              int(width),
		ImageHeight:             int(height),
		ImageCompressionQuality: c.opts.ScreenshotQuality,
	})
	if err != nil {
		c.log.Debug("screenshot_failed", "scene", st.programScene, "error", err.Error())
		return
	}
	encoded := resp.ImageData
	if idx := strings.IndexByte(encoded, ','); idx >= 0 {
		encoded = encoded[idx+1:]
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		c.log.Warn("screenshot_decode_failed", "error", err.Error())
		return
	}
	c.pubBytes(ctx, c.topics.Screenshot, data, false)
}

func clamp(v, lo, hi float64) float64 {
	return math.Min(math.Max(v, lo), hi)
}
