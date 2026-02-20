package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/zhy0216/pigo/pkg/types"
)

// ToolRegistry manages all registered tools.
type ToolRegistry struct {
	tools map[string]types.Tool
	mu    sync.RWMutex
}

// NewToolRegistry creates a new ToolRegistry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]types.Tool),
	}
}

// Register adds a tool to the registry.
func (r *ToolRegistry) Register(tool types.Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name()] = tool
}

// Get retrieves a tool by name.
func (r *ToolRegistry) Get(name string) (types.Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	return tool, ok
}

// Execute runs a tool by name with the given arguments.
// Arguments are validated against the tool's parameter schema before execution.
func (r *ToolRegistry) Execute(ctx context.Context, name string, args map[string]interface{}) *types.ToolResult {
	tool, ok := r.Get(name)
	if !ok {
		return types.ErrorResult("tool not found: " + name)
	}
	if err := ValidateArgs(tool.Parameters(), args); err != nil {
		return types.ErrorResult(fmt.Sprintf("argument validation failed: %v", err))
	}
	return tool.Execute(ctx, args)
}

// ValidateArgs checks args against a JSON schema (params). It validates
// required fields and basic type matching (string, integer/number, boolean).
func ValidateArgs(params map[string]interface{}, args map[string]interface{}) error {
	// Check required fields
	if required, ok := params["required"]; ok {
		var requiredFields []string
		switch r := required.(type) {
		case []string:
			requiredFields = r
		case []interface{}:
			for _, v := range r {
				if s, ok := v.(string); ok {
					requiredFields = append(requiredFields, s)
				}
			}
		}
		var missing []string
		for _, field := range requiredFields {
			if _, ok := args[field]; !ok {
				missing = append(missing, field)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("missing required fields: %s", strings.Join(missing, ", "))
		}
	}

	// Check type matching for provided args
	properties, _ := params["properties"].(map[string]interface{})
	if properties == nil {
		return nil
	}

	for key, val := range args {
		propDef, ok := properties[key].(map[string]interface{})
		if !ok {
			continue // unknown property, skip
		}
		expectedType, _ := propDef["type"].(string)
		if expectedType == "" {
			continue
		}
		if err := checkType(key, val, expectedType); err != nil {
			return err
		}
	}

	return nil
}

// checkType verifies that val matches the expected JSON schema type.
func checkType(key string, val interface{}, expectedType string) error {
	switch expectedType {
	case "string":
		if _, ok := val.(string); !ok {
			return fmt.Errorf("field %q must be a string", key)
		}
	case "integer":
		switch v := val.(type) {
		case float64:
			if v != float64(int(v)) {
				return fmt.Errorf("field %q must be an integer", key)
			}
		default:
			return fmt.Errorf("field %q must be an integer", key)
		}
	case "number":
		if _, ok := val.(float64); !ok {
			return fmt.Errorf("field %q must be a number", key)
		}
	case "boolean":
		if _, ok := val.(bool); !ok {
			return fmt.Errorf("field %q must be a boolean", key)
		}
	}
	return nil
}

// GetDefinitions returns OpenAI-compatible tool definitions.
func (r *ToolRegistry) GetDefinitions() []map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	definitions := make([]map[string]interface{}, 0, len(r.tools))
	for _, tool := range r.tools {
		definitions = append(definitions, map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        tool.Name(),
				"description": tool.Description(),
				"parameters":  tool.Parameters(),
			},
		})
	}
	return definitions
}

// List returns all registered tool names.
func (r *ToolRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}
