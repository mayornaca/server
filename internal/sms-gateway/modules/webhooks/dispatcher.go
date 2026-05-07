package webhooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/android-sms-gateway/client-go/smsgateway"
	"github.com/android-sms-gateway/server/internal/sms-gateway/pubsub"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// dispatcherPubsubTopic is the in-process channel for server-side webhook
// dispatch. Published from messages.Service.UpdateState (when the gateway
// reports an outbound SMS state change), consumed by Dispatcher.Run which
// POSTs to user-registered webhook URLs.
//
// Note: with PUBSUB__URL=memory:// the publisher and consumer MUST live in the
// same process. Dispatcher is registered in the server binary (not worker),
// co-located with the publisher in messages.Service.
const dispatcherPubsubTopic = "webhooks.dispatch"

// backoffSchedule is the fixed exponential backoff applied between retries.
// Short enough to be useful during transient receiver flakes, bounded so
// retries don't pile up indefinitely.
//
//nolint:gochecknoglobals // fixed schedule, not runtime-configurable
var backoffSchedule = []time.Duration{
	5 * time.Second,
	15 * time.Second,
	45 * time.Second,
}

// webhookLookup is the minimal slice of Repository the dispatcher needs.
// Introduced so tests can inject an in-memory fake without dragging in CGO
// sqlite drivers.
type webhookLookup interface {
	Select(filters ...SelectFilter) ([]*Webhook, error)
}

// DispatcherParams wires the dispatcher via Fx. Kept separate from ServiceParams
// to avoid retrofitting the pre-existing Service constructor.
type DispatcherParams struct {
	fx.In

	Config     Config
	Webhooks   *Repository
	PubSub     pubsub.PubSub
	HTTPClient *http.Client `optional:"true"`
	Logger     *zap.Logger
}

// Dispatcher publishes webhook events to pubsub and consumes them to POST to
// user-registered URLs. When Config.ServerSideEnabled is false, Publish is a
// no-op and Run returns immediately — the Android gateway remains the only
// webhook dispatcher.
type Dispatcher struct {
	cfg      Config
	webhooks webhookLookup
	pubsub   pubsub.PubSub
	client   *http.Client
	logger   *zap.Logger
}

// NewDispatcher constructs the dispatcher. If HTTPClient is not provided, a
// default client with Config.Timeout is created.
func NewDispatcher(p DispatcherParams) *Dispatcher {
	client := p.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: p.cfgTimeout()}
	}
	return &Dispatcher{
		cfg:      p.Config,
		webhooks: p.Webhooks,
		pubsub:   p.PubSub,
		client:   client,
		logger:   p.Logger.Named("dispatcher"),
	}
}

func (p DispatcherParams) cfgTimeout() time.Duration {
	if p.Config.Timeout <= 0 {
		return 10 * time.Second
	}
	return p.Config.Timeout
}

// Publish enqueues a webhook event for server-side dispatch. No-op if the
// feature flag is off. Caller is responsible for building Payload to match
// the shape the Android gateway uses for the same event (homologation).
func (d *Dispatcher) Publish(ctx context.Context, userID string, deviceID *string, event smsgateway.WebhookEvent, payload map[string]any) {
	if !d.cfg.ServerSideEnabled {
		return
	}
	if userID == "" || event == "" {
		d.logger.Warn("ignoring dispatch with empty userID or event", zap.String("user_id", userID), zap.String("event", event))
		return
	}

	msg := DispatchEvent{
		UserID:   userID,
		DeviceID: deviceID,
		Event:    event,
		Payload:  payload,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		d.logger.Error("failed to marshal dispatch event", zap.Error(err))
		return
	}

	pubCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if pubErr := d.pubsub.Publish(pubCtx, dispatcherPubsubTopic, data); pubErr != nil {
		d.logger.Error("failed to publish webhook dispatch", zap.Error(pubErr))
	}
}

