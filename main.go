package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
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

func main() {
	// Load configuration from environment
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		fmt.Println("Error: OPENAI_API_KEY environment variable is required")
		os.Exit(1)
	}

	baseURL := os.Getenv("OPENAI_BASE_URL")
	model := GetEnvOrDefault("PIGO_MODEL", "gpt-4o")

	// Create client and tool registry
	client := NewClient(apiKey, baseURL, model)
	registry := NewToolRegistry()

	// Register tools
	registry.Register(NewReadTool())
	registry.Register(NewWriteTool())
	registry.Register(NewEditTool())
	registry.Register(NewBashTool())

	// Print welcome message
	fmt.Printf("%spigo%s - minimal AI coding assistant (model: %s)\n", colorGreen, colorReset, model)
	fmt.Printf("Tools: %s\n", strings.Join(registry.List(), ", "))
	fmt.Printf("Commands: /q (quit), /c (clear)\n\n")

	// Setup signal handling
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

	// Initialize conversation history
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

	// Main loop
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

		// Handle commands
		switch input {
		case "/q", "exit", "quit":
			fmt.Println("Goodbye!")
			return
		case "/c", "clear":
			messages = messages[:1] // Keep system message
			fmt.Println("Conversation cleared.")
			continue
		}

		// Add user message
		messages = append(messages, Message{
			Role:    "user",
			Content: input,
		})

		// Agent loop
		for iterations := 0; iterations < 10; iterations++ {
			response, err := client.Chat(ctx, messages, registry.GetDefinitions())
			if err != nil {
				fmt.Printf("%sError: %v%s\n", colorYellow, err, colorReset)
				break
			}

			// Handle tool calls
			if len(response.ToolCalls) > 0 {
				// Add assistant message with tool calls
				messages = append(messages, Message{
					Role:      "assistant",
					Content:   response.Content,
					ToolCalls: response.ToolCalls,
				})

				// Execute each tool call
				for _, tc := range response.ToolCalls {
					fmt.Printf("%s[%s]%s ", colorGray, tc.Function.Name, colorReset)

					// Parse arguments
					var args map[string]interface{}
					if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
						result := ErrorResult(fmt.Sprintf("failed to parse arguments: %v", err))
						messages = append(messages, Message{
							Role:       "tool",
							Content:    result.ForLLM,
							ToolCallID: tc.ID,
						})
						continue
					}

					// Execute tool
					result := registry.Execute(ctx, tc.Function.Name, args)

					// Show user-facing output
					if !result.Silent && result.ForUser != "" {
						fmt.Println()
						lines := strings.Split(result.ForUser, "\n")
						if len(lines) > 20 {
							for _, line := range lines[:20] {
								fmt.Println(line)
							}
							fmt.Printf("... (%d more lines)\n", len(lines)-20)
						} else {
							fmt.Println(result.ForUser)
						}
					}

					// Add tool result to messages
					messages = append(messages, Message{
						Role:       "tool",
						Content:    result.ForLLM,
						ToolCallID: tc.ID,
					})
				}
				fmt.Println()
				continue // Continue agent loop for more tool calls
			}

			// No tool calls - print response and break
			if response.Content != "" {
				fmt.Printf("\n%s\n\n", response.Content)
			}
			messages = append(messages, Message{
				Role:    "assistant",
				Content: response.Content,
			})
			break
		}
	}
}
