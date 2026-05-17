package server

import (
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers"
	"github.com/go-core-fx/fiberfx"
	"github.com/go-core-fx/fiberfx/statuscode"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/fx"
)

func Module() fx.Option {
	return fx.Module(
		"server",
		fx.Provide(func(c Config) (fiberfx.Config, fiberfx.Options) {
			return fiberfx.Config{
				Address:     c.Address,
				ProxyHeader: fiber.HeaderXForwardedFor,
				Proxies:     c.Proxies,
			}, fiberfx.Options{}
		}),
		fx.Provide(
			handlers.NewHealthHandler,
			fx.Private,
		),
		fx.Invoke(func(app *fiber.App, health *handlers.HealthHandler) {
			health.Register(app)
			app.Use(statuscode.New())
		}),
	)
}
