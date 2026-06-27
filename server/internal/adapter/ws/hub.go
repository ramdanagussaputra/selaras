package ws

import (
	"context"
	"log/slog"

	appboard "github.com/ramdanaguss/selaras/server/internal/app/board"
	domain "github.com/ramdanaguss/selaras/server/internal/domain/board"
)

// broadcastQueueSize bounds the hub's inbound queue of broadcast requests from
// handlers. A full queue (extreme backpressure) drops a broadcast and logs it;
// clients re-converge on their next reconnect's resync, so it's safe (design D9).
const broadcastQueueSize = 256

// Hub is the in-process WebSocket hub. Its room map is owned by the single Run
// goroutine and mutated only there; connections and handlers interact with it
// exclusively through channels (design D2). That is what makes it lock-free and
// race-free (AC-7).
type Hub struct {
	logger     *slog.Logger
	register   chan *conn
	unregister chan *conn
	broadcast  chan outbound
	rooms      map[string]map[*conn]struct{} // boardID -> connections; touched only by Run
}

// outbound is a single pre-marshalled envelope addressed to one board's room.
type outbound struct {
	boardID string
	data    []byte
}

// Compile-time proof the hub satisfies the app-layer Broadcaster port.
var _ appboard.Broadcaster = (*Hub)(nil)

// NewHub constructs a hub. Call Run to start its goroutine.
func NewHub(logger *slog.Logger) *Hub {
	return &Hub{
		logger:     logger,
		register:   make(chan *conn),
		unregister: make(chan *conn),
		broadcast:  make(chan outbound, broadcastQueueSize),
		rooms:      make(map[string]map[*conn]struct{}),
	}
}

// Run owns the room map until ctx is cancelled, at which point it closes every
// connection (1001) and returns. It must never block on a connection: broadcast
// uses a non-blocking send so one slow consumer can't stall the room (D3).
func (h *Hub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			h.closeAll()
			return

		case connection := <-h.register:
			room := h.rooms[connection.boardID]
			if room == nil {
				room = make(map[*conn]struct{})
				h.rooms[connection.boardID] = room
			}
			room[connection] = struct{}{}

		case connection := <-h.unregister:
			h.removeConn(connection)

		case message := <-h.broadcast:
			for connection := range h.rooms[message.boardID] {
				select {
				case connection.send <- message.data:
				default:
					// Full buffer ⇒ a stalled reader: drop it, leave the room intact.
					h.logger.Warn("dropping slow websocket consumer",
						"board", message.boardID, "conn", connection.id)
					h.removeConn(connection)
				}
			}
		}
	}
}

// removeConn deletes a connection from its room and releases it. It is called
// only from the Run goroutine and is idempotent (a connection removed twice — by
// a slow-consumer drop and its own read-error unregister — closes its send once).
func (h *Hub) removeConn(connection *conn) {
	room, ok := h.rooms[connection.boardID]
	if !ok {
		return
	}
	if _, present := room[connection]; !present {
		return
	}
	delete(room, connection)
	if len(room) == 0 {
		delete(h.rooms, connection.boardID)
	}
	close(connection.send) // ends the write pump
	connection.cancel()    // ends the read pump
}

// closeAll releases every connection on shutdown.
func (h *Hub) closeAll() {
	for boardID, room := range h.rooms {
		for connection := range room {
			close(connection.send)
			connection.cancel()
		}
		delete(h.rooms, boardID)
	}
}

// add registers a connection, or reports false if the hub is already shutting
// down (so the caller doesn't block forever on a stopped Run goroutine).
func (h *Hub) add(ctx context.Context, connection *conn) bool {
	select {
	case h.register <- connection:
		return true
	case <-ctx.Done():
		return false
	}
}

// remove unregisters a connection when its read pump exits. It gives up if the
// hub has stopped, so a shutdown can't leave this goroutine blocked.
func (h *Hub) remove(ctx context.Context, connection *conn) {
	select {
	case h.unregister <- connection:
	case <-ctx.Done():
	}
}

// Broadcast implements the app-layer Broadcaster port. It maps each domain event
// to its wire envelope in the *calling* goroutine (keeping the hub goroutine
// light), then hands each to the hub addressed to the event's own board (D4).
// The enqueue is non-blocking, so a slow hub never stalls the HTTP handler (D8).
func (h *Hub) Broadcast(events []domain.Event, actorUserID, actorConnID string) {
	for _, event := range events {
		data, boardID, ok, err := marshalEnvelope(event, actorUserID, actorConnID)
		if err != nil {
			h.logger.Error("encoding broadcast envelope", "error", err.Error())
			continue
		}
		if !ok {
			continue // event has no M4 broadcast type (board create/delete)
		}
		select {
		case h.broadcast <- outbound{boardID: boardID, data: data}:
		default:
			h.logger.Warn("broadcast queue full, dropping event", "board", boardID)
		}
	}
}
