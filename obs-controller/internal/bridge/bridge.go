// Package bridge connects configured OBS instances to the MQTT broker, exposing each
// one as a Home Assistant device.
package bridge

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/jacobgad/obs-controller/internal/config"
	"github.com/jacobgad/obs-controller/internal/mqtt"
)

// Deps are the bridge's collaborators.
type Deps struct {
	MQTT    mqtt.Connection
	Options config.Options
	Log     *slog.Logger
	Origin  mqtt.Origin
	// HTTPAddr is the MJPEG server listen address; empty means ":9981".
	HTTPAddr string
}

// Bridge runs one connection loop per configured OBS instance and routes MQTT
// commands to them.
type Bridge struct {
	deps       Deps
	conns      map[string]*connection
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	httpServer *http.Server
	listener   net.Listener
}

// New wires a Bridge; Start launches it.
func New(deps Deps) *Bridge {
	b := &Bridge{deps: deps, conns: make(map[string]*connection)}
	for _, cc := range deps.Options.Connections {
		b.conns[cc.ID] = newConnection(cc, deps)
	}
	return b
}

const startupTimeout = 30 * time.Second

// Start waits for the broker, announces the bridge and launches one loop per OBS
// connection. It returns once startup is complete; the loops run until Stop.
func (b *Bridge) Start(ctx context.Context) error {
	b.deps.MQTT.OnMessage(b.route)
	b.deps.MQTT.OnConnect(func() { go b.announce(context.WithoutCancel(ctx)) })

	awaitCtx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	if err := b.deps.MQTT.AwaitConnection(awaitCtx); err != nil {
		return err
	}
	if err := b.deps.MQTT.Subscribe(ctx, []string{mqtt.CommandSetWildcard, mqtt.CommandPressWildcard}); err != nil {
		return err
	}

	if err := b.startHTTP(ctx); err != nil {
		return err
	}

	runCtx, stop := context.WithCancel(context.WithoutCancel(ctx))
	b.cancel = stop
	for _, c := range b.conns {
		b.wg.Add(1)
		go func(c *connection) {
			defer b.wg.Done()
			c.run(runCtx)
		}(c)
	}
	return nil
}

func (b *Bridge) startHTTP(ctx context.Context) error {
	addr := b.deps.HTTPAddr
	if addr == "" {
		addr = ":9981"
	}
	listener, err := new(net.ListenConfig).Listen(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	b.listener = listener
	b.httpServer = &http.Server{Handler: b.httpHandler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := b.httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			b.deps.Log.Error("mjpeg_server_failed", "error", err.Error())
		}
	}()
	b.deps.Log.Info("mjpeg_server_listening", "addr", listener.Addr().String())
	return nil
}

// HTTPAddr is the MJPEG server's bound address.
func (b *Bridge) HTTPAddr() string {
	if b.listener == nil {
		return ""
	}
	return b.listener.Addr().String()
}

// announce republishes bridge availability and per-connection state after each MQTT
// (re)connect, since the Last Will may have marked everything offline in between.
func (b *Bridge) announce(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := b.deps.MQTT.Publish(ctx, mqtt.BridgeAvailability, []byte(mqtt.PayloadOnline), true); err != nil {
		b.deps.Log.Warn("bridge_availability_publish_failed", "error", err.Error())
	}
	for _, c := range b.conns {
		c.republish(ctx)
	}
}

func (b *Bridge) route(topic string, payload []byte) {
	cmd, ok := mqtt.ParseCommand(topic)
	if !ok {
		return
	}
	conn, ok := b.conns[cmd.ConnectionID]
	if !ok {
		b.deps.Log.Warn("command_for_unknown_connection", "topic", topic)
		return
	}
	go conn.handleCommand(context.Background(), cmd.Object, payload)
}

// Stop halts the connection loops and marks everything offline before the broker
// session closes cleanly (a clean DISCONNECT suppresses the Last Will).
func (b *Bridge) Stop(ctx context.Context) {
	if b.cancel != nil {
		b.cancel()
	}
	if b.httpServer != nil {
		// Shutdown never finishes while MJPEG streams are open; the deadline, then
		// Close, is what actually ends them.
		if err := b.httpServer.Shutdown(ctx); err != nil {
			_ = b.httpServer.Close()
		}
	}
	done := make(chan struct{})
	go func() {
		b.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
	for _, c := range b.conns {
		c.publishDown(ctx)
	}
	if err := b.deps.MQTT.Publish(ctx, mqtt.BridgeAvailability, []byte(mqtt.PayloadOffline), true); err != nil {
		b.deps.Log.Warn("bridge_availability_publish_failed", "error", err.Error())
	}
}
