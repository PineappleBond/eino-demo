package db

import "gorm.io/gorm"

// setupPGVector creates the vector extension if it doesn't exist.
// GORM cannot create extensions, so this runs raw SQL before AutoMigrate.
func setupPGVector(db *gorm.DB) error {
	return db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error
}
