// Package panelevents exposes a single SSE endpoint that streams
// paneleventsbus.Event messages to authenticated panel clients. Used by the
// React admin UI to refresh dashboards/lists without polling.
//
// Endpoint: GET /api/3rdparty/v1/events/panel  (auth: JWT or Basic)
//
// Format: standard text/event-stream with one JSON-encoded Event per
// message. The client uses the browser's native EventSource API.
//
// cloud-gesvial.19+.
package panelevents

import (
	"bufio"
	"encoding/json"
	"fmt"
	"time"

	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/base"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/middlewares/userauth"
	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/paneleventsbus"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type controllerParams struct {
	fx.In

	Bus       *paneleventsbus.Service
	Logger    *zap.Logger
	Validator *validator.Validate
}

type ThirdPartyController struct {
	base.Handler

	bus *paneleventsbus.Service
}

func NewThirdPartyController(p controllerParams) *ThirdPartyController {
	return &ThirdPartyController{
		Handler: base.Handler{Logger: p.Logger, Validator: p.Validator},
		bus:     p.Bus,
	}
}

// streamPanel opens a long-lived SSE connection. Closes when the client
// disconnects or the server tears down the request context.
func (h *ThirdPartyController) streamPanel(_ string, c *fiber.Ctx) error {
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no") // disable nginx buffering for SSE

	sub := h.bus.Subscribe()

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		// Send initial hello so the client knows the stream is alive even
		// before the first real event. The browser also uses this to flag
		// "connected" — without it EventSource holds the readyState=0 for
		// up to 30s on some networks.
		_, _ = fmt.Fprintf(w, "event: hello\ndata: {\"at\":\"%s\"}\n\n", time.Now().UTC().Format(time.RFC3339))
		if err := w.Flush(); err != nil {
			sub.Close()
			return
		}

		// Periodic keep-alive comments (`: ping`) every 15s to defeat
		// proxy idle timeouts. Doesn't trigger any client handler.
		ping := time.NewTicker(15 * time.Second)
		defer ping.Stop()
		defer sub.Close()

		for {
			select {
			case ev, ok := <-sub.Receive():
				if !ok {
					return
				}
				payload, err := json.Marshal(ev)
				if err != nil {
					h.Logger.Error("failed to marshal panel event", zap.Error(err))
					continue
				}
				_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, payload)
				if err != nil {
					return
				}
				if err := w.Flush(); err != nil {
					return
				}
			case <-ping.C:
				_, err := fmt.Fprint(w, ": ping\n\n")
				if err != nil {
					return
				}
				if err := w.Flush(); err != nil {
					return
				}
			}
		}
	}))

	return nil
}

// HoistTokenFromQuery copies `?token=...` into the Authorization header
// before the auth middleware runs. EventSource (browser SSE client) cannot
// set custom headers, so the React panel passes the JWT in the URL. This is
// safe because (a) the URL is HTTPS-only, (b) the server only reads the
// query param for this specific SSE endpoint, and (c) the token is short
// lived (15 min).
//
// Exported so the parent 3rdparty router can chain it BEFORE the JWT/Basic
// auth middlewares — registering it as a route-level handler does not work
// because Fiber v2 runs group-level Use() middlewares first.
// cloud-gesvial.19.1: pre-fix the SSE endpoint always 401'd in browsers.
func HoistTokenFromQuery(c *fiber.Ctx) error {
	if c.Get("Authorization") == "" {
		if t := c.Query("token"); t != "" {
			c.Request().Header.Set("Authorization", "Bearer "+t)
		}
	}
	return c.Next()
}

func (h *ThirdPartyController) Register(router fiber.Router) {
	// No fine-grained scope check: any authenticated user with a valid
	// JWT/Basic gets the panel stream. The UI itself filters by role for
	// what to render. Centralising auth here keeps the SSE handler simple.
	router.Get("/panel", userauth.WithUserID(h.streamPanel))
}
