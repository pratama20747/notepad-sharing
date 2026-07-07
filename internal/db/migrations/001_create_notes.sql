-- init.sql
-- Create notes table
CREATE TABLE IF NOT EXISTS notes (
    id         VARCHAR(21) PRIMARY KEY,
    mode       VARCHAR(10) NOT NULL CHECK (mode IN ('public', 'private')),
    -- content: plaintext (mode=public) atau nonce||ciphertext hasil AES-GCM (mode=private)
    content    BYTEA NOT NULL,
    -- salt: kosong untuk mode public, random 16 byte untuk mode private (dipakai derive key dari password)
    salt       BYTEA NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Create index on created_at
CREATE INDEX IF NOT EXISTS idx_notes_created_at ON notes (created_at);

-- Add title column as TEXT (plaintext)
-- agar judul private note tetap terlihat di daftar & saat share link
ALTER TABLE notes ADD COLUMN IF NOT EXISTS title TEXT NOT NULL DEFAULT '';

-- Hapus komentar yang tidak perlu untuk production
COMMENT ON TABLE notes IS 'Table for storing notes with public/private mode';
COMMENT ON COLUMN notes.mode IS 'public: plaintext, private: nonce||ciphertext AES-GCM';
COMMENT ON COLUMN notes.content IS 'plaintext for public, nonce||ciphertext for private';
COMMENT ON COLUMN notes.salt IS '16 byte random salt for private mode, empty for public';
COMMENT ON COLUMN notes.title IS 'Plaintext title for display in list and share links';
