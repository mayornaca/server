-- +goose Up
-- +goose StatementBegin
-- cloud-gesvial.18.1: add cancel_pending column to test_schedules.
-- gesvial.17 introduced this field via GORM AutoMigrate but the field was
-- silently skipped (AutoMigrate is conservative with non-NULL bool defaults
-- on existing tables in some MySQL/GORM versions). Goose handles it cleanly.
-- Default false preserves the legacy dedupe behaviour for existing rows.
ALTER TABLE `test_schedules`
    ADD COLUMN `cancel_pending` TINYINT(1) NOT NULL DEFAULT 0 AFTER `only_enabled_posts`;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE `test_schedules` DROP COLUMN `cancel_pending`;
-- +goose StatementEnd
