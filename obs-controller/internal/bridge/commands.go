package bridge

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jacobgad/obs-controller/internal/mqtt"
	"github.com/jacobgad/obs-controller/internal/obs"
)

func (c *connection) handleCommand(ctx context.Context, object string, payload []byte) {
	client := c.snapshotClient()
	if client == nil {
		c.log.Warn("command_dropped_obs_disconnected", "object", object, "payload", string(payload))
		return
	}
	if err := c.execute(ctx, client, object, string(payload)); err != nil {
		c.log.Warn("command_failed", "object", object, "payload", string(payload), "error", err.Error())
		return
	}
	c.log.Info("command_executed", "object", object, "payload", string(payload))
}

func (c *connection) execute(ctx context.Context, client *obs.Client, object, payload string) error {
	switch object {
	case "stream":
		return boolCommand(payload,
			func() error { return client.StartStream(ctx) },
			func() error { return client.StopStream(ctx) })
	case "record":
		return boolCommand(payload,
			func() error { return client.StartRecord(ctx) },
			func() error { return client.StopRecord(ctx) })
	case "record_paused":
		return boolCommand(payload,
			func() error { return client.PauseRecord(ctx) },
			func() error { return client.ResumeRecord(ctx) })
	case "studio_mode":
		enabled, err := parseOnOff(payload)
		if err != nil {
			return err
		}
		return client.SetStudioModeEnabled(ctx, enabled)
	case "program_scene":
		return client.SetCurrentProgramScene(ctx, payload)
	case "preview_scene":
		return client.SetCurrentPreviewScene(ctx, payload)
	case "transition":
		return client.SetCurrentSceneTransition(ctx, payload)
	case "transition_duration":
		ms, err := strconv.Atoi(payload)
		if err != nil || ms < 50 || ms > 20000 {
			return fmt.Errorf("transition duration %q must be an integer between 50 and 20000", payload)
		}
		return client.SetCurrentSceneTransitionDuration(ctx, ms)
	case "trigger_transition":
		if payload != mqtt.PayloadPress {
			return fmt.Errorf("unexpected press payload %q", payload)
		}
		return client.TriggerStudioModeTransition(ctx)
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
