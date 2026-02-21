package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/responses"
	"github.com/openai/openai-go/shared"

	"github.com/zhy0216/pigo/pkg/types"
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
		option.WithMaxRetries(3),
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

// Chat sends a chat request, dispatching to the appropriate API based on apiType.
func (c *Client) Chat(ctx context.Context, messages []types.Message, toolDefs []map[string]interface{}, opts ...types.ChatOption) (*types.ChatResponse, error) {
	cfg := types.ApplyChatOptions(opts)
	if c.apiType == "responses" {
		return c.chatViaResponses(ctx, messages, toolDefs, cfg)
	}
	return c.chatViaCompletions(ctx, messages, toolDefs, cfg)
}

// ChatStream sends a streaming chat request. Text deltas are written to w as they
// arrive. The complete ChatResponse (with any tool calls) is returned when done.
func (c *Client) ChatStream(ctx context.Context, messages []types.Message, toolDefs []map[string]interface{}, w io.Writer) (*types.ChatResponse, error) {
	if c.apiType == "responses" {
		return c.chatStreamViaResponses(ctx, messages, toolDefs, w)
	}
	return c.chatStreamViaCompletions(ctx, messages, toolDefs, w)
}

// chatViaCompletions sends a chat completion request using the Chat Completions API.
func (c *Client) chatViaCompletions(ctx context.Context, messages []types.Message, toolDefs []map[string]interface{}, cfg types.ChatConfig) (*types.ChatResponse, error) {
	openaiMessages := make([]openai.ChatCompletionMessageParamUnion, len(messages))
	for i, msg := range messages {
		switch msg.Role {
		case "user":
			openaiMessages[i] = openai.UserMessage(msg.Content)
		case "assistant":
			if len(msg.ToolCalls) > 0 {
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

	var tools []openai.ChatCompletionToolParam
	for _, def := range toolDefs {
		fn, ok := def["function"].(map[string]interface{})
		if !ok {
			fmt.Fprintf(os.Stderr, "warning: skipping malformed tool definition (missing 'function' key)\n")
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

	params := openai.ChatCompletionNewParams{
		Model:    c.model,
		Messages: openaiMessages,
	}
	if len(tools) > 0 {
		params.Tools = tools
	}
	if cfg.JSONMode {
		params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &shared.ResponseFormatJSONObjectParam{},
		}
	}

	completion, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, wrapAPIError("chat completion failed", err)
	}

	if len(completion.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	choice := completion.Choices[0]
	response := &types.ChatResponse{
		Content:      choice.Message.Content,
		FinishReason: string(choice.FinishReason),
		Usage: types.TokenUsage{
			PromptTokens:     completion.Usage.PromptTokens,
			CompletionTokens: completion.Usage.CompletionTokens,
			TotalTokens:      completion.Usage.TotalTokens,
		},
	}

	for _, tc := range choice.Message.ToolCalls {
		response.ToolCalls = append(response.ToolCalls, types.ToolCall{
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
func (c *Client) chatViaResponses(ctx context.Context, messages []types.Message, toolDefs []map[string]interface{}, cfg types.ChatConfig) (*types.ChatResponse, error) {
	var instructions string
	var inputMessages []types.Message
	for _, msg := range messages {
		if msg.Role == "system" {
			instructions = msg.Content
		} else {
			inputMessages = append(inputMessages, msg)
		}
	}

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

	var rtools []responses.ToolUnionParam
	for _, def := range toolDefs {
		fn, ok := def["function"].(map[string]interface{})
		if !ok {
			fmt.Fprintf(os.Stderr, "warning: skipping malformed tool definition (missing 'function' key)\n")
			continue
		}
		name, _ := fn["name"].(string)
		desc, _ := fn["description"].(string)
		params, _ := fn["parameters"].(map[string]interface{})

		rtools = append(rtools, responses.ToolUnionParam{
			OfFunction: &responses.FunctionToolParam{
				Name:        name,
				Description: openai.String(desc),
				Parameters:  params,
			},
		})
	}

	reqParams := responses.ResponseNewParams{
		Model: c.model,
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: input,
		},
	}
	if instructions != "" {
		reqParams.Instructions = openai.String(instructions)
	}
	if len(rtools) > 0 {
		reqParams.Tools = rtools
	}
	if cfg.JSONMode {
		reqParams.Text = responses.ResponseTextConfigParam{
			Format: responses.ResponseFormatTextConfigUnionParam{
				OfJSONObject: &shared.ResponseFormatJSONObjectParam{},
			},
		}
	}

	resp, err := c.client.Responses.New(ctx, reqParams)
	if err != nil {
		return nil, wrapAPIError("responses API call failed", err)
	}

	response := &types.ChatResponse{
		Usage: types.TokenUsage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}
	for _, item := range resp.Output {
		switch item.Type {
		case "message":
			for _, content := range item.Content {
				if content.Type == "output_text" {
					response.Content += content.Text
				}
			}
		case "function_call":
			response.ToolCalls = append(response.ToolCalls, types.ToolCall{
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

// chatStreamViaCompletions streams a chat completion using the accumulator.
func (c *Client) chatStreamViaCompletions(ctx context.Context, messages []types.Message, toolDefs []map[string]interface{}, w io.Writer) (*types.ChatResponse, error) {
	openaiMessages := make([]openai.ChatCompletionMessageParamUnion, len(messages))
	for i, msg := range messages {
		switch msg.Role {
		case "user":
			openaiMessages[i] = openai.UserMessage(msg.Content)
		case "assistant":
			if len(msg.ToolCalls) > 0 {
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

	var tools []openai.ChatCompletionToolParam
	for _, def := range toolDefs {
		fn, ok := def["function"].(map[string]interface{})
		if !ok {
			fmt.Fprintf(os.Stderr, "warning: skipping malformed tool definition (missing 'function' key)\n")
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

	params := openai.ChatCompletionNewParams{
		Model:    c.model,
		Messages: openaiMessages,
	}
	if len(tools) > 0 {
		params.Tools = tools
	}

	stream := c.client.Chat.Completions.NewStreaming(ctx, params)
	acc := openai.ChatCompletionAccumulator{}

	for stream.Next() {
		chunk := stream.Current()
		acc.AddChunk(chunk)

		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			fmt.Fprint(w, chunk.Choices[0].Delta.Content)
		}
	}

	if err := stream.Err(); err != nil {
		return nil, wrapAPIError("streaming chat completion failed", err)
	}

	if len(acc.Choices) == 0 {
		return nil, fmt.Errorf("no choices in streaming response")
	}

	choice := acc.Choices[0]
	response := &types.ChatResponse{
		Content:      choice.Message.Content,
		FinishReason: string(choice.FinishReason),
		Usage: types.TokenUsage{
			PromptTokens:     acc.Usage.PromptTokens,
			CompletionTokens: acc.Usage.CompletionTokens,
			TotalTokens:      acc.Usage.TotalTokens,
		},
	}

	for _, tc := range choice.Message.ToolCalls {
		response.ToolCalls = append(response.ToolCalls, types.ToolCall{
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

// chatStreamViaResponses streams a response using the Responses API.
func (c *Client) chatStreamViaResponses(ctx context.Context, messages []types.Message, toolDefs []map[string]interface{}, w io.Writer) (*types.ChatResponse, error) {
	var instructions string
	var inputMessages []types.Message
	for _, msg := range messages {
		if msg.Role == "system" {
			instructions = msg.Content
		} else {
			inputMessages = append(inputMessages, msg)
		}
	}

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

	var rtools []responses.ToolUnionParam
	for _, def := range toolDefs {
		fn, ok := def["function"].(map[string]interface{})
		if !ok {
			fmt.Fprintf(os.Stderr, "warning: skipping malformed tool definition (missing 'function' key)\n")
			continue
		}
		name, _ := fn["name"].(string)
		desc, _ := fn["description"].(string)
		params, _ := fn["parameters"].(map[string]interface{})
		rtools = append(rtools, responses.ToolUnionParam{
			OfFunction: &responses.FunctionToolParam{
				Name:        name,
				Description: openai.String(desc),
				Parameters:  params,
			},
		})
	}

	reqParams := responses.ResponseNewParams{
		Model: c.model,
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: input,
		},
	}
	if instructions != "" {
		reqParams.Instructions = openai.String(instructions)
	}
	if len(rtools) > 0 {
		reqParams.Tools = rtools
	}

	stream := c.client.Responses.NewStreaming(ctx, reqParams)

	type pendingCall struct {
		callID string
		name   string
		args   string
	}
	var pendingCalls []pendingCall
	var textContent string
	var usage types.TokenUsage

	for stream.Next() {
		event := stream.Current()

		switch event.Type {
		case "response.output_text.delta":
			delta := event.Delta.OfString
			if delta != "" {
				fmt.Fprint(w, delta)
				textContent += delta
			}
		case "response.output_item.added":
			if event.Item.Type == "function_call" {
				pendingCalls = append(pendingCalls, pendingCall{
					callID: event.Item.CallID,
					name:   event.Item.Name,
				})
			}
		case "response.function_call_arguments.delta":
			if len(pendingCalls) > 0 {
				pendingCalls[len(pendingCalls)-1].args += event.Delta.OfString
			}
		case "response.function_call_arguments.done":
			if len(pendingCalls) > 0 {
				pendingCalls[len(pendingCalls)-1].args = event.Arguments
			}
		case "response.completed":
			if event.Response.Usage.TotalTokens > 0 {
				usage = types.TokenUsage{
					PromptTokens:     event.Response.Usage.InputTokens,
					CompletionTokens: event.Response.Usage.OutputTokens,
					TotalTokens:      event.Response.Usage.TotalTokens,
				}
			}
		}
	}

	if err := stream.Err(); err != nil {
		return nil, wrapAPIError("streaming responses API call failed", err)
	}

	response := &types.ChatResponse{Content: textContent, Usage: usage}
	for _, pc := range pendingCalls {
		response.ToolCalls = append(response.ToolCalls, types.ToolCall{
			ID:   pc.callID,
			Type: "function",
			Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{
				Name:      pc.name,
				Arguments: pc.args,
			},
		})
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

// SetModel changes the model used for subsequent requests.
func (c *Client) SetModel(model string) {
	c.model = model
}

// wrapAPIError wraps an API error with context information, extracting HTTP
// status codes from openai.Error when available.
func wrapAPIError(context string, err error) error {
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		return fmt.Errorf("%s: HTTP %d: %w", context, apiErr.StatusCode, err)
	}
	return fmt.Errorf("%s: %w", context, err)
}

// IsContextOverflow checks if an error indicates that the request exceeded
// the model's context window. It looks for HTTP 400/413 status codes combined
// with known error message patterns from various providers.
func IsContextOverflow(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *openai.Error
	if !errors.As(err, &apiErr) {
		return false
	}
	if apiErr.StatusCode != 400 && apiErr.StatusCode != 413 {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "context length") ||
		strings.Contains(msg, "context window") ||
		strings.Contains(msg, "too many tokens") ||
		strings.Contains(msg, "token limit") ||
		strings.Contains(msg, "prompt is too long") ||
		strings.Contains(msg, "maximum prompt length") ||
		strings.Contains(msg, "reduce the length") ||
		strings.Contains(msg, "input token count") ||
		(strings.Contains(msg, "maximum") && strings.Contains(msg, "token"))
}
