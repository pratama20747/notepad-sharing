// Package service — session cleanup untuk menghapus session yang sudah
// expired atau di-revoke dari tabel sessions secara periodik.
package service

import (
	"context"
	"log"
	"time"

	"notepad-sharelink/internal/db/sqlc"
)

// SessionCleaner menjalankan cleanup session expired/revoked secara periodik
// di background goroutine agar tabel sessions tidak terus membengkak.
type SessionCleaner struct {
	q        *sqlc.Queries
	interval time.Duration
}

// NewSessionCleaner membuat SessionCleaner baru.
// interval: seberapa sering cleanup dijalankan (mis. 1 * time.Hour)
func NewSessionCleaner(q *sqlc.Queries, interval time.Duration) *SessionCleaner {
	return &SessionCleaner{q: q, interval: interval}
}

// Start menjalankan cleanup loop di background goroutine.
// Gunakan context yang bisa di-cancel untuk graceful shutdown.
//
// Contoh pemakaian di main.go:
//
//	cleaner := service.NewSessionCleaner(queries, 1*time.Hour)
//	go cleaner.Start(ctx)
func (sc *SessionCleaner) Start(ctx context.Context) {
	log.Printf("session cleaner: mulai, interval %s", sc.interval)

	ticker := time.NewTicker(sc.interval)
	defer ticker.Stop()

	// Jalankan sekali saat startup
	sc.run(ctx)

	for {
		select {
		case <-ticker.C:
			sc.run(ctx)
		case <-ctx.Done():
			log.Println("session cleaner: berhenti")
			return
		}
	}
}

func (sc *SessionCleaner) run(ctx context.Context) {
	deleted, err := sc.q.DeleteExpiredAndRevokedSessions(ctx)
	if err != nil {
		log.Printf("session cleaner: error saat cleanup: %v", err)
		return
	}
	if deleted > 0 {
		log.Printf("session cleaner: %d session expired/revoked dihapus", deleted)
	}
}
