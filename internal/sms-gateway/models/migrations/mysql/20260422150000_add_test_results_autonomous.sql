-- +goose Up
-- +goose StatementBegin
ALTER TABLE `test_results`
    ADD COLUMN `autonomous` TINYINT(1) NOT NULL DEFAULT 0,
    ADD INDEX `idx_test_results_autonomous` (`autonomous`);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE `test_results`
    DROP INDEX `idx_test_results_autonomous`,
    DROP COLUMN `autonomous`;
-- +goose StatementEnd
