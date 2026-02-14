package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
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
}

// NewApp creates a new App instance.
func NewApp(cfg *Config) *App {
	client := NewClient(cfg.APIKey, cfg.BaseURL, cfg.Model, cfg.APIType)
	registry := NewToolRegistry()

	registry.Register(NewReadTool())
	registry.Register(NewWriteTool())
	registry.Register(NewEditTool())
	registry.Register(NewBashTool())

	messages := []Message{
		{
			Role: "system",
			Content: `You are a helpful AI coding assistant. You have access to tools for reading, writing, and editing files, as well as executing bash commands.

When helping with coding tasks:
1. Read files before modifying them
2. Make targeted edits rather than rewriting entire files
3. Test your changes when possible
4. Be concise in your explanations`,
		},
	}

	return &App{
		client:   client,
		registry: registry,
		messages: messages,
		output:   os.Stdout,
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
	for iterations := 0; iterations < 10; iterations++ {
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

			for _, tc := range response.ToolCalls {
				fmt.Fprintf(a.output, "%s[%s]%s ", colorGray, tc.Function.Name, colorReset)

				var args map[string]interface{}
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					result := ErrorResult(fmt.Sprintf("failed to parse arguments: %v", err))
					a.messages = append(a.messages, Message{
						Role:       "tool",
						Content:    result.ForLLM,
						ToolCallID: tc.ID,
					})
					continue
				}

				result := a.registry.Execute(ctx, tc.Function.Name, args)

				if !result.Silent && result.ForUser != "" {
					fmt.Fprintln(a.output)
					lines := strings.Split(result.ForUser, "\n")
					if len(lines) > 20 {
						for _, line := range lines[:20] {
							fmt.Fprintln(a.output, line)
						}
						fmt.Fprintf(a.output, "... (%d more lines)\n", len(lines)-20)
					} else {
						fmt.Fprintln(a.output, result.ForUser)
					}
				}

				a.messages = append(a.messages, Message{
					Role:       "tool",
					Content:    result.ForLLM,
					ToolCallID: tc.ID,
				})
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
		break
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
	cfg, err := LoadConfig()
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	app := NewApp(cfg)

	fmt.Printf("%spigo%s - minimal AI coding assistant (model: %s, api: %s)\n", colorGreen, colorReset, app.GetModel(), cfg.APIType)
	fmt.Printf("Tools: %s\n", strings.Join(app.GetRegistry().List(), ", "))
	fmt.Printf("Commands: /q (quit), /c (clear)\n\n")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nGoodbye!")
		cancel()
		os.Exit(0)
	}()

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("%s> %s", colorBlue, colorReset)
		input, err := reader.ReadString('\n')
		if err != nil {
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

		if err := app.ProcessInput(ctx, input); err != nil {
			fmt.Printf("%sError: %v%s\n", colorYellow, err, colorReset)
		}
	}
}
