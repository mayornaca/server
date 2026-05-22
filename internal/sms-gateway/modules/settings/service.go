package settings

import (
	"context"
	"time"

	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/events"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type ServiceParams struct {
	fx.In

	Repository *repository

	EventsSvc *events.Service

	Logger *zap.Logger
}

type Service struct {
	settings *repository

	eventsSvc *events.Service

	logger *zap.Logger
}

func NewService(params ServiceParams) *Service {
	return &Service{
		settings: params.Repository,

		eventsSvc: params.EventsSvc,

		logger: params.Logger.Named("service"),
	}
}

func (s *Service) GetSettings(userID string, public bool) (map[string]any, error) {
	settings, err := s.settings.GetSettings(userID)
	if err != nil {
		return nil, err
	}

	if !public {
		return settings.Settings, nil
	}

	return filterMap(settings.Settings, rulesPublic)
}

func (s *Service) UpdateSettings(userID string, settings map[string]any) (map[string]any, error) {
	filtered, err := filterMap(settings, rules)
	if err != nil {
		return nil, err
	}

	updatedSettings, err := s.settings.UpdateSettings(NewDeviceSettings(userID, filtered))
	if err != nil {
		return nil, err
	}

	s.notifyDevices(userID)

	return filterMap(updatedSettings.Settings, rulesPublic)
}

func (s *Service) ReplaceSettings(userID string, settings map[string]any) (map[string]any, error) {
	filtered, err := filterMap(settings, rules)
	if err != nil {
		return nil, err
	}

	updated, err := s.settings.ReplaceSettings(NewDeviceSettings(userID, filtered))
	if err != nil {
		return nil, err
	}

	s.notifyDevices(userID)

	return filterMap(updated.Settings, rulesPublic)
}

// notifyDevices notifica a los devices del user de un settings update. Sync
// con timeout — Fase 3 plan QA reemplaza goroutine anónima sin await.
func (s *Service) notifyDevices(userID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.eventsSvc.Notify(ctx, userID, nil, events.NewSettingsUpdatedEvent()); err != nil {
		s.logger.Error("failed to notify devices of settings change", zap.Error(err))
	}
}
