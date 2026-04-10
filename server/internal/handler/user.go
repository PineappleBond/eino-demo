package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
	"github.com/PineappleBond/eino-demo-dev/server/internal/service"
	"github.com/PineappleBond/eino-demo-dev/server/internal/ws"
)

// RegisterUserRoutes registers GET /users/me and GET /users/me/updates.
func RegisterUserRoutes(api *gin.RouterGroup, svc *service.UserService, db *gorm.DB, rdb *redis.Client, wsManager *ws.Manager, log *zap.Logger) {
	api.GET("/users/me", func(c *gin.Context) {
		userID := getUserID(c)
		user, settings, err := svc.GetMe(userID)
		if err != nil {
			respondError(c, 500, "INTERNAL_ERROR", "failed to fetch user")
			return
		}
		respondJSON(c, 200, gin.H{
			"id":         user.ID,
			"name":       user.Name,
			"created_at": user.CreatedAt,
			"settings": gin.H{
				"model_tier": settings.ModelTier,
				"locale":     settings.Locale,
				"theme":      settings.Theme,
				"updated_at": settings.UpdatedAt,
			},
		})
	})

	// Offline polling endpoint: GET /api/v1/users/me/updates?last_seq=N
	// Returns updates with seq > last_seq for the authenticated user.
	api.GET("/users/me/updates", func(c *gin.Context) {
		userID := getUserID(c)

		lastSeqStr := c.DefaultQuery("last_seq", "0")
		lastSeq, err := strconv.ParseInt(lastSeqStr, 10, 64)
		if err != nil {
			respondError(c, 400, "INVALID_PARAM", "last_seq must be an integer")
			return
		}

		var updates []model.UserUpdate
		if err := db.Where("user_id = ? AND seq > ?", userID, lastSeq).
			Order("seq ASC").
			Limit(100).
			Find(&updates).Error; err != nil {
			log.Error("failed to fetch updates", zap.Error(err))
			respondError(c, 500, "INTERNAL_ERROR", "failed to fetch updates")
			return
		}

		// Get current max seq from Redis
		ctx := c.Request.Context()
		maxSeq, _ := rdb.Get(ctx, "seq:"+userID.String()).Int64()

		frames := make([]ws.Update, len(updates))
		for i, u := range updates {
			frames[i] = ws.Update{
				Seq:     u.Seq,
				Type:    u.Type,
				Payload: u.Payload,
			}
		}

		respondJSON(c, 200, gin.H{
			"updates": frames,
			"max_seq": maxSeq,
			"has_more": len(updates) == 100,
		})
	})
}
