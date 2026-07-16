CREATE TABLE IF NOT EXISTS users (
    id            VARCHAR(21) PRIMARY KEY,
    email         VARCHAR(255) NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    -- Email verification
    email_verified           BOOLEAN NOT NULL DEFAULT false,
    verification_token_hash  VARCHAR(64),
    verification_expires_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_users_verification_token_hash ON users (verification_token_hash);

CREATE TABLE IF NOT EXISTS sessions (
    id         VARCHAR(21) PRIMARY KEY,
    user_id    VARCHAR(21) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- token_hash: SHA-256 hex dari refresh token. Refresh token asli TIDAK
    -- pernah disimpan, hanya hash-nya (mirip cara password disimpan).
    token_hash VARCHAR(64) NOT NULL UNIQUE,
    user_agent TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_sessions_token_hash ON sessions (token_hash);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions (user_id);

CREATE TABLE IF NOT EXISTS notes (
    id         VARCHAR(21) PRIMARY KEY,
    user_id    VARCHAR(21) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    mode       VARCHAR(10) NOT NULL CHECK (mode IN ('public', 'private')),
    -- content: plaintext (mode=public) atau nonce||ciphertext hasil AES-GCM (mode=private)
    content    BYTEA NOT NULL,
    -- salt: kosong untuk mode public, random 16 byte untuk mode private
    salt       BYTEA NOT NULL DEFAULT '',
    title      TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_notes_created_at ON notes (created_at);
CREATE INDEX IF NOT EXISTS idx_notes_user_id ON notes (user_id);

COMMENT ON TABLE notes IS 'Table for storing notes with public/private mode';
COMMENT ON COLUMN notes.mode IS 'public: plaintext, private: nonce||ciphertext AES-GCM';
COMMENT ON COLUMN notes.content IS 'plaintext for public, nonce||ciphertext for private';
COMMENT ON COLUMN notes.salt IS '16 byte random salt for private mode, empty for public';
COMMENT ON COLUMN notes.user_id IS 'Pemilik note. List/Update/Delete hanya boleh oleh pemilik; Get/Unlock via share link tetap terbuka untuk siapapun.';
