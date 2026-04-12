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

	// Before AutoMigrate: alter checkpoints.message_id to nullable.
	// The ADK CheckPointStore.Set callback doesn't receive message info, so new
	// checkpoints have a nil MessageID. AutoMigrate won't drop NOT NULL on
	// existing columns, so we do it manually here.
	_ = db.Exec("ALTER TABLE checkpoints ALTER COLUMN message_id DROP NOT NULL").Error
	// Also change the FK from CASCADE to SET NULL (ignore "already exists" errors).
	_ = db.Exec("ALTER TABLE checkpoints DROP CONSTRAINT IF EXISTS fk_checkpoints_message_id").Error
	_ = db.Exec("ALTER TABLE checkpoints ADD CONSTRAINT fk_checkpoints_message_id FOREIGN KEY (message_id) REFERENCES messages(id) ON DELETE SET NULL").Error

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
		&model.HumanInTheLoop{},
		&model.Todo{},
		&model.CronTask{},
		&model.ProjectToolPermission{},
		&model.HumanInPermission{},
	); err != nil {
		log.Fatal("db: AutoMigrate failed", zap.Error(err))
	}

	// GORM does not create FK constraints for plain UUID fields (only for associations).
	// Add them explicitly via raw SQL.
	if err := setupForeignKeys(db, log); err != nil {
		log.Fatal("db: failed to setup foreign keys", zap.Error(err))
	}

	// Add CHECK constraints for enum columns.
	if err := setupCheckConstraints(db, log); err != nil {
		log.Fatal("db: failed to setup check constraints", zap.Error(err))
	}

	log.Info("db: connected and migrated")
	return db
}

func setupForeignKeys(db *gorm.DB, log *zap.Logger) error {
	fkStatements := []string{
		`ALTER TABLE projects ADD CONSTRAINT fk_projects_user_id
			 FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE`,
		`ALTER TABLE conversations ADD CONSTRAINT fk_conversations_project_id
			 FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE`,
		`ALTER TABLE conversations ADD CONSTRAINT fk_conversations_user_id
			 FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE`,
		`ALTER TABLE conversations ADD CONSTRAINT fk_conversations_parent
			 FOREIGN KEY (parent_conversation_id) REFERENCES conversations(id) ON DELETE SET NULL`,
		`ALTER TABLE conversation_members ADD CONSTRAINT fk_conv_members_conversation
			 FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE`,
		`ALTER TABLE agents ADD CONSTRAINT fk_agents_project_id
			 FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE`,
		`ALTER TABLE agent_relationships ADD CONSTRAINT fk_agent_rel_project
			 FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE`,
		`ALTER TABLE agent_relationships ADD CONSTRAINT fk_agent_rel_parent
			 FOREIGN KEY (parent_id) REFERENCES agents(id) ON DELETE CASCADE`,
		`ALTER TABLE agent_relationships ADD CONSTRAINT fk_agent_rel_child
			 FOREIGN KEY (child_id) REFERENCES agents(id) ON DELETE CASCADE`,
		`ALTER TABLE messages ADD CONSTRAINT fk_messages_conversation_id
			 FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE`,
		`ALTER TABLE user_updates ADD CONSTRAINT fk_user_updates_user_id
			 FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE`,
		`ALTER TABLE checkpoints ADD CONSTRAINT fk_checkpoints_conversation_id
			 FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE`,
		`ALTER TABLE checkpoints ADD CONSTRAINT fk_checkpoints_message_id
			 FOREIGN KEY (message_id) REFERENCES messages(id) ON DELETE SET NULL`,
		`ALTER TABLE human_in_the_loops ADD CONSTRAINT fk_hitl_conversation_id
			 FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE`,
		`ALTER TABLE todos ADD CONSTRAINT fk_todos_conversation_id
			 FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE`,
		`ALTER TABLE cron_tasks ADD CONSTRAINT fk_cron_tasks_conversation_id
			 FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE`,
		`ALTER TABLE human_in_permissions ADD CONSTRAINT fk_hip_conversation_id
			 FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE`,
		`ALTER TABLE project_tool_permissions ADD CONSTRAINT fk_ptp_project_id
			 FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE`,
		`ALTER TABLE settings ADD CONSTRAINT fk_settings_user_id
			 FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE`,
	}

	for _, stmt := range fkStatements {
		if err := db.Exec(stmt).Error; err != nil {
			// Ignore "already exists" errors — this is idempotent.
			log.Debug("db: fk constraint (may already exist)", zap.Error(err))
		}
	}
	return nil
}

func setupCheckConstraints(db *gorm.DB, log *zap.Logger) error {
	constraints := []string{
		`ALTER TABLE conversations ADD CONSTRAINT chk_conv_status
			 CHECK (status IN ('active','compacting','compacted','archived'))`,
		`ALTER TABLE messages ADD CONSTRAINT chk_msg_sender_role
			 CHECK (sender_role IN ('user','assistant','system','tool'))`,
		`ALTER TABLE messages ADD CONSTRAINT chk_msg_finish_reason
			 CHECK (finish_reason IS NULL OR finish_reason IN ('stop','length','error','tool_calls'))`,
		`ALTER TABLE settings ADD CONSTRAINT chk_settings_model_tier
			 CHECK (model_tier IN ('haiku','sonnet','opus'))`,
		`ALTER TABLE settings ADD CONSTRAINT chk_settings_locale
			 CHECK (locale IN ('en','zh'))`,
		`ALTER TABLE settings ADD CONSTRAINT chk_settings_theme
			 CHECK (theme IN ('light','dark'))`,
		`ALTER TABLE conversation_members ADD CONSTRAINT chk_conv_member_type
			 CHECK (member_type IN ('user','agent'))`,
		`ALTER TABLE agent_relationships ADD CONSTRAINT chk_agent_rel_relationship
			 CHECK (relationship != '')`,
		`ALTER TABLE human_in_the_loops ADD CONSTRAINT chk_hitl_answer_type
			 CHECK (answer_type IN ('single','multi','text'))`,
		`ALTER TABLE human_in_the_loops ADD CONSTRAINT chk_hitl_status
			 CHECK (status IN ('pending','answered','expired'))`,
		`ALTER TABLE cron_tasks ADD CONSTRAINT chk_cron_task_status
			 CHECK (status IN ('pending','active','cancelled','completed','failed'))`,
	}

	for _, c := range constraints {
		if err := db.Exec(c).Error; err != nil {
			// Ignore "already exists" errors — this is idempotent.
			log.Debug("db: check constraint (may already exist)", zap.Error(err))
		}
	}
	return nil
}
