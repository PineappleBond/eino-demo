package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/PineappleBond/eino-demo-dev/server/internal/config"
)

// RegisterModelRoutes registers the models info endpoint.
func RegisterModelRoutes(api *gin.RouterGroup, cfg *config.Config) {
	api.GET("/models", func(c *gin.Context) {
		type ModelInfo struct {
			Name    string `json:"name"`
			BaseURL string `json:"base_url"`
			Model   string `json:"model"`
		}

		models := make([]ModelInfo, 0, len(cfg.Models))
		for tier, mc := range cfg.Models {
			models = append(models, ModelInfo{
				Name:    tier,
				BaseURL: mc.BaseURL,
				Model:   mc.Model,
			})
		}

		respondJSON(c, http.StatusOK, gin.H{"models": models})
	})
}
