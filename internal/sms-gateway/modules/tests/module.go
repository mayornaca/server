package tests

import (
	"github.com/capcom6/go-infra-fx/db"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

func Module() fx.Option {
	return fx.Module(
		"tests",
		fx.Decorate(func(log *zap.Logger) *zap.Logger {
			return log.Named("tests")
		}),
		fx.Provide(NewRepository, fx.Private),
		fx.Provide(NewService),
	)
}

//nolint:gochecknoinits //framework-specific
func init() {
	db.RegisterMigration(Migrate)
}