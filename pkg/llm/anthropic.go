package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/zhy0216/pigo/pkg/types"
)

// AnthropicProvider wraps the Anthropic client for chat completions with tools.
type AnthropicProvider struct {
	client anthropic.Client
	model  string
}

// NewAnthropicProvider creates a new AnthropicProvider.
func NewAnthropicProvider(apiKey, baseURL, model string) *AnthropicProvider {
	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
		option.WithMaxRetries(3),
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	client := anthropic.NewClient(opts...)
	return &AnthropicProvider{
		client: client,
		model:  model,
	}
}

// GetModel returns the model name.
func (a *AnthropicProvider) GetModel() string {
	return a.model
}

// SetModel changes the model used for subsequent requests.
func (a *AnthropicProvider) SetModel(model string) {
	a.model = model
}

func (a *AnthropicProvider) buildRequest(messages []types.Message, toolDefs []map[string]interface{}, cfg types.ChatConfig) (anthropic.MessageNewParams, error) {
	var systemBlocks []anthropic.TextBlockParam
	var anthropicMessages []anthropic.MessageParam

	for _, msg := range messages {
		switch msg.Role {
		case "system":
			systemBlocks = append(systemBlocks, *anthropic.NewTextBlock(msg.Content).OfText)
		case "user":
			anthropicMessages = append(anthropicMessages, anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Content)))
		case "assistant":
			var blocks []anthropic.ContentBlockParamUnion
			if msg.Content != "" {
				blocks = append(blocks, anthropic.NewTextBlock(msg.Content))
			}
			for _, tc := range msg.ToolCalls {
				var input map[string]interface{}
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
					// Fallback if args isn't valid JSON, though it should be
					input = map[string]interface{}{}
				}
				blocks = append(blocks, anthropic.NewToolUseBlock(tc.ID, input, tc.Function.Name))
			}
			anthropicMessages = append(anthropicMessages, anthropic.NewAssistantMessage(blocks...))
		case "tool":
			anthropicMessages = append(anthropicMessages, anthropic.MessageParam{
				Role: anthropic.MessageParamRoleUser,
				Content: []anthropic.ContentBlockParamUnion{
					anthropic.NewToolResultBlock(msg.ToolCallID, msg.Content, false),
				},
			})
		}
	}

	// Anthropic doesn't have a native response_format like OpenAI.
	// Enforce JSON output via a system prompt instruction when requested.
	if cfg.JSONSchema != nil {
		schemaBytes, err := json.Marshal(cfg.JSONSchema.Schema)
		if err == nil {
			systemBlocks = append(systemBlocks, *anthropic.NewTextBlock(
				fmt.Sprintf("You must respond with a JSON object conforming to this schema: %s\nDo not include any text outside the JSON object.", string(schemaBytes)),
			).OfText)
		}
	} else if cfg.JSONMode {
		systemBlocks = append(systemBlocks, *anthropic.NewTextBlock(
			"You must respond with a valid JSON object. Do not include any text outside the JSON object.",
		).OfText)
	}

	// Add caching breakpoint to the last system prompt block if it exists
	if len(systemBlocks) > 0 {
		systemBlocks[len(systemBlocks)-1].CacheControl = anthropic.NewCacheControlEphemeralParam()
	}

	// Add caching breakpoint to the 2nd to last and 4th to last message if they exist
	// This follows Anthropic's recommendation for multi-turn conversations
	msgLen := len(anthropicMessages)
	if msgLen >= 2 {
		for i := len(anthropicMessages[msgLen-2].Content) - 1; i >= 0; i-- {
			block := &anthropicMessages[msgLen-2].Content[i]
			if block.OfText != nil {
				block.OfText.CacheControl = anthropic.NewCacheControlEphemeralParam()
				break
			}
		}
	}
	if msgLen >= 4 {
		for i := len(anthropicMessages[msgLen-4].Content) - 1; i >= 0; i-- {
			block := &anthropicMessages[msgLen-4].Content[i]
			if block.OfText != nil {
				block.OfText.CacheControl = anthropic.NewCacheControlEphemeralParam()
				break
			}
		}
	}

	var tools []anthropic.ToolUnionParam
	for _, def := range toolDefs {
		fn, ok := def["function"].(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := fn["name"].(string)
		desc, _ := fn["description"].(string)
		params, _ := fn["parameters"].(map[string]interface{})

		tools = append(tools, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        name,
				Description: anthropic.String(desc),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: params["properties"],
					Required:   extractRequired(params["required"]),
				},
			},
		})
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(a.model),
		MaxTokens: int64(8192), // Claude 3.5 max tokens
		Messages:  anthropicMessages,
	}

	if len(systemBlocks) > 0 {
		params.System = systemBlocks
	}

	if len(tools) > 0 {
		params.Tools = tools
	}

	return params, nil
}

