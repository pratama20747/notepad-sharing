// Package router mendefinisikan seluruh route HTTP aplikasi.
package router

import (
	"log"

	"github.com/gin-gonic/gin"

	"notepad-sharelink/internal/authutil"
	"notepad-sharelink/internal/handler"
	"notepad-sharelink/internal/middleware"
)

// New membuat *gin.Engine lengkap dengan seluruh route ter-registrasi.
func New(
	noteHandler *handler.NoteHandler,
	authHandler *handler.AuthHandler,
	jwtManager *authutil.JWTManager,
) *gin.Engine {
	r := gin.New() // pakai gin.New() bukan gin.Default() karena kita pasang logger sendiri

	// Structured logging untuk semua request
	r.Use(middleware.Logger())
	r.Use(gin.Recovery())

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

	// Rate limiter untuk endpoint auth (lebih ketat dari endpoint biasa)
	// 10 request per menit per IP — cukup longgar untuk pemakaian normal
	// tapi efektif menghambat brute force otomatis
	authLimiter, err := middleware.NewRateLimiter("10-M")
	if err != nil {
		log.Fatalf("gagal membuat auth rate limiter: %v", err)
	}

	auth := r.Group("/api/auth")
	auth.Use(authLimiter)
	{
		// Endpoint untuk web (cookie-based refresh token)
		auth.POST("/register", authHandler.Register)
		auth.POST("/login", authHandler.Login)
		auth.POST("/refresh", authHandler.Refresh) // baca refresh token dari cookie
		auth.POST("/logout", authHandler.Logout)   // baca refresh token dari cookie

		// Endpoint untuk Flutter mobile (body-based refresh token)
		auth.POST("/login-mobile", authHandler.MobileLoginHandler)
		auth.POST("/refresh-mobile", authHandler.MobileRefreshHandler)
		auth.POST("/logout-mobile", authHandler.MobileLogoutHandler)
	}

	notes := r.Group("/api/notes")
	{
		// Publik — akses via share link, tanpa perlu login
		notes.GET("/:id", noteHandler.Get)
		notes.POST("/:id/unlock", noteHandler.Unlock)

		// Protected — butuh JWT access token di header Authorization
		authed := notes.Group("")
		authed.Use(middleware.RequireAuth(jwtManager))
		{
			authed.GET("", noteHandler.List)
			authed.POST("", noteHandler.Create)
			authed.PUT("/:id", noteHandler.Update)
			authed.DELETE("/:id", noteHandler.Delete)
		}
	}

	// Catch-all: sajikan index.html untuk semua route yang tidak match
	r.NoRoute(func(c *gin.Context) {
		c.File("./frontend/index.html")
	})

	return r
}
