package types

// Message represents a chat message.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall represents a tool call from the model.
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// ChatResponse represents the response from a chat completion.
type ChatResponse struct {
	Content      string
	ToolCalls    []ToolCall
	FinishReason string
	Usage        TokenUsage
}

// TokenUsage tracks token counts from an API response.
type TokenUsage struct {
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
}

// ChatOption configures optional behavior for a Chat call.
type ChatOption func(*ChatConfig)

// ChatConfig holds optional settings for a Chat call.
type ChatConfig struct {
	JSONMode   bool              // Request JSON object response format
	JSONSchema *JSONSchemaConfig // Request JSON schema response format (takes precedence over JSONMode)
}

// JSONSchemaConfig defines a strict JSON schema for the response.
type JSONSchemaConfig struct {
	Name   string
	Schema map[string]interface{}
}

// ApplyChatOptions merges variadic options into a ChatConfig.
func ApplyChatOptions(opts []ChatOption) ChatConfig {
	var cfg ChatConfig
	for _, o := range opts {
		o(&cfg)
	}
	return cfg
}

// WithJSONMode requests JSON object response format from the API.
func WithJSONMode() ChatOption {
	return func(c *ChatConfig) { c.JSONMode = true }
}

// WithJSONSchema requests a strict JSON schema response format from the API.
func WithJSONSchema(name string, schema map[string]interface{}) ChatOption {
	return func(c *ChatConfig) {
		c.JSONSchema = &JSONSchemaConfig{Name: name, Schema: schema}
	}
}
