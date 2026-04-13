package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/PineappleBond/eino-demo-dev/server/internal/types"
	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
)

// MessageSender sends a message to a conversation, returning the result.
// This avoids circular dependencies with ChatService.
type MessageSender func(
	ctx context.Context,
	userID, conversationID uuid.UUID,
	content string,
	senderRole string,
) (*SendMessageResponse, error)

// CronService manages scheduled tasks that trigger messages in conversations.
type CronService struct {
	db            *gorm.DB
	log           *zap.Logger
	scheduler     *cron.Cron
	taskEntries   map[uuid.UUID]cron.EntryID
	messageSender MessageSender
	pushSync      func(ctx context.Context, userID, conversationID uuid.UUID)
}

// NewCronService creates a CronService.
func NewCronService(db *gorm.DB, log *zap.Logger) *CronService {
	s := cron.New(cron.WithSeconds())
	cs := &CronService{
		db:          db,
		log:         log,
		scheduler:   s,
		taskEntries: make(map[uuid.UUID]cron.EntryID),
	}
	return cs
}

// RegisterMessageSender sets the function used to send messages when tasks fire.
func (s *CronService) RegisterMessageSender(sender MessageSender) {
	s.messageSender = sender
}

// Start loads existing pending tasks from DB into the scheduler and starts it.
func (s *CronService) Start(ctx context.Context) error {
	var tasks []model.CronTask
	if err := s.db.WithContext(ctx).
		Where("status = ?", "active").
		Find(&tasks).Error; err != nil {
		return fmt.Errorf("failed to load cron tasks: %w", err)
	}

	for _, task := range tasks {
		if err := s.registerTask(task); err != nil {
			s.log.Error("CronService: failed to register task",
				zap.String("task_id", task.ID.String()),
				zap.Error(err),
			)
		}
	}

	s.scheduler.Start()
	s.log.Info("CronService: scheduler started", zap.Int("loaded_tasks", len(tasks)))
	return nil
}

// Stop gracefully stops the scheduler.
func (s *CronService) Stop() {
	s.scheduler.Stop()
	s.log.Info("CronService: scheduler stopped")
}

// CreateTask creates a new scheduled task in DB and registers it with the scheduler.
func (s *CronService) CreateTask(ctx context.Context, userID, conversationID uuid.UUID, content, schedule string, senderRole types.CronTaskSenderRole) (*model.CronTask, error) {
	var conv model.Conversation
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return nil, fmt.Errorf("get conversation: %w", ErrConversationNotFound)
	}

	sched, nextRun, err := parseSchedule(schedule)
	if err != nil {
		return nil, fmt.Errorf("invalid schedule: %w", err)
	}

	task := model.CronTask{
		ConversationID: conversationID,
		Content:        content,
		SenderRole:     string(senderRole),
		Schedule:       sched,
		NextRunAt:      nextRun,
		Status:         "active",
	}
	if err := s.db.WithContext(ctx).Create(&task).Error; err != nil {
		return nil, fmt.Errorf("failed to create cron task: %w", err)
	}

	if err := s.registerTask(task); err != nil {
		s.db.WithContext(ctx).Model(&task).Update("status", "cancelled")
		s.log.Error("CreateTask: failed to register with scheduler",
			zap.String("task_id", task.ID.String()),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to register task with scheduler: %w", err)
	}

	return &task, nil
}

// ListTasks returns cron tasks for a conversation, optionally filtered by status.
func (s *CronService) ListTasks(ctx context.Context, userID, conversationID uuid.UUID, status string) ([]model.CronTask, error) {
	var conv model.Conversation
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return nil, fmt.Errorf("get conversation: %w", ErrConversationNotFound)
	}

	q := s.db.WithContext(ctx).Where("conversation_id = ?", conversationID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var tasks []model.CronTask
	if err := q.Order("next_run_at ASC").Find(&tasks).Error; err != nil {
		return nil, fmt.Errorf("failed to list cron tasks: %w", err)
	}
	return tasks, nil
}

// CancelTask cancels a scheduled task.
func (s *CronService) CancelTask(ctx context.Context, userID, conversationID, taskID uuid.UUID) error {
	var conv model.Conversation
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		return fmt.Errorf("get conversation: %w", ErrConversationNotFound)
	}

	var task model.CronTask
	if err := s.db.WithContext(ctx).Where("id = ? AND conversation_id = ?", taskID, conversationID).First(&task).Error; err != nil {
		return fmt.Errorf("cron task not found")
	}

	if entryID, ok := s.taskEntries[task.ID]; ok {
		s.scheduler.Remove(entryID)
		delete(s.taskEntries, task.ID)
	}

	if err := s.db.WithContext(ctx).Model(&task).Update("status", "cancelled").Error; err != nil {
		return fmt.Errorf("failed to cancel cron task: %w", err)
	}

	return nil
}

