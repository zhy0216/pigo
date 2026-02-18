package main

import (
	"testing"
)

func TestMemoryRecallToolMetadata(t *testing.T) {
	tool := NewMemoryRecallTool(nil)
	if tool.Name() != "memory_recall" {
		t.Errorf("expected 'memory_recall', got '%s'", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("description should not be empty")
	}

	params := tool.Parameters()
	props, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("parameters should have properties")
	}
	if _, ok := props["query"]; !ok {
		t.Error("should have 'query' parameter")
	}
	if _, ok := props["category"]; !ok {
		t.Error("should have 'category' parameter")
	}
	if _, ok := props["top_k"]; !ok {
		t.Error("should have 'top_k' parameter")
	}

	required, ok := params["required"].([]string)
	if !ok {
		t.Fatal("should have required fields")
	}
	if len(required) != 1 || required[0] != "query" {
		t.Error("'query' should be the only required field")
	}
}

func TestMemoryRememberToolMetadata(t *testing.T) {
	tool := NewMemoryRememberTool(nil)
	if tool.Name() != "memory_remember" {
		t.Errorf("expected 'memory_remember', got '%s'", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("description should not be empty")
	}

	params := tool.Parameters()
	props, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("parameters should have properties")
	}
	if _, ok := props["category"]; !ok {
		t.Error("should have 'category' parameter")
	}
	if _, ok := props["abstract"]; !ok {
		t.Error("should have 'abstract' parameter")
	}
	if _, ok := props["content"]; !ok {
		t.Error("should have 'content' parameter")
	}
}

func TestMemoryForgetToolMetadata(t *testing.T) {
	tool := NewMemoryForgetTool(nil)
	if tool.Name() != "memory_forget" {
		t.Errorf("expected 'memory_forget', got '%s'", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("description should not be empty")
	}

	params := tool.Parameters()
	props, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("parameters should have properties")
	}
	if _, ok := props["id"]; !ok {
		t.Error("should have 'id' parameter")
	}
}
