package router

import (
	"os"

	"notepad-sharelink/internal/authutil"
	"notepad-sharelink/internal/handler"
	"notepad-sharelink/internal/middleware"

	"notepad-sharelink/internal/db/sqlc"

	"github.com/gin-gonic/gin"
)

func New(
	noteHandler *handler.NoteHandler,
	authHandler *handler.AuthHandler,
	avatarHandler *handler.AvatarHandler,
	attachmentHandler *handler.AttachmentHandler,
	jwtManager *authutil.JWTManager,
	queries *sqlc.Queries,
) *gin.Engine {
	r := gin.New()

	r.Use(middleware.Logger())
	r.Use(gin.Recovery())
	r.Use(corsMiddleware())

	r.GET("/health", healthCheck)

	// Auth routes
	strictAuthLimiter, _ := middleware.NewRateLimiter("10-M")
	LooseAuthLimiter, _ := middleware.NewRateLimiter("60-M")
	meLimiter, _ := middleware.NewRateLimiter("50-M")
	resendLimiter, _ := middleware.NewRateLimiter("10-M")
	auth := r.Group("/api/auth")
	{
		loginGroup := auth.Group("")
		loginGroup.Use(strictAuthLimiter)
		loginGroup.POST("/register", authHandler.Register)
		loginGroup.POST("/login", authHandler.Login)
		loginGroup.POST("/refresh", authHandler.Refresh)

		looseGroup := auth.Group("")
		looseGroup.Use(LooseAuthLimiter)
		looseGroup.POST("/logout", authHandler.Logout)
		looseGroup.GET("/verify-email", authHandler.VerifyEmail)
		looseGroup.POST("/login-mobile", authHandler.MobileLoginHandler)
		looseGroup.POST("/refresh-mobile", authHandler.MobileRefreshHandler)
		looseGroup.POST("/logout-mobile", authHandler.MobileLogoutHandler)
		looseGroup.GET("/google/login", authHandler.GoogleLoginRedirect)
		looseGroup.GET("/google/callback", authHandler.GoogleCallback)
		looseGroup.GET("/verify-merge-password", authHandler.VerifyMergePassword)

		authedAuth := auth.Group("")
		authedAuth.Use(middleware.RequireAuth(jwtManager))
		meGroup := authedAuth.Group("")
		meGroup.Use(meLimiter)
		resendGroup := authedAuth.Group("")
		resendGroup.Use(resendLimiter)
		{
			meGroup.GET("/me", authHandler.Me)
			resendGroup.POST("/resend-verification", authHandler.ResendVerification)
		}
	}

	// Notes routes
	notes := r.Group("/api/notes")
	{
		notes.GET("/:id", noteHandler.Get)
		notes.POST("/:id/unlock", noteHandler.Unlock)
		notes.GET("/:id/attachments", attachmentHandler.List)
		notes.POST("/attachments/:attachmentId/download", attachmentHandler.DownloadPrivate)

		authed := notes.Group("")
		authed.Use(middleware.RequireAuth(jwtManager))
		authed.Use(middleware.RequireVerified(queries))
		{
			authed.GET("", noteHandler.List)
			authed.POST("", noteHandler.Create)
			authed.PUT("/:id", noteHandler.Update)
			authed.DELETE("/:id", noteHandler.Delete)

			authed.POST("/:id/attachments/presign", attachmentHandler.PresignUpload)
			authed.POST("/:id/attachments/confirm", attachmentHandler.ConfirmUpload)
			authed.POST("/:id/attachments/private", attachmentHandler.UploadPrivate)
			authed.DELETE("/attachments/:attachmentId", attachmentHandler.Delete)
		}
	}

	// Avatar routes (butuh login, tidak perlu verified)
	users := r.Group("/api/users")
	users.Use(middleware.RequireAuth(jwtManager))
	{
		users.POST("/avatar/presign", avatarHandler.PresignUpload)
		users.POST("/avatar/confirm", avatarHandler.ConfirmUpload)
	}

	r.NoRoute(func(c *gin.Context) {
		c.JSON(404, gin.H{"error": "Not found"})
	})

	return r
}

// CORS middleware
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		if os.Getenv("APP_ENV") == "development" {
			if origin != "" {
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
			return
		}

		// PRODUCTION: Allowlist
		allowed := map[string]bool{
			"https://pratama20747.github.io": true,
			"https://binery.my.id":           true,
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
