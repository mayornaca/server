package events

import (
	"github.com/android-sms-gateway/server/internal/sms-gateway/models"
	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/devices"
	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/push"
	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/sse"
)

// sseSender es la interface mínima que events.Service requiere de sse.Service.
// Permite inyectar fakes in-memory en tests sin tocar el package sse.
// El tipo concreto *sse.Service la satisface por duck-typing.
type sseSender interface {
	Send(deviceID string, event sse.Event) error
}

// pushEnqueuer es la interface mínima que events.Service requiere de push.Service.
type pushEnqueuer interface {
	Enqueue(token string, event push.Event) error
}

// deviceSelector es la interface mínima que events.Service requiere de devices.Service.
type deviceSelector interface {
	Select(userID string, filter ...devices.SelectFilter) ([]models.Device, error)
}
