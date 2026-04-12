package di

import (
	"context"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"gorm.io/gorm"

	_ "github.com/PineappleBond/eino-demo-dev/server/internal/templates"

	"github.com/PineappleBond/eino-demo-dev/server/internal/convert"
	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
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
		service.NewCronService,
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
	cronSvc *service.CronService,
) {
	r := handler.NewRouter(cfg, log)

	api := handler.GetAPI(r, db)
	handler.RegisterUserRoutes(api, userSvc, db, wsManager, log)
	handler.RegisterSettingsRoutes(api, settingsSvc, wsManager, log)
	handler.RegisterTemplateRoutes(api, tplSvc, wsManager, log)
	handler.RegisterProjectRoutes(api, projectSvc, wsManager, log)
	handler.RegisterConversationRoutes(api, convSvc, chatSvc, wsManager, log)
	handler.RegisterChatRoutes(api, chatSvc, wsManager)
	handler.RegisterTodoRoutes(api, todoSvc, wsManager, db)
	handler.RegisterCronTaskRoutes(api, cronSvc, wsManager, db)
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

			// Wire cron service: register message sender + start scheduler
			cronSvc.RegisterMessageSender(func(
				ctx context.Context,
				userID, conversationID uuid.UUID,
				content string,
				senderRole string,
			) (*service.SendMessageResponse, error) {
				// For tool-sent messages (cron tasks), create a tool-role message
				// which triggers the full AI agent response flow.
				if senderRole == "tool" {
					return chatSvc.CompleteToolMessage(
						ctx,
						userID, conversationID,
						content,
						wsManager.NextSeq,
						func(userID uuid.UUID, update model.UserUpdate) {
							wsUpdate := convert.ToUpdate(update)
							wsManager.PushToUserConnections(userID, wsUpdate)
						},
					)
				}
				return chatSvc.CompleteSendMessage(
					ctx,
					userID, conversationID,
					service.SendMessageRequest{Content: content},
					wsManager.NextSeq,
					func(userID uuid.UUID, update model.UserUpdate) {
						wsUpdate := convert.ToUpdate(update)
						wsManager.PushToUserConnections(userID, wsUpdate)
					},
				)
			})

			// Wire the register callback so the cron tool can register tasks
			// with the scheduler after creating them in the DB.
			tools.CronTaskRegisterFunc = cronSvc.RegisterTask

			// Wire the cron sync callback so cron_tool can push WS events
			// on create/cancel actions, so the frontend panel refreshes.
			tools.CronTaskSyncFunc = func(ctx context.Context, userID, conversationID uuid.UUID) {
				seq, err := wsManager.NextSeq(ctx, userID)
				if err != nil {
					return
				}
				payload := model.JSONMap{
					"conversation_id": conversationID.String(),
					"seq":             seq,
				}
				update := model.UserUpdate{
					UserID:  userID,
					Seq:     seq,
					Type:    "cron_task.sync",
					Payload: payload,
				}
				if err := db.WithContext(ctx).Create(&update).Error; err != nil {
					return
				}
				wsManager.PushToUserConnections(userID, convert.ToUpdate(update))
			}

			// Wire the todo sync callback so todo_write tool can push WS events
			// on create/update/delete actions, so the frontend panel refreshes.
			tools.TodoSyncFunc = func(ctx context.Context, userID, conversationID uuid.UUID) {
				seq, err := wsManager.NextSeq(ctx, userID)
				if err != nil {
					return
				}
				payload := model.JSONMap{
					"conversation_id": conversationID.String(),
					"seq":             seq,
				}
				update := model.UserUpdate{
					UserID:  userID,
					Seq:     seq,
					Type:    "todo.sync",
					Payload: payload,
				}
				if err := db.WithContext(ctx).Create(&update).Error; err != nil {
					return
				}
				wsManager.PushToUserConnections(userID, convert.ToUpdate(update))
			}

			// Wire the push callback so the cron service can push cron_task.sync
			// when a task fires, so the frontend panel refreshes.
			cronSvc.RegisterPushFunc(func(ctx context.Context, userID, conversationID uuid.UUID) {
				seq, err := wsManager.NextSeq(ctx, userID)
				if err != nil {
					return
				}
				payload := model.JSONMap{
					"conversation_id": conversationID.String(),
					"seq":             seq,
				}
				update := model.UserUpdate{
					UserID:  userID,
					Seq:     seq,
					Type:    "cron_task.sync",
					Payload: payload,
				}
				if err := db.WithContext(ctx).Create(&update).Error; err != nil {
					return
				}
				wsManager.PushToUserConnections(userID, convert.ToUpdate(update))
			})
			if err := cronSvc.Start(ctx); err != nil {
				log.Error("cron service start failed", zap.Error(err))
			}

			return nil
		},
		OnStop: func(ctx context.Context) error {
			cronSvc.Stop()
			return nil
		},
	})
}
