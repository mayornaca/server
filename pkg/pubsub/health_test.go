package pubsub

import (
	"context"
	"testing"
	"time"
)

// TestHealthCheck_RoundTripSucceeds valida que HealthCheck publica + recibe
// un mensaje de prueba via la implementación in-memory en <500ms. Es el
// check que `/health/ready` ejecuta en cada poll del orquestador (k8s,
// docker healthcheck) para confirmar que pubsub no está degradado.
//
// Fase 4 plan QA 2026-05-17.
func TestHealthCheck_RoundTripSucceeds(t *testing.T) {
	ps := NewMemory()
	defer ps.Close() //nolint:errcheck // test cleanup

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	if err := HealthCheck(ctx, ps); err != nil {
		t.Fatalf("expected HealthCheck Pass on healthy memory pubsub, got: %v", err)
	}
}
