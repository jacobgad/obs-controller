package obs

import "context"

// GetVersion reports OBS and protocol versions plus supported screenshot formats.
func (c *Client) GetVersion(ctx context.Context) (*Version, error) {
	out := &Version{}
	return out, c.Call(ctx, "GetVersion", nil, out)
}

// GetVideoSettings reports the canvas dimensions.
func (c *Client) GetVideoSettings(ctx context.Context) (*VideoSettings, error) {
	out := &VideoSettings{}
	return out, c.Call(ctx, "GetVideoSettings", nil, out)
}

// GetSceneList reports all scenes and the current program/preview scenes.
func (c *Client) GetSceneList(ctx context.Context) (*SceneList, error) {
	out := &SceneList{}
	return out, c.Call(ctx, "GetSceneList", nil, out)
}

// GetCurrentPreviewScene reports the preview scene; it fails when studio mode is off.
func (c *Client) GetCurrentPreviewScene(ctx context.Context) (*PreviewScene, error) {
	out := &PreviewScene{}
	return out, c.Call(ctx, "GetCurrentPreviewScene", nil, out)
}

// GetSceneTransitionList reports all transitions and the current one.
func (c *Client) GetSceneTransitionList(ctx context.Context) (*TransitionList, error) {
	out := &TransitionList{}
	return out, c.Call(ctx, "GetSceneTransitionList", nil, out)
}

// GetCurrentSceneTransition reports the current transition and its duration.
func (c *Client) GetCurrentSceneTransition(ctx context.Context) (*CurrentTransition, error) {
	out := &CurrentTransition{}
	return out, c.Call(ctx, "GetCurrentSceneTransition", nil, out)
}

// GetStudioModeEnabled reports whether studio mode is on.
func (c *Client) GetStudioModeEnabled(ctx context.Context) (*StudioMode, error) {
	out := &StudioMode{}
	return out, c.Call(ctx, "GetStudioModeEnabled", nil, out)
}

// GetStreamStatus reports the stream output state and statistics.
func (c *Client) GetStreamStatus(ctx context.Context) (*StreamStatus, error) {
	out := &StreamStatus{}
	return out, c.Call(ctx, "GetStreamStatus", nil, out)
}

// GetRecordStatus reports the record output state.
func (c *Client) GetRecordStatus(ctx context.Context) (*RecordStatus, error) {
	out := &RecordStatus{}
	return out, c.Call(ctx, "GetRecordStatus", nil, out)
}

// GetStats reports OBS performance statistics.
func (c *Client) GetStats(ctx context.Context) (*Stats, error) {
	out := &Stats{}
	return out, c.Call(ctx, "GetStats", nil, out)
}

// GetSourceScreenshot renders a scaled still of a source.
func (c *Client) GetSourceScreenshot(ctx context.Context, params ScreenshotParams) (*Screenshot, error) {
	out := &Screenshot{}
	return out, c.Call(ctx, "GetSourceScreenshot", params, out)
}

// StartStream starts the stream output.
func (c *Client) StartStream(ctx context.Context) error { return c.Call(ctx, "StartStream", nil, nil) }

// StopStream stops the stream output.
func (c *Client) StopStream(ctx context.Context) error { return c.Call(ctx, "StopStream", nil, nil) }

// StartRecord starts the record output.
func (c *Client) StartRecord(ctx context.Context) error { return c.Call(ctx, "StartRecord", nil, nil) }

// StopRecord stops the record output.
func (c *Client) StopRecord(ctx context.Context) error { return c.Call(ctx, "StopRecord", nil, nil) }

// PauseRecord pauses the record output.
func (c *Client) PauseRecord(ctx context.Context) error { return c.Call(ctx, "PauseRecord", nil, nil) }

// ResumeRecord resumes the record output.
func (c *Client) ResumeRecord(ctx context.Context) error {
	return c.Call(ctx, "ResumeRecord", nil, nil)
}

// SetStudioModeEnabled turns studio mode on or off.
func (c *Client) SetStudioModeEnabled(ctx context.Context, enabled bool) error {
	return c.Call(ctx, "SetStudioModeEnabled", map[string]any{"studioModeEnabled": enabled}, nil)
}

// SetCurrentProgramScene switches the program scene.
func (c *Client) SetCurrentProgramScene(ctx context.Context, sceneName string) error {
	return c.Call(ctx, "SetCurrentProgramScene", map[string]any{"sceneName": sceneName}, nil)
}

// SetCurrentPreviewScene switches the preview scene; it fails when studio mode is off.
func (c *Client) SetCurrentPreviewScene(ctx context.Context, sceneName string) error {
	return c.Call(ctx, "SetCurrentPreviewScene", map[string]any{"sceneName": sceneName}, nil)
}

// SetCurrentSceneTransition switches the active transition.
func (c *Client) SetCurrentSceneTransition(ctx context.Context, transitionName string) error {
	return c.Call(ctx, "SetCurrentSceneTransition", map[string]any{"transitionName": transitionName}, nil)
}

// SetCurrentSceneTransitionDuration sets the transition duration in milliseconds.
func (c *Client) SetCurrentSceneTransitionDuration(ctx context.Context, ms int) error {
	return c.Call(ctx, "SetCurrentSceneTransitionDuration", map[string]any{"transitionDuration": ms}, nil)
}

// TriggerStudioModeTransition sends the preview scene to program.
func (c *Client) TriggerStudioModeTransition(ctx context.Context) error {
	return c.Call(ctx, "TriggerStudioModeTransition", nil, nil)
}
