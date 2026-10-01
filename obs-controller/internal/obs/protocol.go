// Package obs is a minimal obs-websocket 5.x client covering the requests and events
// this add-on uses: context-aware calls, typed events and slog-friendly errors.
package obs

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

const (
	opHello           = 0
	opIdentify        = 1
	opIdentified      = 2
	opEvent           = 5
	opRequest         = 6
	opRequestResponse = 7
)

// Event subscription bits from the obs-websocket 5 protocol.
const (
	SubGeneral     = 1 << 0
	SubConfig      = 1 << 1
	SubScenes      = 1 << 2
	SubTransitions = 1 << 4
	SubOutputs     = 1 << 6
	SubUI          = 1 << 10
)

// CloseAuthenticationFailed is the websocket close code OBS sends on a bad password.
const CloseAuthenticationFailed = 4009

type envelope struct {
	Op int             `json:"op"`
	D  json.RawMessage `json:"d"`
}

type helloData struct {
	ObsWebSocketVersion string `json:"obsWebSocketVersion"`
	RPCVersion          int    `json:"rpcVersion"`
	Authentication      *struct {
		Challenge string `json:"challenge"`
		Salt      string `json:"salt"`
	} `json:"authentication"`
}

type identifyData struct {
	RPCVersion         int    `json:"rpcVersion"`
	Authentication     string `json:"authentication,omitempty"`
	EventSubscriptions int    `json:"eventSubscriptions"`
}

type requestData struct {
	RequestType string `json:"requestType"`
	RequestID   string `json:"requestId"`
	RequestData any    `json:"requestData,omitempty"`
}

type responseData struct {
	RequestType   string `json:"requestType"`
	RequestID     string `json:"requestId"`
	RequestStatus struct {
		Result  bool   `json:"result"`
		Code    int    `json:"code"`
		Comment string `json:"comment"`
	} `json:"requestStatus"`
	ResponseData json.RawMessage `json:"responseData"`
}

type eventData struct {
	EventType string          `json:"eventType"`
	EventData json.RawMessage `json:"eventData"`
}

func marshalEnvelope(op int, d any) (envelope, error) {
	data, err := json.Marshal(d)
	if err != nil {
		return envelope{}, err
	}
	return envelope{Op: op, D: data}, nil
}

// AuthResponse computes the Identify authentication string for a Hello challenge:
// base64(sha256(base64(sha256(password + salt)) + challenge)).
func AuthResponse(password, salt, challenge string) string {
	secret := sha256.Sum256([]byte(password + salt))
	secretB64 := base64.StdEncoding.EncodeToString(secret[:])
	auth := sha256.Sum256([]byte(secretB64 + challenge))
	return base64.StdEncoding.EncodeToString(auth[:])
}

func randomID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// RequestError is a request the OBS server answered with a failure status.
type RequestError struct {
	RequestType string
	Code        int
	Comment     string
}

func (e *RequestError) Error() string {
	return fmt.Sprintf("obs: %s failed with code %d: %s", e.RequestType, e.Code, e.Comment)
}
