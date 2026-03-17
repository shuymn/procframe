package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/coder/websocket"

	"github.com/shuymn/procframe"
)

// clientSession tracks a single in-flight RPC on the client side.
type clientSession struct {
	recvCh chan outboundFrame
}

// Conn is a multiplexed WebSocket client connection. It manages sessions
// over a single underlying WebSocket connection, routing server frames
// to the appropriate session by ID.
//
// The caller owns the underlying *websocket.Conn and must close it when done.
type Conn struct {
	ws      *websocket.Conn
	writeCh chan []byte
	cancel  context.CancelFunc // cancels the internal context

	mu       sync.Mutex
	sessions map[string]*clientSession

	nextID atomic.Uint64
	done   chan struct{}
	err    error // first fatal error
}

// NewConn creates a multiplexed client connection over the given WebSocket.
// It starts internal read and write loops that run until the connection is
// closed or an error occurs.
func NewConn(ctx context.Context, ws *websocket.Conn) *Conn {
	innerCtx, cancel := context.WithCancel(ctx) //nolint:gosec // cancel is stored in Conn and called on disconnect

	c := &Conn{
		ws:       ws,
		writeCh:  make(chan []byte, writeBufSize),
		cancel:   cancel,
		sessions: make(map[string]*clientSession),
		done:     make(chan struct{}),
	}

	var wg sync.WaitGroup
	wg.Add(2) //nolint:mnd // readLoop + writeLoop

	go func() {
		defer wg.Done()
		c.readLoop(innerCtx)
		cancel() // stop writeLoop if readLoop exits first
	}()
	go func() {
		defer wg.Done()
		c.writeLoop(innerCtx)
	}()

	go func() {
		wg.Wait()
		c.mu.Lock()
		for _, sess := range c.sessions {
			close(sess.recvCh)
		}
		c.sessions = nil
		c.mu.Unlock()
		close(c.done)
	}()

	return c
}

// Close closes the underlying WebSocket connection.
func (c *Conn) Close() error {
	return c.ws.Close(websocket.StatusNormalClosure, "")
}

// closeErr returns the connection-level error, or a generic error if none.
func (c *Conn) closeErr() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return c.err
	}
	return procframe.NewError(procframe.CodeUnavailable, "connection closed")
}

// newSession allocates a session ID and registers the session.
func (c *Conn) newSession() (string, <-chan outboundFrame) {
	id := fmt.Sprintf("s-%d", c.nextID.Add(1))
	sess := &clientSession{
		recvCh: make(chan outboundFrame, recvChBufSize),
	}
	c.mu.Lock()
	if c.sessions != nil {
		c.sessions[id] = sess
	}
	c.mu.Unlock()
	return id, sess.recvCh
}

// removeSession unregisters a session.
func (c *Conn) removeSession(id string) {
	c.mu.Lock()
	delete(c.sessions, id)
	c.mu.Unlock()
}

// readLoop reads outbound frames from the WS connection and routes them
// to the appropriate session by ID.
func (c *Conn) readLoop(ctx context.Context) {
	for {
		_, data, rErr := c.ws.Read(ctx)
		if rErr != nil {
			c.mu.Lock()
			if c.err == nil {
				c.err = rErr
			}
			c.mu.Unlock()
			return
		}
		var frame outboundFrame
		if uErr := json.Unmarshal(data, &frame); uErr != nil {
			continue
		}
		c.mu.Lock()
		sess, ok := c.sessions[frame.ID]
		c.mu.Unlock()
		if !ok {
			continue
		}
		select {
		case sess.recvCh <- frame:
		case <-ctx.Done():
			return
		}
	}
}

// writeLoop drains writeCh and sends each message to the WS connection.
// It exits when the internal context is cancelled or a write error occurs.
// Senders blocked on writeCh will be unblocked when c.done is closed.
func (c *Conn) writeLoop(ctx context.Context) {
	for {
		select {
		case data, ok := <-c.writeCh:
			if !ok {
				return
			}
			if wErr := c.ws.Write(ctx, websocket.MessageText, data); wErr != nil {
				c.mu.Lock()
				if c.err == nil {
					c.err = wErr
				}
				c.mu.Unlock()
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// send marshals a frame and enqueues it for writing.
func (c *Conn) send(frame *inboundFrame) error {
	data, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	select {
	case c.writeCh <- data:
		return nil
	case <-c.done:
		return c.closeErr()
	}
}

// sendOpen sends an open frame for the given session.
func (c *Conn) sendOpen(id, procedure, shape string) error {
	return c.send(&inboundFrame{
		Type:      frameTypeOpen,
		ID:        id,
		Procedure: procedure,
		Shape:     shape,
	})
}

// sendMessage sends a message frame with the given payload.
func (c *Conn) sendMessage(id string, payload json.RawMessage) error {
	return c.send(&inboundFrame{
		Type:    frameTypeMessage,
		ID:      id,
		Payload: payload,
	})
}

// sendClose sends a close frame for the given session.
func (c *Conn) sendClose(id string) error {
	return c.send(&inboundFrame{
		Type: frameTypeClose,
		ID:   id,
	})
}

// sendCancel sends a cancel frame for the given session.
func (c *Conn) sendCancel(id string) error {
	return c.send(&inboundFrame{
		Type: frameTypeCancel,
		ID:   id,
	})
}
