package events

import (
	"testing"

	"github.com/android-sms-gateway/client-go/smsgateway"
	"github.com/android-sms-gateway/server/internal/sms-gateway/models"
	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/devices"
	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/push"
	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/sse"
	"go.uber.org/zap"
)

// fakeSSE es un sseSender que registra cada Send para inspección post-call.
type fakeSSE struct {
	sends []sseSendCall
}

type sseSendCall struct {
	DeviceID string
	Event    sse.Event
}

func (f *fakeSSE) Send(deviceID string, event sse.Event) error {
	f.sends = append(f.sends, sseSendCall{deviceID, event})
	return nil
}

// fakePush es un pushEnqueuer que registra cada Enqueue.
type fakePush struct {
	enqueues []pushEnqueueCall
}

type pushEnqueueCall struct {
	Token string
	Event push.Event
}

func (f *fakePush) Enqueue(token string, event push.Event) error {
	f.enqueues = append(f.enqueues, pushEnqueueCall{token, event})
	return nil
}

// fakeDevices es un deviceSelector que retorna una lista fija.
type fakeDevices struct {
	devices []models.Device
}

func (f *fakeDevices) Select(_ string, _ ...devices.SelectFilter) ([]models.Device, error) {
	return f.devices, nil
}

// TestService_PublishesByBothSSEAndFCMWhenDeviceHasBoth valida la decisión
// arquitectural 3 del plan QA 2026-05-17: cuando un device tiene PushToken
// activo, el servidor envía el evento por SSE Y FCM en paralelo (no excluyente).
// La app deduplica por event_id.
//
// Pre-Fase 3: events/service.go:processEvent hacía if/else excluyente —
// si PushToken != nil → solo FCM, ignorando SSE. Si la app perdía el SSE
// (Doze, swipe-out) y el push también fallaba (silencioso), el evento se
// perdía. El fan-out paralelo es la red de seguridad.
func TestService_PublishesByBothSSEAndFCMWhenDeviceHasBoth(t *testing.T) {
	pushToken := "fcm-token-abc"
	device := models.Device{
		ID:        "device-1",
		UserID:    "user-1",
		PushToken: &pushToken,
	}

	fSSE := &fakeSSE{}
	fPush := &fakePush{}

	svc := &Service{
		deviceSvc: &fakeDevices{devices: []models.Device{device}},
		sseSvc:    fSSE,
		pushSvc:   fPush,
		logger:    zap.NewNop(),
	}

	ev := NewEvent(smsgateway.PushMessageEnqueued, nil)
	wrapper := &eventWrapper{
		UserID: "user-1",
		Event:  ev,
	}
	svc.processEvent(wrapper)

	if len(fPush.enqueues) != 1 {
		t.Errorf("expected 1 push enqueue (FCM), got %d", len(fPush.enqueues))
	}
	if len(fSSE.sends) != 1 {
		t.Errorf("expected 1 sse send (parallel fan-out), got %d", len(fSSE.sends))
	}

	if len(fPush.enqueues) == 1 {
		if got := fPush.enqueues[0].Event.Data["event_id"]; got != ev.ID {
			t.Errorf("push enqueue event_id mismatch: got %q, want %q", got, ev.ID)
		}
	}
	if len(fSSE.sends) == 1 {
		if got := fSSE.sends[0].Event.Data["event_id"]; got != ev.ID {
			t.Errorf("sse send event_id mismatch: got %q, want %q", got, ev.ID)
		}
	}
}
