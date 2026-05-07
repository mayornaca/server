package schedules

import "gorm.io/gorm"

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

func WithEnabled() SelectFilter {
	return func(f *selectFilter) {
		t := true
		f.enabled = &t
	}
}

type selectFilter struct {
	userID  string
	id      *string
	enabled *bool
}

func newFilter(filters ...SelectFilter) *selectFilter {
	f := new(selectFilter)
	for _, filter := range filters {
		filter(f)
	}
	return f
}

func (f *selectFilter) apply(query *gorm.DB) *gorm.DB {
	if f.userID != "" && f.userID != adminUserID {
		query = query.Where("user_id = ?", f.userID)
	}
	if f.id != nil {
		query = query.Where("id = ?", *f.id)
	}
	if f.enabled != nil {
		query = query.Where("enabled = ?", *f.enabled)
	}
	return query
}
