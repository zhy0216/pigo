package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/zhy0216/pigo/pkg/config"
	"github.com/zhy0216/pigo/pkg/hooks"
	"github.com/zhy0216/pigo/pkg/llm"
	"github.com/zhy0216/pigo/pkg/ops"
	"github.com/zhy0216/pigo/pkg/skills"
	"github.com/zhy0216/pigo/pkg/tools"
	"github.com/zhy0216/pigo/pkg/types"
	"github.com/zhy0216/pigo/pkg/util"
)

// Agent represents the pigo application.
type Agent struct {
	client        llm.Provider
	registry      *tools.ToolRegistry
	messages      []types.Message
	output        io.Writer
	Skills        []skills.Skill
	visibleSkills []skills.Skill // skills eligible for pre-flight matching
	events        *types.EventEmitter
	hookMgr       *hooks.HookManager
	usage         types.TokenUsage // accumulated session usage
}

// NewAgent creates a new Agent instance.
func NewAgent(cfg *config.Config) *Agent {
	var provider llm.Provider
	switch cfg.Provider {
	case "anthropic":
		provider = llm.NewAnthropicProvider(cfg.APIKey, cfg.BaseURL, cfg.Model)
	default:
		provider = llm.NewOpenAIProvider(cfg.APIKey, cfg.BaseURL, cfg.Model, cfg.APIType)
	}
	registry := tools.NewToolRegistry()

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

	registry.Register(tools.NewReadTool(allowedDir, &ops.RealFileOps{}))
	registry.Register(tools.NewWriteTool(allowedDir, &ops.RealFileOps{}))
	registry.Register(tools.NewEditTool(allowedDir, &ops.RealFileOps{}))
	registry.Register(tools.NewBashTool(&ops.RealExecOps{}))
	registry.Register(tools.NewGrepTool(allowedDir, &ops.RealFileOps{}, &ops.RealExecOps{}))
	registry.Register(tools.NewFindTool(allowedDir, &ops.RealFileOps{}, &ops.RealExecOps{}))
	registry.Register(tools.NewLsTool(allowedDir, &ops.RealFileOps{}))

	loadedSkills, diags := skills.LoadSkills(cwd)
	for _, d := range diags {
		fmt.Fprintf(os.Stderr, "skill warning: %s (%s)\n", d.Message, d.Path)
	}

	registry.Register(tools.NewUseSkillTool(loadedSkills))

	var visibleSkills []skills.Skill
	for _, s := range loadedSkills {
		if !s.DisableModelInvocation {
			visibleSkills = append(visibleSkills, s)
		}
	}

	systemPrompt := cfg.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = `You are a helpful AI coding assistant. You have access to tools for reading, writing, and editing files, as well as executing bash commands.

When helping with coding tasks:
1. Read files before modifying them
2. Make targeted edits rather than rewriting entire files
3. Test your changes when possible
4. Be concise in your explanations`
	}

	if cwd != "" {
		systemPrompt += fmt.Sprintf("\n\nCurrent working directory: %s", cwd)
	}

	systemPrompt += skills.FormatSkillsForPrompt(loadedSkills)

	messages := []types.Message{
		{
			Role:    "system",
			Content: systemPrompt,
		},
	}

	events := types.NewEventEmitter()
	hookMgr := hooks.NewHookManager(cfg.Plugins)

	agent := &Agent{
		client:        provider,
		registry:      registry,
		messages:      messages,
		output:        os.Stdout,
		Skills:        loadedSkills,
		visibleSkills: visibleSkills,
		events:        events,
		hookMgr:       hookMgr,
	}

	events.Subscribe(agent.defaultOutputHandler)

	return agent
}

