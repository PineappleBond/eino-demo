package di

import (
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/PineappleBond/eino-demo-dev/server/internal/config"
	"github.com/PineappleBond/eino-demo-dev/server/internal/db"
)

// Module wires all dependencies for the application.
var Module = fx.Options(
	fx.Provide(
		config.Load,
		ProvideLogger,
		db.ProvideDB,
		ProvideRedis,
	),
	// Handler modules — register Gin routes
	fx.Invoke(RegisterRoutes),
)

// ProvideRedis creates a Redis client.
func ProvideRedis(cfg *config.Config) *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr: cfg.RedisAddr,
	})
	return rdb
}

// RegisterRoutes sets up all Gin routes.
func RegisterRoutes(
	lc fx.Lifecycle,
	cfg *config.Config,
	log *zap.Logger,
	db *gorm.DB,
	rdb *redis.Client,
) {
	// TODO: will be filled in later tasks
	log.Info("routes registered (stub)")
}
