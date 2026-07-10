package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"notepad-sharelink/internal/service"
)

type AuthHandler struct {
	svc *service.AuthService
}

func NewAuthHandler(svc *service.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

type authResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type registerRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

// Register menangani POST /api/auth/register.
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

	c.JSON(http.StatusCreated, authResponse{AccessToken: tokens.AccessToken, RefreshToken: tokens.RefreshToken})
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// Login menangani POST /api/auth/login.
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

	c.JSON(http.StatusOK, authResponse{AccessToken: tokens.AccessToken, RefreshToken: tokens.RefreshToken})
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// Refresh menangani POST /api/auth/refresh.
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tokens, err := h.svc.Refresh(c.Request.Context(), req.RefreshToken, c.Request.UserAgent())
	if err != nil {
		respondAuthError(c, err)
		return
	}

	c.JSON(http.StatusOK, authResponse{AccessToken: tokens.AccessToken, RefreshToken: tokens.RefreshToken})
}

// Logout menangani POST /api/auth/logout — revoke satu session (per-device).
func (h *AuthHandler) Logout(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.svc.Logout(c.Request.Context(), req.RefreshToken); err != nil {
		respondAuthError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "berhasil logout"})
}

func respondAuthError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrEmailTaken):
		c.JSON(http.StatusConflict, gin.H{"error": "email sudah terdaftar"})
	case errors.Is(err, service.ErrInvalidCredential):
		c.JSON(http.StatusUnauthorized, gin.H{"error": "email atau password salah"})
	case errors.Is(err, service.ErrSessionInvalid):
		c.JSON(http.StatusUnauthorized, gin.H{"error": "session tidak valid, silakan login ulang"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}
