// Package handler menerjemahkan HTTP request/response (Gin) ke pemanggilan
// service layer. Tidak ada business logic di sini — hanya binding,
// validasi input dasar, dan pemetaan error ke HTTP status code.
package handler

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"notepad-sharelink/internal/middleware"
	"notepad-sharelink/internal/service"
)

// NoteHandler menampung dependency yang dibutuhkan handler.
type NoteHandler struct {
	svc *service.NoteService
}

// NewNoteHandler membuat NoteHandler baru.
func NewNoteHandler(svc *service.NoteService) *NoteHandler {
	return &NoteHandler{svc: svc}
}

type createNoteRequest struct {
	Mode     string `json:"mode" binding:"required,oneof=public private"`
	Title    string `json:"title"`
	Content  string `json:"content"`
	Password string `json:"password"`
}

type createNoteResponse struct {
	ID       string `json:"id"`
	ShareURL string `json:"share_url"`
	Mode     string `json:"mode"`
	Title    string `json:"title"`
}

// Create menangani POST /api/notes.
// Endpoint ini BUTUH LOGIN (protected oleh middleware.RequireAuth).
// User yang login akan menjadi pemilik note (user_id dari JWT token).
func (h *NoteHandler) Create(c *gin.Context) {
	var req createNoteRequest
	ctx := c.Request.Context()
	if ctx.Err() != nil {
		c.JSON(499, gin.H{"error": "request dibatalkan"})
		return
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Mode == service.ModePrivate && len(req.Password) < 4 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password minimal 4 karakter untuk mode private"})
		return
	}

	if len(req.Title) > 200 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "judul terlalu panjang, maksimal 200 karakter"})
		return
	}

	title := req.Title
	if title == "" {
		title = "Catatan tanpa judul"
	}

	// Ambil userID dari context (di-set oleh middleware.RequireAuth)
	userID := middleware.UserID(c)

	id, err := h.svc.CreateNote(ctx, req.Mode, title, req.Content, req.Password, userID)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusCreated, createNoteResponse{
		ID:       id,
		ShareURL: "/n/" + id,
		Mode:     req.Mode,
		Title:    title,
	})
}

// Get menangani GET /api/notes/:id.
// Endpoint ini PUBLIK (tanpa middleware login) — siapapun bisa akses via share link.
//
// Untuk note public, title & content langsung dikembalikan.
// Untuk note private, hanya info bahwa note "locked" yang dikembalikan
// beserta title (plaintext); klien harus memanggil endpoint Unlock dengan password.
func (h *NoteHandler) Get(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()
	if ctx.Err() != nil {
		c.JSON(499, gin.H{"error": "request dibatalkan"})
		return
	}

	mode, title, content, err := h.svc.GetNoteMeta(ctx, id)
	if err != nil {
		respondError(c, err)
		return
	}

	resp := gin.H{"id": id, "mode": mode, "title": title}
	if mode == service.ModePublic {
		resp["content"] = content
	} else {
		resp["locked"] = true
	}

	c.JSON(http.StatusOK, resp)
}

type unlockRequest struct {
	Password string `json:"password" binding:"required"`
}

// Unlock menangani POST /api/notes/:id/unlock untuk mode private.
// Endpoint ini PUBLIK (tanpa middleware login) — siapapun bisa unlock dengan password note.
// Dipakai saat user akses share link note private dan ingin membuka konten.
func (h *NoteHandler) Unlock(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()
	if ctx.Err() != nil {
		c.JSON(499, gin.H{"error": "request dibatalkan"})
		return
	}
	var req unlockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	title, content, err := h.svc.UnlockPrivateNote(ctx, id, req.Password)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": id, "title": title, "content": content})
}

type updateNoteRequest struct {
	Title    string `json:"title"`
	Content  string `json:"content"`
	Password string `json:"password"`
}

// Update menangani PUT /api/notes/:id.
// Endpoint ini BUTUH LOGIN (protected oleh middleware.RequireAuth).
// Hanya pemilik note (user_id dari JWT token) yang bisa update note.
// Jika user lain mencoba update, akan di-reject dengan 403 Forbidden.
func (h *NoteHandler) Update(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()
	if ctx.Err() != nil {
		c.JSON(499, gin.H{"error": "request dibatalkan"})
		return
	}
	var req updateNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	title := req.Title
	if title == "" {
		title = "Catatan tanpa judul"
	}

	// Ambil userID dari context (di-set oleh middleware.RequireAuth)
	userID := middleware.UserID(c)

	if err := h.svc.UpdateNote(ctx, id, title, req.Content, req.Password, userID); err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "note berhasil diupdate"})
}

type deleteNoteRequest struct {
	Password string `json:"password"`
}

// Delete menangani DELETE /api/notes/:id.
// Endpoint ini BUTUH LOGIN (protected oleh middleware.RequireAuth).
// Hanya pemilik note (user_id dari JWT token) yang bisa delete note.
// Jika user lain mencoba delete, akan di-reject dengan 403 Forbidden.
func (h *NoteHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()
	if ctx.Err() != nil {
		c.JSON(499, gin.H{"error": "request dibatalkan"})
		return
	}
	var req deleteNoteRequest
	// Body opsional untuk note public, sengaja abaikan binding error
	// (mis. body kosong pada request DELETE tanpa password).
	_ = c.ShouldBindJSON(&req)

	// Ambil userID dari context (di-set oleh middleware.RequireAuth)
	userID := middleware.UserID(c)

	if err := h.svc.DeleteNote(ctx, id, req.Password, userID); err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "note berhasil dihapus"})
}

// List menangani GET /api/notes (tanpa :id).
// Endpoint ini BUTUH LOGIN (protected oleh middleware.RequireAuth).
// User hanya melihat note punya mereka sendiri (filter by user_id dari JWT token).
// Note user lain TIDAK muncul di list.
func (h *NoteHandler) List(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "20")
	offsetStr := c.DefaultQuery("offset", "0")
	ctx := c.Request.Context()
	if ctx.Err() != nil {
		c.JSON(499, gin.H{"error": "request dibatalkan"})
		return
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = 0
	}

	// Ambil userID dari context (di-set oleh middleware.RequireAuth)
	userID := middleware.UserID(c)

	notes, err := h.svc.ListNotes(ctx, userID, int32(limit), int32(offset))
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"notes": notes})
}

// respondError memetakan sentinel error dari service layer ke HTTP status code.
func respondError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "note tidak ditemukan"})
	case errors.Is(err, service.ErrForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": "anda tidak punya akses untuk mengubah note ini"})
	case errors.Is(err, context.Canceled):
		c.JSON(499, gin.H{"error": "request dibatalkan oleh client"})
	case errors.Is(err, service.ErrWrongPassword):
		c.JSON(http.StatusUnauthorized, gin.H{"error": "password salah"})
	case errors.Is(err, service.ErrPasswordNeeded):
		c.JSON(http.StatusBadRequest, gin.H{"error": "password wajib diisi"})
	case errors.Is(err, service.ErrInvalidMode):
		c.JSON(http.StatusBadRequest, gin.H{"error": "mode tidak valid"})
	case errors.Is(err, service.ErrTitleTooLong):
		c.JSON(http.StatusBadRequest, gin.H{"error": "judul terlalu panjang, maksimal 200 karakter"})
	default:
		log.Printf("internal error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}
