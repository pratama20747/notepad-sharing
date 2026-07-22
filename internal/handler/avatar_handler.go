// Package handler — avatar_handler.go menangani upload avatar user.
package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"notepad-sharelink/internal/middleware"
	"notepad-sharelink/internal/service"
)

type AvatarHandler struct {
	svc *service.AvatarService
}

func NewAvatarHandler(svc *service.AvatarService) *AvatarHandler {
	return &AvatarHandler{svc: svc}
}

type presignAvatarRequest struct {
	ContentType string `json:"content_type" binding:"required"`
	FileSize    int64  `json:"file_size" binding:"required"`
}

func (h *AvatarHandler) PresignUpload(c *gin.Context) {
	var req presignAvatarRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := middleware.UserID(c)
	uploadURL, key, err := h.svc.PresignUpload(c.Request.Context(), userID, req.ContentType, req.FileSize)
	if err != nil {
		respondAvatarError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"upload_url": uploadURL, "key": key})
}

type confirmAvatarRequest struct {
	Key string `json:"key" binding:"required"`
}

func (h *AvatarHandler) ConfirmUpload(c *gin.Context) {
	var req confirmAvatarRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := middleware.UserID(c)
	url, err := h.svc.ConfirmUpload(c.Request.Context(), userID, req.Key)
	if err != nil {
		respondAvatarError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"avatar_url": url})
}

func respondAvatarError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrContentTypeBad):
		c.JSON(http.StatusBadRequest, gin.H{"error": "tipe file tidak diizinkan"})
	case errors.Is(err, service.ErrFileTooLarge):
		c.JSON(http.StatusBadRequest, gin.H{"error": "ukuran file melebihi batas maksimal"})
	case errors.Is(err, service.ErrUploadNotFound):
		c.JSON(http.StatusBadRequest, gin.H{"error": "file belum berhasil diupload ke storage"})
	case errors.Is(err, service.ErrForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": "tidak diizinkan"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}
