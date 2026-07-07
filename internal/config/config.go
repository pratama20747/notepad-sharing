// Package config memuat konfigurasi aplikasi dari environment variable / file .env.
package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// Config menyimpan seluruh konfigurasi yang dibutuhkan aplikasi.
type Config struct {
	DatabaseURL string
	Port        string
}

// Load membaca file .env (jika ada) lalu memvalidasi environment variable wajib.
func Load() (*Config, error) {
	// Abaikan error jika file .env tidak ada, karena di production
	// env var biasanya di-set langsung oleh platform hosting.
	_ = godotenv.Load()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("environment variable DATABASE_URL wajib di-set")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	return &Config{
		DatabaseURL: dbURL,
		Port:        port,
	}, nil
}
