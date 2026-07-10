package router

import (
	"github.com/gin-gonic/gin"

	"notepad-sharelink/internal/authutil"
	"notepad-sharelink/internal/handler"
	"notepad-sharelink/internal/middleware"
)

func New(noteHandler *handler.NoteHandler, authHandler *handler.AuthHandler, jwtManager *authutil.JWTManager) *gin.Engine {
	r := gin.Default()

	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	r.Static("/assets", "./frontend")

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	auth := r.Group("/api/auth")
	{
		auth.POST("/register", authHandler.Register)
		auth.POST("/login", authHandler.Login)
		auth.POST("/refresh", authHandler.Refresh)
		auth.POST("/logout", authHandler.Logout)
	}

	notes := r.Group("/api/notes")
	{
		// Publik — akses via share link, tanpa perlu login.
		notes.GET("/:id", noteHandler.Get)
		notes.POST("/:id/unlock", noteHandler.Unlock)

		// Butuh login.
		authed := notes.Group("")
		authed.Use(middleware.RequireAuth(jwtManager))
		{
			authed.GET("", noteHandler.List)
			authed.POST("", noteHandler.Create)
			authed.PUT("/:id", noteHandler.Update)
			authed.DELETE("/:id", noteHandler.Delete)
		}
	}

	r.NoRoute(func(c *gin.Context) {
		c.File("./frontend/index.html")
	})

	return r
}
