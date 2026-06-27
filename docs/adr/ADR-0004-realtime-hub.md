# ADR-0004: realtime-hub

**Status**: Proposed
**Version**: 1.0
**Date**: 2026-06-27
**Author**: Ramdan Agus Saputra

## Context

M4 adds live multi-user sync — the project's interview centerpiece (see
[PRD](../selaras-prd.md) Story 3 / Feature 3 and [spec 04-realtime](../specs/04-realtime.md)).
Two people on one board must see each other's changes within ~500 ms, the server
must stay correct under concurrent connects/disconnects/broadcasts (the spec
demands `go test -race` clean), and none of it may compromise the existing write
path. The forces in tension:

- **One write path, one auth path** vs a second ingress — adding a socket that
  also accepts mutations would duplicate validation, authorization, and the
  transaction boundary, and invite the two paths to diverge.
- **Concurrency correctness without a coordinator** — many connections register,
  unregister, and receive broadcasts at once against shared room state; this is
  exactly where data races and lock-ordering deadlocks live.
- **One slow client must not stall a room** — a browser that stops reading cannot
  be allowed to block the goroutine that fans out to everyone else.
- **Single instance, honestly** — there is no Redis or message bus at this scale;
  the design must be correct for one process and leave a clean seam for later
  horizontal fan-out.

This ADR records the realtime design: broadcast-only WebSocket, the hub's
concurrency model, backpressure, and why multi-instance fan-out is deferred.

## Decision

**We will run an in-process WebSocket hub that is broadcast-only — all writes stay
on REST — with the room map owned by a single goroutine driven by channels, per-
connection pumps over bounded send buffers, and post-commit fan-out of the domain
events the M3 use cases already return.**

1. **Broadcast-only WebSocket (the architectural invariant).** `GET /api/v1/ws?board={id}`
   upgrades, authenticates on its first message (`{type:auth, token}` within 5 s,
   reusing the M2 access-token verification and the M3 board-membership check),
   joins a per-board room, and from then on only *sends*. The sole client→server
   frames are `auth` and `ping`; anything else closes `4400`. Every mutation still
   goes through the existing REST endpoints — one write path, one auth path. The
   HTTP handlers publish the `[]domain.Event` the use cases return to a
   `Broadcaster` port **after** the transaction commits.

2. **Concurrency model: a single hub goroutine owns the room map (actor model),
   not a mutex.** `map[boardID]map[*conn]struct{}` is mutated only by the hub's
   `Run` goroutine, which serves `register` / `unregister` / `broadcast` channels
   in one `select` loop. Connections never touch the map. There is **no lock**.
   Broadcasting does a **non-blocking** send to each connection's buffered channel.

3. **Per-connection pumps + bounded send buffer (backpressure).** Each connection
   runs a read pump (reads `ping`, detects disconnect) and a write pump (drains a
   64-deep send channel, plus a 30 s heartbeat ping / 60 s pong-timeout reap). When
   a connection's buffer is full during a broadcast, the hub drops that connection
   and moves on — one stalled reader never delays or fails delivery to the rest.

4. **Lifecycle tied to context.** Each connection's pumps run on a context the hub
   cancels on removal or shutdown; closing the connection cancels the context,
   which ends both pumps — so no goroutine outlives its connection (verified by a
   leak test). Graceful shutdown closes every connection with `1001`.

5. **Client: patch then resync.** `useBoardChannel` patches the TanStack Query
   cache on each event for an instant, flicker-free update, suppresses the echo of
   its own connection (via the `X-Conn-Id` it sent on the originating REST call),
   and on every reconnect (jittered backoff) invalidates the board query for a full
   authoritative resync that covers anything missed.

## Alternatives considered

- **A `sync.RWMutex`-guarded room map** — the obvious alternative, and it works,
  but every broadcast holds the lock while iterating a room, and the natural
  implementation (hold the lock, push to each connection's channel) sets up a
  lock-ordering deadlock the instant a send blocks. Serializing all map access
  through one goroutine removes the lock entirely and makes the concurrency story
  linear: "only the hub touches the map, one operation at a time." Chosen for that
  clarity and its clean `-race` story; the mutex is the documented fallback if the
  single goroutine ever became a throughput bottleneck (it won't at this scale).
- **Mutations over the WebSocket (client→server writes)** — fewer round-trips, but
  it duplicates the REST write path's validation, authorization, and transaction
  handling on a second ingress and invites drift. Rejected to preserve the one-
  write-path invariant; the socket only fans out.
- **Unbounded send channels / blocking sends** — simpler, but a single slow or
  malicious reader would back up the hub and stall the whole room. Rejected in
  favor of bounded buffers with drop-on-full (a connection is the unit of failure).
- **Broadcast before commit (from the use case)** — would shave latency, but if the
  transaction then rolled back, peers would render a change that never persisted
  and diverge until their next resync. Rejected; publishing strictly post-commit
  makes every broadcast a faithful echo of committed state.
- **Redis pub/sub for multi-instance fan-out now** — necessary the day Selaras runs
  more than one API instance (rooms would otherwise be per-process), but it is a
  real dependency and operational surface we don't need at single-instance demo
  scale. Deferred: the `Broadcaster` **port** is the seam where a Redis-backed
  implementation slots in later without touching the handlers or the domain.
- **A heavier WS library (`gorilla/websocket`)** — mature but pre-`context`; for a
  hub whose correctness is about cancellation and deadlines, `coder/websocket`'s
  context-native API is the better fit (and a cleaner teaching artifact).

## Consequences

- A move travels REST-commit → `Broadcaster` → hub goroutine → each room member's
  buffered channel → write pump → peer, with the client patching its cache on
  arrival — typically well under the 500 ms target on one instance.
- The hub is lock-free and passes `go test -race`; its correctness rests on a
  single, easily-explained rule (one goroutine owns the map), which is the exact
  thing an interviewer will probe.
- A slow consumer degrades only itself (dropped within one broadcast cycle); the
  room is unaffected.
- The system is correct for exactly one process. A second instance would not share
  rooms — accepted and documented; the `Broadcaster` port is the upgrade seam.
- Events now carry their `BoardID` (the four card events gained the field) so the
  adapter can route them; the domain stays transport-free (routing lives in the
  adapter's event→envelope mapping switch).
- **Revisit trigger.** Adopt Redis (or another shared bus) behind the `Broadcaster`
  port the moment Selaras needs a second API instance, or when a single hub
  goroutine measurably bottlenecks broadcast throughput. Reconsider WS-ingress for
  mutations only if REST round-trip latency becomes a demonstrated UX problem
  (it won't at this scale).
