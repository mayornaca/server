package pubsub

import (
	"context"
	"errors"
	"fmt"

	"github.com/jaevor/go-nanoid"
)

// ErrHealthCheckTimeout indica que el round-trip publish→subscribe no
// completó dentro del ctx provisto. Probable causa: cliente desconectado
// del broker, broker down, o latencia patológica.
var ErrHealthCheckTimeout = errors.New("pubsub health check timeout")

// HealthCheck publica un payload único en un topic efímero y verifica que
// el subscriber lo reciba antes de que ctx.Done(). Confirma que publish y
// subscribe están operativos end-to-end — no solo el state interno del
// cliente. Llamado por `/health/ready` con ctx de 500ms (Fase 4 plan QA).
func HealthCheck(ctx context.Context, ps PubSub) error {
	idgen, err := nanoid.Standard(21)
	if err != nil {
		return fmt.Errorf("idgen: %w", err)
	}
	topic := "_health-check-" + idgen()
	payload := []byte("ping-" + idgen())

	sub, err := ps.Subscribe(ctx, topic)
	if err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}
	defer sub.Close()

	// Publish va en goroutine porque el MemoryPubSub bloquea hasta que
	// algún subscriber lea (channel unbuffered por default). Sin esto, el
	// Receive de abajo nunca recibiría: Publish esperaría a Receive,
	// Receive esperaría a Publish → deadlock hasta ctx.Done.
	pubErr := make(chan error, 1)
	go func() {
		pubErr <- ps.Publish(ctx, topic, payload)
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("%w: %w", ErrHealthCheckTimeout, ctx.Err())
	case err := <-pubErr:
		if err != nil {
			return fmt.Errorf("publish: %w", err)
		}
		// Publish completó (subscriber recibió); ahora lee el mensaje.
		select {
		case <-ctx.Done():
			return fmt.Errorf("%w: %w", ErrHealthCheckTimeout, ctx.Err())
		case msg, ok := <-sub.Receive():
			if !ok {
				return errors.New("subscription channel closed before round-trip")
			}
			if string(msg.Data) != string(payload) {
				return fmt.Errorf("payload mismatch: sent %q got %q", payload, msg.Data)
			}
			return nil
		}
	case msg, ok := <-sub.Receive():
		if !ok {
			return errors.New("subscription channel closed before round-trip")
		}
		if string(msg.Data) != string(payload) {
			return fmt.Errorf("payload mismatch: sent %q got %q", payload, msg.Data)
		}
		return nil
	}
}
