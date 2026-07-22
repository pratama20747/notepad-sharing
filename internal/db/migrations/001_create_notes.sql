-- ============================================================
-- TABEL users
-- ============================================================
CREATE TABLE IF NOT EXISTS users (
    id                            VARCHAR(21) PRIMARY KEY,
    email                         VARCHAR(255) NOT NULL UNIQUE,
    password_hash                 TEXT NOT NULL DEFAULT '',
    avatar_url                    TEXT,
    avatar_source                 VARCHAR(20) NOT NULL DEFAULT 'none', -- 'none' | 'google' | 'upload'
    -- Email verification
    email_verified                BOOLEAN NOT NULL DEFAULT false,
    verification_token_hash       VARCHAR(64),
    verification_expires_at       TIMESTAMPTZ,
    -- Google OAuth
    google_id                     VARCHAR(255) UNIQUE,
    -- Pending password merge (untuk akun Google yang mau setup password)
    pending_password_hash         TEXT,
    pending_password_token_hash   VARCHAR(64),
    pending_password_expires_at   TIMESTAMPTZ,
    created_at                    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_users_verification_token_hash ON users (verification_token_hash);
CREATE INDEX IF NOT EXISTS idx_users_google_id ON users (google_id);
CREATE INDEX IF NOT EXISTS idx_users_pending_password_token_hash ON users (pending_password_token_hash);

COMMENT ON COLUMN users.google_id IS 'Sub ID dari Google OAuth, NULL kalau user belum pernah login via Google';
COMMENT ON COLUMN users.pending_password_hash IS 'Bcrypt hash password yang MENUNGGU verifikasi email sebelum di-merge ke akun Google yang sudah ada';
COMMENT ON COLUMN users.pending_password_token_hash IS 'SHA-256 dari token merge yang dikirim ke email';

-- ============================================================
-- TABEL sessions
-- ============================================================
CREATE TABLE IF NOT EXISTS sessions (
    id           VARCHAR(21) PRIMARY KEY,
    user_id      VARCHAR(21) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash   VARCHAR(64) NOT NULL UNIQUE,
    user_agent   TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ NOT NULL,
    revoked_at   TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_sessions_token_hash ON sessions (token_hash);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions (user_id);

-- ============================================================
-- TABEL notes
-- ============================================================
CREATE TABLE IF NOT EXISTS notes (
    id           VARCHAR(21) PRIMARY KEY,
    user_id      VARCHAR(21) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    mode         VARCHAR(10) NOT NULL CHECK (mode IN ('public', 'private')),
    content      BYTEA NOT NULL,
    salt         BYTEA NOT NULL DEFAULT '',
    title        TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_notes_created_at ON notes (created_at);
CREATE INDEX IF NOT EXISTS idx_notes_user_id ON notes (user_id);

COMMENT ON TABLE notes IS 'Table for storing notes with public/private mode';
COMMENT ON COLUMN notes.mode IS 'public: plaintext, private: nonce||ciphertext AES-GCM';
COMMENT ON COLUMN notes.content IS 'plaintext for public, nonce||ciphertext for private';
COMMENT ON COLUMN notes.salt IS '16 byte random salt for private mode, empty for public';
COMMENT ON COLUMN notes.user_id IS 'Pemilik note. List/Update/Delete hanya boleh oleh pemilik; Get/Unlock via share link tetap terbuka untuk siapapun.';

CREATE TABLE IF NOT EXISTS note_attachments (
    id           VARCHAR(21) PRIMARY KEY,
    note_id      VARCHAR(21) NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
    r2_key       TEXT NOT NULL,
    url          TEXT NOT NULL,
    content_type VARCHAR(50) NOT NULL,
    file_size    BIGINT NOT NULL,
    kind         VARCHAR(10) NOT NULL CHECK (kind IN ('image', 'video')),
    encrypted    BOOLEAN NOT NULL DEFAULT false,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- ============================================================
-- TABEL note_attachments
-- ============================================================
CREATE INDEX IF NOT EXISTS idx_note_attachments_note_id ON note_attachments (note_id);

COMMENT ON COLUMN note_attachments.encrypted IS 'true jika note pemiliknya mode private — content_type & file_size di sini merujuk ke file ASLI (sebelum dienkripsi), bukan blob terenkripsi di R2';
COMMENT ON COLUMN note_attachments.r2_key IS 'key object di R2. Untuk attachment private, isinya adalah nonce||ciphertext AES-GCM (application/octet-stream)';
