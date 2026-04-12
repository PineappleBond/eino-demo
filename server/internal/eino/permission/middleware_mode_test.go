package permission

import "testing"

func TestConversationMode_Values(t *testing.T) {
	tests := []struct {
		name     string
		mode     ConversationMode
		expected string
	}{
		{"ask_before_edits", ModeAskBeforeEdits, "ask_before_edits"},
		{"edit_automatically", ModeEditAutomatically, "edit_automatically"},
		{"bypass_permissions", ModeBypassPermissions, "bypass_permissions"},
		{"plan_mode", ModePlanMode, "plan_mode"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.mode) != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, tt.mode)
			}
		})
	}
}

func TestConversationMode_DefaultIsEmpty(t *testing.T) {
	var mode ConversationMode
	if mode != "" {
		t.Errorf("expected default ConversationMode to be empty, got %q", mode)
	}
}