// Run subscribes to the dispatch topic and POSTs to user webhooks. Blocks
// until ctx is cancelled. Registered as an Fx runnable; the framework manages
// its lifecycle.
func (d *Dispatcher) Run(ctx context.Context) error {
	if !d.cfg.ServerSideEnabled {
		d.logger.Info("server-side webhook dispatch disabled; run loop exits (set WEBHOOKS__SERVER_SIDE_ENABLED=true to activate)")
		<-ctx.Done()
		return nil
	}
	sub, err := d.pubsub.Subscribe(ctx, dispatcherPubsubTopic)
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", dispatcherPubsubTopic, err)
	}
	defer sub.Close()

	d.logger.Info("server-side webhook dispatch active", zap.Int("max_retries", int(d.cfg.MaxRetries)))

	ch := sub.Receive()
	for {
		select {
		case <-ctx.Done():
			d.logger.Info("dispatcher stopped")
			return nil
		case msg, ok := <-ch:
			if !ok {
				d.logger.Info("dispatcher subscription closed")
				return nil
			}
			var ev DispatchEvent
			if jsonErr := json.Unmarshal(msg.Data, &ev); jsonErr != nil {
				d.logger.Error("failed to unmarshal dispatch event", zap.Error(jsonErr))
				continue
			}
			d.process(ctx, ev)
		}
	}
}

func (d *Dispatcher) process(ctx context.Context, ev DispatchEvent) {
	filters := []SelectFilter{
		WithUserID(ev.UserID),
		WithEvent(ev.Event),
	}
	if ev.DeviceID != nil {
		filters = append(filters, WithDeviceID(*ev.DeviceID, false))
	}

	hooks, err := d.webhooks.Select(filters...)
	if err != nil {
		d.logger.Error("failed to load webhooks for dispatch",
			zap.String("user_id", ev.UserID), zap.String("event", ev.Event), zap.Error(err))
		return
	}
	if len(hooks) == 0 {
		return
	}

	for _, h := range hooks {
		d.dispatch(ctx, h, ev)
	}
}

func (d *Dispatcher) dispatch(ctx context.Context, hook *Webhook, ev DispatchEvent) {
	body := OutboundRequest{
		WebhookID: hook.ExtID,
		DeviceID:  ev.DeviceID,
		Event:     ev.Event,
		Payload:   ev.Payload,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		d.logger.Error("failed to marshal webhook body", zap.String("webhook_id", hook.ExtID), zap.Error(err))
		return
	}

	maxAttempts := max(int(d.cfg.MaxRetries)+1, 1)
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		status, attemptErr := d.postOnce(ctx, hook.URL, bodyBytes)
		if attemptErr == nil && status >= 200 && status < 300 {
			d.logger.Info("webhook delivered",
				zap.String("webhook_id", hook.ExtID),
				zap.String("event", ev.Event),
				zap.String("url", hook.URL),
				zap.Int("status", status),
				zap.Int("attempt", attempt),
			)
			return
		}
		// cloud-gesvial.19.1: 4xx is the receiver telling us the request will
		// never be accepted (auth, validation, gone). Retrying just wastes
		// goroutine time and delays valid deliveries. 5xx and network errors
		// remain retriable.
		if attemptErr == nil && status >= 400 && status < 500 {
			d.logger.Warn("webhook delivery rejected (4xx); not retrying",
				zap.String("webhook_id", hook.ExtID),
				zap.String("event", ev.Event),
				zap.String("url", hook.URL),
				zap.Int("status", status),
				zap.Int("attempt", attempt),
			)
			return
		}
		// Last attempt: log as error, no sleep.
		if attempt == maxAttempts {
			d.logger.Error("webhook delivery exhausted retries",
				zap.String("webhook_id", hook.ExtID),
				zap.String("event", ev.Event),
				zap.String("url", hook.URL),
				zap.Int("status", status),
				zap.Int("attempts", attempt),
				zap.Error(attemptErr),
			)
			return
		}
		// Backoff before next try.
		delay := backoffSchedule[min(attempt-1, len(backoffSchedule)-1)]
		d.logger.Warn("webhook attempt failed; retrying",
			zap.String("webhook_id", hook.ExtID),
			zap.String("url", hook.URL),
			zap.Int("status", status),
			zap.Int("attempt", attempt),
			zap.Duration("next_retry_in", delay),
			zap.Error(attemptErr),
		)
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}
}

func (d *Dispatcher) postOnce(ctx context.Context, url string, body []byte) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "gesvial-sosgw/1.x (server; homologation)")

	resp, err := d.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("http do: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	return resp.StatusCode, nil
}

