package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"

	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
)

// CronTaskInput is the input schema for the cron_task tool.
type CronTaskInput struct {
	Action     string `json:"action" jsonschema_description:"Action: 'create', 'list', or 'cancel'"`
	Content    string `json:"content" jsonschema_description:"Message text to send when the task fires. Required for create."`
	SenderRole string `json:"sender_role" jsonschema_description:"Message sender role: 'user', 'assistant', 'system', or 'tool'."`
	Schedule   string `json:"schedule" jsonschema_description:"Required for create. One of: (1) 'once:' + Go duration like 'once:2m', 'once:1h', 'once:30s'. (2) 5-field cron: 'MINUTE HOUR DOM MONTH DOW' — MINUTE(0-59), HOUR(0-23), DOM=day-of-month(1-31), MONTH(1-12), DOW=day-of-week(0-6, 0=Sun). Example: '0 9 * * *' = daily 9AM. (3) Alias: @hourly, @daily, @midnight, @weekly, @monthly, @yearly, @annually, @every 1h. NOTE: @at is NOT supported."`
	TaskID     string `json:"task_id" jsonschema_description:"UUID of the task to cancel. Required for cancel."`
}

// CronTaskInfo represents a single cron task in tool output.
type CronTaskInfo struct {
	ID         string `json:"id"`
	Content    string `json:"content"`
	SenderRole string `json:"sender_role"`
	Schedule   string `json:"schedule"`
	NextRunAt  string `json:"next_run_at"`
	Status     string `json:"status"`
}

// CronTaskOutput is the output schema for the cron_task tool.
type CronTaskOutput struct {
	Success bool           `json:"success" jsonschema_description:"Whether the operation succeeded"`
	Message string         `json:"message" jsonschema_description:"Human-readable result description"`
	Tasks   []CronTaskInfo `json:"tasks,omitempty" jsonschema_description:"List of cron tasks (for list action)"`
}

// CronTaskRegisterFunc is a callback that registers a created task with the scheduler.
// This is set by module.go to avoid import cycles (tools → service → tools).
var CronTaskRegisterFunc func(ctx context.Context, taskID uuid.UUID, nextRun time.Time) error

// CronTaskSyncFunc is a callback that pushes a cron_task.sync update to the user.
// This is set by module.go to avoid import cycles.
var CronTaskSyncFunc func(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID)

// NewCronTaskTool creates a cron_task tool scoped to a conversation.
func NewCronTaskTool(db *gorm.DB, conversationID uuid.UUID) (tool.InvokableTool, error) {
	t := &cronTaskRunner{db: db, conversationID: conversationID}
	return utils.InferTool("cron_task", "Schedule, list, or cancel timed messages in this conversation. Create: provide 'content' (message text) and 'schedule' ('once:2m' for 2 min from now, '0 9 * * *' for daily 9AM, or @hourly). Cancel: provide 'task_id'.",
		func(ctx context.Context, input CronTaskInput) (CronTaskOutput, error) {
			return t.Run(ctx, input)
		})
}

type cronTaskRunner struct {
	db             *gorm.DB
	conversationID uuid.UUID
}

