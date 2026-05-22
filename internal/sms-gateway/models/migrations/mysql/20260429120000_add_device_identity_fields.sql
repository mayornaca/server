-- +goose Up
-- +goose StatementBegin
-- cloud-gesvial.19.3 D7: identificación humana del gateway. El operador del
-- panel hoy ve sólo `samsung/q5qxxx` + un ID hash (`IYtJS5qQB70qP0Wzyi-Ov`)
-- y no puede distinguir un Z5 de otro al reportar a soporte. Estas columnas
-- (todas NULLable) llegan a poblarse cuando la app Android empieza a
-- reportarlas en `POST /api/mobile/v1/heartbeat` o `POST /api/mobile/v1/register`.
-- Por compat hacia atrás, gateways viejos quedan con NULL hasta el primer
-- heartbeat de la versión nueva de la app.
ALTER TABLE `devices`
    ADD COLUMN `phone_number` VARCHAR(20) NULL AFTER `push_token`,
    ADD COLUMN `model`        VARCHAR(64) NULL AFTER `phone_number`,
    ADD COLUMN `os_version`   VARCHAR(32) NULL AFTER `model`;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Se preserva la columna en caso de rollback para no perder datos. Si se
-- requiere la baja real, ejecutar manualmente:
--   ALTER TABLE devices DROP COLUMN os_version, DROP COLUMN model, DROP COLUMN phone_number;
SELECT 1;
-- +goose StatementEnd
