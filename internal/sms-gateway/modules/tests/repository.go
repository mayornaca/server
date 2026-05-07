package tests

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Select(filters ...SelectFilter) ([]*TestResult, error) {
	results := []*TestResult{}
	if err := newFilter(filters...).apply(r.db).Order("created_at DESC").Find(&results).Error; err != nil {
		return nil, err
	}
	return results, nil
}

func (r *Repository) SelectOne(filters ...SelectFilter) (*TestResult, error) {
	result := new(TestResult)
	if err := newFilter(filters...).apply(r.db).First(result).Error; err != nil {
		return nil, err
	}
	return result, nil
}

func (r *Repository) InsertBatch(results []*TestResult) error {
	if len(results) == 0 {
		return nil
	}
	// cloud-gesvial.20.3: include the four classification columns in the
	// ON DUPLICATE KEY UPDATE list. Without them, when the schedule path
	// inserts a PENDING row first and then Service.Report comes in with
	// the finalised result + classifier-derived evidence on the SAME ID,
	// the UPSERT updates status/details/error/completed_at but silently
	// drops delivery_confirmed/delivery_carrier/delivery_at/failure_kind
	// — the panel ends up with the right status but no transport-layer
	// evidence, so the dual badge never lights up. Audited 2026-04-30
	// after a real lote produced 10 finalised tests, all with NULL on
	// the four new columns despite Service.applyClassification setting
	// them correctly in-memory.
	return r.db.
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"status", "details", "error", "completed_at",
				"call_record_id", "message_id", "fft_analysis_json",
				"autonomous", "deleted_at",
				"delivery_confirmed", "delivery_at", "delivery_carrier", "failure_kind",
			}),
		}).
		Create(&results).
		Error
}

func (r *Repository) Count(filters ...SelectFilter) (int64, error) {
	var count int64
	if err := newFilter(filters...).applyWithoutPagination(r.db).Model(new(TestResult)).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *Repository) Insert(result *TestResult) error {
	return r.db.Create(result).Error
}

// Update saves the mutable fields of a TestResult. Used by Service.Cancel
// to flip a PENDING into ERROR with a reason, while preserving the original
// CreatedAt and id. Only updates a curated column set (status/error/completed_at)
// so callers don't accidentally clobber metadata.
func (r *Repository) Update(result *TestResult) error {
	return r.db.
		Model(result).
		Select("status", "error", "completed_at").
		Updates(result).
		Error
}

// SelectMostRecentFinalised returns the most recently finalised
// (PASSED/FAILED/ERROR) test for (postID, testType) whose completed_at is at
// or after `since`. Returns nil + gorm.ErrRecordNotFound when there is no
// match in the window. cloud-gesvial.20: backbone of the dedup-and-merge
// strategy in Service.Report — when the same logical test produces multiple
// physical reports (carrier receipt + post response), the cloud merges the
// new evidence into the existing row instead of inserting a duplicate.
func (r *Repository) SelectMostRecentFinalised(postID string, testType TestType, since time.Time) (*TestResult, error) {
	result := new(TestResult)
	if err := r.db.
		Where("deleted_at IS NULL").
		Where("post_id = ?", postID).
		Where("test_type = ?", testType).
		Where("status IN ?", []TestStatus{TestStatusPassed, TestStatusFailed, TestStatusError}).
		Where("completed_at >= ?", since).
		Order("completed_at DESC").
		First(result).Error; err != nil {
		return nil, err
	}
	return result, nil
}

// UpdateEvidence applies a column-level patch to a TestResult by id. Used by
// Service.mergeEvidenceIntoExisting to enrich an existing row with new
// classification data (delivery_confirmed, delivery_carrier, delivery_at,
// failure_kind, status, error) without replaying GORM associations or
// clobbering audit columns. cloud-gesvial.20.
func (r *Repository) UpdateEvidence(testID string, fields map[string]any) error {
	if testID == "" || len(fields) == 0 {
		return nil
	}
	return r.db.Model(new(TestResult)).Where("id = ?", testID).Updates(fields).Error
}

// SelectRetryCandidates returns tests in the given statuses that have been
// retried less than maxAttempts times AND were created after the cutoff.
// The query uses the composite index `idx_test_results_retry` (status,
// retry_count, created_at) added in cloud-gesvial.19. Excludes tests that
// already have a successor (`superseded_by_test_id` set) to avoid retrying
// the same parent multiple times within one tick. cloud-gesvial.19+.
func (r *Repository) SelectRetryCandidates(ctx context.Context, statuses []TestStatus, maxAttempts uint8, cutoff time.Time) ([]*TestResult, error) {
	results := []*TestResult{}
	if err := r.db.WithContext(ctx).
		Where("deleted_at IS NULL"). // cloud-gesvial.19.1: avoid retrying cancelled/superseded tests
		Where("status IN ?", statuses).
		Where("retry_count < ?", maxAttempts).
		Where("created_at >= ?", cutoff).
		Where("superseded_by_test_id IS NULL").
		Order("created_at ASC").
		Limit(100). // safety cap per tick
		Find(&results).Error; err != nil {
		return nil, err
	}
	return results, nil
}

