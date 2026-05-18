package push

import (
	"context"

	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/push/client"
	"github.com/android-sms-gateway/server/pkg/health"
)

// HealthProvider expone el state del backend de push a `/health/ready`.
// Bajo cualquiera de las 3 implementaciones de client.Client (fcm, upstream,
// noop), HealthCheck retorna nil si está listo para enviar. FCM falla con
// ErrNotInitialized si Open() no corrió. Fase 4 plan QA 2026-05-17.
type HealthProvider struct {
	client client.Client
}

// NewHealthProvider construye el provider con el client subyacente.
// Registrado via fx.Provide + health.AsHealthProvider en Module().
func NewHealthProvider(c client.Client) *HealthProvider {
	return &HealthProvider{client: c}
}

// Name implements health.Provider.
func (p *HealthProvider) Name() string { return "push" }

// LiveProbe siempre pass: la liveness del proceso no depende del backend
// de push (el server arranca aunque FCM esté caído).
func (p *HealthProvider) LiveProbe(_ context.Context) (health.Checks, error) {
	return nil, nil //nolint:nilnil // empty result
}

// ReadyProbe delega a client.HealthCheck. Si falla, /health/ready devuelve
// Fail y el orquestador detiene tráfico al pod.
func (p *HealthProvider) ReadyProbe(ctx context.Context) (health.Checks, error) {
	check := health.CheckDetail{
		Description:   "push backend initialization",
		ObservedUnit:  "",
		ObservedValue: 0,
		Status:        health.StatusPass,
	}
	if err := p.client.HealthCheck(ctx); err != nil {
		check.Status = health.StatusFail
		check.Description = "push backend not ready: " + err.Error()
	}
	return health.Checks{"init": check}, nil
}

// StartedProbe usa el mismo chequeo que ReadyProbe — si el client no
// inicializó al boot, el startup no completó.
func (p *HealthProvider) StartedProbe(ctx context.Context) (health.Checks, error) {
	return p.ReadyProbe(ctx)
}

var _ health.Provider = (*HealthProvider)(nil)
