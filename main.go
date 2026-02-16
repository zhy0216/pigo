package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
	"syscall"
	"time"
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
	events   *EventEmitter
}

// NewApp creates a new App instance.
func NewApp(cfg *Config) *App {
	client := NewClient(cfg.APIKey, cfg.BaseURL, cfg.Model, cfg.APIType)
	registry := NewToolRegistry()

	// Resolve the working directory for file tool boundary enforcement
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: cannot get working directory: %v\n", err)
	}
	allowedDir := ""
	if cwd != "" {
		resolved, err := filepath.EvalSymlinks(cwd)
		if err == nil {
			allowedDir = resolved
		} else {
			allowedDir = cwd
		}
	}

	registry.Register(NewReadTool(allowedDir))
	registry.Register(NewWriteTool(allowedDir))
	registry.Register(NewEditTool(allowedDir))
	registry.Register(NewBashTool())
	registry.Register(NewGrepTool(allowedDir))
	registry.Register(NewFindTool(allowedDir))

	// Load skills
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

	events := NewEventEmitter()

	app := &App{
		client:   client,
		registry: registry,
		messages: messages,
		output:   os.Stdout,
		skills:   skills,
		events:   events,
	}

	// Register default output subscriber
	events.Subscribe(app.defaultOutputHandler)

	return app
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

// truncateMessages trims the message history when it exceeds maxContextChars,
// keeping the system prompt (index 0) and the most recent minKeepMessages.
func (a *App) truncateMessages() {
	if len(a.messages) <= minKeepMessages+1 {
		return
	}

	total := estimateMessageChars(a.messages)
	if total <= maxContextChars {
		return
	}

	// Keep system prompt + truncation notice + last minKeepMessages
	truncated := len(a.messages) - 1 - minKeepMessages // messages being dropped
	if truncated <= 0 {
		return
	}

	kept := make([]Message, 0, minKeepMessages+2)
	kept = append(kept, a.messages[0]) // system prompt
	kept = append(kept, Message{
		Role:    "user",
		Content: fmt.Sprintf("[%d earlier messages truncated to save context]", truncated),
	})
	kept = append(kept, a.messages[len(a.messages)-minKeepMessages:]...)

	a.messages = kept
	fmt.Fprintf(a.output, "%s[context truncated: %d messages removed]%s\n", colorYellow, truncated, colorReset)
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

	// Manage context window
	a.compactMessages(ctx)

	a.events.Emit(AgentEvent{Type: EventAgentStart})

	// Agent loop
	maxIterations := 10
	completed := false
	var agentErr error
	for iterations := 0; iterations < maxIterations; iterations++ {
		a.events.Emit(AgentEvent{Type: EventTurnStart})

		response, err := a.client.ChatStream(ctx, a.messages, a.registry.GetDefinitions(), a.output)
		if err != nil {
			agentErr = fmt.Errorf("chat error: %w", err)
			a.events.Emit(AgentEvent{Type: EventTurnEnd, Error: agentErr})
			break
		}

		// Handle tool calls
		if len(response.ToolCalls) > 0 {
			a.messages = append(a.messages, Message{
				Role:      "assistant",
				Content:   response.Content,
				ToolCalls: response.ToolCalls,
			})

			// Execute tool calls sequentially with interrupt checks
			for i, tc := range response.ToolCalls {
				// Check for interruption between tool calls
				if ctx.Err() != nil {
					for j := i; j < len(response.ToolCalls); j++ {
						a.messages = append(a.messages, Message{
							Role:       "tool",
							Content:    "Skipped due to user interrupt",
							ToolCallID: response.ToolCalls[j].ID,
						})
					}
					break
				}

				a.events.Emit(AgentEvent{Type: EventToolStart, ToolName: tc.Function.Name})

				var args map[string]interface{}
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					result := ErrorResult(fmt.Sprintf("failed to parse arguments: %v", err))
					a.messages = append(a.messages, Message{
						Role:       "tool",
						Content:    result.ForLLM,
						ToolCallID: tc.ID,
					})
					a.events.Emit(AgentEvent{Type: EventToolEnd, ToolName: tc.Function.Name})
					continue
				}

				result := a.registry.Execute(ctx, tc.Function.Name, args)

				var output string
				if !result.Silent && result.ForUser != "" {
					lines := strings.Split(result.ForUser, "\n")
					var buf strings.Builder
					if len(lines) > 20 {
						for _, line := range lines[:20] {
							fmt.Fprintln(&buf, line)
						}
						fmt.Fprintf(&buf, "... (%d more lines)\n", len(lines)-20)
					} else {
						fmt.Fprintln(&buf, result.ForUser)
					}
					output = buf.String()
				}

				a.messages = append(a.messages, Message{
					Role:       "tool",
					Content:    result.ForLLM,
					ToolCallID: tc.ID,
				})
				a.events.Emit(AgentEvent{Type: EventToolEnd, ToolName: tc.Function.Name, Content: output})
			}

			a.events.Emit(AgentEvent{Type: EventTurnEnd})
			continue
		}

		// No tool calls - text was already streamed
		a.messages = append(a.messages, Message{
			Role:    "assistant",
			Content: response.Content,
		})
		a.events.Emit(AgentEvent{Type: EventMessageEnd, Content: response.Content})
		a.events.Emit(AgentEvent{Type: EventTurnEnd})
		completed = true
		break
	}

	if !completed && agentErr == nil {
		agentErr = fmt.Errorf("agent loop reached maximum iterations (%d) without completing", maxIterations)
	}

	a.events.Emit(AgentEvent{Type: EventAgentEnd, Error: agentErr})
	return agentErr
}

