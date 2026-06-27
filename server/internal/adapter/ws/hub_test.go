package ws

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	domain "github.com/ramdanaguss/selaras/server/internal/domain/board"
)

func testHub(t *testing.T) *Hub {
	t.Helper()
	wsHub := NewHub(slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go wsHub.Run(ctx)
	return wsHub
}

// testConn builds a connection with a buffered send channel and a real context,
// so a test can register it with the wsHub (no socket needed — Run never touches
// it) and observe cancellation when the wsHub drops or closes it.
func testConn(boardID string, buffer int) (*conn, context.Context) {
	ctx, cancel := context.WithCancel(context.Background())
	return &conn{
		id:      boardID + "-" + time.Now().Format("150405.000000000"),
		boardID: boardID,
		send:    make(chan []byte, buffer),
		cancel:  cancel,
	}, ctx
}

func recv(t *testing.T, channel chan []byte) envelope {
	t.Helper()
	select {
	case data := <-channel:
		var env envelope
		if err := json.Unmarshal(data, &env); err != nil {
			t.Fatalf("decoding envelope: %v", err)
		}
		return env
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for a broadcast")
		return envelope{}
	}
}

func recvNothing(t *testing.T, channel chan []byte) {
	t.Helper()
	select {
	case <-channel:
		t.Fatal("received a broadcast that should not have arrived")
	case <-time.After(100 * time.Millisecond):
	}
}

func waitCancelled(ctx context.Context, t *testing.T) {
	t.Helper()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("connection was not cancelled")
	}
}

func TestHubBroadcastReachesOnlyItsRoom(t *testing.T) {
	wsHub := testHub(t)
	first, _ := testConn("board-1", 4)
	second, _ := testConn("board-1", 4)
	other, _ := testConn("board-2", 4)
	for _, connection := range []*conn{first, second, other} {
		wsHub.add(context.Background(), connection)
	}

	wsHub.Broadcast([]domain.Event{domain.CardMoved{BoardID: "board-1", CardID: "card-9", ToColumnID: "col-2", Position: "V"}}, "user-a", "conn-a")

	for _, channel := range []chan []byte{first.send, second.send} {
		env := recv(t, channel)
		if env.Type != "card.moved" || env.BoardID != "board-1" || env.Payload.CardID != "card-9" {
			t.Errorf("unexpected envelope: %+v", env)
		}
		if env.Actor.ConnID != "conn-a" {
			t.Errorf("actor.connId = %q, want conn-a (echo suppression metadata)", env.Actor.ConnID)
		}
	}
	recvNothing(t, other.send) // a different board's room must not receive it
}

func TestHubDropsSlowConsumerWithoutAffectingRoom(t *testing.T) {
	wsHub := testHub(t)
	slow, slowCtx := testConn("board-1", 1)
	healthy, _ := testConn("board-1", 4)
	wsHub.add(context.Background(), slow)
	wsHub.add(context.Background(), healthy)
	slow.send <- []byte("backlog") // fill the slow consumer's only buffer slot

	wsHub.Broadcast([]domain.Event{domain.CardDeleted{BoardID: "board-1", CardID: "card-1"}}, "user-a", "conn-a")

	// The healthy member still gets the event...
	if env := recv(t, healthy.send); env.Type != "card.deleted" {
		t.Errorf("healthy member type = %q, want card.deleted", env.Type)
	}
	// ...and the stalled one is dropped (its context is cancelled).
	waitCancelled(slowCtx, t)
}

func TestHubUnregisterRemovesFromRoom(t *testing.T) {
	wsHub := testHub(t)
	connection, ctx := testConn("board-1", 4)
	wsHub.add(context.Background(), connection)
	wsHub.remove(context.Background(), connection)
	waitCancelled(ctx, t) // removal cancels the connection and closes its send channel

	// A broadcast after removal must not reach (or panic on) the gone connection.
	wsHub.Broadcast([]domain.Event{domain.CardCreated{BoardID: "board-1", CardID: "card-1", ColumnID: "col-1", Position: "V"}}, "user-a", "conn-a")
	if _, open := <-connection.send; open {
		t.Fatal("removed connection received a real broadcast")
	}
}

func TestHubShutdownClosesAllConnections(t *testing.T) {
	wsHub := NewHub(slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	go wsHub.Run(ctx)

	first, firstCtx := testConn("board-1", 4)
	second, secondCtx := testConn("board-2", 4)
	wsHub.add(context.Background(), first)
	wsHub.add(context.Background(), second)

	cancel() // shutdown
	waitCancelled(firstCtx, t)
	waitCancelled(secondCtx, t)
}

func TestHubSkipsNonBroadcastEvents(t *testing.T) {
	wsHub := testHub(t)
	connection, _ := testConn("board-1", 4)
	wsHub.add(context.Background(), connection)

	// Board create/delete have no M4 wire type; they must not be delivered.
	wsHub.Broadcast([]domain.Event{domain.Created{BoardID: "board-1", OwnerID: "user-a"}}, "user-a", "conn-a")
	recvNothing(t, connection.send)
}
