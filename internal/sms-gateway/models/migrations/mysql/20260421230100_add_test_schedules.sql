-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS `test_schedules` (
    `id`                 CHAR(21) NOT NULL,
    `user_id`            VARCHAR(32) NOT NULL,
    `name`               VARCHAR(64) NOT NULL,
    `cron_expression`    VARCHAR(120) NOT NULL,
    `test_type`          VARCHAR(32) NOT NULL,
    `enabled`            TINYINT(1) NOT NULL DEFAULT 1,
    `only_enabled_posts` TINYINT(1) NOT NULL DEFAULT 1,
    `filter_post_ids`    JSON NULL,
    `device_id`          CHAR(21) NULL,
    `last_run_at`        DATETIME(3) NULL,
    `created_at`         DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    `updated_at`         DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    `deleted_at`         DATETIME(3) NULL,
    PRIMARY KEY (`id`),
    INDEX `idx_test_schedules_user` (`user_id`),
    CONSTRAINT `fk_test_schedules_user` FOREIGN KEY (`user_id`) REFERENCES `users`(`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS `test_schedules`;
-- +goose StatementEnd
