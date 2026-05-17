package events

import (
	"context"
	"fmt"
	"time"

	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/devices"
	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/push"
	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/sse"
	"github.com/android-sms-gateway/server/internal/sms-gateway/pubsub"
	"go.uber.org/zap"
)

const (
	pubsubTopic   = "events"
	pubsubTimeout = 5 * time.Second
)

type Service struct {
	deviceSvc *devices.Service

	sseSvc  *sse.Service
	pushSvc *push.Service

	pubsub pubsub.PubSub

	logger *zap.Logger
}

func NewService(
	devicesSvc *devices.Service,
	sseSvc *sse.Service,
	pushSvc *push.Service,
	pubsub pubsub.PubSub,
	logger *zap.Logger,
) *Service {
	return &Service{
		deviceSvc: devicesSvc,
		sseSvc:    sseSvc,
		pushSvc:   pushSvc,
		pubsub:    pubsub,
		logger:    logger,
	}
}

func (s *Service) Notify(userID string, deviceID *string, event Event) error {
	if event.EventType == "" {
		return fmt.Errorf("%w: event type is empty", ErrValidationFailed)
	}

	subCtx, cancel := context.WithTimeout(context.Background(), pubsubTimeout)
	defer cancel()

	wrapper := eventWrapper{
		UserID:   userID,
		DeviceID: deviceID,
		Event:    event,
	}

	wrapperBytes, err := wrapper.serialize()
	if err != nil {
		return fmt.Errorf("failed to serialize event wrapper: %w", err)
	}

	if pubErr := s.pubsub.Publish(subCtx, pubsubTopic, wrapperBytes); pubErr != nil {
		return fmt.Errorf("failed to publish event: %w", pubErr)
	}

	return nil
}

func (s *Service) Run(ctx context.Context) error {
	sub, err := s.pubsub.Subscribe(ctx, pubsubTopic)
	if err != nil {
		return fmt.Errorf("failed to subscribe to pubsub: %w", err)
	}
	defer sub.Close()

	ch := sub.Receive()
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Event service stopped")
			return nil
		case msg, ok := <-ch:
			if !ok {
				s.logger.Info("Subscription closed")
				return nil
			}
			wrapper := new(eventWrapper)
			if jsonErr := wrapper.deserialize(msg.Data); jsonErr != nil {
				s.logger.Error("failed to deserialize event wrapper", zap.Error(jsonErr))
				continue
			}
			s.safeProcessEvent(wrapper)
		}
	}
}

// safeProcessEvent wraps processEvent with panic recovery so a single bad
// event (nil deref in a nested handler, runtime error) doesn't take down
// the whole event loop. cloud-gesvial.19.1 M-MED-1.
func (s *Service) safeProcessEvent(wrapper *eventWrapper) {
	defer func() {
		if rec := recover(); rec != nil {
			s.logger.Error("event processing panicked",
				zap.Any("recover", rec),
				zap.String("event_type", string(wrapper.Event.EventType)),
				zap.String("event_id", wrapper.Event.ID),
			)
		}
	}()
	s.processEvent(wrapper)
}

func (s *Service) processEvent(wrapper *eventWrapper) {
	filters := []devices.SelectFilter{}
	if wrapper.DeviceID != nil {
		filters = append(filters, devices.WithID(*wrapper.DeviceID))
	}

	devices, err := s.deviceSvc.Select(wrapper.UserID, filters...)
	if err != nil {
		s.logger.Error("failed to select devices",
			zap.String("event_id", wrapper.Event.ID),
			zap.String("user_id", wrapper.UserID),
			zap.Error(err),
		)
		return
	}

	if len(devices) == 0 {
		s.logger.Info("no devices found for user",
			zap.String("event_id", wrapper.Event.ID),
			zap.String("user_id", wrapper.UserID),
		)
		return
	}

	for _, device := range devices {
		if device.PushToken != nil {
			start := time.Now()
			if enqErr := s.pushSvc.Enqueue(*device.PushToken, push.Event{
				Type: wrapper.Event.EventType,
				Data: eventDataWithID(wrapper.Event),
			}); enqErr != nil {
				s.logger.Error("event delivery failed",
					zap.String("event_id", wrapper.Event.ID),
					zap.String("event_type", string(wrapper.Event.EventType)),
					zap.String("user_id", wrapper.UserID),
					zap.String("device_id", device.ID),
					zap.String("channel", "fcm"),
					zap.Error(enqErr),
				)
			} else {
				s.logger.Info("event delivered",
					zap.String("event_id", wrapper.Event.ID),
					zap.String("event_type", string(wrapper.Event.EventType)),
					zap.String("user_id", wrapper.UserID),
					zap.String("device_id", device.ID),
					zap.String("channel", "fcm"),
					zap.Duration("latency_ms", time.Since(start)),
				)
			}
			continue
		}

		start := time.Now()
		if sseErr := s.sseSvc.Send(device.ID, sse.Event{
			Type: wrapper.Event.EventType,
			Data: eventDataWithID(wrapper.Event),
		}); sseErr != nil {
			s.logger.Error("event delivery failed",
				zap.String("event_id", wrapper.Event.ID),
				zap.String("event_type", string(wrapper.Event.EventType)),
				zap.String("user_id", wrapper.UserID),
				zap.String("device_id", device.ID),
				zap.String("channel", "sse"),
				zap.Error(sseErr),
			)
		} else {
			s.logger.Info("event delivered",
				zap.String("event_id", wrapper.Event.ID),
				zap.String("event_type", string(wrapper.Event.EventType)),
				zap.String("user_id", wrapper.UserID),
				zap.String("device_id", device.ID),
				zap.String("channel", "sse"),
				zap.Duration("latency_ms", time.Since(start)),
			)
		}
	}
}

// eventDataWithID retorna el map data del event con event_id agregado para
// que la app Android pueda deduplicar por id cuando llegue el mismo evento
// vía SSE y FCM en paralelo (Fase 3 plan QA).
func eventDataWithID(ev Event) map[string]string {
	out := make(map[string]string, len(ev.Data)+1)
	for k, v := range ev.Data {
		out[k] = v
	}
	out["event_id"] = ev.ID
	return out
}
