package bridge

import (
	"context"
	"encoding/base64"
	"errors"
	"math"
	"strconv"
	"strings"

	"github.com/andreykaipov/goobs/api/requests/sources"
	"github.com/jacobgad/obs-controller/internal/mqtt"
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
	return mqtt.DeviceInfo{ID: c.cfg.ID, OBSVersion: c.st.obsVersion, Platform: c.st.platform}
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
	c.pollSensors(ctx)
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

func (c *connection) pollSensors(ctx context.Context) {
	client := c.snapshotClient()
	if client == nil {
		return
	}
	stream, err := client.Stream.GetStreamStatus()
	if err != nil {
		c.log.Debug("stream_status_failed", "error", err.Error())
		return
	}
	record, err := client.Record.GetRecordStatus()
	if err != nil {
		c.log.Debug("record_status_failed", "error", err.Error())
		return
	}
	stats, err := client.General.GetStats()
	if err != nil {
		c.log.Debug("stats_failed", "error", err.Error())
		return
	}

	c.reconcileOutputs(ctx, stream.OutputActive, record.OutputActive, record.OutputPaused)

	sensor := func(object, value string) { c.pub(ctx, c.topics.SensorState(object), value) }
	sensor("stream_duration", strconv.Itoa(int(stream.OutputDuration/1000)))
	sensor("stream_congestion", formatFloat(stream.OutputCongestion*100, 1))
	sensor("stream_dropped_pct", formatFloat(percent(stream.OutputSkippedFrames, stream.OutputTotalFrames), 2))
	sensor("record_duration", strconv.Itoa(int(record.OutputDuration/1000)))
	sensor("cpu_usage", formatFloat(stats.CpuUsage, 1))
	sensor("memory_usage", formatFloat(stats.MemoryUsage, 0))
	sensor("active_fps", formatFloat(stats.ActiveFps, 1))
	sensor("render_lag_pct", formatFloat(percent(stats.RenderSkippedFrames, stats.RenderTotalFrames), 2))
	sensor("encode_lag_pct", formatFloat(percent(stats.OutputSkippedFrames, stats.OutputTotalFrames), 2))
	sensor("free_disk_space", formatFloat(stats.AvailableDiskSpace/1024, 1))
}

// reconcileOutputs repairs switch states if an event was missed; normally a no-op.
func (c *connection) reconcileOutputs(ctx context.Context, streaming, recording, paused bool) {
	st := c.snapshotState()
	if st.streaming != streaming {
		c.update(func(st *obsState) { st.streaming = streaming })
		c.pub(ctx, c.topics.StreamState, onOff(streaming))
	}
	if st.recording != recording || st.recordPaused != paused {
		c.update(func(st *obsState) { st.recording, st.recordPaused = recording, paused })
		c.pub(ctx, c.topics.RecordState, onOff(recording))
		c.pub(ctx, c.topics.RecordPausedState, onOff(paused))
	}
}

func (c *connection) publishScreenshot(ctx context.Context) {
	client := c.snapshotClient()
	st := c.snapshotState()
	if client == nil || st.programScene == "" || st.canvasWidth <= 0 || st.canvasHeight <= 0 {
		return
	}
	width := float64(c.opts.ScreenshotWidth)
	height := clamp(math.Round(width*st.canvasHeight/st.canvasWidth), 8, 4096)
	params := sources.NewGetSourceScreenshotParams().
		WithSourceName(st.programScene).
		WithImageFormat(st.imageFormat).
		WithImageWidth(width).
		WithImageHeight(height).
		WithImageCompressionQuality(float64(c.opts.ScreenshotQuality))
	resp, err := client.Sources.GetSourceScreenshot(params)
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

func percent(part, total float64) float64 {
	if total <= 0 {
		return 0
	}
	return part / total * 100
}

func formatFloat(v float64, decimals int) string {
	return strconv.FormatFloat(v, 'f', decimals, 64)
}

func clamp(v, lo, hi float64) float64 {
	return math.Min(math.Max(v, lo), hi)
}
