-- Drop in reverse FK order so no constraint blocks the drop (cards → columns →
-- board_members → boards). DROP TABLE removes the table's indexes with it.
DROP TABLE cards;
DROP TABLE columns;
DROP TABLE board_members;
DROP TABLE boards;
