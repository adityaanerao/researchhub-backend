package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"researchhub-backend/config"
	"researchhub-backend/models"
	"researchhub-backend/utils"

	"github.com/gin-gonic/gin"
)

// CORSMiddleware configures strict CORS headers for the Angular frontend
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		allowed := "http://localhost:4200"
		if config.Cfg != nil && config.Cfg.CORSAllowedOrigins != "" {
			allowed = config.Cfg.CORSAllowedOrigins
		}
		if origin != "" && (strings.Contains(allowed, origin) || allowed == "*") {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		} else {
			c.Writer.Header().Set("Access-Control-Allow-Origin", "http://localhost:4200")
		}
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, PATCH, DELETE")
		c.Writer.Header().Set("Access-Control-Expose-Headers", "Content-Disposition")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// AuthRequired verifies the JWT token and ensures the user account is still active
func AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		tokenString := ""
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenString = strings.TrimPrefix(authHeader, "Bearer ")
		} else if qToken := c.Query("token"); qToken != "" {
			tokenString = qToken
		}

		if tokenString == "" {
			utils.RespondError(c, http.StatusUnauthorized, "Authentication required: missing access token", nil)
			c.Abort()
			return
		}

		claims, err := utils.ValidateJWT(tokenString)
		if err != nil {
			utils.RespondError(c, http.StatusUnauthorized, "Invalid or expired access token", err.Error())
			c.Abort()
			return
		}

		// Verify user still exists and is active in database
		if config.DB != nil {
			var user models.User
			if err := config.DB.Select("id, email, full_name, role_name, is_active").Where("id = ?", claims.UserID).First(&user).Error; err != nil {
				utils.RespondError(c, http.StatusUnauthorized, "User account no longer exists", nil)
				c.Abort()
				return
			}
			if !user.IsActive {
				utils.RespondError(c, http.StatusForbidden, "Your account has been deactivated. Please contact the administrator.", nil)
				c.Abort()
				return
			}
			c.Set("user_id", user.ID)
			c.Set("user_email", user.Email)
			c.Set("user_name", user.FullName)
			c.Set("user_role", user.RoleName)
		} else {
			c.Set("user_id", claims.UserID)
			c.Set("user_email", claims.Email)
			c.Set("user_name", claims.FullName)
			c.Set("user_role", claims.Role)
		}

		c.Next()
	}
}

// RequireRoles enforces Role-Based Access Control (RBAC) on backend endpoints
func RequireRoles(allowedRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		roleVal, exists := c.Get("user_role")
		if !exists {
			utils.RespondError(c, http.StatusUnauthorized, "Unauthorized role context", nil)
			c.Abort()
			return
		}
		userRole := roleVal.(string)
		for _, r := range allowedRoles {
			if userRole == r {
				c.Next()
				return
			}
		}
		utils.RespondError(c, http.StatusForbidden, "Access denied: insufficient role permissions", map[string]string{
			"required_roles": strings.Join(allowedRoles, ", "),
			"current_role":   userRole,
		})
		c.Abort()
	}
}

type rateVisitor struct {
	count    int
	windowAt time.Time
}

var (
	visitors = make(map[string]*rateVisitor)
	mu       sync.Mutex
)

// AuthRateLimiter protects authentication routes from brute-force attempts
func AuthRateLimiter(maxRequests int, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		mu.Lock()
		v, exists := visitors[ip]
		now := time.Now()
		if !exists || now.Sub(v.windowAt) > window {
			visitors[ip] = &rateVisitor{count: 1, windowAt: now}
			mu.Unlock()
			c.Next()
			return
		}
		if v.count >= maxRequests {
			mu.Unlock()
			utils.RespondError(c, http.StatusTooManyRequests, "Too many authentication attempts. Please try again shortly.", nil)
			c.Abort()
			return
		}
		v.count++
		mu.Unlock()
		c.Next()
	}
}
