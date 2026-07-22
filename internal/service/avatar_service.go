// Package service — avatar_service.go menangani avatar Google (mirror ke R2)
// dan upload avatar manual oleh user (presigned URL pattern).
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"notepad-sharelink/internal/db/sqlc"
	"notepad-sharelink/internal/fileutil"
	"notepad-sharelink/internal/storage"
)

var (
	ErrFileTooLarge   = errors.New("ukuran file melebihi batas maksimal")
	ErrContentTypeBad = errors.New("tipe file tidak diizinkan")
	ErrUploadNotFound = errors.New("file belum diupload atau upload gagal")
)

type AvatarService struct {
	q             *sqlc.Queries
	storage       *storage.Client
	maxAvatarSize int64
}

func NewAvatarService(q *sqlc.Queries, storage *storage.Client, maxAvatarSize int64) *AvatarService {
	return &AvatarService{q: q, storage: storage, maxAvatarSize: maxAvatarSize}
}

// MirrorGoogleAvatar mendownload foto profil Google lalu upload ke R2.
// Dipanggil fire-and-forget dari AuthService.GoogleLogin — best-effort,
// TIDAK boleh menggagalkan proses login.
func (s *AvatarService) MirrorGoogleAvatar(ctx context.Context, sourceURL, userID string) error {
	key := fmt.Sprintf("avatars/%s/google-%d.jpg", userID, time.Now().Unix())

	avatarURL, err := s.storage.MirrorFromURL(ctx, sourceURL, key)
	if err != nil {
		return err
	}

	return s.q.SetAvatar(ctx, sqlc.SetAvatarParams{
		ID:           userID,
		AvatarUrl:    pgtype.Text{String: avatarURL, Valid: true},
		AvatarSource: "google",
	})
}

// PresignUpload memvalidasi request lalu mengembalikan presigned PUT URL.
func (s *AvatarService) PresignUpload(ctx context.Context, userID, contentType string, fileSize int64) (uploadURL, key string, err error) {
	if !fileutil.AllowedImageTypes[contentType] {
		return "", "", ErrContentTypeBad
	}
	if fileSize <= 0 || fileSize > s.maxAvatarSize {
		return "", "", ErrFileTooLarge
	}

	rnd, err := storage.RandomHex(8)
	if err != nil {
		return "", "", err
	}
	ext := fileutil.ExtFromContentType(contentType)
	key = fmt.Sprintf("avatars/%s/%d-%s.%s", userID, time.Now().Unix(), rnd, ext)

	uploadURL, err = s.storage.GeneratePresignedPutURL(ctx, key, contentType)
	if err != nil {
		return "", "", err
	}
	return uploadURL, key, nil
}

// ConfirmUpload memvalidasi ulang object yang sudah di-PUT ke R2 (jangan
// percaya klaim dari client), lalu update avatar_url di DB dan hapus avatar
// lama kalau ada.
func (s *AvatarService) ConfirmUpload(ctx context.Context, userID, key string) (string, error) {
	// Validasi kepemilikan key sederhana: key avatar harus di bawah folder user ini.
	prefix := fmt.Sprintf("avatars/%s/", userID)
	if len(key) < len(prefix) || key[:len(prefix)] != prefix {
		return "", ErrForbidden
	}

	size, _, err := s.storage.HeadObject(ctx, key)
	if err != nil {
		return "", ErrUploadNotFound
	}
	if size > s.maxAvatarSize {
		_ = s.storage.DeleteObject(ctx, key)
		return "", ErrFileTooLarge
	}

	head, err := s.storage.GetObjectRange(ctx, key, 512)
	if err != nil {
		return "", err
	}
	if _, err := fileutil.DetectAndValidate(head, "", fileutil.AllowedImageTypes); err != nil {
		_ = s.storage.DeleteObject(ctx, key)
		return "", err
	}

	// Ambil avatar lama untuk dihapus setelah avatar baru berhasil di-set.
	oldUser, err := s.q.GetUserByID(ctx, userID)
	if err != nil {
		return "", err
	}

	newURL := s.storage.PublicURL(key)
	if err := s.q.SetAvatar(ctx, sqlc.SetAvatarParams{
		ID:           userID,
		AvatarUrl:    pgtype.Text{String: newURL, Valid: true},
		AvatarSource: "upload",
	}); err != nil {
		return "", err
	}

	// Hapus avatar lama HANYA kalau sumbernya juga upload
	if oldUser.AvatarSource == "upload" && oldUser.AvatarUrl.Valid {
		oldKey := extractKeyFromURL(s.storage.PublicURL(""), oldUser.AvatarUrl.String)
		if oldKey != "" {
			_ = s.storage.DeleteObject(ctx, oldKey)
		}
	}

	return newURL, nil
}

func extractKeyFromURL(base, fullURL string) string {
	if len(fullURL) <= len(base) {
		return ""
	}
	return fullURL[len(base):]
}
