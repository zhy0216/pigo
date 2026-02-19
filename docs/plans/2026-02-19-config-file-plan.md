# Config File Support Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add JSON config file at `~/.pigo/config.json` with priority over env vars.

**Architecture:** New `pkg/config/` package owns the `Config` struct and `Load()` function. It reads `~/.pigo/config.json`, merges with env vars (config > env > defaults), and returns a unified `Config`. The `embed_model` field gets threaded through `NewClient` to eliminate the direct `os.Getenv` call in `pkg/llm/client.go`.

**Tech Stack:** Go standard library (`encoding/json`, `os`, `path/filepath`)

---

### Task 1: Create `pkg/config/` package with Config struct and Load function

**Files:**
- Create: `pkg/config/config.go`
- Create: `pkg/config/config_test.go`

**Step 1: Write the failing tests**

In `pkg/config/config_test.go`:

```go
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_EnvVarsOnly(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "env-key")
	t.Setenv("OPENAI_BASE_URL", "https://env.example.com")
	t.Setenv("PIGO_MODEL", "gpt-4")
	t.Setenv("OPENAI_API_TYPE", "responses")
	t.Setenv("PIGO_EMBED_MODEL", "text-embedding-ada-002")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.APIKey != "env-key" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "env-key")
	}
	if cfg.BaseURL != "https://env.example.com" {
		t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, "https://env.example.com")
	}
	if cfg.Model != "gpt-4" {
		t.Errorf("Model = %q, want %q", cfg.Model, "gpt-4")
	}
	if cfg.APIType != "responses" {
		t.Errorf("APIType = %q, want %q", cfg.APIType, "responses")
	}
	if cfg.EmbedModel != "text-embedding-ada-002" {
		t.Errorf("EmbedModel = %q, want %q", cfg.EmbedModel, "text-embedding-ada-002")
	}
}

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("PIGO_MODEL", "")
	t.Setenv("OPENAI_API_TYPE", "")
	t.Setenv("PIGO_EMBED_MODEL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Model != "gpt-4o" {
		t.Errorf("Model = %q, want default %q", cfg.Model, "gpt-4o")
	}
	if cfg.APIType != "chat" {
		t.Errorf("APIType = %q, want default %q", cfg.APIType, "chat")
	}
	if cfg.EmbedModel != "text-embedding-3-small" {
		t.Errorf("EmbedModel = %q, want default %q", cfg.EmbedModel, "text-embedding-3-small")
	}
}

func TestLoad_MissingAPIKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")

	_, err := Load()
	if err == nil {
		t.Error("expected error for missing API key")
	}
}

func TestLoad_ConfigFileOverridesEnv(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("PIGO_HOME", tmpDir)

	t.Setenv("OPENAI_API_KEY", "env-key")
	t.Setenv("PIGO_MODEL", "env-model")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("OPENAI_API_TYPE", "")
	t.Setenv("PIGO_EMBED_MODEL", "")

	fileConfig := fileConfig{
		APIKey: "file-key",
		Model:  "file-model",
	}
	data, _ := json.Marshal(fileConfig)
	os.WriteFile(filepath.Join(tmpDir, "config.json"), data, 0600)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.APIKey != "file-key" {
		t.Errorf("APIKey = %q, want config file value %q", cfg.APIKey, "file-key")
	}
	if cfg.Model != "file-model" {
		t.Errorf("Model = %q, want config file value %q", cfg.Model, "file-model")
	}
}

func TestLoad_ConfigFilePartialOverride(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("PIGO_HOME", tmpDir)

	t.Setenv("OPENAI_API_KEY", "env-key")
	t.Setenv("PIGO_MODEL", "env-model")
	t.Setenv("OPENAI_BASE_URL", "https://env.example.com")
	t.Setenv("OPENAI_API_TYPE", "")
	t.Setenv("PIGO_EMBED_MODEL", "")

	// Config file only sets model, rest should come from env/defaults
	fileConfig := fileConfig{
		Model: "file-model",
	}
	data, _ := json.Marshal(fileConfig)
	os.WriteFile(filepath.Join(tmpDir, "config.json"), data, 0600)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.APIKey != "env-key" {
		t.Errorf("APIKey = %q, want env value %q", cfg.APIKey, "env-key")
	}
	if cfg.Model != "file-model" {
		t.Errorf("Model = %q, want config file value %q", cfg.Model, "file-model")
	}
	if cfg.BaseURL != "https://env.example.com" {
		t.Errorf("BaseURL = %q, want env value %q", cfg.BaseURL, "https://env.example.com")
	}
}

func TestLoad_MissingConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("PIGO_HOME", tmpDir)
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("PIGO_MODEL", "")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("OPENAI_API_TYPE", "")
	t.Setenv("PIGO_EMBED_MODEL", "")

	// No config file exists — should succeed with env/defaults
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.APIKey != "test-key" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "test-key")
	}
}

func TestLoad_MalformedConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("PIGO_HOME", tmpDir)
	t.Setenv("OPENAI_API_KEY", "test-key")

	os.WriteFile(filepath.Join(tmpDir, "config.json"), []byte("{invalid json"), 0600)

	_, err := Load()
	if err == nil {
		t.Error("expected error for malformed config file")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./pkg/config/ -v`
