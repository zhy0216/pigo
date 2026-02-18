package types

import "context"

// Tool is the interface that all tools must implement.
type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]interface{}
	Execute(ctx context.Context, args map[string]interface{}) *ToolResult
}
