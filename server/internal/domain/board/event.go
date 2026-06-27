package board

// Event is a domain event describing a change a use case made. It is a *sealed*
// interface: the marker method is unexported, so only types declared in this
// package can satisfy it, and a future broadcaster can switch over them
// exhaustively. Use cases return []Event; in M3 the HTTP layer discards them,
// and M4 forwards them to a Broadcaster port for realtime fan-out (design D4).
// Events live in the pure domain so nothing about them depends on transport.
type Event interface {
	isBoardEvent()
}

// Created is emitted when a board (and its owner membership) is created.
type Created struct {
	BoardID string
	OwnerID string
}

// Updated is emitted when a board's title changes.
type Updated struct {
	BoardID string
}

// Deleted is emitted when a board is deleted (its children cascade in the DB).
type Deleted struct {
	BoardID string
}

// ColumnCreated is emitted when a column is added to a board.
type ColumnCreated struct {
	BoardID  string
	ColumnID string
	Position Position
}

// ColumnRenamed is emitted when a column's title changes.
type ColumnRenamed struct {
	BoardID  string
	ColumnID string
}

// ColumnMoved is emitted when a column's position within its board changes.
type ColumnMoved struct {
	BoardID  string
	ColumnID string
	Position Position
}

// ColumnDeleted is emitted when a column (and its cards) is deleted.
type ColumnDeleted struct {
	BoardID  string
	ColumnID string
}

// CardCreated is emitted when a card is added to a column.
type CardCreated struct {
	ColumnID string
	CardID   string
	Position Position
}

// CardUpdated is emitted when a card's title or description changes.
type CardUpdated struct {
	CardID string
}

// CardMoved is emitted when a card changes position, column, or both. It carries
// everything a broadcaster needs to relocate the card on a peer's board.
type CardMoved struct {
	CardID       string
	FromColumnID string
	ToColumnID   string
	Position     Position
}

// CardDeleted is emitted when a card is deleted.
type CardDeleted struct {
	CardID string
}

func (Created) isBoardEvent()       {}
func (Updated) isBoardEvent()       {}
func (Deleted) isBoardEvent()       {}
func (ColumnCreated) isBoardEvent() {}
func (ColumnRenamed) isBoardEvent() {}
func (ColumnMoved) isBoardEvent()   {}
func (ColumnDeleted) isBoardEvent() {}
func (CardCreated) isBoardEvent()   {}
func (CardUpdated) isBoardEvent()   {}
func (CardMoved) isBoardEvent()     {}
func (CardDeleted) isBoardEvent()   {}