func (t *cronTaskRunner) Run(ctx context.Context, input CronTaskInput) (CronTaskOutput, error) {
	if t.db == nil {
		return CronTaskOutput{Success: false, Message: "database not available"}, nil
	}

	switch input.Action {
	case "create":
		if input.Content == "" {
			return CronTaskOutput{Success: false, Message: "content is required for create action"}, nil
		}
		if input.Schedule == "" {
			return CronTaskOutput{Success: false, Message: "schedule is required for create action"}, nil
		}
		if input.SenderRole != "assistant" && input.SenderRole != "user" && input.SenderRole != "tool" && input.SenderRole != "system" {
			return CronTaskOutput{Success: false, Message: "sender_role must is one of ('assistant', 'user', 'tool', 'system')"}, nil
		}

		// Verify conversation exists and get user_id
		var conv model.Conversation
		if err := t.db.WithContext(ctx).Where("id = ?", t.conversationID).First(&conv).Error; err != nil {
			return CronTaskOutput{Success: false, Message: "conversation not found"}, nil
		}

		var task model.CronTask
		task.ConversationID = t.conversationID
		task.Content = input.Content
		task.SenderRole = input.SenderRole
		task.Status = "pending"

		sched, nextRun, err := parseCronSchedule(input.Schedule)
		if err != nil {
			return CronTaskOutput{Success: false, Message: "invalid schedule: " + err.Error()}, nil
		}
		task.Schedule = sched
		task.NextRunAt = nextRun
		task.Status = "active"

		if err := t.db.WithContext(ctx).Create(&task).Error; err != nil {
			return CronTaskOutput{Success: false, Message: "failed to create cron task: " + err.Error()}, nil
		}

		// Register with the scheduler so it actually fires.
		if CronTaskRegisterFunc != nil {
			if err := CronTaskRegisterFunc(ctx, task.ID, nextRun); err != nil {
				// Rollback the DB task to avoid zombie entries.
				t.db.WithContext(ctx).Model(&task).Update("status", "cancelled")
				return CronTaskOutput{Success: false, Message: "failed to register task with scheduler: " + err.Error()}, nil
			}
		}

		// Notify the frontend via cron_task.sync so the cron panel refreshes.
		if CronTaskSyncFunc != nil {
			CronTaskSyncFunc(ctx, conv.UserID, t.conversationID)
		}

		return CronTaskOutput{
			Success: true,
			Message: "Created scheduled task. Message will be sent at " + nextRun.Format("2006-01-02 15:04:05"),
			Tasks: []CronTaskInfo{{
				ID:         task.ID.String(),
				Content:    task.Content,
				SenderRole: task.SenderRole,
				Schedule:   task.Schedule,
				NextRunAt:  nextRun.Format("2006-01-02 15:04:05"),
				Status:     task.Status,
			}},
		}, nil

	case "list":
		var tasks []model.CronTask
		if err := t.db.WithContext(ctx).
			Where("conversation_id = ?", t.conversationID).
			Order("next_run_at ASC").
			Find(&tasks).Error; err != nil {
			return CronTaskOutput{Success: false, Message: "failed to list cron tasks: " + err.Error()}, nil
		}

		infos := make([]CronTaskInfo, 0, len(tasks))
		for _, task := range tasks {
			infos = append(infos, CronTaskInfo{
				ID:         task.ID.String(),
				SenderRole: task.SenderRole,
				Content:    task.Content,
				Schedule:   task.Schedule,
				NextRunAt:  task.NextRunAt.Format("2006-01-02 15:04:05"),
				Status:     task.Status,
			})
		}

		return CronTaskOutput{
			Success: true,
			Message: fmt.Sprintf("Found %d scheduled tasks", len(infos)),
			Tasks:   infos,
		}, nil

	case "cancel":
		if input.TaskID == "" {
			return CronTaskOutput{Success: false, Message: "task_id is required for cancel action"}, nil
		}

		taskID, err := uuid.Parse(input.TaskID)
		if err != nil {
			return CronTaskOutput{Success: false, Message: "invalid task_id: " + err.Error()}, nil
		}

		result := t.db.WithContext(ctx).
			Where("id = ? AND conversation_id = ?", taskID, t.conversationID).
			Model(&model.CronTask{}).
			Updates(map[string]interface{}{
				"status": "cancelled",
			})
		if result.Error != nil {
			return CronTaskOutput{Success: false, Message: "failed to cancel task: " + result.Error.Error()}, nil
		}
		if result.RowsAffected == 0 {
			return CronTaskOutput{Success: false, Message: "task not found"}, nil
		}

		// Notify the frontend via cron_task.sync so the cron panel refreshes.
		var conv model.Conversation
		if err := t.db.WithContext(ctx).Where("id = ?", t.conversationID).First(&conv).Error; err == nil && CronTaskSyncFunc != nil {
			CronTaskSyncFunc(ctx, conv.UserID, t.conversationID)
		}

		return CronTaskOutput{
			Success: true,
			Message: "Cancelled scheduled task",
		}, nil

	default:
		return CronTaskOutput{
			Success: false,
			Message: "unknown action: " + input.Action + ". Use 'create', 'list', or 'cancel'.",
		}, nil
	}
}

// parseCronSchedule validates a schedule string and returns the normalized spec + next run time.
// Supported:
//   - Standard cron: "0 9 * * *" (5 fields)
//   - Predefined: "@hourly", "@daily", "@every 1h"
//   - One-time: "once:5m", "once:1h"
func parseCronSchedule(schedule string) (string, time.Time, error) {
	if len(schedule) > 5 && schedule[:5] == "once:" {
		d, err := time.ParseDuration(schedule[5:])
		if err != nil {
			return "", time.Time{}, fmt.Errorf("invalid once duration %q: %w", schedule[5:], err)
		}
		nextRun := time.Now().Add(d)
		// Generate a 6-field cron expression (sec min hour day month dow) that fires at the exact time.
		// The scheduler expects 6 fields (created with cron.WithSeconds()).
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
