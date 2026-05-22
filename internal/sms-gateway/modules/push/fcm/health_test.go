package fcm

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"
)

// TestClient_HealthCheck_FailsBeforeOpen valida que un Client recién
// construido pero sin Open() retorna ErrNotInitialized. Eso protege el
// `/health/ready` de reportar Pass cuando el SA JSON no se cargó por un
// fallo de boot (archivo missing, invalid JSON) — pre-Fase 4 el handler
// simplemente delegaba al pkg/health/system sin saber del FCM.
//
// Fase 4 plan QA 2026-05-17.
func TestClient_HealthCheck_FailsBeforeOpen(t *testing.T) {
	c, err := New(map[string]string{}, zap.NewNop())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := c.HealthCheck(context.Background()); !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("expected ErrNotInitialized before Open(), got: %v", err)
	}
}

// TestClient_HealthCheck_PassesAfterOpenStub stub-injects un client no-nil
// para verificar que tras Open() exitoso (representado por client != nil)
// HealthCheck retorna nil. No tocamos Google ni un SA JSON real — testea
// la semántica del state local.
func TestClient_HealthCheck_PassesAfterOpenStub(t *testing.T) {
	c, _ := New(map[string]string{}, zap.NewNop())
	// stub-inject: estamos en el mismo package, podemos tocar el campo
	// `c.client` para simular post-Open sin necesidad de conectividad real.
	// El tipo *messaging.Client requiere un valor no nil — usamos un cast
	// con unsafe imposible; en su lugar marcamos el state alternativo
	// mediante un flag.
	c.markInitializedForTest()

	if err := c.HealthCheck(context.Background()); err != nil {
		t.Fatalf("expected nil after stub init, got: %v", err)
	}
}