// UpdateRetryLink atomically writes the retry-trail fields: the new test's
// retry_count and the original's superseded_by_test_id. Both run inside one
// transaction so the audit trail is consistent. cloud-gesvial.19+.
func (r *Repository) UpdateRetryLink(ctx context.Context, original *TestResult, newResult *TestResult) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(newResult).Updates(map[string]any{
			"retry_count": newResult.RetryCount,
		}).Error; err != nil {
			return err
		}
		return tx.Model(original).Updates(map[string]any{
			"superseded_by_test_id": original.SupersededByTestID,
		}).Error
	})
}

// HistoryBucket aggregates test outcomes for one time bucket (day or hour)
// of a single post. cloud-gesvial.19+.
type HistoryBucket struct {
	Bucket  string `json:"bucket"`  // ISO date for day-grain, ISO datetime for hour-grain
	Passed  int64  `json:"passed"`
	Failed  int64  `json:"failed"`
	Error   int64  `json:"error"`
	Pending int64  `json:"pending"`
}

// HistoryGranularity is the resolution of a HistoryBucket.
type HistoryGranularity string

const (
	HistoryGranularityDay  HistoryGranularity = "day"
	HistoryGranularityHour HistoryGranularity = "hour"
)

// SelectHistory aggregates tests for a single post in [from, to] grouped by
// day or hour. Used by the /posts/:id/history endpoint to feed heatmap and
// trend charts. cloud-gesvial.19+.
func (r *Repository) SelectHistory(ctx context.Context, userID, postID string, from, to time.Time, gran HistoryGranularity) ([]HistoryBucket, error) {
	dateExpr := "DATE(COALESCE(completed_at, created_at))"
	if gran == HistoryGranularityHour {
		dateExpr = "DATE_FORMAT(COALESCE(completed_at, created_at), '%Y-%m-%dT%H:00:00Z')"
	}

	type row struct {
		Bucket string
		Status string
		Count  int64
	}
	var rows []row
	// cloud-gesvial.19.1 H-MED-4: filter on the same expression used for the
	// bucket so a test created just before `from` but completed inside the
	// window lands in the bucket where it belongs. Pre-fix the WHERE used
	// only created_at — bucketing on completed_at then dropped those rows.
	q := r.db.WithContext(ctx).
		Model(new(TestResult)).
		Select(dateExpr+" AS bucket, status, COUNT(*) AS count").
		Where("deleted_at IS NULL").
		Where("post_id = ?", postID).
		Where("COALESCE(completed_at, created_at) >= ?", from).
		Where("COALESCE(completed_at, created_at) <= ?", to)
	if userID != "" && userID != "__ADMIN__" {
		q = q.Where("user_id = ?", userID)
	}
	if err := q.Group("bucket, status").Order("bucket ASC").Scan(&rows).Error; err != nil {
		return nil, err
	}

	// Pivot rows into HistoryBucket per bucket.
	byBucket := map[string]*HistoryBucket{}
	for _, row := range rows {
		hb, ok := byBucket[row.Bucket]
		if !ok {
			hb = &HistoryBucket{Bucket: row.Bucket}
			byBucket[row.Bucket] = hb
		}
		switch TestStatus(row.Status) {
		case TestStatusPassed:
			hb.Passed = row.Count
		case TestStatusFailed:
			hb.Failed = row.Count
		case TestStatusError:
			hb.Error = row.Count
		case TestStatusPending:
			hb.Pending = row.Count
		}
	}

	// Output in chronological order (re-sort because map iteration is random).
	buckets := make([]HistoryBucket, 0, len(byBucket))
	for _, hb := range byBucket {
		buckets = append(buckets, *hb)
	}
	// Already sorted by bucket via SQL ORDER BY but the pivot loses order;
	// sort by bucket string (works for ISO day and hour formats).
	for i := 1; i < len(buckets); i++ {
		for j := i; j > 0 && buckets[j-1].Bucket > buckets[j].Bucket; j-- {
			buckets[j-1], buckets[j] = buckets[j], buckets[j-1]
		}
	}
	return buckets, nil
}

// ExpirePending marks every PENDING test whose created_at is older than
// `cutoff` as ERROR with the provided error message, stamping completed_at=NOW.
// Returns the number of affected rows.
func (r *Repository) ExpirePending(ctx context.Context, cutoff time.Time, errorMsg string) (int64, error) {
	res := r.db.WithContext(ctx).
		Model(new(TestResult)).
		Where("deleted_at IS NULL"). // cloud-gesvial.19.1: don't expire already-cancelled rows
		Where("status = ? AND created_at < ?", TestStatusPending, cutoff).
		Updates(map[string]any{
			"status":       TestStatusError,
			"error":        errorMsg,
			"completed_at": time.Now(),
		})
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}