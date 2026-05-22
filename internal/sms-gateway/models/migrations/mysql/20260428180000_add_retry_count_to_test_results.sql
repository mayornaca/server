-- +goose Up
-- +goose StatementBegin
-- cloud-gesvial.19: track retry attempts of failed tests + audit trail of
-- which test superseded a previous one. Both columns are nullable / default
-- 0 so existing rows behave correctly without backfill.
ALTER TABLE `test_results`
    ADD COLUMN `retry_count` TINYINT UNSIGNED NOT NULL DEFAULT 0 AFTER `autonomous`,
    ADD COLUMN `superseded_by_test_id` CHAR(21) NULL AFTER `retry_count`,
    ADD INDEX `idx_test_results_retry` (`status`, `retry_count`, `created_at`);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE `test_results`
    DROP INDEX `idx_test_results_retry`,
    DROP COLUMN `superseded_by_test_id`,
    DROP COLUMN `retry_count`;
-- +goose StatementEnd
