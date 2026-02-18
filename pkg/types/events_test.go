package types

import (
	"sync"
	"testing"
)

func TestEventEmitter(t *testing.T) {
	t.Run("subscribe and emit", func(t *testing.T) {
		emitter := NewEventEmitter()
		var received []AgentEvent

		emitter.Subscribe(func(e AgentEvent) {
			received = append(received, e)
		})

		emitter.Emit(AgentEvent{Type: EventAgentStart})
		emitter.Emit(AgentEvent{Type: EventToolStart, ToolName: "bash"})
		emitter.Emit(AgentEvent{Type: EventAgentEnd})

		if len(received) != 3 {
			t.Fatalf("expected 3 events, got %d", len(received))
		}
		if received[0].Type != EventAgentStart {
			t.Errorf("expected AgentStart, got %s", received[0].Type)
		}
		if received[1].ToolName != "bash" {
			t.Errorf("expected tool name 'bash', got %s", received[1].ToolName)
		}
		if received[2].Type != EventAgentEnd {
			t.Errorf("expected AgentEnd, got %s", received[2].Type)
		}
	})

	t.Run("multiple subscribers", func(t *testing.T) {
		emitter := NewEventEmitter()
		var count1, count2 int

		emitter.Subscribe(func(e AgentEvent) { count1++ })
		emitter.Subscribe(func(e AgentEvent) { count2++ })

		emitter.Emit(AgentEvent{Type: EventAgentStart})

		if count1 != 1 || count2 != 1 {
			t.Errorf("expected both subscribers to receive event, got %d and %d", count1, count2)
		}
	})

	t.Run("unsubscribe", func(t *testing.T) {
		emitter := NewEventEmitter()
		var count int

		unsub := emitter.Subscribe(func(e AgentEvent) { count++ })
		emitter.Emit(AgentEvent{Type: EventAgentStart})

		unsub()
		emitter.Emit(AgentEvent{Type: EventAgentEnd})

		if count != 1 {
			t.Errorf("expected 1 event after unsubscribe, got %d", count)
		}
	})

	t.Run("concurrent emit", func(t *testing.T) {
		emitter := NewEventEmitter()
		var mu sync.Mutex
		var count int

		emitter.Subscribe(func(e AgentEvent) {
			mu.Lock()
			count++
			mu.Unlock()
		})

		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				emitter.Emit(AgentEvent{Type: EventTurnStart})
			}()
		}
		wg.Wait()

		if count != 100 {
			t.Errorf("expected 100 events, got %d", count)
		}
	})

	t.Run("no subscribers", func(t *testing.T) {
		emitter := NewEventEmitter()
		// Should not panic
		emitter.Emit(AgentEvent{Type: EventAgentStart})
	})
}
