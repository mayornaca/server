package webhooks

import (
	"github.com/android-sms-gateway/client-go/smsgateway"
	"gorm.io/gorm"
)

type SelectFilter func(*selectFilter)

func WithExtID(extID string) SelectFilter {
	return func(f *selectFilter) {
		f.extID = &extID
	}
}

func WithUserID(userID string) SelectFilter {
	return func(f *selectFilter) {
		f.userID = userID
	}
}

// WithEvent filters webhooks by their subscribed event type (e.g. "sms:sent").
// Used by the server-side dispatcher to locate webhooks that should fire for
// a given in-flight event.
func WithEvent(event smsgateway.WebhookEvent) SelectFilter {
	return func(f *selectFilter) {
		f.event = &event
	}
}

type selectFilter struct {
	userID        string
	extID         *string
	deviceID      *string
	deviceIDExact bool
	event         *smsgateway.WebhookEvent
}

func newFilter(filters ...SelectFilter) *selectFilter {
	f := new(selectFilter)
	f.merge(filters...)
	return f
}

func (f *selectFilter) merge(filters ...SelectFilter) {
	for _, filter := range filters {
		filter(f)
	}
}

// WithDeviceID creates a SelectFilter that filters by device ID.
// If exact is true, only records with the exact device ID are matched.
// If exact is false, records with the device ID or with a null device ID are matched.
func WithDeviceID(deviceID string, exact bool) SelectFilter {
	return func(f *selectFilter) {
		f.deviceID = &deviceID
		f.deviceIDExact = exact
	}
}

func (f *selectFilter) apply(query *gorm.DB) *gorm.DB {
	query = query.Where("user_id = ?", f.userID)
	if f.extID != nil {
		query = query.Where("ext_id = ?", *f.extID)
	}
	if f.deviceID != nil {
		if f.deviceIDExact {
			query = query.Where("device_id = ?", *f.deviceID)
		} else {
			query = query.Where("device_id = ? OR device_id IS NULL", *f.deviceID)
		}
	}
	if f.event != nil {
		query = query.Where("event = ?", *f.event)
	}
	return query
}