// defaultOutputHandler is the default event subscriber that handles console output.
func (a *App) defaultOutputHandler(event AgentEvent) {
	switch event.Type {
	case EventToolStart:
		fmt.Fprintf(a.output, "%s[%s]%s ", colorGray, event.ToolName, colorReset)
	case EventToolEnd:
		if event.Content != "" {
			fmt.Fprintln(a.output)
			fmt.Fprint(a.output, event.Content)
		}
	case EventTurnEnd:
		fmt.Fprintln(a.output)
	case EventMessageEnd:
		if event.Content != "" {
			fmt.Fprintf(a.output, "\n\n")
		}
	}
}

// GetRegistry returns the tool registry.
func (a *App) GetRegistry() *ToolRegistry {
	return a.registry
}

// Events returns the event emitter.
func (a *App) Events() *EventEmitter {
	return a.events
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

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Per-turn cancellation: first Ctrl+C cancels the current turn,
	// second Ctrl+C within 1 second exits the process.
	var turnCancel context.CancelFunc
	var lastSigTime time.Time
	var mu sync.Mutex

	go func() {
		for sig := range sigChan {
			if sig == syscall.SIGTERM {
				fmt.Println("\nGoodbye!")
				os.Exit(0)
			}
			mu.Lock()
			now := time.Now()
			if now.Sub(lastSigTime) < time.Second {
				mu.Unlock()
				fmt.Println("\nGoodbye!")
				os.Exit(0)
			}
			lastSigTime = now
			if turnCancel != nil {
				turnCancel()
				turnCancel = nil
				fmt.Fprintf(os.Stderr, "\n%s[interrupted]%s\n", colorYellow, colorReset)
			}
			mu.Unlock()
		}
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

		// Create per-turn context
		turnCtx, cancel := context.WithCancel(context.Background())
		mu.Lock()
		turnCancel = cancel
		mu.Unlock()

		err = app.ProcessInput(turnCtx, input)
		cancel()

		mu.Lock()
		turnCancel = nil
		mu.Unlock()

		if err != nil {
			if turnCtx.Err() == context.Canceled {
				// Turn was interrupted, return to prompt
				continue
			}
			fmt.Printf("%sError: %v%s\n", colorYellow, err, colorReset)
		}
	}
}
