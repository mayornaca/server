package sse

import (
	"errors"
	"testing"

	"github.com/android-sms-gateway/client-go/smsgateway"
	"go.uber.org/zap"
)

// TestSSEService_BackpressureOnFullBuffer valida la decisión arquitectural
// del plan QA Fase 3 2026-05-17: cuando un subscriber tiene su buffer SSE
// saturado (eventsBufferSize=8), Send debe retornar ErrBufferFull explícito
// en lugar de drop silencioso. El caller (events.Service.deliverSSE) loguea
// el error con event_id para que el operador detecte el evento perdido.
//
// Pre-Fase 3: el default case del select solo emitía log warn; el Send
// retornaba ErrNoConnection cuando sent==0, confundiendo "buffer full" con
// "no hay conexión".
func TestSSEService_BackpressureOnFullBuffer(t *testing.T) {
	svc := NewService(DefaultConfig(), zap.NewNop())
	deviceID := "device-overloaded"

	// Registra una conexión "fake" sin handleStream — su channel nunca se
	// drena, simulando un cliente HTTP lento que no consume el stream.
	conn := svc.registerConnection(deviceID)
	defer close(conn.closeSignal)

	// Llena el buffer (eventsBufferSize=8). El 9° Send debe fallar con
	// ErrBufferFull, NO con ErrNoConnection.
	ev := Event{Type: smsgateway.PushMessageEnqueued, Data: map[string]string{"event_id": "evt-test"}}
	for i := 0; i < eventsBufferSize; i++ {
		if err := svc.Send(deviceID, ev); err != nil {
			t.Fatalf("send %d (buffer should accept): %v", i, err)
		}
	}

	err := svc.Send(deviceID, ev)
	if err == nil {
		t.Fatalf("expected error on full buffer, got nil")
	}
	if !errors.Is(err, ErrBufferFull) {
		t.Fatalf("expected ErrBufferFull, got: %v", err)
	}
	if errors.Is(err, ErrNoConnection) {
		t.Fatalf("got ErrNoConnection instead of ErrBufferFull — backpressure no diferenciado: %v", err)
	}
}
