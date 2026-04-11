package runner

import (
	"context"
	"fmt"
)

func unknownToolsHandler(ctx context.Context, name string, input string) (string, error) {
	return fmt.Sprintf("Unknown tool '%s'. Please check your input or use one of the available tools listed in your system prompt.", name), nil
}
