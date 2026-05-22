package servertasks

import (
	"context"

	"go.uber.org/zap"
)

// runRetryFailed sweeps recent FAILED/ERROR test results and re-schedules
// them up to RetryConfig.MaxAttempts. Idempotent under contention via the
// MySQL lock the runner holds — only one server replica sweeps at a time.
// cloud-gesvial.19+.
func (r *Runner) runRetryFailed(ctx context.Context) {
	count, err := r.testsSvc.RetryFailed(ctx, r.retryCfg)
	if err != nil {
		r.logger.Warn("retry sweep failed", zap.Error(err))
		return
	}
	if count > 0 {
		r.logger.Info("retry sweep complete", zap.Int("retried", count))
	}
}
