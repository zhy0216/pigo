package main

import (
	"context"
	"fmt"
	"os"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/responses"
	"github.com/openai/openai-go/shared"
)

// Client wraps the OpenAI client for chat completions with tools.
type Client struct {
	client  openai.Client
	model   string
	apiType string // "chat" or "responses"
}

// NewClient creates a new Client with the given configuration.
func NewClient(apiKey, baseURL, model, apiType string) *Client {
	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}

	return &Client{
		client:  openai.NewClient(opts...),
		model:   model,
		apiType: apiType,
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

// Chat sends a chat request, dispatching to the appropriate API based on apiType.
func (c *Client) Chat(ctx context.Context, messages []Message, toolDefs []map[string]interface{}) (*ChatResponse, error) {
	if c.apiType == "responses" {
		return c.chatViaResponses(ctx, messages, toolDefs)
	}
	return c.chatViaCompletions(ctx, messages, toolDefs)
}

// chatViaCompletions sends a chat completion request using the Chat Completions API.
func (c *Client) chatViaCompletions(ctx context.Context, messages []Message, toolDefs []map[string]interface{}) (*ChatResponse, error) {
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

// chatViaResponses sends a chat request using the Responses API.
func (c *Client) chatViaResponses(ctx context.Context, messages []Message, toolDefs []map[string]interface{}) (*ChatResponse, error) {
	// Extract system message for instructions
	var instructions string
	var inputMessages []Message
	for _, msg := range messages {
		if msg.Role == "system" {
			instructions = msg.Content
		} else {
			inputMessages = append(inputMessages, msg)
		}
	}

	// Convert messages to Responses API input items
	var input responses.ResponseInputParam
	for _, msg := range inputMessages {
		switch msg.Role {
		case "user":
			input = append(input, responses.ResponseInputItemUnionParam{
				OfMessage: &responses.EasyInputMessageParam{
					Role: responses.EasyInputMessageRoleUser,
					Content: responses.EasyInputMessageContentUnionParam{
						OfString: openai.String(msg.Content),
					},
				},
			})
		case "assistant":
			if len(msg.ToolCalls) > 0 {
				for _, tc := range msg.ToolCalls {
					input = append(input, responses.ResponseInputItemUnionParam{
						OfFunctionCall: &responses.ResponseFunctionToolCallParam{
							CallID:    tc.ID,
							Name:      tc.Function.Name,
							Arguments: tc.Function.Arguments,
						},
					})
				}
			} else {
				input = append(input, responses.ResponseInputItemUnionParam{
					OfMessage: &responses.EasyInputMessageParam{
						Role: responses.EasyInputMessageRoleAssistant,
						Content: responses.EasyInputMessageContentUnionParam{
							OfString: openai.String(msg.Content),
						},
					},
				})
			}
		case "tool":
			input = append(input, responses.ResponseInputItemUnionParam{
				OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
					CallID: msg.ToolCallID,
					Output: msg.Content,
				},
			})
		}
	}

	// Convert tool definitions
	var tools []responses.ToolUnionParam
	for _, def := range toolDefs {
		fn, ok := def["function"].(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := fn["name"].(string)
		desc, _ := fn["description"].(string)
		params, _ := fn["parameters"].(map[string]interface{})

		tools = append(tools, responses.ToolUnionParam{
			OfFunction: &responses.FunctionToolParam{
				Name:        name,
				Description: openai.String(desc),
				Parameters:  params,
			},
		})
	}

	// Build request
	reqParams := responses.ResponseNewParams{
		Model: c.model,
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: input,
		},
	}
	if instructions != "" {
		reqParams.Instructions = openai.String(instructions)
	}
	if len(tools) > 0 {
		reqParams.Tools = tools
	}

	// Make the API call
	resp, err := c.client.Responses.New(ctx, reqParams)
	if err != nil {
		return nil, fmt.Errorf("responses API call failed: %w", err)
	}

	// Parse response output
	response := &ChatResponse{}
	for _, item := range resp.Output {
		switch item.Type {
		case "message":
			for _, content := range item.Content {
				if content.Type == "output_text" {
					response.Content += content.Text
				}
			}
		case "function_call":
			response.ToolCalls = append(response.ToolCalls, ToolCall{
				ID:   item.CallID,
				Type: "function",
				Function: struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}{
					Name:      item.Name,
					Arguments: item.Arguments,
				},
			})
		}
	}

	if len(response.ToolCalls) > 0 {
		response.FinishReason = "tool_calls"
	} else {
		response.FinishReason = "stop"
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