Expected: FAIL (package doesn't exist yet)

**Step 3: Write the implementation**

In `pkg/config/config.go`:

```go
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

// fileConfig is the JSON structure of ~/.pigo/config.json.
// All fields are pointers so we can distinguish "not set" from "set to empty".
type fileConfig struct {
	APIKey     string `json:"api_key,omitempty"`
	BaseURL    string `json:"base_url,omitempty"`
	Model      string `json:"model,omitempty"`
	APIType    string `json:"api_type,omitempty"`
	EmbedModel string `json:"embed_model,omitempty"`
}

// Load loads configuration by merging config file, env vars, and defaults.
// Priority: config file > env var > hardcoded default.
func Load() (*Config, error) {
	fc, err := readConfigFile()
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		APIKey:     resolve(fc.APIKey, os.Getenv("OPENAI_API_KEY"), ""),
		BaseURL:    resolve(fc.BaseURL, os.Getenv("OPENAI_BASE_URL"), ""),
		Model:      resolve(fc.Model, os.Getenv("PIGO_MODEL"), "gpt-4o"),
		APIType:    resolve(fc.APIType, os.Getenv("OPENAI_API_TYPE"), "chat"),
		EmbedModel: resolve(fc.EmbedModel, os.Getenv("PIGO_EMBED_MODEL"), "text-embedding-3-small"),
	}

	if cfg.APIKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY is required (set in config file or environment)")
	}

	return cfg, nil
}

// configDir returns the pigo home directory, respecting PIGO_HOME env var.
func configDir() string {
	if dir := os.Getenv("PIGO_HOME"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".pigo")
}

// readConfigFile reads and parses ~/.pigo/config.json.
// Returns zero-value fileConfig if file doesn't exist.
func readConfigFile() (fileConfig, error) {
	dir := configDir()
	if dir == "" {
		return fileConfig{}, nil
	}

	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return fileConfig{}, nil
		}
		return fileConfig{}, fmt.Errorf("reading config file: %w", err)
	}

	var fc fileConfig
	if err := json.Unmarshal(data, &fc); err != nil {
		return fileConfig{}, fmt.Errorf("parsing config file: %w", err)
	}

	return fc, nil
}

// resolve returns the first non-empty value from file, env, or default.
func resolve(fileVal, envVal, defaultVal string) string {
	if fileVal != "" {
		return fileVal
	}
	if envVal != "" {
		return envVal
	}
	return defaultVal
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./pkg/config/ -v`
Expected: PASS (all 7 tests)

**Step 5: Commit**

```bash
git add pkg/config/config.go pkg/config/config_test.go
git commit -m "feat: add pkg/config with JSON config file support"
```

---

### Task 2: Migrate `pkg/agent/` to use `pkg/config/`

**Files:**
- Modify: `pkg/agent/agent.go:22-43` (remove Config struct and LoadConfig)
- Modify: `pkg/agent/agent.go:60` (change NewAgent signature)
- Modify: `pkg/agent/agent_test.go` (update all references to Config)

**Step 1: Update `pkg/agent/agent.go`**

Remove the `Config` struct (lines 22-28) and `LoadConfig` function (lines 30-43).

Change the import to include `"github.com/user/pigo/pkg/config"`.

Change `NewAgent(cfg *Config)` to `NewAgent(cfg *config.Config)`.

Update the `llm.NewClient` call at line 61 to pass `cfg.EmbedModel`:

```go
client := llm.NewClient(cfg.APIKey, cfg.BaseURL, cfg.Model, cfg.APIType, cfg.EmbedModel)
```

**Step 2: Update `pkg/agent/agent_test.go`**

Replace all `&Config{...}` with `&config.Config{...}`.

Remove `TestLoadConfig` (it's now in `pkg/config/config_test.go`).

Add import `"github.com/user/pigo/pkg/config"`.

**Step 3: Run tests**

Run: `go test ./pkg/agent/ -v`
Expected: PASS

**Step 4: Commit**

```bash
git add pkg/agent/agent.go pkg/agent/agent_test.go
git commit -m "refactor: migrate agent package to use pkg/config"
```

---

### Task 3: Thread `EmbedModel` through `pkg/llm/` client

**Files:**
- Modify: `pkg/llm/client.go:22,28,567-574` (add embedModel field, update NewClient, update Embed)
- Modify: `pkg/llm/client_test.go` (update NewClient calls)

**Step 1: Update `pkg/llm/client.go`**

Add `embedModel` field to Client struct:

```go
type Client struct {
	client     openai.Client
	model      string
	apiType    string
	embedModel string
}
```

Update `NewClient` to accept `embedModel`:

```go
func NewClient(apiKey, baseURL, model, apiType, embedModel string) *Client {
	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
		option.WithMaxRetries(3),
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}

	return &Client{
		client:     openai.NewClient(opts...),
		model:      model,
		apiType:    apiType,
		embedModel: embedModel,
	}
}
```

Update `Embed` to use the field instead of `os.Getenv`:

```go
func (c *Client) Embed(ctx context.Context, text string) ([]float64, error) {
	if text == "" {
		return nil, fmt.Errorf("cannot embed empty text")
	}

	embedModel := c.embedModel
	if embedModel == "" {
		embedModel = string(openai.EmbeddingModelTextEmbedding3Small)
	}

	// ... rest unchanged
}
```

Remove the `util` import from `client.go` if it's no longer needed.

**Step 2: Update all `NewClient` calls in tests**

Search `pkg/llm/` test files for `NewClient(` and add the 5th argument `""` (or a test embed model).

**Step 3: Run tests**

Run: `go test ./pkg/llm/ -v`
Expected: PASS

**Step 4: Commit**

```bash
git add pkg/llm/client.go pkg/llm/client_test.go
git commit -m "refactor: thread embed model through LLM client"
```

---

### Task 4: Update `cmd/pigo/main.go` to use `config.Load()`

**Files:**
- Modify: `cmd/pigo/main.go:16-17,40-41`

**Step 1: Update imports**

Replace `"github.com/user/pigo/pkg/agent"` import usage for config:

```go
import (
	...
	"github.com/user/pigo/pkg/agent"
	"github.com/user/pigo/pkg/config"
	...
)
```

**Step 2: Replace `agent.LoadConfig()` with `config.Load()`**

Change line 40:

```go
cfg, err := config.Load()
```

**Step 3: Build and verify**

Run: `make build`
Expected: builds successfully

**Step 4: Commit**

```bash
git add cmd/pigo/main.go
git commit -m "refactor: use config.Load() in CLI entrypoint"
```

---

### Task 5: Update integration tests and run full test suite

**Files:**
- Modify: `pkg/agent/integration_test.go` (update Config references if any)

**Step 1: Check and update integration tests**

Search for `agent.Config` or `agent.LoadConfig` in integration tests and update to `config.Config`.

**Step 2: Run full test suite**

Run: `make test`
Expected: all tests pass (except the pre-existing `TestGrepTool_SkipsHiddenDirs` failure)

**Step 3: Run lint**

Run: `make lint`
Expected: PASS

**Step 4: Commit if any changes were needed**

```bash
git add -A && git commit -m "test: update integration tests for config package"
```

---

### Task 6: Update documentation

**Files:**
- Modify: `README.md` (add config file section)
- Modify: `CLAUDE.md` (add config package to architecture list)

**Step 1: Add config file section to README.md**

Add a "Configuration" section after the "Environment Variables" section documenting `~/.pigo/config.json` format and priority order.

**Step 2: Update CLAUDE.md architecture section**

Add `pkg/config/` to the package list.

**Step 3: Commit**

```bash
git add README.md CLAUDE.md
git commit -m "docs: add config file documentation"
```
