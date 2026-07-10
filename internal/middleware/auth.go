// Package middleware berisi middleware Gin, termasuk validasi JWT access token.
package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"notepad-sharelink/internal/authutil"
)

const contextUserIDKey = "user_id"

// RequireAuth memvalidasi JWT dari header "Authorization: Bearer <token>".
// Jika valid, user_id disimpan ke context untuk dipakai handler berikutnya.
func RequireAuth(jwtManager *authutil.JWTManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "token tidak ditemukan"})
			return
		}

		claims, err := jwtManager.Parse(parts[1])
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "token tidak valid atau sudah expired"})
			return
		}

		c.Set(contextUserIDKey, claims.UserID)
		c.Next()
	}
}

// UserID mengambil user_id yang sudah di-set oleh RequireAuth.
func UserID(c *gin.Context) string {
	return c.GetString(contextUserIDKey)
}
