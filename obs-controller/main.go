// Command obs-controller is the Home Assistant add-on binary: it bridges OBS Studio
// instances to MQTT so they appear as native Home Assistant devices.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jacobgad/obs-controller/internal/bridge"
	"github.com/jacobgad/obs-controller/internal/config"
	"github.com/jacobgad/obs-controller/internal/mqtt"
)

var version = "dev"

const (
	supportURL      = "https://github.com/jacobgad/obs-controller"
	shutdownTimeout = 10 * time.Second
)

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	cfg, err := config.Load(ctx)
	if err != nil {
		newLogger(slog.LevelInfo).Error("config_invalid", "detail", err.Error())
		return err
	}
	log := newLogger(cfg.Options.LogLevel)

	conn, err := mqtt.Connect(ctx, mqtt.PahoOptions{
		Settings: cfg.MQTT,
		ClientID: fmt.Sprintf("obs-controller-%d", os.Getpid()),
		Will:     mqtt.Will{Topic: mqtt.BridgeAvailability, Payload: mqtt.PayloadOffline},
		Log:      log,
	})
	if err != nil {
		log.Error("mqtt_setup_failed", "error", err.Error())
		return err
	}

	b := bridge.New(bridge.Deps{
		MQTT:    conn,
		Options: cfg.Options,
		Log:     log,
		Origin:  mqtt.Origin{Version: version, SupportURL: supportURL},
	})
	if err := b.Start(ctx); err != nil {
		log.Error("startup_failed", "error", err.Error())
		shutdown(b, conn, log)
		return err
	}

	<-ctx.Done()
	log.Info("shutdown_requested")
	shutdown(b, conn, log)
	return nil
}

func shutdown(b *bridge.Bridge, conn mqtt.Connection, log *slog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	b.Stop(ctx)
	if err := conn.Close(ctx); err != nil {
		log.Warn("mqtt_close_failed", "error", err.Error())
	}
}

func newLogger(level slog.Level) *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}
