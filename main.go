package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
	"syscall"
)

const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorGray   = "\033[90m"
)

// Config holds the application configuration.
type Config struct {
	APIKey  string
	BaseURL string
	Model   string
	APIType string // "chat" or "responses"
}

// LoadConfig loads configuration from environment variables.
func LoadConfig() (*Config, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY environment variable is required")
	}

	return &Config{
		APIKey:  apiKey,
		BaseURL: os.Getenv("OPENAI_BASE_URL"),
		Model:   GetEnvOrDefault("PIGO_MODEL", "gpt-4o"),
		APIType: GetEnvOrDefault("OPENAI_API_TYPE", "chat"),
	}, nil
}

// App represents the pigo application.
type App struct {
	client   *Client
	registry *ToolRegistry
	messages []Message
	output   io.Writer
	skills   []Skill
}

// NewApp creates a new App instance.
func NewApp(cfg *Config) *App {
	client := NewClient(cfg.APIKey, cfg.BaseURL, cfg.Model, cfg.APIType)
	registry := NewToolRegistry()

	registry.Register(NewReadTool())
	registry.Register(NewWriteTool())
	registry.Register(NewEditTool())
	registry.Register(NewBashTool())

	// Load skills
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: cannot get working directory: %v\n", err)
	}
	skills, diags := LoadSkills(cwd)
	for _, d := range diags {
		fmt.Fprintf(os.Stderr, "skill warning: %s (%s)\n", d.Message, d.Path)
	}

	systemPrompt := `You are a helpful AI coding assistant. You have access to tools for reading, writing, and editing files, as well as executing bash commands.

When helping with coding tasks:
1. Read files before modifying them
2. Make targeted edits rather than rewriting entire files
3. Test your changes when possible
4. Be concise in your explanations`

	systemPrompt += formatSkillsForPrompt(skills)

	messages := []Message{
		{
			Role:    "system",
			Content: systemPrompt,
		},
	}

	return &App{
		client:   client,
		registry: registry,
		messages: messages,
		output:   os.Stdout,
		skills:   skills,
	}
}

// HandleCommand processes a command and returns true if handled.
func (a *App) HandleCommand(input string) (handled bool, exit bool) {
	switch input {
	case "/q", "exit", "quit":
		fmt.Fprintln(a.output, "Goodbye!")
		return true, true
	case "/c", "clear":
		a.messages = a.messages[:1]
		fmt.Fprintln(a.output, "Conversation cleared.")
		return true, false
	case "/skills":
		if len(a.skills) == 0 {
			fmt.Fprintln(a.output, "No skills loaded.")
		} else {
			fmt.Fprintln(a.output, "Loaded skills:")
			for _, s := range a.skills {
				fmt.Fprintf(a.output, "  /skill:%s - %s [%s] (%s)\n", s.Name, s.Description, s.Source, s.FilePath)
			}
		}
		return true, false
	}
	return false, false
}

