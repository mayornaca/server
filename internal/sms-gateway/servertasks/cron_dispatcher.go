package servertasks

import (
	"context"
	"time"

	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/posts"
	"github.com/android-sms-gateway/server/internal/sms-gateway/modules/schedules"
	"go.uber.org/zap"
)

// runCronDispatcher evaluates enabled schedules whose cron expression fires
// in (lastEval, now] and triggers a batch schedule for each one.
func (r *Runner) runCronDispatcher(ctx context.Context, lastEval, now time.Time) {
	due, err := r.schedulesSvc.FetchDue(ctx, lastEval, now)
	if err != nil {
		r.logger.Warn("failed to fetch due schedules", zap.Error(err))
		return
	}
	if len(due) == 0 {
		return
	}

	r.logger.Info("dispatching cron schedules", zap.Int("count", len(due)))

	for _, sch := range due {
		r.dispatchOne(ctx, sch, now)
	}
}

// dispatchOne handles a single schedule's tick. Critical invariant: MarkRun
// must always run regardless of whether resolution / batch dispatch succeeded
// — otherwise `LastRunAt` stays stale, FetchDue keeps returning the same
// schedule every minute, and the cron expression's time-of-day is silently
// ignored. cloud-gesvial.19 fix: the previous code skipped MarkRun on errors,
// which is exactly the loop the operator reported as "cron schedules no se
// ejecutan" — they fired but never advanced.
func (r *Runner) dispatchOne(ctx context.Context, sch *schedules.TestSchedule, now time.Time) {
	defer func() {
		if markErr := r.schedulesSvc.MarkRun(ctx, sch.ID, now); markErr != nil {
			r.logger.Warn("failed to mark schedule run",
				zap.String("scheduleID", sch.ID),
				zap.Error(markErr))
		}
	}()

	postIDs, err := r.resolvePostIDs(sch)
	if err != nil {
		r.logger.Warn("failed to resolve postIDs",
			zap.String("scheduleID", sch.ID),
			zap.Error(err))
		return
	}
	if len(postIDs) == 0 {
		r.logger.Info("schedule has no matching posts",
			zap.String("scheduleID", sch.ID))
		return
	}

	deviceID := ""
	if sch.DeviceID != nil {
		deviceID = *sch.DeviceID
	}

	// CancelPending=true on the schedule overrides the dedupe so the cron
	// always runs on time, cancelling any in-flight PENDING for the same
	// (post, testType). Default false preserves legacy behaviour where
	// the dedupe blocks duplicate runs (safer for tight schedules).
	results, batchErr := r.testsSvc.ScheduleBatchTest(sch.UserID, postIDs, sch.TestType, deviceID, sch.CancelPending)
	if batchErr != nil {
		r.logger.Warn("batch schedule failed",
			zap.String("scheduleID", sch.ID),
			zap.Error(batchErr))
		return
	}

	created, existing, errs := 0, 0, 0
	for _, it := range results {
		switch it.Status {
		case "PENDING":
			created++
		case "PENDING_EXISTING":
			existing++
		default:
			errs++
		}
	}
	r.logger.Info("schedule fired",
		zap.String("scheduleID", sch.ID),
		zap.String("userID", sch.UserID),
		zap.String("testType", sch.TestType),
		zap.Int("created", created),
		zap.Int("existing", existing),
		zap.Int("errors", errs))
}

// resolvePostIDs returns the set of post IDs to schedule for a given rule,
// respecting OnlyEnabledPosts and FilterPostIDs.
func (r *Runner) resolvePostIDs(sch *schedules.TestSchedule) ([]string, error) {
	var filters []posts.SelectFilter
	if sch.OnlyEnabledPosts {
		filters = append(filters, posts.WithOnlyEnabled())
	}

	userPosts, err := r.postsSvc.Select(sch.UserID, filters...)
	if err != nil {
		return nil, err
	}

	// If FilterPostIDs is specified, intersect with it.
	var allowed map[string]struct{}
	if len(sch.FilterPostIDs) > 0 {
		allowed = make(map[string]struct{}, len(sch.FilterPostIDs))
		for _, id := range sch.FilterPostIDs {
			allowed[id] = struct{}{}
		}
	}

	ids := make([]string, 0, len(userPosts))
	for _, p := range userPosts {
		if allowed != nil {
			if _, ok := allowed[p.ID]; !ok {
				continue
			}
		}
		ids = append(ids, p.ID)
	}
	return ids, nil
}
