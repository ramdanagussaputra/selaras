package board

import (
	"errors"
	"strings"
)

// alphabet is the base-62 digit set in ascending order. It is chosen so that a
// position's ASCII byte order *is* its logical order: every character here sorts
// in ASCII the same way it sorts as a base-62 digit, so Postgres TEXT comparison
// and Go string comparison agree with no custom collation (design D1).
const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// base is the radix (62). The midpoint digit (base/2) seeds the first key.
const base = len(alphabet)

// maxPositionLength caps a generated key. Repeated inserts into the same gap
// lengthen keys; once Between would exceed this it returns ErrPositionExhausted
// and the app layer renormalizes the sibling set (design D2).
const maxPositionLength = 64

// ErrPositionExhausted reports that Between could not produce a key within the
// length cap. It is the domain's signal for the app layer to renormalize.
var ErrPositionExhausted = errors.New("position key exhausted; renormalization required")

// errInvalidPosition guards programmer errors (a malformed key or prev >= next);
// it never surfaces to users — the use cases pass ordered, validated neighbors.
var errInvalidPosition = errors.New("invalid position bounds")

// Position is a fractional index: a non-empty string over the base-62 alphabet,
// compared lexicographically. Inserting an item between two neighbors generates
// a key strictly between their positions, so a reorder is a single-row UPDATE
// rather than a renumber of siblings. The empty Position is the open bound used
// by Between: empty prev means "before everything" (prepend), empty next means
// "after everything" (append).
type Position string

// digitValue maps a byte to its base-62 index, or -1 if it is not a valid digit.
func digitValue(character byte) int {
	return strings.IndexByte(alphabet, character)
}

// valid reports whether every byte of the position is an alphabet digit and the
// key does not end in the lowest digit ('0') — the never-trailing-lowest
// invariant that guarantees there is always room to insert before it (D1).
func (p Position) valid() bool {
	if p == "" {
		return false
	}
	for index := 0; index < len(p); index++ {
		if digitValue(p[index]) < 0 {
			return false
		}
	}
	return p[len(p)-1] != alphabet[0]
}

// Between returns the shortest position strictly greater than prev and strictly
// less than next. An empty prev is the bottom bound (prepend) and an empty next
// is the top bound (append). It returns ErrPositionExhausted when the key would
// exceed maxPositionLength, and an error if the bounds are malformed or reversed.
func Between(prev, next Position) (Position, error) {
	if prev != "" && !prev.valid() {
		return "", errInvalidPosition
	}
	if next != "" && !next.valid() {
		return "", errInvalidPosition
	}

	switch {
	case prev == "" && next == "":
		// The very first key: a single midpoint digit, leaving room on both sides.
		return Position(alphabet[base/2 : base/2+1]), nil
	case prev == "":
		return prepend(next)
	case next == "":
		return append1(prev)
	default:
		if prev >= next {
			return "", errInvalidPosition
		}
		return midpoint(prev, next)
	}
}

// append1 returns the shortest key greater than prev with no upper bound. It
// increments prev's last digit by one when there is room — keeping the key the
// same length, so a long run of "add to the end" stays compact — and only grows
// the key (appending a midpoint digit) once the last digit is already the top.
func append1(prev Position) (Position, error) {
	last := digitValue(prev[len(prev)-1])
	if last+1 < base {
		return Position(string(prev[:len(prev)-1]) + string(alphabet[last+1])), nil
	}
	if len(prev)+1 > maxPositionLength {
		return "", ErrPositionExhausted
	}
	return Position(string(prev) + string(alphabet[base/2])), nil
}

// prepend returns the shortest key less than next with no lower bound. It mirrors
// append1: it halves toward the bottom when next's leading digit leaves room, and
// otherwise descends one place (emitting a leading '0') and recurses on the tail.
func prepend(next Position) (Position, error) {
	first := digitValue(next[0])
	if first > 1 {
		return Position(alphabet[first/2 : first/2+1]), nil
	}
	if len(next)+1 > maxPositionLength {
		return "", ErrPositionExhausted
	}
	if first == 1 {
		// Nothing sits strictly between the bottom and a leading '1' at this
		// place, so descend: a leading '0' keeps us below next, then a midpoint.
		return Position("0" + string(alphabet[base/2])), nil
	}
	// next begins with '0' (itself the product of an earlier prepend); keep the
	// shared '0' and find room in the tail.
	tail, err := prepend(next[1:])
	if err != nil {
		return "", err
	}
	return Position("0" + string(tail)), nil
}

// midpoint returns the shortest key strictly between two bounded neighbors. It
// walks both keys digit by digit: where a digit strictly between the two exists
// it emits the midpoint and stops; where the digits are adjacent it copies the
// lower digit, drops the now-satisfied upper bound, and descends one place —
// which is what lengthens keys and eventually triggers renormalization (D2).
func midpoint(prev, next Position) (Position, error) {
	lower, upper := string(prev), string(next)

	var result []byte
	for index := 0; ; index++ {
		if len(result) >= maxPositionLength {
			return "", ErrPositionExhausted
		}

		lowDigit := 0
		if index < len(lower) {
			lowDigit = digitValue(lower[index])
		}
		highDigit := base
		if index < len(upper) {
			highDigit = digitValue(upper[index])
		}

		if lowDigit == highDigit {
			result = append(result, alphabet[lowDigit])
			continue
		}

		midDigit := (lowDigit + highDigit) / 2
		if midDigit == lowDigit {
			// Adjacent digits: no integer strictly between. Copy the lower digit
			// and descend; from here next no longer constrains us (we are already
			// below it), so future places run against an unbounded tail.
			result = append(result, alphabet[lowDigit])
			upper = ""
			continue
		}

		// midDigit exceeds lowDigit (>= 1), so the key never ends in the lowest digit.
		result = append(result, alphabet[midDigit])
		return Position(result), nil
	}
}

// Renormalize returns count evenly-spaced positions in ascending order, used by
// the app layer to rewrite a sibling set whose keys have grown too long (D2). The
// keys are spread across the base-62 range with gaps of at least two units, so
// there is room to insert between any pair again before the next renormalization.
func Renormalize(count int) ([]Position, error) {
	if count == 0 {
		return nil, nil
	}

	// Widen the key until the range holds at least two units per item, so
	// consecutive keys differ by >= 2 and a fresh insert between them never
	// immediately re-exhausts.
	width, capacity := 1, base
	for capacity < (count+1)*2 {
		capacity *= base
		width++
	}
	step := capacity / (count + 1)

	positions := make([]Position, count)
	for item := 1; item <= count; item++ {
		value := item * step
		if value%base == 0 {
			// A trailing '0' would break the invariant; nudge up by one — the
			// >= 2 gap keeps the key strictly below the next item's.
			value++
		}
		positions[item-1] = encodeFixedWidth(value, width)
	}
	return positions, nil
}

// encodeFixedWidth renders value as width base-62 digits, big-endian, so the
// fixed width makes lexicographic order match numeric order.
func encodeFixedWidth(value, width int) Position {
	buffer := make([]byte, width)
	for index := width - 1; index >= 0; index-- {
		buffer[index] = alphabet[value%base]
		value /= base
	}
	return Position(buffer)
}
