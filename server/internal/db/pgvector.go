package db

import (
	"gorm.io/gorm"
	"go.uber.org/zap"
)

// setupPGVector creates the vector extension if it doesn't exist.
// GORM cannot create extensions, so this runs raw SQL before AutoMigrate.
// If the extension is not available (e.g. Alpine image without pgvector), log a warning and continue.
func setupPGVector(db *gorm.DB, log *zap.Logger) error {
	err := db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error
	if err != nil {
		log.Warn("db: pgvector extension not available, skipping", zap.Error(err))
		return nil // Non-fatal for Phase 1
	}
	return nil
}
