package worker

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestConnectWithRetry_SucceedsAfterTransientFailures verifica que cuando la
// función probe falla algunos intentos transitorios y luego tiene éxito,
// connectWithRetry retorna nil y reporta el número correcto de intentos.
// Reproduce el escenario real "MariaDB reiniciando durante `docker compose
// restart db`": probe falla unos segundos con `connection refused`, después
// el listener vuelve y probe succeed.
func TestConnectWithRetry_SucceedsAfterTransientFailures(t *testing.T) {
	attempts := 0
	probe := func(ctx context.Context) error {
		attempts++
		if attempts < 3 {
			return errors.New("connection refused")
		}
		return nil
	}

	cfg := RetryConfig{
		MaxAttempts: 5,
		InitialWait: 1 * time.Millisecond,
		MaxWait:     5 * time.Millisecond,
	}

	if err := connectWithRetry(context.Background(), probe, cfg); err != nil {
		t.Fatalf("expected nil error after transient failures, got: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("expected exactly 3 probe attempts, got %d", attempts)
	}
}

// TestConnectWithRetry_HonorsContextCancellation verifica que cuando el
// contexto se cancela durante un sleep entre intentos, connectWithRetry
// retorna context.Canceled rápidamente en lugar de seguir agotando el
// backoff. Reproduce el escenario "operador hace Ctrl+C / shutdown del
// pod mientras el worker está reintentando conexión a DB caída".
func TestConnectWithRetry_HonorsContextCancellation(t *testing.T) {
	probe := func(ctx context.Context) error {
		return errors.New("connection refused")
	}

	cfg := RetryConfig{
		MaxAttempts: 100,
		InitialWait: 100 * time.Millisecond,
		MaxWait:     100 * time.Millisecond,
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	err := connectWithRetry(ctx, probe, cfg)
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("expected fast return on cancel (<500ms), took %v", elapsed)
	}
}
