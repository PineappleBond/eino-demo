package runner

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestJSONLLogger_Log(t *testing.T) {
	tmp := t.TempDir() + "/test.jsonl"

	logger, err := NewJSONLLogger(tmp)
	if err != nil {
		t.Fatalf("NewJSONLLogger error: %v", err)
	}
	defer logger.Close()

	if logger.FilePath() != tmp {
		t.Errorf("FilePath = %q, want %q", logger.FilePath(), tmp)
	}

	// Write two entries
	err = logger.Log(JSONLLogEntry{Type: "message.new", Role: "user", Content: "hello"})
	if err != nil {
		t.Fatalf("Log error: %v", err)
	}

	err = logger.Log(JSONLLogEntry{Type: "message.delta", Role: "assistant", Delta: "world"})
	if err != nil {
		t.Fatalf("Log error: %v", err)
	}

	// Read back and verify
	data, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	var first JSONLLogEntry
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("unmarshal first entry: %v", err)
	}

	if first.Type != "message.new" {
		t.Errorf("first entry type = %q, want %q", first.Type, "message.new")
	}
	if first.Role != "user" {
		t.Errorf("first entry role = %q, want %q", first.Role, "user")
	}
	if first.Content != "hello" {
		t.Errorf("first entry content = %q, want %q", first.Content, "hello")
	}
	if first.Seq != 1 {
		t.Errorf("first entry seq = %d, want 1", first.Seq)
	}

	var second JSONLLogEntry
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatalf("unmarshal second entry: %v", err)
	}
	if second.Seq != 2 {
		t.Errorf("second entry seq = %d, want 2", second.Seq)
	}
}
