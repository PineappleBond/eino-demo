package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/PineappleBond/eino-demo-dev/server/internal/config"
	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
)

// NewRouter creates and configures the Gin engine with all routes.
func NewRouter(
	cfg *config.Config,
	log *zap.Logger,
	db *gorm.DB,
) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(requestLogger(log))

	api := r.Group("/api/v1")
	api.Use(authMiddleware(db))

	// Routes will be registered by individual handlers here.
	// For now, register a health check.
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	_ = api // api group will be used by route registration functions

	return r
}

// authMiddleware extracts user_id from Bearer token via FirstOrCreate.
func authMiddleware(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		token := strings.TrimPrefix(auth, "Bearer ")
		if token == "" || token == auth {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{"code": "UNAUTHORIZED", "message": "missing or empty Authorization header"},
			})
			return
		}

		// Demo mode: derive a deterministic UUID from the token.
		userID := uuid.NewSHA1(uuid.Nil, []byte(token))
		var user model.User
		result := db.Where("id = ?", userID).First(&user)
		if result.Error != nil {
			if result.Error == gorm.ErrRecordNotFound {
				user = model.User{Name: ""}
				if err := db.FirstOrCreate(&user, model.User{BaseModel: model.BaseModel{ID: userID}}).Error; err != nil {
					c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
						"error": gin.H{"code": "INTERNAL_ERROR", "message": "database error"},
					})
					return
				}
			} else {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": gin.H{"code": "INTERNAL_ERROR", "message": "database error"},
				})
				return
			}
		}

		c.Set("user_id", user.ID)
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

// respondJSON sends a JSON response.
func respondJSON(c *gin.Context, status int, data any) {
	c.JSON(status, data)
}

// respondError sends a uniform error response.
func respondError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{"code": code, "message": message},
	})
}
