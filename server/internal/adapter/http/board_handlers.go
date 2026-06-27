package http

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	appboard "github.com/ramdanaguss/selaras/server/internal/app/board"
	domain "github.com/ramdanaguss/selaras/server/internal/domain/auth"
	board "github.com/ramdanaguss/selaras/server/internal/domain/board"
)

// connIDHeader carries the originating WebSocket connection id on REST mutations,
// so the broadcast can stamp it and that connection suppresses its own echo (D7).
const connIDHeader = "X-Conn-Id"

// BoardHandler serves the kanban endpoints under the Bearer-protected /api/v1
// subrouter. Each handler resolves the caller from the request context (injected
// by requireAuth), drives a use case, and maps the result — or a domain error —
// to the shared JSON envelope. After a mutation commits, it publishes the use
// case's []board.Event to the broadcaster for realtime fan-out (design D8).
type BoardHandler struct {
	service     *appboard.Service
	broadcaster appboard.Broadcaster
	logger      *slog.Logger
}

// NewBoardHandler constructs the handler. A nil broadcaster falls back to a no-op,
// so REST works with realtime disabled.
func NewBoardHandler(service *appboard.Service, broadcaster appboard.Broadcaster, logger *slog.Logger) *BoardHandler {
	if broadcaster == nil {
		broadcaster = appboard.NoopBroadcaster{}
	}
	return &BoardHandler{service: service, broadcaster: broadcaster, logger: logger}
}

// publish fans a mutation's events out to the board's room, stamped with the
// acting user and the originating connection id. Called only after the use case
// returns (its transaction already committed), so a rolled-back change is never
// broadcast (design D8).
func (h *BoardHandler) publish(r *http.Request, events []board.Event) {
	if len(events) == 0 {
		return
	}
	userID, _ := userIDFromContext(r.Context())
	h.broadcaster.Broadcast(events, userID, r.Header.Get(connIDHeader))
}

// --- wire shapes -------------------------------------------------------------

