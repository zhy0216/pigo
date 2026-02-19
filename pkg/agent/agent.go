package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/user/pigo/pkg/config"
	"github.com/user/pigo/pkg/llm"
	"github.com/user/pigo/pkg/memory"
	"github.com/user/pigo/pkg/ops"
	"github.com/user/pigo/pkg/session"
	"github.com/user/pigo/pkg/skills"
	"github.com/user/pigo/pkg/tools"
	"github.com/user/pigo/pkg/types"
)

// Agent represents the pigo application.
type Agent struct {
	client       *llm.Client
	registry     *tools.ToolRegistry
	messages     []types.Message
	output       io.Writer
	Skills       []skills.Skill
	events       *types.EventEmitter
	usage        types.TokenUsage // accumulated session usage
	Memory       *memory.MemoryStore
	extractor    *memory.MemoryExtractor
	deduplicator *memory.MemoryDeduplicator
}

const memoryContextPrefix = "## Retrieved Memories (auto)\n"

// NewAgent creates a new Agent instance.
func NewAgent(cfg *config.Config) *Agent {
	client := llm.NewClient(cfg.APIKey, cfg.BaseURL, cfg.Model, cfg.APIType, cfg.EmbedModel)
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

	memStore := memory.NewMemoryStore()
	if err := memStore.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to load memories: %v\n", err)
	}

	systemPrompt := `You are a helpful AI coding assistant. You have access to tools for reading, writing, and editing files, as well as executing bash commands.

When helping with coding tasks:
1. Read files before modifying them
2. Make targeted edits rather than rewriting entire files
3. Test your changes when possible
4. Be concise in your explanations`

	systemPrompt += skills.FormatSkillsForPrompt(loadedSkills)

	if memStore.Count() > 0 {
		memContext := memStore.FormatForPrompt(20)
		if memContext != "" {
			systemPrompt += "\n\n## Your Memories\nThe following are memories from previous sessions:\n\n" + memContext
		}
	}

	messages := []types.Message{
		{
			Role:    "system",
			Content: systemPrompt,
		},
	}

	events := types.NewEventEmitter()

	dedup := &memory.MemoryDeduplicator{
		Client: client,
		Store:  memStore,
	}

	extractor := &memory.MemoryExtractor{
		Client:       client,
		Store:        memStore,
		Deduplicator: dedup,
		Output:       os.Stdout,
	}

	agent := &Agent{
		client:       client,
		registry:     registry,
		messages:     messages,
		output:       os.Stdout,
		Skills:       loadedSkills,
		events:       events,
		Memory:       memStore,
		extractor:    extractor,
		deduplicator: dedup,
	}

	// Register memory tools
	memDeps := memory.MemoryToolDeps{
		Client:       client,
		Store:        memStore,
		Extractor:    extractor,
		Deduplicator: dedup,
	}
	registry.Register(memory.NewMemoryRecallTool(memDeps))
	registry.Register(memory.NewMemoryRememberTool(memDeps))
	registry.Register(memory.NewMemoryForgetTool(memDeps))

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
	case "/sessions":
		sessions, err := session.ListSessions()
		if err != nil {
			fmt.Fprintf(a.output, "Error listing sessions: %v\n", err)
			return true, false
		}
		if len(sessions) == 0 {
			fmt.Fprintln(a.output, "No saved sessions.")
		} else {
			fmt.Fprintln(a.output, "Saved sessions:")
			for _, s := range sessions {
				fmt.Fprintf(a.output, "  %s  (%d messages, %s)\n",
					s.ID, s.Messages, s.ModTime.Format("2006-01-02 15:04"))
			}
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
	case "/memory":
		if a.Memory == nil || a.Memory.Count() == 0 {
			fmt.Fprintln(a.output, "No memories stored.")
		} else {
			fmt.Fprintf(a.output, "Stored memories (%d total):\n", a.Memory.Count())
			for _, cat := range memory.ValidCategories {
				mems := a.Memory.List(cat)
				if len(mems) == 0 {
					continue
				}
				fmt.Fprintf(a.output, "\n  [%s] (%d)\n", cat, len(mems))
				for _, m := range mems {
					fmt.Fprintf(a.output, "    %s  %s\n", m.ID, m.Abstract)
				}
			}
		}
		return true, false
	}

	// Commands with arguments
	if input == "/save" || strings.HasPrefix(input, "/save ") {
		name := strings.TrimPrefix(input, "/save")
		name = strings.TrimSpace(name)
		if name == "" {
			name = session.SessionID()
		}
		if err := session.SaveSession(name, a.messages); err != nil {
			fmt.Fprintf(a.output, "Error saving session: %v\n", err)
		} else {
			fmt.Fprintf(a.output, "Session saved: %s\n", name)
		}
		return true, false
	}
	if strings.HasPrefix(input, "/load ") {
		name := strings.TrimSpace(strings.TrimPrefix(input, "/load"))
		if name == "" {
			fmt.Fprintln(a.output, "Usage: /load <session-id>")
			return true, false
		}
		loaded, err := session.LoadSession(name)
		if err != nil {
			fmt.Fprintf(a.output, "Error loading session: %v\n", err)
			return true, false
		}
		a.messages = append(a.messages[:1], loaded...)
		fmt.Fprintf(a.output, "Session loaded: %s (%d messages)\n", name, len(loaded))
		return true, false
	}
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
	if input == "/memory clear" {
		if a.Memory != nil {
			a.Memory.Clear()
			a.Memory.Save()
			fmt.Fprintln(a.output, "All memories cleared.")
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

	a.compactMessages(ctx)

	memoryInjected := a.injectMemoryContext(ctx, input)
	if memoryInjected {
		defer a.removeMemoryContext()
	}

	turnMessages := []types.Message{}
	if len(a.messages) > 0 {
		turnMessages = append(turnMessages, a.messages[len(a.messages)-1])
	}

	a.events.Emit(types.AgentEvent{Type: types.EventAgentStart})

	maxIterations := types.MaxAgentIterations
	completed := false
	var agentErr error
	for iterations := 0; iterations < maxIterations; iterations++ {
		a.events.Emit(types.AgentEvent{Type: types.EventTurnStart})

		var response *types.ChatResponse
		var chatErr error
		var savedMessages []types.Message
		for retries := 0; retries <= types.MaxOverflowRetries; retries++ {
			response, chatErr = a.client.ChatStream(ctx, a.messages, a.registry.GetDefinitions(), a.output)
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

				var args map[string]interface{}
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					result := types.ErrorResult(fmt.Sprintf("failed to parse arguments: %v", err))
					a.messages = append(a.messages, types.Message{
						Role:       "tool",
						Content:    result.ForLLM,
						ToolCallID: tc.ID,
					})
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
			}

			a.events.Emit(types.AgentEvent{Type: types.EventTurnEnd})
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
		completed = true
		break
	}

	if completed && a.extractor != nil && ctx.Err() == nil && len(turnMessages) > 1 {
		a.extractor.ExtractMemories(ctx, turnMessages)
	}

	if !completed && agentErr == nil {
		agentErr = fmt.Errorf("agent loop reached maximum iterations (%d) without completing", maxIterations)
	}

	a.events.Emit(types.AgentEvent{Type: types.EventAgentEnd, Error: agentErr})
	return agentErr
}

func (a *Agent) injectMemoryContext(ctx context.Context, query string) bool {
	if a.Memory == nil || a.Memory.Count() == 0 {
		return false
	}

	const maxResults = 5
	var results []*memory.Memory

	vec, err := a.client.Embed(ctx, query)
	if err == nil && len(vec) > 0 {
		results = a.Memory.SearchByVector(vec, maxResults, "")
	}

	if len(results) == 0 {
		results = a.Memory.SearchByKeyword(query, maxResults)
	}

	if len(results) == 0 {
		return false
	}

	for _, m := range results {
		a.Memory.IncrementActive(m.ID)
	}
	_ = a.Memory.Save()

	var buf strings.Builder
	buf.WriteString(memoryContextPrefix)
	buf.WriteString("Use these only if relevant to the user's current request.\n\n")
	for _, m := range results {
		fmt.Fprintf(&buf, "- [%s] %s\n", m.Category, m.Abstract)
		if m.Overview != "" && m.Overview != m.Abstract {
			fmt.Fprintf(&buf, "  %s\n", m.Overview)
		}
	}

	a.messages = append(a.messages, types.Message{
		Role:    "system",
		Content: buf.String(),
	})

	return true
}

func (a *Agent) removeMemoryContext() {
	for i := len(a.messages) - 1; i >= 0; i-- {
		if a.messages[i].Role == "system" && strings.HasPrefix(a.messages[i].Content, memoryContextPrefix) {
			a.messages = append(a.messages[:i], a.messages[i+1:]...)
			return
		}
	}
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
