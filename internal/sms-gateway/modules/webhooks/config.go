package webhooks

import "time"

// Config controls the server-side dispatcher (gesvial.14+).
// When ServerSideEnabled=false (default), Dispatch is a no-op — only the
// Android gateway POSTs to user webhooks. When true, the server dispatches in
// parallel for homologation/validation before a future release deprecates the
// app-side handler.
type Config struct {
	ServerSideEnabled bool
	Timeout           time.Duration
	MaxRetries        uint8
}
