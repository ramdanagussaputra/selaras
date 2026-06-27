-- 0003 kanban: boards, membership, columns, cards (spec 03-kanban-crud Data Model).
-- Positions are fractional-index TEXT compared lexicographically (design D1); the
-- UNIQUE (parent, position) constraints are the concurrency backstop (D3) that
-- makes a colliding concurrent move detectable rather than silently duplicating.
-- ON DELETE CASCADE chains keep deletes single-statement: dropping a board takes
-- its members, columns, and cards; dropping a column takes its cards.

CREATE TABLE boards (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id   uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    title      text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE board_members (
    board_id   uuid NOT NULL REFERENCES boards (id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role       text NOT NULL CHECK (role IN ('owner', 'member')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (board_id, user_id)
);

-- The (parent, position) UNIQUE constraints are the concurrency backstop (D3).
-- They are DEFERRABLE INITIALLY IMMEDIATE: ordinary single-row moves still fail
-- fast with 23505 (driving the use case's re-read-and-retry), but the rare
-- renormalization path sets them DEFERRED inside its transaction so it can
-- rewrite every sibling's key without tripping a transient mid-rewrite collision
-- (the check runs once at commit instead of per row) (design D2).
CREATE TABLE columns (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    board_id   uuid NOT NULL REFERENCES boards (id) ON DELETE CASCADE,
    title      text NOT NULL,
    position   text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT columns_board_position_key UNIQUE (board_id, position) DEFERRABLE INITIALLY IMMEDIATE
);

CREATE TABLE cards (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    column_id   uuid NOT NULL REFERENCES columns (id) ON DELETE CASCADE,
    title       text NOT NULL,
    description text NOT NULL DEFAULT '',
    position    text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT cards_column_position_key UNIQUE (column_id, position) DEFERRABLE INITIALLY IMMEDIATE
);

-- Listing a user's boards joins through board_members; reading a board's tree
-- selects columns by board_id and cards by column_id, both ordered by position.
CREATE INDEX board_members_user_id_idx ON board_members (user_id);
CREATE INDEX columns_board_id_idx ON columns (board_id);
CREATE INDEX cards_column_id_idx ON cards (column_id);
