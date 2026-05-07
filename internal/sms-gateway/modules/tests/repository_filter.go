package tests

import (
	"time"

	"gorm.io/gorm"
)

type SelectFilter func(*selectFilter)

func WithUserID(userID string) SelectFilter {
	return func(f *selectFilter) {
		f.userID = userID
	}
}

func WithID(id string) SelectFilter {
	return func(f *selectFilter) {
		f.id = &id
	}
}

func WithPostID(postID string) SelectFilter {
	return func(f *selectFilter) {
		f.postID = &postID
	}
}

func WithPostIDs(postIDs []string) SelectFilter {
	return func(f *selectFilter) {
		f.postIDs = postIDs
	}
}

func WithDeviceID(deviceID string) SelectFilter {
	return func(f *selectFilter) {
		f.deviceID = &deviceID
	}
}

func WithTestType(testType TestType) SelectFilter {
	return func(f *selectFilter) {
		f.testType = &testType
	}
}

func WithStatus(status TestStatus) SelectFilter {
	return func(f *selectFilter) {
		f.status = &status
	}
}

func WithFrom(from time.Time) SelectFilter {
	return func(f *selectFilter) {
		f.from = &from
	}
}

func WithTo(to time.Time) SelectFilter {
	return func(f *selectFilter) {
		f.to = &to
	}
}

func WithLimit(limit int) SelectFilter {
	return func(f *selectFilter) {
		f.limit = limit
	}
}

func WithOffset(offset int) SelectFilter {
	return func(f *selectFilter) {
		f.offset = offset
	}
}

// WithCreatedAfter returns tests with created_at strictly greater than t.
func WithCreatedAfter(t time.Time) SelectFilter {
	return func(f *selectFilter) {
		f.createdAfter = &t
	}
}

type selectFilter struct {
	userID       string
	id           *string
	postID       *string
	postIDs      []string
	deviceID     *string
	testType     *TestType
	status       *TestStatus
	from         *time.Time
	to           *time.Time
	createdAfter *time.Time
	limit        int
	offset       int
}

func newFilter(filters ...SelectFilter) *selectFilter {
	f := &selectFilter{limit: 50}
	for _, filter := range filters {
		filter(f)
	}
	return f
}

const adminUserID = "__ADMIN__"

func (f *selectFilter) applyFilters(query *gorm.DB) *gorm.DB {
	// cloud-gesvial.19.1: soft-deleted rows must be invisible to every selector.
	// SoftDeletableModel uses *time.Time (not gorm.DeletedAt), so GORM does NOT
	// inject this clause automatically. Pre-fix Cancel/superseded entries were
	// returned by the dedupe SelectOne, blocking new schedules silently.
	query = query.Where("deleted_at IS NULL")
	if f.userID != "" && f.userID != adminUserID {
		query = query.Where("user_id = ?", f.userID)
	}
	if f.id != nil {
		query = query.Where("id = ?", *f.id)
	}
	if f.postID != nil {
		query = query.Where("post_id = ?", *f.postID)
	}
	if len(f.postIDs) > 0 {
		query = query.Where("post_id IN ?", f.postIDs)
	}
	if f.deviceID != nil {
		query = query.Where("device_id = ?", *f.deviceID)
	}
	if f.testType != nil {
		query = query.Where("test_type = ?", *f.testType)
	}
	if f.status != nil {
		query = query.Where("status = ?", *f.status)
	}
	if f.from != nil {
		query = query.Where("created_at >= ?", *f.from)
	}
	if f.to != nil {
		query = query.Where("created_at <= ?", *f.to)
	}
	if f.createdAfter != nil {
		query = query.Where("created_at > ?", *f.createdAfter)
	}
	return query
}

func (f *selectFilter) apply(query *gorm.DB) *gorm.DB {
	query = f.applyFilters(query)
	if f.limit > 0 {
		query = query.Limit(f.limit)
	}
	if f.offset > 0 {
		query = query.Offset(f.offset)
	}
	return query
}

func (f *selectFilter) applyWithoutPagination(query *gorm.DB) *gorm.DB {
	return f.applyFilters(query)
}