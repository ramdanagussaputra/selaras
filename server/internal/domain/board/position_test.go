package board

import (
	"errors"
	"strings"
	"testing"
)

// TestBetween covers the core generation paths: a fresh first key, append,
// prepend, a midpoint between far-apart neighbors, and the descend path between
// adjacent neighbors that lengthens the key. Each case asserts the strict
// ordering contract and the never-trailing-lowest invariant.
func TestBetween(t *testing.T) {
	tests := []struct {
		name string
		prev Position
		next Position
	}{
		{name: "first key (both open)", prev: "", next: ""},
		{name: "append after a key", prev: "V", next: ""},
		{name: "prepend before a key", prev: "", next: "V"},
		{name: "midpoint between far neighbors", prev: "1", next: "z"},
		{name: "between adjacent single digits", prev: "V", next: "W"},
		{name: "between adjacent multi-digit", prev: "V1", next: "V2"},
		{name: "append after the top digit", prev: "z", next: ""},
		{name: "prepend before the bottom-most key", prev: "", next: "1"},
		{name: "between a prefix and its extension", prev: "V", next: "VV"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := Between(testCase.prev, testCase.next)
			if err != nil {
				t.Fatalf("Between(%q, %q) error: %v", testCase.prev, testCase.next, err)
			}

			if testCase.prev != "" && testCase.prev >= got {
				t.Errorf("got %q is not strictly after prev %q", got, testCase.prev)
			}
			if testCase.next != "" && got >= testCase.next {
				t.Errorf("got %q is not strictly before next %q", got, testCase.next)
			}
			if !got.valid() {
				t.Errorf("got %q violates the position invariant (empty or trailing lowest digit)", got)
			}
		})
	}
}

// TestBetweenRejectsOutOfOrderBounds ensures the domain refuses bounds that are
// equal or reversed — a programmer error the use cases must never pass.
func TestBetweenRejectsOutOfOrderBounds(t *testing.T) {
	tests := []struct {
		name string
		prev Position
		next Position
	}{
		{name: "equal bounds", prev: "V", next: "V"},
		{name: "reversed bounds", prev: "W", next: "V"},
		{name: "invalid prev digit", prev: "*", next: "z"},
		{name: "prev ends in lowest digit", prev: "V0", next: ""},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := Between(testCase.prev, testCase.next); err == nil {
				t.Errorf("Between(%q, %q) = nil error, want rejection", testCase.prev, testCase.next)
			}
		})
	}
}

// TestBetweenRepeatedAppendStaysOrdered generates a long ascending run by
// repeated append and asserts the keys stay strictly increasing and valid — the
// common "always add to the end" workload.
func TestBetweenRepeatedAppendStaysOrdered(t *testing.T) {
	previous := Position("")
	for iteration := 0; iteration < 500; iteration++ {
		next, err := Between(previous, "")
		if err != nil {
			t.Fatalf("append #%d: %v", iteration, err)
		}
		if previous != "" && previous >= next {
			t.Fatalf("append #%d: %q not strictly after %q", iteration, next, previous)
		}
		if !next.valid() {
			t.Fatalf("append #%d: %q invalid", iteration, next)
		}
		previous = next
	}
}

// TestBetweenExhaustionTriggersRenormalization repeatedly inserts into the same
// gap (always between the same low bound and the previous result) until a key
// would exceed the length cap, proving the renormalization trigger fires (D2)
// and that every intermediate key remains correctly ordered until it does.
func TestBetweenExhaustionTriggersRenormalization(t *testing.T) {
	const lowBound = Position("V")
	high := Position("W")

	var lastErr error
	for iteration := 0; iteration < 10_000; iteration++ {
		inserted, err := Between(lowBound, high)
		if err != nil {
			lastErr = err
			break
		}
		if lowBound >= inserted || inserted >= high {
			t.Fatalf("insert #%d: %q not strictly between %q and %q", iteration, inserted, lowBound, high)
		}
		if len(inserted) > maxPositionLength {
			t.Fatalf("insert #%d produced key longer than the cap: len %d", iteration, len(inserted))
		}
		high = inserted // keep squeezing the same gap
	}

	if !errors.Is(lastErr, ErrPositionExhausted) {
		t.Fatalf("repeated same-gap inserts did not exhaust: lastErr = %v", lastErr)
	}
}

// TestRenormalizeProducesOrderedShortKeys checks the renormalization helper hands
// back the requested count of ascending, valid, short keys.
func TestRenormalizeProducesOrderedShortKeys(t *testing.T) {
	const count = 50
	positions, err := Renormalize(count)
	if err != nil {
		t.Fatalf("Renormalize(%d): %v", count, err)
	}
	if len(positions) != count {
		t.Fatalf("got %d positions, want %d", len(positions), count)
	}

	for index := range positions {
		if !positions[index].valid() {
			t.Errorf("position[%d] = %q is invalid", index, positions[index])
		}
		if index > 0 && positions[index-1] >= positions[index] {
			t.Errorf("position[%d] %q not strictly after %q", index, positions[index], positions[index-1])
		}
		if len(positions[index]) > 4 {
			t.Errorf("renormalized key %q is longer than expected (%d chars)", positions[index], len(positions[index]))
		}
	}
}

// TestDigitValue verifies the alphabet maps to ascending indices and rejects
// non-alphabet bytes — the backbone of lexicographic order matching logical order.
func TestDigitValue(t *testing.T) {
	for index := 0; index < len(alphabet); index++ {
		if got := digitValue(alphabet[index]); got != index {
			t.Errorf("digitValue(%q) = %d, want %d", alphabet[index], got, index)
		}
	}
	for _, bad := range []byte{'*', '-', ' ', '!'} {
		if got := digitValue(bad); got != -1 {
			t.Errorf("digitValue(%q) = %d, want -1", bad, got)
		}
	}
	if strings.IndexByte(alphabet, alphabet[0]) != 0 || alphabet[0] != '0' {
		t.Error("alphabet must begin with the lowest digit '0'")
	}
}
