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

	return &Config{
		DatabaseURL:     dbURL,
		Port:            port,
		JWTSecret:       jwtSecret,
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 30 * 24 * time.Hour,
		IsProd:          isProd,
	}, nil
}
