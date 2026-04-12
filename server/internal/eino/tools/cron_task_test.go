package tools

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// ── parseCronSchedule tests ──
// Note: the implementation uses cron.NewParser(cron.SecondOptional) which
// only accepts single-field (seconds-only) cron specs plus @every descriptors
// via the underlying parser. Standard 5-field cron and @hourly etc. are
// rejected by this parser config. These tests document the actual behavior.

func TestParseCronSchedule_SecondsOnly(t *testing.T) {
	// The parser with SecondOptional flag accepts single integer values
	// (interpreted as "at this second of every minute")
	tests := []struct {
		name string
		spec string
	}{
		{"zero second", "0"},
		{"thirty second", "30"},
		{"fifteenth second", "15"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, nextRun, err := parseCronSchedule(tt.spec)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if spec != tt.spec {
				t.Errorf("spec = %q, want %q", spec, tt.spec)
			}
			if nextRun.IsZero() {
				t.Error("nextRun should not be zero")
			}
		})
	}
}

func TestParseCronSchedule_Once(t *testing.T) {
	tests := []struct {
		name    string
		schedule string
		wantErr bool
	}{
		{"once 5 minutes", "once:5m", false},
		{"once 1 hour", "once:1h", false},
		{"once 30 seconds", "once:30s", false},
		{"once 2 hours 30 minutes", "once:2h30m", false},
		{"once 100ms", "once:100ms", false},
		{"once invalid duration", "once:xyz", true},
		{"once empty duration", "once:", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, nextRun, err := parseCronSchedule(tt.schedule)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// once: schedules are normalized to @at <RFC3339>
			if len(spec) < 5 || spec[:4] != "@at " {
				t.Errorf("spec should start with '@at ', got %q", spec)
			}
			if nextRun.IsZero() {
				t.Error("nextRun should not be zero")
			}
			// nextRun should be roughly now + duration
			if !nextRun.After(time.Now().Add(-time.Second)) {
				t.Errorf("nextRun %v should be in the near future", nextRun)
			}
		})
	}
}

func TestParseCronSchedule_Invalid(t *testing.T) {
	invalidSchedules := []string{
		"not a cron",
		"60 25 * * *",
		"* * * * *",
		"0 9 * * *",
		"@hourly",
		"@daily",
		"@every 1h",
		"",
	}

	for _, schedule := range invalidSchedules {
		t.Run(schedule, func(t *testing.T) {
			spec, nextRun, err := parseCronSchedule(schedule)
			if err == nil {
				t.Errorf("expected error for schedule %q, got spec=%q nextRun=%v", schedule, spec, nextRun)
			}
		})
	}
}

// ── CronTaskTool with nil DB ──

func TestCronTaskRunner_NilDB(t *testing.T) {
	runner := &cronTaskRunner{db: nil}

	tests := []struct {
		name string
		input CronTaskInput
	}{
		{
			name:  "create fails with nil db",
			input: CronTaskInput{Action: "create", Content: "hello", Schedule: "once:5m"},
		},
		{
			name:  "list fails with nil db",
			input: CronTaskInput{Action: "list"},
		},
		{
			name:  "cancel fails with nil db",
			input: CronTaskInput{Action: "cancel", TaskID: "some-id"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := runner.Run(context.Background(), tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if out.Success {
				t.Errorf("Success = true, want false (nil DB). Message: %s", out.Message)
			}
			if out.Message != "database not available" {
				t.Errorf("Message = %q, want %q", out.Message, "database not available")
			}
		})
	}
}

// ── NewCronTaskTool creation test ──

func TestNewCronTaskTool(t *testing.T) {
	tool, err := NewCronTaskTool(nil, testUUID())
	if err != nil {
		t.Fatalf("NewCronTaskTool() error = %v", err)
	}
	if tool == nil {
		t.Error("NewCronTaskTool() returned nil tool")
	}
}

// ── CronTask output structure tests ──

func TestCronTaskOutput_Structure(t *testing.T) {
	out := CronTaskOutput{
		Success: true,
		Message: "test",
		Tasks: []CronTaskInfo{
			{
				ID:        "test-id",
				Content:   "test content",
				Schedule:  "once:5m",
				NextRunAt: "2026-01-01 09:00:00",
				Status:    "active",
			},
		},
	}

	if !out.Success {
		t.Error("Success should be true")
	}
	if out.Message != "test" {
		t.Errorf("Message = %q, want %q", out.Message, "test")
	}
	if len(out.Tasks) != 1 {
		t.Fatalf("Tasks count = %d, want 1", len(out.Tasks))
	}
	if out.Tasks[0].ID != "test-id" {
		t.Errorf("Task ID = %q, want %q", out.Tasks[0].ID, "test-id")
	}
}

func TestCronTaskInfo_Structure(t *testing.T) {
	info := CronTaskInfo{
		ID:        "abc-123",
		Content:   "send reminder",
		Schedule:  "once:1h",
		NextRunAt: "2026-04-12 10:00:00",
		Status:    "pending",
	}

	if info.ID != "abc-123" {
		t.Errorf("ID = %q, want %q", info.ID, "abc-123")
	}
	if info.Status != "pending" {
		t.Errorf("Status = %q, want %q", info.Status, "pending")
	}
}

func TestCronTaskInput_Structure(t *testing.T) {
	input := CronTaskInput{
		Action:   "create",
		Content:  "hello",
		Schedule: "once:5m",
		TaskID:   "task-1",
	}

	if input.Action != "create" {
		t.Errorf("Action = %q, want %q", input.Action, "create")
	}
	if input.Content != "hello" {
		t.Errorf("Content = %q, want %q", input.Content, "hello")
	}
	if input.Schedule != "once:5m" {
		t.Errorf("Schedule = %q, want %q", input.Schedule, "once:5m")
	}
}

func testUUID() uuid.UUID {
	return uuid.MustParse("00000000-0000-0000-0000-000000000001")
}
