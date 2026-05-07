-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS `sos_posts` (
    `id`              CHAR(21) NOT NULL,
    `user_id`         VARCHAR(32) NOT NULL,
    `name`            VARCHAR(128) NOT NULL,
    `km_marker`       VARCHAR(32) NOT NULL,
    `phone_number`    VARCHAR(20) NOT NULL,
    `status`          VARCHAR(20) NOT NULL DEFAULT 'UNKNOWN',
    `last_test_date`  DATETIME(3) NULL,
    `created_at`      DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    `updated_at`      DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    `deleted_at`      DATETIME(3) NULL,
    PRIMARY KEY (`id`),
    INDEX `idx_sos_posts_user` (`user_id`),
    INDEX `idx_sos_posts_status` (`status`),
    UNIQUE INDEX `unq_sos_posts_phone` (`user_id`, `phone_number`),
    CONSTRAINT `fk_sos_posts_user` FOREIGN KEY (`user_id`) REFERENCES `users`(`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS `sos_posts`;
-- +goose StatementEnd