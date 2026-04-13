package runner

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// JSONLLogEntry represents a single line in the sub-conversation JSONL log.
type JSONLLogEntry struct {
	TS              string `json:"ts"`
	Seq             int64  `json:"seq"`
	Type            string `json:"type"` // message.new, message.delta, message.tool_call, message.done, error
	Role            string `json:"role,omitempty"`
	Content         string `json:"content,omitempty"`
	Delta           string `json:"delta,omitempty"`
	Tool            string `json:"tool,omitempty"`
	ToolInput       string `json:"tool_input,omitempty"`
	ToolOutput      string `json:"tool_output,omitempty"`
	Status          string `json:"status,omitempty"` // completed, failed
	FinishReason    string `json:"finish_reason,omitempty"`
	TokenPrompt     int64  `json:"token_prompt,omitempty"`
	TokenCompletion int64  `json:"token_completion,omitempty"`
	Error           string `json:"error,omitempty"`
}

// JSONLLogger writes structured log entries to a JSONL file.
// It is safe for concurrent use — appends from the same sub-agent goroutine.
type JSONLLogger struct {
	mu   sync.Mutex
	file *os.File
	seq  int64
}

// NewJSONLLogger creates a JSONL logger that writes to the given file path.
func NewJSONLLogger(filePath string) (*JSONLLogger, error) {
	f, err := os.Create(filePath)
	if err != nil {
		return nil, err
	}
	return &JSONLLogger{file: f}, nil
}

// Log writes a log entry to the JSONL file. The seq is auto-incremented
// per entry. Callers provide the Type and other fields.
func (l *JSONLLogger) Log(entry JSONLLogEntry) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.seq++
	entry.TS = time.Now().UTC().Format(time.RFC3339)
	entry.Seq = l.seq

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	_, err = l.file.Write(append(data, '\n'))
	return err
}

// FilePath returns the path of the log file.
func (l *JSONLLogger) FilePath() string {
	return l.file.Name()
}

// Close flushes and closes the file.
func (l *JSONLLogger) Close() error {
	return l.file.Close()
}
