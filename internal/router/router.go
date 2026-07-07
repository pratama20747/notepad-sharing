// Package router mendefinisikan seluruh route HTTP aplikasi.
package router

import (
	"github.com/gin-gonic/gin"

	"notepad-sharelink/internal/handler"
)

// New membuat *gin.Engine lengkap dengan seluruh route ter-registrasi.
func New(noteHandler *handler.NoteHandler) *gin.Engine {
	r := gin.Default()

	// CORS sederhana untuk MVP — perketat allowed origin saat production.
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// Sajikan file statis frontend (CSS, JS, assets, dll)
	r.Static("/assets", "./frontend")

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	notes := r.Group("/api/notes")
	{
		notes.GET("", noteHandler.List)
		notes.POST("", noteHandler.Create)
		notes.GET("/:id", noteHandler.Get)
		notes.POST("/:id/unlock", noteHandler.Unlock)
		notes.PUT("/:id", noteHandler.Update)
		notes.DELETE("/:id", noteHandler.Delete)
	}

	// Catch-all: semua route yang tidak match (termasuk /n/:id) akan menyajikan index.html
	// sehingga JS handleShareLink bisa membaca path dan fetch note via API.
	r.NoRoute(func(c *gin.Context) {
		c.File("./frontend/index.html")
	})

	return r
}
