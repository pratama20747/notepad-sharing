// Package service berisi business logic notepad, terpisah dari HTTP layer
// (handler) dan data access layer (sqlc) agar mudah di-testing dan di-reuse.
package service

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"notepad-sharelink/internal/cryptoutil"
	"notepad-sharelink/internal/db/sqlc"
	"notepad-sharelink/internal/idgen"
)

// Mode notepad yang didukung.
const (
	ModePublic  = "public"
	ModePrivate = "private"
)

// Sentinel errors — di-map ke HTTP status code yang sesuai di handler layer.
var (
	ErrNotFound       = errors.New("note tidak ditemukan")
	ErrWrongPassword  = errors.New("password salah")
	ErrInvalidMode    = errors.New("mode tidak valid")
	ErrPasswordNeeded = errors.New("password wajib diisi untuk note mode private")
	ErrTitleTooLong   = errors.New("judul terlalu panjang, maksimal 200 karakter")
	ErrForbidden      = errors.New("anda tidak punya akses ke note ini")
)

// NoteSummary digunakan untuk response list notes ringan — tanpa content/salt.
type NoteSummary struct {
	ID        string    `json:"id"`
	Mode      string    `json:"mode"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NoteService merangkum seluruh use-case terkait notepad.
type NoteService struct {
	q *sqlc.Queries
}

// NewNoteService membuat NoteService baru.
func NewNoteService(q *sqlc.Queries) *NoteService {
	return &NoteService{q: q}
}

// CreateNote membuat note baru dan mengembalikan ID (slug) untuk share link.
//
// Title selalu disimpan sebagai plaintext (TEXT) agar judul private note tetap
// terlihat di daftar catatan dan saat share link.
// Content: plaintext untuk public, dienkripsi (AES-256-GCM) untuk private.
// Note selalu di-link ke userID yang membuat (isolasi per user).
func (s *NoteService) CreateNote(ctx context.Context, mode, title, content, password, userID string) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if mode != ModePublic && mode != ModePrivate {
		return "", ErrInvalidMode
	}

	if len(title) > 200 {
		return "", ErrTitleTooLong
	}

	id, err := idgen.New()
	if err != nil {
		return "", err
	}

	var (
		storedContent []byte
		salt          []byte
	)

	switch mode {
	case ModePublic:
		storedContent = []byte(content)
		salt = []byte{}

	case ModePrivate:
		if password == "" {
			return "", ErrPasswordNeeded
		}

		salt, err = cryptoutil.GenerateSalt()
		if err != nil {
			return "", err
		}

		key := cryptoutil.DeriveKey(password, salt)
		storedContent, err = cryptoutil.Encrypt([]byte(content), key)
		if err != nil {
			return "", err
		}
	}

	_, err = s.q.CreateNote(ctx, sqlc.CreateNoteParams{
		ID:      id,
		UserID:  userID,
		Mode:    mode,
		Content: storedContent,
		Salt:    salt,
		Title:   title,
	})
	if err != nil {
		return "", err
	}

	return id, nil
}

// GetNoteMeta mengembalikan mode note, title, dan content HANYA jika mode-nya public.
// Untuk mode private, content tidak dikembalikan — klien harus memanggil
// UnlockPrivateNote dengan password yang benar.
// Title selalu dikembalikan (plaintext) untuk semua mode.
//
// Endpoint ini PUBLIK — tidak butuh login. Siapapun bisa akses lewat share link.
// Note: GetNoteMeta tidak memvalidasi user — untuk share link, access diberikan ke semua orang.
func (s *NoteService) GetNoteMeta(ctx context.Context, id string) (mode string, title string, content string, err error) {
	if ctx.Err() != nil {
		return "", "", "", ctx.Err()
	}
	n, err := s.q.GetNote(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", "", ErrNotFound
		}
		return "", "", "", err
	}

	if n.Mode == ModePublic {
		return n.Mode, n.Title, string(n.Content), nil
	}
	return n.Mode, n.Title, "", nil
}

// UnlockPrivateNote memverifikasi password dan mengembalikan title & content asli
// (hasil dekripsi content) jika password benar.
// Title langsung dikembalikan sebagai plaintext (tidak perlu didekripsi).
//
// Endpoint ini PUBLIK — tidak butuh login. Siapapun bisa unlock pakai password.
// Note: UnlockPrivateNote tidak memvalidasi user — untuk share link, akses diberikan ke semua orang.
func (s *NoteService) UnlockPrivateNote(ctx context.Context, id, password string) (title string, content string, err error) {
	if ctx.Err() != nil {
		return "", "", ctx.Err()
	}
	n, err := s.q.GetNote(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", ErrNotFound
		}
		return "", "", err
	}

	if n.Mode != ModePrivate {
		return "", "", ErrInvalidMode
	}

	key := cryptoutil.DeriveKey(password, n.Salt)
	plaintextContent, err := cryptoutil.Decrypt(n.Content, key)
	if err != nil {
		return "", "", ErrWrongPassword
	}

	// Title plaintext, tidak perlu didekripsi
	return n.Title, string(plaintextContent), nil
}

// UpdateNote mengubah isi note (content & title). Untuk mode private, password
// wajib dikirim dan akan diverifikasi terlebih dahulu (dengan mencoba dekripsi
// content lama) sebelum content baru dienkripsi ulang dan disimpan.
//
// PENTING: UpdateNote hanya boleh dijalankan oleh pemilik note (userID harus cocok).
// Jika userID tidak cocok, mengembalikan ErrForbidden.
func (s *NoteService) UpdateNote(ctx context.Context, id, title, content, password, userID string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if len(title) > 200 {
		return ErrTitleTooLong
	}

	n, err := s.q.GetNote(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}

	// Verifikasi bahwa userID yang melakukan update adalah pemilik note
	if n.UserID != userID {
		return ErrForbidden
	}

	var newContent []byte

	switch n.Mode {
	case ModePublic:
		newContent = []byte(content)

	case ModePrivate:
		if password == "" {
			return ErrPasswordNeeded
		}

		key := cryptoutil.DeriveKey(password, n.Salt)

		// Verifikasi password: coba dekripsi content lama dulu.
		if _, err := cryptoutil.Decrypt(n.Content, key); err != nil {
			return ErrWrongPassword
		}

		newContent, err = cryptoutil.Encrypt([]byte(content), key)
		if err != nil {
			return err
		}
	}

	_, err = s.q.UpdateNoteContent(ctx, sqlc.UpdateNoteContentParams{
		ID:      id,
		Content: newContent,
		Title:   title,
	})
	return err
}

// DeleteNote menghapus note. Untuk mode private, password wajib diverifikasi
// terlebih dahulu sebelum penghapusan dieksekusi.
//
// PENTING: DeleteNote hanya boleh dijalankan oleh pemilik note (userID harus cocok).
// Jika userID tidak cocok, mengembalikan ErrForbidden.
func (s *NoteService) DeleteNote(ctx context.Context, id, password, userID string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	n, err := s.q.GetNote(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}

	// Verifikasi bahwa userID yang melakukan delete adalah pemilik note
	if n.UserID != userID {
		return ErrForbidden
	}

	if n.Mode == ModePrivate {
		if password == "" {
			return ErrPasswordNeeded
		}

		key := cryptoutil.DeriveKey(password, n.Salt)
		if _, err := cryptoutil.Decrypt(n.Content, key); err != nil {
			return ErrWrongPassword
		}
	}

	rows, err := s.q.DeleteNote(ctx, id)
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}

	return nil
}

// ListNotes mengembalikan daftar note ringan (tanpa content/salt) yang punya
// userID tertentu. Title langsung dikembalikan apa adanya karena disimpan
// sebagai plaintext untuk semua mode.
//
// PENTING: ListNotes hanya menampilkan note punya userID yang request.
// Note user lain TIDAK terlihat (isolasi per user).
func (s *NoteService) ListNotes(ctx context.Context, userID string, limit, offset int32) ([]NoteSummary, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	rows, err := s.q.ListNotesByUser(ctx, sqlc.ListNotesByUserParams{
		UserID: userID,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, err
	}

	summaries := make([]NoteSummary, len(rows))
	for i, row := range rows {
		summaries[i] = NoteSummary{
			ID:        row.ID,
			Mode:      row.Mode,
			Title:     row.Title,
			CreatedAt: row.CreatedAt,
			UpdatedAt: row.UpdatedAt,
		}
	}
	return summaries, nil
}
