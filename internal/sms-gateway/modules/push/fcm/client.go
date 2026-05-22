package fcm

import (
	"context"
	"fmt"
	"sync"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/push/client"
	"go.uber.org/zap"
	"google.golang.org/api/option"
)

type Client struct {
	options map[string]string

	client      *messaging.Client
	initialized bool
	mux         sync.Mutex
	logger      *zap.Logger
}

// New creates an FCM client. cloud-gesvial.19.3: a logger argument is now
// required so the operator can trace, in `docker compose logs server`, which
// device token received which FCM message and the FCM message-id returned by
// Google. Pre-fix the only visible log was "messages sent successfully total=N"
// (a per-batch counter) — debugging "the cloud sent the test, did the Z5
// receive it?" required attaching a debugger.
func New(options map[string]string, logger *zap.Logger) (*Client, error) {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Client{
		options: options,
		client:  nil,
		mux:     sync.Mutex{},
		logger:  logger.Named("fcm"),
	}, nil
}

func (c *Client) Open(ctx context.Context) error {
	c.mux.Lock()
	defer c.mux.Unlock()

	if c.client != nil {
		return nil
	}

	creds := c.options["credentials"]
	if creds == "" {
		return fmt.Errorf("%w: no credentials provided", ErrInitializationFailed)
	}

	opt := option.WithAuthCredentialsJSON(option.ServiceAccount, []byte(creds))

	app, err := firebase.NewApp(ctx, nil, opt)
	if err != nil {
		return fmt.Errorf("%w: failed to create firebase app: %w", ErrInitializationFailed, err)
	}

	c.client, err = app.Messaging(ctx)
	if err != nil {
		return fmt.Errorf("%w: failed to create firebase messaging client: %w", ErrInitializationFailed, err)
	}

	c.initialized = true
	return nil
}

// Send dispatches a batch of FCM messages. cloud-gesvial.18.4 switched from
// a sequential `client.Send` per message to `client.SendEach`, which uses
// HTTP/2 stream multiplexing (up to 500 messages per batch). The previous
// sequential loop hit `context deadline exceeded` whenever sendAll drained
// >5-10 events in one debounce window — each `Send` round-trip is 200-800ms
// to fcm.googleapis.com, so 19 events × 500ms = ~10s blew the default
// timeout. SendEach is parallel under the hood and finishes in ~1 round-trip.
func (c *Client) Send(ctx context.Context, messages []client.Message) ([]error, error) {
	errs := make([]error, len(messages))
	if len(messages) == 0 {
		return errs, nil
	}

	fcmMessages := make([]*messaging.Message, 0, len(messages))
	indexes := make([]int, 0, len(messages))
	for i, message := range messages {
		data, err := eventToMap(message.Event)
		if err != nil {
			errs[i] = fmt.Errorf("failed to marshal event: %w", err)
			continue
		}
		fcmMessages = append(fcmMessages, &messaging.Message{
			Data:    data,
			Android: &messaging.AndroidConfig{Priority: "high"},
			Token:   message.Token,
		})
		indexes = append(indexes, i)
	}

	if len(fcmMessages) == 0 {
		return errs, nil
	}

	// SendEach returns a BatchResponse aligned with the input order. Errors
	// per-message are surfaced via Responses[i].Error; a top-level error
	// indicates an end-to-end failure (auth, network, etc).
	batch, err := c.client.SendEach(ctx, fcmMessages)
	if err != nil {
		// Fail every message of this batch with the same error — the caller
		// (push.Service) already retries individual wrappers.
		for _, idx := range indexes {
			errs[idx] = fmt.Errorf("failed to send batch: %w", err)
		}
		return errs, nil
	}

	for j, resp := range batch.Responses {
		token := tokenSuffix(messages[indexes[j]].Token)
		if !resp.Success && resp.Error != nil {
			errs[indexes[j]] = fmt.Errorf("failed to send message: %w", resp.Error)
			c.logger.Warn("fcm send failed",
				zap.String("token_suffix", token),
				zap.Error(resp.Error),
			)
			continue
		}
		// Per-message success log so the operator can correlate one entry
		// here with one row in the Firebase console (message_id is the
		// stable key Google uses across both surfaces).
		c.logger.Info("fcm send ok",
			zap.String("token_suffix", token),
			zap.String("message_id", resp.MessageID),
		)
	}

	return errs, nil
}

// tokenSuffix returns the last 8 chars of the FCM registration token. We never
// log the full token because it is enough to push to the device — leaking it
// to logs would let an attacker with read access spam that specific Z5.
func tokenSuffix(token string) string {
	if len(token) <= 8 {
		return token
	}
	return "..." + token[len(token)-8:]
}

func (c *Client) Close(_ context.Context) error {
	c.mux.Lock()
	defer c.mux.Unlock()

	c.client = nil
	c.initialized = false

	return nil
}
