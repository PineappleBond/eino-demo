package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/PineappleBond/eino-demo-dev/server/internal/convert"
	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
	"github.com/PineappleBond/eino-demo-dev/server/internal/service"
	"github.com/PineappleBond/eino-demo-dev/server/internal/types"
	"github.com/PineappleBond/eino-demo-dev/server/internal/ws"
)

// RegisterUserRoutes registers GET /users/me and GET /users/me/updates.
func RegisterUserRoutes(api *gin.RouterGroup, svc *service.UserService, db *gorm.DB, wsManager *ws.Manager, log *zap.Logger) {
	api.GET("/users/me", func(c *gin.Context) {
		userID := getUserID(c)
		user, settings, err := svc.GetMe(userID)
		if err != nil {
			log.Error("get me failed",
				zap.String("user_id", userID.String()),
				zap.Error(err),
			)
			respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to fetch user")
			return
		}
		createdAt := user.CreatedAt
		resp := convert.MeResponse{
			ID:       user.ID,
			Settings: convert.ToSettings(*settings),
		}
		if user.Name != "" {
			resp.Name = &user.Name
		}
		resp.CreatedAt = &createdAt
		respondJSON(c, http.StatusOK, resp)
	})

	// Offline polling endpoint: GET /api/v1/users/me/updates?last_seq=N
	// Returns { updates: [...], max_seq, has_more } — client passes updates to applyUpdates().
	api.GET("/users/me/updates", func(c *gin.Context) {
		userID := getUserID(c)

		lastSeqStr := c.DefaultQuery("last_seq", "0")
		lastSeq, err := strconv.ParseInt(lastSeqStr, 10, 64)
		if err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "last_seq must be an integer")
			return
		}

		var updates []model.UserUpdate
		log.Debug("fetch updates",
			zap.String("user_id", userID.String()),
			zap.Int64("last_seq", lastSeq),
		)
		if err := db.Where("user_id = ? AND seq > ?", userID, lastSeq).
			Order("seq ASC").
			Limit(100).
			Find(&updates).Error; err != nil {
			log.Error("failed to fetch updates", zap.Error(err))
			respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to fetch updates")
			return
		}

		// Fill seq gaps with empty updates so the client's continuity check always passes.
		// Redis INCR is not rolled back on DB write failure, so gaps can exist.
		result := make([]types.Update, 0, len(updates))
		expectedSeq := lastSeq + 1
		for _, u := range updates {
			for u.Seq > expectedSeq {
				result = append(result, types.Update{
					Seq:     expectedSeq,
					Type:    types.Empty,
					Payload: convert.ToUpdatePayload(model.JSONMap{}),
				})
				expectedSeq++
			}
			result = append(result, convert.ToUpdate(u))
			expectedSeq = u.Seq + 1
		}

		var maxSeq int64
		if len(result) > 0 {
			maxSeq = result[len(result)-1].Seq
		}

		c.JSON(http.StatusOK, convert.UpdatesResponse{
			Updates: result,
			MaxSeq:  maxSeq,
			HasMore: false,
		})
	})
}
