// Package service — bagian auth: register, login, refresh token (dengan rotation),
// dan logout. Session (refresh token) disimpan di tabel sessions agar bisa
// dipakai lintas device (mis. mobile + web) dan bisa di-revoke.
package service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"notepad-sharelink/internal/authutil"
	"notepad-sharelink/internal/db/sqlc"
	"notepad-sharelink/internal/idgen"
)

var (
	ErrEmailTaken        = errors.New("email sudah terdaftar")
	ErrInvalidCredential = errors.New("email atau password salah")
	ErrSessionInvalid    = errors.New("session tidak valid, silakan login ulang")
	ErrTokenInvalid      = errors.New("token verifikasi tidak valid atau sudah expired")
	ErrEmailNotVerified  = errors.New("email belum diverifikasi")
	ErrGoogleOnlyAccount = errors.New("akun ini terdaftar via Google, silakan login dengan Google")
	ErrMergePending      = errors.New("email sudah terdaftar via Google, cek email untuk verifikasi penautan password")
)

const verificationTokenTTL = 24 * time.Hour

type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

type AuthService struct {
	q          *sqlc.Queries
	jwtManager *authutil.JWTManager
	refreshTTL time.Duration
	mailer     *Mailer
	avatarSvc  *AvatarService
}

func NewAuthService(q *sqlc.Queries, jwtManager *authutil.JWTManager, refreshTTL time.Duration, mailer *Mailer, avatarSvc *AvatarService) *AuthService {
	return &AuthService{q: q, jwtManager: jwtManager, refreshTTL: refreshTTL, mailer: mailer, avatarSvc: avatarSvc}
}

func (s *AuthService) Register(ctx context.Context, email, password, userAgent string) (TokenPair, error) {
	if ctx.Err() != nil {
		return TokenPair{}, ctx.Err()
	}

	existing, err := s.q.GetUserByEmail(ctx, email)
	if err == nil {
		// Email sudah ada.
		if existing.GoogleID.Valid && existing.PasswordHash == "" {
			// Akun Google-only. JANGAN buat user baru, JANGAN langsung merge.
			// Simpan password sebagai pending + kirim link verifikasi ke email
			// tersebut. Password baru aktif setelah link diklik — ini membuktikan
			// yang register memang menguasai inbox email itu, bukan cuma tebak email.
			if mergeErr := s.startPasswordMerge(ctx, existing, password); mergeErr != nil {
				return TokenPair{}, mergeErr
			}
			return TokenPair{}, ErrMergePending
		}
		return TokenPair{}, ErrEmailTaken
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return TokenPair{}, err
	}

	hash, err := authutil.HashPassword(password)
	if err != nil {
		return TokenPair{}, err
	}

	id, err := idgen.New()
	if err != nil {
		return TokenPair{}, err
	}

	_, err = s.q.CreateUser(ctx, sqlc.CreateUserParams{
		ID:           id,
		Email:        email,
		PasswordHash: hash,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return TokenPair{}, ErrEmailTaken
		}
		return TokenPair{}, err
	}

	token, err := authutil.GenerateVerificationToken()
	if err == nil {
		_ = s.q.SetVerificationToken(ctx, sqlc.SetVerificationTokenParams{
			ID:                    id,
			VerificationTokenHash: pgtype.Text{String: authutil.HashVerificationToken(token), Valid: true},
			VerificationExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(verificationTokenTTL), Valid: true},
		})
		go func() {
			_ = s.mailer.SendVerificationEmail(context.Background(), email, token)
		}()
	}

	return s.issueTokenPair(ctx, id, userAgent)
}

// startPasswordMerge menyimpan password baru sebagai "pending" pada akun
// Google-only yang emailnya sama, lalu kirim email verifikasi penautan.
func (s *AuthService) startPasswordMerge(ctx context.Context, u sqlc.User, password string) error {
	hash, err := authutil.HashPassword(password)
	if err != nil {
		return err
	}

	token, err := authutil.GenerateVerificationToken()
	if err != nil {
		return err
	}

	if err := s.q.SetPendingPassword(ctx, sqlc.SetPendingPasswordParams{
		ID:                       u.ID,
		PendingPasswordHash:      pgtype.Text{String: hash, Valid: true},
		PendingPasswordTokenHash: pgtype.Text{String: authutil.HashVerificationToken(token), Valid: true},
		PendingPasswordExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(verificationTokenTTL), Valid: true},
	}); err != nil {
		return err
	}

	go func() {
		_ = s.mailer.SendMergePasswordEmail(context.Background(), u.Email, token)
	}()
	return nil
}

