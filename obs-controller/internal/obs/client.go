package obs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ErrAuthFailed is returned by Dial when OBS rejects the password.
var ErrAuthFailed = errors.New("obs: authentication failed")

// ErrClosed is returned by Call once the connection has died.
var ErrClosed = errors.New("obs: connection closed")

// Options configure Dial.
type Options struct {
	Addr               string
	Password           string
	EventSubscriptions int
	HandshakeTimeout   time.Duration
	RequestTimeout     time.Duration
}

// Client is one identified obs-websocket session. It is safe for concurrent use and
// lives until Close is called or the connection drops.
type Client struct {
	conn       *websocket.Conn
	reqTimeout time.Duration

	writeMu sync.Mutex

	pendingMu sync.Mutex
	pending   map[string]chan responseData

	events  chan any
	done    chan struct{}
	closing chan struct{}
	once    sync.Once
}

const (
	defaultHandshakeTimeout = 10 * time.Second
	defaultRequestTimeout   = 10 * time.Second
	eventBuffer             = 128
)

// Dial connects, performs the Hello/Identify handshake and starts the read loop.
// A wrong password surfaces as ErrAuthFailed.
func Dial(ctx context.Context, opts Options) (*Client, error) {
	if opts.HandshakeTimeout <= 0 {
		opts.HandshakeTimeout = defaultHandshakeTimeout
	}
	if opts.RequestTimeout <= 0 {
		opts.RequestTimeout = defaultRequestTimeout
	}
	dialer := websocket.Dialer{HandshakeTimeout: opts.HandshakeTimeout}
	conn, resp, err := dialer.DialContext(ctx, "ws://"+opts.Addr, nil)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return nil, err
	}
	if err := identify(conn, opts); err != nil {
		_ = conn.Close()
		return nil, err
	}
	c := &Client{
		conn:       conn,
		reqTimeout: opts.RequestTimeout,
		pending:    make(map[string]chan responseData),
		events:     make(chan any, eventBuffer),
		done:       make(chan struct{}),
		closing:    make(chan struct{}),
	}
	go c.readLoop()
	return c, nil
}

func identify(conn *websocket.Conn, opts Options) error {
	deadline := time.Now().Add(opts.HandshakeTimeout)
	if err := conn.SetReadDeadline(deadline); err != nil {
		return err
	}
	var helloEnv envelope
	if err := conn.ReadJSON(&helloEnv); err != nil {
		return err
	}
	if helloEnv.Op != opHello {
		return fmt.Errorf("obs: expected Hello, got op %d", helloEnv.Op)
	}
	var hello helloData
	if err := json.Unmarshal(helloEnv.D, &hello); err != nil {
		return err
	}
	ident := identifyData{RPCVersion: 1, EventSubscriptions: opts.EventSubscriptions}
	if hello.Authentication != nil {
		if opts.Password == "" {
			return fmt.Errorf("%w: a password is required but none is configured", ErrAuthFailed)
		}
		ident.Authentication = AuthResponse(opts.Password, hello.Authentication.Salt, hello.Authentication.Challenge)
	}
	identEnv, err := marshalEnvelope(opIdentify, ident)
	if err != nil {
		return err
	}
	if err := conn.WriteJSON(identEnv); err != nil {
		return err
	}
	var identifiedEnv envelope
	if err := conn.ReadJSON(&identifiedEnv); err != nil {
		var closeErr *websocket.CloseError
		if errors.As(err, &closeErr) && closeErr.Code == CloseAuthenticationFailed {
			return ErrAuthFailed
		}
		return err
	}
	if identifiedEnv.Op != opIdentified {
		return fmt.Errorf("obs: expected Identified, got op %d", identifiedEnv.Op)
	}
	return conn.SetReadDeadline(time.Time{})
}

func (c *Client) readLoop() {
	defer func() {
		_ = c.conn.Close()
		close(c.done)
		close(c.events)
	}()
	for {
		var env envelope
		if err := c.conn.ReadJSON(&env); err != nil {
			return
		}
		switch env.Op {
		case opEvent:
			var ev eventData
			if err := json.Unmarshal(env.D, &ev); err != nil {
				continue
			}
			decoded := decodeEvent(ev.EventType, ev.EventData)
			if decoded == nil {
				continue
			}
			select {
			case c.events <- decoded:
			case <-c.closing:
				return
			}
		case opRequestResponse:
			var resp responseData
			if err := json.Unmarshal(env.D, &resp); err != nil {
				continue
			}
			c.deliver(resp)
		}
	}
}

func (c *Client) deliver(resp responseData) {
	c.pendingMu.Lock()
	ch, ok := c.pending[resp.RequestID]
	delete(c.pending, resp.RequestID)
	c.pendingMu.Unlock()
	if ok {
		ch <- resp
	}
}

// Call sends one request and decodes the response into out (which may be nil).
func (c *Client) Call(ctx context.Context, requestType string, params, out any) error {
	ctx, cancel := context.WithTimeout(ctx, c.reqTimeout)
	defer cancel()

	id := randomID()
	ch := make(chan responseData, 1)
	c.pendingMu.Lock()
	c.pending[id] = ch
	c.pendingMu.Unlock()
	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
	}()

	env, err := marshalEnvelope(opRequest, requestData{RequestType: requestType, RequestID: id, RequestData: params})
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	err = c.conn.WriteJSON(env)
	c.writeMu.Unlock()
	if err != nil {
		return err
	}

	select {
	case resp := <-ch:
		if !resp.RequestStatus.Result {
			return &RequestError{RequestType: requestType, Code: resp.RequestStatus.Code, Comment: resp.RequestStatus.Comment}
		}
		if out != nil && len(resp.ResponseData) > 0 {
			return json.Unmarshal(resp.ResponseData, out)
		}
		return nil
	case <-c.done:
		return ErrClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Events delivers decoded events in arrival order; the channel closes when the
// connection dies.
func (c *Client) Events() <-chan any { return c.events }

// Done is closed when the connection dies, however that happens.
func (c *Client) Done() <-chan struct{} { return c.done }

// Close tears the connection down; safe to call more than once.
func (c *Client) Close() error {
	c.once.Do(func() {
		close(c.closing)
		c.writeMu.Lock()
		_ = c.conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye"), time.Now().Add(time.Second))
		c.writeMu.Unlock()
		_ = c.conn.Close()
	})
	return nil
}
