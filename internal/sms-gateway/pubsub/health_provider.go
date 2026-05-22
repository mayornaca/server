package pubsub

import (
	"context"
	"time"

	pkgpubsub "github.com/android-sms-gateway/server/pkg/pubsub"
	"github.com/android-sms-gateway/server/pkg/health"
)

const healthCheckTimeout = 500 * time.Millisecond

// HealthProvider expone el state del pubsub a `/health/ready`. Ejecuta un
// round-trip publish→subscribe en un topic efímero, con timeout de 500ms.
// Si el broker está down o latencia patológica, Fail. Fase 4 plan QA.
type HealthProvider struct {
	ps pkgpubsub.PubSub
}

func NewHealthProvider(ps pkgpubsub.PubSub) *HealthProvider {
	return &HealthProvider{ps: ps}
}

// Name implements health.Provider.
func (p *HealthProvider) Name() string { return "pubsub" }

// LiveProbe pass siempre: el proceso vive aunque el pubsub esté degradado.
func (p *HealthProvider) LiveProbe(_ context.Context) (health.Checks, error) {
	return nil, nil //nolint:nilnil // empty result
}

// ReadyProbe ejecuta el round-trip. Si timeout (500ms) → Fail.
func (p *HealthProvider) ReadyProbe(ctx context.Context) (health.Checks, error) {
	probeCtx, cancel := context.WithTimeout(ctx, healthCheckTimeout)
	defer cancel()

	check := health.CheckDetail{
		Description:   "pubsub round-trip publish-subscribe",
		ObservedUnit:  "",
		ObservedValue: 0,
		Status:        health.StatusPass,
	}
	if err := pkgpubsub.HealthCheck(probeCtx, p.ps); err != nil {
		check.Status = health.StatusFail
		check.Description = "pubsub round-trip failed: " + err.Error()
	}
	return health.Checks{"roundtrip": check}, nil
}

// StartedProbe usa el mismo chequeo que ReadyProbe.
func (p *HealthProvider) StartedProbe(ctx context.Context) (health.Checks, error) {
	return p.ReadyProbe(ctx)
}

var _ health.Provider = (*HealthProvider)(nil)
