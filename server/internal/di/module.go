package di

import (
	"context"

	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"gorm.io/gorm"

	// Import templates package to trigger init() registration.
	_ "github.com/PineappleBond/eino-demo-dev/server/internal/templates"

	"github.com/PineappleBond/eino-demo-dev/server/internal/config"
	"github.com/PineappleBond/eino-demo-dev/server/internal/db"
	"github.com/PineappleBond/eino-demo-dev/server/internal/handler"
	"github.com/PineappleBond/eino-demo-dev/server/internal/service"
)

// Module wires all dependencies for the application.
var Module = fx.Options(
	fx.Provide(
		config.Load,
		ProvideLogger,
		db.ProvideDB,
		ProvideRedis,
		service.NewUserService,
		service.NewSettingsService,
		service.NewTemplateService,
		service.NewProjectService,
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

// RegisterRoutes sets up all Gin routes and starts the server.
func RegisterRoutes(
	lc fx.Lifecycle,
	cfg *config.Config,
	log *zap.Logger,
	db *gorm.DB,
	rdb *redis.Client,
	userSvc *service.UserService,
	settingsSvc *service.SettingsService,
	tplSvc *service.TemplateService,
	projectSvc *service.ProjectService,
) {
	r := handler.NewRouter(cfg, log, db)

	api := handler.GetAPI(r)
	handler.RegisterUserRoutes(api, userSvc)
	handler.RegisterSettingsRoutes(api, settingsSvc)
	handler.RegisterTemplateRoutes(api, tplSvc)
	handler.RegisterProjectRoutes(api, projectSvc)

	_ = rdb // Will be used in later tasks (WebSocket, seq queue)

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			addr := ":" + cfg.ServerPort
			log.Info("server starting", zap.String("addr", addr))
			go func() {
				if err := r.Run(addr); err != nil {
					log.Error("server error", zap.Error(err))
				}
			}()
			return nil
		},
	})
}
