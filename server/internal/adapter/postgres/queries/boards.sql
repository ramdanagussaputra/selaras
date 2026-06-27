-- name: InsertBoard :exec
INSERT INTO boards (id, owner_id, title, created_at, updated_at)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(owner_id)::uuid, sqlc.arg(title), sqlc.arg(created_at), sqlc.arg(updated_at));

-- name: InsertMember :exec
INSERT INTO board_members (board_id, user_id, role)
VALUES (sqlc.arg(board_id)::uuid, sqlc.arg(user_id)::uuid, sqlc.arg(role));

-- name: ListBoardsForUser :many
SELECT b.id::text AS id, b.owner_id::text AS owner_id, b.title, b.created_at, b.updated_at
FROM boards b
JOIN board_members m ON m.board_id = b.id
WHERE m.user_id = sqlc.arg(user_id)::uuid
ORDER BY b.created_at;

-- name: GetBoard :one
SELECT id::text AS id, owner_id::text AS owner_id, title, created_at, updated_at
FROM boards
WHERE id = sqlc.arg(id)::uuid;

-- name: UpdateBoardTitle :exec
UPDATE boards SET title = sqlc.arg(title), updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id)::uuid;

-- name: DeleteBoard :exec
DELETE FROM boards WHERE id = sqlc.arg(id)::uuid;

-- name: MembershipByBoard :one
SELECT board_id::text AS board_id, role
FROM board_members
WHERE board_id = sqlc.arg(board_id)::uuid AND user_id = sqlc.arg(user_id)::uuid;

-- name: MembershipByColumn :one
SELECT c.board_id::text AS board_id, m.role
FROM columns c
JOIN board_members m ON m.board_id = c.board_id AND m.user_id = sqlc.arg(user_id)::uuid
WHERE c.id = sqlc.arg(column_id)::uuid;

-- name: MembershipByCard :one
SELECT col.board_id::text AS board_id, m.role
FROM cards cd
JOIN columns col ON col.id = cd.column_id
JOIN board_members m ON m.board_id = col.board_id AND m.user_id = sqlc.arg(user_id)::uuid
WHERE cd.id = sqlc.arg(card_id)::uuid;
