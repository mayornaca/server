package schedules

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Select(filters ...SelectFilter) ([]*TestSchedule, error) {
	schedules := []*TestSchedule{}
	if err := newFilter(filters...).apply(r.db).Order("name ASC").Find(&schedules).Error; err != nil {
		return nil, err
	}
	return schedules, nil
}

func (r *Repository) SelectOne(filters ...SelectFilter) (*TestSchedule, error) {
	schedule := new(TestSchedule)
	if err := newFilter(filters...).apply(r.db).First(schedule).Error; err != nil {
		return nil, err
	}
	return schedule, nil
}

// SelectAllEnabled returns every enabled schedule across all users.
// Used by the cron dispatcher to evaluate due rules.
func (r *Repository) SelectAllEnabled(ctx context.Context) ([]*TestSchedule, error) {
	schedules := []*TestSchedule{}
	if err := r.db.WithContext(ctx).
		Where("enabled = ?", true).
		Find(&schedules).Error; err != nil {
		return nil, err
	}
	return schedules, nil
}

func (r *Repository) Insert(schedule *TestSchedule) error {
	return r.db.Create(schedule).Error
}

func (r *Repository) Update(schedule *TestSchedule) error {
	return r.db.Save(schedule).Error
}

// UpdateLastRunAt atomically bumps the last_run_at column.
func (r *Repository) UpdateLastRunAt(ctx context.Context, id string, at time.Time) error {
	return r.db.WithContext(ctx).
		Model(new(TestSchedule)).
		Where("id = ?", id).
		Update("last_run_at", at).Error
}

func (r *Repository) Delete(filters ...SelectFilter) error {
	return newFilter(filters...).apply(r.db).Delete(new(TestSchedule)).Error
}
