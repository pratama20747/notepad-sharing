-- name: CreateUser :one
INSERT INTO users (id, email, password_hash)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: SetVerificationToken :exec
UPDATE users
SET verification_token_hash = $2, verification_expires_at = $3
WHERE id = $1;

-- name: GetUserByVerificationTokenHash :one
SELECT * FROM users WHERE verification_token_hash = $1;

-- name: MarkEmailVerified :exec
UPDATE users
SET email_verified = true, verification_token_hash = NULL, verification_expires_at = NULL
WHERE id = $1;

-- name: DeleteUnverifiedUsers :execrows
-- Hapus user yang belum verifikasi email dan sudah lewat 2 hari sejak register.
-- ON DELETE CASCADE di tabel sessions & notes otomatis ikut membersihkan data terkait.
DELETE FROM users
WHERE email_verified = false
  AND created_at < now() - INTERVAL '2 days';
