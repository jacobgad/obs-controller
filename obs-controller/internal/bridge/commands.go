package bridge

import (
	"fmt"
	"strconv"

	"github.com/andreykaipov/goobs"
	"github.com/andreykaipov/goobs/api/requests/record"
	"github.com/andreykaipov/goobs/api/requests/scenes"
	"github.com/andreykaipov/goobs/api/requests/stream"
	"github.com/andreykaipov/goobs/api/requests/transitions"
	"github.com/andreykaipov/goobs/api/requests/ui"
	"github.com/jacobgad/obs-controller/internal/mqtt"
)

func (c *connection) handleCommand(object string, payload []byte) {
	client := c.snapshotClient()
	if client == nil {
		c.log.Warn("command_dropped_obs_disconnected", "object", object, "payload", string(payload))
		return
	}
	if err := c.execute(client, object, string(payload)); err != nil {
		c.log.Warn("command_failed", "object", object, "payload", string(payload), "error", err.Error())
		return
	}
	c.log.Info("command_executed", "object", object, "payload", string(payload))
}

func (c *connection) execute(client *goobs.Client, object, payload string) error {
	switch object {
	case "stream":
		return boolCommand(payload,
			func() error { _, err := client.Stream.StartStream(&stream.StartStreamParams{}); return err },
			func() error { _, err := client.Stream.StopStream(&stream.StopStreamParams{}); return err })
	case "record":
		return boolCommand(payload,
			func() error { _, err := client.Record.StartRecord(&record.StartRecordParams{}); return err },
			func() error { _, err := client.Record.StopRecord(&record.StopRecordParams{}); return err })
	case "record_paused":
		return boolCommand(payload,
			func() error { _, err := client.Record.PauseRecord(&record.PauseRecordParams{}); return err },
			func() error { _, err := client.Record.ResumeRecord(&record.ResumeRecordParams{}); return err })
	case "studio_mode":
		enabled, err := parseOnOff(payload)
		if err != nil {
			return err
		}
		_, err = client.Ui.SetStudioModeEnabled(ui.NewSetStudioModeEnabledParams().WithStudioModeEnabled(enabled))
		return err
	case "program_scene":
		_, err := client.Scenes.SetCurrentProgramScene(scenes.NewSetCurrentProgramSceneParams().WithSceneName(payload))
		return err
	case "preview_scene":
		_, err := client.Scenes.SetCurrentPreviewScene(scenes.NewSetCurrentPreviewSceneParams().WithSceneName(payload))
		return err
	case "transition":
		_, err := client.Transitions.SetCurrentSceneTransition(
			transitions.NewSetCurrentSceneTransitionParams().WithTransitionName(payload))
		return err
	case "transition_duration":
		ms, err := strconv.Atoi(payload)
		if err != nil || ms < 50 || ms > 20000 {
			return fmt.Errorf("transition duration %q must be an integer between 50 and 20000", payload)
		}
		_, err = client.Transitions.SetCurrentSceneTransitionDuration(
			transitions.NewSetCurrentSceneTransitionDurationParams().WithTransitionDuration(float64(ms)))
		return err
	case "trigger_transition":
		if payload != mqtt.PayloadPress {
			return fmt.Errorf("unexpected press payload %q", payload)
		}
		_, err := client.Transitions.TriggerStudioModeTransition()
		return err
	default:
		return fmt.Errorf("unknown command object %q", object)
	}
}

func boolCommand(payload string, on, off func() error) error {
	v, err := parseOnOff(payload)
	if err != nil {
		return err
	}
	if v {
		return on()
	}
	return off()
}

func parseOnOff(payload string) (bool, error) {
	switch payload {
	case mqtt.PayloadOn:
		return true, nil
	case mqtt.PayloadOff:
		return false, nil
	default:
		return false, fmt.Errorf("payload %q must be %s or %s", payload, mqtt.PayloadOn, mqtt.PayloadOff)
	}
}