// ExecuteTask sends the scheduled message when a cron task fires.
func (s *CronService) ExecuteTask(ctx context.Context, taskID uuid.UUID) error {
	var task model.CronTask
	if err := s.db.WithContext(ctx).Where("id = ? AND status = ?", taskID, "active").First(&task).Error; err != nil {
		return fmt.Errorf("cron task not found or not active")
	}

	if s.messageSender == nil {
		s.log.Error("ExecuteTask: message sender not registered")
		return fmt.Errorf("message sender not configured")
	}

	var conv model.Conversation
	if err := s.db.WithContext(ctx).Where("id = ?", task.ConversationID).First(&conv).Error; err != nil {
		return fmt.Errorf("conversation not found for task")
	}

	resp, err := s.messageSender(ctx, conv.UserID, task.ConversationID, task.Content, task.SenderRole)
	if err != nil {
		s.log.Error("ExecuteTask: failed to send message",
			zap.String("task_id", task.ID.String()),
			zap.Error(err),
		)
		return fmt.Errorf("failed to send message: %w", err)
	}

	s.log.Info("ExecuteTask: message sent",
		zap.String("task_id", task.ID.String()),
		zap.Int64("seq", resp.Seq),
	)

	// For one-time tasks (6-field schedule that only fires once), mark as completed.
	// For recurring tasks (cron expression or @-descriptor), the task stays active.
	if s.isOneTimeTask(task.Schedule) {
		if err := s.db.WithContext(ctx).Model(&task).Update("status", "completed").Error; err != nil {
			s.log.Warn("ExecuteTask: failed to mark task as completed", zap.Error(err))
		}
	}

	// Push cron_task.sync to notify the frontend.
	s.pushTaskSync(ctx, conv.UserID, task.ConversationID)

	return nil
}

// isOneTimeTask checks if a schedule is a one-time 6-field cron expression.
// One-time tasks are stored as "SEC MIN HOUR DOM MONTH *" — 6 fields with
// all numeric values and "*" as day-of-week.
func (s *CronService) isOneTimeTask(schedule string) bool {
	fields := strings.Fields(schedule)
	if len(fields) != 6 {
		return false
	}
	// All fields must be pure numbers except DOW which must be "*"
	return isSpecificField(fields[0]) && isSpecificField(fields[1]) &&
		isSpecificField(fields[2]) && isSpecificField(fields[3]) &&
		isSpecificField(fields[4]) && fields[5] == "*"
}

func isSpecificField(f string) bool {
	// A specific field is a single number, not a wildcard, range, list, or step.
	for _, c := range f {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(f) > 0
}

// RegisterPushFunc sets the callback used to push cron_task.sync updates to the user.
func (s *CronService) RegisterPushFunc(fn func(ctx context.Context, userID, conversationID uuid.UUID)) {
	s.pushSync = fn
}

// pushTaskSync pushes a cron_task.sync update to the user so the frontend panel refreshes.
func (s *CronService) pushTaskSync(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID) {
	if s.pushSync != nil {
		s.pushSync(ctx, userID, conversationID)
	}
}

// RegisterTask registers an existing DB task with the scheduler.
// Called by the Eino tool after creating a task in the DB, so the scheduler
// picks it up without needing to import the service package from tools.
func (s *CronService) RegisterTask(ctx context.Context, taskID uuid.UUID, nextRun time.Time) error {
	var task model.CronTask
	if err := s.db.WithContext(ctx).Where("id = ?", taskID).First(&task).Error; err != nil {
		return fmt.Errorf("task not found: %w", err)
	}
	// Update next_run_at if the caller computed a different value.
	if !nextRun.IsZero() {
		if err := s.db.WithContext(ctx).Model(&task).Update("next_run_at", nextRun).Error; err != nil {
			s.log.Warn("RegisterTask: failed to update next_run_at", zap.Error(err))
		}
		task.NextRunAt = nextRun
	}
	if err := s.registerTask(task); err != nil {
		s.log.Error("RegisterTask: failed to register with scheduler",
			zap.String("task_id", task.ID.String()),
			zap.Error(err),
		)
		return err
	}
	s.log.Info("RegisterTask: task registered with scheduler",
		zap.String("task_id", task.ID.String()),
		zap.Time("next_run", nextRun),
	)
	return nil
}

func (s *CronService) registerTask(task model.CronTask) error {
	job := cron.FuncJob(func() {
		if err := s.ExecuteTask(context.Background(), task.ID); err != nil {
			s.log.Error("cron job failed",
				zap.String("task_id", task.ID.String()),
				zap.Error(err),
			)
		}
	})

	entryID, err := s.scheduler.AddJob(task.Schedule, job)
	if err != nil {
		return fmt.Errorf("failed to add cron job: %w", err)
	}

	s.taskEntries[task.ID] = entryID
	return nil
}

// parseSchedule validates a schedule string and returns the normalized spec + next run time.
// Supported:
//   - Standard cron: "0 9 * * *" (5 fields)
//   - Predefined: "@hourly", "@daily", "@every 1h"
//   - One-time: "once:5m", "once:1h"
func parseSchedule(schedule string) (string, time.Time, error) {
	if len(schedule) > 5 && schedule[:5] == "once:" {
		d, err := time.ParseDuration(schedule[5:])
		if err != nil {
			return "", time.Time{}, fmt.Errorf("invalid once duration %q: %w", schedule[5:], err)
		}
		nextRun := time.Now().Add(d)
		// Generate a 6-field cron expression (sec min hour day month dow) that fires at the exact time.
		sched := fmt.Sprintf("%d %d %d %d %d *",
			nextRun.Second(), nextRun.Minute(), nextRun.Hour(),
			nextRun.Day(), int(nextRun.Month()))
		return sched, nextRun, nil
	}

	p := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	spec, err := p.Parse(schedule)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("invalid cron expression %q: %w", schedule, err)
	}

	nextRun := spec.Next(time.Now())
	if nextRun.IsZero() {
		return "", time.Time{}, fmt.Errorf("schedule %q never fires", schedule)
	}

	return schedule, nextRun, nil
}
