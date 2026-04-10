package db

import (
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
)

// ProvideDB opens a GORM connection and runs AutoMigrate.
func ProvideDB(databaseURL string, log *zap.Logger) *gorm.DB {
	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{
		Logger: logger.Discard,
	})
	if err != nil {
		log.Fatal("db: failed to connect", zap.Error(err))
	}

	// Run pgvector extension first (before AutoMigrate creates tables that use it)
	if err := setupPGVector(db, log); err != nil {
		log.Fatal("db: failed to setup pgvector", zap.Error(err))
	}

	// AutoMigrate all models
	if err := db.AutoMigrate(
		&model.User{},
		&model.Project{},
		&model.Conversation{},
		&model.ConversationMember{},
		&model.Agent{},
		&model.AgentRelationship{},
		&model.Message{},
		&model.UserUpdate{},
		&model.Settings{},
		&model.Checkpoint{},
	); err != nil {
		log.Fatal("db: AutoMigrate failed", zap.Error(err))
	}

	log.Info("db: connected and migrated")
	return db
}
