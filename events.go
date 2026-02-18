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

// subscriber is an identified event callback.
type subscriber struct {
	id uint64
	fn func(AgentEvent)
}

// EventEmitter provides a simple pub-sub mechanism for agent events.
type EventEmitter struct {
	mu     sync.RWMutex
	subs   []subscriber
	nextID uint64
}

// NewEventEmitter creates a new EventEmitter.
func NewEventEmitter() *EventEmitter {
	return &EventEmitter{}
}

// Subscribe registers a callback to receive events. Returns an unsubscribe
// function that safely removes the callback by ID.
func (e *EventEmitter) Subscribe(fn func(AgentEvent)) func() {
	e.mu.Lock()
	defer e.mu.Unlock()
	id := e.nextID
	e.nextID++
	e.subs = append(e.subs, subscriber{id: id, fn: fn})
	return func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		for i, s := range e.subs {
			if s.id == id {
				// Remove by swapping with last element to avoid O(n) shifts.
				e.subs[i] = e.subs[len(e.subs)-1]
				e.subs = e.subs[:len(e.subs)-1]
				break
			}
		}
	}
}

// Emit sends an event to all subscribers.
func (e *EventEmitter) Emit(event AgentEvent) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, s := range e.subs {
		s.fn(event)
	}
}
