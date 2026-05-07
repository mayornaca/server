package handlers

import (
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/devices"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/events"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/logs"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/messages"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/panelevents"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/posts"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/schedules"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/settings"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/tests"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/thirdparty"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/webhooks"
	"github.com/capcom6/go-infra-fx/http"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

func Module() fx.Option {
	return fx.Module(
		"handlers",
		fx.Decorate(func(log *zap.Logger) *zap.Logger {
			return log.Named("handlers")
		}),
		fx.Provide(
			http.AsRootHandler(newRootHandler),
			http.AsRootHandler(newWebHandler),
			http.AsApiHandler(newThirdPartyHandler),
			http.AsApiHandler(newMobileHandler),
			http.AsApiHandler(newUpstreamHandler),
		),
		fx.Provide(
			NewHealthHandler,
			messages.NewThirdPartyController,
			messages.NewMobileController,
			webhooks.NewThirdPartyController,
			webhooks.NewMobileController,
			devices.NewThirdPartyController,
			settings.NewThirdPartyController,
			settings.NewMobileController,
			logs.NewThirdPartyController,
			posts.NewThirdPartyController,
			posts.NewMobileController,
			schedules.NewThirdPartyController,
			tests.NewThirdPartyController,
			tests.NewMobileController,
			events.NewMobileController,
			panelevents.NewThirdPartyController,
			fx.Private,
		),
		thirdparty.Module(),
	)
}
