package posts

import (
	"strings"

	"gorm.io/gorm"
)

const adminUserID = "__ADMIN__"

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

func WithStatus(status PostStatus) SelectFilter {
	return func(f *selectFilter) {
		f.status = &status
	}
}

func WithSearch(search string) SelectFilter {
	return func(f *selectFilter) {
		f.search = &search
	}
}

// WithOnlyEnabled filters out posts with disabled=true.
func WithOnlyEnabled() SelectFilter {
	return func(f *selectFilter) {
		t := true
		f.onlyEnabled = &t
	}
}

// WithIncludeDisabled is a no-op, intended for documentation/readability
// (default Select already returns both enabled and disabled posts).
func WithIncludeDisabled() SelectFilter {
	return func(f *selectFilter) {
		f.onlyEnabled = nil
	}
}

type selectFilter struct {
	userID      string
	id          *string
	status      *PostStatus
	search      *string
	onlyEnabled *bool
}

func newFilter(filters ...SelectFilter) *selectFilter {
	f := new(selectFilter)
	for _, filter := range filters {
		filter(f)
	}
	return f
}

func (f *selectFilter) apply(query *gorm.DB) *gorm.DB {
	// cloud-gesvial.19.1: SoftDeletableModel uses *time.Time, GORM does NOT
	// auto-inject deleted_at IS NULL. Without this, deleted posts reappear
	// in list/get/summary endpoints.
	query = query.Where("deleted_at IS NULL")
	if f.userID != "" && f.userID != adminUserID {
		query = query.Where("user_id = ?", f.userID)
	}
	if f.id != nil {
		query = query.Where("id = ?", *f.id)
	}
	if f.status != nil {
		query = query.Where("status = ?", *f.status)
	}
	if f.search != nil {
		escaped := strings.NewReplacer(`%`, `|%`, `_`, `|_`, `|`, `||`).Replace(*f.search)
		like := "%" + escaped + "%"
		query = query.Where("(name LIKE ? ESCAPE '|' OR km_marker LIKE ? ESCAPE '|' OR phone_number LIKE ? ESCAPE '|')", like, like, like)
	}
	if f.onlyEnabled != nil && *f.onlyEnabled {
		query = query.Where("disabled = ?", false)
	}
	return query
}
