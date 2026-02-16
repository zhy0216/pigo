package main

import (
	"testing"
)

func TestToolRegistry(t *testing.T) {
	t.Run("register and get tool", func(t *testing.T) {
		registry := NewToolRegistry()
		registry.Register(NewReadTool("", &RealFileOps{}))

		tool, ok := registry.Get("read")
		if !ok {
			t.Error("expected to find 'read' tool")
		}
		if tool.Name() != "read" {
			t.Errorf("expected name 'read', got '%s'", tool.Name())
		}
	})

	t.Run("get nonexistent tool", func(t *testing.T) {
		registry := NewToolRegistry()
		_, ok := registry.Get("nonexistent")
		if ok {
			t.Error("expected not to find nonexistent tool")
		}
	})

	t.Run("list tools", func(t *testing.T) {
		registry := NewToolRegistry()
		registry.Register(NewReadTool("", &RealFileOps{}))
		registry.Register(NewWriteTool("", &RealFileOps{}))

		names := registry.List()
		if len(names) != 2 {
			t.Errorf("expected 2 tools, got %d", len(names))
		}
	})

	t.Run("get definitions", func(t *testing.T) {
		registry := NewToolRegistry()
		registry.Register(NewReadTool("", &RealFileOps{}))

		defs := registry.GetDefinitions()
		if len(defs) != 1 {
			t.Errorf("expected 1 definition, got %d", len(defs))
		}

		def := defs[0]
		if def["type"] != "function" {
			t.Errorf("expected type 'function', got '%v'", def["type"])
		}

		fn, ok := def["function"].(map[string]interface{})
		if !ok {
			t.Error("expected function to be a map")
		}
		if fn["name"] != "read" {
			t.Errorf("expected name 'read', got '%v'", fn["name"])
		}
	})
}

func TestToolDescriptionAndParameters(t *testing.T) {
	tools := []Tool{
		NewReadTool("", &RealFileOps{}),
		NewWriteTool("", &RealFileOps{}),
		NewEditTool("", &RealFileOps{}),
		NewBashTool(&RealExecOps{}),
	}

	for _, tool := range tools {
		t.Run(tool.Name()+"_description", func(t *testing.T) {
			desc := tool.Description()
			if desc == "" {
				t.Errorf("tool %s has empty description", tool.Name())
			}
		})

		t.Run(tool.Name()+"_parameters", func(t *testing.T) {
			params := tool.Parameters()
			if params == nil {
				t.Errorf("tool %s has nil parameters", tool.Name())
			}
			if params["type"] != "object" {
				t.Errorf("tool %s parameters should have type 'object'", tool.Name())
			}
			props, ok := params["properties"].(map[string]interface{})
			if !ok {
				t.Errorf("tool %s should have properties", tool.Name())
			}
			if len(props) == 0 {
				t.Errorf("tool %s should have at least one property", tool.Name())
			}
		})
	}
}

func TestValidateArgs(t *testing.T) {
	params := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name":  map[string]interface{}{"type": "string"},
			"count": map[string]interface{}{"type": "integer"},
			"ratio": map[string]interface{}{"type": "number"},
			"flag":  map[string]interface{}{"type": "boolean"},
		},
		"required": []string{"name"},
	}

	t.Run("valid args", func(t *testing.T) {
		err := ValidateArgs(params, map[string]interface{}{
			"name":  "test",
			"count": float64(5),
			"ratio": float64(1.5),
			"flag":  true,
		})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("missing required field", func(t *testing.T) {
		err := ValidateArgs(params, map[string]interface{}{
			"count": float64(5),
		})
		if err == nil {
			t.Error("expected error for missing required field")
		}
		if !contains(err.Error(), "name") {
			t.Errorf("error should mention 'name', got: %v", err)
		}
	})

	t.Run("wrong type string", func(t *testing.T) {
		err := ValidateArgs(params, map[string]interface{}{
			"name": 42.0,
		})
		if err == nil {
			t.Error("expected error for wrong type")
		}
	})

	t.Run("wrong type integer", func(t *testing.T) {
		err := ValidateArgs(params, map[string]interface{}{
			"name":  "test",
			"count": "not a number",
		})
		if err == nil {
			t.Error("expected error for wrong type")
		}
	})

	t.Run("float as integer fails", func(t *testing.T) {
		err := ValidateArgs(params, map[string]interface{}{
			"name":  "test",
			"count": float64(3.7),
		})
		if err == nil {
			t.Error("expected error for non-whole float as integer")
		}
	})

	t.Run("wrong type boolean", func(t *testing.T) {
		err := ValidateArgs(params, map[string]interface{}{
			"name": "test",
			"flag": "true",
		})
		if err == nil {
			t.Error("expected error for string as boolean")
		}
	})

	t.Run("no required field in schema", func(t *testing.T) {
		noReq := map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{"type": "string"},
			},
		}
		err := ValidateArgs(noReq, map[string]interface{}{})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("required as interface slice", func(t *testing.T) {
		p := map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
			"required":   []interface{}{"field1"},
		}
		err := ValidateArgs(p, map[string]interface{}{})
		if err == nil {
			t.Error("expected error for missing required field")
		}
	})

	t.Run("unknown property allowed", func(t *testing.T) {
		err := ValidateArgs(params, map[string]interface{}{
			"name":    "test",
			"unknown": "value",
		})
		if err != nil {
			t.Errorf("unexpected error for unknown property: %v", err)
		}
	})
}

func TestValidateArgsInExecute(t *testing.T) {
	registry := NewToolRegistry()
	registry.Register(NewReadTool("", &RealFileOps{}))

	ctx := t.Context()

	t.Run("validation catches missing required", func(t *testing.T) {
		result := registry.Execute(ctx, "read", map[string]interface{}{})
		if !result.IsError {
			t.Error("expected error for missing required 'path'")
		}
		if !contains(result.ForLLM, "validation failed") {
			t.Errorf("expected validation error message, got: %s", result.ForLLM)
		}
	})
}
