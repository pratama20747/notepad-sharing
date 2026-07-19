// Package oauthutil membungkus komunikasi ke Google OAuth 2.0 (authorization
// code flow, server-side redirect) — tanpa library eksternal tambahan.
package oauthutil

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const (
	googleAuthURL     = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL    = "https://oauth2.googleapis.com/token"
	googleUserInfoURL = "https://www.googleapis.com/oauth2/v3/userinfo"
)

// GoogleConfig menyimpan kredensial & URL yang dibutuhkan untuk OAuth flow.
type GoogleConfig struct {
	ClientID            string
	ClientSecret        string
	RedirectURL         string // URL callback backend, mis. https://api.domain.com/api/auth/google/callback
	FrontendRedirectURL string // URL frontend tujuan setelah login sukses
}

// Enabled mengecek apakah kredensial Google OAuth sudah dikonfigurasi.
func (g *GoogleConfig) Enabled() bool {
	return g != nil && g.ClientID != "" && g.ClientSecret != ""
}

// AuthURL membangun URL consent screen Google, termasuk state untuk CSRF protection.
func (g *GoogleConfig) AuthURL(state string) string {
	v := url.Values{}
	v.Set("client_id", g.ClientID)
	v.Set("redirect_uri", g.RedirectURL)
	v.Set("response_type", "code")
	v.Set("scope", "openid email profile")
	v.Set("state", state)
	v.Set("access_type", "online")
	v.Set("prompt", "select_account")
	return googleAuthURL + "?" + v.Encode()
}

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	IDToken     string `json:"id_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

// ExchangeCode menukar authorization code dari callback dengan access token Google.
func (g *GoogleConfig) ExchangeCode(ctx context.Context, code string) (*TokenResponse, error) {
	form := url.Values{}
	form.Set("client_id", g.ClientID)
	form.Set("client_secret", g.ClientSecret)
	form.Set("code", code)
	form.Set("grant_type", "authorization_code")
	form.Set("redirect_uri", g.RedirectURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("google token exchange gagal, status %d", resp.StatusCode)
	}

	var tr TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, err
	}
	return &tr, nil
}

type GoogleUserInfo struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
}

// GetUserInfo mengambil profil user dari Google pakai access token hasil ExchangeCode.
func (g *GoogleConfig) GetUserInfo(ctx context.Context, accessToken string) (*GoogleUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleUserInfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("google userinfo gagal, status %d", resp.StatusCode)
	}

	var info GoogleUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}
	return &info, nil
}
