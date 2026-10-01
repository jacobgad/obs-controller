// Package obstest is a fake obs-websocket 5.x server for exercising the obs client
// and the bridge without a real OBS instance.
package obstest

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jacobgad/obs-controller/internal/obs"
)

// Handler answers one request type; the returned value becomes responseData.
type Handler func(requestData json.RawMessage) (any, error)

// ReceivedRequest is one request a client sent.
type ReceivedRequest struct {
	Type string
	Data json.RawMessage
}

type session struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (s *session) write(v any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conn.WriteJSON(v)
}

// Server speaks enough of the protocol for tests: handshake with optional auth,
// request dispatch to registered handlers, and event injection.
type Server struct {
	httpServer *httptest.Server
	password   string
	upgrader   websocket.Upgrader

	mu       sync.Mutex
	handlers map[string]Handler
	received []ReceivedRequest
	sessions []*session
}

// New starts a fake server; empty password disables authentication.
func New(password string) *Server {
	s := &Server{password: password, handlers: map[string]Handler{}}
	s.httpServer = httptest.NewServer(http.HandlerFunc(s.serve))
	return s
}

// Addr is the host:port clients dial.
func (s *Server) Addr() string {
	return strings.TrimPrefix(s.httpServer.URL, "http://")
}

// Close shuts the server down, dropping live websocket connections first (httptest
// forgets hijacked connections, so they would otherwise outlive the HTTP server).
func (s *Server) Close() {
	s.mu.Lock()
	for _, sess := range s.sessions {
		_ = sess.conn.Close()
	}
	s.sessions = nil
	s.mu.Unlock()
	s.httpServer.Close()
}

// Handle registers the answer for one request type; unregistered types succeed with
// an empty response.
func (s *Server) Handle(requestType string, h Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[requestType] = h
}

// Respond registers a fixed successful response for one request type.
func (s *Server) Respond(requestType string, response any) {
	s.Handle(requestType, func(json.RawMessage) (any, error) { return response, nil })
}

// Received lists every request seen so far, in arrival order.
func (s *Server) Received() []ReceivedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]ReceivedRequest{}, s.received...)
}

// SendEvent pushes an event to every identified client.
func (s *Server) SendEvent(eventType string, data any) error {
	payload, err := json.Marshal(map[string]any{"eventType": eventType, "eventIntent": 1, "eventData": data})
	if err != nil {
		return err
	}
	s.mu.Lock()
	sessions := append([]*session{}, s.sessions...)
	s.mu.Unlock()
	if len(sessions) == 0 {
		return errors.New("obstest: no identified clients")
	}
	for _, sess := range sessions {
		if err := sess.write(map[string]any{"op": 5, "d": json.RawMessage(payload)}); err != nil {
			return err
		}
	}
	return nil
}

const (
	testSalt      = "test-salt"
	testChallenge = "test-challenge"
)

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	sess := &session{conn: conn}
	if !s.handshake(sess) {
		return
	}
	s.mu.Lock()
	s.sessions = append(s.sessions, sess)
	s.mu.Unlock()
	s.requestLoop(sess)
}

func (s *Server) handshake(sess *session) bool {
	hello := map[string]any{"obsWebSocketVersion": "5.5.0", "rpcVersion": 1}
	if s.password != "" {
		hello["authentication"] = map[string]any{"challenge": testChallenge, "salt": testSalt}
	}
	if err := sess.write(map[string]any{"op": 0, "d": hello}); err != nil {
		return false
	}
	var env struct {
		Op int `json:"op"`
		D  struct {
			RPCVersion     int    `json:"rpcVersion"`
			Authentication string `json:"authentication"`
		} `json:"d"`
	}
	if err := sess.conn.ReadJSON(&env); err != nil || env.Op != 1 {
		return false
	}
	if s.password != "" && env.D.Authentication != obs.AuthResponse(s.password, testSalt, testChallenge) {
		_ = sess.conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(obs.CloseAuthenticationFailed, "authentication failed"),
			time.Now().Add(time.Second))
		return false
	}
	return sess.write(map[string]any{"op": 2, "d": map[string]any{"negotiatedRpcVersion": 1}}) == nil
}

func (s *Server) requestLoop(sess *session) {
	for {
		var env struct {
			Op int `json:"op"`
			D  struct {
				RequestType string          `json:"requestType"`
				RequestID   string          `json:"requestId"`
				RequestData json.RawMessage `json:"requestData"`
			} `json:"d"`
		}
		if err := sess.conn.ReadJSON(&env); err != nil {
			return
		}
		if env.Op != 6 {
			continue
		}
		s.mu.Lock()
		s.received = append(s.received, ReceivedRequest{Type: env.D.RequestType, Data: env.D.RequestData})
		handler := s.handlers[env.D.RequestType]
		s.mu.Unlock()

		status := map[string]any{"result": true, "code": 100}
		var response any
		if handler != nil {
			var err error
			response, err = handler(env.D.RequestData)
			if err != nil {
				status = map[string]any{"result": false, "code": 700, "comment": err.Error()}
				response = nil
			}
		}
		reply := map[string]any{
			"requestType":   env.D.RequestType,
			"requestId":     env.D.RequestID,
			"requestStatus": status,
		}
		if response != nil {
			reply["responseData"] = response
		}
		if err := sess.write(map[string]any{"op": 7, "d": reply}); err != nil {
			return
		}
	}
}
