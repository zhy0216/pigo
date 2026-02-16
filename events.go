package main

import "sync"

// AgentEventType identifies the kind of agent event.
type AgentEventType string

const (
	EventAgentStart   AgentEventType = "agent_start"
	EventTurnStart    AgentEventType = "turn_start"
	EventTurnEnd      AgentEventType = "turn_end"
	EventMessageStart AgentEventType = "message_start"
	EventMessageEnd   AgentEventType = "message_end"
	EventToolStart    AgentEventType = "tool_start"
	EventToolEnd      AgentEventType = "tool_end"
	EventAgentEnd     AgentEventType = "agent_end"
)

// AgentEvent represents an event emitted during agent execution.
type AgentEvent struct {
	Type     AgentEventType
	ToolName string // set for ToolStart/ToolEnd
	Content  string // set for MessageEnd (the response text)
	Error    error  // set for TurnEnd/AgentEnd on failure
}

// EventEmitter provides a simple pub-sub mechanism for agent events.
type EventEmitter struct {
	mu          sync.RWMutex
	subscribers []func(AgentEvent)
}

// NewEventEmitter creates a new EventEmitter.
func NewEventEmitter() *EventEmitter {
	return &EventEmitter{}
}

// Subscribe registers a callback to receive events. Returns an unsubscribe
// function (not commonly needed, but available).
func (e *EventEmitter) Subscribe(fn func(AgentEvent)) func() {
	e.mu.Lock()
	defer e.mu.Unlock()
	idx := len(e.subscribers)
	e.subscribers = append(e.subscribers, fn)
	return func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		if idx < len(e.subscribers) {
			e.subscribers[idx] = nil
		}
	}
}

// Emit sends an event to all subscribers.
func (e *EventEmitter) Emit(event AgentEvent) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, fn := range e.subscribers {
		if fn != nil {
			fn(event)
		}
	}
}