type boardResponse struct {
	ID        string    `json:"id"`
	OwnerID   string    `json:"ownerId"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type columnResponse struct {
	ID        string         `json:"id"`
	BoardID   string         `json:"boardId"`
	Title     string         `json:"title"`
	Position  string         `json:"position"`
	CreatedAt time.Time      `json:"createdAt"`
	Cards     []cardResponse `json:"cards"`
}

type cardResponse struct {
	ID          string    `json:"id"`
	ColumnID    string    `json:"columnId"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Position    string    `json:"position"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type boardEnvelope struct {
	Board boardResponse `json:"board"`
}

type boardsEnvelope struct {
	Boards []boardResponse `json:"boards"`
}

type boardTreeEnvelope struct {
	Board   boardResponse    `json:"board"`
	Columns []columnResponse `json:"columns"`
}

type columnEnvelope struct {
	Column columnResponse `json:"column"`
}

type cardEnvelope struct {
	Card cardResponse `json:"card"`
}

type titleRequest struct {
	Title string `json:"title"`
}

type createPositionedRequest struct {
	Title    string `json:"title"`
	Position *int   `json:"position"`
}

type columnPatchRequest struct {
	Title    *string `json:"title"`
	Position *int    `json:"position"`
}

type cardPatchRequest struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	ColumnID    *string `json:"columnId"`
	Position    *int    `json:"position"`
}

func toBoardResponse(value board.Board) boardResponse {
	return boardResponse{
		ID: value.ID, OwnerID: value.OwnerID, Title: value.Title,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func toCardResponse(value board.Card) cardResponse {
	return cardResponse{
		ID: value.ID, ColumnID: value.ColumnID, Title: value.Title, Description: value.Description,
		Position: string(value.Position), CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func toColumnResponse(value board.Column, cards []cardResponse) columnResponse {
	return columnResponse{
		ID: value.ID, BoardID: value.BoardID, Title: value.Title,
		Position: string(value.Position), CreatedAt: value.CreatedAt, Cards: cards,
	}
}

// --- boards ------------------------------------------------------------------

// ListBoards handles GET /api/v1/boards.
func (h *BoardHandler) ListBoards(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, r, h.logger, domain.ErrTokenInvalid)
		return
	}

	boards, err := h.service.ListBoards(r.Context(), userID)
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}

	responses := make([]boardResponse, len(boards))
	for index, value := range boards {
		responses[index] = toBoardResponse(value)
	}
	writeJSON(w, http.StatusOK, boardsEnvelope{Boards: responses})
}

// CreateBoard handles POST /api/v1/boards.
func (h *BoardHandler) CreateBoard(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, r, h.logger, domain.ErrTokenInvalid)
		return
	}

	var input titleRequest
	if !decodeJSON(w, r, &input) {
		return
	}

	created, events, err := h.service.CreateBoard(r.Context(), userID, input.Title)
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}
	h.publish(r, events)
	writeJSON(w, http.StatusCreated, boardEnvelope{Board: toBoardResponse(created)})
}

// GetBoard handles GET /api/v1/boards/{id}, returning the nested board tree.
func (h *BoardHandler) GetBoard(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, r, h.logger, domain.ErrTokenInvalid)
		return
	}

	tree, err := h.service.GetBoard(r.Context(), userID, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}

	columns := make([]columnResponse, len(tree.Columns))
	for index, withCards := range tree.Columns {
		cards := make([]cardResponse, len(withCards.Cards))
		for cardIndex, value := range withCards.Cards {
			cards[cardIndex] = toCardResponse(value)
		}
		columns[index] = toColumnResponse(withCards.Column, cards)
	}
	writeJSON(w, http.StatusOK, boardTreeEnvelope{Board: toBoardResponse(tree.Board), Columns: columns})
}

// RenameBoard handles PATCH /api/v1/boards/{id}.
func (h *BoardHandler) RenameBoard(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, r, h.logger, domain.ErrTokenInvalid)
		return
	}

	var input titleRequest
	if !decodeJSON(w, r, &input) {
		return
	}

	events, err := h.service.RenameBoard(r.Context(), userID, chi.URLParam(r, "id"), input.Title)
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}
	h.publish(r, events)
	w.WriteHeader(http.StatusOK)
}

// DeleteBoard handles DELETE /api/v1/boards/{id} (owner only).
func (h *BoardHandler) DeleteBoard(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, r, h.logger, domain.ErrTokenInvalid)
		return
	}

	events, err := h.service.DeleteBoard(r.Context(), userID, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}
	h.publish(r, events)
	w.WriteHeader(http.StatusNoContent)
}

// --- columns -----------------------------------------------------------------

// CreateColumn handles POST /api/v1/boards/{id}/columns.
func (h *BoardHandler) CreateColumn(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, r, h.logger, domain.ErrTokenInvalid)
		return
	}

	var input createPositionedRequest
	if !decodeJSON(w, r, &input) {
		return
	}

	created, events, err := h.service.CreateColumn(r.Context(), userID, chi.URLParam(r, "id"), input.Title, input.Position)
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}
	h.publish(r, events)
	writeJSON(w, http.StatusCreated, columnEnvelope{Column: toColumnResponse(created, []cardResponse{})})
}

// UpdateColumn handles PATCH /api/v1/columns/{id}: a title change renames, a
// position change reorders, and a body with both does both.
func (h *BoardHandler) UpdateColumn(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, r, h.logger, domain.ErrTokenInvalid)
		return
	}

	var input columnPatchRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	columnID := chi.URLParam(r, "id")

	var events []board.Event
	if input.Title != nil {
		renamed, err := h.service.RenameColumn(r.Context(), userID, columnID, *input.Title)
		if err != nil {
			writeError(w, r, h.logger, err)
			return
		}
		events = append(events, renamed...)
	}
	if input.Position != nil {
		moved, err := h.service.ReorderColumn(r.Context(), userID, columnID, input.Position)
		if err != nil {
			writeError(w, r, h.logger, err)
			return
		}
		events = append(events, moved...)
	}
	h.publish(r, events)
	w.WriteHeader(http.StatusOK)
}

// DeleteColumn handles DELETE /api/v1/columns/{id} (cascades cards).
func (h *BoardHandler) DeleteColumn(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, r, h.logger, domain.ErrTokenInvalid)
		return
	}

	events, err := h.service.DeleteColumn(r.Context(), userID, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}
	h.publish(r, events)
	w.WriteHeader(http.StatusNoContent)
}

// --- cards -------------------------------------------------------------------

// CreateCard handles POST /api/v1/columns/{id}/cards.
func (h *BoardHandler) CreateCard(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, r, h.logger, domain.ErrTokenInvalid)
		return
	}

	var input createPositionedRequest
	if !decodeJSON(w, r, &input) {
		return
	}

	created, events, err := h.service.CreateCard(r.Context(), userID, chi.URLParam(r, "id"), input.Title, input.Position)
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}
	h.publish(r, events)
	writeJSON(w, http.StatusCreated, cardEnvelope{Card: toCardResponse(created)})
}

// UpdateCard handles PATCH /api/v1/cards/{id}: title/description edits the card,
// columnId/position moves it, and a combined body does both.
func (h *BoardHandler) UpdateCard(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, r, h.logger, domain.ErrTokenInvalid)
		return
	}

	var input cardPatchRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	cardID := chi.URLParam(r, "id")

	var events []board.Event
	if input.Title != nil || input.Description != nil {
		edited, err := h.service.EditCard(r.Context(), userID, cardID, input.Title, input.Description)
		if err != nil {
			writeError(w, r, h.logger, err)
			return
		}
		events = append(events, edited...)
	}
	if input.ColumnID != nil || input.Position != nil {
		moved, err := h.service.MoveCard(r.Context(), userID, cardID, input.ColumnID, input.Position)
		if err != nil {
			writeError(w, r, h.logger, err)
			return
		}
		events = append(events, moved...)
	}
	h.publish(r, events)
	w.WriteHeader(http.StatusOK)
}

// DeleteCard handles DELETE /api/v1/cards/{id}.
func (h *BoardHandler) DeleteCard(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, r, h.logger, domain.ErrTokenInvalid)
		return
	}

	events, err := h.service.DeleteCard(r.Context(), userID, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}
	h.publish(r, events)
	w.WriteHeader(http.StatusNoContent)
}
