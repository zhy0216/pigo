package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds the application configuration.
type Config struct {
	APIKey     string
	BaseURL    string
	Model      string
	APIType    string // "chat" or "responses"
	EmbedModel string
}

// fileConfig maps to the JSON config file structure.
type fileConfig struct {
	APIKey     string `json:"api_key,omitempty"`
	BaseURL    string `json:"base_url,omitempty"`
	Model      string `json:"model,omitempty"`
	APIType    string `json:"api_type,omitempty"`
	EmbedModel string `json:"embed_model,omitempty"`
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
	fc, err := readConfigFile()
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		APIKey:     resolve(fc.APIKey, os.Getenv("OPENAI_API_KEY")),
		BaseURL:    resolve(fc.BaseURL, os.Getenv("OPENAI_BASE_URL"), ""),
		Model:      resolve(fc.Model, os.Getenv("PIGO_MODEL"), "gpt-4o"),
		APIType:    resolve(fc.APIType, os.Getenv("OPENAI_API_TYPE"), "chat"),
		EmbedModel: resolve(fc.EmbedModel, os.Getenv("PIGO_EMBED_MODEL"), "text-embedding-3-small"),
	}

	if cfg.APIKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY is required (set via env var or config file)")
	}

	return cfg, nil
}

// readConfigFile reads and parses the JSON config file.
// Returns a zero-value fileConfig if the file does not exist.
func readConfigFile() (fileConfig, error) {
	var fc fileConfig

	homeDir := os.Getenv("PIGO_HOME")
	if homeDir == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return fc, fmt.Errorf("failed to determine home directory: %w", err)
		}
		homeDir = filepath.Join(h, ".pigo")
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
