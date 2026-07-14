// Command server menjalankan HTTP server notepad-sharelink.
package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"notepad-sharelink/internal/authutil"
	"notepad-sharelink/internal/config"
	"notepad-sharelink/internal/db/sqlc"
	"notepad-sharelink/internal/handler"
	"notepad-sharelink/internal/router"
	"notepad-sharelink/internal/service"
)

func main() {
	// Setup structured logger (JSON di production, text di development)
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("gagal load config: %v", err)
	}

	setupLogger(cfg.IsProd)

	// Context untuk sinyal interrupt
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		slog.Error("gagal parse database url", "error", err)
		os.Exit(1)
	}
	poolCfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		slog.Error("gagal konek ke database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		slog.Error("gagal ping database", "error", err)
		os.Exit(1)
	}
	slog.Info("berhasil konek ke database")

	queries := sqlc.New(pool)

	jwtManager := authutil.NewJWTManager(cfg.JWTSecret, cfg.AccessTokenTTL)

	noteService := service.NewNoteService(queries)
	authService := service.NewAuthService(queries, jwtManager, cfg.RefreshTokenTTL)

	noteHandler := handler.NewNoteHandler(noteService)
	authHandler := handler.NewAuthHandler(authService, cfg.IsProd)

	// Jalankan session cleaner di background — cleanup setiap 1 jam
	cleaner := service.NewSessionCleaner(queries, 1*time.Hour)
	go cleaner.Start(ctx)

	// Buat HTTP server dari router
	r := router.New(noteHandler, authHandler, jwtManager)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	// Jalankan server di goroutine
	go func() {
		slog.Info("server berjalan", "port", cfg.Port, "prod", cfg.IsProd)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server gagal berjalan", "error", err)
			os.Exit(1)
		}
	}()

	// Tunggu sinyal interrupt
	<-ctx.Done()
	slog.Info("menerima sinyal shutdown, menghentikan server...")

	// Buat context dengan timeout untuk graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	// Shutdown server
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
	}

	slog.Info("server berhenti dengan graceful")
}

// setupLogger mengkonfigurasi slog sebagai default logger.
// Production: JSON format (mudah di-parse oleh log aggregator seperti Loki/Datadog).
// Development: Text format (mudah dibaca manusia).
func setupLogger(isProd bool) {
	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}

	if isProd {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	slog.SetDefault(slog.New(handler))
}
