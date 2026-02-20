package config

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/user/pigo/pkg/hooks"
)

//go:embed config.schema.json
var configSchema []byte

// Config holds the application configuration.
type Config struct {
	APIKey  string
	BaseURL string
	Model   string
	APIType string // "chat" or "responses"
	Plugins []hooks.PluginConfig
}

// fileConfig maps to the JSON config file structure.
type fileConfig struct {
	APIKey  string   `json:"api_key,omitempty"`
	BaseURL string   `json:"base_url,omitempty"`
	Model   string   `json:"model,omitempty"`
	APIType string   `json:"api_type,omitempty"`
	Plugins []string `json:"plugins,omitempty"`
}

// defaultFileConfig is used only for writing the seed config.json.
// It omits the omitempty tags so all fields appear in the output.
type defaultFileConfig struct {
	Schema  string `json:"$schema"`
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	APIType string `json:"api_type"`
}

// resolve returns the first non-empty value from the provided strings.
func resolve(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// Load reads configuration by merging config file, environment variables,
// and defaults. Priority: config file > env var > default.
// Returns an error if the API key is not set from any source.
func Load() (*Config, error) {
	if err := EnsureWorkspace(); err != nil {
		return nil, err
	}

	fc, err := readConfigFile()
	if err != nil {
		return nil, err
	}

	plugins, err := resolvePlugins(fc.Plugins)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		APIKey:  resolve(fc.APIKey, os.Getenv("OPENAI_API_KEY")),
		BaseURL: resolve(fc.BaseURL, os.Getenv("OPENAI_BASE_URL"), ""),
		Model:   resolve(fc.Model, os.Getenv("PIGO_MODEL"), "gpt-4o"),
		APIType: resolve(fc.APIType, os.Getenv("OPENAI_API_TYPE"), "chat"),
		Plugins: plugins,
	}

	if cfg.APIKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY is required (set via env var or config file)")
	}

	return cfg, nil
}

// HomeDir returns the pigo home directory path (~/.pigo or PIGO_HOME).
func HomeDir() (string, error) {
	homeDir := os.Getenv("PIGO_HOME")
	if homeDir == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to determine home directory: %w", err)
		}
		homeDir = filepath.Join(h, ".pigo")
	}
	return homeDir, nil
}

// EnsureWorkspace creates the pigo home directory, its subdirectories,
// and a default config.json if they do not already exist.
func EnsureWorkspace() error {
	homeDir, err := HomeDir()
	if err != nil {
		return err
	}

	dirs := []string{
		homeDir,
		filepath.Join(homeDir, "skills"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Always write/update the schema file.
	schemaPath := filepath.Join(homeDir, "config.schema.json")
	if err := os.WriteFile(schemaPath, configSchema, 0644); err != nil {
		return fmt.Errorf("failed to write schema file %s: %w", schemaPath, err)
	}

	configPath := filepath.Join(homeDir, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		seed := defaultFileConfig{
			Schema:  "./config.schema.json",
			APIKey:  "",
			BaseURL: "https://api.openai.com/v1",
			Model:   "gpt-4o",
			APIType: "chat",
		}
		data, err := json.MarshalIndent(seed, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal default config: %w", err)
		}
		data = append(data, '\n')
		if err := os.WriteFile(configPath, data, 0644); err != nil {
			return fmt.Errorf("failed to write default config %s: %w", configPath, err)
		}
	}

	return nil
}

// readConfigFile reads and parses the JSON config file.
// Returns a zero-value fileConfig if the file does not exist.
func readConfigFile() (fileConfig, error) {
	var fc fileConfig

	homeDir, err := HomeDir()
	if err != nil {
		return fc, err
	}

	path := filepath.Join(homeDir, "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fc, nil
		}
		return fc, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	if err := json.Unmarshal(data, &fc); err != nil {
		return fc, fmt.Errorf("failed to parse config file %s: %w", path, err)
	}

	return fc, nil
}

// resolvePlugins reads plugins.json and resolves plugin names to full PluginConfig.
// Unknown plugin names are silently skipped with a warning to stderr.
func resolvePlugins(names []string) ([]hooks.PluginConfig, error) {
	if len(names) == 0 {
		return nil, nil
	}

	defs, err := readPluginsFile()
	if err != nil {
		return nil, err
	}

	var plugins []hooks.PluginConfig
	for _, name := range names {
		def, ok := defs[name]
		if !ok {
			fmt.Fprintf(os.Stderr, "warning: plugin %q not found in plugins.json, skipping\n", name)
			continue
		}
		if def.Name == "" {
			def.Name = name
		}
		plugins = append(plugins, def)
	}
	return plugins, nil
}

// readPluginsFile reads and parses the plugins.json file.
// Returns an empty map if the file does not exist.
func readPluginsFile() (map[string]hooks.PluginConfig, error) {
	homeDir, err := HomeDir()
	if err != nil {
		return nil, err
	}

	path := filepath.Join(homeDir, "plugins.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]hooks.PluginConfig{}, nil
		}
		return nil, fmt.Errorf("failed to read plugins file %s: %w", path, err)
	}

	var defs map[string]hooks.PluginConfig
	if err := json.Unmarshal(data, &defs); err != nil {
		return nil, fmt.Errorf("failed to parse plugins file %s: %w", path, err)
	}

	return defs, nil
}
