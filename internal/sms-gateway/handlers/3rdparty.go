package handlers

import (
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/base"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/devices"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/logs"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/messages"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/middlewares/jwtauth"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/middlewares/userauth"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/panelevents"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/posts"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/schedules"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/settings"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/tests"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/thirdparty"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/webhooks"
	"github.com/android-sms-gateway/server/internal/sms-gateway/jwt"
	"github.com/android-sms-gateway/server/internal/sms-gateway/users"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

type thirdPartyHandler struct {
	base.Handler

	usersSvc *users.Service
	jwtSvc   jwt.Service

	healthHandler   *HealthHandler
	messagesHandler *messages.ThirdPartyController
	webhooksHandler *webhooks.ThirdPartyController
	devicesHandler  *devices.ThirdPartyController
	settingsHandler *settings.ThirdPartyController
	logsHandler     *logs.ThirdPartyController
	postsHandler        *posts.ThirdPartyController
	schedulesHandler    *schedules.ThirdPartyController
	testsHandler        *tests.ThirdPartyController
	panelEventsHandler  *panelevents.ThirdPartyController
	authHandler         *thirdparty.AuthHandler
}

func newThirdPartyHandler(
	usersSvc *users.Service,
	jwtService jwt.Service,

	healthHandler *HealthHandler,
	messagesHandler *messages.ThirdPartyController,
	webhooksHandler *webhooks.ThirdPartyController,
	devicesHandler *devices.ThirdPartyController,
	settingsHandler *settings.ThirdPartyController,
	logsHandler *logs.ThirdPartyController,
	postsHandler *posts.ThirdPartyController,
	schedulesHandler *schedules.ThirdPartyController,
	testsHandler *tests.ThirdPartyController,
	panelEventsHandler *panelevents.ThirdPartyController,
	authHandler *thirdparty.AuthHandler,

	logger *zap.Logger,
	validator *validator.Validate,
) *thirdPartyHandler {
	return &thirdPartyHandler{
		Handler: base.Handler{
			Logger:    logger,
			Validator: validator,
		},

		usersSvc: usersSvc,
		jwtSvc:   jwtService,

		healthHandler:   healthHandler,
		messagesHandler: messagesHandler,
		webhooksHandler: webhooksHandler,
		devicesHandler:  devicesHandler,
		settingsHandler: settingsHandler,
		logsHandler:     logsHandler,
		postsHandler:       postsHandler,
		schedulesHandler:   schedulesHandler,
		testsHandler:       testsHandler,
		panelEventsHandler: panelEventsHandler,
		authHandler:        authHandler,
	}
}

func (h *thirdPartyHandler) Register(router fiber.Router) {
	router = router.Group("/3rdparty/v1")

	h.healthHandler.Register(router)

	router.Use(
		// cloud-gesvial.19.1: HoistTokenFromQuery must run BEFORE jwtauth.NewJWT
		// so EventSource browser clients (which can't set headers) can pass
		// the JWT via ?token=. No-op when an Authorization header is already
		// present, so it's safe to apply globally.
		panelevents.HoistTokenFromQuery,
		userauth.NewBasic(h.usersSvc),
		jwtauth.NewJWT(h.jwtSvc),
		userauth.UserRequired(),
	)

	h.authHandler.Register(router.Group("/auth"))

	// cloud-gesvial.19.2 C7: retirado el alias singular `/device` (deadline
	// 2025-07-11 vencida hace 9 meses). Cualquier cliente legacy que aún lo
	// use recibirá 404 — la ruta canónica `/devices` está en producción
	// desde gesvial.10. Si reaparece tráfico al singular, el log de fiber
	// 404 lo evidencia.
	h.messagesHandler.Register(router.Group("/message")) // TODO: remove after 2025-12-31
	h.messagesHandler.Register(router.Group("/messages"))

	h.devicesHandler.Register(router.Group("/devices"))

	h.settingsHandler.Register(router.Group("/settings"))

	h.webhooksHandler.Register(router.Group("/webhooks"))

	h.logsHandler.Register(router.Group("/logs"))

	h.postsHandler.Register(router.Group("/posts"))

	h.schedulesHandler.Register(router.Group("/schedules"))

	h.testsHandler.Register(router.Group("/tests"))

	// cloud-gesvial.19: SSE stream of panel-relevant events (test scheduled,
	// test completed, post status changed, gateway connect/disconnect). The
	// React panel uses EventSource on this endpoint to refresh views without
	// polling.
	h.panelEventsHandler.Register(router.Group("/events"))
}
