// Package service — bagian auth: register, login, refresh token (dengan rotation),
// dan logout. Session (refresh token) disimpan di tabel sessions agar bisa
// dipakai lintas device (mis. mobile + web) dan bisa di-revoke.
package service

import (
	"context"
	"errors"
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
}

func NewAuthService(q *sqlc.Queries, jwtManager *authutil.JWTManager, refreshTTL time.Duration, mailer *Mailer) *AuthService {
	return &AuthService{q: q, jwtManager: jwtManager, refreshTTL: refreshTTL, mailer: mailer}
}

// Register membuat user baru lalu langsung mengembalikan token pair (auto-login).
func (s *AuthService) Register(ctx context.Context, email, password, userAgent string) (TokenPair, error) {
	if ctx.Err() != nil {
		return TokenPair{}, ctx.Err()
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
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation (email)
			return TokenPair{}, ErrEmailTaken
		}
		return TokenPair{}, err
	}

	// Generate & simpan token verifikasi, lalu kirim email (best-effort —
	// kalau gagal kirim, user tetap bisa minta resend nanti; register tidak dibatalkan).
	token, err := authutil.GenerateVerificationToken()
	if err == nil {
		_ = s.q.SetVerificationToken(ctx, sqlc.SetVerificationTokenParams{
			ID:                    id,
			VerificationTokenHash: pgtype.Text{String: authutil.HashVerificationToken(token), Valid: true},
			VerificationExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(verificationTokenTTL), Valid: true},
		})
		go func() {
			// Kirim di background supaya response register tidak nunggu Resend.
			_ = s.mailer.SendVerificationEmail(context.Background(), email, token)
		}()
	}

	return s.issueTokenPair(ctx, id, userAgent)
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

	if !authutil.VerifyPassword(u.PasswordHash, password) {
		return TokenPair{}, ErrInvalidCredential
	}

	return s.issueTokenPair(ctx, u.ID, userAgent)
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

// Logout me-revoke satu session berdasarkan refresh token yang dikirim client.
// Device lain yang masih login tidak terpengaruh (logout per-device).
func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return s.q.RevokeSessionByTokenHash(ctx, authutil.HashRefreshToken(refreshToken))
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
