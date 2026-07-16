// Package middleware — RequireVerified memastikan user sudah verifikasi email
// sebelum boleh mengakses endpoint tertentu (mis. create/update/delete note).
package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"notepad-sharelink/internal/db/sqlc"
)

// RequireVerified harus dipasang SETELAH RequireAuth (butuh user_id di context).
func RequireVerified(q *sqlc.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := UserID(c)

		u, err := q.GetUserByID(c.Request.Context(), userID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}

		if !u.EmailVerified {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "email belum diverifikasi, cek inbox kamu"})
			return
		}

		c.Next()
	}
}
