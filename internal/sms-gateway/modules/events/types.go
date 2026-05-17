package events

import (
	"encoding/json"
	"fmt"

	"github.com/android-sms-gateway/client-go/smsgateway"
	"github.com/jaevor/go-nanoid"
)

const eventIDLength = 21

// idgen es el generador de IDs nanoid (21 chars ASCII, ~125 bits de entropía)
// para correlación de eventos cloud-side logs ↔ logcat del device Android.
// Introducido en cloud-gesvial.22.2 (Fase 2 plan QA 2026-05-17). Reemplaza
// el approach previo sin id — antes los logs Zap no podían cruzarse con
// logcat del BV7100 porque cada lado generaba sus propios IDs internos.
var idgen = func() func() string {
	g, _ := nanoid.Standard(eventIDLength)
	return g
}()

type Event struct {
	ID        string                   `json:"event_id"`
	EventType smsgateway.PushEventType `json:"event_type"`
	Data      map[string]string        `json:"data"`
}

func NewEvent(eventType smsgateway.PushEventType, data map[string]string) Event {
	return Event{
		ID:        idgen(),
		EventType: eventType,
		Data:      data,
	}
}

type eventWrapper struct {
	UserID   string  `json:"user_id"`
	DeviceID *string `json:"device_id,omitempty"`
	Event    Event   `json:"event"`
}

func (w *eventWrapper) serialize() ([]byte, error) {
	data, err := json.Marshal(w)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event: %w", err)
	}

	return data, nil
}

func (w *eventWrapper) deserialize(data []byte) error {
	if err := json.Unmarshal(data, w); err != nil {
		return fmt.Errorf("failed to unmarshal event: %w", err)
	}

	return nil
}
