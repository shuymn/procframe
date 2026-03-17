package ws

import "encoding/json"

// Frame type constants for the v2 session protocol.
const (
	frameTypeOpen    = "open"
	frameTypeMessage = "message"
	frameTypeClose   = "close"
	frameTypeCancel  = "cancel"
	frameTypeError   = "error"
)

type inboundFrame struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Procedure string          `json:"procedure,omitempty"`
	Shape     string          `json:"shape,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

type outboundFrame struct {
	Type    string          `json:"type"`
	ID      string          `json:"id"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   *errorDetail    `json:"error,omitempty"`
}

type errorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}
