package board

import domain "github.com/ramdanaguss/selaras/server/internal/domain/board"

// Broadcaster fans a use case's domain events out to connected clients (driven
// port; the WebSocket hub adapter implements it). It is called by the HTTP layer
// *after* the mutation's transaction commits, so a rolled-back change is never
// broadcast (design D8). actorConnID is the originating connection (from the
// X-Conn-Id header) so the adapter can stamp it on the envelope for client-side
// echo suppression (design D7). Events self-route: the hub reads each event's
// Board() to pick the room (design D4).
//
// Implementations MUST be non-blocking and safe for concurrent use — a slow or
// absent consumer must never stall the calling handler.
type Broadcaster interface {
	Broadcast(events []domain.Event, actorUserID, actorConnID string)
}

// NoopBroadcaster discards events. It is the default when no hub is wired (e.g.
// tests that don't exercise realtime), so the use cases and handlers can depend
// on a non-nil Broadcaster without a nil check.
type NoopBroadcaster struct{}

// Broadcast does nothing.
func (NoopBroadcaster) Broadcast(_ []domain.Event, _, _ string) {}
