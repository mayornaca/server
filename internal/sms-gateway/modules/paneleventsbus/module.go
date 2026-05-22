package paneleventsbus

import "go.uber.org/fx"

func Module() fx.Option {
	return fx.Module(
		"paneleventsbus",
		fx.Provide(NewService),
	)
}
