// Package service — cleanup untuk pending_password_hash yang expired karena
// user tidak pernah klik link merge password-ke-akun-Google. Tanpa cleaner
// ini, kolom pending_password_hash bisa numpuk terus kalau ada banyak
// percobaan merge yang tidak pernah diselesaikan.
package service

import (
	"context"
	"log"
	"time"

	"notepad-sharelink/internal/db/sqlc"
)

type PendingPasswordCleaner struct {
	q        *sqlc.Queries
	interval time.Duration
}

func NewPendingPasswordCleaner(q *sqlc.Queries, interval time.Duration) *PendingPasswordCleaner {
	return &PendingPasswordCleaner{q: q, interval: interval}
}

func (pc *PendingPasswordCleaner) Start(ctx context.Context) {
	log.Printf("pending password cleaner: mulai, interval %s", pc.interval)

	ticker := time.NewTicker(pc.interval)
	defer ticker.Stop()

	pc.run(ctx)

	for {
		select {
		case <-ticker.C:
			pc.run(ctx)
		case <-ctx.Done():
			log.Println("pending password cleaner: berhenti")
			return
		}
	}
}

func (pc *PendingPasswordCleaner) run(ctx context.Context) {
	cleared, err := pc.q.ClearExpiredPendingPasswords(ctx)
	if err != nil {
		log.Printf("pending password cleaner: error saat cleanup: %v", err)
		return
	}
	if cleared > 0 {
		log.Printf("pending password cleaner: %d pending password expired dibersihkan", cleared)
	}
}
