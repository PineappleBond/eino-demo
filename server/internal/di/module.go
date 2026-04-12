package di

import (
	"context"

	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"gorm.io/gorm"

	// Import templates package to trigger init() registration.
	_ "github.com/PineappleBond/eino-demo-dev/server/internal/templates"

	"github.com/PineappleBond/eino-demo-dev/server/internal/ws"

	"github.com/PineappleBond/eino-demo-dev/server/internal/config"
	"github.com/PineappleBond/eino-demo-dev/server/internal/db"
	"github.com/PineappleBond/eino-demo-dev/server/internal/eino"
	"github.com/PineappleBond/eino-demo-dev/server/internal/eino/runner"
	"github.com/PineappleBond/eino-demo-dev/server/internal/eino/tools"
	"github.com/PineappleBond/eino-demo-dev/server/internal/handler"
	"github.com/PineappleBond/eino-demo-dev/server/internal/service"
)

// Module wires all dependencies for the application.
var Module = fx.Options(
	fx.Provide(
		config.Load,
		func(cfg *config.Config) string { return cfg.DatabaseURL },
		ProvideLogger,
		db.ProvideDB,
		ProvideRedis,
		ws.NewManager,
		eino.NewModelProvider,
		tools.NewToolRegistry,
		runner.NewRunSessionManager,
		runner.NewMessageQueue,
		service.NewCompressionService,
		service.NewUserService,
		service.NewSettingsService,
		service.NewTemplateService,
		service.NewProjectService,
		service.NewConversationService,
		service.NewChatService,
		service.NewTodoService,
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
	wsManager *ws.Manager,
	userSvc *service.UserService,
	settingsSvc *service.SettingsService,
	tplSvc *service.TemplateService,
	projectSvc *service.ProjectService,
	convSvc *service.ConversationService,
	chatSvc *service.ChatService,
	todoSvc *service.TodoService,
) {
	r := handler.NewRouter(cfg, log)

	api := handler.GetAPI(r, db)
	handler.RegisterUserRoutes(api, userSvc, db, wsManager, log)
	handler.RegisterSettingsRoutes(api, settingsSvc, wsManager)
	handler.RegisterTemplateRoutes(api, tplSvc, wsManager)
	handler.RegisterProjectRoutes(api, projectSvc, wsManager)
	handler.RegisterConversationRoutes(api, convSvc, chatSvc, wsManager)
	handler.RegisterChatRoutes(api, chatSvc, wsManager)
	handler.RegisterTodoRoutes(api, todoSvc, wsManager, db)
	handler.RegisterModelRoutes(api, cfg)

	// WebSocket upgrade endpoint (not under /api/v1)
	ws.RegisterWSRoutes(r, wsManager, db, log)

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
