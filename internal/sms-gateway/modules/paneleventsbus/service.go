// Package paneleventsbus provides an in-process publish/subscribe service
// for delivering lightweight events to the React admin panel via SSE.
//
// Why a separate module: events.Service routes events to mobile devices
// (via FCM/SSE per-device); paneleventsbus broadcasts to all panel
// subscribers regardless of device — they see the whole picture.
//
// cloud-gesvial.19+.
package paneleventsbus

import (
	"sync"
	"time"

	"github.com/jaevor/go-nanoid"
)

// Event is the wire format streamed over SSE to the panel. It is intentionally
// small — clients can call back to the appropriate REST endpoint to get the
// full resource if they need details. The panel uses these events only to
// invalidate cache and trigger refetch of the affected page section.
type Event struct {
	Type       EventType `json:"type"`
	UserID     string    `json:"userId,omitempty"`
	ResourceID string    `json:"resourceId,omitempty"`
	Status     string    `json:"status,omitempty"`
	At         time.Time `json:"at"`
}

// EventType enumerates panel-relevant changes.
type EventType string

const (
	EventTypeTestScheduled       EventType = "test.scheduled"
	EventTypeTestCompleted       EventType = "test.completed"
	EventTypePostStatusChanged   EventType = "post.statusChanged"
	EventTypeDeviceConnected     EventType = "device.connected"
	EventTypeDeviceDisconnected  EventType = "device.disconnected"
	EventTypeScheduleFired       EventType = "schedule.fired"
)

// subscriber wraps an output channel for one connected panel client.
type subscriber struct {
	id string
	ch chan Event
}

// Service is the broadcaster. Publish() is non-blocking — slow consumers
// drop events rather than back-pressuring the publisher (we'd rather miss
// a UI refresh than freeze the cron).
type Service struct {
	mu          sync.RWMutex
	subscribers map[string]*subscriber
	idgen       func() string
}

// NewService constructs a Service. Buffer size of 16 events per subscriber
// is enough to absorb a small burst of concurrent test schedules without
// dropping. Past that we drop the oldest.
func NewService() *Service {
	idgen, _ := nanoid.Standard(21)
	return &Service{
		subscribers: make(map[string]*subscriber),
		idgen:       idgen,
	}
}

// Publish broadcasts ev to every subscriber. Non-blocking: if a subscriber's
// buffer is full, the event is dropped for that consumer (the rest still
// receive it). At = now() is filled in here if the caller didn't.
func (s *Service) Publish(ev Event) {
	if ev.At.IsZero() {
		ev.At = time.Now()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, sub := range s.subscribers {
		select {
		case sub.ch <- ev:
		default:
			// drop for slow consumer
		}
	}
}

// Subscription is the consumer-facing handle.
type Subscription struct {
	svc *Service
	id  string
	ch  chan Event
}

// Subscribe registers a new consumer. The returned Subscription must be
// Close()d when the SSE stream disconnects, otherwise we leak the channel.
func (s *Service) Subscribe() *Subscription {
	id := s.idgen()
	sub := &subscriber{
		id: id,
		ch: make(chan Event, 16),
	}
	s.mu.Lock()
	s.subscribers[id] = sub
	s.mu.Unlock()
	return &Subscription{svc: s, id: id, ch: sub.ch}
}

// Receive returns the channel of incoming events.
func (sub *Subscription) Receive() <-chan Event {
	return sub.ch
}

// Close removes the subscription from the broadcaster and drains the channel.
// Safe to call multiple times.
func (sub *Subscription) Close() {
	sub.svc.mu.Lock()
	if existing, ok := sub.svc.subscribers[sub.id]; ok {
		delete(sub.svc.subscribers, sub.id)
		close(existing.ch)
	}
	sub.svc.mu.Unlock()
}
