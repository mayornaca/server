-- +goose Up
-- +goose StatementBegin
ALTER TABLE `sos_posts`
    ADD COLUMN `disabled` TINYINT(1) NOT NULL DEFAULT 0,
    ADD INDEX `idx_sos_posts_disabled` (`disabled`);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE `sos_posts`
    DROP INDEX `idx_sos_posts_disabled`,
    DROP COLUMN `disabled`;
-- +goose StatementEnd
