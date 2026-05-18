package push

import (
	"context"

	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/push/client"
	"github.com/android-sms-gateway/server/pkg/health"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

func Module() fx.Option {
	return fx.Module(
		"push",
		fx.Decorate(func(log *zap.Logger) *zap.Logger {
			return log.Named("push")
		}),
		fx.Provide(
			newClient,
			fx.Private,
		),
		fx.Provide(
			New,
		),
		fx.Provide(
			health.AsHealthProvider(NewHealthProvider),
		),
		fx.Invoke(func(lc fx.Lifecycle, c client.Client) {
			lc.Append(fx.Hook{
				OnStart: func(ctx context.Context) error {
					return c.Open(ctx)
				},
				OnStop: func(ctx context.Context) error {
					return c.Close(ctx)
				},
			})
		}),
	)
}
