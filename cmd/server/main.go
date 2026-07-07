// Command server menjalankan HTTP server notepad-sharelink.
package main

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"notepad-sharelink/internal/config"
	"notepad-sharelink/internal/db/sqlc"
	"notepad-sharelink/internal/handler"
	"notepad-sharelink/internal/router"
	"notepad-sharelink/internal/service"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("gagal load config: %v", err)
	}

	ctx := context.Background()

	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("gagal parse database url: %v", err)
	}
	poolCfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		log.Fatalf("gagal konek ke database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("gagal ping database: %v", err)
	}
	log.Println("berhasil konek ke database")

	queries := sqlc.New(pool)
	noteService := service.NewNoteService(queries)
	noteHandler := handler.NewNoteHandler(noteService)

	r := router.New(noteHandler)

	log.Printf("server berjalan di port %s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("server gagal berjalan: %v", err)
	}
}
