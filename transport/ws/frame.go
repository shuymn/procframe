package ws

import "encoding/json"

type inboundFrame struct {
	ID        string          `json:"id"`
	Procedure string          `json:"procedure"`
	Payload   json.RawMessage `json:"payload"`
}

type outboundFrame struct {
	ID      string          `json:"id"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   *errorDetail    `json:"error,omitempty"`
	EOS     bool            `json:"eos"`
}

type errorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}