// Chat sends a chat request.
func (a *AnthropicProvider) Chat(ctx context.Context, messages []types.Message, toolDefs []map[string]interface{}, opts ...types.ChatOption) (*types.ChatResponse, error) {
	cfg := types.ApplyChatOptions(opts)
	params, err := a.buildRequest(messages, toolDefs, cfg)
	if err != nil {
		return nil, err
	}

	msg, err := a.client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("anthropic chat failed: %w", err)
	}

	response := &types.ChatResponse{
		FinishReason: string(msg.StopReason),
		Usage: types.TokenUsage{
			PromptTokens:     msg.Usage.InputTokens,
			CompletionTokens: msg.Usage.OutputTokens,
			TotalTokens:      msg.Usage.InputTokens + msg.Usage.OutputTokens,
		},
	}

	for _, block := range msg.Content {
		if block.Type == "tool_use" {
			args, _ := json.Marshal(block.Input)
			response.ToolCalls = append(response.ToolCalls, types.ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}{
					Name:      block.Name,
					Arguments: string(args),
				},
			})
		} else if block.Type == "text" {
			response.Content += block.Text
		}
	}

	return response, nil
}

// ChatStream sends a streaming chat request.
func (a *AnthropicProvider) ChatStream(ctx context.Context, messages []types.Message, toolDefs []map[string]interface{}, w io.Writer) (*types.ChatResponse, error) {
	params, err := a.buildRequest(messages, toolDefs, types.ChatConfig{})
	if err != nil {
		return nil, err
	}

	stream := a.client.Messages.NewStreaming(ctx, params)
	var textContent string
	var usage types.TokenUsage
	var finishReason string

	type pendingCall struct {
		callID string
		name   string
		args   strings.Builder
	}
	var currentCall *pendingCall
	var pendingCalls []pendingCall

	for stream.Next() {
		event := stream.Current()
		switch variant := event.AsAny().(type) {
		case anthropic.MessageStartEvent:
			usage.PromptTokens = variant.Message.Usage.InputTokens
		case anthropic.ContentBlockStartEvent:
			if variant.ContentBlock.Type == "tool_use" {
				toolUse := variant.ContentBlock.AsToolUse()
				currentCall = &pendingCall{
					callID: toolUse.ID,
					name:   toolUse.Name,
				}
			}
		case anthropic.ContentBlockDeltaEvent:
			if variant.Delta.Type == "text_delta" {
				textDelta := variant.Delta.AsTextDelta()
				fmt.Fprint(w, textDelta.Text)
				textContent += textDelta.Text
			} else if variant.Delta.Type == "input_json_delta" {
				jsonDelta := variant.Delta.AsInputJSONDelta()
				if currentCall != nil {
					currentCall.args.WriteString(jsonDelta.PartialJSON)
				}
			}
		case anthropic.ContentBlockStopEvent:
			if currentCall != nil {
				pendingCalls = append(pendingCalls, *currentCall)
				currentCall = nil
			}
		case anthropic.MessageDeltaEvent:
			usage.CompletionTokens = variant.Usage.OutputTokens
			finishReason = string(variant.Delta.StopReason)
		}
	}

	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("anthropic stream failed: %w", err)
	}

	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens

	response := &types.ChatResponse{
		Content:      textContent,
		Usage:        usage,
		FinishReason: finishReason,
	}

	for _, pc := range pendingCalls {
		response.ToolCalls = append(response.ToolCalls, types.ToolCall{
			ID:   pc.callID,
			Type: "function",
			Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{
				Name:      pc.name,
				Arguments: pc.args.String(),
			},
		})
	}

	return response, nil
}

// extractRequired safely extracts a []string from a required field value,
// handling both []string (from Go tool definitions) and []interface{} (from JSON).
func extractRequired(v interface{}) []string {
	switch req := v.(type) {
	case []string:
		return req
	case []interface{}:
		var res []string
		for _, r := range req {
			if s, ok := r.(string); ok {
				res = append(res, s)
			}
		}
		return res
	default:
		return nil
	}
}
