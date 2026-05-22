// Package noop provides a push client that accepts sends without delivering
// anywhere. Used when PushBackend=disabled so the server relies on SSE only
// without attempting any egress to external push providers.
package noop

import (
	"context"

	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/push/client"
)

type Client struct{}

func New(_ map[string]string) (*Client, error) {
	return &Client{}, nil
}

// Open is a no-op.
func (c *Client) Open(_ context.Context) error { return nil }

// Send returns nil per message, effectively discarding the batch.
// The server's SSE path still delivers the event to connected gateways.
func (c *Client) Send(_ context.Context, messages []client.Message) ([]error, error) {
	return make([]error, len(messages)), nil
}

// Close is a no-op.
func (c *Client) Close(_ context.Context) error { return nil }

// HealthCheck is a no-op (siempre Pass). push_backend=disabled no tiene
// state externo que pueda fallar.
func (c *Client) HealthCheck(_ context.Context) error { return nil }

var _ client.Client = (*Client)(nil)