// HandleCommand processes a command and returns true if handled.
func (a *Agent) HandleCommand(input string) (handled bool, exit bool) {
	switch input {
	case "/q", "exit", "quit":
		u := a.GetUsage()
		if u.TotalTokens > 0 {
			fmt.Fprintf(a.output, "%sSession usage: %d prompt + %d completion = %d total tokens%s\n",
				types.ColorGray, u.PromptTokens, u.CompletionTokens, u.TotalTokens, types.ColorReset)
		}
		fmt.Fprintln(a.output, "Goodbye!")
		return true, true
	case "/c", "clear":
		a.messages = a.messages[:1]
		fmt.Fprintln(a.output, "Conversation cleared.")
		return true, false
	case "/usage":
		u := a.GetUsage()
		if u.TotalTokens == 0 {
			fmt.Fprintln(a.output, "No tokens used yet.")
		} else {
			fmt.Fprintf(a.output, "Session usage:\n  Prompt tokens:     %d\n  Completion tokens: %d\n  Total tokens:      %d\n",
				u.PromptTokens, u.CompletionTokens, u.TotalTokens)
		}
		return true, false
	case "/skills":
		if len(a.Skills) == 0 {
			fmt.Fprintln(a.output, "No skills loaded.")
		} else {
			fmt.Fprintln(a.output, "Loaded skills:")
			for _, s := range a.Skills {
				fmt.Fprintf(a.output, "  /skill:%s - %s [%s] (%s)\n", s.Name, s.Description, s.Source, s.FilePath)
			}
		}
		return true, false
	case "/plugins":
		plugins := a.hookMgr.GetPlugins()
		if len(plugins) == 0 {
			fmt.Fprintln(a.output, "No plugins loaded.")
		} else {
			fmt.Fprintln(a.output, "Loaded plugins:")
			for _, p := range plugins {
				hookCount := 0
				for _, hs := range p.Hooks {
					hookCount += len(hs)
				}
				fmt.Fprintf(a.output, "  %s (%d hooks)\n", p.Name, hookCount)
			}
		}
		return true, false
	}

	// Commands with arguments
	if input == "/model" || strings.HasPrefix(input, "/model ") {
		newModel := strings.TrimSpace(strings.TrimPrefix(input, "/model"))
		if newModel == "" {
			fmt.Fprintf(a.output, "Current model: %s\n", a.client.GetModel())
		} else {
			a.client.SetModel(newModel)
			fmt.Fprintf(a.output, "Model changed to: %s\n", newModel)
		}
		return true, false
	}

	return false, false
}

// truncateMessages trims the message history when it exceeds maxContextChars.
func (a *Agent) truncateMessages() {
	if len(a.messages) <= types.MinKeepMessages+1 {
		return
	}

	total := util.EstimateMessageChars(a.messages)
	if total <= types.MaxContextChars {
		return
	}

	truncated := len(a.messages) - 1 - types.MinKeepMessages
	if truncated <= 0 {
		return
	}

	kept := make([]types.Message, 0, types.MinKeepMessages+2)
	kept = append(kept, a.messages[0])
	kept = append(kept, types.Message{
		Role:    "user",
		Content: fmt.Sprintf("[%d earlier messages truncated to save context]", truncated),
	})
	kept = append(kept, a.messages[len(a.messages)-types.MinKeepMessages:]...)

	a.messages = kept
	fmt.Fprintf(a.output, "%s[context truncated: %d messages removed]%s\n", types.ColorYellow, truncated, types.ColorReset)
}

