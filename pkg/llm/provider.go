package llm

import (
	"context"
	"io"

	"github.com/zhy0216/pigo/pkg/types"
)

// Provider defines the interface for different LLM backends.
type Provider interface {
	Chat(ctx context.Context, messages []types.Message, toolDefs []map[string]interface{}, opts ...types.ChatOption) (*types.ChatResponse, error)
	ChatStream(ctx context.Context, messages []types.Message, toolDefs []map[string]interface{}, w io.Writer) (*types.ChatResponse, error)
	GetModel() string
	SetModel(model string)
}

// Compile-time interface compliance checks.
var (
	_ Provider = (*OpenAIProvider)(nil)
	_ Provider = (*AnthropicProvider)(nil)
)

// Client wraps an LLM Provider.
type Client struct {
	Provider
}
