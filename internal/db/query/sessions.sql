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

-- name: DeleteExpiredAndRevokedSessions :execrows
-- Hapus session yang sudah expired ATAU sudah di-revoke lebih dari 1 hari.
-- Revoked session diberi grace period 1 hari sebelum dihapus untuk keperluan
-- audit log jika diperlukan di masa depan.
DELETE FROM sessions
WHERE
    expires_at < now()
    OR (revoked_at IS NOT NULL AND revoked_at < now() - INTERVAL '1 day');
