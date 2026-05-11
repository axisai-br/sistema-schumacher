package realtime

import (
	"sync"
	"time"
)

type ChatEvent struct {
	Type      string    `json:"type"`
	Channel   string    `json:"channel"`
	SessionID string    `json:"session_id,omitempty"`
	At        time.Time `json:"at"`
}

type Broker struct {
	mu          sync.RWMutex
	subscribers map[chan ChatEvent]struct{}
}

func NewBroker() *Broker {
	return &Broker{
		subscribers: make(map[chan ChatEvent]struct{}),
	}
}

func (b *Broker) Subscribe() (<-chan ChatEvent, func()) {
	ch := make(chan ChatEvent, 16)
	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()

	cancel := func() {
		b.mu.Lock()
		if _, ok := b.subscribers[ch]; ok {
			delete(b.subscribers, ch)
			close(ch)
		}
		b.mu.Unlock()
	}
	return ch, cancel
}

func (b *Broker) Publish(event ChatEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subscribers {
		select {
		case ch <- event:
		default:
			// Drop event for slow subscribers to avoid blocking webhook processing.
		}
	}
}

