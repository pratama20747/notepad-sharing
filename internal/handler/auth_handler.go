// Package handler menerjemahkan HTTP request/response (Gin) ke pemanggilan
// service layer untuk operasi autentikasi.
package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"notepad-sharelink/internal/service"
)

const (
	refreshTokenCookie = "refresh_token"
	refreshTokenMaxAge = 30 * 24 * 60 * 60 // 30 hari dalam detik
)

// AuthHandler menampung dependency yang dibutuhkan handler auth.
type AuthHandler struct {
	svc          *service.AuthService
	cookieSecure bool // true di production (HTTPS), false di development (HTTP)
}

// NewAuthHandler membuat AuthHandler baru.
// cookieSecure harus true di production (HTTPS) dan false di development (HTTP lokal).
func NewAuthHandler(svc *service.AuthService, cookieSecure bool) *AuthHandler {
	return &AuthHandler{svc: svc, cookieSecure: cookieSecure}
}

type registerRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

type accessTokenResponse struct {
	AccessToken string `json:"access_token"`
	// refresh_token tidak di-return di body — disimpan di HttpOnly cookie
	// agar tidak bisa diakses oleh JavaScript (XSS protection)
}

// Register menangani POST /api/auth/register.
// Setelah register berhasil, refresh token di-set sebagai HttpOnly cookie
// dan access token dikembalikan di body response.
func (h *AuthHandler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tokens, err := h.svc.Register(c.Request.Context(), req.Email, req.Password, c.Request.UserAgent())
	if err != nil {
		respondAuthError(c, err)
		return
	}

	h.setRefreshTokenCookie(c, tokens.RefreshToken)
	c.JSON(http.StatusCreated, accessTokenResponse{AccessToken: tokens.AccessToken})
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// Login menangani POST /api/auth/login.
// Refresh token di-set sebagai HttpOnly cookie, access token di body.
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tokens, err := h.svc.Login(c.Request.Context(), req.Email, req.Password, c.Request.UserAgent())
	if err != nil {
		respondAuthError(c, err)
		return
	}

	h.setRefreshTokenCookie(c, tokens.RefreshToken)
	c.JSON(http.StatusOK, accessTokenResponse{AccessToken: tokens.AccessToken})
}

// Refresh menangani POST /api/auth/refresh.
// Refresh token dibaca dari HttpOnly cookie (bukan body) — client tidak perlu
// mengirim apapun di body untuk endpoint ini.
func (h *AuthHandler) Refresh(c *gin.Context) {
	refreshToken, err := c.Cookie(refreshTokenCookie)
	if err != nil || refreshToken == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "refresh token tidak ditemukan"})
		return
	}

	tokens, err := h.svc.Refresh(c.Request.Context(), refreshToken, c.Request.UserAgent())
	if err != nil {
		// Jika session invalid/expired, hapus cookie yang ada
		h.clearRefreshTokenCookie(c)
		respondAuthError(c, err)
		return
	}

	// Set cookie baru dengan refresh token baru (rotation)
	h.setRefreshTokenCookie(c, tokens.RefreshToken)
	c.JSON(http.StatusOK, accessTokenResponse{AccessToken: tokens.AccessToken})
}

// Logout menangani POST /api/auth/logout.
// Refresh token dibaca dari cookie, session di-revoke, lalu cookie dihapus.
// Device lain yang masih login tidak terpengaruh (per-device logout).
func (h *AuthHandler) Logout(c *gin.Context) {
	refreshToken, err := c.Cookie(refreshTokenCookie)
	if err != nil || refreshToken == "" {
		// Tidak ada cookie pun tetap return OK — idempotent
		c.JSON(http.StatusOK, gin.H{"message": "berhasil logout"})
		return
	}

	// Best-effort: error revoke diabaikan karena cookie tetap dihapus
	_ = h.svc.Logout(c.Request.Context(), refreshToken)

	h.clearRefreshTokenCookie(c)
	c.JSON(http.StatusOK, gin.H{"message": "berhasil logout"})
}

// setRefreshTokenCookie menyimpan refresh token ke HttpOnly cookie.
// HttpOnly: tidak bisa diakses JavaScript → aman dari XSS
// Secure: hanya dikirim lewat HTTPS (di production)
// SameSite=Strict: tidak dikirim di cross-site request → aman dari CSRF
func (h *AuthHandler) setRefreshTokenCookie(c *gin.Context, token string) {
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(
		refreshTokenCookie,
		token,
		refreshTokenMaxAge,
		"/api/auth", // cookie hanya dikirim ke path /api/auth (minimal exposure)
		"",          // domain: kosong = domain saat ini
		h.cookieSecure,
		true, // HttpOnly: tidak bisa diakses JavaScript
	)
}

// clearRefreshTokenCookie menghapus cookie refresh token (set MaxAge = -1).
func (h *AuthHandler) clearRefreshTokenCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(
		refreshTokenCookie,
		"",
		-1,
		"/api/auth",
		"",
		h.cookieSecure,
		true,
	)
}

func respondAuthError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrEmailTaken):
		c.JSON(http.StatusConflict, gin.H{"error": "email sudah terdaftar"})
	case errors.Is(err, service.ErrInvalidCredential):
		// Sengaja tidak bilang "email tidak ada" atau "password salah" secara spesifik
		// agar attacker tidak bisa enumerate email yang terdaftar
		c.JSON(http.StatusUnauthorized, gin.H{"error": "email atau password salah"})
	case errors.Is(err, service.ErrSessionInvalid):
		c.JSON(http.StatusUnauthorized, gin.H{"error": "session tidak valid, silakan login ulang"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}

// MobileRefreshHandler adalah alternatif endpoint refresh untuk Flutter mobile
// yang tidak bisa pakai cookie (kirim refresh_token di body JSON).
// Route: POST /api/auth/refresh-mobile
func (h *AuthHandler) MobileRefreshHandler(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tokens, err := h.svc.Refresh(c.Request.Context(), req.RefreshToken, c.Request.UserAgent())
	if err != nil {
		respondAuthError(c, err)
		return
	}

	// Mobile: return kedua token di body (tidak pakai cookie)
	c.JSON(http.StatusOK, gin.H{
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
	})
}

// MobileLogoutHandler adalah alternatif endpoint logout untuk Flutter mobile.
// Route: POST /api/auth/logout-mobile
func (h *AuthHandler) MobileLogoutHandler(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_ = h.svc.Logout(c.Request.Context(), req.RefreshToken)
	c.JSON(http.StatusOK, gin.H{"message": "berhasil logout"})
}

// MobileLoginHandler return kedua token di body untuk Flutter mobile.
// Route: POST /api/auth/login-mobile
func (h *AuthHandler) MobileLoginHandler(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tokens, err := h.svc.Login(c.Request.Context(), req.Email, req.Password, c.Request.UserAgent())
	if err != nil {
		respondAuthError(c, err)
		return
	}

	// Mobile: return kedua token di body
	c.JSON(http.StatusOK, gin.H{
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
	})
}
