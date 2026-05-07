-- +goose Up
-- +goose StatementBegin
-- cloud-gesvial.20: dual-evidence classification of SMS tests.
--
-- Hasta gesvial.19 una prueba SMS contra un poste se modelaba como un único
-- evento (status PASSED/FAILED). En realidad el Z5 puede recibir hasta DOS
-- mensajes asíncronos por test:
--   1. Un delivery receipt del carrier (Entel/Claro/Movistar/WOM) — confirma
--      que el SMS llegó al teléfono celular del módulo SOS (capa transporte).
--   2. La respuesta del firmware del poste con su estado (battery, firmware,
--      red, señal) — confirma que el módulo está operativo (capa aplicación).
--
-- Estas 4 columnas separan ambas capas en lugar de colapsarlas en un único
-- status. Todas son NULLable: filas legacy quedan con delivery_confirmed=NULL
-- y se renderizan como "—" en el panel hasta que llegue el primer test
-- post-deploy. No se hace backfill — la información retroactiva no es
-- recuperable.
--
-- Ver `docs/manual-operador.md` sección 6.3 para la matriz de outcomes.
-- Un único ALTER TABLE consolida columnas + índice. Goose pasa el contenido
-- entero del bloque StatementBegin/StatementEnd al driver como una sola
-- query, y el driver `go-sql-driver/mysql` rechaza multi-statement por
-- default — separar en dos `ALTER TABLE` con `;` causa Error 1064.
-- MariaDB acepta sin problema múltiples cláusulas `ADD COLUMN`/`ADD INDEX`
-- en un solo statement.
--
-- Índice compuesto: el dedup window check de Service.Report() busca
-- `(post_id, test_type, completed_at>=cutoff)`; sin índice escanea
-- linealmente y degrada cuando la tabla supera el millón de filas.
ALTER TABLE `test_results`
    ADD COLUMN `delivery_confirmed` TINYINT(1)   NULL AFTER `cloud_whisper_result_json`,
    ADD COLUMN `delivery_at`        DATETIME(3)  NULL AFTER `delivery_confirmed`,
    ADD COLUMN `delivery_carrier`   VARCHAR(32)  NULL AFTER `delivery_at`,
    ADD COLUMN `failure_kind`       VARCHAR(32)  NULL AFTER `delivery_carrier`,
    ADD INDEX  `idx_test_results_dedup` (`post_id`, `test_type`, `completed_at`);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Rollback safe: se preservan los datos hasta confirmar que el rollback es
-- definitivo. Si se requiere drop real, ejecutar manualmente:
--   ALTER TABLE test_results
--     DROP INDEX idx_test_results_dedup,
--     DROP COLUMN failure_kind,
--     DROP COLUMN delivery_carrier,
--     DROP COLUMN delivery_at,
--     DROP COLUMN delivery_confirmed;
SELECT 1;
-- +goose StatementEnd
