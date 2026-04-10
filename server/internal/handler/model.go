package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/PineappleBond/eino-demo-dev/server/internal/config"
	"github.com/PineappleBond/eino-demo-dev/server/internal/convert"
)

// RegisterModelRoutes registers the models info endpoint.
func RegisterModelRoutes(api *gin.RouterGroup, cfg *config.Config) {
	api.GET("/models", func(c *gin.Context) {
		models := make([]convert.ModelInfo, 0, len(cfg.Models))
		for tier, mc := range cfg.Models {
			models = append(models, convert.ModelInfo{
				Name:    tier,
				BaseURL: mc.BaseURL,
				Model:   mc.Model,
			})
		}

		respondJSON(c, http.StatusOK, convert.ModelsResponse{Models: models})
	})
}
