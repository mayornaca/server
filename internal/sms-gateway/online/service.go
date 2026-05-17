package online

import (
	"context"
	"fmt"
	"time"

	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/devices"
	"github.com/capcom6/go-helpers/maps"
	"github.com/go-core-fx/cachefx/cache"
	"go.uber.org/zap"
)

type Service interface {
	Run(ctx context.Context) error
	SetOnline(ctx context.Context, deviceID string)
}

type service struct {
	devicesSvc *devices.Service

	cache cache.Cache

	logger *zap.Logger
}

func New(devicesSvc *devices.Service, cache cache.Cache, logger *zap.Logger) Service {
	return &service{
		devicesSvc: devicesSvc,
		cache:      cache,
		logger:     logger,
	}
}

func (s *service) Run(ctx context.Context) error {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			s.logger.Debug("Persisting online status")
			if err := s.persist(ctx); err != nil {
				s.logger.Error("failed to persist online status", zap.Error(err))
			}
		}
	}
}

func (s *service) SetOnline(ctx context.Context, deviceID string) {
	dt := time.Now().UTC().Format(time.RFC3339)

	s.logger.Debug("Setting online status", zap.String("device_id", deviceID), zap.String("last_seen", dt))

	if err := s.cache.Set(ctx, deviceID, []byte(dt)); err != nil {
		s.logger.Error("failed to set online status", zap.String("device_id", deviceID), zap.Error(err))
		return
	}

	s.logger.Debug("Online status set", zap.String("device_id", deviceID))
}

func (s *service) persist(ctx context.Context) error {
	items, err := s.cache.Drain(ctx)
	if err != nil {
		return fmt.Errorf("failed to drain cache: %w", err)
	}

	if len(items) == 0 {
		s.logger.Debug("No online statuses to persist")
		return nil
	}
	s.logger.Debug("Drained cache", zap.Int("count", len(items)))

	timestamps := maps.MapValues(items, func(v []byte) time.Time {
		t, parseErr := time.Parse(time.RFC3339, string(v))
		if parseErr != nil {
			s.logger.Warn("failed to parse last seen", zap.String("last_seen", string(v)), zap.Error(parseErr))
			return time.Now().UTC()
		}
		return t
	})

	s.logger.Debug("Parsed last seen timestamps", zap.Int("count", len(timestamps)))

	if seenErr := s.devicesSvc.SetLastSeen(ctx, timestamps); seenErr != nil {
		return fmt.Errorf("failed to set last seen: %w", seenErr)
	}

	s.logger.Info("Set last seen", zap.Int("count", len(timestamps)))
	return nil
}
