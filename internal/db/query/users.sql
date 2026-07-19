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
-- name: GetUserByGoogleID :one
SELECT * FROM users WHERE google_id = $1;

-- name: CreateGoogleUser :one
-- User baru yang daftar lewat Google. password_hash sengaja kosong,
-- email_verified langsung true karena Google sudah verifikasi email-nya.
INSERT INTO users (id, email, password_hash, google_id, email_verified)
VALUES ($1, $2, '', $3, true)
RETURNING *;

-- name: LinkGoogleID :exec
-- Dipakai saat user login Google dengan email yang SUDAH punya akun password.
-- Aman langsung link (tanpa nunggu verifikasi) karena email di sini
-- sudah dibuktikan kepemilikannya oleh Google sendiri.
UPDATE users SET google_id = $2 WHERE id = $1;

-- name: SetPendingPassword :exec
-- Simpan password "menunggu" ketika ada yang register pakai email yang
-- sudah terdaftar sebagai akun Google-only. Password baru aktif setelah
-- link verifikasi di-klik (lihat MergePendingPassword).
UPDATE users
SET pending_password_hash = $2, pending_password_token_hash = $3, pending_password_expires_at = $4
WHERE id = $1;

-- name: GetUserByPendingPasswordTokenHash :one
SELECT * FROM users WHERE pending_password_token_hash = $1;

-- name: MergePendingPassword :exec
-- Dipanggil setelah token merge terverifikasi: password pending dipindah
-- jadi password_hash aktif, field pending dibersihkan.
UPDATE users
SET password_hash = pending_password_hash,
    pending_password_hash = NULL,
    pending_password_token_hash = NULL,
    pending_password_expires_at = NULL
WHERE id = $1;
-- name: ClearExpiredPendingPasswords :execrows
-- Bersihkan pending_password_* pada akun Google-only yang link merge-nya
-- tidak pernah diklik sampai expired. User & google_id TIDAK dihapus —
-- cuma percobaan merge password yang basi ini yang dibuang, supaya
-- attempt merge berikutnya (attempt baru) bisa mulai bersih.
UPDATE users
SET pending_password_hash = NULL,
    pending_password_token_hash = NULL,
    pending_password_expires_at = NULL
WHERE pending_password_token_hash IS NOT NULL
  AND pending_password_expires_at < now();
