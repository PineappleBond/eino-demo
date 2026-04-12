package model

import "testing"

func TestConversationModeDefault(t *testing.T) {
	c := Conversation{}
	if c.Mode != "" {
		t.Errorf("expected empty Mode in zero-value struct, got %q", c.Mode)
	}
}
