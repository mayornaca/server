package servertasks

import (
	"go.uber.org/fx"
	"go.uber.org/zap"
)

func Module() fx.Option {
	return fx.Module(
		"servertasks",
		fx.Decorate(func(log *zap.Logger) *zap.Logger {
			return log.Named("servertasks")
		}),
		fx.Provide(NewRunner),
	)
}
