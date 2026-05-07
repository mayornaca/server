package posts

import (
	"fmt"
	"time"

	"github.com/android-sms-gateway/server/internal/sms-gateway/models"
	"github.com/android-sms-gateway/server/internal/sms-gateway/users"
	"gorm.io/gorm"
)

type PostStatus string

const (
	PostStatusOK      PostStatus = "OK"
	PostStatusFail    PostStatus = "FAIL"
	PostStatusUnknown PostStatus = "UNKNOWN"
	PostStatusTesting PostStatus = "TESTING"
)

type SosPost struct {
	models.SoftDeletableModel

	ID           string     `json:"id"          gorm:"primaryKey;type:char(21)"`
	UserID       string     `json:"-"           gorm:"<-:create;not null;type:varchar(32);uniqueIndex:unq_sos_posts_phone,priority:1"`
	Name         string     `json:"name"        validate:"required,max=128" gorm:"not null;type:varchar(128)"`
	KmMarker     string     `json:"kmMarker"    validate:"required,max=32"  gorm:"column:km_marker;not null;type:varchar(32)"`
	PhoneNumber  string     `json:"phoneNumber" validate:"required,max=20"  gorm:"column:phone_number;not null;type:varchar(20);uniqueIndex:unq_sos_posts_phone,priority:2"`
	Status       PostStatus `json:"status"                                  gorm:"not null;type:varchar(20);default:UNKNOWN;index:idx_sos_posts_status"`
	Disabled     bool       `json:"disabled"                                gorm:"not null;default:false;index:idx_sos_posts_disabled"`
	LastTestDate *time.Time `json:"lastTestDate,omitempty"                   gorm:"column:last_test_date;type:datetime(3)"`

	User users.User `json:"-" gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`
}

func (SosPost) TableName() string {
	return "sos_posts"
}

func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(new(SosPost)); err != nil {
		return fmt.Errorf("sos_posts migration failed: %w", err)
	}
	return nil
}
