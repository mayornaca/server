package push

import (
	"encoding/json"
	"fmt"

	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/push/client"
)

type Mode string

const (
	ModeFCM      Mode = "fcm"
	ModeUpstream Mode = "upstream"
	ModeDisabled Mode = "disabled" // no push; relies on SSE only
)

type Event = client.Event

type eventWrapper struct {
	Token   string `json:"token"`
	Event   Event  `json:"event"`
	Retries int    `json:"retries"`
}

// key returns a cache key unique per enqueue — NOT per (token, eventType).
//
// cloud-gesvial.18.3 fix: the previous key `token:eventType` collapsed every
// concurrent event of the same kind to the same gateway into a single cache
// slot. When the cron dispatcher schedules a batch of N TestRequested events
// to the same Z5 token, only the LAST one survived the debounce window —
// `sendAll` drained one entry and shipped one push, losing the other N-1.
// That's why "se envia el primer (último) mensaje del batch, pero no el resto".
//
// Strategy: for events that carry a per-instance identifier in `Data`
// (TestRequested has `testResultId`, MessagesExportRequested has timestamps),
// we incorporate that into the key. For idempotent events without an instance
// id (MessageEnqueued, WebhooksUpdated, SettingsUpdated, MessagesPing) the
// previous behaviour of "collapse repeated pings within the debounce window"
// is correct and we keep `token:eventType` as the key.
func (e *eventWrapper) key() string {
	base := e.Token + ":" + string(e.Event.Type)
	// First check known per-instance identifiers; testResultId is currently
	// the only one in use (TestRequested), but the pattern extends naturally.
	if e.Event.Data != nil {
		if id, ok := e.Event.Data["testResultId"]; ok && id != "" {
			return base + ":" + id
		}
	}
	return base
}

func (e *eventWrapper) serialize() ([]byte, error) {
	data, err := json.Marshal(e)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event: %w", err)
	}

	return data, nil
}

func (e *eventWrapper) deserialize(data []byte) error {
	if err := json.Unmarshal(data, e); err != nil {
		return fmt.Errorf("failed to unmarshal event: %w", err)
	}

	return nil
}