// ProcessInput processes user input and runs the agent loop.
func (a *Agent) ProcessInput(ctx context.Context, input string) error {
	if input == "" {
		return nil
	}

	a.messages = append(a.messages, types.Message{
		Role:    "user",
		Content: input,
	})

	// Pre-flight skill matching
	if len(a.visibleSkills) > 0 {
		result := skills.MatchSkills(ctx, a.client, input, a.visibleSkills)
		if types.Debug {
			if result.Err != nil {
				fmt.Fprintf(a.output, "%s[skill match: error: %v, raw: %q]%s\n", types.ColorYellow, result.Err, result.RawResponse, types.ColorReset)
			} else {
				fmt.Fprintf(a.output, "%s[skill match: %s]%s\n", types.ColorGray, result.RawResponse, types.ColorReset)
			}
		}
		for _, name := range result.Names {
			for _, s := range a.visibleSkills {
				if s.Name == name {
					content, err := skills.LoadSkillContent(s)
					if err != nil {
						fmt.Fprintf(a.output, "%s[skill match warning: %v]%s\n", types.ColorYellow, err, types.ColorReset)
						continue
					}
					a.messages = append(a.messages, types.Message{
						Role:    "system",
						Content: content,
					})
					fmt.Fprintf(a.output, "%s[skill: %s]%s\n", types.ColorGray, name, types.ColorReset)
					break
				}
			}
		}
	}

	a.compactMessages(ctx)

	turnMessages := []types.Message{}
	if len(a.messages) > 0 {
		turnMessages = append(turnMessages, a.messages[len(a.messages)-1])
	}

	a.events.Emit(types.AgentEvent{Type: types.EventAgentStart})
	contexts, _ := a.hookMgr.Run(ctx, a.newHookContext("agent_start"))
	for _, c := range contexts {
		a.messages = append(a.messages, types.Message{
			Role:    "system",
			Content: fmt.Sprintf("[plugin:%s]\n%s", c.Plugin, c.Content),
		})
	}

	maxIterations := types.MaxAgentIterations
	completed := false
	var agentErr error
	for iterations := 0; iterations < maxIterations; iterations++ {
		a.events.Emit(types.AgentEvent{Type: types.EventTurnStart})
		hctxTurn := a.newHookContext("turn_start")
		hctxTurn.TurnNumber = iterations + 1
		turnContexts, _ := a.hookMgr.Run(ctx, hctxTurn)

		callMessages := a.messages
		if len(turnContexts) > 0 {
			callMessages = make([]types.Message, len(a.messages))
			copy(callMessages, a.messages)
			for _, c := range turnContexts {
				callMessages = append(callMessages, types.Message{
					Role:    "system",
					Content: fmt.Sprintf("[plugin:%s]\n%s", c.Plugin, c.Content),
				})
			}
		}

		var response *types.ChatResponse
		var chatErr error
		var savedMessages []types.Message
		for retries := 0; retries <= types.MaxOverflowRetries; retries++ {
			response, chatErr = a.client.ChatStream(ctx, callMessages, a.registry.GetDefinitions(), a.output)
			if chatErr == nil {
				break
			}
			if !llm.IsContextOverflow(chatErr) || retries == types.MaxOverflowRetries {
				if savedMessages != nil {
					a.messages = savedMessages
				}
				break
			}
			if savedMessages == nil {
				savedMessages = make([]types.Message, len(a.messages))
				copy(savedMessages, a.messages)
			}
			fmt.Fprintf(a.output, "%s[context overflow, compacting and retrying...]%s\n", types.ColorYellow, types.ColorReset)
			a.truncateMessages()
		}

		if chatErr != nil {
			agentErr = fmt.Errorf("chat error: %w", chatErr)
			a.events.Emit(types.AgentEvent{Type: types.EventTurnEnd, Error: agentErr})
			hctxTurnEnd := a.newHookContext("turn_end")
			hctxTurnEnd.TurnNumber = iterations + 1
			_, _ = a.hookMgr.Run(ctx, hctxTurnEnd)
			break
		}

		a.addUsage(response.Usage)

		if len(response.ToolCalls) > 0 {
			assistantMsg := types.Message{
				Role:      "assistant",
				Content:   response.Content,
				ToolCalls: response.ToolCalls,
			}
			a.messages = append(a.messages, assistantMsg)
			turnMessages = append(turnMessages, assistantMsg)

			for i, tc := range response.ToolCalls {
				if ctx.Err() != nil {
					for j := i; j < len(response.ToolCalls); j++ {
						skipMsg := types.Message{
							Role:       "tool",
							Content:    "Skipped due to user interrupt",
							ToolCallID: response.ToolCalls[j].ID,
						}
						a.messages = append(a.messages, skipMsg)
						turnMessages = append(turnMessages, skipMsg)
					}
					break
				}

				a.events.Emit(types.AgentEvent{Type: types.EventToolStart, ToolName: tc.Function.Name})

				// Run tool_start hooks — blocking hook failure cancels the tool
				toolStartCtx := a.newHookContext("tool_start")
				toolStartCtx.ToolName = tc.Function.Name
				toolStartCtx.ToolArgs = tc.Function.Arguments
				if input, ok := extractToolInput(tc.Function.Name, tc.Function.Arguments); ok {
					toolStartCtx.ToolInput = input
				}
				if _, hookErr := a.hookMgr.Run(ctx, toolStartCtx); hookErr != nil {
					result := types.ErrorResult(fmt.Sprintf("blocked by hook: %v", hookErr))
					blockedMsg := types.Message{
						Role:       "tool",
						Content:    result.ForLLM,
						ToolCallID: tc.ID,
					}
					a.messages = append(a.messages, blockedMsg)
					turnMessages = append(turnMessages, blockedMsg)
					a.events.Emit(types.AgentEvent{Type: types.EventToolEnd, ToolName: tc.Function.Name})
					continue
				}

				var args map[string]interface{}
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					result := types.ErrorResult(fmt.Sprintf("failed to parse arguments: %v", err))
					errMsg := types.Message{
						Role:       "tool",
						Content:    result.ForLLM,
						ToolCallID: tc.ID,
					}
					a.messages = append(a.messages, errMsg)
					turnMessages = append(turnMessages, errMsg)
					a.events.Emit(types.AgentEvent{Type: types.EventToolEnd, ToolName: tc.Function.Name})
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

				toolMsg := types.Message{
					Role:       "tool",
					Content:    result.ForLLM,
					ToolCallID: tc.ID,
				}
				a.messages = append(a.messages, toolMsg)
				turnMessages = append(turnMessages, toolMsg)
				a.events.Emit(types.AgentEvent{Type: types.EventToolEnd, ToolName: tc.Function.Name, Content: output})

				toolEndCtx := a.newHookContext("tool_end")
				toolEndCtx.ToolName = tc.Function.Name
				toolEndCtx.ToolArgs = tc.Function.Arguments
				toolOutput := result.ForLLM
				if len(toolOutput) > 10000 {
					toolOutput = toolOutput[len(toolOutput)-10000:]
				}
				toolEndCtx.ToolOutput = toolOutput
				toolEndCtx.ToolError = result.IsError
				_, _ = a.hookMgr.Run(ctx, toolEndCtx)
			}

			a.events.Emit(types.AgentEvent{Type: types.EventTurnEnd})
			hctxTurnEnd := a.newHookContext("turn_end")
			hctxTurnEnd.TurnNumber = iterations + 1
			_, _ = a.hookMgr.Run(ctx, hctxTurnEnd)
			continue
		}

		assistantMsg := types.Message{
			Role:    "assistant",
			Content: response.Content,
		}
		a.messages = append(a.messages, assistantMsg)
		turnMessages = append(turnMessages, assistantMsg)
		a.events.Emit(types.AgentEvent{Type: types.EventMessageEnd, Content: response.Content})
		a.events.Emit(types.AgentEvent{Type: types.EventTurnEnd})
		hctxTurnEnd := a.newHookContext("turn_end")
		hctxTurnEnd.TurnNumber = iterations + 1
		_, _ = a.hookMgr.Run(ctx, hctxTurnEnd)
		completed = true
		break
	}

	if !completed && agentErr == nil {
		agentErr = fmt.Errorf("agent loop reached maximum iterations (%d) without completing", maxIterations)
	}

	a.events.Emit(types.AgentEvent{Type: types.EventAgentEnd, Error: agentErr})
	_, _ = a.hookMgr.Run(ctx, a.newHookContext("agent_end"))
	return agentErr
}

