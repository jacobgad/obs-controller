package obs

import "encoding/json"

// Version is the GetVersion response subset this add-on uses.
type Version struct {
	ObsVersion            string   `json:"obsVersion"`
	ObsWebSocketVersion   string   `json:"obsWebSocketVersion"`
	Platform              string   `json:"platform"`
	SupportedImageFormats []string `json:"supportedImageFormats"`
}

// VideoSettings is the GetVideoSettings response subset this add-on uses.
type VideoSettings struct {
	BaseWidth  float64 `json:"baseWidth"`
	BaseHeight float64 `json:"baseHeight"`
}

// Scene is one entry of a scene list.
type Scene struct {
	SceneName string `json:"sceneName"`
}

// SceneList is the GetSceneList response.
type SceneList struct {
	CurrentProgramSceneName string  `json:"currentProgramSceneName"`
	CurrentPreviewSceneName string  `json:"currentPreviewSceneName"`
	Scenes                  []Scene `json:"scenes"`
}

// PreviewScene is the GetCurrentPreviewScene response.
type PreviewScene struct {
	CurrentPreviewSceneName string `json:"currentPreviewSceneName"`
}

// Transition is one entry of a transition list.
type Transition struct {
	TransitionName string `json:"transitionName"`
}

// TransitionList is the GetSceneTransitionList response.
type TransitionList struct {
	CurrentSceneTransitionName string       `json:"currentSceneTransitionName"`
	Transitions                []Transition `json:"transitions"`
}

// CurrentTransition is the GetCurrentSceneTransition response subset this add-on uses.
type CurrentTransition struct {
	TransitionName     string  `json:"transitionName"`
	TransitionDuration float64 `json:"transitionDuration"`
}

// StudioMode is the GetStudioModeEnabled response.
type StudioMode struct {
	StudioModeEnabled bool `json:"studioModeEnabled"`
}

// StreamStatus is the GetStreamStatus response subset this add-on uses.
type StreamStatus struct {
	OutputActive        bool    `json:"outputActive"`
	OutputDuration      float64 `json:"outputDuration"`
	OutputCongestion    float64 `json:"outputCongestion"`
	OutputSkippedFrames float64 `json:"outputSkippedFrames"`
	OutputTotalFrames   float64 `json:"outputTotalFrames"`
}

// RecordStatus is the GetRecordStatus response subset this add-on uses.
type RecordStatus struct {
	OutputActive   bool    `json:"outputActive"`
	OutputPaused   bool    `json:"outputPaused"`
	OutputDuration float64 `json:"outputDuration"`
}

// Stats is the GetStats response subset this add-on uses.
type Stats struct {
	CPUUsage            float64 `json:"cpuUsage"`
	MemoryUsage         float64 `json:"memoryUsage"`
	ActiveFps           float64 `json:"activeFps"`
	AvailableDiskSpace  float64 `json:"availableDiskSpace"`
	RenderSkippedFrames float64 `json:"renderSkippedFrames"`
	RenderTotalFrames   float64 `json:"renderTotalFrames"`
	OutputSkippedFrames float64 `json:"outputSkippedFrames"`
	OutputTotalFrames   float64 `json:"outputTotalFrames"`
}

// ScreenshotParams are the GetSourceScreenshot request fields this add-on uses.
type ScreenshotParams struct {
	SourceName              string `json:"sourceName"`
	ImageFormat             string `json:"imageFormat"`
	ImageWidth              int    `json:"imageWidth"`
	ImageHeight             int    `json:"imageHeight"`
	ImageCompressionQuality int    `json:"imageCompressionQuality"`
}

// Screenshot is the GetSourceScreenshot response: a base64 data URI.
type Screenshot struct {
	ImageData string `json:"imageData"`
}

// Events delivered by Client.Events. Field subsets mirror the protocol.
type (
	// StreamStateChanged reports the stream output starting or stopping.
	StreamStateChanged struct {
		OutputActive bool   `json:"outputActive"`
		OutputState  string `json:"outputState"`
	}
	// RecordStateChanged reports the record output starting, stopping, pausing or resuming.
	RecordStateChanged struct {
		OutputActive bool   `json:"outputActive"`
		OutputState  string `json:"outputState"`
		OutputPath   string `json:"outputPath"`
	}
	// CurrentProgramSceneChanged reports a program scene switch.
	CurrentProgramSceneChanged struct {
		SceneName string `json:"sceneName"`
	}
	// CurrentPreviewSceneChanged reports a preview scene switch.
	CurrentPreviewSceneChanged struct {
		SceneName string `json:"sceneName"`
	}
	// SceneListChanged reports scene creation, removal or renaming.
	SceneListChanged struct {
		Scenes []Scene `json:"scenes"`
	}
	// SceneNameChanged reports one scene rename.
	SceneNameChanged struct {
		OldSceneName string `json:"oldSceneName"`
		SceneName    string `json:"sceneName"`
	}
	// StudioModeStateChanged reports studio mode toggling.
	StudioModeStateChanged struct {
		StudioModeEnabled bool `json:"studioModeEnabled"`
	}
	// CurrentSceneTransitionChanged reports the active transition changing.
	CurrentSceneTransitionChanged struct {
		TransitionName string `json:"transitionName"`
	}
	// CurrentSceneTransitionDurationChanged reports the transition duration changing.
	CurrentSceneTransitionDurationChanged struct {
		TransitionDuration float64 `json:"transitionDuration"`
	}
	// ExitStarted reports OBS shutting down.
	ExitStarted struct{}
)

func decodeEvent(eventType string, data json.RawMessage) any {
	var target any
	switch eventType {
	case "StreamStateChanged":
		target = &StreamStateChanged{}
	case "RecordStateChanged":
		target = &RecordStateChanged{}
	case "CurrentProgramSceneChanged":
		target = &CurrentProgramSceneChanged{}
	case "CurrentPreviewSceneChanged":
		target = &CurrentPreviewSceneChanged{}
	case "SceneListChanged":
		target = &SceneListChanged{}
	case "SceneNameChanged":
		target = &SceneNameChanged{}
	case "StudioModeStateChanged":
		target = &StudioModeStateChanged{}
	case "CurrentSceneTransitionChanged":
		target = &CurrentSceneTransitionChanged{}
	case "CurrentSceneTransitionDurationChanged":
		target = &CurrentSceneTransitionDurationChanged{}
	case "ExitStarted":
		return &ExitStarted{}
	default:
		return nil
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, target); err != nil {
			return nil
		}
	}
	return target
}