// VerifyMergePassword dipanggil saat user klik link di email merge.
// Kalau token valid & belum expired, password pending dipindah jadi aktif.
func (s *AuthService) VerifyMergePassword(ctx context.Context, token string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	hash := authutil.HashVerificationToken(token)
	u, err := s.q.GetUserByPendingPasswordTokenHash(ctx, pgtype.Text{String: hash, Valid: true})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTokenInvalid
		}
		return err
	}

	if !u.PendingPasswordExpiresAt.Valid || time.Now().After(u.PendingPasswordExpiresAt.Time) {
		return ErrTokenInvalid
	}

	return s.q.MergePendingPassword(ctx, u.ID)
}

// Login memverifikasi email+password lalu mengembalikan token pair baru.
func (s *AuthService) Login(ctx context.Context, email, password, userAgent string) (TokenPair, error) {
	if ctx.Err() != nil {
		return TokenPair{}, ctx.Err()
	}

	u, err := s.q.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TokenPair{}, ErrInvalidCredential
		}
		return TokenPair{}, err
	}

	if u.PasswordHash == "" {
		// Akun murni Google, belum ada password yang berhasil di-merge.
		return TokenPair{}, ErrGoogleOnlyAccount
	}

	if !authutil.VerifyPassword(u.PasswordHash, password) {
		return TokenPair{}, ErrInvalidCredential
	}

	return s.issueTokenPair(ctx, u.ID, userAgent)
}

// GoogleLogin menangani login/register via Google OAuth.
//
// Prioritas pencarian:
//  1. google_id cocok → langsung login (device Google yang sama pernah dipakai).
//  2. email cocok tapi belum ada google_id (akun password biasa) → tautkan
//     google_id ke akun itu SEKARANG JUGA. Ini aman (beda dengan arah
//     sebaliknya) karena Google sendiri sudah membuktikan kepemilikan email.
//  3. belum ada sama sekali → buat user baru, email_verified=true.
func (s *AuthService) GoogleLogin(ctx context.Context, googleID, email, picture string, userAgent string) (TokenPair, error) {
	if ctx.Err() != nil {
		return TokenPair{}, ctx.Err()
	}

	u, err := s.q.GetUserByGoogleID(ctx, pgtype.Text{String: googleID, Valid: true})
	if err == nil {
		s.maybeMirrorAvatar(u, picture)
		return s.issueTokenPair(ctx, u.ID, userAgent)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return TokenPair{}, err
	}

	existing, err := s.q.GetUserByEmail(ctx, email)
	if err == nil {
		if linkErr := s.q.LinkGoogleID(ctx, sqlc.LinkGoogleIDParams{
			ID:       existing.ID,
			GoogleID: pgtype.Text{String: googleID, Valid: true},
		}); linkErr != nil {
			return TokenPair{}, linkErr
		}
		if !existing.EmailVerified {
			_ = s.q.MarkEmailVerified(ctx, existing.ID)
		}
		s.maybeMirrorAvatar(existing, picture)
		return s.issueTokenPair(ctx, existing.ID, userAgent)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return TokenPair{}, err
	}

	id, err := idgen.New()
	if err != nil {
		return TokenPair{}, err
	}

	if _, err := s.q.CreateGoogleUser(ctx, sqlc.CreateGoogleUserParams{
		ID:       id,
		Email:    email,
		GoogleID: pgtype.Text{String: googleID, Valid: true},
	}); err != nil {
		return TokenPair{}, err
	}

	if s.avatarSvc != nil && picture != "" {
		go func() {
			bCtx := context.Background()
			if err := s.avatarSvc.MirrorGoogleAvatar(bCtx, picture, id); err != nil {
				slog.Warn("gagal mirror avatar google", "error", err, "user_id", id)
			}
		}()
	}

	return s.issueTokenPair(ctx, id, userAgent)
}

// maybeMirrorAvatar hanya mirror kalau user belum punya avatar sama sekali —
// supaya tidak menimpa avatar hasil upload manual user setiap kali login Google.
func (s *AuthService) maybeMirrorAvatar(u sqlc.User, picture string) {
	if s.avatarSvc == nil || picture == "" || u.AvatarSource == "upload" {
		return
	}
	go func() {
		bCtx := context.Background()
		if err := s.avatarSvc.MirrorGoogleAvatar(bCtx, picture, u.ID); err != nil {
			slog.Warn("gagal mirror avatar google", "error", err, "user_id", u.ID)
		}
	}()
}

