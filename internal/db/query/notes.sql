-- name: CreateNote :one
INSERT INTO notes (id, user_id, mode, content, salt, title)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetNote :one
SELECT * FROM notes WHERE id = $1;

-- name: UpdateNoteContent :one
UPDATE notes
SET content = $2, title = $3, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteNote :execrows
DELETE FROM notes WHERE id = $1;

-- name: ListNotesByUser :many
SELECT id, mode, title, created_at, updated_at FROM notes WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: CountNotesByUser :one
SELECT COUNT(*) FROM notes WHERE user_id = $1;
