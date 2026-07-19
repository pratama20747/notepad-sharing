// Package config memuat konfigurasi aplikasi dari environment variable / file .env.
package config

import (
	"fmt"
	"os"
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
	// IsProd mengontrol behavior yang berbeda antara development dan production:
	// - Cookie Secure flag (true di production = HTTPS only)
	// - Log format (JSON di production, text di development)
	IsProd bool
	// Resend API untuk email verifikasi
	ResendAPIKey              string
	FromEmail                 string
	BaseURL                   string
	GoogleClientID            string
	GoogleClientSecret        string
	GoogleRedirectURL         string
	GoogleFrontendRedirectURL string
}

// Load membaca file .env (jika ada) lalu memvalidasi environment variable wajib.
func Load() (*Config, error) {
	// Abaikan error jika file .env tidak ada (env var di-set langsung di production)
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
		fromEmail = "onboarding@resend.dev" // default sandbox Resend
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
	}, nil
}
