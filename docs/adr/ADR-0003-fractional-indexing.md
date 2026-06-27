# ADR-0003: fractional-indexing

**Status**: Proposed
**Version**: 1.0
**Date**: 2026-06-25
**Author**: Ramdan Agus Saputra

## Context

M3 makes boards orderable: cards within a column and columns within a board are
drag-and-drop reorderable, and the order must persist (see [PRD](../selaras-prd.md)
Story 2 / Feature 2 and [spec 03-kanban-crud](../specs/03-kanban-crud.md), design
decisions D1–D3). The interview-facing requirement is that a reorder be a
**single-row `UPDATE`** — dragging one card must not renumber its siblings — while
order stays correct under the concurrent edits a future realtime board invites.
The forces in tension:

- **O(1) writes vs a totally-ordered sequence** — integer ranks (1, 2, 3, …) give
  a clean order but force a multi-row renumber the moment a gap fills.
- **Lexicographic storage** — Postgres should sort positions with a plain `ORDER
  BY position` and `TEXT` comparison, with no custom collation or numeric parsing.
- **Bounded key growth** — any "insert between two neighbors" scheme lengthens
  keys over repeated same-gap inserts; that growth must be bounded, not unbounded.
- **Concurrency without a coordinator** — there is no lock server; two clients may
  compute the same key for the same gap, and the database must stay consistent
  without surfacing a raw constraint error to either user.

This ADR records the ordering scheme: the key alphabet, the `Between` generator,
the renormalization threshold, and the unique-constraint concurrency backstop.

## Decision

**We will order columns and cards by a base-62 fractional-index `Position` stored
as `TEXT`, generate keys strictly between neighbors so a reorder is one row, and
back the scheme with app-layer renormalization and a deferrable unique constraint.**

1. **Base-62 ASCII alphabet, lexicographic order.** A position is a non-empty
   string over `0-9 A-Z a-z` (62 digits). The alphabet is ordered so that each
   character's ASCII byte order *is* its base-62 digit order, so Go string
   comparison and Postgres `TEXT` comparison agree with no collation config.
   `domain/board.Position` wraps such a string; the domain imports stdlib only.

2. **`Between(prev, next)` returns the shortest key strictly between its
   neighbors.** An empty `prev` means "before everything" (prepend) and an empty
   `next` means "after everything" (append). Append increments the last digit when
   there is room (so a long "add to the end" run stays compact) and only grows the
   key when the last digit is already the top; the two-bounded case walks both
   keys digit by digit, emitting a midpoint where one exists and descending a place
   where the digits are adjacent. A generated key never ends in the lowest digit
   (`0`), guaranteeing there is always room to insert before it.

3. **Renormalization at the app layer, threshold key length > 64.** When `Between`
   would exceed 64 characters it returns `ErrPositionExhausted`; the **use case**
   (not the domain) reads the affected sibling set in order, reassigns evenly-spaced
   fresh keys to all of them, and retries — all inside one transaction, so readers
   never see a half-renormalized column and the visible order is unchanged.

4. **Concurrency backstop — deferrable `UNIQUE (parent, position)` + retry.** Each
   parent/position pair is unique. Two clients moving the same card into the same
   gap compute the same key and collide on `23505`; the move use case runs in a
   transaction and, on that violation, re-reads neighbors, recomputes `Between`,
   and retries up to three times — last-write-wins, with no raw constraint error
   surfaced. The constraint is `DEFERRABLE INITIALLY IMMEDIATE`: ordinary moves
   still fail fast (driving the retry), but renormalization sets it `DEFERRED` so
   it can rewrite every sibling without a transient mid-rewrite collision.

## Alternatives considered

- **Integer ranks (100, 200, 300 …)** — simple and human-readable, but inserting
  between two adjacent ranks with no free integer forces renumbering the whole tail
  — a multi-row write, violating the single-row-`UPDATE` goal. Rejected.
- **Floating-point ranks (midpoint = (a+b)/2)** — the spec's "predict what goes
  wrong" question. A float has ~52 bits of mantissa, so repeatedly inserting into
  the *same* gap halves the interval each time and exhausts precision after ~50
  inserts; the midpoint then rounds to equal one neighbor and the order silently
  collapses (two items compare equal, ties resolve arbitrarily). Strings have no
  such ceiling — they just grow, and growth is bounded by renormalization.
  Rejected as silently incorrect.
- **rocicorp/Figma jitter variant (base-95 + length headers + random suffix)** —
  more robust against *interleaving* when many clients insert into the same gap
  concurrently, because the random suffix makes their keys diverge. But it is
  materially more complex (variable-length integer headers, a randomness source in
  the generator). At single-instance demo scale we keep the simpler deterministic
  generator and lean on the retry + unique constraint; this variant is the
  documented upgrade path if real contention appears.
- **CRDT / operational-transform sequence (e.g. LSEQ, RGA)** — convergent under
  arbitrary concurrency without a backstop, but a large dependency and conceptual
  surface for a single-instance app. Last-write-wins is honest and sufficient here.
- **`ltree` / array positions / a dedicated ordering extension** — either ties us
  to a Postgres-specific type with its own operators or reintroduces multi-row
  rewrites. A plain `TEXT` column with lexicographic compare needs nothing special.

## Consequences

- A reorder is a single-row `UPDATE` of the moved item's `position`; siblings are
  untouched except in the rare renormalization path. Reads stay a plain
  `ORDER BY position`.
- The domain `Position` type is pure and exhaustively unit-tested (between /
  append / prepend / adjacent-key descent / exhaustion boundary), so a generator
  bug shows up as a failing table test or, in the worst case, a caught `23505`
  rather than silent disorder.
- Keys can grow under pathological same-gap insertion, and renormalization is a
  multi-row write — but it is bounded (triggered only past 64 chars) and atomic.
- The retry loop is bounded at three attempts; under truly pathological contention
  it returns `409 CONFLICT` honestly rather than looping unbounded.
- **Revisit trigger.** If realtime collaboration (M4+) produces real concurrent
  contention on the same gap — visible as frequent retries or renormalizations in
  metrics — adopt the jitter variant (random suffix) to reduce interleaving, or
  move ordering into a CRDT. Until that signal appears, the simpler scheme stands.
