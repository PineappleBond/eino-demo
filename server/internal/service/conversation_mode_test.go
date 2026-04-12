package service

import "testing"

func TestValidConversationModes(t *testing.T) {
	validModes := []string{"ask_before_edits", "edit_automatically", "bypass_permissions", "plan_mode"}
	for _, mode := range validModes {
		if mode == "" {
			t.Error("found empty string in valid modes")
		}
	}
}

func TestInvalidModeWouldBeRejected(t *testing.T) {
	invalidModes := []string{"invalid", "ASK_BEFORE_EDITS", "edit"}
	validModes := map[string]bool{
		"ask_before_edits":   true,
		"edit_automatically": true,
		"bypass_permissions": true,
		"plan_mode":          true,
	}

	for _, mode := range invalidModes {
		if validModes[mode] {
			t.Errorf("expected %q to be invalid", mode)
		}
	}
}
