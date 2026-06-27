-- name: GetColumn :one
SELECT id::text AS id, board_id::text AS board_id, title, position, created_at
FROM columns
WHERE id = sqlc.arg(id)::uuid;

-- name: ListColumns :many
SELECT id::text AS id, board_id::text AS board_id, title, position, created_at
FROM columns
WHERE board_id = sqlc.arg(board_id)::uuid
ORDER BY position;

-- name: InsertColumn :exec
INSERT INTO columns (id, board_id, title, position, created_at)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(board_id)::uuid, sqlc.arg(title), sqlc.arg(position), sqlc.arg(created_at));

-- name: UpdateColumnTitle :exec
UPDATE columns SET title = sqlc.arg(title) WHERE id = sqlc.arg(id)::uuid;

-- name: UpdateColumnPosition :exec
UPDATE columns SET position = sqlc.arg(position) WHERE id = sqlc.arg(id)::uuid;

-- name: DeleteColumn :exec
DELETE FROM columns WHERE id = sqlc.arg(id)::uuid;
