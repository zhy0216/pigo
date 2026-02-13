package main

import (
	"testing"
)

func TestToolRegistry(t *testing.T) {
	t.Run("register and get tool", func(t *testing.T) {
		registry := NewToolRegistry()
		registry.Register(NewReadTool())

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
		registry.Register(NewReadTool())
		registry.Register(NewWriteTool())

		names := registry.List()
		if len(names) != 2 {
			t.Errorf("expected 2 tools, got %d", len(names))
		}
	})

	t.Run("get definitions", func(t *testing.T) {
		registry := NewToolRegistry()
		registry.Register(NewReadTool())

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
