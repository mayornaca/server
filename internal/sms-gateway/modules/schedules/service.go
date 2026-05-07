package schedules

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/db"
	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/events"
	"github.com/robfig/cron/v3"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var (
	ErrNotFound        = errors.New("schedule not found")
	ErrInvalidCronExpr = errors.New("invalid cron expression")
)

// cronParser is the standard 5-field cron parser (minute hour dom month dow).
// Shared at package level so validation and FetchDue use the same dialect.
var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

type ServiceParams struct {
	fx.In

	IDGen db.IDGen

	Schedules *Repository

	EventsSvc *events.Service

	Logger *zap.Logger
}

type Service struct {
	idgen db.IDGen

	schedules *Repository

	eventsSvc *events.Service

	logger *zap.Logger
}

func NewService(params ServiceParams) *Service {
	return &Service{
		idgen:     params.IDGen,
		schedules: params.Schedules,
		eventsSvc: params.EventsSvc,
		logger:    params.Logger,
	}
}

// notifyDevices asynchronously emits SettingsUpdatedEvent so the gateway app
// re-pulls settings (including the updated testing.schedules list).
func (s *Service) notifyDevices(userID string) {
	go func(userID string) {
		if err := s.eventsSvc.Notify(userID, nil, events.NewSettingsUpdatedEvent()); err != nil {
			s.logger.Warn("failed to notify devices of schedule change", zap.Error(err))
		}
	}(userID)
}

// ValidateCronExpression returns nil when expr parses as a standard 5-field cron.
func ValidateCronExpression(expr string) error {
	if _, err := cronParser.Parse(expr); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidCronExpr, err.Error())
	}
	return nil
}

func (s *Service) Select(userID string, filters ...SelectFilter) ([]*TestSchedule, error) {
	filters = append(filters, WithUserID(userID))
	items, err := s.schedules.Select(filters...)
	if err != nil {
		return nil, fmt.Errorf("failed to select schedules: %w", err)
	}
	return items, nil
}

func (s *Service) Get(userID, id string) (*TestSchedule, error) {
	item, err := s.schedules.SelectOne(WithUserID(userID), WithID(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get schedule: %w", err)
	}
	return item, nil
}

func (s *Service) Create(userID string, schedule *TestSchedule) error {
	if err := ValidateCronExpression(schedule.CronExpression); err != nil {
		return err
	}
	if schedule.ID == "" {
		schedule.ID = s.idgen()
	}
	schedule.UserID = userID
	if err := s.schedules.Insert(schedule); err != nil {
		return fmt.Errorf("failed to create schedule: %w", err)
	}
	s.notifyDevices(userID)
	return nil
}

func (s *Service) Update(userID, id string, schedule *TestSchedule) (*TestSchedule, error) {
	existing, err := s.Get(userID, id)
	if err != nil {
		return nil, err
	}

	if schedule.CronExpression != "" && schedule.CronExpression != existing.CronExpression {
		if err := ValidateCronExpression(schedule.CronExpression); err != nil {
			return nil, err
		}
		existing.CronExpression = schedule.CronExpression
	}
	if schedule.Name != "" {
		existing.Name = schedule.Name
	}
	if schedule.TestType != "" {
		existing.TestType = schedule.TestType
	}
	existing.Enabled = schedule.Enabled
	existing.OnlyEnabledPosts = schedule.OnlyEnabledPosts
	existing.FilterPostIDs = schedule.FilterPostIDs
	existing.DeviceID = schedule.DeviceID

	if err := s.schedules.Update(existing); err != nil {
		return nil, fmt.Errorf("failed to update schedule: %w", err)
	}
	s.notifyDevices(userID)
	return existing, nil
}

func (s *Service) Delete(userID, id string) error {
	if _, err := s.Get(userID, id); err != nil {
		return err
	}
	if err := s.schedules.Delete(WithUserID(userID), WithID(id)); err != nil {
		return fmt.Errorf("failed to delete schedule: %w", err)
	}
	s.notifyDevices(userID)
	return nil
}

// FetchDue returns enabled schedules whose cron expression fires between
// lastEval (exclusive) and now (inclusive), grouped by user.
// A schedule without LastRunAt starts counting from lastEval.
func (s *Service) FetchDue(ctx context.Context, lastEval, now time.Time) ([]*TestSchedule, error) {
	all, err := s.schedules.SelectAllEnabled(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to select enabled schedules: %w", err)
	}

	due := make([]*TestSchedule, 0, len(all))
	for _, sch := range all {
		schedExpr, parseErr := cronParser.Parse(sch.CronExpression)
		if parseErr != nil {
			s.logger.Warn("invalid cron expression stored in db",
				zap.String("scheduleID", sch.ID),
				zap.String("cron", sch.CronExpression),
				zap.Error(parseErr))
			continue
		}

		// Anchor is the last run time, or the previous evaluation boundary,
		// whichever is more recent. Using max(lastRun, lastEval) prevents
		// a just-created schedule from immediately firing on its first tick
		// if the cron would have matched between "creation" and "now".
		anchor := lastEval
		if sch.LastRunAt != nil && sch.LastRunAt.After(anchor) {
			anchor = *sch.LastRunAt
		}

		nextFire := schedExpr.Next(anchor)
		if !nextFire.After(now) {
			due = append(due, sch)
		}
	}

	return due, nil
}

// MarkRun persists last_run_at for the given schedule.
func (s *Service) MarkRun(ctx context.Context, id string, at time.Time) error {
	if err := s.schedules.UpdateLastRunAt(ctx, id, at); err != nil {
		return fmt.Errorf("failed to update last_run_at: %w", err)
	}
	return nil
}