// UserProfile digunakan untuk response GET /api/auth/me.
type UserProfile struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	AvatarURL     string `json:"avatar_url,omitempty"`
	AvatarSource  string `json:"avatar_source"`
	HasPassword   bool   `json:"has_password"`
	HasGoogle     bool   `json:"has_google"`
}

func (s *AuthService) GetProfile(ctx context.Context, userID string) (UserProfile, error) {
	u, err := s.q.GetUserByID(ctx, userID)
	if err != nil {
		return UserProfile{}, err
	}
	return UserProfile{
		ID:            u.ID,
		Email:         u.Email,
		EmailVerified: u.EmailVerified,
		AvatarURL:     u.AvatarUrl.String,
		AvatarSource:  u.AvatarSource,
		HasPassword:   u.PasswordHash != "",
		HasGoogle:     u.GoogleID.Valid,
	}, nil
}

// Logout me-revoke satu session berdasarkan refresh token yang dikirim client.
// Device lain yang masih login tidak terpengaruh (logout per-device).
func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return s.q.RevokeSessionByTokenHash(ctx, authutil.HashRefreshToken(refreshToken))
}

// Refresh menukar refresh token lama dengan token pair baru.
// Refresh token lama langsung di-revoke (rotation) supaya token yang sama
// tidak bisa dipakai berulang kali jika bocor.
func (s *AuthService) Refresh(ctx context.Context, refreshToken, userAgent string) (TokenPair, error) {
	if ctx.Err() != nil {
		return TokenPair{}, ctx.Err()
	}

	hash := authutil.HashRefreshToken(refreshToken)
	sess, err := s.q.GetSessionByTokenHash(ctx, hash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TokenPair{}, ErrSessionInvalid
		}
		return TokenPair{}, err
	}

	if sess.RevokedAt.Valid || time.Now().After(sess.ExpiresAt) {
		return TokenPair{}, ErrSessionInvalid
	}

	if err := s.q.RevokeSession(ctx, sess.ID); err != nil {
		return TokenPair{}, err
	}

	return s.issueTokenPair(ctx, sess.UserID, userAgent)
}

// VerifyEmail mencocokkan token dari link email, menandai user sebagai verified
// kalau valid dan belum expired.
func (s *AuthService) VerifyEmail(ctx context.Context, token string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	hash := authutil.HashVerificationToken(token)
	u, err := s.q.GetUserByVerificationTokenHash(ctx, pgtype.Text{String: hash, Valid: true})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTokenInvalid
		}
		return err
	}

	if !u.VerificationExpiresAt.Valid || time.Now().After(u.VerificationExpiresAt.Time) {
		return ErrTokenInvalid
	}

	return s.q.MarkEmailVerified(ctx, u.ID)
}

// ResendVerificationEmail generate token baru & kirim ulang, dipanggil kalau
// user belum sempat verifikasi / link lama sudah expired.
func (s *AuthService) ResendVerificationEmail(ctx context.Context, userID string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	u, err := s.q.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if u.EmailVerified {
		return nil // sudah verified, no-op
	}

	token, err := authutil.GenerateVerificationToken()
	if err != nil {
		return err
	}

	if err := s.q.SetVerificationToken(ctx, sqlc.SetVerificationTokenParams{
		ID:                    userID,
		VerificationTokenHash: pgtype.Text{String: authutil.HashVerificationToken(token), Valid: true},
		VerificationExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(verificationTokenTTL), Valid: true},
	}); err != nil {
		return err
	}

	return s.mailer.SendVerificationEmail(ctx, u.Email, token)
}

func (s *AuthService) issueTokenPair(ctx context.Context, userID, userAgent string) (TokenPair, error) {
	accessToken, err := s.jwtManager.Generate(userID)
	if err != nil {
		return TokenPair{}, err
	}

	refreshToken, err := authutil.GenerateRefreshToken()
	if err != nil {
		return TokenPair{}, err
	}

	sessID, err := idgen.New()
	if err != nil {
		return TokenPair{}, err
	}

	_, err = s.q.CreateSession(ctx, sqlc.CreateSessionParams{
		ID:        sessID,
		UserID:    userID,
		TokenHash: authutil.HashRefreshToken(refreshToken),
		UserAgent: userAgent,
		ExpiresAt: time.Now().Add(s.refreshTTL),
	})
	if err != nil {
		return TokenPair{}, err
	}

	return TokenPair{AccessToken: accessToken, RefreshToken: refreshToken}, nil
}
