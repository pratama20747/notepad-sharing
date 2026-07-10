-- name: CreateSession :one
INSERT INTO sessions (id, user_id, token_hash, user_agent, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetSessionByTokenHash :one
SELECT * FROM sessions WHERE token_hash = $1;

-- name: RevokeSession :exec
UPDATE sessions SET revoked_at = now() WHERE id = $1;

-- name: RevokeSessionByTokenHash :exec
UPDATE sessions SET revoked_at = now() WHERE token_hash = $1;
