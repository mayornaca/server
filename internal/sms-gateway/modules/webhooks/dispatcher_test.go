package webhooks

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/android-sms-gateway/client-go/smsgateway"
	"github.com/android-sms-gateway/server/pkg/pubsub"
	"go.uber.org/zap"
)

// fakeLookup is an in-memory stand-in for Repository. Avoids the CGO sqlite
// driver in tests while exercising the dispatcher end-to-end.
type fakeLookup struct {
	hooks []*Webhook
}

func (f *fakeLookup) Select(filters ...SelectFilter) ([]*Webhook, error) {
	sel := new(selectFilter)
	for _, fn := range filters {
		fn(sel)
	}
	out := make([]*Webhook, 0, len(f.hooks))
	for _, h := range f.hooks {
		if sel.userID != "" && h.UserID != sel.userID {
			continue
		}
		if sel.event != nil && h.Event != *sel.event {
			continue
		}
		if sel.deviceID != nil {
			if h.DeviceID == nil {
				if sel.deviceIDExact {
					continue
				}
			} else if *h.DeviceID != *sel.deviceID {
				continue
			}
		}
		out = append(out, h)
	}
	return out, nil
}

// newDispatcher constructs a Dispatcher bypassing Fx wiring for tests.
func newDispatcher(cfg Config, lookup webhookLookup, ps pubsub.PubSub) *Dispatcher {
	return &Dispatcher{
		cfg:      cfg,
		webhooks: lookup,
		pubsub:   ps,
		client:   &http.Client{Timeout: 2 * time.Second},
		logger:   zap.NewNop().Named("test"),
	}
}

func newLookup(target string, event smsgateway.WebhookEvent) *fakeLookup {
	return &fakeLookup{hooks: []*Webhook{
		{ExtID: "wh1", UserID: "u1", URL: target, Event: event}, //nolint:exhaustruct
	}}
}

// TestDispatcher_FlagOff_DropsPublish verifies that Publish is a no-op when
// ServerSideEnabled is false — nothing reaches the pubsub topic, nothing is
// POSTed. This is the default behaviour in gesvial.14.
func TestDispatcher_FlagOff_DropsPublish(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ps := pubsub.NewMemory()
	defer ps.Close() //nolint:errcheck
	lookup := newLookup(srv.URL, smsgateway.WebhookEventSmsSent)

	d := newDispatcher(Config{ServerSideEnabled: false, Timeout: 2 * time.Second, MaxRetries: 0}, lookup, ps)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = d.Run(ctx) }()
	time.Sleep(50 * time.Millisecond)

	d.Publish(ctx, "u1", nil, smsgateway.WebhookEventSmsSent, map[string]any{"messageId": "m1"})
	time.Sleep(100 * time.Millisecond)

	if got := calls.Load(); got != 0 {
		t.Fatalf("flag OFF must not fire POST; got %d calls", got)
	}
}

// TestDispatcher_FlagOn_FiresPost verifies that Publish reaches the consumer
// and a real HTTP POST is issued with the expected body shape. Happy-path E2E
// of the in-process dispatcher.
func TestDispatcher_FlagOn_FiresPost(t *testing.T) {
	received := make(chan OutboundRequest, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body OutboundRequest
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &body)
		received <- body
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ps := pubsub.NewMemory()
	defer ps.Close() //nolint:errcheck
	lookup := newLookup(srv.URL, smsgateway.WebhookEventSmsSent)

	d := newDispatcher(Config{ServerSideEnabled: true, Timeout: 2 * time.Second, MaxRetries: 0}, lookup, ps)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = d.Run(ctx) }()
	time.Sleep(50 * time.Millisecond)

	d.Publish(ctx, "u1", nil, smsgateway.WebhookEventSmsSent, map[string]any{
		"messageId":   "m1",
		"phoneNumber": "+56912345678",
	})

	select {
	case body := <-received:
		if body.WebhookID != "wh1" {
			t.Errorf("expected webhookId=wh1, got %q", body.WebhookID)
		}
		if body.Event != smsgateway.WebhookEventSmsSent {
			t.Errorf("expected event=sms:sent, got %q", body.Event)
		}
		if body.Payload["messageId"] != "m1" {
			t.Errorf("expected messageId=m1 in payload, got %v", body.Payload["messageId"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for POST")
	}
}

// TestDispatcher_Retries_OnNon2xx verifies the retry loop: first call returns
// 503, second returns 200 — the dispatcher retries and eventually succeeds.
func TestDispatcher_Retries_OnNon2xx(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := attempts.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ps := pubsub.NewMemory()
	defer ps.Close() //nolint:errcheck
	lookup := newLookup(srv.URL, smsgateway.WebhookEventSmsSent)

	// Shorten backoff for test speed.
	origSchedule := backoffSchedule
	backoffSchedule = []time.Duration{10 * time.Millisecond}
	t.Cleanup(func() { backoffSchedule = origSchedule })

	d := newDispatcher(Config{ServerSideEnabled: true, Timeout: 2 * time.Second, MaxRetries: 2}, lookup, ps)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = d.Run(ctx) }()
	time.Sleep(50 * time.Millisecond)

	d.Publish(ctx, "u1", nil, smsgateway.WebhookEventSmsSent, map[string]any{"messageId": "m1"})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if attempts.Load() >= 2 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("expected at least 2 attempts (1 fail + 1 retry succeeds), got %d", attempts.Load())
}

// TestOutboundRequest_JSONShape locks the wire format so future refactors
// don't silently break receivers that parse these POSTs.
func TestOutboundRequest_JSONShape(t *testing.T) {
	body := OutboundRequest{
		WebhookID: "wh1",
		Event:     smsgateway.WebhookEventSmsSent,
		Payload:   map[string]any{"messageId": "m1"},
	}
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	expected := `{"webhookId":"wh1","event":"sms:sent","payload":{"messageId":"m1"}}`
	if !bytes.Equal(data, []byte(expected)) {
		t.Errorf("shape drift:\n  got:  %s\n  want: %s", data, expected)
	}
}