// defaultOutputHandler is the default event subscriber that handles console output.
func (a *Agent) defaultOutputHandler(event types.AgentEvent) {
	switch event.Type {
	case types.EventToolStart:
		fmt.Fprintf(a.output, "%s[%s]%s ", types.ColorGray, event.ToolName, types.ColorReset)
	case types.EventToolEnd:
		if event.Content != "" {
			fmt.Fprintln(a.output)
			fmt.Fprint(a.output, event.Content)
		}
	case types.EventTurnEnd:
		fmt.Fprintln(a.output)
	case types.EventMessageEnd:
		if event.Content != "" {
			fmt.Fprintf(a.output, "\n\n")
		}
	}
}

// addUsage accumulates token usage from an API response.
func (a *Agent) addUsage(u types.TokenUsage) {
	a.usage.PromptTokens += u.PromptTokens
	a.usage.CompletionTokens += u.CompletionTokens
	a.usage.TotalTokens += u.TotalTokens
}

// GetUsage returns the accumulated session usage.
func (a *Agent) GetUsage() types.TokenUsage {
	return a.usage
}

// GetRegistry returns the tool registry.
func (a *Agent) GetRegistry() *tools.ToolRegistry {
	return a.registry
}

// Events returns the event emitter.
func (a *Agent) Events() *types.EventEmitter {
	return a.events
}

// GetModel returns the model name.
func (a *Agent) GetModel() string {
	return a.client.GetModel()
}

// lastMessage returns the last message with the given role, or empty string.
func (a *Agent) lastMessage(role string) string {
	for i := len(a.messages) - 1; i >= 0; i-- {
		if a.messages[i].Role == role {
			return a.messages[i].Content
		}
	}
	return ""
}

// newHookContext creates a base HookContext with common fields.
func (a *Agent) newHookContext(event string) *hooks.HookContext {
	wd, _ := os.Getwd()
	var systemPrompt string
	if len(a.messages) > 0 && a.messages[0].Role == "system" {
		systemPrompt = a.messages[0].Content
	}
	return &hooks.HookContext{
		Event:            event,
		WorkDir:          wd,
		Model:            a.client.GetModel(),
		SystemPrompt:     systemPrompt,
		UserMessage:      a.lastMessage("user"),
		AssistantMessage: a.lastMessage("assistant"),
	}
}

// extractToolInput extracts a human-readable input string for common tools.
func extractToolInput(toolName, argsJSON string) (string, bool) {
	if toolName == "bash" {
		var args map[string]interface{}
		if err := json.Unmarshal([]byte(argsJSON), &args); err == nil {
			if cmd, ok := args["command"].(string); ok {
				return cmd, true
			}
		}
	}
	return "", false
}
