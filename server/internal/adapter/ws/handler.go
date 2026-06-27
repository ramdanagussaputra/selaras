package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
)

// authTimeout is the window for a client to send its first-message auth (spec).
const authTimeout = 5 * time.Second

// Authenticator verifies an access token and returns its subject user id (the
// existing appauth.Service satisfies this — no new auth path, design D6).
type Authenticator interface {
	Authenticate(accessToken string) (userID string, err error)
}

// MembershipChecker reports whether a user may join a board's room (the board
// repository's membership lookup, adapted to a bool in wiring).
type MembershipChecker func(ctx context.Context, boardID, userID string) (bool, error)

// Handler upgrades GET /api/v1/ws?board={id} to a WebSocket, authenticates the
// first message, gates on board membership, and joins the connection to the
// hub's room for that board. The socket is broadcast-only — it never accepts a
// mutation (PRD invariant); the only client frames are auth (handled here) and
// ping (handled by the read pump).
type Handler struct {
	hub            *Hub
	authenticator  Authenticator
	isMember       MembershipChecker
	originPatterns []string
	logger         *slog.Logger
}

// NewHandler constructs the upgrade handler. originPatterns authorizes the
// browser origin (the configured CORS origin in dev; same-origin in prod).
func NewHandler(hub *Hub, authenticator Authenticator, isMember MembershipChecker, originPatterns []string, logger *slog.Logger) *Handler {
	return &Handler{hub: hub, authenticator: authenticator, isMember: isMember, originPatterns: originPatterns, logger: logger}
}

type authMessage struct {
	Type  string `json:"type"`
	Token string `json:"token"`
}

type welcomeMessage struct {
	Type   string `json:"type"`
	ConnID string `json:"connId"`
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	boardID := r.URL.Query().Get("board")
	if boardID == "" {
		http.Error(w, "missing board query parameter", http.StatusBadRequest)
		return
	}

	socket, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: h.originPatterns})
	if err != nil {
		return // Accept already wrote the failure
	}
	defer func() { _ = socket.CloseNow() }()

	// The pumps live on an independent context cancelled by the hub (on removal or
	// shutdown) — not the request context, which ends when ServeHTTP returns.
	connCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	userID, ok := h.authenticate(connCtx, socket, boardID)
	if !ok {
		return // authenticate already closed the socket with 4401
	}

	connection := &conn{
		id:      uuid.NewString(),
		userID:  userID,
		boardID: boardID,
		socket:  socket,
		send:    make(chan []byte, sendBufferSize),
		cancel:  cancel,
	}

	if !h.hub.add(connCtx, connection) {
		_ = socket.Close(websocket.StatusGoingAway, "server shutting down")
		return
	}

	if err := writeJSON(connCtx, socket, welcomeMessage{Type: "welcome", ConnID: connection.id}); err != nil {
		h.hub.remove(connCtx, connection)
		return
	}

	go connection.writePump(connCtx)
	connection.readPump(connCtx) // blocks until the connection closes
	h.hub.remove(connCtx, connection)
}

// authenticate enforces the first-message auth handshake within authTimeout and
// the membership gate, closing the socket with 4401 on any failure (design D6).
func (h *Handler) authenticate(ctx context.Context, socket *websocket.Conn, boardID string) (userID string, ok bool) {
	authCtx, cancel := context.WithTimeout(ctx, authTimeout)
	defer cancel()

	_, data, err := socket.Read(authCtx)
	if err != nil {
		_ = socket.Close(closeAuth, "auth timeout")
		return "", false
	}

	var message authMessage
	if json.Unmarshal(data, &message) != nil || message.Type != "auth" {
		_ = socket.Close(closeAuth, "expected auth message")
		return "", false
	}

	userID, err = h.authenticator.Authenticate(message.Token)
	if err != nil {
		_ = socket.Close(closeAuth, "invalid token")
		return "", false
	}

	member, err := h.isMember(ctx, boardID, userID)
	if err != nil {
		h.logger.ErrorContext(ctx, "ws membership check failed", "error", err.Error())
		_ = socket.Close(websocket.StatusInternalError, "membership check failed")
		return "", false
	}
	if !member {
		_ = socket.Close(closeAuth, "not a board member")
		return "", false
	}
	return userID, true
}

// writeJSON marshals and writes a text frame.
func writeJSON(ctx context.Context, socket *websocket.Conn, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return socket.Write(ctx, websocket.MessageText, data)
}