// ProcessInput processes user input and runs the agent loop.
func (a *App) ProcessInput(ctx context.Context, input string) error {
	if input == "" {
		return nil
	}

	// Add user message
	a.messages = append(a.messages, Message{
		Role:    "user",
		Content: input,
	})

	// Agent loop
	maxIterations := 10
	completed := false
	for iterations := 0; iterations < maxIterations; iterations++ {
		response, err := a.client.Chat(ctx, a.messages, a.registry.GetDefinitions())
		if err != nil {
			return fmt.Errorf("chat error: %w", err)
		}

		// Handle tool calls
		if len(response.ToolCalls) > 0 {
			a.messages = append(a.messages, Message{
				Role:      "assistant",
				Content:   response.Content,
				ToolCalls: response.ToolCalls,
			})

			// Print tool names
			for _, tc := range response.ToolCalls {
				fmt.Fprintf(a.output, "%s[%s]%s ", colorGray, tc.Function.Name, colorReset)
			}

			// Execute tool calls concurrently
			type toolCallResult struct {
				msg    Message
				output string
			}
			results := make([]toolCallResult, len(response.ToolCalls))
			var wg sync.WaitGroup

			for i, tc := range response.ToolCalls {
				wg.Add(1)
				go func(i int, tc ToolCall) {
					defer wg.Done()

					var args map[string]interface{}
					if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
						result := ErrorResult(fmt.Sprintf("failed to parse arguments: %v", err))
						results[i] = toolCallResult{
							msg: Message{
								Role:       "tool",
								Content:    result.ForLLM,
								ToolCallID: tc.ID,
							},
						}
						return
					}

					result := a.registry.Execute(ctx, tc.Function.Name, args)

					var buf strings.Builder
					if !result.Silent && result.ForUser != "" {
						lines := strings.Split(result.ForUser, "\n")
						if len(lines) > 20 {
							for _, line := range lines[:20] {
								fmt.Fprintln(&buf, line)
							}
							fmt.Fprintf(&buf, "... (%d more lines)\n", len(lines)-20)
						} else {
							fmt.Fprintln(&buf, result.ForUser)
						}
					}

					results[i] = toolCallResult{
						msg: Message{
							Role:       "tool",
							Content:    result.ForLLM,
							ToolCallID: tc.ID,
						},
						output: buf.String(),
					}
				}(i, tc)
			}

			wg.Wait()

			// Print outputs and collect messages in order
			for _, r := range results {
				if r.output != "" {
					fmt.Fprintln(a.output)
					fmt.Fprint(a.output, r.output)
				}
				a.messages = append(a.messages, r.msg)
			}
			fmt.Fprintln(a.output)
			continue
		}

		// No tool calls - print response and break
		if response.Content != "" {
			fmt.Fprintf(a.output, "\n%s\n\n", response.Content)
		}
		a.messages = append(a.messages, Message{
			Role:    "assistant",
			Content: response.Content,
		})
		completed = true
		break
	}

	if !completed {
		return fmt.Errorf("agent loop reached maximum iterations (%d) without completing", maxIterations)
	}

	return nil
}

// GetRegistry returns the tool registry.
func (a *App) GetRegistry() *ToolRegistry {
	return a.registry
}

// GetModel returns the model name.
func (a *App) GetModel() string {
	return a.client.GetModel()
}

func main() {
	if memProfile := os.Getenv("PIGO_MEMPROFILE"); memProfile != "" {
		defer func() {
			runtime.GC()
			f, err := os.Create(memProfile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "could not create memory profile: %v\n", err)
				return
			}
			defer f.Close()
			if err := pprof.WriteHeapProfile(f); err != nil {
				fmt.Fprintf(os.Stderr, "could not write memory profile: %v\n", err)
			}
		}()
	}

	cfg, err := LoadConfig()
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	app := NewApp(cfg)

	fmt.Printf("%spigo%s - minimal AI coding assistant (model: %s, api: %s)\n", colorGreen, colorReset, app.GetModel(), cfg.APIType)
	fmt.Printf("Tools: %s\n", strings.Join(app.GetRegistry().List(), ", "))
	fmt.Printf("Commands: /q (quit), /c (clear), /skills\n")
	if len(app.skills) > 0 {
		var skillNames []string
		for _, s := range app.skills {
			skillNames = append(skillNames, "/skill:"+s.Name)
		}
		fmt.Printf("Skills: %s\n", strings.Join(skillNames, ", "))
	}
	fmt.Println()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nGoodbye!")
		cancel()
	}()

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("%s> %s", colorBlue, colorReset)
		input, err := reader.ReadString('\n')
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			break
		}
		input = strings.TrimSpace(input)

		if input == "" {
			continue
		}

		handled, exit := app.HandleCommand(input)
		if exit {
			return
		}
		if handled {
			continue
		}

		// Expand skill commands
		if expanded, ok := expandSkillCommand(input, app.skills); ok {
			// Extract skill name for indicator
			name := strings.TrimPrefix(input, "/skill:")
			if idx := strings.Index(name, " "); idx >= 0 {
				name = name[:idx]
			}
			fmt.Printf("%s[skill: %s]%s\n", colorGray, name, colorReset)
			input = expanded
		}

		if err := app.ProcessInput(ctx, input); err != nil {
			fmt.Printf("%sError: %v%s\n", colorYellow, err, colorReset)
		}
	}
}
