// Package service — cleanup untuk menghapus user yang belum verifikasi email
// dan sudah lewat masa tenggang, supaya tabel users tidak menumpuk sampah
// dari pendaftaran yang tidak pernah diselesaikan.
package service

import (
	"context"
	"log"
	"time"

	"notepad-sharelink/internal/db/sqlc"
)

type VerificationCleaner struct {
	q        *sqlc.Queries
	interval time.Duration
}

func NewVerificationCleaner(q *sqlc.Queries, interval time.Duration) *VerificationCleaner {
	return &VerificationCleaner{q: q, interval: interval}
}

func (vc *VerificationCleaner) Start(ctx context.Context) {
	log.Printf("verification cleaner: mulai, interval %s", vc.interval)

	ticker := time.NewTicker(vc.interval)
	defer ticker.Stop()

	vc.run(ctx)

	for {
		select {
		case <-ticker.C:
			vc.run(ctx)
		case <-ctx.Done():
			log.Println("verification cleaner: berhenti")
			return
		}
	}
}

func (vc *VerificationCleaner) run(ctx context.Context) {
	deleted, err := vc.q.DeleteUnverifiedUsers(ctx)
	if err != nil {
		log.Printf("verification cleaner: error saat cleanup: %v", err)
		return
	}
	if deleted > 0 {
		log.Printf("verification cleaner: %d user unverified dihapus", deleted)
	}
}
