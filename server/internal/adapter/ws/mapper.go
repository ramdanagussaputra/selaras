package ws

import (
	"encoding/json"
	"time"

	domain "github.com/ramdanaguss/selaras/server/internal/domain/board"
)

// protocolVersion is the envelope schema version (spec: `v:1`).
const protocolVersion = 1

// envelope is the server→client message shape (spec Protocol).
type envelope struct {
	Version int     `json:"v"`
	Type    string  `json:"type"`
	BoardID string  `json:"boardId"`
	Actor   actor   `json:"actor"`
	Payload payload `json:"payload"`
	Time    string  `json:"ts"`
}

type actor struct {
	UserID string `json:"userId"`
	ConnID string `json:"connId"`
}

// payload carries the identifiers a client needs to patch its cache. Moves and
// deletes are applied directly from these fields; creates and content updates are
// the client's cue to invalidate and refetch the authoritative entity (design D5
// — the broadcast path stays read-free and the reconnect/invalidate machinery is
// the correctness backstop, design D9). Empty fields are omitted.
type payload struct {
	ColumnID     string `json:"columnId,omitempty"`
	CardID       string `json:"cardId,omitempty"`
	FromColumnID string `json:"fromColumnId,omitempty"`
	ToColumnID   string `json:"toColumnId,omitempty"`
	Position     string `json:"position,omitempty"`
}

// describe maps a domain event to its room key (boardID), wire type, and payload.
// ok is false for events with no M4 broadcast type (board create/delete: the room
// is empty on create and vanishing on delete — only board.updated is in scope
// this milestone). Extracting the boardID here keeps the routing knowledge beside
// the protocol mapping, so the domain needs no transport-facing accessor.
func describe(event domain.Event) (boardID, eventType string, body payload, ok bool) {
	switch typed := event.(type) {
	case domain.Updated:
		return typed.BoardID, "board.updated", payload{}, true
	case domain.ColumnCreated:
		return typed.BoardID, "column.created", payload{ColumnID: typed.ColumnID, Position: string(typed.Position)}, true
	case domain.ColumnRenamed:
		return typed.BoardID, "column.updated", payload{ColumnID: typed.ColumnID}, true
	case domain.ColumnMoved:
		return typed.BoardID, "column.moved", payload{ColumnID: typed.ColumnID, Position: string(typed.Position)}, true
	case domain.ColumnDeleted:
		return typed.BoardID, "column.deleted", payload{ColumnID: typed.ColumnID}, true
	case domain.CardCreated:
		return typed.BoardID, "card.created", payload{CardID: typed.CardID, ColumnID: typed.ColumnID, Position: string(typed.Position)}, true
	case domain.CardUpdated:
		return typed.BoardID, "card.updated", payload{CardID: typed.CardID}, true
	case domain.CardMoved:
		return typed.BoardID, "card.moved", payload{
			CardID: typed.CardID, FromColumnID: typed.FromColumnID,
			ToColumnID: typed.ToColumnID, Position: string(typed.Position),
		}, true
	case domain.CardDeleted:
		return typed.BoardID, "card.deleted", payload{CardID: typed.CardID}, true
	default:
		return "", "", payload{}, false
	}
}

// marshalEnvelope builds and JSON-encodes the envelope for an event, returning
// the room key to address it to, or ok=false when the event has no broadcast type.
func marshalEnvelope(event domain.Event, actorUserID, actorConnID string) (data []byte, boardID string, ok bool, err error) {
	boardID, eventType, body, ok := describe(event)
	if !ok {
		return nil, "", false, nil
	}
	data, err = json.Marshal(envelope{
		Version: protocolVersion,
		Type:    eventType,
		BoardID: boardID,
		Actor:   actor{UserID: actorUserID, ConnID: actorConnID},
		Payload: body,
		Time:    time.Now().UTC().Format(time.RFC3339),
	})
	return data, boardID, ok, err
}
