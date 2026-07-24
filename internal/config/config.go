// Package config memuat konfigurasi aplikasi dari environment variable / file .env.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config menyimpan seluruh konfigurasi yang dibutuhkan aplikasi.
type Config struct {
	DatabaseURL     string
	Port            string
	JWTSecret       string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	IsProd          bool

	ResendAPIKey string
	FromEmail    string
	BaseURL      string

	GoogleClientID            string
	GoogleClientSecret        string
	GoogleRedirectURL         string
	GoogleFrontendRedirectURL string
	GoogleTimeout             time.Duration

	// Cloudflare R2
	R2AccountID       string
	R2AccessKeyID     string
	R2SecretAccessKey string
	R2BucketName      string
	R2PublicBaseURL   string
	PresignTTL        time.Duration

	// Limit ukuran file, dalam byte (diturunkan dari env dalam MB)
	MaxAvatarSize         int64
	MaxImageAttachSize    int64
	MaxVideoAttachSize    int64
	MaxAttachmentsPerNote int
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("environment variable DATABASE_URL wajib di-set")
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return nil, fmt.Errorf("environment variable JWT_SECRET wajib di-set")
	}
	if len(jwtSecret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET minimal 32 karakter untuk keamanan yang memadai")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	isProd := os.Getenv("APP_ENV") == "production"

	resendAPIKey := os.Getenv("RESEND_API_KEY")
	if resendAPIKey == "" {
		return nil, fmt.Errorf("environment variable RESEND_API_KEY wajib di-set")
	}
	fromEmail := os.Getenv("FROM_EMAIL")
	if fromEmail == "" {
		fromEmail = "onboarding@resend.dev"
	}
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:" + port
	}
	googleClientID := os.Getenv("GOOGLE_CLIENT_ID")
	googleClientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	googleRedirectURL := os.Getenv("GOOGLE_REDIRECT_URL")

	if googleRedirectURL == "" {
		googleRedirectURL = baseURL + "/api/auth/google/callback"
	}
	googleFrontendRedirectURL := os.Getenv("GOOGLE_FRONTEND_REDIRECT_URL")
	if googleFrontendRedirectURL == "" {
		googleFrontendRedirectURL = baseURL
	}
	googleTimeout := envDuration("GOOGLE_TIMEOUT", 5*time.Second)

	// --- R2 ---
	r2AccountID := os.Getenv("R2_ACCOUNT_ID")
	r2AccessKeyID := os.Getenv("R2_ACCESS_KEY_ID")
	r2SecretAccessKey := os.Getenv("R2_SECRET_ACCESS_KEY")
	r2BucketName := os.Getenv("R2_BUCKET_NAME")
	r2PublicBaseURL := os.Getenv("R2_PUBLIC_BASE_URL")

	presignMinutes := envInt("PRESIGN_TTL_MINUTES", 10)

	maxAvatarMB := envInt("MAX_AVATAR_SIZE_MB", 5)
	maxImageMB := envInt("MAX_IMAGE_ATTACHMENT_SIZE_MB", 10)
	maxVideoMB := envInt("MAX_VIDEO_ATTACHMENT_SIZE_MB", 50)
	maxAttachments := envInt("MAX_ATTACHMENTS_PER_NOTE", 10)

	return &Config{
		DatabaseURL:               dbURL,
		Port:                      port,
		JWTSecret:                 jwtSecret,
		AccessTokenTTL:            15 * time.Minute,
		RefreshTokenTTL:           30 * 24 * time.Hour,
		IsProd:                    isProd,
		ResendAPIKey:              resendAPIKey,
		FromEmail:                 fromEmail,
		BaseURL:                   baseURL,
		GoogleClientID:            googleClientID,
		GoogleClientSecret:        googleClientSecret,
		GoogleRedirectURL:         googleRedirectURL,
		GoogleFrontendRedirectURL: googleFrontendRedirectURL,
		GoogleTimeout:             googleTimeout,

		R2AccountID:       r2AccountID,
		R2AccessKeyID:     r2AccessKeyID,
		R2SecretAccessKey: r2SecretAccessKey,
		R2BucketName:      r2BucketName,
		R2PublicBaseURL:   r2PublicBaseURL,
		PresignTTL:        time.Duration(presignMinutes) * time.Minute,

		MaxAvatarSize:         int64(maxAvatarMB) * 1024 * 1024,
		MaxImageAttachSize:    int64(maxImageMB) * 1024 * 1024,
		MaxVideoAttachSize:    int64(maxVideoMB) * 1024 * 1024,
		MaxAttachmentsPerNote: maxAttachments,
	}, nil
}

// R2Enabled mengecek apakah kredensial R2 sudah dikonfigurasi.
func (c *Config) R2Enabled() bool {
	return c.R2AccountID != "" && c.R2AccessKeyID != "" && c.R2SecretAccessKey != "" && c.R2BucketName != ""
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func envDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return def
		}
		return d
	}
	return def
}
