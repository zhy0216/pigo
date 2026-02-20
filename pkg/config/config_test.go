package config

import (
	"os"
	"path/filepath"
	"testing"
)

// clearEnv unsets all config-related env vars for a clean test.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"OPENAI_API_KEY",
		"OPENAI_BASE_URL",
		"PIGO_MODEL",
		"OPENAI_API_TYPE",
		"PIGO_HOME",
	} {
		t.Setenv(key, "")
	}
}

func TestLoadEnvVarsOnly(t *testing.T) {
	clearEnv(t)
	// Point PIGO_HOME to a dir with no config file.
	t.Setenv("PIGO_HOME", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "sk-test-123")
	t.Setenv("OPENAI_BASE_URL", "https://custom.api.com/v1")
	t.Setenv("PIGO_MODEL", "gpt-3.5-turbo")
	t.Setenv("OPENAI_API_TYPE", "responses")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.APIKey != "sk-test-123" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "sk-test-123")
	}
	if cfg.BaseURL != "https://custom.api.com/v1" {
		t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, "https://custom.api.com/v1")
	}
	if cfg.Model != "gpt-3.5-turbo" {
		t.Errorf("Model = %q, want %q", cfg.Model, "gpt-3.5-turbo")
	}
	if cfg.APIType != "responses" {
		t.Errorf("APIType = %q, want %q", cfg.APIType, "responses")
	}
}

func TestLoadDefaults(t *testing.T) {
	clearEnv(t)
	t.Setenv("PIGO_HOME", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "sk-required")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.BaseURL != "" {
		t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, "")
	}
	if cfg.Model != "gpt-4o" {
		t.Errorf("Model = %q, want %q", cfg.Model, "gpt-4o")
	}
	if cfg.APIType != "chat" {
		t.Errorf("APIType = %q, want %q", cfg.APIType, "chat")
	}
}

func TestLoadMissingAPIKey(t *testing.T) {
	clearEnv(t)
	t.Setenv("PIGO_HOME", t.TempDir())

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing API key, got nil")
	}
}

func TestLoadConfigFileOverridesEnv(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	t.Setenv("PIGO_HOME", dir)

	// Set env vars
	t.Setenv("OPENAI_API_KEY", "sk-from-env")
	t.Setenv("PIGO_MODEL", "gpt-3.5-turbo")

	// Write config file that overrides both
	configJSON := `{
		"api_key": "sk-from-file",
		"model": "claude-3-opus"
	}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(configJSON), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.APIKey != "sk-from-file" {
		t.Errorf("APIKey = %q, want %q (file should override env)", cfg.APIKey, "sk-from-file")
	}
	if cfg.Model != "claude-3-opus" {
		t.Errorf("Model = %q, want %q (file should override env)", cfg.Model, "claude-3-opus")
	}
}

func TestLoadPartialOverride(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	t.Setenv("PIGO_HOME", dir)

	// Set env vars
	t.Setenv("OPENAI_API_KEY", "sk-from-env")
	t.Setenv("OPENAI_BASE_URL", "https://env.api.com")

	// Config file only sets model and api_type
	configJSON := `{
		"model": "custom-model",
		"api_type": "responses"
	}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(configJSON), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// api_key from env
	if cfg.APIKey != "sk-from-env" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "sk-from-env")
	}
	// base_url from env
	if cfg.BaseURL != "https://env.api.com" {
		t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, "https://env.api.com")
	}
	// model from file
	if cfg.Model != "custom-model" {
		t.Errorf("Model = %q, want %q", cfg.Model, "custom-model")
	}
	// api_type from file
	if cfg.APIType != "responses" {
		t.Errorf("APIType = %q, want %q", cfg.APIType, "responses")
	}
}

func TestLoadMissingConfigFile(t *testing.T) {
	clearEnv(t)
	t.Setenv("PIGO_HOME", t.TempDir()) // empty dir, no config.json
	t.Setenv("OPENAI_API_KEY", "sk-test")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error when config file missing: %v", err)
	}

	if cfg.APIKey != "sk-test" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "sk-test")
	}
	if cfg.Model != "gpt-4o" {
		t.Errorf("Model = %q, want %q", cfg.Model, "gpt-4o")
	}
}

func TestLoadMalformedConfigFile(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	t.Setenv("PIGO_HOME", dir)
	t.Setenv("OPENAI_API_KEY", "sk-test")

	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{bad json"), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for malformed config file, got nil")
	}
}

func TestLoadWithPlugins(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	t.Setenv("PIGO_HOME", dir)
	t.Setenv("OPENAI_API_KEY", "sk-test")

	configJSON := `{
		"plugins": [
			{
				"name": "test-plugin",
				"hooks": {
					"tool_start": [
						{"command": "echo hello", "blocking": true, "timeout": 5}
					]
				}
			}
		]
	}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(configJSON), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(cfg.Plugins))
	}
	if cfg.Plugins[0].Name != "test-plugin" {
		t.Errorf("plugin name = %q, want %q", cfg.Plugins[0].Name, "test-plugin")
	}
}

func TestLoadUnreadableConfigFile(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	t.Setenv("PIGO_HOME", dir)
	t.Setenv("OPENAI_API_KEY", "sk-test")

	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"model":"x"}`), 0000); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for unreadable config file")
	}
}

func TestLoadWithUserHomeDir(t *testing.T) {
	clearEnv(t)
	// Do NOT set PIGO_HOME -- force the UserHomeDir fallback path
	t.Setenv("OPENAI_API_KEY", "sk-test-home")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.APIKey != "sk-test-home" {
		t.Errorf("expected 'sk-test-home', got '%s'", cfg.APIKey)
	}
}

func TestEnsureWorkspaceCreatesDirectories(t *testing.T) {
	clearEnv(t)
	dir := filepath.Join(t.TempDir(), "new-pigo-home")
	t.Setenv("PIGO_HOME", dir)

	if err := EnsureWorkspace(); err != nil {
		t.Fatalf("EnsureWorkspace() error: %v", err)
	}

	// Check that home dir and skills subdir were created
	for _, sub := range []string{"", "skills"} {
		path := filepath.Join(dir, sub)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("expected directory %s to exist: %v", path, err)
		} else if !info.IsDir() {
			t.Errorf("expected %s to be a directory", path)
		}
	}
}

func TestEnsureWorkspaceIdempotent(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	t.Setenv("PIGO_HOME", dir)

	// Call twice — second call should not error
	if err := EnsureWorkspace(); err != nil {
		t.Fatalf("first EnsureWorkspace() error: %v", err)
	}
	if err := EnsureWorkspace(); err != nil {
		t.Fatalf("second EnsureWorkspace() error: %v", err)
	}
}

func TestLoadCreatesWorkspace(t *testing.T) {
	clearEnv(t)
	dir := filepath.Join(t.TempDir(), "fresh")
	t.Setenv("PIGO_HOME", dir)
	t.Setenv("OPENAI_API_KEY", "sk-test")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.APIKey != "sk-test" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "sk-test")
	}

	// Verify workspace was created
	if _, err := os.Stat(filepath.Join(dir, "skills")); err != nil {
		t.Errorf("expected skills directory to exist after Load(): %v", err)
	}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   string
	}{
		{"first non-empty", []string{"a", "b", "c"}, "a"},
		{"skip empty", []string{"", "b", "c"}, "b"},
		{"all empty", []string{"", "", ""}, ""},
		{"single value", []string{"x"}, "x"},
		{"no values", []string{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolve(tt.values...)
			if got != tt.want {
				t.Errorf("resolve(%v) = %q, want %q", tt.values, got, tt.want)
			}
		})
	}
}
