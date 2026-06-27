package ws_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/ramdanaguss/selaras/server/internal/adapter/ws"
	domain "github.com/ramdanaguss/selaras/server/internal/domain/board"
)

// fakeAuth verifies tokens against a fixed map (stands in for appauth.Service).
type fakeAuth struct{ users map[string]string }

func (f fakeAuth) Authenticate(token string) (string, error) {
	if userID, ok := f.users[token]; ok {
		return userID, nil
	}
	return "", errors.New("invalid token")
}

// realtimeFixture spins up a wsHub + ws handler behind an httptest server.
func realtimeFixture(t *testing.T) (server *httptest.Server, wsHub *ws.Hub) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	wsHub = ws.NewHub(logger)
	ctx, cancel := context.WithCancel(context.Background())
	go wsHub.Run(ctx)

	auth := fakeAuth{users: map[string]string{"token-a": "user-a", "token-b": "user-b", "token-x": "user-x"}}
	members := map[string]map[string]bool{
		"board-1": {"user-a": true, "user-b": true},
		"board-2": {"user-a": true},
	}
	isMember := func(_ context.Context, boardID, userID string) (bool, error) {
		return members[boardID][userID], nil
	}
	handler := ws.NewHandler(wsHub, auth, isMember, nil, logger)

	server = httptest.NewServer(handler)
	t.Cleanup(func() {
		server.Close()
		cancel()
	})
	return server, wsHub
}

func wsURL(server *httptest.Server, boardID string) string {
	return "ws" + strings.TrimPrefix(server.URL, "http") + "/?board=" + boardID
}

// dialAndAuth connects, sends the first-message auth, and reads the welcome.
func dialAndAuth(t *testing.T, server *httptest.Server, boardID, token string) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	socket, _, err := websocket.Dial(ctx, wsURL(server, boardID), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if err := wsjson.Write(ctx, socket, map[string]string{"type": "auth", "token": token}); err != nil {
		t.Fatalf("auth write: %v", err)
	}
	var welcome struct {
		Type   string `json:"type"`
		ConnID string `json:"connId"`
	}
	if err := wsjson.Read(ctx, socket, &welcome); err != nil {
		t.Fatalf("welcome read: %v", err)
	}
	if welcome.Type != "welcome" || welcome.ConnID == "" {
		t.Fatalf("unexpected welcome: %+v", welcome)
	}
	return socket
}

func readType(t *testing.T, socket *websocket.Conn) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var received struct {
		Type string `json:"type"`
	}
	if err := wsjson.Read(ctx, socket, &received); err != nil {
		t.Fatalf("read event: %v", err)
	}
	return received.Type
}

func TestAuthThenFanOut(t *testing.T) {
	server, wsHub := realtimeFixture(t)

	alice := dialAndAuth(t, server, "board-1", "token-a")
	defer func() { _ = alice.CloseNow() }()
	bravo := dialAndAuth(t, server, "board-1", "token-b")
	defer func() { _ = bravo.CloseNow() }()

	// Simulate a committed REST mutation publishing its event.
	wsHub.Broadcast([]domain.Event{domain.CardMoved{BoardID: "board-1", CardID: "card-1", ToColumnID: "col-2", Position: "V"}}, "user-a", "conn-a")

	if got := readType(t, alice); got != "card.moved" {
		t.Errorf("alice received %q, want card.moved", got)
	}
	if got := readType(t, bravo); got != "card.moved" {
		t.Errorf("bravo received %q, want card.moved", got)
	}
}

func TestRoomsAreIsolated(t *testing.T) {
	server, wsHub := realtimeFixture(t)

	onBoard2 := dialAndAuth(t, server, "board-2", "token-a")
	defer func() { _ = onBoard2.CloseNow() }()

	wsHub.Broadcast([]domain.Event{domain.CardDeleted{BoardID: "board-1", CardID: "card-1"}}, "user-a", "conn-a")

	// The board-2 subscriber must not receive a board-1 event.
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	var ignored struct {
		Type string `json:"type"`
	}
	if err := wsjson.Read(ctx, onBoard2, &ignored); err == nil {
		t.Errorf("board-2 subscriber received a board-1 event: %q", ignored.Type)
	}
}

func TestNonMemberRejectedWith4401(t *testing.T) {
	server, _ := realtimeFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// user-x is authenticated but not a member of board-1.
	socket, _, err := websocket.Dial(ctx, wsURL(server, "board-1"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = socket.CloseNow() }()
	if err := wsjson.Write(ctx, socket, map[string]string{"type": "auth", "token": "token-x"}); err != nil {
		t.Fatalf("auth write: %v", err)
	}

	_, _, readErr := socket.Read(ctx)
	if websocket.CloseStatus(readErr) != 4401 {
		t.Errorf("close status = %v, want 4401", websocket.CloseStatus(readErr))
	}
}

func TestConnectionsDoNotLeakGoroutines(t *testing.T) {
	server, _ := realtimeFixture(t)

	// Warm up so one-time goroutines (httptest, transport) are already counted.
	_ = dialAndAuth(t, server, "board-1", "token-a").CloseNow()
	time.Sleep(100 * time.Millisecond)
	baseline := runtime.NumGoroutine()

	for iteration := 0; iteration < 20; iteration++ {
		socket := dialAndAuth(t, server, "board-1", "token-a")
		_ = socket.CloseNow()
	}

	// The pumps die with their connection (design D10): the count returns to ~baseline.
	settled := false
	for attempt := 0; attempt < 50; attempt++ {
		if runtime.NumGoroutine() <= baseline+2 {
			settled = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !settled {
		t.Errorf("goroutines did not settle: baseline %d, now %d", baseline, runtime.NumGoroutine())
	}
}

// ensure the handler is a plain http.Handler (mountable on the chi router).
var _ http.Handler = (*ws.Handler)(nil)
