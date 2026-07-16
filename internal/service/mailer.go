// Package service — mailer.go berisi wrapper sederhana untuk mengirim email
// verifikasi lewat Resend API (https://resend.com/docs/api-reference/emails/send-email).
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type Mailer struct {
	apiKey    string
	fromEmail string
	baseURL   string // URL frontend/backend untuk link verifikasi, mis. "http://localhost:8080"
}

func NewMailer(apiKey, fromEmail, baseURL string) *Mailer {
	return &Mailer{apiKey: apiKey, fromEmail: fromEmail, baseURL: baseURL}
}

type resendEmailPayload struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Html    string   `json:"html"`
}

// SendVerificationEmail mengirim email berisi link verifikasi ke user.
// Link mengarah ke endpoint backend GET /api/auth/verify-email?token=...
func (m *Mailer) SendVerificationEmail(ctx context.Context, toEmail, token string) error {
	verifyURL := fmt.Sprintf("%s/api/auth/verify-email?token=%s", m.baseURL, token)

	html := fmt.Sprintf(`
		<h2>Verifikasi email kamu</h2>
		<p>Klik link di bawah untuk verifikasi email dan mengaktifkan akun Notepad Sharelink kamu.</p>
		<p><a href="%s">Verifikasi Email</a></p>
		<p>Link ini berlaku selama 24 jam. Kalau bukan kamu yang mendaftar, abaikan email ini.</p>
	`, verifyURL)

	payload := resendEmailPayload{
		From:    m.fromEmail,
		To:      []string{toEmail},
		Subject: "Verifikasi email — Notepad Sharelink",
		Html:    html,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("resend: gagal kirim email, status %d", resp.StatusCode)
	}
	return nil
}
