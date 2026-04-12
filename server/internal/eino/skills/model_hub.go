package skills

import (
	"context"
	"fmt"
	"sync"

	"github.com/cloudwego/eino/adk/middlewares/skill"
	"github.com/cloudwego/eino/components/model"

	openai "github.com/cloudwego/eino-ext/components/model/openai"

	"github.com/PineappleBond/eino-demo-dev/server/internal/eino"
)

// ModelHub implements skill.ModelHub by creating OpenAI chat models
// from the ModelProvider config on demand.
type ModelHub struct {
	provider *eino.ModelProvider

	mu    sync.Mutex
	cache map[string]model.ToolCallingChatModel
}

// NewModelHub creates a ModelHub backed by the given ModelProvider.
func NewModelHub(provider *eino.ModelProvider) *ModelHub {
	return &ModelHub{
		provider: provider,
		cache:    make(map[string]model.ToolCallingChatModel),
	}
}

// Get returns a ToolCallingChatModel for the given tier name (e.g. "haiku", "sonnet", "opus").
// Results are cached to avoid repeated model creation overhead.
func (h *ModelHub) Get(ctx context.Context, name string) (model.ToolCallingChatModel, error) {
	h.mu.Lock()
	if m, ok := h.cache[name]; ok {
		h.mu.Unlock()
		return m, nil
	}
	h.mu.Unlock()

	mc := h.provider.GetModel(name)
	if mc.BaseURL == "" && mc.Model == "" {
		return nil, fmt.Errorf("unknown model tier: %s", name)
	}

	m, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: mc.BaseURL,
		APIKey:  mc.APIKey,
		Model:   mc.Model,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create model %q: %w", name, err)
	}

	h.mu.Lock()
	h.cache[name] = m
	h.mu.Unlock()

	return m, nil
}

var _ skill.ModelHub = (*ModelHub)(nil)
