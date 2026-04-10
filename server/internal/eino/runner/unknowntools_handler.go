package runner

import (
	"context"
	"fmt"
)

func unknownToolsHandler(ctx context.Context, name string, input string) (string, error) {
	return fmt.Sprintf("未知的Tool(%s), 请检查你的输入", name), nil
}
