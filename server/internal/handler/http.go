package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/PineappleBond/eino-demo-dev/server/internal/auth"
	"github.com/PineappleBond/eino-demo-dev/server/internal/config"
	"github.com/PineappleBond/eino-demo-dev/server/internal/types"
)

// Setup holds all dependencies and provides route registration.
type Setup struct {
	Cfg *config.Config
	Log *zap.Logger
	DB  *gorm.DB
	API *gin.RouterGroup
}

// corsMiddleware allows cross-origin requests for local development.
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Header("Access-Control-Max-Age", "86400")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// NewRouter creates and configures the Gin engine with all routes.
func NewRouter(
	cfg *config.Config,
	log *zap.Logger,
) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(zapRecovery(log))
	r.Use(requestLogger(log))
	r.Use(corsMiddleware())

	// Health check (no auth).
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	return r
}

// GetAPI returns the authenticated API group for route registration.
func GetAPI(r *gin.Engine, db *gorm.DB) *gin.RouterGroup {
	api := r.Group("/api/v1")
	api.Use(authMiddleware(db))
	return api
}

// authMiddleware extracts user_id from Bearer token via FirstOrCreate.
func authMiddleware(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		token := strings.TrimPrefix(authHeader, "Bearer ")
		userID, err := auth.ResolveTokenToUser(db, token)
		if err != nil {
			respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid Authorization header")
			c.Abort()
			return
		}

		c.Set("user_id", userID)
		c.Next()
	}
}

// getUserID extracts the authenticated user's UUID from context.
func getUserID(c *gin.Context) uuid.UUID {
	id, _ := c.Get("user_id")
	return id.(uuid.UUID)
}

// requestLogger logs each request's method, path, and status.
func requestLogger(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		log.Info("request",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
		)
	}
}

// zapRecovery returns a Gin recovery middleware that logs panics via zap.
func zapRecovery(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Error("panic recovered",
					zap.Any("recover", r),
					zap.String("method", c.Request.Method),
					zap.String("path", c.Request.URL.Path),
				)
				respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "an internal error occurred")
				c.Abort()
			}
		}()
		c.Next()
	}
}

// respondJSON sends a JSON response.
func respondJSON(c *gin.Context, status int, data any) {
	c.JSON(status, data)
}

// respondError sends a uniform error response using the OpenAPI ErrorResponse type.
func respondError(c *gin.Context, status int, code, message string) {
	c.JSON(status, types.ErrorResponse{
		Error: struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}{
			Code:    code,
			Message: message,
		},
	})
}
