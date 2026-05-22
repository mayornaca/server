package schedules

import (
	"fmt"
	"time"

	"github.com/android-sms-gateway/server/internal/sms-gateway/models"
	"github.com/android-sms-gateway/server/internal/sms-gateway/users"
	"gorm.io/gorm"
)

// TestSchedule represents a cron-based rule that triggers periodic tests
// against a set of SOS posts belonging to a user.
type TestSchedule struct {
	models.SoftDeletableModel

	ID     string `json:"id"     gorm:"primaryKey;type:char(21)"`
	UserID string `json:"userId" gorm:"<-:create;not null;type:varchar(32);index:idx_test_schedules_user"`

	Name             string     `json:"name"             validate:"required,max=64"  gorm:"not null;type:varchar(64)"`
	CronExpression   string     `json:"cronExpression"   validate:"required,max=120" gorm:"column:cron_expression;not null;type:varchar(120)"`
	TestType         string     `json:"testType"         validate:"required,oneof=SMS CONNECTIVITY AUDIO_MIC AUDIO_SPEAKER" gorm:"column:test_type;not null;type:varchar(32)"`
	Enabled          bool       `json:"enabled"                                      gorm:"not null;default:true"`
	OnlyEnabledPosts bool       `json:"onlyEnabledPosts"                             gorm:"column:only_enabled_posts;not null;default:true"`
	// CancelPending: when true, the cron dispatcher cancels any in-flight
	// PENDING test for the same post+testType BEFORE creating the new one.
	// Default false (legacy behaviour: dedupe blocks the new run). Set true
	// when the operator wants the cron to ALWAYS run on schedule, not be
	// silently skipped because a previous tick is still mid-flight or stuck.
	CancelPending    bool       `json:"cancelPending"                                gorm:"column:cancel_pending;not null;default:false"`
	FilterPostIDs    []string   `json:"filterPostIds,omitempty"                      gorm:"column:filter_post_ids;type:json;serializer:json"`
	DeviceID         *string    `json:"deviceId,omitempty"                           gorm:"column:device_id;type:char(21)"`
	LastRunAt        *time.Time `json:"lastRunAt,omitempty"                          gorm:"column:last_run_at;type:datetime(3)"`

	User users.User `json:"-" gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`
}

func (TestSchedule) TableName() string {
	return "test_schedules"
}

func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(new(TestSchedule)); err != nil {
		return fmt.Errorf("test_schedules migration failed: %w", err)
	}
	return nil
}
