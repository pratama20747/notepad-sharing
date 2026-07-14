package router

import (
	"os" // ⭐ Jangan lupa import os

	"notepad-sharelink/internal/authutil"
	"notepad-sharelink/internal/handler"
	"notepad-sharelink/internal/middleware"

	"github.com/gin-gonic/gin"
)

func New(
	noteHandler *handler.NoteHandler,
	authHandler *handler.AuthHandler,
	jwtManager *authutil.JWTManager,
) *gin.Engine {
	r := gin.New()

	r.Use(middleware.Logger())
	r.Use(gin.Recovery())
	r.Use(corsMiddleware())

	r.GET("/health", healthCheck)

	// Auth routes
	authLimiter, _ := middleware.NewRateLimiter("10-M")
	auth := r.Group("/api/auth")
	auth.Use(authLimiter)
	{
		auth.POST("/register", authHandler.Register)
		auth.POST("/login", authHandler.Login)
		auth.POST("/refresh", authHandler.Refresh)
		auth.POST("/logout", authHandler.Logout)
		auth.POST("/login-mobile", authHandler.MobileLoginHandler)
		auth.POST("/refresh-mobile", authHandler.MobileRefreshHandler)
		auth.POST("/logout-mobile", authHandler.MobileLogoutHandler)
	}

	// Notes routes
	notes := r.Group("/api/notes")
	{
		notes.GET("/:id", noteHandler.Get)
		notes.POST("/:id/unlock", noteHandler.Unlock)

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
		c.JSON(404, gin.H{"error": "Not found"})
	})

	return r
}

// ⭐ Fixed: CORS middleware
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// DEVELOPMENT: Allow all
		if os.Getenv("APP_ENV") == "development" {
			c.Header("Access-Control-Allow-Origin", "*")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")

			if c.Request.Method == "OPTIONS" {
				c.AbortWithStatus(204)
				return
			}
			c.Next()
			return
		}

		// PRODUCTION: Allowlist
		allowed := map[string]bool{
			"https://frontend-domain.com":     true,
			"https://www.frontend-domain.com": true,
			"http://localhost:3000":           true,
			"http://localhost:8080":           true,
		}

		if allowed[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
		}

		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

func healthCheck(c *gin.Context) {
	c.JSON(200, gin.H{"status": "ok"})
}
