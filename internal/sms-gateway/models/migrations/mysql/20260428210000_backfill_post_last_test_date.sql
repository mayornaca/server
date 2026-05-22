-- +goose Up
-- +goose StatementBegin
-- cloud-gesvial.19.1: backfill sos_posts.last_test_date desde el MAX(created_at)
-- de test_results por poste. La convención (cloud-gesvial.18.3) es "last attempt
-- en cualquier estado", por eso usamos created_at y no completed_at — un test
-- PENDING o ERROR sigue contando como "el sistema lo intentó". El panel mostraba
-- "Sin pruebas aún" para postes que en realidad ya tenían historial porque la
-- lógica de bump fue agregada después y nunca se backfilleó.
UPDATE `sos_posts` p
SET p.`last_test_date` = (
    SELECT MAX(t.`created_at`)
    FROM `test_results` t
    WHERE t.`post_id` = p.`id`
      AND t.`deleted_at` IS NULL
)
WHERE p.`deleted_at` IS NULL
  AND EXISTS (
    SELECT 1 FROM `test_results` t
    WHERE t.`post_id` = p.`id`
      AND t.`deleted_at` IS NULL
  );
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- No-op: revertir el backfill borraría datos válidos.
SELECT 1;
-- +goose StatementEnd
