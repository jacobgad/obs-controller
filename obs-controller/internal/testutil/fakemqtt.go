// Package testutil provides fakes for bridge tests.
package testutil

import (
	"context"
	"sync"

	"github.com/jacobgad/obs-controller/internal/mqtt"
)

// PublishedMessage is one recorded broker publish.
type PublishedMessage struct {
	Topic   string
	Payload []byte
	Retain  bool
}

// FakeMQTT records publishes and lets tests inject inbound messages.
type FakeMQTT struct {
	mu            sync.Mutex
	published     []PublishedMessage
	subscriptions []string
	onMessage     []mqtt.MessageHandler
	onConnect     []func()
}

// NewFakeMQTT builds an always-connected fake broker session.
func NewFakeMQTT() *FakeMQTT { return &FakeMQTT{} }

// Publish records the message.
func (f *FakeMQTT) Publish(_ context.Context, topic string, payload []byte, retain bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.published = append(f.published, PublishedMessage{Topic: topic, Payload: append([]byte{}, payload...), Retain: retain})
	return nil
}

// Subscribe records the topic filters.
func (f *FakeMQTT) Subscribe(_ context.Context, topics []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.subscriptions = append(f.subscriptions, topics...)
	return nil
}

// OnMessage registers an inbound handler.
func (f *FakeMQTT) OnMessage(handler mqtt.MessageHandler) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onMessage = append(f.onMessage, handler)
}

// OnConnect registers a connect handler.
func (f *FakeMQTT) OnConnect(handler func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onConnect = append(f.onConnect, handler)
}

// Connected always reports true.
func (f *FakeMQTT) Connected() bool { return true }

// AwaitConnection returns immediately.
func (f *FakeMQTT) AwaitConnection(context.Context) error { return nil }

// Close is a no-op.
func (f *FakeMQTT) Close(context.Context) error { return nil }

// Deliver feeds one inbound message to every registered handler.
func (f *FakeMQTT) Deliver(topic string, payload []byte) {
	f.mu.Lock()
	handlers := append([]mqtt.MessageHandler{}, f.onMessage...)
	f.mu.Unlock()
	for _, h := range handlers {
		h(topic, payload)
	}
}

// Subscriptions lists every subscribed topic filter.
func (f *FakeMQTT) Subscriptions() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.subscriptions...)
}

// Last returns the most recent payload published to a topic.
func (f *FakeMQTT) Last(topic string) ([]byte, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.published) - 1; i >= 0; i-- {
		if f.published[i].Topic == topic {
			return append([]byte{}, f.published[i].Payload...), true
		}
	}
	return nil, false
}
