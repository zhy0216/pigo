package types

import "testing"

func TestApplyChatOptions_Nil(t *testing.T) {
	cfg := ApplyChatOptions(nil)
	if cfg.JSONMode {
		t.Error("expected JSONMode=false for nil opts")
	}
	if cfg.JSONSchema != nil {
		t.Error("expected JSONSchema=nil for nil opts")
	}
}

func TestApplyChatOptions_Empty(t *testing.T) {
	cfg := ApplyChatOptions([]ChatOption{})
	if cfg.JSONMode {
		t.Error("expected JSONMode=false for empty opts")
	}
}

func TestWithJSONMode(t *testing.T) {
	cfg := ApplyChatOptions([]ChatOption{WithJSONMode()})
	if !cfg.JSONMode {
		t.Error("expected JSONMode=true")
	}
}

func TestWithJSONSchema(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"answer": map[string]interface{}{"type": "string"},
		},
	}
	cfg := ApplyChatOptions([]ChatOption{WithJSONSchema("my_schema", schema)})
	if cfg.JSONSchema == nil {
		t.Fatal("expected JSONSchema to be set")
	}
	if cfg.JSONSchema.Name != "my_schema" {
		t.Errorf("expected name 'my_schema', got %q", cfg.JSONSchema.Name)
	}
	if cfg.JSONSchema.Schema["type"] != "object" {
		t.Errorf("expected schema type 'object', got %v", cfg.JSONSchema.Schema["type"])
	}
}

func TestApplyChatOptions_Multiple(t *testing.T) {
	schema := map[string]interface{}{"type": "object"}
	cfg := ApplyChatOptions([]ChatOption{
		WithJSONMode(),
		WithJSONSchema("test", schema),
	})
	if !cfg.JSONMode {
		t.Error("expected JSONMode=true")
	}
	if cfg.JSONSchema == nil || cfg.JSONSchema.Name != "test" {
		t.Error("expected JSONSchema to be set")
	}
}
