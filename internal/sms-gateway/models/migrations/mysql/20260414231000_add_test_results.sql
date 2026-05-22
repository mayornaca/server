-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS `test_results` (
    `id`                        CHAR(21) NOT NULL,
    `user_id`                   VARCHAR(32) NOT NULL,
    `device_id`                 CHAR(21) NULL,
    `post_id`                   CHAR(21) NOT NULL,
    `test_type`                 VARCHAR(20) NOT NULL,
    `status`                    VARCHAR(10) NOT NULL,
    `call_record_id`            VARCHAR(64) NULL,
    `message_id`                VARCHAR(64) NULL,
    `fft_analysis_json`         TEXT NULL,
    `details`                   TEXT NULL,
    `error`                     TEXT NULL,
    `cloud_verification_status` VARCHAR(20) NULL,
    `cloud_whisper_result_json` TEXT NULL,
    `started_at`                DATETIME(3) NULL,
    `completed_at`              DATETIME(3) NULL,
    `created_at`                DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    `updated_at`                DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    `deleted_at`                DATETIME(3) NULL,
    PRIMARY KEY (`id`),
    INDEX `idx_test_results_user` (`user_id`),
    INDEX `idx_test_results_device` (`device_id`),
    INDEX `idx_test_results_post` (`post_id`),
    INDEX `idx_test_results_type_status` (`test_type`, `status`),
    INDEX `idx_test_results_created` (`created_at`),
    CONSTRAINT `fk_test_results_user` FOREIGN KEY (`user_id`) REFERENCES `users`(`id`) ON DELETE CASCADE,
    CONSTRAINT `fk_test_results_post` FOREIGN KEY (`post_id`) REFERENCES `sos_posts`(`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS `test_results`;
-- +goose StatementEnd
