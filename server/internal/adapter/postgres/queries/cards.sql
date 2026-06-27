-- name: GetCard :one
SELECT id::text AS id, column_id::text AS column_id, title, description, position, created_at, updated_at
FROM cards
WHERE id = sqlc.arg(id)::uuid;

-- name: ListCardsByColumn :many
SELECT id::text AS id, column_id::text AS column_id, title, description, position, created_at, updated_at
FROM cards
WHERE column_id = sqlc.arg(column_id)::uuid
ORDER BY position;

-- name: ListCardsByBoard :many
SELECT cd.id::text AS id, cd.column_id::text AS column_id, cd.title, cd.description, cd.position,
       cd.created_at, cd.updated_at
FROM cards cd
JOIN columns col ON col.id = cd.column_id
WHERE col.board_id = sqlc.arg(board_id)::uuid
ORDER BY cd.column_id, cd.position;

-- name: InsertCard :exec
INSERT INTO cards (id, column_id, title, description, position, created_at, updated_at)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(column_id)::uuid, sqlc.arg(title), sqlc.arg(description),
        sqlc.arg(position), sqlc.arg(created_at), sqlc.arg(updated_at));

-- name: UpdateCardContent :exec
UPDATE cards SET title = sqlc.arg(title), description = sqlc.arg(description), updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id)::uuid;

-- name: UpdateCardPosition :exec
UPDATE cards SET column_id = sqlc.arg(column_id)::uuid, position = sqlc.arg(position), updated_at = now()
WHERE id = sqlc.arg(id)::uuid;

-- name: DeleteCard :exec
DELETE FROM cards WHERE id = sqlc.arg(id)::uuid;
