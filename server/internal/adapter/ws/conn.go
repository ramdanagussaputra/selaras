// Package ws holds the WebSocket adapter: an in-process hub with per-board rooms
// that fans committed board mutations out to connected members. It is broadcast-
// only — all writes stay on REST (PRD invariant). The hub owns its room map on a
// single goroutine and communicates with connections over channels (design D2),
// so it carries no locks and passes `go test -race`.
package ws

import (
	"context"
	"encoding/json"
	"time"

	"github.com/coder/websocket"
)

const (
	// sendBufferSize bounds a connection's outbound queue. A full buffer means the
	// client has stopped reading; the hub drops it rather than stall the room (D3).
	sendBufferSize = 64
	// heartbeatInterval / pongTimeout implement the spec's 30 s ping / 60 s reap.
	heartbeatInterval = 30 * time.Second
	pongTimeout       = 60 * time.Second
)

// Application-level close codes from the protocol (spec). Control-frame codes
// like going-away (1001) come from the websocket package.
const (
	closeBadFrame = 4400 // a client frame that is neither auth nor ping
	closeAuth     = 4401 // missing/invalid/expired token, non-member, or auth timeout
)

// conn is one client connection: its identity, its room (boardID), the socket,
// and a bounded send channel drained by the write pump. Only the hub goroutine
// mutates room membership; the pumps only read/write the socket and the channel.
type conn struct {
	id      string
	userID  string
	boardID string
	socket  *websocket.Conn
	send    chan []byte
	cancel  context.CancelFunc // cancels both pumps; called by the hub on removal/shutdown
}

// clientMessage is the only shape a client may send after auth: a keepalive ping.
// (The initial auth message is handled before the read pump starts.)
type clientMessage struct {
	Type string `json:"type"`
}

// readPump reads client frames until the connection closes. Post-auth the only
// legal frame is `{"type":"ping"}`; anything else closes the socket with 4400.
// It also drives pong processing for the write pump's heartbeat (the websocket
// library handles control frames as a side effect of Read being called).
func (c *conn) readPump(ctx context.Context) {
	for {
		messageType, data, err := c.socket.Read(ctx)
		if err != nil {
			return // disconnect, context cancelled, or a closed socket
		}
		if messageType != websocket.MessageText {
			_ = c.socket.Close(closeBadFrame, "unexpected binary frame")
			return
		}
		var message clientMessage
		if json.Unmarshal(data, &message) != nil || message.Type != "ping" {
			_ = c.socket.Close(closeBadFrame, "unexpected message")
			return
		}
	}
}

// writePump drains the send channel to the socket and owns the heartbeat. It ends
// when the context is cancelled (hub removal or shutdown → close 1001) or the send
// channel is closed by the hub (normal removal). A heartbeat with no pong inside
// pongTimeout closes the connection.
func (c *conn) writePump(ctx context.Context) {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			_ = c.socket.Close(websocket.StatusGoingAway, "server shutting down")
			return
		case data, open := <-c.send:
			if !open {
				_ = c.socket.Close(websocket.StatusNormalClosure, "")
				return
			}
			if err := c.socket.Write(ctx, websocket.MessageText, data); err != nil {
				return
			}
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, pongTimeout)
			err := c.socket.Ping(pingCtx)
			cancel()
			if err != nil {
				_ = c.socket.Close(websocket.StatusPolicyViolation, "heartbeat timeout")
				return
			}
		}
	}
}
