package utils

import (
	"fmt"

	"github.com/pkoukk/tiktoken-go"
)

// TokenCounter counts tokens for a specific model encoding.
type TokenCounter struct {
	enc *tiktoken.Tiktoken
}

// NewTokenCounter creates a counter for a model tier.
// Supported tiers: haiku, sonnet, opus.
// Note: tiktoken uses OpenAI encodings. For Anthropic Claude models,
// o200k_base is the closest approximation (~80-90% accuracy).
// This is sufficient for threshold-based compaction checks.
func NewTokenCounter(modelTier string) (*TokenCounter, error) {
	encoding := modelTierToEncoding(modelTier)
	enc, err := tiktoken.GetEncoding(encoding)
	if err != nil {
		return nil, fmt.Errorf("tiktoken encoding %q for tier %q: %w", encoding, modelTier, err)
	}
	return &TokenCounter{enc: enc}, nil
}

// CountMessage returns the token count for a single message content.
// It includes a role prefix to approximate the formatting overhead
// that the LLM applies when constructing the prompt.
func (tc *TokenCounter) CountMessage(role, content string) int {
	tokens := tc.enc.Encode(fmt.Sprintf("%s: %s", role, content), nil, nil)
	return len(tokens)
}

// CountMessages returns the total token count for a list of messages.
func (tc *TokenCounter) CountMessages(roles []string, contents []string) int {
	total := 0
	for i, role := range roles {
		content := ""
		if i < len(contents) {
			content = contents[i]
		}
		total += tc.CountMessage(role, content)
	}
	return total
}

// modelTierToEncoding maps a model tier to the closest tiktoken encoding.
func modelTierToEncoding(tier string) string {
	switch tier {
	case "haiku":
		// Claude Haiku: o200k_base is the closest approximation
		return "o200k_base"
	case "sonnet":
		// Claude Sonnet 3.5/4/4.5: o200k_base
		return "o200k_base"
	case "opus":
		// Claude Opus: o200k_base
		return "o200k_base"
	default:
		return "o200k_base"
	}
}
