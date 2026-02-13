package main

import (
	"context"
	"fmt"
	"os"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
)

// Client wraps the OpenAI client for chat completions with tools.
type Client struct {
	client openai.Client
	model  string
}

// NewClient creates a new Client with the given configuration.
func NewClient(apiKey, baseURL, model string) *Client {
	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}

	return &Client{
		client: openai.NewClient(opts...),
		model:  model,
	}
}

// Message represents a chat message.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall represents a tool call from the model.
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// ChatResponse represents the response from a chat completion.
type ChatResponse struct {
	Content      string
	ToolCalls    []ToolCall
	FinishReason string
}

// Chat sends a chat completion request with tools.
func (c *Client) Chat(ctx context.Context, messages []Message, toolDefs []map[string]interface{}) (*ChatResponse, error) {
	// Convert messages to OpenAI format
	openaiMessages := make([]openai.ChatCompletionMessageParamUnion, len(messages))
	for i, msg := range messages {
		switch msg.Role {
		case "user":
			openaiMessages[i] = openai.UserMessage(msg.Content)
		case "assistant":
			if len(msg.ToolCalls) > 0 {
				// Assistant message with tool calls
				toolCalls := make([]openai.ChatCompletionMessageToolCallParam, len(msg.ToolCalls))
				for j, tc := range msg.ToolCalls {
					toolCalls[j] = openai.ChatCompletionMessageToolCallParam{
						ID:   tc.ID,
						Type: "function",
						Function: openai.ChatCompletionMessageToolCallFunctionParam{
							Name:      tc.Function.Name,
							Arguments: tc.Function.Arguments,
						},
					}
				}
				openaiMessages[i] = openai.ChatCompletionMessageParamUnion{
					OfAssistant: &openai.ChatCompletionAssistantMessageParam{
						ToolCalls: toolCalls,
					},
				}
			} else {
				openaiMessages[i] = openai.AssistantMessage(msg.Content)
			}
		case "tool":
			openaiMessages[i] = openai.ToolMessage(msg.Content, msg.ToolCallID)
		case "system":
			openaiMessages[i] = openai.SystemMessage(msg.Content)
		}
	}

	// Convert tool definitions to OpenAI format
	var tools []openai.ChatCompletionToolParam
	for _, def := range toolDefs {
		fn, ok := def["function"].(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := fn["name"].(string)
		desc, _ := fn["description"].(string)
		params, _ := fn["parameters"].(map[string]interface{})

		tools = append(tools, openai.ChatCompletionToolParam{
			Type: "function",
			Function: shared.FunctionDefinitionParam{
				Name:        name,
				Description: openai.String(desc),
				Parameters:  shared.FunctionParameters(params),
			},
		})
	}

	// Create request params
	params := openai.ChatCompletionNewParams{
		Model:    c.model,
		Messages: openaiMessages,
	}
	if len(tools) > 0 {
		params.Tools = tools
	}

	// Make the API call
	completion, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("chat completion failed: %w", err)
	}

	if len(completion.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	choice := completion.Choices[0]
	response := &ChatResponse{
		Content:      choice.Message.Content,
		FinishReason: string(choice.FinishReason),
	}

	// Convert tool calls
	for _, tc := range choice.Message.ToolCalls {
		response.ToolCalls = append(response.ToolCalls, ToolCall{
			ID:   tc.ID,
			Type: string(tc.Type),
			Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		})
	}

	return response, nil
}

// GetModel returns the model name.
func (c *Client) GetModel() string {
	return c.model
}

// GetEnvOrDefault returns environment variable or default value.
func GetEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
