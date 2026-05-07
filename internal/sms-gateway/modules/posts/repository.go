package posts

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Select(filters ...SelectFilter) ([]*SosPost, error) {
	posts := []*SosPost{}
	if err := newFilter(filters...).apply(r.db).Order("name ASC").Find(&posts).Error; err != nil {
		return nil, err
	}
	return posts, nil
}

func (r *Repository) SelectOne(filters ...SelectFilter) (*SosPost, error) {
	post := new(SosPost)
	if err := newFilter(filters...).apply(r.db).First(post).Error; err != nil {
		return nil, err
	}
	return post, nil
}

func (r *Repository) Count(filters ...SelectFilter) (int64, error) {
	var count int64
	if err := newFilter(filters...).apply(r.db).Model(new(SosPost)).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// CountByStatus returns post counts grouped by status, optionally including
// disabled posts. cloud-gesvial.20.1: when `includeDisabled` is false (the
// default for /posts/summary), disabled posts are excluded from every bucket
// AND from the implicit total — so Total computed by the caller equals
// OK+FAIL+UNKNOWN+TESTING. Pre-fix the dashboard showed Total=12 with
// OK+FAIL+UNKNOWN=11, the missing 1 being a disabled post counted only in
// Total. Pass `includeDisabled=true` to recover legacy behaviour (e.g. for
// admin auditing UIs that explicitly want to see disabled).
func (r *Repository) CountByStatus(userID string, includeDisabled bool) (map[string]int, error) {
	type result struct {
		Status string
		Count  int
	}
	var results []result
	query := r.db.Model(new(SosPost)).
		Select("status, COUNT(*) as count").
		Where("deleted_at IS NULL")
	if !includeDisabled {
		query = query.Where("disabled = ?", false)
	}
	if userID != "" && userID != "__ADMIN__" {
		query = query.Where("user_id = ?", userID)
	}
	if err := query.Group("status").
		Find(&results).Error; err != nil {
		return nil, err
	}
	m := make(map[string]int)
	for _, r := range results {
		m[r.Status] = r.Count
	}
	return m, nil
}

func (r *Repository) Insert(post *SosPost) error {
	return r.db.Create(post).Error
}

// Upsert is the path used by the Android gateway sync (POST /mobile/v1/posts).
// The app does NOT know about server-authoritative fields and always sends the
// Kotlin zero value for them — `disabled=false`, `status="UNKNOWN"`, no
// `last_test_date`. cloud-gesvial.19.1: removed `disabled`, `status`, and
// `last_test_date` from the on-conflict update set so a sync cycle never
// overwrites operator-managed flags or test-result-derived state. The fields
// still get inserted on first creation (the InsertDefaults path), which is
// the only time we want the app's notion of them to land.
func (r *Repository) Upsert(posts []*SosPost) error {
	if len(posts) == 0 {
		return nil
	}
	return r.db.
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"name", "km_marker", "phone_number", "deleted_at"}),
		}).
		Create(&posts).
		Error
}

func (r *Repository) Update(post *SosPost) error {
	return r.db.Save(post).Error
}

// UpdateFields applies a partial update with WHERE id=? AND deleted_at IS NULL.
// Use this for concurrent single-field updates (e.g. bumping LastTestDate from
// a goroutine that ran in parallel with a PATCH). cloud-gesvial.19.1: avoids
// the lost-update race where Save() rewrites every column with whatever the
// in-memory struct holds, silently overwriting concurrent writes.
func (r *Repository) UpdateFields(id string, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	return r.db.Model(new(SosPost)).
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(fields).Error
}

func (r *Repository) Delete(filters ...SelectFilter) error {
	return newFilter(filters...).apply(r.db).Delete(new(SosPost)).Error
}
